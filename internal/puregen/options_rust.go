// Copyright 2025 V Kontakte LLC
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package puregen

import "flag"

type OptionsRust struct {
	CrateName           string
	RetainVectorBuffers bool
}

func (opt *OptionsRust) Bind(f *flag.FlagSet) {
	f.BoolVar(&opt.RetainVectorBuffers, "retain-vector-buffers", false, "Rust: retain initialized elements of compound vectors across binary reads")
	f.StringVar(&opt.CrateName, "crate-name", "gentl",
		"not validate, must be compatible with rust build system")
}
