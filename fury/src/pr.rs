use std::sync::Arc;
use tokio::io::{AsyncReadExt, AsyncWriteExt};

/// Pump stdio between the operator channel and a spawned child process.
pub async fn pump<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    writer: std::sync::Arc<russh::ChannelWriteHalf<S>>,
    child: &mut tokio::process::Child,
    stdin_rx: tokio::sync::mpsc::UnboundedReceiver<Vec<u8>>,
) {
    let mut stdin_rx = stdin_rx;
    let ch = writer.clone();

    let mut stdout = child.stdout.take();
    let mut stderr = child.stderr.take();
    let mut stdin = child.stdin.take();

    let ch_err = ch.clone();
    let t_err = if let Some(stderr) = stderr.take() {
        Some(tokio::spawn(async move {
            let mut stderr = stderr;
            let mut buf = vec![0u8; 16 * 1024];
            loop {
                match stderr.read(&mut buf).await {
                    Ok(0) | Err(_) => break,
                    Ok(n) => {
                        if ch_err.extended_data_bytes(1, buf[..n].to_vec()).await.is_err() {
                            break;
                        }
                    }
                }
            }
        }))
    } else {
        None
    };

    let ch_out = ch.clone();
    let t_out = if let Some(stdout) = stdout.take() {
        Some(tokio::spawn(async move {
            let mut stdout = stdout;
            let mut buf = vec![0u8; 16 * 1024];
            loop {
                match stdout.read(&mut buf).await {
                    Ok(0) | Err(_) => break,
                    Ok(n) => {
                        if ch_out.data_bytes(buf[..n].to_vec()).await.is_err() {
                            break;
                        }
                    }
                }
            }
        }))
    } else {
        None
    };

    let t_in = if let Some(mut stdin) = stdin.take() {
        Some(tokio::spawn(async move {
            let mut stdin_rx = stdin_rx;
            while let Some(data) = stdin_rx.recv().await {
                if stdin.write_all(&data).await.is_err() {
                    break;
                }
            }
        }))
    } else {
        None
    };

    if let Some(t) = t_out {
        let _ = t.await;
    }
    if let Some(t) = t_err {
        let _ = t.await;
    }
    if let Some(t) = t_in {
        t.abort();
    }

    let code = child.wait().await.ok().and_then(|s| s.code()).unwrap_or(0);
    let _ = ch.eof().await;
    let _ = ch.exit_status(code as u32).await;
    let _ = ch.close().await;
}
