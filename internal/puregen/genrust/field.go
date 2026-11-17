// Copyright 2025 V Kontakte LLC
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"
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

func (f *Field) TL2MaskForOP(op string) string {
	return fmt.Sprintf("tl2mask%d %s %d", *f.MaskTL2Bit()/8, op, 1<<(*f.MaskTL2Bit()%8))
}
