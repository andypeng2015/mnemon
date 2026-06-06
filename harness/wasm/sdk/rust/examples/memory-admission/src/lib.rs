use mnemon_wasm_sdk::{alloc_bytes, pack};

#[link(wasm_import_module = "env")]
extern "C" {
    fn read_state_view(ptr: u32, len: u32) -> u32;
}

const PROPOSE: &[u8] = br#"{"Verdict":"propose"}"#;
const DENY_EMPTY: &[u8] =
    br#"{"Verdict":"deny","Reasons":["memory candidate denied: empty content"]}"#;
const DENY_SECRET: &[u8] =
    br#"{"Verdict":"deny","Reasons":["memory candidate denied: secret-like content"]}"#;
const DENY_INJECTION: &[u8] =
    br#"{"Verdict":"deny","Reasons":["memory candidate denied: prompt-injection-shaped content"]}"#;
const DENY_SOURCE: &[u8] =
    br#"{"Verdict":"deny","Reasons":["memory candidate denied: missing source"]}"#;
const DENY_CONFIDENCE: &[u8] =
    br#"{"Verdict":"deny","Reasons":["memory candidate denied: missing confidence"]}"#;

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
    let lower = input
        .iter()
        .map(|b| b.to_ascii_lowercase())
        .collect::<Vec<u8>>();
    if !contains(&lower, br#""content":""#) && !contains(&lower, br#""content":"#) {
        return DENY_EMPTY;
    }
    if contains(&lower, br#""content":"""#) {
        return DENY_EMPTY;
    }
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
    ] {
        if contains(&lower, marker) {
            return DENY_SECRET;
        }
    }
    for marker in [
        b"ignore previous instructions" as &[u8],
        b"disregard previous instructions",
        b"reveal the system prompt",
        b"show the system prompt",
        b"developer message",
        b"act as system",
    ] {
        if contains(&lower, marker) {
            return DENY_INJECTION;
        }
    }
    if contains(&lower, br#""source":"""#) || !contains(&lower, br#""source":"#) {
        return DENY_SOURCE;
    }
    if contains(&lower, br#""confidence":"""#) || !contains(&lower, br#""confidence":"#) {
        return DENY_CONFIDENCE;
    }
    PROPOSE
}

fn contains(haystack: &[u8], needle: &[u8]) -> bool {
    haystack.windows(needle.len()).any(|window| window == needle)
}
