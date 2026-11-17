// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"
	"strings"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (gen *genRust) generateRPC() string {
	var functions []*TypeRWStruct
	for _, wr := range gen.generatedTypesList {
		if function, ok := wr.trw.(*TypeRWStruct); ok && function.ResultType != nil {
			functions = append(functions, function)
		}
	}
	if len(functions) == 0 {
		return ""
	}

	cc := codecreator.NewRustCodeCreator()
	cc.AddLinef("use crate::basictl::{self, Error, Result, TLRead as _};")
	cc.AddLinef(jsonFeature)
	cc.AddLinef("use crate::json::{self, JsonMode};")
	cc.AddEmptyLine()
	cc.AddLinef("#[derive(Debug, Clone, Copy, PartialEq, Eq)]")
	cc.AddLinef("pub enum Format { Tl1, Tl2 }")
	cc.AddEmptyLine()
	cc.AddLinef("#[derive(Debug)]")
	cc.AddLinef("pub enum Request {")
	cc.FinishBlock(func() {
		for _, fn := range functions {
			cc.AddLinef("%s(%s),", fn.wr.goGlobalName, fn.wr.rustType())
		}
	}, "}")
	cc.AddEmptyLine()
	cc.AddLinef("#[derive(Debug)]")
	cc.AddLinef("pub enum Response {")
	cc.FinishBlock(func() {
		for _, fn := range functions {
			cc.AddLinef("%s(Box<%s>),", fn.wr.goGlobalName, fn.ResultType.rustType())
		}
	}, "}")
	cc.AddEmptyLine()
	cc.AddLinef("impl Request {")
	cc.FinishBlock(func() {
		generateRPCBinary(cc, functions)
		generateRPCJSON(cc, functions)
	}, "}")
	return cc.Text()
}

func generateRPCBinary(cc *codecreator.RustCodeCreator, functions []*TypeRWStruct) {
	cc.AddLinef("pub fn read_tl1(buf: &mut &[u8]) -> Result<Self> { Self::read(buf, Format::Tl1) }")
	cc.AddLinef("pub fn write_tl1(&self, buf: &mut Vec<u8>) -> Result<()> { self.write(buf, Format::Tl1) }")
	cc.AddLinef("pub fn read(buf: &mut &[u8], format: Format) -> Result<Self> {")
	cc.FinishBlock(func() {
		cc.AddLinef("let mut input = *buf;")
		cc.AddLinef("let tag = input.read_u32()?;")
		cc.AddLinef("let value = match (format, tag) {")
		cc.FinishBlock(func() {
			for _, fn := range functions {
				for _, format := range []string{"Tl1", "Tl2"} {
					if format == "Tl1" && fn.wr.OriginTL2() || format == "Tl2" && !fn.wr.HasTL2() {
						continue
					}
					cc.AddLinef("(Format::%s, 0x%08x) => {", format, fn.wr.TLTag())
					cc.FinishBlock(func() {
						cc.AddLinef("let mut req: %s = Default::default();", fn.wr.rustType())
						cc.AddLinef("req.read_%s(&mut input)?;", strings.ToLower(format))
						cc.AddLinef("Self::%s(req)", fn.wr.goGlobalName)
					}, "}")
				}
			}
			cc.AddLinef("_ => return Err(Error::UnexpectedMagic(tag)),")
		}, "};")
		cc.AddLinef("*buf = input;")
		cc.AddLinef("Ok(value)")
	}, "}")
	cc.AddLinef("pub fn write(&self, buf: &mut Vec<u8>, format: Format) -> Result<()> {")
	cc.FinishBlock(func() {
		cc.AddLinef("basictl::append(buf, |buf| match self {")
		cc.FinishBlock(func() {
			for _, fn := range functions {
				cc.AddLinef("Self::%s(req) => match format {", fn.wr.goGlobalName)
				cc.FinishBlock(func() {
					if fn.wr.OriginTL2() {
						cc.AddLinef("Format::Tl1 => Err(Error::NoTL1(%q)),", fn.wr.TLName().String())
					} else {
						cc.AddLinef("Format::Tl1 => req.write_tl1_boxed(buf),")
					}
					if fn.wr.HasTL2() {
						cc.AddLinef("Format::Tl2 => { basictl::write_tag(buf, 0x%08x); req.write_tl2(buf) }", fn.wr.TLTag())
					} else {
						cc.AddLinef("Format::Tl2 => Err(Error::NoTL2(%q)),", fn.wr.TLName().String())
					}
				}, "},")
			}
		}, "})")
	}, "}")
	cc.AddLinef("pub fn read_response_tl1(&self, buf: &mut &[u8]) -> Result<Response> { self.read_response(buf, Format::Tl1) }")
	cc.AddLinef("pub fn write_response_tl1(&self, value: &Response, buf: &mut Vec<u8>) -> Result<()> { self.write_response(value, buf, Format::Tl1) }")
	cc.AddLinef("pub fn read_response(&self, buf: &mut &[u8], format: Format) -> Result<Response> {")
	cc.FinishBlock(func() {
		cc.AddLinef("match self {")
		cc.FinishBlock(func() {
			for _, fn := range functions {
				name := fn.wr.goGlobalName
				cc.AddLinef("Self::%s(req) => {", name)
				cc.FinishBlock(func() {
					cc.AddLinef("let result = match format { Format::Tl1 => req.read_result_tl1(buf)?, Format::Tl2 => req.read_result_tl2(buf)? };")
					cc.AddLinef("Ok(Response::%s(Box::new(result)))", name)
				}, "}")
			}
		}, "}")
	}, "}")
	cc.AddLinef("pub fn write_response(&self, value: &Response, buf: &mut Vec<u8>, format: Format) -> Result<()> {")
	cc.FinishBlock(func() {
		cc.AddLinef("match (self, value) {")
		cc.FinishBlock(func() {
			for _, fn := range functions {
				name := fn.wr.goGlobalName
				cc.AddLinef("(Self::%s(req), Response::%s(result)) => match format {", name, name)
				cc.FinishBlock(func() {
					cc.AddLinef("Format::Tl1 => req.write_result_tl1(result, buf),")
					cc.AddLinef("Format::Tl2 => req.write_result_tl2(result, buf),")
				}, "},")
			}
			cc.AddLinef("_ => Err(Error::WrongRpcResponse),")
		}, "}")
	}, "}")
}

func generateRPCJSON(cc *codecreator.RustCodeCreator, functions []*TypeRWStruct) {
	cc.AddLinef(jsonFeature)
	cc.AddLinef("pub fn read_json(buf: &[u8], mode: JsonMode) -> Result<Self> {")
	cc.FinishBlock(func() {
		cc.AddLinef("Self::decode_json(Some(&json::parse(buf)?), mode)")
	}, "}")
	cc.AddLinef(jsonFeature)
	cc.AddLinef("pub fn write_json(&self, buf: &mut Vec<u8>, mode: JsonMode) -> Result<()> {")
	cc.FinishBlock(func() {
		cc.AddLinef("let value = self.encode_json(mode)?;")
		cc.AddLinef("basictl::append(buf, |buf| json::serialize(&value, buf))")
	}, "}")
	cc.AddLinef(jsonFeature)
	cc.AddLinef("pub(crate) fn decode_json(input: Option<&json::Value>, mode: JsonMode) -> Result<Self> {")
	cc.FinishBlock(func() {
		cc.AddLinef("let (name, args) = json::read_variant(input)?;")
		cc.AddLinef("match name {")
		cc.FinishBlock(func() {
			for _, fn := range functions {
				name := fn.wr.TLName().String()
				cc.AddLinef("%q => %s.map(Self::%s),", name, fn.wr.decodeJSON("args", nil), fn.wr.goGlobalName)
				if !fn.wr.OriginTL2() {
					cc.AddLinef("%q if mode == JsonMode::Legacy => %s.map(Self::%s),",
						fmt.Sprintf("%s#%08x", name, fn.wr.TLTag()), fn.wr.decodeJSON("args", nil), fn.wr.goGlobalName)
				}
			}
			cc.AddLinef("_ => Err(json::error(format!(\"unknown RPC function {name:?}\"))),")
		}, "}")
	}, "}")
	cc.AddLinef(jsonFeature)
	cc.AddLinef("pub(crate) fn encode_json(&self, mode: JsonMode) -> Result<json::Value> {")
	cc.FinishBlock(func() {
		cc.AddLinef("match self {")
		cc.FinishBlock(func() {
			for _, fn := range functions {
				name := fn.wr.TLName().String()
				tag := fmt.Sprintf("%q", name)
				if !fn.wr.OriginTL2() {
					tag = fmt.Sprintf("if mode == JsonMode::Legacy { %q } else { %q }",
						fmt.Sprintf("%s#%08x", name, fn.wr.TLTag()), name)
				}
				cc.AddLinef("Self::%s(req) => Ok(json::write_variant(%s, Some(%s?))),",
					fn.wr.goGlobalName, tag, fn.wr.encodeJSON("req", nil))
			}
		}, "}")
	}, "}")
	cc.AddLinef(jsonFeature)
	cc.AddLinef("pub(crate) fn decode_response_json(&self, input: Option<&json::Value>, mode: JsonMode) -> Result<Response> {")
	cc.FinishBlock(func() {
		cc.AddLinef("match self {")
		cc.FinishBlock(func() {
			for _, fn := range functions {
				name := fn.wr.goGlobalName
				cc.AddLinef("Self::%s(req) => Ok(Response::%s(Box::new(req.decode_result_json(input, mode)?))),", name, name)
			}
		}, "}")
	}, "}")
	cc.AddLinef(jsonFeature)
	cc.AddLinef("pub(crate) fn encode_response_json(&self, response: &Response, mode: JsonMode) -> Result<json::Value> {")
	cc.FinishBlock(func() {
		cc.AddLinef("match (self, response) {")
		cc.FinishBlock(func() {
			for _, fn := range functions {
				name := fn.wr.goGlobalName
				cc.AddLinef("(Self::%s(req), Response::%s(result)) => req.encode_result_json(result, mode),", name, name)
			}
			cc.AddLinef("_ => Err(Error::WrongRpcResponse),")
		}, "}")
	}, "}")
}
