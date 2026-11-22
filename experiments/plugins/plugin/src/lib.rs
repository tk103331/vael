//! A Vael plugin, built for wasm: it counts the words of the text it is
//! given.
//!
//! The editor and a plugin speak one call: the host writes a JSON request
//! into the plugin's memory and calls `vael_run` with where it is, the
//! plugin answers with where its JSON answer is, and the host applies
//! what the answer says.

use std::alloc::{alloc, Layout};

/// Gives the host room of `size` bytes it writes a request into.
#[no_mangle]
pub extern "C" fn vael_alloc(size: usize) -> *mut u8 {
    if size == 0 {
        return std::ptr::null_mut();
    }
    unsafe { alloc(Layout::from_size_align(size, 8).unwrap()) }
}

/// Runs a command of the plugin: `ptr` is the request, the top half of
/// the answer is where it is and the bottom half how long it is.
#[no_mangle]
pub extern "C" fn vael_run(ptr: *const u8, size: usize) -> u64 {
    let request = unsafe { std::slice::from_raw_parts(ptr, size) };
    let request = String::from_utf8_lossy(request).into_owned();
    if request.contains("\"command\":\"loop\"") {
        // A plugin that never stops, which the editor must be able to.
        loop {}
    }
    let bytes = handle(&request).into_bytes().into_boxed_slice();
    let len = bytes.len() as u64;
    let at = Box::into_raw(bytes) as *mut u8 as u64;
    (at << 32) | len
}

fn handle(request: &str) -> String {
    let text = text_of(request);
    let words = text.split_whitespace().count();
    let lines = text.lines().count();
    let chars = text.chars().count();
    format!(
        "{{\"notify\":\"{} 词 · {} 行 · {} 字符\"}}",
        words, lines, chars
    )
}

/// text_of reads the text of a request, and undoes the escapes JSON puts
/// into a string.
fn text_of(request: &str) -> String {
    let mut text = String::new();
    if let Some(at) = request.find("\"text\":\"") {
        let rest = &request[at + 8..];
        if let Some(end) = rest.rfind('"') {
            let mut chars = rest[..end].chars();
            while let Some(c) = chars.next() {
                if c != '\\' {
                    text.push(c);
                    continue;
                }
                match chars.next() {
                    Some('n') => text.push('\n'),
                    Some('t') => text.push('\t'),
                    Some('r') => text.push('\r'),
                    Some('"') => text.push('"'),
                    Some('\\') => text.push('\\'),
                    Some(other) => text.push(other),
                    None => break,
                }
            }
        }
    }
    text
}