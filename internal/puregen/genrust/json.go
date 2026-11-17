// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

const jsonFeature = `#[cfg(feature = "tl-json")]`

func (wr *TypeRWWrapper) decodeJSON(input string, args []string) string {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		switch typ.canonicalType {
		case "string":
			return "json::read_bytes(" + input + ")"
		case "__function":
			return fmt.Sprintf("crate::rpc::Request::decode_json(%s, mode).map(|value| Some(Box::new(value)))", input)
		case "__function_result":
			return fmt.Sprintf("request.decode_response_json(%s, mode).map(|value| Some(Box::new(value)))", input)
		default:
			return fmt.Sprintf("json::read_number::<%s>(%s)", typ.goType, input)
		}
	case *TypeRWBool:
		return "json::read_bool(" + input + ")"
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return typ.Fields[0].t.decodeJSON(input, typ.pureTypeStruct.ReplaceUnwrapArgs(0, args))
		}
	}
	return fmt.Sprintf("%s::decode_json(%s, mode%s%s)", wr.rustModule(), input, joinWithCommas(args), wr.fetcherCall())
}

func (wr *TypeRWWrapper) encodeJSON(value string, args []string) string {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		switch typ.canonicalType {
		case "string":
			return fmt.Sprintf("Ok::<json::Value, basictl::Error>(json::write_bytes(%s))", value)
		case "__function":
			return fmt.Sprintf("(%s).as_deref().ok_or(basictl::Error::MissingRpcPayload)?.encode_json(mode)", value)
		case "__function_result":
			return fmt.Sprintf("request.encode_response_json((%s).as_deref().ok_or(basictl::Error::MissingRpcPayload)?, mode)", value)
		case "float32", "float64":
			return fmt.Sprintf("json::write_float(%[1]s, f64::from(*(%[1]s)))", value)
		default:
			return fmt.Sprintf("Ok::<json::Value, basictl::Error>(json::Value::from(*(%s)))", value)
		}
	case *TypeRWBool:
		return fmt.Sprintf("Ok::<json::Value, basictl::Error>(json::Value::from(*(%s)))", value)
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return typ.Fields[0].t.encodeJSON(value, typ.pureTypeStruct.ReplaceUnwrapArgs(0, args))
		}
	}
	return fmt.Sprintf("%s::encode_json(%s, mode%s%s)", wr.rustModule(), value, joinWithCommas(args), wr.fetcherCall())
}

func (wr *TypeRWWrapper) jsonNonEmpty(value string) string {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		switch typ.canonicalType {
		case "string":
			return fmt.Sprintf("!(%s).is_empty()", value)
		case "__function", "__function_result":
			return fmt.Sprintf("(%s).is_some()", value)
		default:
			return fmt.Sprintf("*(%s) != %s::default()", value, typ.goType)
		}
	case *TypeRWBool:
		return fmt.Sprintf("*(%s)", value)
	case *TypeRWBrackets:
		if typ.vectorLike || typ.dynamicSize {
			return fmt.Sprintf("!(%s).is_empty()", value)
		}
	case *TypeRWDict:
		return fmt.Sprintf("!(%s).is_empty()", value)
	case *TypeRWMaybe:
		return fmt.Sprintf("(%s).is_some()", value)
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return typ.Fields[0].t.jsonNonEmpty(value)
		}
		if typ.isTypedef() {
			return typ.Fields[0].t.jsonNonEmpty("&(" + value + ").0")
		}
	}
	return "true"
}

func (wr *TypeRWWrapper) generateJSON() string {
	cc := codecreator.NewRustCodeCreator()
	switch wr.trw.(type) {
	case *TypeRWPrimitive, *TypeRWBool:
		return ""
	default:
		cc.AddEmptyLine()
		cc.AddLinef(jsonFeature)
		cc.AddLinef("use crate::json::{self, JsonMode};")
	}
	switch typ := wr.trw.(type) {
	case *TypeRWStruct:
		typ.generateJSON(cc)
	case *TypeRWUnion:
		typ.generateJSON(cc)
	case *TypeRWBrackets:
		typ.generateJSON(cc)
	case *TypeRWDict:
		typ.generateJSON(cc)
	case *TypeRWMaybe:
		typ.generateJSON(cc)
	}
	return cc.Text()
}

func (wr *TypeRWWrapper) generateJSONMethods(cc *codecreator.RustCodeCreator) {
	natDecl := wr.formatNatArgsDecl() + wr.fetcherDecl()
	natCall := wr.formatNatArgsDeclCall() + wr.fetcherCall()
	vector, vectorFastPath := wr.jsonVector()
	cc.AddEmptyLine()
	cc.AddLinef(jsonFeature)
	cc.AddLinef("impl %s {", wr.goGlobalName)
	cc.FinishBlock(func() {
		cc.AddLinef("pub fn read_json(&mut self, input: &[u8], mode: JsonMode%s) -> basictl::Result<()> {", natDecl)
		cc.FinishBlock(func() {
			if vectorFastPath {
				vector.generateRead(cc)
			}
			cc.AddLinef("let data = json::parse(input)?;")
			cc.AddLinef("*self = decode_json(Some(&data), mode%s)?;", natCall)
			cc.AddLinef("Ok(())")
		}, "}")
		cc.AddLinef("pub fn write_json(&self, buf: &mut Vec<u8>, mode: JsonMode%s) -> basictl::Result<()> {", natDecl)
		cc.FinishBlock(func() {
			if vectorFastPath {
				vector.generateWrite(cc)
				return
			}
			cc.AddLinef("let data = encode_json(self, mode%s)?;", natCall)
			cc.AddLinef("basictl::append(buf, |buf| json::serialize(&data, buf))")
		}, "}")
	}, "}")
}

func (wr *TypeRWWrapper) generateJSONFunctions(cc *codecreator.RustCodeCreator, typ string, read, write func()) {
	natDecl := wr.formatNatArgsDecl() + wr.fetcherDecl()
	cc.AddEmptyLine()
	cc.AddLinef(jsonFeature)
	cc.AddLinef("pub(crate) fn decode_json(input: Option<&json::Value>, mode: JsonMode%s) -> basictl::Result<%s> {", natDecl, typ)
	cc.FinishBlock(read, "}")
	cc.AddEmptyLine()
	cc.AddLinef(jsonFeature)
	cc.AddLinef("pub(crate) fn encode_json(value: &%s, mode: JsonMode%s) -> basictl::Result<json::Value> {", typ, natDecl)
	cc.FinishBlock(write, "}")
}
