use russh_sftp::protocol::{
    File, FileAttributes, Handle, Name, Status, StatusCode, Version,
};
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use tokio::io::{AsyncReadExt, AsyncSeekExt, AsyncWriteExt, SeekFrom};

pub struct FsSession {
    handles: HashMap<String, HandleKind>,
    counter: u64,
    cwd: PathBuf,
}

enum HandleKind {
    File(tokio::fs::File),
    Dir(Vec<File>),
}

impl FsSession {
    pub fn new() -> Self {
        Self {
            handles: HashMap::new(),
            counter: 0,
            cwd: current_dir(),
        }
    }

    fn next_handle(&mut self) -> String {
        self.counter += 1;
        format!("/h/{}", self.counter)
    }

    fn resolve(&self, path: &str) -> PathBuf {
        if path.is_empty() || path == "." {
            return self.cwd.clone();
        }
        // Absolute POSIX path ("/a/b")
        if path.starts_with('/') {
            return PathBuf::from(path);
        }
        // Windows drive ("C:/x") or UNC ("//?/C:/x", "\\srv\share")
        if path.contains(':') || path.starts_with("\\\\") || path.starts_with("//?/") {
            return PathBuf::from(path.replace('\\', "/"));
        }
        self.cwd.join(path)
    }
}

fn current_dir() -> PathBuf {
    #[cfg(windows)]
    {
        // home dir by default like pkg/sftp WithServerWorkingDirectory
        if let Some(home) = std::env::var_os("USERPROFILE") {
            return PathBuf::from(home);
        }
    }
    #[cfg(unix)]
    {
        if let Some(home) = std::env::var_os("HOME") {
            return PathBuf::from(home);
        }
    }
    std::env::current_dir().unwrap_or_else(|_| PathBuf::from("/"))
}

fn attrs_from_meta(meta: &std::io::Result<std::fs::Metadata>, name: &str) -> File {
    match meta {
        Ok(m) => {
            let perms = if m.is_dir() { 0o755 } else { 0o644 } as u32;
            let size = m.len() as u64;
            let mtime = m
                .modified()
                .ok()
                .and_then(|t| t.duration_since(std::time::UNIX_EPOCH).ok())
                .map(|d| d.as_secs() as u32)
                .unwrap_or(0);
            let atime = m
                .accessed()
                .ok()
                .and_then(|t| t.duration_since(std::time::UNIX_EPOCH).ok())
                .map(|d| d.as_secs() as u32)
                .unwrap_or(0);
            let mut attrs = FileAttributes::default();
            attrs.size = Some(size);
            attrs.permissions = Some(perms);
            attrs.mtime = Some(mtime);
            attrs.atime = Some(atime);
            #[cfg(windows)]
            {}
            File {
                filename: name.to_string(),
                longname: format_longname(name, &attrs),
                attrs,
            }
        }
        Err(_) => File {
            filename: name.to_string(),
            longname: name.to_string(),
            attrs: FileAttributes::default(),
        },
    }
}

fn format_longname(name: &str, attrs: &FileAttributes) -> String {
    let kind = if attrs.permissions.unwrap_or(0) & 0o40000 != 0 { 'd' } else { '-' };
    format!(
        "{kind}rw-rw-rw-   1 0      0    {:>10} {}",
        attrs.size.unwrap_or(0),
        name
    )
}

fn ok_status(id: u32) -> Status {
    Status {
        id,
        status_code: StatusCode::Ok,
        error_message: "Ok".into(),
        language_tag: "en-US".into(),
    }
}

fn err_status(id: u32, code: StatusCode, msg: impl Into<String>) -> Status {
    Status {
        id,
        status_code: code,
        error_message: msg.into(),
        language_tag: "en-US".into(),
    }
}

impl russh_sftp::server::Handler for FsSession {
    type Error = StatusCode;

    fn unimplemented(&self) -> Self::Error {
        StatusCode::OpUnsupported
    }

    async fn init(
        &mut self,
        version: u32,
        _extensions: HashMap<String, String>,
    ) -> Result<Version, Self::Error> {
        let _ = version;
        Ok(Version::new())
    }

    async fn open(
        &mut self,
        id: u32,
        filename: String,
        pflags: russh_sftp::protocol::OpenFlags,
        _attrs: FileAttributes,
    ) -> Result<Handle, Self::Error> {
        let path = self.resolve(&filename);
        use russh_sftp::protocol::OpenFlags as F;
        let mut opts = tokio::fs::OpenOptions::new();
        opts.read(pflags.contains(F::READ));
        opts.write(pflags.contains(F::WRITE));
        opts.append(pflags.contains(F::APPEND));
        opts.create(pflags.contains(F::CREATE));
        opts.truncate(pflags.contains(F::TRUNCATE));
        let file = opts
            .open(&path)
            .await
            .map_err(|_| StatusCode::NoSuchFile)?;
        let handle = self.next_handle();
        self.handles.insert(handle.clone(), HandleKind::File(file));
        Ok(Handle { id, handle })
    }

    async fn close(&mut self, id: u32, handle: String) -> Result<Status, Self::Error> {
        self.handles.remove(&handle);
        Ok(ok_status(id))
    }

    async fn read(
        &mut self,
        id: u32,
        handle: String,
        offset: u64,
        len: u32,
    ) -> Result<russh_sftp::protocol::Data, Self::Error> {
        let Some(h) = self.handles.get_mut(&handle) else {
            return Err(StatusCode::Failure);
        };
        let HandleKind::File(f) = h else {
            return Err(StatusCode::Failure);
        };
        f.seek(SeekFrom::Start(offset))
            .await
            .map_err(|_| StatusCode::Failure)?;
        let mut buf = vec![0u8; len as usize];
        let n = f.read(&mut buf).await.map_err(|_| StatusCode::Failure)?;
        if n == 0 {
            // Reads at EOF must answer with SSH_FX_EOF; an empty DATA packet
            // makes OpenSSH-style clients re-request the same range forever.
            return Err(StatusCode::Eof);
        }
        buf.truncate(n);
        Ok(russh_sftp::protocol::Data { id, data: buf })
    }

    async fn write(
        &mut self,
        id: u32,
        handle: String,
        offset: u64,
        data: Vec<u8>,
    ) -> Result<Status, Self::Error> {
        let Some(h) = self.handles.get_mut(&handle) else {
            return Err(StatusCode::Failure);
        };
        let HandleKind::File(f) = h else {
            return Err(StatusCode::Failure);
        };
        f.seek(SeekFrom::Start(offset))
            .await
            .map_err(|_| StatusCode::Failure)?;
        f.write_all(&data)
            .await
            .map_err(|_| StatusCode::Failure)?;
        Ok(ok_status(id))
    }

    async fn lstat(
        &mut self,
        id: u32,
        path: String,
    ) -> Result<russh_sftp::protocol::Attrs, Self::Error> {
        let p = self.resolve(&path);
        let meta = std::fs::symlink_metadata(&p);
        match meta {
            Ok(m) => {
                let f = attrs_from_meta(&Ok(m), "");
                Ok(russh_sftp::protocol::Attrs { id, attrs: f.attrs })
            }
            Err(_) => Err(StatusCode::NoSuchFile),
        }
    }

    async fn fstat(
        &mut self,
        id: u32,
        handle: String,
    ) -> Result<russh_sftp::protocol::Attrs, Self::Error> {
        let Some(h) = self.handles.get(&handle) else {
            return Err(StatusCode::Failure);
        };
        let HandleKind::File(f) = h else {
            return Err(StatusCode::Failure);
        };
        let meta = f.metadata().await.map_err(|_| StatusCode::Failure)?;
        let at = attrs_from_meta(&Ok(meta), "");
        let _ = id;
        Ok(russh_sftp::protocol::Attrs { id, attrs: at.attrs })
    }

    async fn setstat(
        &mut self,
        id: u32,
        path: String,
        _attrs: FileAttributes,
    ) -> Result<Status, Self::Error> {
        let _ = self.resolve(&path);
        Ok(ok_status(id))
    }

    async fn fsetstat(
        &mut self,
        id: u32,
        handle: String,
        _attrs: FileAttributes,
    ) -> Result<Status, Self::Error> {
        let _ = &handle;
        Ok(ok_status(id))
    }

    async fn opendir(&mut self, id: u32, path: String) -> Result<Handle, Self::Error> {
        let p = self.resolve(&path);
        let mut entries = Vec::new();
        let mut rd = tokio::fs::read_dir(&p)
            .await
            .map_err(|_| StatusCode::NoSuchFile)?;
        while let Ok(Some(entry)) = rd.next_entry().await {
            let name = entry.file_name().to_string_lossy().to_string();
            let meta = entry.metadata().await;
            entries.push(attrs_from_meta(&meta, &name));
        }
        let handle = self.next_handle();
        self.handles.insert(handle.clone(), HandleKind::Dir(entries));
        Ok(Handle { id, handle })
    }

    async fn readdir(&mut self, id: u32, handle: String) -> Result<Name, Self::Error> {
        let Some(h) = self.handles.get_mut(&handle) else {
            return Err(StatusCode::Failure);
        };
        let HandleKind::Dir(entries) = h else {
            return Err(StatusCode::Failure);
        };
        if entries.is_empty() {
            return Err(StatusCode::Eof);
        }
        let files = std::mem::take(entries);
        Ok(Name { id, files })
    }

    async fn remove(&mut self, id: u32, filename: String) -> Result<Status, Self::Error> {
        let p = self.resolve(&filename);
        tokio::fs::remove_file(&p)
            .await
            .map_err(|_| StatusCode::Failure)?;
        Ok(ok_status(id))
    }

    async fn mkdir(&mut self, id: u32, path: String, _attrs: FileAttributes) -> Result<Status, Self::Error> {
        let p = self.resolve(&path);
        tokio::fs::create_dir(&p)
            .await
            .map_err(|_| StatusCode::Failure)?;
        Ok(ok_status(id))
    }

    async fn rmdir(&mut self, id: u32, path: String) -> Result<Status, Self::Error> {
        let p = self.resolve(&path);
        tokio::fs::remove_dir(&p)
            .await
            .map_err(|_| StatusCode::Failure)?;
        Ok(ok_status(id))
    }

    async fn realpath(&mut self, id: u32, path: String) -> Result<Name, Self::Error> {
        let p = if path == "." {
            self.cwd.clone()
        } else {
            self.resolve(&path)
        };
        let canonical = p
            .canonicalize()
            .unwrap_or(p);
        let s = canonical.to_string_lossy().replace("\\\\?\\", "");
        let s = s.replace('\\', "/");
        let s = if s.len() > 1 && s.starts_with('/') && !Path::new(&s).exists() {
            s.trim_start_matches('/').to_string()
        } else {
            s
        };
        #[cfg(windows)]
        let s = {
            // Windows: report as "C:/..." without leading slash
            s.trim_start_matches('/').to_string()
        };
        Ok(Name {
            id,
            files: vec![File::new(s, FileAttributes::default())],
        })
    }

    async fn stat(&mut self, id: u32, path: String) -> Result<russh_sftp::protocol::Attrs, Self::Error> {
        let p = self.resolve(&path);
        let meta = tokio::fs::metadata(&p).await;
        match meta {
            Ok(m) => {
                let f = attrs_from_meta(&Ok(m), "");
                let _ = id;
                Ok(russh_sftp::protocol::Attrs { id, attrs: f.attrs })
            }
            Err(_) => Err(StatusCode::NoSuchFile),
        }
    }

    async fn rename(
        &mut self,
        id: u32,
        oldpath: String,
        newpath: String,
    ) -> Result<Status, Self::Error> {
        let from = self.resolve(&oldpath);
        let to = self.resolve(&newpath);
        tokio::fs::rename(&from, &to)
            .await
            .map_err(|_| StatusCode::Failure)?;
        Ok(ok_status(id))
    }

    async fn readlink(&mut self, id: u32, path: String) -> Result<Name, Self::Error> {
        let p = self.resolve(&path);
        let target = tokio::fs::read_link(&p)
            .await
            .map_err(|_| StatusCode::NoSuchFile)?;
        Ok(Name {
            id,
            files: vec![File::new(
                target.to_string_lossy(),
                FileAttributes::default(),
            )],
        })
    }
}
