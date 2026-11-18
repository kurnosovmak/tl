// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"strings"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (trw *TypeRWStruct) rpcSetup(cc *codecreator.RustCodeCreator) {
	cc.AddLinef("let value = self;")
	for _, field := range trw.Fields {
		if field.t.pureType.CanonicalName() == "__function" {
			cc.AddLinef("let request = self.%s.as_deref().ok_or(basictl::Error::MissingRpcPayload)?;", field.goName)
		}
	}
}

func (trw *TypeRWStruct) generateRPC() string {
	cc := codecreator.NewRustCodeCreator()
	wr, result := trw.wr, trw.ResultType
	name, resultType := wr.goGlobalName, result.rustType()
	args := wr.formatNatArgs(trw.Fields, trw.ResultNatArgs)
	bare := trw.pureTypeStruct.ResultTypeBare()
	cc.AddEmptyLine()
	cc.AddLinef("impl %s {", name)
	cc.FinishBlock(func() {
		cc.AddLinef("pub const TL_NAME: &'static str = %q;", wr.TLName().String())
		cc.AddLinef("pub const TL_TAG: u32 = 0x%08x;", wr.TLTag())
		cc.AddLinef("pub fn read_result_tl1(&self, buf: &mut &[u8]) -> basictl::Result<%s> {", resultType)
		cc.FinishBlock(func() {
			if wr.OriginTL2() {
				noTL1(cc, wr)
				return
			}
			trw.rpcSetup(cc)
			cc.AddLinef("let mut input = *buf;")
			cc.AddLinef("let mut result: %s = %s;", resultType, rustDefault(result))
			cc.AddLinef("{")
			cc.FinishBlock(func() {
				cc.AddLinef("let buf = &mut input;")
				result.TypeReadingCode(cc, false, &DirectImports{}, "result", bare, args, false)
			}, "}")
			cc.AddLinef("*buf = input;")
			cc.AddLinef("Ok(result)")
		}, "}")
		cc.AddLinef("pub fn write_result_tl1(&self, result: &%s, buf: &mut Vec<u8>) -> basictl::Result<()> {", resultType)
		cc.FinishBlock(func() {
			if wr.OriginTL2() {
				noTL1(cc, wr)
				return
			}
			trw.rpcSetup(cc)
			cc.AddLinef("basictl::append(buf, |buf| {")
			cc.FinishBlock(func() {
				cc.AddLines(strings.Split(result.TypeWritingCode("result", bare, args, true), "\n")...)
				cc.AddLinef("Ok(())")
			}, "})")
		}, "}")

		cc.AddLinef("pub fn read_result_tl2(&self, buf: &mut &[u8]) -> basictl::Result<%s> {", resultType)
		cc.FinishBlock(func() {
			if !wr.HasTL2() {
				cc.AddLinef("return Err(basictl::Error::NoTL2(Self::TL_NAME));")
				return
			}
			cc.AddLinef("let mut input = *buf;")
			if trw.pureTypeStruct.IsResultAlias() {
				cc.AddLinef("let result = %s?;", result.readTL2("&mut input"))
			} else {
				cc.AddLinef("let (index, mut object) = crate::tl2::read_object(&mut input)?;")
				cc.If("index != 0", func() { cc.AddLinef("return Err(basictl::Error::UnexpectedConstructor(index));") })
				cc.AddLinef("let result = if object.mask & 2 != 0 { %s? } else { %s };",
					result.readTL2("&mut object.data"), rustDefault(result))
			}
			cc.AddLinef("*buf = input;")
			cc.AddLinef("Ok(result)")
		}, "}")
		cc.AddLinef("pub fn write_result_tl2(&self, result: &%s, buf: &mut Vec<u8>) -> basictl::Result<()> {", resultType)
		cc.FinishBlock(func() {
			if !wr.HasTL2() {
				cc.AddLinef("return Err(basictl::Error::NoTL2(Self::TL_NAME));")
				return
			}
			cc.AddLinef("basictl::append(buf, |buf| {")
			cc.FinishBlock(func() {
				if trw.pureTypeStruct.IsResultAlias() {
					cc.AddLinef("%s", result.writeTL2("buf", "result", "false"))
					return
				}
				cc.AddLinef("crate::tl2::write_object(buf, 0, false, |object| {")
				cc.FinishBlock(func() {
					cc.AddLinef("object.field(2, false, |buf| %s)", result.writeTL2("buf", "result", "true"))
				}, "})")
			}, "})")
		}, "}")
		cc.AddLinef(jsonFeature)
		cc.AddLinef("pub fn read_result_json(&self, input: &[u8], mode: JsonMode) -> basictl::Result<%s> {", resultType)
		cc.FinishBlock(func() {
			cc.AddLinef("let data = json::parse(input)?;")
			cc.AddLinef("self.decode_result_json(Some(&data), mode)")
		}, "}")
		cc.AddLinef(jsonFeature)
		cc.AddLinef("pub fn write_result_json(&self, result: &%s, buf: &mut Vec<u8>, mode: JsonMode) -> basictl::Result<()> {", resultType)
		cc.FinishBlock(func() {
			cc.AddLinef("let data = self.encode_result_json(result, mode)?;")
			cc.AddLinef("basictl::append(buf, |buf| json::serialize(&data, buf))")
		}, "}")
		cc.AddLinef(jsonFeature)
		cc.AddLinef("pub(crate) fn decode_result_json(&self, input: Option<&json::Value>, mode: JsonMode) -> basictl::Result<%s> {", resultType)
		cc.FinishBlock(func() {
			trw.rpcSetup(cc)
			cc.AddLinef("%s", result.decodeJSON("input", args))
		}, "}")
		cc.AddLinef(jsonFeature)
		cc.AddLinef("pub(crate) fn encode_result_json(&self, result: &%s, mode: JsonMode) -> basictl::Result<json::Value> {", resultType)
		cc.FinishBlock(func() {
			trw.rpcSetup(cc)
			cc.AddLinef("%s", result.encodeJSON("result", args))
		}, "}")
	}, "}")
	return cc.Text()
}
