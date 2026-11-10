// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt_test

import (
	"errors"
	"testing"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/debug"
)

// TestSubsetVariableFontGuard checks that Subset refuses a variable font and
// succeeds once the font has been pinned to a single instance.
func TestSubsetVariableFontGuard(t *testing.T) {
	f := debug.MakeVarFont()
	numGlyphs := f.Outlines.(*glyf.Outlines).NumGlyphs()
	glyphs := make([]glyph.ID, numGlyphs)
	for i := range glyphs {
		glyphs[i] = glyph.ID(i)
	}

	if _, err := f.Subset(glyphs); !errors.Is(err, sfnt.ErrVariableFont) {
		t.Errorf("Subset on a variable font: err = %v, want ErrVariableFont", err)
	}

	inst, err := f.Instantiate(nil)
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	if _, err := inst.Subset(glyphs); err != nil {
		t.Errorf("Subset on the default instance: %v", err)
	}
}
