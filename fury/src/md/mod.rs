// Client subsystem modules: registry, manifests, and the shared module IO.
// Mirrors internal/client/handlers/subsystems of the Go client.

#[cfg(feature = "pscan")]
pub mod pscan;
#[cfg(feature = "execass")]
pub mod execass;
#[cfg(any(target_os = "windows", target_os = "linux"))]
pub mod service;

use russh::ChannelMsg;
use std::sync::Arc;
use tokio::sync::mpsc::UnboundedReceiver;

pub struct Manifest {
    pub name: String,
    pub description: String,
    pub version: String,
    pub usage: String,
    pub build_tags: Vec<String>,
    pub platforms: Vec<String>,
    pub dangerous: bool,
    pub disabled: bool,
    pub timeout_seconds: i64,
    pub output_bytes: i64,
    pub stdin_bytes: i64,
    pub max_args: i64,
}

impl Manifest {
    pub fn name_only(name: &str) -> Manifest {
        Manifest {
            name: name.to_string(),
            description: String::new(),
            version: String::new(),
            usage: String::new(),
            build_tags: Vec::new(),
            platforms: Vec::new(),
            dangerous: false,
            disabled: false,
            timeout_seconds: 0,
            output_bytes: 0,
            stdin_bytes: 0,
            max_args: 0,
        }
    }

    pub fn to_json(&self) -> String {
        let mut out = String::from("{");
        out.push_str(&json_kv("name", &self.name));
        if !self.description.is_empty() {
            out.push_str(&json_kv("description", &self.description));
        }
        if !self.version.is_empty() {
            out.push_str(&json_kv("version", &self.version));
        }
        if !self.usage.is_empty() {
            out.push_str(&json_kv("usage", &self.usage));
        }
        if !self.build_tags.is_empty() {
            out.push_str("\"buildTags\":");
            out.push_str(&json_string_array(&self.build_tags));
            out.push(',');
        }
        if !self.platforms.is_empty() {
            out.push_str("\"platforms\":");
            out.push_str(&json_string_array(&self.platforms));
            out.push(',');
        }
        if self.dangerous {
            out.push_str("\"dangerous\":true,");
        }
        if self.disabled {
            out.push_str("\"disabled\":true,");
        }
        if self.timeout_seconds != 0 || self.output_bytes != 0 || self.stdin_bytes != 0 || self.max_args != 0 {
            out.push_str("\"limits\":{");
            let mut parts = Vec::new();
            if self.timeout_seconds != 0 {
                parts.push(format!("\"timeoutSeconds\":{}", self.timeout_seconds));
            }
            if self.output_bytes != 0 {
                parts.push(format!("\"outputBytes\":{}", self.output_bytes));
            }
            if self.stdin_bytes != 0 {
                parts.push(format!("\"stdinBytes\":{}", self.stdin_bytes));
            }
            if self.max_args != 0 {
                parts.push(format!("\"maxArgs\":{}", self.max_args));
            }
            out.push_str(&parts.join(","));
            out.push_str("},");
        }
        if out.ends_with(',') {
            out.pop();
        }
        out.push('}');
        out
    }
}

fn json_escape(value: &str) -> String {
    let mut out = String::with_capacity(value.len() + 2);
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

fn json_kv(key: &str, value: &str) -> String {
    format!("\"{}\":\"{}\",", key, json_escape(value))
}

fn json_string_array(values: &[String]) -> String {
    let items: Vec<String> = values.iter().map(|v| format!("\"{}\"", json_escape(v))).collect();
    format!("[{}]", items.join(","))
}

pub fn manifests() -> Vec<Manifest> {
    let mut list = Vec::new();
    list.push(sftp_manifest());
    list.push(list_manifest());
    #[cfg(feature = "pscan")]
    list.push(pscan::manifest());
    #[cfg(feature = "execass")]
    list.push(execass::manifest());
    #[cfg(any(target_os = "windows", target_os = "linux"))]
    list.push(service::manifest());
    list
}

fn sftp_manifest() -> Manifest {
    let mut m = Manifest::name_only(&crate::ob!("sftp"));
    m.description = crate::ob!("SFTP file transfer over the control connection.");
    m.usage = crate::ob!("sftp");
    m.timeout_seconds = 5;
    m.output_bytes = 128 * 1024;
    m.max_args = 1;
    m
}

fn list_manifest() -> Manifest {
    let mut m = Manifest::name_only(&crate::ob!("list"));
    m.description = crate::ob!("List available client subsystem modules.");
    m.usage = crate::ob!("list [--json]");
    m.timeout_seconds = 5;
    m.output_bytes = 128 * 1024;
    m.max_args = 1;
    m
}

pub fn names() -> Vec<String> {
    manifests().into_iter().map(|m| m.name).collect()
}

pub fn manifests_json() -> String {
    let items: Vec<String> = manifests().iter().map(|m| m.to_json()).collect();
    format!("[{}]", items.join(","))
}

/// Module IO: stdout/stderr flow back over the SSH channel, stdin is bridged
/// from the operator (base64 artifact uploads for execass).
pub struct ModuleIo<S: From<(russh::ChannelId, ChannelMsg)>> {
    pub writer: Arc<russh::ChannelWriteHalf<S>>,
    pub stdin: Option<UnboundedReceiver<Vec<u8>>>,
}

impl<S: From<(russh::ChannelId, ChannelMsg)> + Send + Sync + 'static> ModuleIo<S> {
    pub async fn stdout(&self, data: Vec<u8>) {
        let _ = self.writer.data_bytes(data).await;
    }

    pub async fn stdout_str(&self, text: &str) {
        self.stdout(text.as_bytes().to_vec()).await;
    }

    pub async fn stderr(&self, data: Vec<u8>) {
        // 1 = SSH_EXTENDED_DATA_STDERR
        let _ = self.writer.extended_data_bytes(1, data).await;
    }

    pub async fn stderr_str(&self, text: &str) {
        self.stderr(text.as_bytes().to_vec()).await;
    }

    /// Drains subsystem stdin into a byte buffer with a size cap, mirroring
    /// Go's io.ReadAll(io.LimitReader(...)) artifact load.
    pub async fn read_stdin(&mut self, cap: usize) -> Result<Vec<u8>, String> {
        let mut rx = match self.stdin.take() {
            Some(rx) => rx,
            None => return Ok(Vec::new()),
        };
        let mut out = Vec::new();
        while let Some(chunk) = rx.recv().await {
            // channel_eof delivers an empty chunk as the EOF marker.
            if chunk.is_empty() {
                break;
            }
            out.extend_from_slice(&chunk);
            if out.len() > cap {
                return Err(crate::ob!("input exceeds maximum size").to_string());
            }
        }
        Ok(out)
    }
}

/// Quote-aware argument tokenizer for subsystem request lines, close to the
/// Go client's terminal parsing: supports 'single', "double" quotes and
/// backslash escapes inside double quotes.
pub fn tokenize(line: &str) -> Vec<String> {
    let mut args = Vec::new();
    let mut cur = String::new();
    let mut quote: Option<char> = None;
    let mut escaping = false;
    let mut chars = line.chars().peekable();

    while let Some(ch) = chars.next() {
        if escaping {
            cur.push(ch);
            escaping = false;
            continue;
        }
        match quote {
            Some(q) => {
                if ch == q {
                    quote = None;
                } else if q == '"' && ch == '\\' {
                    if let Some(&next) = chars.peek() {
                        if matches!(next, '"' | '\\') {
                            escaping = true;
                            continue;
                        }
                    }
                    cur.push(ch);
                } else {
                    cur.push(ch);
                }
            }
            None => match ch {
                '\'' | '"' => quote = Some(ch),
                c if c.is_whitespace() => {
                    if !cur.is_empty() {
                        args.push(std::mem::take(&mut cur));
                    }
                }
                c => cur.push(c),
            },
        }
    }
    if !cur.is_empty() {
        args.push(cur);
    }
    args
}

/// Dispatch a module run; returns the process exit code.
pub async fn run<S: From<(russh::ChannelId, ChannelMsg)> + Send + Sync + 'static>(
    module: &str,
    args: &[String],
    mut io: ModuleIo<S>,
) -> i32 {
    let name = module.to_lowercase();
    let result: Result<(), String> = match name.as_str() {
        m if m == crate::ob!("pscan") && cfg!(feature = "pscan") => {
            #[cfg(feature = "pscan")]
            {
                pscan::run(args, &mut io).await
            }
            #[cfg(not(feature = "pscan"))]
            {
                let _ = m;
                Err(crate::ob!("module not compiled in").to_string())
            }
        }
        m if m == crate::ob!("execass") && cfg!(feature = "execass") => {
            #[cfg(feature = "execass")]
            {
                execass::run(args, &mut io).await
            }
            #[cfg(not(feature = "execass"))]
            {
                let _ = m;
                Err(crate::ob!("module not compiled in").to_string())
            }
        }
        m if m == crate::ob!("service") => {
            #[cfg(any(target_os = "windows", target_os = "linux"))]
            {
                service::run(args, &mut io).await
            }
            #[cfg(not(any(target_os = "windows", target_os = "linux")))]
            {
                Err(crate::ob!("module not compiled in").to_string())
            }
        }
        other => Err(format!("{}{}", crate::ob!("unknown subsystem: "), other)),
    };

    match result {
        Ok(()) => 0,
        Err(message) => {
            io.stderr_str(&format!("{}{}", "subsystem error: \"", message))
                .await;
            io.stderr_str("\"").await;
            1
        }
    }
}
