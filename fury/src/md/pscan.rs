// Port scanner subsystem, a faithful port of the Go pscan engine
// (internal/client/handlers/subsystems/pscan/engine): identical CLI,
// identical result JSON/line formats, same probes and limits.

use crate::md::Manifest;
use std::collections::HashMap;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpStream, UdpSocket};
use tokio::sync::mpsc;
use tokio::time::{sleep, Duration, Instant};

const MAX_HOSTS: usize = 1024;
const MAX_PORTS: usize = 65535;
const MAX_WORKERS: usize = 1024;
const MAX_RATE: u32 = 10000;
const MAX_SCAN_DURATION_SECS: u64 = 24 * 60 * 60;
const DEFAULT_TIMEOUT_MS: u64 = 750;
const DEFAULT_WORKERS: usize = 600;
const MAX_WEB_PROBE_BODY: usize = 128 * 1024;

const DEFAULT_PORTS: &str = "21,22,23,25,53,80,88,135-139,161,389,443,445,623,636,860,873,902,10050,10051,10052,1090,1098,1099,1194,1195,1196,1197,1198,1199,1190,1433,1500,1521,1540,1541,1883,2001,2010,2022,2049,2222,2375,2376,2379,2380,2809,3050,3051,3128,3260,3268,3299,3300,3306,3389,3636,3690,4369,4444,4445,4786,4848,4899,4990,5432,5555,5556,5557,5558,5559,5666,5671,5672,5800,5900,5901,5985,6000,6001,6066,6129,6379,6666,7000-7004,7070,7071,7873,7990,8000-8003,8008,8009,8042,8080,8081,8082,8083,8088,8090,8181,8222,8291,8333,8383,8443,8500,8530,8531,8686,8879,8880,8883,8888,8983,9000,9000-9003,9012,9043,9060,9080,9081,9090,9091,9099,9380,9418,9443,9503,9800,9990,10000,10050,10123,10250,10255,10999,11002,11004,11006,11099,11111,15672,15674,15675,15692,25672,27017,27018,45000,45001,47001,47002,50000-50014,50500,61613,61614,61616";

const PRIORITY_WEB_PORTS: &[u16] = &[80, 443, 8080, 8443, 81, 7001, 8000, 8001, 8008, 8081, 8088, 8089, 8888, 9000, 9200, 9443, 10000];

pub fn manifest() -> Manifest {
    let mut m = Manifest::name_only(&crate::ob!("pscan"));
    m.description = crate::ob!("TCP/UDP scanner with HTTP/HTTPS metadata probing on TCP open ports.");
    m.version = crate::ob!("1").to_string();
    m.usage = crate::ob!("pscan -h <host,ip,cidr,...> [-p <port,range,all>] [--udp|--tcp --udp] [-t workers] [-time timeout] [--max-duration duration] [--json]").to_string();
    m.build_tags = vec![crate::ob!("pscan")];
    m.timeout_seconds = -1;
    m.output_bytes = 1024 * 1024;
    m.max_args = 20;
    m
}

#[derive(Clone, Copy, PartialEq)]
enum Protocol {
    Tcp,
    Udp,
}

#[derive(Clone, Default)]
struct Config {
    hosts: Vec<IpAddr>,
    ports: Vec<u16>,
    protocols: Vec<Protocol>,
    timeout: Duration,
    workers: usize,
    rate: u32,
    json: bool,
    max_duration: Duration,
}

#[derive(Clone, Default)]
struct WebInfo {
    url: String,
    scheme: String,
    status_code: i64,
    title: String,
    server: String,
}

#[derive(Clone, Default)]
struct NbInfo {
    hostname: String,
    domain: String,
    mac: String,
}

#[derive(Clone)]
struct ScanResult {
    ip: String,
    port: u16,
    protocol: Protocol,
    state: String,
    open: bool,
    error: String,
    duration_ms: i64,
    web: Option<WebInfo>,
    netbios: Option<NbInfo>,
}

impl ScanResult {
    fn to_json(&self) -> String {
        let mut out = String::from("{");
        out.push_str(&format!("\"ip\":\"{}\",", json_escape(&self.ip)));
        out.push_str(&format!("\"port\":{},", self.port));
        out.push_str(&format!("\"protocol\":\"{}\",", if self.protocol == Protocol::Udp { "udp" } else { "tcp" }));
        if !self.state.is_empty() {
            out.push_str(&format!("\"state\":\"{}\",", json_escape(&self.state)));
        }
        out.push_str(&format!("\"open\":{},", self.open));
        if !self.error.is_empty() {
            out.push_str(&format!("\"error\":\"{}\",", json_escape(&self.error)));
        }
        out.push_str(&format!("\"durationMs\":{}", self.duration_ms));
        if let Some(web) = &self.web {
            out.push_str(",\"web\":{");
            out.push_str(&format!("\"url\":\"{}\",", json_escape(&web.url)));
            out.push_str(&format!("\"scheme\":\"{}\",", json_escape(&web.scheme)));
            if web.status_code != 0 {
                out.push_str(&format!("\"statusCode\":{},", web.status_code));
            }
            if !web.title.is_empty() {
                out.push_str(&format!("\"title\":\"{}\",", json_escape(&web.title)));
            }
            if !web.server.is_empty() {
                out.push_str(&format!("\"server\":\"{}\",", json_escape(&web.server)));
            }
            if out.ends_with(',') {
                out.pop();
            }
            out.push('}');
        }
        if let Some(nb) = &self.netbios {
            out.push_str(",\"netbios\":{");
            let mut parts = Vec::new();
            if !nb.hostname.is_empty() {
                parts.push(format!("\"hostname\":\"{}\"", json_escape(&nb.hostname)));
            }
            if !nb.domain.is_empty() {
                parts.push(format!("\"domain\":\"{}\"", json_escape(&nb.domain)));
            }
            if !nb.mac.is_empty() {
                parts.push(format!("\"mac\":\"{}\"", json_escape(&nb.mac)));
            }
            out.push_str(&parts.join(","));
            out.push('}');
        }
        out.push('}');
        out
    }

    fn to_line(&self) -> String {
        let endpoint = if self.protocol == Protocol::Udp {
            format!("{}:{}/udp", self.ip, self.port)
        } else {
            format!("{}:{}", self.ip, self.port)
        };
        let state = if self.state.is_empty() { "open".to_string() } else { self.state.clone() };
        let mut parts = vec![endpoint, state];
        if let Some(web) = &self.web {
            parts.push(web.scheme.clone());
            if web.status_code > 0 {
                parts.push(format!("status={}", web.status_code));
            }
            if !web.title.is_empty() {
                parts.push(format!("title={}", quote_text(&web.title)));
            }
            if !web.server.is_empty() {
                parts.push(format!("server={}", quote_text(&web.server)));
            }
        }
        if let Some(nb) = &self.netbios {
            if !nb.hostname.is_empty() {
                parts.push(format!("netbios-host={}", quote_text(&nb.hostname)));
            }
            if !nb.domain.is_empty() {
                parts.push(format!("netbios-domain={}", quote_text(&nb.domain)));
            }
            if !nb.mac.is_empty() {
                parts.push(format!("netbios-mac={}", quote_text(&nb.mac)));
            }
        }
        parts.join(" ")
    }
}

fn quote_text(value: &str) -> String {
    format!("\"{}\"", value.replace('\\', "\\\\").replace('"', "\\\""))
}

fn json_escape(value: &str) -> String {
    let mut out = String::with_capacity(value.len());
    for ch in value.chars() {
        match ch {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out
}

// ---------------------------------------------------------------------------
// Argument parsing (mirrors the Go engine's parse.go)
// ---------------------------------------------------------------------------

fn normalize_fscan_args(args: &[String]) -> Vec<String> {
    let mut out = Vec::with_capacity(args.len());
    for arg in args {
        if arg.starts_with("-p") && arg.len() > 2 && !arg[2..].starts_with('-') && arg[2..].starts_with(|c: char| c.is_ascii_digit()) {
            out.push("-p".to_string());
            out.push(arg[2..].to_string());
            continue;
        }
        if arg.starts_with("-h") && arg.len() > 2 && !arg[2..].starts_with('-') && arg != "-help" && arg[2..].starts_with(|c: char| c.is_ascii_digit() || c.is_ascii_alphabetic()) {
            out.push("-h".to_string());
            out.push(arg[2..].to_string());
            continue;
        }
        out.push(arg.clone());
    }
    out
}

fn parse_args(args: &[String]) -> Result<Config, String> {
    let mut hosts_raw = String::new();
    let mut ports_raw = String::new();
    let mut protocol_raw = String::new();
    let mut timeout_raw = String::new();
    let mut duration_raw = String::new();
    let mut exclude_raw = String::new();
    let mut tcp_scan = false;
    let mut udp_scan = false;
    let mut time_seconds: i64 = 0;
    let mut max_time_secs: i64 = 0;

    let mut cfg = Config {
        timeout: Duration::from_millis(DEFAULT_TIMEOUT_MS),
        workers: DEFAULT_WORKERS,
        ..Default::default()
    };

    let args = normalize_fscan_args(args);
    fn next_value(args: &[String], i: &mut usize, inline: Option<String>, name: &str) -> Result<String, String> {
        if let Some(v) = inline {
            return Ok(v);
        }
        *i += 1;
        args.get(*i)
            .cloned()
            .ok_or_else(|| format!("flag needs an argument: {name}"))
    }
    let mut i = 0;
    let mut positional = Vec::new();
    while i < args.len() {
        let (raw_flag, inline_value) = match args[i].split_once('=') {
            Some((f, v)) if f.starts_with('-') => (f.to_string(), Some(v.to_string())),
            _ => (args[i].clone(), None),
        };
        if !raw_flag.starts_with('-') {
            positional.push(raw_flag);
            i += 1;
            continue;
        }
        // Go's flag package accepts both -flag and --flag.
        let flag = raw_flag.trim_start_matches('-').to_string();
        let flag_str = flag.as_str();
        match flag_str {
            "h" | "ips" => hosts_raw = next_value(&args, &mut i, inline_value, flag_str)?,
            "p" | "ports" => ports_raw = next_value(&args, &mut i, inline_value, flag_str)?,
            "proto" | "protocol" => protocol_raw = next_value(&args, &mut i, inline_value, flag_str)?,
            "tcp" => tcp_scan = true,
            "udp" | "u" => udp_scan = true,
            "timeout" => timeout_raw = next_value(&args, &mut i, inline_value, flag_str)?,
            "time" => {
                let v = next_value(&args, &mut i, inline_value, flag_str)?;
                time_seconds = v.parse::<i64>().map_err(|_| format!("invalid value for -time: {}", v))?;
            }
            "max-duration" | "duration" => duration_raw = next_value(&args, &mut i, inline_value, flag_str)?,
            "max-time" => {
                let v = next_value(&args, &mut i, inline_value, flag_str)?;
                max_time_secs = v.parse::<i64>().map_err(|_| format!("invalid value for --max-time: {}", v))?;
            }
            "t" | "workers" => {
                let v = next_value(&args, &mut i, inline_value, flag_str)?;
                cfg.workers = v.parse::<usize>().map_err(|_| format!("invalid value for --workers: {}", v))?;
            }
            "rate" => {
                let v = next_value(&args, &mut i, inline_value, flag_str)?;
                cfg.rate = v.parse::<u32>().map_err(|_| format!("invalid value for --rate: {}", v))?;
            }
            "exclude" | "hn" => exclude_raw = next_value(&args, &mut i, inline_value, flag_str)?,
            "json" => cfg.json = true,
            other => {
                return Err(format!("flag provided but not defined: -{}", other));
            }
        }
        i += 1;
    }
    if !positional.is_empty() {
        return Err(format!("unexpected positional arguments: {}", positional.join(" ")));
    }

    if hosts_raw.trim().is_empty() {
        return Err("-h/--ips is required".to_string());
    }
    if ports_raw.trim().is_empty() {
        ports_raw = DEFAULT_PORTS.to_string();
    }

    if !timeout_raw.trim().is_empty() {
        let duration = parse_go_duration(&timeout_raw).map_err(|e| format!("invalid --timeout: {}", e))?;
        if duration.is_zero() {
            return Err("--timeout must be positive".to_string());
        }
        cfg.timeout = duration;
    }
    if time_seconds < 0 {
        return Err("-time must be positive".to_string());
    }
    if time_seconds > 0 {
        cfg.timeout = Duration::from_secs(time_seconds as u64);
    }

    let mut max_duration = Duration::ZERO;
    if !duration_raw.trim().is_empty() {
        max_duration = parse_go_duration(&duration_raw).map_err(|e| format!("invalid --max-duration: {}", e))?;
    }
    if max_time_secs < 0 {
        return Err("--max-time must be positive".to_string());
    }
    if max_time_secs > 0 {
        max_duration = Duration::from_secs(max_time_secs as u64);
    }
    if !max_duration.is_zero() && max_duration > Duration::from_secs(MAX_SCAN_DURATION_SECS) {
        return Err(format!("--max-duration exceeds maximum of {}", format_go_duration(Duration::from_secs(MAX_SCAN_DURATION_SECS))));
    }
    cfg.max_duration = max_duration;

    let mut protocols = Vec::new();
    for item in split_csv(&protocol_raw) {
        match item.to_lowercase().as_str() {
            "tcp" => protocols.push(Protocol::Tcp),
            "udp" => protocols.push(Protocol::Udp),
            "both" | "all" => {
                protocols.push(Protocol::Tcp);
                protocols.push(Protocol::Udp);
            }
            other => return Err(format!("unsupported protocol \"{}\"", other)),
        }
    }
    if tcp_scan {
        protocols.push(Protocol::Tcp);
    }
    if udp_scan {
        protocols.push(Protocol::Udp);
    }
    if protocols.is_empty() {
        protocols.push(Protocol::Tcp);
    }
    cfg.protocols = protocols;

    if cfg.workers == 0 {
        return Err("--workers must be positive".to_string());
    }
    if cfg.workers > MAX_WORKERS {
        return Err(format!("--workers exceeds maximum of {}", MAX_WORKERS));
    }
    if cfg.rate > MAX_RATE {
        return Err(format!("--rate exceeds maximum of {}", MAX_RATE));
    }

    let exclusions = parse_exclusions(&exclude_raw)?;
    cfg.hosts = parse_hosts(&hosts_raw, &exclusions)?;
    if cfg.hosts.is_empty() {
        return Err("no hosts remain after exclusions".to_string());
    }
    if cfg.hosts.len() > MAX_HOSTS {
        return Err(format!("host count {} exceeds maximum of {}", cfg.hosts.len(), MAX_HOSTS));
    }

    cfg.ports = parse_ports(&ports_raw)?;
    if cfg.ports.is_empty() {
        return Err("no ports selected".to_string());
    }
    if cfg.ports.len() > MAX_PORTS {
        return Err(format!("port count {} exceeds maximum of {}", cfg.ports.len(), MAX_PORTS));
    }

    Ok(cfg)
}

struct Exclusions {
    addrs: Vec<IpAddr>,
    prefixes: Vec<(IpAddr, u8)>,
}

fn parse_exclusions(raw: &str) -> Result<Exclusions, String> {
    let mut out = Exclusions { addrs: Vec::new(), prefixes: Vec::new() };
    for item in split_csv(raw) {
        if item.contains('/') {
            let (addr, mask) = parse_cidr(&item).map_err(|e| format!("invalid --exclude prefix \"{}\": {}", item, e))?;
            out.prefixes.push((addr, mask));
        } else {
            let addr: IpAddr = item.parse().map_err(|e| format!("invalid --exclude IP \"{}\": {}", item, e))?;
            out.addrs.push(addr);
        }
    }
    Ok(out)
}

fn is_excluded(addr: IpAddr, exclusions: &Exclusions) -> bool {
    if exclusions.addrs.contains(&addr) {
        return true;
    }
    for (net, mask) in &exclusions.prefixes {
        if cidr_contains(*net, *mask, addr) {
            return true;
        }
    }
    false
}

fn parse_hosts(raw: &str, exclusions: &Exclusions) -> Result<Vec<IpAddr>, String> {
    let mut seen = std::collections::HashSet::new();
    let mut hosts: Vec<IpAddr> = Vec::new();
    for item in split_csv(raw) {
        if item.contains('/') {
            let (base, mask) = parse_cidr(&item).map_err(|e| format!("invalid --ips prefix \"{}\": {}", item, e))?;
            for addr in iter_cidr(base, mask)? {
                push_host(&mut hosts, &mut seen, addr, exclusions)?;
            }
        } else if let Ok(addr) = item.parse::<IpAddr>() {
            push_host(&mut hosts, &mut seen, addr, exclusions)?;
        } else {
            let resolved = resolve_host(&item).map_err(|e| format!("invalid --ips host/IP \"{}\": {}", item, e))?;
            for addr in resolved {
                push_host(&mut hosts, &mut seen, addr, exclusions)?;
            }
        }
    }
    hosts.sort();
    Ok(hosts)
}

fn push_host(
    hosts: &mut Vec<IpAddr>,
    seen: &mut std::collections::HashSet<IpAddr>,
    addr: IpAddr,
    exclusions: &Exclusions,
) -> Result<(), String> {
    if is_excluded(addr, exclusions) {
        return Ok(());
    }
    if seen.insert(addr) {
        hosts.push(addr);
    }
    if hosts.len() > MAX_HOSTS {
        return Err(format!("host count exceeds maximum of {}", MAX_HOSTS));
    }
    Ok(())
}

async fn resolve_host_async(host: &str) -> Result<Vec<IpAddr>, String> {
    let mut result = Vec::new();
    for candidate in [format!("{}:0", host), format!("[{}]:0", host)] {
        if let Ok(mut addrs) = tokio::net::lookup_host(candidate).await {
            while let Some(sock) = addrs.next() {
                result.push(sock.ip());
            }
            if !result.is_empty() {
                break;
            }
        }
    }
    result.sort();
    result.dedup();
    if result.is_empty() {
        return Err("no addresses resolved".to_string());
    }
    Ok(result)
}

fn resolve_host(host: &str) -> Result<Vec<IpAddr>, String> {
    tokio::task::block_in_place(|| {
        let rt = tokio::runtime::Handle::current();
        rt.block_on(resolve_host_async(host))
    })
}

fn parse_ports(raw: &str) -> Result<Vec<u16>, String> {
    let mut seen = std::collections::HashSet::new();
    for item in split_csv(raw) {
        if item.eq_ignore_ascii_case("all") {
            for port in 1..=65535u16 {
                seen.insert(port);
            }
            continue;
        }
        let (start_text, end_text) = match item.split_once('-') {
            Some((s, e)) => (s, Some(e)),
            None => (item.as_str(), None),
        };
        let start = parse_port(start_text)?;
        let end = match end_text {
            Some(text) => parse_port(text)?,
            None => start,
        };
        if end < start {
            return Err(format!("invalid port range \"{}\"", item));
        }
        for port in start..=end {
            seen.insert(port);
            if seen.len() > MAX_PORTS {
                return Err(format!("port count exceeds maximum of {}", MAX_PORTS));
            }
        }
    }
    let mut ports: Vec<u16> = seen.into_iter().collect();
    ports.sort_by(|a, b| {
        let a_priority = PRIORITY_WEB_PORTS.contains(a);
        let b_priority = PRIORITY_WEB_PORTS.contains(b);
        match (a_priority, b_priority) {
            (true, false) => std::cmp::Ordering::Less,
            (false, true) => std::cmp::Ordering::Greater,
            _ => a.cmp(b),
        }
    });
    Ok(ports)
}

fn parse_port(raw: &str) -> Result<u16, String> {
    let port: u32 = raw.trim().parse().map_err(|_| format!("invalid port \"{}\"", raw))?;
    if !(1..=65535).contains(&port) {
        return Err(format!("port {} is outside 1-65535", port));
    }
    Ok(port as u16)
}

fn split_csv(raw: &str) -> Vec<String> {
    raw.split(',')
        .map(|part| part.trim())
        .filter(|part| !part.is_empty())
        .map(|part| part.to_string())
        .collect()
}

fn parse_cidr(item: &str) -> Result<(IpAddr, u8), String> {
    let (addr_text, mask_text) = item.split_once('/').ok_or_else(|| "missing prefix length".to_string())?;
    let addr: IpAddr = addr_text.parse().map_err(|e| format!("invalid address: {}", e))?;
    let mask: u8 = mask_text.parse().map_err(|_| format!("invalid prefix length \"{}\"", mask_text))?;
    let max = if addr.is_ipv4() { 32 } else { 128 };
    if mask > max {
        return Err(format!("prefix length {} exceeds {}", mask, max));
    }
    Ok((addr, mask))
}

fn cidr_contains(base: IpAddr, mask: u8, addr: IpAddr) -> bool {
    match (base, addr) {
        (IpAddr::V4(b), IpAddr::V4(a)) => {
            if mask == 0 {
                return true;
            }
            let m = u32::MAX << (32 - mask);
            (u32::from(b) & m) == (u32::from(a) & m)
        }
        (IpAddr::V6(b), IpAddr::V6(a)) => {
            if mask == 0 {
                return true;
            }
            let m = u128::MAX << (128 - mask);
            (u128::from(b) & m) == (u128::from(a) & m)
        }
        _ => false,
    }
}

fn iter_cidr(base: IpAddr, mask: u8) -> Result<Vec<IpAddr>, String> {
    let mut out: Vec<IpAddr> = Vec::new();
    match base {
        IpAddr::V4(v4) => {
            let m: u32 = if mask == 0 { 0 } else { u32::MAX << (32 - mask) };
            let network = u32::from(v4) & m;
            for offset in 0u32.. {
                out.push(IpAddr::V4(Ipv4Addr::from(network.wrapping_add(offset))));
                if out.len() > MAX_HOSTS {
                    return Err(format!("host count exceeds maximum of {}", MAX_HOSTS));
                }
                if mask < 32 && offset >= (1u32 << (32 - mask)) - 1 {
                    break;
                }
                if mask == 32 && offset >= 1 {
                    break;
                }
                if mask == 0 && offset == u32::MAX {
                    break;
                }
            }
        }
        IpAddr::V6(v6) => {
            let m: u128 = if mask == 0 { 0 } else { u128::MAX << (128 - mask as u32) };
            let network = u128::from(v6) & m;
            let count: u128 = if mask == 128 { 1 } else { 1u128 << (128 - mask as u32) };
            for offset in 0..count {
                out.push(IpAddr::V6(Ipv6Addr::from(network + offset)));
                if out.len() > MAX_HOSTS {
                    return Err(format!("host count exceeds maximum of {}", MAX_HOSTS));
                }
            }
        }
    }
    Ok(out)
}

/// Go-style duration parsing (ns, us, ms, s, m, h with fractions).
fn parse_go_duration(value: &str) -> Result<Duration, String> {
    let raw = value.trim();
    if raw.is_empty() {
        return Err("empty duration".to_string());
    }
    let negative = raw.starts_with('-');
    let raw = raw.trim_start_matches(['-', '+']);
    if raw == "0" {
        return Ok(Duration::ZERO);
    }
    if raw.is_empty() {
        return Err("invalid duration".to_string());
    }
    let mut total_ns: f64 = 0.0;
    let mut rest = raw;
    let mut matched = false;
    while !rest.is_empty() {
        let digits_end = rest.find(|c: char| !c.is_ascii_digit() && c != '.').unwrap_or(rest.len());
        if digits_end == 0 {
            return Err(format!("invalid duration \"{}\"", value));
        }
        let number: f64 = rest[..digits_end].parse().map_err(|_| format!("invalid duration \"{}\"", value))?;
        rest = &rest[digits_end..];
        let unit_end = rest.find(|c: char| c.is_ascii_digit() || c == '.').unwrap_or(rest.len());
        let unit = &rest[..unit_end];
        rest = &rest[unit_end..];
        let factor: f64 = match unit {
            "ns" => 1.0,
            "us" | "\u{00b5}s" | "\u{03bc}s" => 1_000.0,
            "ms" => 1_000_000.0,
            "s" => 1_000_000_000.0,
            "m" => 60.0 * 1_000_000_000.0,
            "h" => 3_600.0 * 1_000_000_000.0,
            "" => return Err(format!("missing unit in duration \"{}\"", value)),
            _ => return Err(format!("unknown unit \"{}\" in duration \"{}\"", unit, value)),
        };
        total_ns += number * factor;
        matched = true;
    }
    if !matched {
        return Err(format!("invalid duration \"{}\"", value));
    }
    if negative {
        return Err("negative duration".to_string());
    }
    Ok(Duration::from_nanos(total_ns as u64))
}

/// Go-style duration formatting (e.g. 30m0s, 1h30m0s, 750ms).
fn format_go_duration(duration: Duration) -> String {
    let total_secs = duration.as_secs();
    let hours = total_secs / 3600;
    let minutes = (total_secs % 3600) / 60;
    let seconds = total_secs % 60;
    if total_secs == 0 {
        return format!("{}ms", duration.as_millis());
    }
    let mut out = String::new();
    if hours > 0 {
        out.push_str(&format!("{}h", hours));
    }
    if hours > 0 || minutes > 0 {
        out.push_str(&format!("{}m", minutes));
    }
    out.push_str(&format!("{}s", seconds));
    out
}

// ---------------------------------------------------------------------------
// Scanning
// ---------------------------------------------------------------------------

struct RateLimiter {
    interval: Option<Duration>,
    next: Mutex<Instant>,
}

impl RateLimiter {
    fn new(rate: u32) -> Self {
        RateLimiter {
            interval: if rate > 0 { Some(Duration::from_nanos(1_000_000_000 / rate as u64)) } else { None },
            next: Mutex::new(Instant::now()),
        }
    }

    async fn wait(&self) {
        let interval = match self.interval {
            Some(interval) => interval,
            None => return,
        };
        loop {
            let now = Instant::now();
            let wait = {
                let mut next = self.next.lock().unwrap();
                if now >= *next {
                    *next = now + interval;
                    return;
                }
                *next = *next + interval;
                *next - now
            };
            sleep(wait).await;
        }
    }
}

pub async fn run<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    args: &[String],
    io: &mut crate::md::ModuleIo<S>,
) -> Result<(), String> {
    let cfg = parse_args(args)?;
    let (tx, mut rx) = mpsc::unbounded_channel::<ScanResult>();
    let limiter = Arc::new(RateLimiter::new(cfg.rate));
    let cancelled = Arc::new(AtomicBool::new(false));
    let emitted_bytes = Arc::new(std::sync::atomic::AtomicUsize::new(0));

    let scan_cfg = cfg.clone();
    let scan_cancelled = cancelled.clone();
    let producer = tokio::spawn(async move {
        scan(scan_cfg, limiter, tx, scan_cancelled, emitted_bytes).await
    });

    while let Some(result) = rx.recv().await {
        let payload = if cfg.json {
            format!("{}\n", result.to_json())
        } else if result.open {
            format!("{}\n", result.to_line())
        } else {
            continue;
        };
        if io.writer.data_bytes(payload.into_bytes()).await.is_err() {
            cancelled.store(true, Ordering::SeqCst);
            break;
        }
    }

    match producer.await {
        Ok(result) => result,
        Err(e) => Err(format!("scan task failed: {}", e)),
    }
}

async fn scan(
    cfg: Config,
    limiter: Arc<RateLimiter>,
    tx: mpsc::UnboundedSender<ScanResult>,
    cancelled: Arc<AtomicBool>,
    emitted_bytes: Arc<std::sync::atomic::AtomicUsize>,
) -> Result<(), String> {
    const MAX_EMITTED_BYTES: usize = 8 * 1024 * 1024;
    let deadline = if cfg.max_duration > Duration::ZERO {
        Some(tokio::time::Instant::now() + cfg.max_duration)
    } else {
        None
    };

    let (job_tx, job_rx) = mpsc::unbounded_channel::<(IpAddr, u16, Protocol)>();
    let job_rx = Arc::new(tokio::sync::Mutex::new(job_rx));

    let mut workers = Vec::with_capacity(cfg.workers);
    for _ in 0..cfg.workers {
        let rx = job_rx.clone();
        let tx = tx.clone();
        let limiter = limiter.clone();
        let timeout = cfg.timeout;
        let cancelled = cancelled.clone();
        let emitted = emitted_bytes.clone();
        workers.push(tokio::spawn(async move {
            loop {
                if cancelled.load(Ordering::SeqCst) {
                    break;
                }
                if emitted.load(Ordering::SeqCst) > MAX_EMITTED_BYTES {
                    break;
                }
                let job = {
                    let mut guard = rx.lock().await;
                    match guard.try_recv() {
                        Ok(job) => job,
                        Err(mpsc::error::TryRecvError::Disconnected) => break,
                        Err(mpsc::error::TryRecvError::Empty) => {
                            drop(guard);
                            tokio::task::yield_now().await;
                            continue;
                        }
                    }
                };
                limiter.wait().await;
                let result = scan_one(timeout, job.0, job.1, job.2).await;
                emitted.fetch_add(160, Ordering::SeqCst);
                if tx.send(result).is_err() {
                    break;
                }
            }
        }));
    }

    let mut deadline_exceeded = false;
    for host in &cfg.hosts {
        for port in &cfg.ports {
            for protocol in &cfg.protocols {
                if emitted_bytes.load(Ordering::SeqCst) > MAX_EMITTED_BYTES
                    || cancelled.load(Ordering::SeqCst)
                {
                    break;
                }
                if let Some(deadline) = deadline {
                    if tokio::time::Instant::now() >= deadline {
                        deadline_exceeded = true;
                        break;
                    }
                }
                if job_tx.send((*host, *port, *protocol)).is_err() {
                    break;
                }
            }
        }
    }
    drop(job_tx);
    for worker in workers {
        let _ = worker.await;
    }
    drop(tx);

    if deadline_exceeded && cfg.max_duration > Duration::ZERO {
        return Err(format!("scan exceeded --max-duration {}", format_go_duration(cfg.max_duration)));
    }
    Ok(())
}

async fn scan_one(timeout: Duration, host: IpAddr, port: u16, protocol: Protocol) -> ScanResult {
    match protocol {
        Protocol::Udp => scan_udp(timeout, host, port).await,
        Protocol::Tcp => scan_tcp(timeout, host, port).await,
    }
}

async fn scan_tcp(timeout: Duration, host: IpAddr, port: u16) -> ScanResult {
    let start = Instant::now();
    let address = std::net::SocketAddr::new(host, port);
    let connect = tokio::time::timeout(timeout, TcpStream::connect(address)).await;
    let mut result = ScanResult {
        ip: host.to_string(),
        port,
        protocol: Protocol::Tcp,
        state: "closed".to_string(),
        open: false,
        error: String::new(),
        duration_ms: start.elapsed().as_millis() as i64,
        web: None,
        netbios: None,
    };
    match connect {
        Ok(Ok(_stream)) => {
            result.open = true;
            result.state = "open".to_string();
            if let Some(web) = probe_web(host, port, timeout).await {
                result.web = Some(web);
            }
            if port == 445 {
                if let Some(nb) = probe_netbios(host, timeout).await {
                    result.netbios = Some(nb);
                }
            }
        }
        Ok(Err(error)) => {
            result.error = error.to_string();
        }
        Err(_) => {
            result.error = "i/o timeout".to_string();
        }
    }
    result
}

struct UdpProbe {
    payload: Vec<u8>,
    netbios_trans_id: Option<u16>,
}

fn udp_probe_for_port(port: u16) -> UdpProbe {
    match port {
        53 => UdpProbe {
            payload: vec![
                0x70, 0x53, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01,
            ],
            netbios_trans_id: None,
        },
        123 => {
            let mut packet = vec![0u8; 48];
            packet[0] = 0x1b;
            UdpProbe { payload: packet, netbios_trans_id: None }
        }
        137 => {
            let transaction_id = (std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map(|d| d.subsec_nanos() as u64)
                .unwrap_or(0)
                & 0xffff) as u16;
            UdpProbe {
                payload: build_nbns_node_status_request(transaction_id),
                netbios_trans_id: Some(transaction_id),
            }
        }
        161 => UdpProbe {
            payload: vec![
                0x30, 0x26, 0x02, 0x01, 0x01, 0x04, 0x06, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63, 0xa0, 0x19, 0x02, 0x04,
                0x70, 0x73, 0x63, 0x6e, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00, 0x30, 0x0b, 0x30, 0x09, 0x06, 0x05, 0x2b,
                0x06, 0x01, 0x02, 0x01, 0x05, 0x00,
            ],
            netbios_trans_id: None,
        },
        1900 => UdpProbe {
            payload: crate::ob!("M-SEARCH * HTTP/1.1\r\nHOST:239.255.255.250:1900\r\nMAN:\"ssdp:discover\"\r\nMX:1\r\nST:ssdp:all\r\n\r\n")
                .into_bytes(),
            netbios_trans_id: None,
        },
        _ => UdpProbe { payload: vec![0u8], netbios_trans_id: None },
    }
}

async fn scan_udp(timeout: Duration, host: IpAddr, port: u16) -> ScanResult {
    let start = Instant::now();
    let mut result = ScanResult {
        ip: host.to_string(),
        port,
        protocol: Protocol::Udp,
        state: "open|filtered".to_string(),
        open: false,
        error: String::new(),
        duration_ms: 0,
        web: None,
        netbios: None,
    };

    let local: std::net::SocketAddr = if host.is_ipv4() {
        std::net::SocketAddr::new(IpAddr::V4(Ipv4Addr::UNSPECIFIED), 0)
    } else {
        std::net::SocketAddr::new(IpAddr::V6(Ipv6Addr::UNSPECIFIED), 0)
    };
    let socket = match UdpSocket::bind(local).await {
        Ok(socket) => socket,
        Err(error) => {
            result.duration_ms = start.elapsed().as_millis() as i64;
            result.state = "closed".to_string();
            result.error = error.to_string();
            return result;
        }
    };

    let probe = udp_probe_for_port(port);
    if socket.send_to(&probe.payload, std::net::SocketAddr::new(host, port)).await.is_err() {
        result.duration_ms = start.elapsed().as_millis() as i64;
        result.state = "closed".to_string();
        return result;
    }

    let mut buffer = vec![0u8; 1500];
    let read = tokio::time::timeout(timeout, socket.recv(&mut buffer)).await;
    result.duration_ms = start.elapsed().as_millis() as i64;
    match read {
        Ok(Ok(n)) => {
            result.open = true;
            result.state = "open".to_string();
            if probe.netbios_trans_id.is_some() {
                if let Some(nb) = parse_nbns_node_status_response(&buffer[..n], probe.netbios_trans_id.unwrap()) {
                    result.netbios = Some(nb);
                }
            }
        }
        Ok(Err(error)) => {
            result.state = udp_state_from_error(&error);
            if result.state != "open|filtered" {
                result.error = error.to_string();
            }
        }
        Err(_) => {
            result.state = "open|filtered".to_string();
        }
    }
    result
}

fn udp_state_from_error(error: &std::io::Error) -> String {
    if error.kind() == std::io::ErrorKind::WouldBlock || error.kind() == std::io::ErrorKind::TimedOut {
        return "open|filtered".to_string();
    }
    let message = error.to_string().to_lowercase();
    if message.contains("connection refused") || message.contains("port unreachable") {
        return "closed".to_string();
    }
    "open|filtered".to_string()
}

// ---------------------------------------------------------------------------
// HTTP(S) metadata probing
// ---------------------------------------------------------------------------

fn web_probe_schemes(port: u16) -> [&'static str; 2] {
    match port {
        443 | 8443 | 9443 => ["https", "http"],
        _ => ["http", "https"],
    }
}

async fn probe_web(host: IpAddr, port: u16, timeout: Duration) -> Option<WebInfo> {
    for scheme in web_probe_schemes(port) {
        if let Some(info) = probe_web_scheme(host, port, scheme, timeout).await {
            return Some(info);
        }
    }
    None
}

async fn probe_web_scheme(host: IpAddr, port: u16, scheme: &str, timeout: Duration) -> Option<WebInfo> {
    let host_header = if host.is_ipv6() { format!("[{}]:{}", host, port) } else { format!("{}:{}", host, port) };
    let url = format!("{}://{}/", scheme, host_header);

    let request = format!(
        "GET / HTTP/1.1\r\nHost: {}\r\nUser-Agent: {}\r\nAccept: */*\r\nConnection: close\r\n\r\n",
        host_header,
        crate::ob!("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
    );

    enum Stream {
        Plain(TcpStream),
        Tls(Box<crate::tl::TlsStream>),
    }
    impl tokio::io::AsyncRead for Stream {
        fn poll_read(
            mut self: std::pin::Pin<&mut Self>,
            cx: &mut std::task::Context<'_>,
            buf: &mut tokio::io::ReadBuf<'_>,
        ) -> std::task::Poll<std::io::Result<()>> {
            match &mut *self {
                Stream::Plain(stream) => std::pin::Pin::new(stream).poll_read(cx, buf),
                Stream::Tls(stream) => std::pin::Pin::new(stream.as_mut()).poll_read(cx, buf),
            }
        }
    }
    impl tokio::io::AsyncWrite for Stream {
        fn poll_write(
            mut self: std::pin::Pin<&mut Self>,
            cx: &mut std::task::Context<'_>,
            buf: &[u8],
        ) -> std::task::Poll<std::io::Result<usize>> {
            match &mut *self {
                Stream::Plain(stream) => std::pin::Pin::new(stream).poll_write(cx, buf),
                Stream::Tls(stream) => std::pin::Pin::new(stream.as_mut()).poll_write(cx, buf),
            }
        }
        fn poll_flush(mut self: std::pin::Pin<&mut Self>, cx: &mut std::task::Context<'_>) -> std::task::Poll<std::io::Result<()>> {
            match &mut *self {
                Stream::Plain(stream) => std::pin::Pin::new(stream).poll_flush(cx),
                Stream::Tls(stream) => std::pin::Pin::new(stream.as_mut()).poll_flush(cx),
            }
        }
        fn poll_shutdown(mut self: std::pin::Pin<&mut Self>, cx: &mut std::task::Context<'_>) -> std::task::Poll<std::io::Result<()>> {
            match &mut *self {
                Stream::Plain(stream) => std::pin::Pin::new(stream).poll_shutdown(cx),
                Stream::Tls(stream) => std::pin::Pin::new(stream.as_mut()).poll_shutdown(cx),
            }
        }
    }

    let mut stream: Stream = if scheme == "https" {
        let tls = tokio::time::timeout(timeout, crate::tl::connect(&host.to_string(), port, &host.to_string())).await.ok()?.ok()?;
        Stream::Tls(Box::new(tls))
    } else {
        let tcp = tokio::time::timeout(timeout, TcpStream::connect(std::net::SocketAddr::new(host, port))).await.ok()?.ok()?;
        Stream::Plain(tcp)
    };

    let written = stream.write_all(request.as_bytes()).await;
    if written.is_err() {
        return None;
    }
    let _ = stream.flush().await;

    let mut raw = Vec::new();
    let mut chunk = vec![0u8; 8192];
    loop {
        let read = tokio::time::timeout(timeout, stream.read(&mut chunk)).await;
        match read {
            Ok(Ok(0)) | Err(_) | Ok(Err(_)) => break,
            Ok(Ok(n)) => {
                raw.extend_from_slice(&chunk[..n]);
                if raw.len() >= MAX_WEB_PROBE_BODY * 2 {
                    break;
                }
            }
        }
    }
    if raw.is_empty() {
        return None;
    }

    let text = String::from_utf8_lossy(&raw).to_string();
    let mut lines = text.split("\r\n");
    let status_line = lines.next().unwrap_or_default().to_string();
    let status_code: i64 = status_line
        .split_whitespace()
        .nth(1)
        .and_then(|code| code.parse().ok())
        .unwrap_or(0);
    if status_code == 0 {
        return None;
    }

    let mut server = String::new();
    for line in lines {
        if line.is_empty() {
            break;
        }
        if let Some((name, value)) = line.split_once(':') {
            if name.trim().eq_ignore_ascii_case("Server") {
                server = value.trim().to_string();
            }
        }
    }

    let body_start = text.find("\r\n\r\n").map(|index| index + 4).unwrap_or(0);
    let body = &text[body_start..];
    let title = extract_title(body);

    Some(WebInfo {
        url,
        scheme: scheme.to_string(),
        status_code,
        title,
        server,
    })
}

fn extract_title(body: &str) -> String {
    let lower = body.to_lowercase();
    let start = match lower.find("<title") {
        Some(index) => index,
        None => return String::new(),
    };
    let rest = &body[start..];
    let content_start = match rest.find('>') {
        Some(index) => index + 1,
        None => return String::new(),
    };
    let content = &rest[content_start..];
    let end = match content.to_lowercase().find("</title") {
        Some(index) => index,
        None => return String::new(),
    };
    let title = html_unescape(&content[..end]);
    let collapsed: String = title.split_whitespace().collect::<Vec<&str>>().join(" ");
    if collapsed.len() > 160 {
        collapsed[..160].to_string()
    } else {
        collapsed
    }
}

fn html_unescape(input: &str) -> String {
    let named: HashMap<&str, &str> = HashMap::from([
        ("amp", "&"), ("lt", "<"), ("gt", ">"), ("quot", "\""), ("apos", "'"),
        ("nbsp", " "), ("copy", "\u{00a9}"), ("reg", "\u{00ae}"), ("trade", "\u{2122}"),
        ("hellip", "\u{2026}"), ("mdash", "\u{2014}"), ("ndash", "\u{2013}"),
        ("lsquo", "\u{2018}"), ("rsquo", "\u{2019}"), ("ldquo", "\u{201c}"), ("rdquo", "\u{201d}"),
        ("laquo", "\u{00ab}"), ("raquo", "\u{00bb}"), ("deg", "\u{00b0}"), ("plusmn", "\u{00b1}"),
        ("para", "\u{00b6}"), ("middot", "\u{00b7}"), ("bull", "\u{2022}"),
        ("dagger", "\u{2020}"), ("Dagger", "\u{2021}"), ("permil", "\u{2030}"),
        ("euro", "\u{20ac}"), ("pound", "\u{00a3}"), ("yen", "\u{00a5}"), ("cent", "\u{00a2}"),
        ("sect", "\u{00a7}"), ("times", "\u{00d7}"), ("divide", "\u{00f7}"),
    ]);

    let mut out = String::with_capacity(input.len());
    let mut chars = input.char_indices().peekable();
    while let Some((index, ch)) = chars.next() {
        if ch != '&' {
            out.push(ch);
            continue;
        }
        let rest = &input[index + 1..];
        if let Some(semi) = rest.find(';') {
            let entity = &rest[..semi];
            if let Some(decoded) = named.get(entity) {
                out.push_str(decoded);
                for _ in 0..(semi + 1) {
                    chars.next();
                }
                continue;
            }
            if let Some(hex) = entity.strip_prefix("#x").or_else(|| entity.strip_prefix("#X")) {
                if let Ok(code) = u32::from_str_radix(hex, 16) {
                    if let Some(decoded) = char::from_u32(code) {
                        out.push(decoded);
                        for _ in 0..(semi + 1) {
                            chars.next();
                        }
                        continue;
                    }
                }
            } else if let Some(dec) = entity.strip_prefix('#') {
                if let Ok(code) = dec.parse::<u32>() {
                    if let Some(decoded) = char::from_u32(code) {
                        out.push(decoded);
                        for _ in 0..(semi + 1) {
                            chars.next();
                        }
                        continue;
                    }
                }
            }
        }
        out.push('&');
    }
    out
}

// ---------------------------------------------------------------------------
// NetBIOS name service probing
// ---------------------------------------------------------------------------

fn build_nbns_node_status_request(transaction_id: u16) -> Vec<u8> {
    let mut packet = Vec::with_capacity(50);
    packet.extend_from_slice(&transaction_id.to_be_bytes());
    packet.extend_from_slice(&[0u8; 10]);
    packet.push(1);
    packet.extend_from_slice(&encoded_netbios_name("*"));
    packet.extend_from_slice(&0x0021u16.to_be_bytes());
    packet.extend_from_slice(&0x0001u16.to_be_bytes());
    packet
}

fn encoded_netbios_name(name: &str) -> Vec<u8> {
    let mut raw = [b' '; 16];
    for (index, byte) in name.bytes().take(16).enumerate() {
        raw[index] = byte;
    }
    let mut out = Vec::with_capacity(34);
    out.push(32);
    for byte in raw {
        out.push(b'A' + (byte >> 4));
        out.push(b'A' + (byte & 0x0f));
    }
    out.push(0);
    out
}

async fn probe_netbios(host: IpAddr, timeout: Duration) -> Option<NbInfo> {
    let host_v4 = match host {
        IpAddr::V4(v4) => v4,
        IpAddr::V6(_) => return None,
    };
    let transaction_id = (std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.subsec_nanos() as u64)
        .unwrap_or(0)
        & 0xffff) as u16;

    let local: std::net::SocketAddr = std::net::SocketAddr::new(IpAddr::V4(Ipv4Addr::UNSPECIFIED), 0);
    let socket = UdpSocket::bind(local).await.ok()?;
    let request = build_nbns_node_status_request(transaction_id);
    socket
        .send_to(&request, std::net::SocketAddr::new(IpAddr::V4(host_v4), 137))
        .await
        .ok()?;

    let mut buffer = vec![0u8; 1500];
    let n = tokio::time::timeout(timeout, socket.recv(&mut buffer)).await.ok()?.ok()?;
    parse_nbns_node_status_response(&buffer[..n], transaction_id)
}

fn parse_nbns_node_status_response(packet: &[u8], transaction_id: u16) -> Option<NbInfo> {
    if packet.len() < 12 || u16::from_be_bytes([packet[0], packet[1]]) != transaction_id {
        return None;
    }
    let answer_count = u16::from_be_bytes([packet[6], packet[7]]) as usize;
    if answer_count == 0 {
        return None;
    }

    let mut offset = 12;
    let next = skip_dns_name(packet, offset)?;
    if packet.len() < next + 4 {
        return None;
    }
    offset = next + 4;

    for _ in 0..answer_count {
        let next = skip_dns_name(packet, offset)?;
        if packet.len() < next + 10 {
            return None;
        }
        let record_type = u16::from_be_bytes([packet[next], packet[next + 1]]);
        let data_len = u16::from_be_bytes([packet[next + 8], packet[next + 9]]) as usize;
        let data_start = next + 10;
        let data_end = data_start + data_len;
        if data_end > packet.len() {
            return None;
        }
        if record_type == 0x0021 {
            return parse_nbns_node_status_data(&packet[data_start..data_end]);
        }
        offset = data_end;
    }
    None
}

fn skip_dns_name(packet: &[u8], mut offset: usize) -> Option<usize> {
    loop {
        if offset >= packet.len() {
            return None;
        }
        let length = packet[offset] as usize;
        offset += 1;
        if length == 0 {
            return Some(offset);
        }
        if length & 0xc0 == 0xc0 {
            if offset >= packet.len() {
                return None;
            }
            return Some(offset + 1);
        }
        offset += length;
        if offset > packet.len() {
            return None;
        }
    }
}

fn parse_nbns_node_status_data(data: &[u8]) -> Option<NbInfo> {
    if data.is_empty() {
        return None;
    }
    let name_count = data[0] as usize;
    let entries_start = 1;
    let entries_end = entries_start + name_count * 18;
    if name_count == 0 || entries_end > data.len() {
        return None;
    }

    let mut info = NbInfo::default();
    let mut names: Vec<String> = Vec::new();
    let mut offset = entries_start;
    while offset < entries_end {
        let raw_name = String::from_utf8_lossy(&data[offset..offset + 15]).trim().to_string();
        let suffix = data[offset + 15];
        let flags = u16::from_be_bytes([data[offset + 16], data[offset + 17]]);
        if !raw_name.is_empty() {
            names.push(raw_name.clone());
            let group = flags & 0x8000 != 0;
            if info.hostname.is_empty() && !group && (suffix == 0x00 || suffix == 0x20) {
                info.hostname = raw_name.clone();
            }
            if info.domain.is_empty() && group && (suffix == 0x00 || suffix == 0x1e) {
                info.domain = raw_name.clone();
            }
        }
        offset += 18;
    }

    let stats = &data[entries_end..];
    if stats.len() >= 6 {
        info.mac = format!(
            "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
            stats[0], stats[1], stats[2], stats[3], stats[4], stats[5]
        );
    }
    if !names.is_empty() || !info.hostname.is_empty() || !info.domain.is_empty() || !info.mac.is_empty() {
        Some(info)
    } else {
        None
    }
}
