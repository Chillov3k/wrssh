use russh::Channel;
use std::io::{Read, Write};
use std::process::Command;
use std::sync::{Arc, Mutex};

pub struct Pty {
    proc: Arc<Mutex<conpty::Process>>,
}

pub struct PtyHandle {
    proc: Arc<Mutex<conpty::Process>>,
}

pub fn open(cols: u16, rows: u16) -> Result<Pty, String> {
    let shell = which_shell();
    let mut opts: conpty::ProcessOptions = Default::default();
    opts.set_console_size(Some((cols as i16, rows as i16)));
    let proc = opts
        .spawn(Command::new(&shell))
        .map_err(|e| format!("term spawn: {e}"))?;
    Ok(Pty {
        proc: Arc::new(Mutex::new(proc)),
    })
}

fn which_shell() -> String {
    for candidate in [
        "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
        "C:\\Windows\\system32\\cmd.exe",
    ] {
        if std::path::Path::new(candidate).exists() {
            return candidate.to_string();
        }
    }
    "powershell.exe".to_string()
}

impl Pty {
    pub fn handle(&self) -> PtyHandle {
        PtyHandle {
            proc: self.proc.clone(),
        }
    }
}

impl PtyHandle {
    pub fn resize(&self, cols: u16, rows: u16) {
        if let Ok(mut p) = self.proc.lock() {
            let _ = p.resize(cols as i16, rows as i16);
        }
    }
}

/// Pump data between the operator channel and the ConPTY process.
/// Runs on a blocking thread; uses the captured tokio handle for channel writes.
/// Returns the process exit code.
pub fn serve_blocking<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    ch: std::sync::Arc<russh::ChannelWriteHalf<S>>,
    pty: Pty,
    mut stdin_rx: tokio::sync::mpsc::UnboundedReceiver<Vec<u8>>,
    rt: tokio::runtime::Handle,
) -> u32 {
    let (reader, writer) = {
        let mut p = pty.proc.lock().unwrap();
        let r = p.output().ok();
        let w = p.input().ok();
        (r, w)
    };
    let (Some(reader), Some(writer)) = (reader, writer) else {
        return 1;
    };
    let pty = pty;
    let reader = Arc::new(Mutex::new(reader));
    let writer = Arc::new(Mutex::new(writer));

    let ch2 = ch.clone();
    let rt2 = rt.clone();
    let reader2 = reader.clone();
    let t_read = std::thread::spawn(move || {
        let mut buf = [0u8; 16384];
        loop {
            let n = {
                let mut r = match reader2.lock() {
                    Ok(r) => r,
                    Err(_) => break,
                };
                match r.read(&mut buf) {
                    Ok(0) | Err(_) => break,
                    Ok(n) => n,
                }
            };
            let ch3 = ch2.clone();
            let data = buf[..n].to_vec();
            let _ = rt2.block_on(async move {
                let _ = ch3.data_bytes(data).await;
            });
        }
        let ch3 = ch2.clone();
        let _ = rt2.block_on(async move {
            let _ = ch3.eof().await;
        });
    });

    let t_write = std::thread::spawn(move || {
        while let Some(data) = stdin_rx.blocking_recv() {
            let mut w = match writer.lock() {
                Ok(w) => w,
                Err(_) => break,
            };
            if w.write_all(&data).is_err() {
                break;
            }
        }
    });

    let _ = t_read.join();
    let _ = t_write.join();

    // reap the shell process
    pty.proc
        .lock()
        .ok()
        .and_then(|p| p.wait(None).ok())
        .unwrap_or(0)
}
