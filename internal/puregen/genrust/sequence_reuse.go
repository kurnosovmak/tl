// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

// A single independent vector can be validated completely before modifying the
// receiver. This lets the public readers keep their transactional behavior and
// reuse the receiver's allocations.
type independentVector struct {
	field   Field
	element *TypeRWPrimitive
	tl1     bool
	tags    []uint32
}

func (trw *TypeRWStruct) independentVector() independentVector {
	if len(trw.Fields) != 1 || trw.isTypedef() || trw.isAlias() {
		return independentVector{}
	}
	field := trw.Fields[0]
	if field.hasPresence() || field.FieldMask() != nil || field.IsTL2Omitted() || len(field.NatArgs()) != 0 {
		return independentVector{}
	}
	path := independentVector{field: field, tl1: true}
	for {
		path.tl1 = path.tl1 && !field.t.OriginTL2()
		if !field.Bare() {
			path.tags = append(path.tags, field.t.TLTag())
		}
		alias, ok := field.t.trw.(*TypeRWStruct)
		if !ok || !alias.isUnwrapType() {
			break
		}
		field = alias.Fields[0]
		if len(field.NatArgs()) != 0 {
			return independentVector{}
		}
	}
	vector, ok := field.t.trw.(*TypeRWBrackets)
	if !ok || !vector.vectorLike || !vector.element.Bare() {
		return independentVector{}
	}
	element, ok := vector.element.t.trw.(*TypeRWPrimitive)
	if !ok || element.canonicalType != "string" && vector.element.t.fixedSequencePrimitive() == "" {
		return independentVector{}
	}
	path.element = element
	return path
}

func reuseVectorTL1(cc *codecreator.RustCodeCreator, wr *TypeRWWrapper) bool {
	trw, ok := wr.trw.(*TypeRWStruct)
	if !ok || wr.OriginTL2() {
		return false
	}
	path := trw.independentVector()
	if path.element == nil || !path.tl1 {
		return false
	}
	for _, tag := range path.tags {
		cc.AddLinef("input.read_exact_tag(0x%08x)?;", tag)
	}
	cc.AddLinef("let count = input.read_u32()? as usize;")
	readVectorInto(cc, path.field, path.element, "&mut input", "basictl::read_string_slice")
	return true
}

func reuseVectorTL2(cc *codecreator.RustCodeCreator, wr *TypeRWWrapper) bool {
	trw, ok := wr.trw.(*TypeRWStruct)
	if !ok {
		return false
	}
	path := trw.independentVector()
	field, element := path.field, path.element
	if element == nil {
		return false
	}
	cc.AddLinef("let (index, mut object) = crate::tl2::read_object(&mut input)?;")
	cc.If(fmt.Sprintf("index != %d", trw.pureTypeStruct.UnionIndex()), func() {
		cc.AddLinef("return Err(basictl::Error::UnexpectedConstructor(index));")
	})
	cc.IfElse("object.mask & 2 == 0", func() {
		cc.AddLinef("self.%s.clear();", field.goName)
	}, func() {
		cc.AddLinef("let mut data = crate::tl2::read_sized(&mut object.data)?;")
		cc.IfElse("data.is_empty()", func() {
			cc.AddLinef("self.%s.clear();", field.goName)
		}, func() {
			cc.AddLinef("let count = crate::tl2::read_size(&mut data)?;")
			if element.canonicalType == "string" {
				cc.If("count > data.len()", func() {
					cc.AddLinef("return Err(basictl::Error::InvalidLength);")
				})
			}
			readVectorInto(cc, field, element, "&mut data", "crate::tl2::read_sized")
		})
	})
	return true
}

func readVectorInto(cc *codecreator.RustCodeCreator, field Field, element *TypeRWPrimitive, input, stringReader string) {
	if element.canonicalType == "string" {
		cc.AddLinef("basictl::read_strings_into(%s, count, &mut self.%s, %s)?;", input, field.goName, stringReader)
		return
	}
	cc.AddLinef("basictl::read_fixed_sequence_into(%s, count, &mut self.%s, %s::from_le_bytes)?;", input, field.goName, element.goType)
}

func writeOneStringVector(cc *codecreator.RustCodeCreator, wr *TypeRWWrapper, tl1 bool) {
	trw, ok := wr.trw.(*TypeRWStruct)
	if !ok {
		return
	}
	path := trw.independentVector()
	if path.element == nil || path.element.canonicalType != "string" {
		return
	}
	if tl1 && (wr.OriginTL2() || !path.tl1) || !tl1 && trw.pureTypeStruct.UnionIndex() != 0 {
		return
	}
	cc.If(fmt.Sprintf("let [text] = self.%s.as_slice()", path.field.goName), func() {
		if tl1 {
			for _, tag := range path.tags {
				cc.AddLinef("basictl::write_tag(buf, 0x%08x);", tag)
			}
			cc.AddLinef("return basictl::write_one_string_vector(buf, text);")
		} else {
			cc.AddLinef("crate::tl2::write_one_string_vector(buf, text);")
			cc.AddLinef("return Ok(());")
		}
	})
}
