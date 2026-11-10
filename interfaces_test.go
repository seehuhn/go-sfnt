// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyf"
)

// Compile-time checks that all outline formats satisfy the Outlines
// interface.
var (
	_ Outlines = (*cff.Outlines)(nil)
	_ Outlines = (*cff.OutlinesCFF2)(nil)
	_ Outlines = (*glyf.Outlines)(nil)
)
