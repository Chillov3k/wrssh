use std::sync::atomic::{AtomicBool, Ordering};

// Build-time injected values (FURY_DEST / FURY_FINGERPRINT env at compile time,
// the Rust equivalent of the Go client's -X ldflags).
static DESTINATION: Option<&str> = option_env!("FURY_DEST");
static FINGERPRINT: Option<&str> = option_env!("FURY_FINGERPRINT");

pub struct Config {
    pub addr: String,
    pub fingerprint: String,
    pub version_string: String,
    pub sni: Option<String>,
}

pub static FOREGROUND: AtomicBool = AtomicBool::new(false);

pub fn load() -> std::sync::Arc<Config> {
    // Parse minimal command line, mirroring the Go client's flags
    let mut addr = DESTINATION.unwrap_or("").to_string();
    let mut fingerprint = FINGERPRINT.unwrap_or("").to_string();
    let mut version_string = String::new();
    let mut sni: Option<String> = None;

    let args: Vec<String> = std::env::args().collect();
    let mut i = 1;
    while i < args.len() {
        let a = &args[i];
        match a.as_str() {
            "-d" | "--destination" if i + 1 < args.len() => { addr = args[i + 1].clone(); i += 1; }
            "--fingerprint" if i + 1 < args.len() => { fingerprint = args[i + 1].clone(); i += 1; }
            "--version-string" if i + 1 < args.len() => { version_string = args[i + 1].clone(); i += 1; }
            "--sni" if i + 1 < args.len() => { sni = Some(args[i + 1].clone()); i += 1; }
            "--foreground" => { FOREGROUND.store(true, Ordering::SeqCst); }
            _ => {}
        }
        i += 1;
    }

    let wants_help = args.iter().any(|a| a == "-h" || a == "--help");

    if addr.is_empty() {
        // take last argument as address guess, like the Go client
        if args.len() > 1 {
            let last = args.last().unwrap().clone();
            if !last.starts_with('-') {
                addr = last;
            }
        }
    }

    if wants_help {
        println!("usage: [-d host:port] [--fingerprint hex] [--sni name] [--version-string str] [--foreground]");
        std::process::exit(0);
    }

    if addr.is_empty() {
        // Nothing to connect to: exit instead of spinning in a reconnect loop.
        eprintln!("no destination specified");
        std::process::exit(1);
    }

    std::sync::Arc::new(Config { addr, fingerprint, version_string, sni })
}
