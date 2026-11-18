// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import "strings"

// Direct readers cover acyclic records with local masks. Other schemas use
// the general readers, which also support recursive types and nat arguments.
type directReadFormat uint8

const (
	binaryRead directReadFormat = iota
	jsonRead
)

func (wr *TypeRWWrapper) supportsDirectRead(format directReadFormat, seen map[*TypeRWWrapper]bool) bool {
	if result, ok := seen[wr]; ok {
		return result
	}
	seen[wr] = false
	if wr.OriginTL2() || len(wr.NatParams()) != 0 || wr.HasFetcher() {
		return false
	}
	fieldOK := func(f Field) bool {
		return !f.IsBit() && !f.IsTL2Omitted() && !f.recursive && len(f.NatArgs()) == 0 && f.t.supportsDirectRead(format, seen)
	}
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		if typ.canonicalType != "string" && (wr.fixedSequencePrimitive() == "" || format == jsonRead && typ.canonicalType == "float32") {
			return false
		}
	case *TypeRWBrackets:
		if !typ.vectorLike || !fieldOK(typ.element) {
			return false
		}
	case *TypeRWStruct:
		if format == jsonRead && len(typ.Fields) > 64 {
			return false
		}
		if !typ.isUnwrapType() && (typ.isAlias() || typ.isTypedef()) {
			return false
		}
		for _, f := range typ.Fields {
			if !fieldOK(f) {
				return false
			}
			if mask := f.FieldMask(); mask != nil {
				if !mask.IsField() {
					return false
				}
				m := typ.Fields[mask.FieldIndex()]
				primitive, ok := m.t.trw.(*TypeRWPrimitive)
				if !ok || primitive.goType != "u32" || m.hasPresence() || m.FieldMask() != nil {
					return false
				}
			}
		}
	case *TypeRWUnion:
		if format == jsonRead && typ.IsEnum {
			return false
		}
		for _, f := range typ.Fields {
			if f.recursive || format == jsonRead && strings.Contains(f.t.TLName().String(), "__") || !f.t.supportsDirectRead(format, seen) {
				return false
			}
			st, ok := f.t.trw.(*TypeRWStruct)
			if !ok || st.isUnwrapType() || st.isAlias() || st.isTypedef() {
				return false
			}
		}
	default:
		return false
	}
	seen[wr] = true
	return true
}
