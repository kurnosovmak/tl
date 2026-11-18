// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

use crate::basictl::{Error, Result, TLRead, take, take_array};

#[inline]
pub(crate) fn read_size(input: &mut &[u8]) -> Result<usize> {
    match input.read_u8()? {
        254 => Ok(254 + usize::from(u16::from_le_bytes(take_array(input)?))),
        255 => {
            let len = input.read_u64()?;
            usize::try_from(len).map_err(|_| Error::InvalidLength)
        }
        len => Ok(usize::from(len)),
    }
}

#[inline]
fn size_bytes(len: usize, bytes: &mut [u8; 9]) -> usize {
    match len {
        0..254 => {
            bytes[0] = len as u8;
            1
        }
        254..65790 => {
            bytes[0] = 254;
            bytes[1..3].copy_from_slice(&((len - 254) as u16).to_le_bytes());
            3
        }
        _ => {
            bytes[0] = 255;
            bytes[1..].copy_from_slice(&(len as u64).to_le_bytes());
            9
        }
    }
}

#[inline]
pub(crate) fn write_size(buf: &mut Vec<u8>, len: usize) {
    let mut bytes = [0; 9];
    let width = size_bytes(len, &mut bytes);
    buf.extend_from_slice(&bytes[..width]);
}

#[inline]
fn size_width(len: usize) -> usize {
    match len {
        0..254 => 1,
        254..65790 => 3,
        _ => 9,
    }
}

#[inline]
pub(crate) fn bytes_size(value: &[u8], omit_empty: bool) -> usize {
    if omit_empty && value.is_empty() {
        return 0;
    }
    size_width(value.len()) + value.len()
}

pub(crate) fn sequence_size(count: usize, payload_size: usize, omit_empty: bool) -> usize {
    if count == 0 {
        return usize::from(!omit_empty);
    }
    let len = size_width(count) + payload_size;
    size_width(len) + len
}

pub(crate) fn write_single_field(
    buf: &mut Vec<u8>,
    field_size: usize,
    omit_empty: bool,
    write: impl FnOnce(&mut Vec<u8>) -> Result<()>,
) -> Result<()> {
    if field_size == 0 {
        if !omit_empty {
            buf.push(0);
        }
        return Ok(());
    }
    let len = field_size + 1;
    buf.reserve(size_width(len) + len);
    write_size(buf, len);
    buf.push(2);
    let start = buf.len();
    write(buf)?;
    debug_assert_eq!(buf.len() - start, field_size);
    Ok(())
}

#[inline]
pub(crate) fn write_bytes(buf: &mut Vec<u8>, value: &[u8], omit_empty: bool) -> Result<()> {
    if !omit_empty || !value.is_empty() {
        buf.reserve(size_width(value.len()) + value.len());
        write_size(buf, value.len());
        buf.extend_from_slice(value);
    }
    Ok(())
}

pub(crate) fn write_sequence(
    buf: &mut Vec<u8>,
    count: usize,
    payload_size: Option<usize>,
    omit_empty: bool,
    write: impl FnOnce(&mut Vec<u8>) -> Result<()>,
) -> Result<()> {
    if count == 0 {
        if !omit_empty {
            buf.push(0);
        }
        return Ok(());
    }
    if let Some(payload_size) = payload_size {
        let len = size_width(count) + payload_size;
        buf.reserve(size_width(len) + len);
        write_size(buf, len);
        write_size(buf, count);
        return write(buf);
    }
    write_sized(buf, false, |buf| {
        write_size(buf, count);
        write(buf)
    })
}

#[inline]
pub(crate) fn read_sized<'a>(input: &mut &'a [u8]) -> Result<&'a [u8]> {
    let len = read_size(input)?;
    take(input, len)
}

pub(crate) fn write_sized(
    buf: &mut Vec<u8>,
    omit_empty: bool,
    write: impl FnOnce(&mut Vec<u8>) -> Result<()>,
) -> Result<()> {
    let start = buf.len();
    buf.push(0);
    write(buf)?;
    let end = buf.len();
    let len = end - start - 1;
    if len == 0 && omit_empty {
        buf.truncate(start);
        return Ok(());
    }
    let mut prefix = [0; 9];
    let width = size_bytes(len, &mut prefix);
    if width > 1 {
        buf.resize(end + width - 1, 0);
        buf.copy_within(start + 1..end, start + width);
    }
    buf[start..start + width].copy_from_slice(&prefix[..width]);
    Ok(())
}

pub(crate) struct ObjectWriter<'a> {
    buf: &'a mut Vec<u8>,
    mask_pos: usize,
    last_used: usize,
}

impl ObjectWriter<'_> {
    pub(crate) fn next_block(&mut self) {
        self.mask_pos = self.buf.len();
        self.buf.push(0);
    }

    pub(crate) fn flag(&mut self, mask: u8) {
        self.buf[self.mask_pos] |= mask;
        self.last_used = self.buf.len();
    }

    pub(crate) fn field(
        &mut self,
        mask: u8,
        present: bool,
        write: impl FnOnce(&mut Vec<u8>) -> Result<()>,
    ) -> Result<()> {
        let pos = self.buf.len();
        write(self.buf)?;
        if present || pos != self.buf.len() {
            self.flag(mask);
        }
        Ok(())
    }
}

pub(crate) fn write_object(
    buf: &mut Vec<u8>,
    index: usize,
    omit_empty: bool,
    write: impl FnOnce(&mut ObjectWriter<'_>) -> Result<()>,
) -> Result<()> {
    write_sized(buf, omit_empty, |buf| {
        let start = buf.len();
        buf.push(0);
        let mut object = ObjectWriter {
            buf,
            mask_pos: start,
            last_used: start,
        };
        if index != 0 {
            write_size(object.buf, index);
            object.flag(1);
        }
        write(&mut object)?;
        object.buf.truncate(object.last_used);
        Ok(())
    })
}

pub(crate) struct ObjectReader<'a> {
    pub(crate) data: &'a [u8],
    pub(crate) mask: u8,
}

impl ObjectReader<'_> {
    pub(crate) fn next_block(&mut self) {
        self.mask = self.data.read_u8().unwrap_or_default();
    }
}

#[inline]
pub(crate) fn read_object<'a>(buf: &mut &'a [u8]) -> Result<(usize, ObjectReader<'a>)> {
    let mut data = read_sized(buf)?;
    let mask = data.read_u8().unwrap_or_default();
    let index = if mask & 1 != 0 {
        read_size(&mut data)?
    } else {
        0
    };
    Ok((index, ObjectReader { data, mask }))
}

#[inline]
pub(crate) fn write_one_string_vector(buf: &mut Vec<u8>, text: &[u8]) {
    let len = text.len();
    if len < 250 {
        let len = len as u8;
        buf.reserve(usize::from(len) + 5);
        buf.extend_from_slice(&[len + 4, 2, len + 2, 1, len]);
        buf.extend_from_slice(text);
        return;
    }
    let vector_len = 1 + size_width(len) + len;
    let object_len = 1 + size_width(vector_len) + vector_len;
    buf.reserve(size_width(object_len) + object_len);
    write_size(buf, object_len);
    buf.push(2);
    write_size(buf, vector_len);
    buf.push(1);
    write_size(buf, len);
    buf.extend_from_slice(text);
}
