use russh::Channel;
use std::io::{Read, Write};
use std::process::{Command, Stdio};
use std::sync::{Arc, Mutex};

pub struct Pty {
    child: std::process::Child,
    stdin: std::process::ChildStdin,
    stdout: std::process::ChildStdout,
}

pub struct PtyHandle;

pub fn open(_cols: u16, _rows: u16) -> Result<Pty, String> {
    let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/sh".to_string());
    let mut child = Command::new(&shell)
        .arg("-i")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .env("TERM", "xterm-256color")
        .spawn()
        .map_err(|e| format!("spawn {shell}: {e}"))?;

    let stdin = child.stdin.take().ok_or("no stdin")?;
    let stdout = child.stdout.take().ok_or("no stdout")?;
    // merge stderr into the same stream is not possible with std; spawn a pump
    let stderr = child.stderr.take();
    if let Some(stderr) = stderr {
        std::thread::spawn(move || {
            let mut buf = [0u8; 8192];
            let mut err = stderr;
            let _ = err.read(&mut buf);
        });
    }
    Ok(Pty { child, stdin, stdout })
}

impl Pty {
    pub fn spawn_shell(&mut self) -> Option<std::process::Child> {
        None
    }

    pub fn resize(&self, _cols: u16, _rows: u16) {}

    pub fn handle(&self) -> PtyHandle {
        PtyHandle
    }

    pub fn wait(&mut self) -> u32 {
        self.child.wait().ok().and_then(|s| s.code()).map(|c| c as u32).unwrap_or(0)
    }
}

impl PtyHandle {
    pub fn resize(&self, _cols: u16, _rows: u16) {}
}

#[allow(dead_code)]
pub fn serve_blocking<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    ch: std::sync::Arc<russh::ChannelWriteHalf<S>>,
    mut pty: Pty,
    mut stdin_rx: tokio::sync::mpsc::UnboundedReceiver<Vec<u8>>,
    rt: tokio::runtime::Handle,
) -> u32 {
    let mut stdout = pty.stdout;
    let mut stdin = pty.stdin;

    let ch2 = ch.clone();
    let t_read = std::thread::spawn(move || {
        let mut buf = [0u8; 16384];
        loop {
            match stdout.read(&mut buf) {
                Ok(0) | Err(_) => break,
                Ok(n) => {
                    let ch3 = ch2.clone();
                    let data = buf[..n].to_vec();
                    let _ = rt.block_on(async move {
                        let _ = ch3.data_bytes(data).await;
                    });
                }
            }
        }
        let ch3 = ch2.clone();
        let _ = rt.block_on(async move {
            let _ = ch3.eof().await;
        });
    });

    let t_write = std::thread::spawn(move || {
        while let Some(data) = stdin_rx.blocking_recv() {
            if stdin.write_all(&data).is_err() {
                break;
            }
        }
    });

    let _ = t_read.join();
    let _ = t_write.join();
    let _ = pty.child;
    0
}
