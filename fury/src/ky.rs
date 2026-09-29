use russh::keys::{decode_secret_key, PrivateKey, PrivateKeyWithHashAlg};
use std::sync::Arc;
use std::sync::OnceLock;

// build-time injected: base64 OpenSSH/PKCS8 private key (FURY_KEY_B64 env at compile time)
static EMBEDDED_KEY_B64: Option<&str> = option_env!("FURY_KEY_B64");

static KEY: OnceLock<Arc<PrivateKey>> = OnceLock::new();

pub fn load() -> Result<Arc<PrivateKey>, String> {
    if let Some(k) = KEY.get() {
        return Ok(k.clone());
    }
    let pem = EMBEDDED_KEY_B64.map(base64_decode).unwrap_or_default();
    let key = if pem.trim().is_empty() {
        PrivateKey::random(&mut rand::rng(), russh::keys::Algorithm::Ed25519)
            .map_err(|e| format!("keygen: {e}"))?
    } else {
        decode_secret_key(&pem, None).map_err(|e| format!("key decode: {e}"))?
    };
    let arc = Arc::new(key);
    let _ = KEY.set(arc.clone());
    Ok(arc)
}

pub fn with_hash_alg(key: Arc<PrivateKey>) -> PrivateKeyWithHashAlg {
    PrivateKeyWithHashAlg::new(key, None)
}

pub fn public_key_blob(key: &PrivateKey) -> Vec<u8> {
    use russh::keys::PublicKeyBase64;
    key.public_key().public_key_bytes()
}

pub fn public_key_line(key: &PrivateKey) -> String {
    let blob = public_key_blob(key);
    let b64 = base64_encode(&blob);
    let kind = key.algorithm().to_string();
    format!("{kind} {b64}")
}

fn base64_decode(s: &str) -> String {
    const TABLE: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = Vec::new();
    let mut buf: u32 = 0;
    let mut bits = 0u32;
    for c in s.bytes() {
        if c == b'=' || c == b'\n' || c == b'\r' {
            continue;
        }
        let Some(pos) = TABLE.iter().position(|&t| t == c) else { continue };
        buf = (buf << 6) | pos as u32;
        bits += 6;
        if bits >= 8 {
            bits -= 8;
            out.push(((buf >> bits) & 0xff) as u8);
        }
    }
    String::from_utf8_lossy(&out).to_string()
}

fn base64_encode(data: &[u8]) -> String {
    const TABLE: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::new();
    for chunk in data.chunks(3) {
        let b = [
            chunk[0],
            chunk.get(1).copied().unwrap_or(0),
            chunk.get(2).copied().unwrap_or(0),
        ];
        let n = ((b[0] as u32) << 16) | ((b[1] as u32) << 8) | b[2] as u32;
        out.push(TABLE[(n >> 18) as usize & 63] as char);
        out.push(TABLE[(n >> 12) as usize & 63] as char);
        out.push(if chunk.len() > 1 { TABLE[(n >> 6) as usize & 63] as char } else { '=' });
        out.push(if chunk.len() > 2 { TABLE[n as usize & 63] as char } else { '=' });
    }
    out
}

pub fn hex(data: &[u8]) -> String {
    let mut out = String::with_capacity(data.len() * 2);
    for b in data {
        out.push_str(&format!("{b:02x}"));
    }
    out
}
