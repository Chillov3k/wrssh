use russh::Channel;
use std::fs::File;
use std::io::{Read, Write};
use std::os::fd::FromRawFd;
use std::os::unix::process::CommandExt;
use std::process::{Command, Stdio};

// Real pseudoterminal, mirroring the Go agent (creack/pty): the shell gets a
// controlling tty, so prompts, job control and TERM handling all behave.
pub struct Pty {
    child: std::process::Child,
    master: Option<File>,
}

pub struct PtyHandle {
    master: Option<File>,
}

pub fn open(cols: u16, rows: u16) -> Result<Pty, String> {
    let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/sh".to_string());

    let master = unsafe { libc::posix_openpt(libc::O_RDWR | libc::O_NOCTTY) };
    if master < 0 {
        return Err("posix_openpt failed".into());
    }
    if unsafe { libc::grantpt(master) } != 0 || unsafe { libc::unlockpt(master) } != 0 {
        unsafe { libc::close(master) };
        return Err("grantpt/unlockpt failed".into());
    }

    let name_ptr = unsafe { libc::ptsname(master) };
    if name_ptr.is_null() {
        unsafe { libc::close(master) };
        return Err("ptsname failed".into());
    }
    let slave_path = unsafe { std::ffi::CStr::from_ptr(name_ptr) }
        .to_string_lossy()
        .into_owned();

    let slave = unsafe { libc::open(slave_path.as_ptr().cast(), libc::O_RDWR | libc::O_NOCTTY) };
    if slave < 0 {
        unsafe { libc::close(master) };
        return Err("open pty slave failed".into());
    }

    set_winsize(master, cols, rows);

    let mut child = unsafe {
        Command::new(&shell)
            .env("TERM", "xterm-256color")
            .stdin(Stdio::from_raw_fd(libc::dup(slave)))
            .stdout(Stdio::from_raw_fd(libc::dup(slave)))
            .stderr(Stdio::from_raw_fd(libc::dup(slave)))
            .pre_exec(move || {
                libc::setsid();
                // fd 0 is the slave pty: make it the controlling terminal.
                libc::ioctl(0, libc::TIOCSCTTY as u64, 0u64);
                Ok(())
            })
            .spawn()
    }
    .map_err(|e| {
        unsafe {
            libc::close(slave);
            libc::close(master);
        }
        format!("spawn {shell}: {e}")
    })?;

    let _ = child.stdin.take();
    let _ = child.stdout.take();
    let _ = child.stderr.take();
    unsafe { libc::close(slave) };

    Ok(Pty {
        child,
        master: Some(unsafe { File::from_raw_fd(master) }),
    })
}

fn set_winsize(fd: i32, cols: u16, rows: u16) {
    let ws = libc::winsize {
        ws_row: rows,
        ws_col: cols,
        ws_xpixel: 0,
        ws_ypixel: 0,
    };
    unsafe { libc::ioctl(fd, libc::TIOCSWINSZ as u64, &ws) };
}

impl Pty {
    pub fn handle(&self) -> PtyHandle {
        PtyHandle {
            master: self.master.as_ref().and_then(|m| m.try_clone().ok()),
        }
    }
}

impl PtyHandle {
    pub fn resize(&self, cols: u16, rows: u16) {
        use std::os::fd::AsRawFd;
        if let Some(master) = &self.master {
            set_winsize(master.as_raw_fd(), cols, rows);
        }
    }
}

#[allow(dead_code)]
pub fn serve_blocking<S: From<(russh::ChannelId, russh::ChannelMsg)> + Send + Sync + 'static>(
    ch: std::sync::Arc<russh::ChannelWriteHalf<S>>,
    mut pty: Pty,
    mut stdin_rx: tokio::sync::mpsc::UnboundedReceiver<Vec<u8>>,
    rt: tokio::runtime::Handle,
) -> u32 {
    let mut master = match pty.master.take() {
        Some(m) => m,
        None => return 0,
    };
    let mut master_read = match master.try_clone() {
        Ok(m) => m,
        Err(_) => return 0,
    };

    let ch2 = ch.clone();
    let t_read = std::thread::spawn(move || {
        let mut buf = [0u8; 16384];
        loop {
            match master_read.read(&mut buf) {
                Ok(0) | Err(_) => break,
                Ok(n) => {
                    let ch3 = ch2.clone();
                    let data = buf[..n].to_vec();
                    let _ = rt.block_on(async move {
                        let _ = ch3.data_bytes(data).await;
                    });
                }
            }
        }
        let ch3 = ch2.clone();
        let _ = rt.block_on(async move {
            let _ = ch3.eof().await;
        });
    });

    let t_write = std::thread::spawn(move || {
        while let Some(data) = stdin_rx.blocking_recv() {
            if master.write_all(&data).is_err() {
                break;
            }
        }
    });

    let _ = t_read.join();
    let _ = t_write.join();
    pty.child
        .wait()
        .ok()
        .and_then(|s| s.code())
        .map(|c| c as u32)
        .unwrap_or(0)
}
