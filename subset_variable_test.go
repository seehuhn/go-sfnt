// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

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
