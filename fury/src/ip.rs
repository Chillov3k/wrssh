// Internal (LAN) IP discovery, mirroring the Go client's local_ip.go:
// prefer the default-route source address (UDP "dial" without traffic),
// fall back to the first usable interface address.

use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr, UdpSocket};

pub fn internal_ip() -> Option<String> {
    for target in ["8.8.8.8:80", "1.1.1.1:80", "[2001:4860:4860::8888]:80"] {
        let address: SocketAddr = target.parse().ok()?;
        if let Ok(conn) = UdpSocket::bind(if address.is_ipv4() { "0.0.0.0:0" } else { "[::]:0" }) {
            if conn.connect(address).is_err() {
                continue;
            }
            if let Ok(local) = conn.local_addr() {
                if usable_local_ip(local.ip()) {
                    return Some(local.ip().to_string());
                }
            }
        }
    }
    first_interface_ip()
}


#[cfg(unix)]
fn first_interface_ip() -> Option<String> {
    let interfaces = nics()?;
    for iface in interfaces {
        if !iface.is_up || iface.is_loopback {
            continue;
        }
        for ip in iface.ips {
            if usable_local_ip(ip) {
                return Some(ip.to_string());
            }
        }
    }
    None
}

#[cfg(not(unix))]
fn first_interface_ip() -> Option<String> {
    None
}

fn usable_local_ip(ip: IpAddr) -> bool {
    match ip {
        IpAddr::V4(v4) => {
            let link_local = (u32::from(v4) & 0xffff0000) == 0xa9fe0000;
            !(v4.is_loopback()
                || v4.is_unspecified()
                || v4.is_multicast()
                || link_local
                || v4.is_broadcast())
        }
        IpAddr::V6(v6) => {
            // fe80::/10 link-local check (stable API lacks is_unicast_link_local).
            let link_local = (v6.segments()[0] & 0xffc0) == 0xfe80;
            !(v6.is_loopback() || v6.is_unspecified() || v6.is_multicast() || link_local)
        }
    }
}

#[cfg(unix)]
struct Nic {
    name: String,
    is_up: bool,
    is_loopback: bool,
    ips: Vec<IpAddr>,
}

#[cfg(unix)]
fn nics() -> Option<Vec<Nic>> {
    // /proc/net/if_inet6 for IPv6, /proc/net/route-based approach is fragile;
    // enumerate through getifaddrs would need libc — reuse the UDP trick plus
    // /proc/net/dev names and parse addresses from /proc/net/fib_trie? Too
    // fragile. Instead shell out to nothing: read /proc/net/if_inet6 and
    // /proc/net/route for interface IPs on Linux; on macOS use the route
    // socket via `route -n get default` output? Keep it dependency-free:
    // the UDP path already covers the default route case; this fallback only
    // needs SOME usable address, so parse /proc/net/if_inet6 (Linux) and skip
    // elsewhere.
    let mut nics: Vec<Nic> = Vec::new();
    if let Ok(content) = std::fs::read_to_string("/proc/net/if_inet6") {
        for line in content.lines() {
            let mut parts = line.split_whitespace();
            let hex = parts.next()?;
            let name = parts.nth(3)?.to_string();
            if hex.len() != 32 {
                continue;
            }
            let mut groups = Vec::new();
            for chunk in hex.as_bytes().chunks(4) {
                let text = std::str::from_utf8(chunk).ok()?;
                groups.push(u16::from_str_radix(text, 16).ok()?);
            }
            let ip = IpAddr::V6(Ipv6Addr::new(
                groups[0], groups[1], groups[2], groups[3],
                groups[4], groups[5], groups[6], groups[7],
            ));
            if let Some(nic) = nics.iter_mut().find(|n| n.name == name) {
                nic.ips.push(ip);
            } else {
                nics.push(Nic {
                    name,
                    is_up: true,
                    is_loopback: false,
                    ips: vec![ip],
                });
            }
        }
    }
    // IPv4 fallback through a connected UDP socket to a private-range target
    // is already the primary path; without libc getifaddrs we stop here.
    Some(nics)
}

