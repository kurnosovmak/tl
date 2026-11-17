// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

use crate::basictl::{Error, Result};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use serde::de::{self, Deserialize, Deserializer, MapAccess, SeqAccess, Visitor};
pub(crate) use serde_json::{Map, Value};
use std::{fmt, str::FromStr, sync::LazyLock};

/// Selects the discriminator names used for TL unions.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum JsonMode {
    /// TL1 constructor names.
    #[default]
    Tl1,
    /// Legacy TL1 constructor names with their hexadecimal tags.
    Legacy,
    /// TL2 variant names.
    Tl2,
}

pub(crate) struct Input<T>(pub(crate) T);

pub(crate) struct Bytes(pub(crate) Vec<u8>);

impl<'de> Deserialize<'de> for Bytes {
    fn deserialize<D: Deserializer<'de>>(decoder: D) -> std::result::Result<Self, D::Error> {
        struct ByteVisitor;
        impl<'de> Visitor<'de> for ByteVisitor {
            type Value = Bytes;
            fn expecting(&self, fmt: &mut fmt::Formatter<'_>) -> fmt::Result {
                fmt.write_str("a string or a TL base64 object")
            }
            fn visit_str<E: de::Error>(self, text: &str) -> std::result::Result<Bytes, E> {
                Ok(Bytes(text.as_bytes().to_vec()))
            }
            fn visit_string<E: de::Error>(self, text: String) -> std::result::Result<Bytes, E> {
                Ok(Bytes(text.into_bytes()))
            }
            fn visit_map<A: MapAccess<'de>>(
                self,
                mut fields: A,
            ) -> std::result::Result<Bytes, A::Error> {
                use de::Error as _;
                if fields.next_key::<&str>()? != Some("base64") {
                    return Err(A::Error::custom("expected base64"));
                }
                let bytes = STANDARD
                    .decode(fields.next_value::<&str>()?)
                    .map_err(A::Error::custom)?;
                if fields.next_key::<&str>()?.is_some() {
                    return Err(A::Error::custom("unexpected base64 field"));
                }
                Ok(Bytes(bytes))
            }
        }
        decoder.deserialize_any(ByteVisitor)
    }
}

struct UniqueValue(Value);

impl<'de> Deserialize<'de> for UniqueValue {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
        deserializer.deserialize_any(UniqueVisitor).map(UniqueValue)
    }
}

struct UniqueVisitor;

impl<'de> Visitor<'de> for UniqueVisitor {
    type Value = Value;

    fn expecting(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str("a JSON value")
    }

    fn visit_unit<E: de::Error>(self) -> std::result::Result<Value, E> {
        Ok(Value::Null)
    }

    fn visit_bool<E: de::Error>(self, value: bool) -> std::result::Result<Value, E> {
        Ok(value.into())
    }

    fn visit_i64<E: de::Error>(self, value: i64) -> std::result::Result<Value, E> {
        Ok(value.into())
    }

    fn visit_u64<E: de::Error>(self, value: u64) -> std::result::Result<Value, E> {
        Ok(value.into())
    }

    fn visit_f64<E: de::Error>(self, value: f64) -> std::result::Result<Value, E> {
        Ok(value.into())
    }

    fn visit_str<E: de::Error>(self, value: &str) -> std::result::Result<Value, E> {
        Ok(value.into())
    }

    fn visit_seq<A: SeqAccess<'de>>(self, mut seq: A) -> std::result::Result<Value, A::Error> {
        let mut items = Vec::new();
        while let Some(UniqueValue(value)) = seq.next_element()? {
            items.push(value);
        }
        Ok(Value::Array(items))
    }

    fn visit_map<A: MapAccess<'de>>(self, mut map: A) -> std::result::Result<Value, A::Error> {
        let mut items = Map::new();
        while let Some((key, UniqueValue(value))) = map.next_entry::<String, UniqueValue>()? {
            match items.entry(key) {
                serde_json::map::Entry::Vacant(entry) => {
                    entry.insert(value);
                }
                serde_json::map::Entry::Occupied(entry) => {
                    return Err(de::Error::custom(format!(
                        "duplicate property {:?}",
                        entry.key()
                    )));
                }
            }
        }
        Ok(Value::Object(items))
    }
}

pub(crate) fn error(message: impl Into<String>) -> Error {
    Error::InvalidJson(message.into())
}

pub(crate) fn parse(input: &[u8]) -> Result<Value> {
    serde_json::from_slice::<UniqueValue>(input)
        .map(|value| value.0)
        .map_err(|err| error(err.to_string()))
}

pub(crate) fn serialize(value: &(impl serde::Serialize + ?Sized), buf: &mut Vec<u8>) -> Result<()> {
    serde_json::to_writer(buf, value).map_err(|err| error(err.to_string()))
}

#[inline]
pub(crate) fn field_name(buf: &mut Vec<u8>, start: usize, name: &[u8]) {
    if buf.len() != start {
        buf.push(b',');
    }
    buf.extend_from_slice(name);
}

#[inline]
pub(crate) fn write_array<T>(
    buf: &mut Vec<u8>,
    values: &[T],
    write: impl Fn(&mut Vec<u8>, &T) -> Result<()>,
) -> Result<()> {
    buf.push(b'[');
    if let Some((first, tail)) = values.split_first() {
        write(buf, first)?;
        for value in tail {
            buf.push(b',');
            write(buf, value)?;
        }
    }
    buf.push(b']');
    Ok(())
}

pub(crate) fn object(value: Option<&Value>) -> Result<&Map<String, Value>> {
    static EMPTY: LazyLock<Map<String, Value>> = LazyLock::new(Map::new);
    match value {
        None => Ok(&EMPTY),
        Some(Value::Object(fields)) => Ok(fields),
        _ => Err(error("expected an object")),
    }
}

pub(crate) fn array(value: Option<&Value>) -> Result<&[Value]> {
    match value {
        None => Ok(&[]),
        Some(Value::Array(items)) => Ok(items),
        _ => Err(error("expected an array")),
    }
}

pub(crate) fn check_fields(fields: &Map<String, Value>, allowed: &[&str]) -> Result<()> {
    if let Some(key) = fields.keys().find(|key| !allowed.contains(&key.as_str())) {
        return Err(error(format!("unexpected property {key:?}")));
    }
    Ok(())
}

pub(crate) fn read_bool(value: Option<&Value>) -> Result<bool> {
    match value {
        None => Ok(false),
        Some(Value::Bool(value)) => Ok(*value),
        _ => Err(error("expected a boolean")),
    }
}

pub(crate) trait JsonNumber: Default + FromStr {
    fn from_number(number: &serde_json::Number) -> Option<Self>;
}

macro_rules! integer {
    ($accessor:ident, $($ty:ty),+) => {
        $(
            impl JsonNumber for $ty {
                fn from_number(number: &serde_json::Number) -> Option<Self> {
                    number.$accessor().and_then(|value| Self::try_from(value).ok())
                }
            }
        )+
    };
}

integer!(as_i64, i32, i64);
integer!(as_u64, u8, u32, u64);

impl JsonNumber for f32 {
    fn from_number(number: &serde_json::Number) -> Option<Self> {
        let float: Self = number.to_string().parse().ok()?;
        float.is_finite().then_some(float)
    }
}

impl JsonNumber for f64 {
    fn from_number(number: &serde_json::Number) -> Option<Self> {
        number.as_f64()
    }
}

pub(crate) fn read_number<T: JsonNumber>(value: Option<&Value>) -> Result<T> {
    let invalid = || error("invalid number");
    match value {
        None => Ok(T::default()),
        Some(Value::Number(number)) => T::from_number(number).ok_or_else(invalid),
        Some(Value::String(number)) => number.parse().map_err(|_| invalid()),
        _ => Err(invalid()),
    }
}

pub(crate) fn write_float(value: impl fmt::Display, float: f64) -> Result<Value> {
    if !float.is_finite() {
        let text = if float.is_nan() {
            "NaN"
        } else if float.is_sign_positive() {
            "+Inf"
        } else {
            "-Inf"
        };
        return Ok(Value::String(text.into()));
    }
    value
        .to_string()
        .parse()
        .map_err(|_| error("invalid float"))
}

pub(crate) fn read_bytes(value: Option<&Value>) -> Result<Vec<u8>> {
    match value {
        None => Ok(Vec::new()),
        Some(Value::String(text)) => Ok(text.as_bytes().to_vec()),
        Some(Value::Object(fields)) => {
            check_fields(fields, &["base64"])?;
            let text = fields
                .get("base64")
                .and_then(Value::as_str)
                .ok_or_else(|| error("expected base64 string"))?;
            STANDARD.decode(text).map_err(|err| error(err.to_string()))
        }
        _ => Err(error("expected a string or base64 object")),
    }
}

pub(crate) fn write_bytes(bytes: &[u8]) -> Value {
    match std::str::from_utf8(bytes) {
        Ok(text) => Value::String(text.into()),
        Err(_) => Value::Object(Map::from_iter([(
            "base64".into(),
            STANDARD.encode(bytes).into(),
        )])),
    }
}

pub(crate) fn read_variant(value: Option<&Value>) -> Result<(&str, Option<&Value>)> {
    if let Some(Value::String(tag)) = value {
        return Ok((tag, None));
    }
    let fields = object(value)?;
    check_fields(fields, &["type", "value"])?;
    let tag = fields
        .get("type")
        .and_then(Value::as_str)
        .ok_or_else(|| error("expected a union type name"))?;
    Ok((tag, fields.get("value")))
}

pub(crate) fn write_variant(name: &str, payload: Option<Value>) -> Value {
    let mut fields = Map::new();
    fields.insert("type".into(), name.into());
    if let Some(payload) = payload {
        fields.insert("value".into(), payload);
    }
    Value::Object(fields)
}

pub(crate) fn read_maybe(value: Option<&Value>) -> Result<(bool, Option<&Value>)> {
    let fields = object(value)?;
    check_fields(fields, &["ok", "value"])?;
    let payload = fields.get("value");
    let present = match fields.get("ok") {
        Some(flag) => read_bool(Some(flag))?,
        None => payload.is_some(),
    };
    if !present && payload.is_some() {
        return Err(error("Maybe contains a value with ok=false"));
    }
    Ok((present, payload))
}

pub(crate) fn write_maybe(present: bool, payload: Option<Value>) -> Value {
    let mut fields = Map::new();
    if present {
        fields.insert("ok".into(), true.into());
        if let Some(payload) = payload {
            fields.insert("value".into(), payload);
        }
    }
    Value::Object(fields)
}

pub(crate) fn key(value: Value) -> Result<String> {
    match value {
        Value::String(key) => Ok(key),
        Value::Number(number) => Ok(number.to_string()),
        _ => Err(error("dictionary key must be a UTF-8 string or integer")),
    }
}
