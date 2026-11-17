// Copyright 2025 V Kontakte LLC
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"strings"

	"github.com/VKCOM/tl/internal/pure"
)

type Field struct {
	pureField pure.Field
	t         *TypeRWWrapper
	goName    string
	recursive bool
}

func (f *Field) OriginalName() string {
	return f.pureField.Name()
}

func (f *Field) Bare() bool {
	return f.pureField.Bare()
}

func (f *Field) FieldMask() *pure.ActualNatArg {
	return f.pureField.FieldMask()
}

func (f *Field) BitNumber() uint32 {
	return f.pureField.BitNumber()
}

func (f *Field) MaskTL2Bit() *int {
	return f.pureField.MaskTL2Bit()
}

func (f *Field) NatArgs() []pure.ActualNatArg {
	return f.pureField.NatArgs()
}

func (f *Field) IsAffectedByLocalFieldMask() bool {
	return f.FieldMask() != nil && f.FieldMask().IsField()
}

func (f *Field) IsAffectedByExternalFieldMask() bool {
	return f.FieldMask() != nil && !f.FieldMask().IsField()
}

func (f *Field) IsTypeDependsFromLocalFields() bool {
	for _, natArg := range f.NatArgs() {
		if natArg.IsField() {
			return true
		}
	}
	return false
}

func (f *Field) HasNatArguments() bool {
	return len(f.NatArgs()) != 0
}

// do not generate fields, but affect block position and skip during reading
// TL1: never
// TL2: _:X
func (f *Field) IsTL2Omitted() bool {
	return strings.HasPrefix(f.OriginalName(), "_")
}

func (f *Field) IsBit() bool {
	return f.pureField.IsBit()
}

// The TL2 presence flag is independent of the legacy TL1 bitmask.
func (f Field) hasPresence() bool {
	return !f.IsBit() && (f.recursive || f.MaskTL2Bit() != nil)
}

func (field Field) wrapDecoded(expr string) string {
	if field.recursive {
		expr = "Box::new(" + expr + ")"
	}
	if field.hasPresence() {
		expr = "Some(" + expr + ")"
	}
	return expr
}

func (field Field) readDefault() string {
	if field.hasPresence() {
		return "None"
	}
	if field.IsBit() {
		return "false"
	}
	return rustDefault(field.t)
}

func (field Field) readSlot(access string) string {
	if !field.hasPresence() {
		return "&mut " + access
	}
	initial := rustDefault(field.t)
	if field.recursive {
		return access + ".get_or_insert_with(|| Box::new(" + initial + ")).as_mut()"
	}
	return access + ".get_or_insert_with(|| " + initial + ")"
}
