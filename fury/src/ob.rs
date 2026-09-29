//! Compile-time string obfuscation: literals never hit .rodata as plaintext
//! (defeats trivial `strings` / static YARA rules on protocol keywords).

/// Decode an obfuscated literal back into a String at runtime.
pub fn de(enc: &[u8], key: u8) -> String {
    let mut out = Vec::with_capacity(enc.len());
    for (i, b) in enc.iter().enumerate() {
        out.push(b ^ key.wrapping_add(i as u8));
    }
    String::from_utf8_lossy(&out).into_owned()
}

/// obf!("literal") -> String, with the literal XOR-encoded in the binary.
#[macro_export]
macro_rules! ob {
    ($s:literal) => {{
        const N: usize = $s.len();
        const K: u8 = (N as u8).wrapping_mul(13) | 0x27;
        const E: [u8; N] = {
            let b = $s.as_bytes();
            let mut o = [0u8; N];
            let mut i = 0;
            while i < N {
                o[i] = b[i] ^ K.wrapping_add(i as u8);
                i += 1;
            }
            o
        };
        $crate::ob::de(&E, K)
    }};
}
