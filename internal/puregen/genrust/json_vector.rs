// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

use crate::{
    basictl::{self, Result},
    json,
};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use std::str::FromStr;

const BYTE_ONES: u64 = 0x0101_0101_0101_0101;
const BYTE_HIGH: u64 = 0x8080_8080_8080_8080;

#[inline]
fn zero_bytes(word: u64) -> u64 {
    word.wrapping_sub(BYTE_ONES) & !word & BYTE_HIGH
}

#[inline]
fn plain_ascii(bytes: &[u8]) -> bool {
    let chunks = bytes.chunks_exact(8);
    // Reduce the byte tests without branching per word so the compiler can
    // vectorize validation of long strings.
    let invalid = chunks.clone().fold(0, |invalid, chunk| {
        let word = u64::from_le_bytes(chunk.try_into().unwrap());
        invalid
            | (word & BYTE_HIGH)
            | zero_bytes(word & (BYTE_ONES * 0xe0))
            | zero_bytes(word ^ (BYTE_ONES * u64::from(b'"')))
            | zero_bytes(word ^ (BYTE_ONES * u64::from(b'\\')))
    });
    invalid == 0
        && chunks
            .remainder()
            .iter()
            .all(|&byte| (0x20..0x80).contains(&byte) && byte != b'"' && byte != b'\\')
}

#[inline]
pub(crate) fn write_bytes(buf: &mut Vec<u8>, bytes: &[u8]) -> Result<()> {
    if plain_ascii(bytes) {
        buf.reserve(bytes.len() + 2);
        buf.push(b'"');
        buf.extend_from_slice(bytes);
        buf.push(b'"');
    } else if let Ok(text) = std::str::from_utf8(bytes) {
        serde_json::to_writer(buf, text).map_err(|err| json::error(err.to_string()))?;
    } else {
        let len = base64::encoded_len(bytes.len(), true).ok_or(basictl::Error::InvalidLength)?;
        buf.extend_from_slice(br#"{"base64":""#);
        let start = buf.len();
        let end = start
            .checked_add(len)
            .ok_or(basictl::Error::InvalidLength)?;
        buf.resize(end, 0);
        STANDARD
            .encode_slice(bytes, &mut buf[start..])
            .expect("exact base64 length");
        buf.extend_from_slice(br#""}"#);
    }
    Ok(())
}

pub(crate) fn write_integers<T: Copy + itoa::Integer>(
    buf: &mut Vec<u8>,
    prefix: &[u8],
    values: &[T],
) -> Result<()> {
    let mut integer = itoa::Buffer::new();
    if values.len() < 16 {
        return write_object(buf, prefix, values, |buf, value| {
            buf.extend_from_slice(itoa::Buffer::new().format(*value).as_bytes());
            Ok(())
        });
    }

    buf.extend_from_slice(prefix);
    // Format numbers in batches to keep per-element work off the output Vec.
    // A signed 128-bit integer and its separator fit in 41 bytes.
    const MAX_ELEMENT_LEN: usize = 41;
    let mut batch = [0; 512];
    let first = integer.format(values[0]).as_bytes();
    batch[..first.len()].copy_from_slice(first);
    let mut end = first.len();
    for &value in &values[1..] {
        if end > batch.len() - MAX_ELEMENT_LEN {
            buf.extend_from_slice(&batch[..end]);
            end = 0;
        }
        let text = integer.format(value).as_bytes();
        batch[end] = b',';
        end += 1;
        batch[end..end + text.len()].copy_from_slice(text);
        end += text.len();
    }
    buf.extend_from_slice(&batch[..end]);
    buf.extend_from_slice(b"]}");
    Ok(())
}

#[inline]
pub(crate) fn write_object<T>(
    buf: &mut Vec<u8>,
    prefix: &[u8],
    values: &[T],
    write: impl Fn(&mut Vec<u8>, &T) -> Result<()>,
) -> Result<()> {
    let Some((first, rest)) = values.split_first() else {
        buf.extend_from_slice(b"{}");
        return Ok(());
    };
    basictl::append(buf, |buf| {
        buf.extend_from_slice(prefix);
        write(buf, first)?;
        for value in rest {
            buf.push(b',');
            write(buf, value)?;
        }
        buf.extend_from_slice(b"]}");
        Ok(())
    })
}

#[inline]
pub(crate) fn read_string(input: &[u8], prefix: &[u8], values: &mut Vec<Vec<u8>>) -> bool {
    if input == b"{}" {
        values.clear();
        return true;
    }
    // Borrow the bytes directly in the common case. A string with escapes,
    // non-ASCII, or a different object layout goes through the full decoder.
    let Some(text) = input
        .strip_prefix(prefix)
        .and_then(|input| input.strip_prefix(b"\""))
        .and_then(|input| input.strip_suffix(b"\"]}"))
    else {
        return false;
    };
    if plain_ascii(text) {
        basictl::replace_one_string(values, text);
        return true;
    }
    match memchr::memchr(b'"', text) {
        Some(quote) => read_plain_strings(text, quote, values),
        None => false,
    }
}

fn read_plain_strings(mut text: &[u8], mut quote: usize, values: &mut Vec<Vec<u8>>) -> bool {
    let mut parts = Vec::new();
    loop {
        let (part, tail) = text.split_at(quote);
        let Some(rest) = tail.strip_prefix(br#"",""#) else {
            return false;
        };
        if !plain_ascii(part) {
            return false;
        }
        parts.push(part);
        text = rest;
        match memchr::memchr(b'"', text) {
            Some(next) => quote = next,
            None => break,
        }
    }
    if !plain_ascii(text) {
        return false;
    }
    parts.push(text);
    values.resize_with(parts.len(), Vec::new);
    for (value, part) in values.iter_mut().zip(parts) {
        basictl::replace_bytes(value, part);
    }
    true
}

#[inline]
fn read_integer<T: FromStr>(input: &mut &[u8]) -> Option<T> {
    let negative = input.first() == Some(&b'-');
    let start = usize::from(negative);
    let mut end = start;
    while input.get(end).is_some_and(u8::is_ascii_digit) {
        end += 1;
    }
    if end == start || input[start] == b'0' && (negative || end > start + 1) {
        return None;
    }
    let (text, rest) = input.split_at(end);
    let number = std::str::from_utf8(text).ok()?.parse().ok()?;
    *input = rest;
    Some(number)
}

pub(crate) fn read_integers<T: Copy + Default + FromStr>(
    input: &[u8],
    prefix: &[u8],
    values: &mut Vec<T>,
) -> bool {
    if input == b"{}" {
        values.clear();
        return true;
    }
    let Some(mut input) = input
        .strip_prefix(prefix)
        .and_then(|input| input.strip_suffix(b"]}"))
    else {
        return false;
    };
    if input.is_empty() {
        values.clear();
        return true;
    }

    // Delay writes to the receiver until the entire array is valid. Most
    // short arrays fit on the stack.
    let mut inline = [T::default(); 8];
    let mut len = 0;
    let mut overflow = Vec::new();
    loop {
        let Some(value) = read_integer(&mut input) else {
            return false;
        };
        if let Some(slot) = inline.get_mut(len) {
            *slot = value;
        } else {
            overflow.push(value);
        }
        len += 1;
        if input.is_empty() {
            break;
        }
        let Some(rest) = input.strip_prefix(b",") else {
            return false;
        };
        input = rest;
    }
    if len <= inline.len() {
        values.clear();
        values.extend_from_slice(&inline[..len]);
    } else {
        values.clear();
        values.reserve(len);
        values.extend_from_slice(&inline);
        values.extend_from_slice(&overflow);
    }
    true
}
