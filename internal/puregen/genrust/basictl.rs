// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

use std::fmt;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Error {
    UnexpectedEof,
    WrongTag { expected: u32, actual: u32 },
    UnexpectedMagic(u32),
    InvalidLength,
    InvalidPadding,
    MissingField(&'static str),
    UnexpectedConstructor(usize),
    NoTL1(&'static str),
    UnsupportedFunction,
    InvalidJson(String),
    MissingRpcPayload,
    WrongRpcResponse,
    NoTL2(&'static str),
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::UnexpectedEof => f.write_str("unexpected end of TL1 input"),
            Self::WrongTag { expected, actual } => {
                write!(f, "expected TL1 tag #{expected:08x}, found #{actual:08x}")
            }
            Self::UnexpectedMagic(tag) => write!(f, "unexpected TL1 tag #{tag:08x}"),
            Self::InvalidLength => f.write_str("invalid TL1 length"),
            Self::InvalidPadding => f.write_str("non-zero TL1 string padding"),
            Self::MissingField(field) => write!(f, "missing field {field}"),
            Self::UnexpectedConstructor(index) => write!(f, "unexpected TL2 constructor {index}"),
            Self::NoTL1(name) => write!(f, "{name} has no TL1 representation"),
            Self::UnsupportedFunction => f.write_str("generic function wrappers are unsupported"),
            Self::InvalidJson(reason) => write!(f, "invalid TL-JSON: {reason}"),
            Self::MissingRpcPayload => f.write_str("missing nested RPC request or response"),
            Self::WrongRpcResponse => f.write_str("response does not match the RPC request"),
            Self::NoTL2(name) => write!(f, "{name} has no generated TL2 codec"),
        }
    }
}

impl std::error::Error for Error {}

pub type Result<T> = std::result::Result<T, Error>;

const MAX_SHORT: usize = 253;
const MAX_MEDIUM: usize = (1 << 24) - 1;
const MAX_LONG: u64 = (1 << 56) - 1;

#[inline]
pub(crate) fn take<'a>(input: &mut &'a [u8], count: usize) -> Result<&'a [u8]> {
    let (value, rest) = input.split_at_checked(count).ok_or(Error::UnexpectedEof)?;
    *input = rest;
    Ok(value)
}

#[inline]
pub(crate) fn take_array<const N: usize>(input: &mut &[u8]) -> Result<[u8; N]> {
    take(input, N)?.try_into().map_err(|_| Error::UnexpectedEof)
}

pub(crate) trait TLRead {
    fn read_u8(&mut self) -> Result<u8>;
    fn read_u32(&mut self) -> Result<u32>;
    fn read_u64(&mut self) -> Result<u64>;
    fn read_i32(&mut self) -> Result<i32>;
    fn read_i64(&mut self) -> Result<i64>;
    fn read_f32(&mut self) -> Result<f32>;
    fn read_f64(&mut self) -> Result<f64>;
    fn read_exact_tag(&mut self, expected: u32) -> Result<()>;
    fn read_bool(&mut self, false_tag: u32, true_tag: u32) -> Result<bool>;
    fn read_string(&mut self, value: &mut Vec<u8>) -> Result<()>;
}

impl TLRead for &[u8] {
    fn read_u8(&mut self) -> Result<u8> {
        Ok(u8::from_le_bytes(take_array(self)?))
    }

    fn read_u32(&mut self) -> Result<u32> {
        Ok(u32::from_le_bytes(take_array(self)?))
    }

    fn read_u64(&mut self) -> Result<u64> {
        Ok(u64::from_le_bytes(take_array(self)?))
    }

    fn read_i32(&mut self) -> Result<i32> {
        Ok(i32::from_le_bytes(take_array(self)?))
    }

    fn read_i64(&mut self) -> Result<i64> {
        Ok(i64::from_le_bytes(take_array(self)?))
    }

    fn read_f32(&mut self) -> Result<f32> {
        Ok(f32::from_le_bytes(take_array(self)?))
    }

    fn read_f64(&mut self) -> Result<f64> {
        Ok(f64::from_le_bytes(take_array(self)?))
    }

    fn read_exact_tag(&mut self, expected: u32) -> Result<()> {
        let actual = self.read_u32()?;
        if actual != expected {
            return Err(Error::WrongTag { expected, actual });
        }
        Ok(())
    }

    fn read_bool(&mut self, false_tag: u32, true_tag: u32) -> Result<bool> {
        match self.read_u32()? {
            actual if actual == false_tag => Ok(false),
            actual if actual == true_tag => Ok(true),
            actual => Err(Error::UnexpectedMagic(actual)),
        }
    }

    fn read_string(&mut self, value: &mut Vec<u8>) -> Result<()> {
        replace_bytes(value, read_string_slice(self)?);
        Ok(())
    }
}

#[inline]
pub(crate) fn read_string_slice<'a>(input: &mut &'a [u8]) -> Result<&'a [u8]> {
    let (len, header_len) = match input.read_u8()? {
        254 => {
            let [a, b, c] = take_array(input)?;
            let len = u32::from_le_bytes([a, b, c, 0]) as usize;
            if len <= MAX_SHORT {
                return Err(Error::InvalidLength);
            }
            (len, 4)
        }
        255 => {
            let [a, b, c, d, e, f, g] = take_array(input)?;
            let len = usize::try_from(u64::from_le_bytes([a, b, c, d, e, f, g, 0]))
                .map_err(|_| Error::InvalidLength)?;
            if len <= MAX_MEDIUM {
                return Err(Error::InvalidLength);
            }
            (len, 8)
        }
        len => (usize::from(len), 1),
    };
    let padding = (4 - (len % 4 + header_len) % 4) % 4;
    let total = len.checked_add(padding).ok_or(Error::InvalidLength)?;
    let (bytes, padding) = take(input, total)?.split_at(len);
    if padding.iter().any(|&byte| byte != 0) {
        return Err(Error::InvalidPadding);
    }
    Ok(bytes)
}

#[inline]
pub(crate) fn replace_bytes(value: &mut Vec<u8>, bytes: &[u8]) {
    if value.len() == bytes.len() {
        value.copy_from_slice(bytes);
    } else {
        value.clear();
        value.extend_from_slice(bytes);
    }
}

#[inline]
pub(crate) fn write_tag(output: &mut Vec<u8>, tag: u32) {
    output.extend_from_slice(&tag.to_le_bytes());
}

#[inline]
pub(crate) fn write_string(output: &mut Vec<u8>, value: &[u8]) -> Result<()> {
    let len = value.len();
    let len_bytes = (len as u64).to_le_bytes();
    let header_len = if len <= MAX_SHORT {
        output.push(len_bytes[0]);
        1
    } else if len <= MAX_MEDIUM {
        output.push(254);
        output.extend_from_slice(&len_bytes[..3]);
        4
    } else if (len as u64) <= MAX_LONG {
        output.push(255);
        output.extend_from_slice(&len_bytes[..7]);
        8
    } else {
        return Err(Error::InvalidLength);
    };
    output.extend_from_slice(value);
    output.extend_from_slice(&[0; 3][..(4 - (len % 4 + header_len) % 4) % 4]);
    Ok(())
}

#[inline]
pub(crate) fn append(
    output: &mut Vec<u8>,
    write: impl FnOnce(&mut Vec<u8>) -> Result<()>,
) -> Result<()> {
    let len = output.len();
    let result = write(output);
    if result.is_err() {
        output.truncate(len);
    }
    result
}

pub(crate) fn read_fixed_sequence<T, const WIDTH: usize>(
    input: &mut &[u8],
    count: usize,
    from_bytes: impl Fn([u8; WIDTH]) -> T,
) -> Result<Vec<T>> {
    let mut values = Vec::new();
    read_fixed_sequence_into(input, count, &mut values, from_bytes)?;
    Ok(values)
}

#[inline]
pub(crate) fn read_fixed_sequence_into<T, const WIDTH: usize>(
    input: &mut &[u8],
    count: usize,
    values: &mut Vec<T>,
    from_bytes: impl Fn([u8; WIDTH]) -> T,
) -> Result<()> {
    let len = count.checked_mul(WIDTH).ok_or(Error::InvalidLength)?;
    let data = take(input, len)?;
    values
        .try_reserve_exact(count.saturating_sub(values.len()))
        .map_err(|_| Error::InvalidLength)?;
    values.clear();
    values.extend(
        data.chunks_exact(WIDTH)
            .map(|chunk| from_bytes(chunk.try_into().expect("exact-width chunk"))),
    );
    Ok(())
}

#[inline]
pub(crate) fn replace_one_string(values: &mut Vec<Vec<u8>>, bytes: &[u8]) {
    if let Some(first) = values.first_mut() {
        replace_bytes(first, bytes);
        values.truncate(1);
    } else {
        values.push(bytes.to_vec());
    }
}

#[inline]
pub(crate) fn read_strings_into<'a>(
    input: &mut &'a [u8],
    count: usize,
    values: &mut Vec<Vec<u8>>,
    read: impl Fn(&mut &'a [u8]) -> Result<&'a [u8]>,
) -> Result<()> {
    match count {
        0 => values.clear(),
        1 => replace_one_string(values, read(input)?),
        _ => read_many_strings_into(input, count, values, read)?,
    }
    Ok(())
}

fn read_many_strings_into<'a>(
    input: &mut &'a [u8],
    count: usize,
    values: &mut Vec<Vec<u8>>,
    read: impl Fn(&mut &'a [u8]) -> Result<&'a [u8]>,
) -> Result<()> {
    // Validate the entire vector before reusing any of its buffers.
    let mut rest = *input;
    for _ in 0..count {
        read(&mut rest)?;
    }
    values
        .try_reserve_exact(count.saturating_sub(values.len()))
        .map_err(|_| Error::InvalidLength)?;
    values.resize_with(count, Vec::new);
    let mut validated = *input;
    for value in values {
        replace_bytes(value, read(&mut validated).expect("validated string"));
    }
    *input = rest;
    Ok(())
}

#[inline]
pub(crate) fn write_one_string_vector(buf: &mut Vec<u8>, text: &[u8]) -> Result<()> {
    let len = text.len();
    if len > MAX_SHORT {
        write_tag(buf, 1);
        return write_string(buf, text);
    }
    let padding = 3 - (len & 3);
    buf.reserve(5 + len + padding);
    buf.extend_from_slice(&[1, 0, 0, 0, len as u8]);
    buf.extend_from_slice(text);
    buf.extend_from_slice(&[0; 3][..padding]);
    Ok(())
}
