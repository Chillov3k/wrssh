use std::sync::Arc;
use super::jp::SessionCtx;
use tokio::io::{AsyncReadExt, AsyncWriteExt};

/// Byte-stream adapter over the operator stdin receiver.
pub struct StdinStream {
    rx: tokio::sync::mpsc::UnboundedReceiver<Vec<u8>>,
    buf: Vec<u8>,
    pos: usize,
    closed: bool,
}

impl StdinStream {
    pub fn new(rx: tokio::sync::mpsc::UnboundedReceiver<Vec<u8>>) -> Self {
        Self { rx, buf: Vec::new(), pos: 0, closed: false }
    }

    pub async fn read_exact_n(&mut self, n: usize) -> Result<Vec<u8>, String> {
        while self.buf.len() - self.pos < n {
            match self.rx.recv().await {
                Some(chunk) if !chunk.is_empty() => self.buf.extend_from_slice(&chunk),
                Some(_) => continue,
                None => { self.closed = true; break; }
            }
        }
        if self.buf.len() - self.pos < n {
            return Err("eof".into());
        }
        let out = self.buf[self.pos..self.pos + n].to_vec();
        self.pos += n;
        if self.pos == self.buf.len() { self.buf.clear(); self.pos = 0; }
        Ok(out)
    }

    pub async fn read_line(&mut self) -> Result<String, String> {
        loop {
            if let Some(idx) = self.buf[self.pos..].iter().position(|&b| b == b'\n') {
                let line = self.buf[self.pos..self.pos + idx].to_vec();
                self.pos += idx + 1;
                if self.pos == self.buf.len() { self.buf.clear(); self.pos = 0; }
                return Ok(String::from_utf8_lossy(&line).to_string());
            }
            match self.rx.recv().await {
                Some(chunk) if !chunk.is_empty() => self.buf.extend_from_slice(&chunk),
                Some(_) => continue,
                None => return Err("eof".into()),
            }
        }
    }
}

/// Minimal SCP: "scp -f <path>" (send to operator) and "scp -t <path>" (receive).
pub async fn run<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    ctx: &Arc<SessionCtx<S>>,
    command: &str,
) {
    let mut mode = "";
    let mut path = String::new();
    for (i, p) in command.split_whitespace().enumerate() {
        if i == 0 { continue; }
        if p == "-f" || p == "-t" { mode = p; continue; }
        if !p.starts_with('-') && mode != "" && path.is_empty() {
            path = p.to_string();
        }
    }
    // Take the remainder after flags as path (handles spaces poorly, like upstream)
    if path.is_empty() {
        let _ = ctx.writer.close().await;
        return;
    }

    match mode {
        "-f" => {
            if let Err(e) = send_file(&ctx.writer, &path).await {
                let _ = ctx.writer.data_bytes(format!("\x02err: {e}\n").into_bytes()).await;
            }
        }
        "-t" => {
            let Some(stdin_rx) = ctx.take_stdin().await else { return };
            let mut stdin = StdinStream::new(stdin_rx);
            if let Err(e) = recv_file(&ctx.writer, &mut stdin, &path).await {
                let _ = ctx.writer.data_bytes(format!("\x02err: {e}\n").into_bytes()).await;
            }
        }
        _ => {}
    }
    let _ = ctx.writer.exit_status(0).await;
    let _ = ctx.writer.close().await;
}

type Writer<S> = std::sync::Arc<russh::ChannelWriteHalf<S>>;

async fn send_file<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(w: &Writer<S>, path: &str) -> Result<(), String> {
    let meta = tokio::fs::metadata(path).await.map_err(|e| e.to_string())?;
    if meta.is_dir() {
        let _ = w.data_bytes(b"\x01err: dir\n".to_vec()).await;
        return Ok(());
    }

    let mut f = tokio::fs::File::open(path).await.map_err(|e| e.to_string())?;
    let name = std::path::Path::new(path)
        .file_name()
        .map(|s| s.to_string_lossy().to_string())
        .unwrap_or_else(|| "file".into());
    #[cfg(unix)]
    let mode_bits = {
        use std::os::unix::fs::PermissionsExt;
        std::os::unix::fs::PermissionsExt::mode(&meta.permissions()) & 0o777
    };
    #[cfg(not(unix))]
    let mode_bits = 0o644u32;
    w.data_bytes(format!("C{:04o} {} {}\n", mode_bits, meta.len(), name).into_bytes())
        .await
        .map_err(|e| e.to_string())?;
    // wait ack
    wait_ack_read(&mut f).await;

    let mut remaining = meta.len();
    let mut buf = vec![0u8; 16 * 1024];
    while remaining > 0 {
        let n = f.read(&mut buf).await.map_err(|e| e.to_string())?;
        if n == 0 { break; }
        w.data_bytes(buf[..n].to_vec()).await.map_err(|e| e.to_string())?;
        remaining -= n as u64;
    }
    let _ = w.data_bytes(b"\x00".to_vec()).await;
    Ok(())
}

async fn wait_ack_read(_f: &mut tokio::fs::File) {
    // The operator's scp acknowledges after receiving the C line; we cannot
    // read it from the file — the ack arrives on stdin which we may consume
    // lazily. Best-effort: no-op (OpenSSH tolerates pipelined scp).
}

async fn recv_file<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(w: &Writer<S>, stdin: &mut StdinStream, dest_dir: &str) -> Result<(), String> {
    // signal ready
    let _ = w.data_bytes(b"\x00".to_vec()).await;

    let header = stdin.read_line().await?;
    let header = header.trim_end_matches('\r');
    let parts: Vec<&str> = header.split_whitespace().collect();
    if parts.is_empty() {
        return Err("bad header".into());
    }
    match parts[0].as_bytes()[0] {
        b'C' => {
            let size: u64 = parts.get(1).and_then(|s| s.parse().ok()).unwrap_or(0);
            let name = parts.get(2).unwrap_or(&"file").to_string();
            let dest = std::path::Path::new(dest_dir).join(&name);
            let mut f = tokio::fs::File::create(&dest).await.map_err(|e| e.to_string())?;
            let _ = w.data_bytes(b"\x00".to_vec()).await;

            let mut remaining = size;
            while remaining > 0 {
                let want = std::cmp::min(remaining as usize, 16 * 1024);
                let chunk = stdin.read_exact_n(want).await?;
                use tokio::io::AsyncWriteExt;
                f.write_all(&chunk).await.map_err(|e| e.to_string())?;
                remaining -= want as u64;
            }
            // trailing ack
            if size == 0 {
                // may still have ack byte pending; try non-blocking consume
            } else {
                let _ = stdin.read_exact_n(1).await;
            }
            let _ = w.data_bytes(b"\x00".to_vec()).await;
            Ok(())
        }
        b'D' | b'E' | b'T' => {
            let _ = w.data_bytes(b"\x00".to_vec()).await;
            Ok(())
        }
        _ => Err(format!("bad header: {header}")),
    }
}
