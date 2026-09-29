#[cfg(windows)]
mod wc;
#[cfg(windows)]
pub use wc::{open, serve_blocking, Pty, PtyHandle};

#[cfg(unix)]
mod ux;
#[cfg(unix)]
pub use ux::{open, serve_blocking, Pty, PtyHandle};

#[cfg(not(any(windows, unix)))]
compile_error!("unsupported platform");
