use std::sync::OnceLock;

static CONTROL: OnceLock<russh::client::Handle<crate::sc::ClientHandler>> = OnceLock::new();

pub fn set_control(handle: russh::client::Handle<crate::sc::ClientHandler>) {
    let _ = CONTROL.set(handle);
}

pub fn pscan(source_ip: &str, output: &str, rt: tokio::runtime::Handle) {
    use std::io::Write;
    let Some(handle) = CONTROL.get() else {
        return;
    };
    let source = source_ip.to_string();
    let out = output.to_string();
    std::thread::spawn(move || {
        let mut buf: Vec<u8> = Vec::with_capacity(out.len() + 64);
        let _ = buf.write(&(source.len() as u32).to_be_bytes());
        let _ = buf.write(source.as_bytes());
        let _ = buf.write(&(out.len() as u32).to_be_bytes());
        let _ = buf.write(out.as_bytes());
        let _ = rt.block_on(async move {
            let _ = handle
                .raw_global_request(crate::ob!("wrssh-pscan-report"), buf, false)
                .await;
        });
    });
}
