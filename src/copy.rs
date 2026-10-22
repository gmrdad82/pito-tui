use std::io::{self, Write};

pub const COPY_MAX: usize = 48 * 1024;

pub fn copy(text: &str) -> bool {
    if text.len() > COPY_MAX {
        return false;
    }
    let mut out = io::stdout();
    write!(out, "\x1b]52;c;{}\x07", base64(text.as_bytes())).is_ok() && out.flush().is_ok()
}

pub fn base64(bytes: &[u8]) -> String {
    const ABC: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::with_capacity(bytes.len().div_ceil(3) * 4);
    for chunk in bytes.chunks(3) {
        let b = [
            chunk[0],
            *chunk.get(1).unwrap_or(&0),
            *chunk.get(2).unwrap_or(&0),
        ];
        let n = (u32::from(b[0]) << 16) | (u32::from(b[1]) << 8) | u32::from(b[2]);
        for k in 0..4 {
            if k <= chunk.len() {
                out.push(ABC[(n >> (18 - 6 * k) & 63) as usize] as char);
            } else {
                out.push('=');
            }
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn base64_matches_the_standard_alphabet() {
        assert_eq!(base64(b""), "");
        assert_eq!(base64(b"f"), "Zg==");
        assert_eq!(base64(b"fo"), "Zm8=");
        assert_eq!(base64(b"foo"), "Zm9v");
        assert_eq!(base64("panic: ü".as_bytes()), "cGFuaWM6IMO8");
    }

    fn decode(text: &str) -> Vec<u8> {
        const ABC: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
        let mut out = Vec::new();
        for quad in text.as_bytes().chunks(4) {
            let mut n = 0u32;
            let mut pad = 0;
            for &c in quad {
                n <<= 6;
                match ABC.iter().position(|&a| a == c) {
                    Some(at) => n |= at as u32,
                    None => pad += 1,
                }
            }
            let bytes = [(n >> 16) as u8, (n >> 8) as u8, n as u8];
            out.extend_from_slice(&bytes[..3 - pad]);
        }
        out
    }

    #[test]
    fn base64_matches_the_rfc_4648_test_vectors() {
        let vectors: [(&[u8], &str); 7] = [
            (b"", ""),
            (b"f", "Zg=="),
            (b"fo", "Zm8="),
            (b"foo", "Zm9v"),
            (b"foob", "Zm9vYg=="),
            (b"fooba", "Zm9vYmE="),
            (b"foobar", "Zm9vYmFy"),
        ];
        for (bytes, encoded) in vectors {
            assert_eq!(base64(bytes), encoded);
        }
    }

    #[test]
    fn base64_uses_plus_and_slash_for_the_top_of_the_alphabet() {
        assert_eq!(base64(&[0, 0, 0]), "AAAA");
        assert_eq!(base64(&[0xff, 0xff, 0xff]), "////");
        assert_eq!(base64(&[0xfb, 0xff]), "+/8=");
        assert_eq!(base64(&[0xfb, 0xef, 0xbe]), "++++");
        assert_eq!(base64(&[0]), "AA==");
        assert_eq!(base64("日本".as_bytes()), "5pel5pys");
    }

    #[test]
    fn base64_pads_to_whole_quads_and_round_trips_every_byte() {
        let all: Vec<u8> = (0..=255).collect();
        for len in 0..=all.len() {
            let bytes = &all[..len];
            let encoded = base64(bytes);
            assert_eq!(encoded.len(), len.div_ceil(3) * 4, "{len} bytes");
            assert_eq!(encoded.trim_end_matches('=').len(), (len * 4).div_ceil(3), "{len} bytes");
            assert!(
                encoded
                    .bytes()
                    .all(|c| c.is_ascii_alphanumeric() || c == b'+' || c == b'/' || c == b'='),
                "{len} bytes"
            );
            assert_eq!(decode(&encoded), bytes, "{len} bytes");
        }
        let reversed: Vec<u8> = (0..=255).rev().collect();
        assert_eq!(decode(&base64(&reversed)), reversed);
    }

    #[test]
    fn copy_refuses_text_over_the_limit_without_writing() {
        assert_eq!(COPY_MAX, 48 * 1024);
        assert!(!copy(&"a".repeat(COPY_MAX + 1)));
        assert!(!copy(&"a".repeat(COPY_MAX * 4)));
    }

    #[test]
    fn the_copy_limit_counts_bytes_not_characters() {
        let wide = "é".repeat(COPY_MAX / 2 + 1);
        assert!(wide.chars().count() < COPY_MAX);
        assert!(wide.len() > COPY_MAX);
        assert!(!copy(&wide));
    }
}
