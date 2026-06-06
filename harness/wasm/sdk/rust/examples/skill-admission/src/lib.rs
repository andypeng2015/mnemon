use mnemon_wasm_sdk::{alloc_bytes, pack};

#[link(wasm_import_module = "env")]
extern "C" {
    fn read_state_view(ptr: u32, len: u32) -> u32;
}

const PROPOSE: &[u8] = br#"{"Verdict":"propose"}"#;
const DENY_MISSING_ID: &[u8] =
    br#"{"Verdict":"deny","Reasons":["skill candidate denied: missing skill_id"]}"#;
const DENY_INVALID_ID: &[u8] =
    br#"{"Verdict":"deny","Reasons":["skill candidate denied: invalid skill_id"]}"#;
const DENY_INVALID_STATUS: &[u8] =
    br#"{"Verdict":"deny","Reasons":["skill candidate denied: invalid status"]}"#;
const DENY_SOURCE: &[u8] =
    br#"{"Verdict":"deny","Reasons":["skill candidate denied: missing source"]}"#;
const DENY_CONFIDENCE: &[u8] =
    br#"{"Verdict":"deny","Reasons":["skill candidate denied: missing confidence"]}"#;
const DENY_UNSAFE: &[u8] =
    br#"{"Verdict":"deny","Reasons":["skill candidate denied: unsafe content"]}"#;

#[no_mangle]
pub extern "C" fn alloc(len: u32) -> u32 {
    alloc_bytes(len as usize) as u32
}

#[no_mangle]
pub extern "C" fn evaluate(ptr: u32, len: u32) -> u64 {
    let _ = unsafe { read_state_view(0, 0) };
    let input = unsafe { core::slice::from_raw_parts(ptr as *const u8, len as usize) };
    let decision = admission_decision(input);
    pack(decision.as_ptr() as u32, decision.len() as u32)
}

fn admission_decision(input: &[u8]) -> &'static [u8] {
    let skill_id = match json_string(input, b"skill_id") {
        Some(value) if !trim_ascii(value).is_empty() => trim_ascii(value),
        _ => return DENY_MISSING_ID,
    };
    if !valid_skill_id(skill_id) {
        return DENY_INVALID_ID;
    }

    if let Some(status) = json_string(input, b"status") {
        let status = trim_ascii(status);
        if !status.is_empty()
            && status != b"active"
            && status != b"stale"
            && status != b"archived"
        {
            return DENY_INVALID_STATUS;
        }
    }

    let source = json_string(input, b"source").map(trim_ascii).unwrap_or_default();
    if source.is_empty() {
        return DENY_SOURCE;
    }

    let confidence = json_string(input, b"confidence")
        .map(trim_ascii)
        .unwrap_or_default();
    if confidence.is_empty() {
        return DENY_CONFIDENCE;
    }

    let lower = input
        .iter()
        .map(|b| b.to_ascii_lowercase())
        .collect::<Vec<u8>>();
    if unsafe_content(&lower) {
        return DENY_UNSAFE;
    }

    PROPOSE
}

fn valid_skill_id(value: &[u8]) -> bool {
    value
        .iter()
        .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || *b == b'-')
}

fn unsafe_content(lower_input: &[u8]) -> bool {
    for marker in [
        b"password=" as &[u8],
        b"password:",
        b"api_key",
        b"api key",
        b"secret=",
        b"secret:",
        b"token=",
        b"token:",
        b"bearer ",
        b"private key",
        b"-----begin",
        b"sk-",
        b"ignore previous instructions",
        b"disregard previous instructions",
        b"reveal the system prompt",
        b"show the system prompt",
        b"developer message",
        b"act as system",
    ] {
        if contains(lower_input, marker) {
            return true;
        }
    }
    false
}

fn json_string<'a>(input: &'a [u8], key: &[u8]) -> Option<&'a [u8]> {
    let mut i = 0;
    while i < input.len() {
        if input[i] != b'"' || !starts_with(&input[i + 1..], key) {
            i += 1;
            continue;
        }
        let mut pos = i + 1 + key.len();
        if pos >= input.len() || input[pos] != b'"' {
            i += 1;
            continue;
        }
        pos += 1;
        pos = skip_ws(input, pos);
        if pos >= input.len() || input[pos] != b':' {
            i += 1;
            continue;
        }
        pos += 1;
        pos = skip_ws(input, pos);
        if pos >= input.len() || input[pos] != b'"' {
            return None;
        }
        pos += 1;
        let start = pos;
        while pos < input.len() {
            if input[pos] == b'\\' {
                pos += 2;
                continue;
            }
            if input[pos] == b'"' {
                return Some(&input[start..pos]);
            }
            pos += 1;
        }
        return None;
    }
    None
}

fn skip_ws(input: &[u8], mut pos: usize) -> usize {
    while pos < input.len()
        && (input[pos] == b' ' || input[pos] == b'\n' || input[pos] == b'\r' || input[pos] == b'\t')
    {
        pos += 1;
    }
    pos
}

fn trim_ascii(mut value: &[u8]) -> &[u8] {
    while let Some((first, rest)) = value.split_first() {
        if !first.is_ascii_whitespace() {
            break;
        }
        value = rest;
    }
    while let Some((last, rest)) = value.split_last() {
        if !last.is_ascii_whitespace() {
            break;
        }
        value = rest;
    }
    value
}

fn starts_with(haystack: &[u8], needle: &[u8]) -> bool {
    haystack.len() >= needle.len() && &haystack[..needle.len()] == needle
}

fn contains(haystack: &[u8], needle: &[u8]) -> bool {
    haystack
        .windows(needle.len())
        .any(|window| window == needle)
}
