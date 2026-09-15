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

package sfnt

import (
	"testing"

	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
)

// makeNameTestFont returns a three-glyph font where the last glyph has no
// name, so that MakeGlyphNames tries to infer the missing name.
func makeNameTestFont() *Font {
	return &Font{
		UnitsPerEm: 1000,
		Outlines: &glyf.Outlines{
			Glyphs: glyf.Glyphs{{}, {}, {}},
			Names:  []string{".notdef", "A", ""},
		},
	}
}

// TestMakeGlyphNamesInvalidCMap tests that glyph IDs beyond the end of the
// font are ignored when names are inferred from the cmap table.
func TestMakeGlyphNamesInvalidCMap(t *testing.T) {
	f := makeNameTestFont()
	f.CMapTable = cmap.Table{
		{PlatformID: 3, EncodingID: 1}: cmap.Format4{'B': glyph.ID(5000)}.Encode(0),
	}

	names := f.MakeGlyphNames()
	if len(names) != 3 {
		t.Fatalf("got %d names, want 3", len(names))
	}
}

// TestMakeGlyphNamesInvalidGsub tests that glyph IDs beyond the end of the
// font are ignored when names are inferred from the GSUB table.
func TestMakeGlyphNamesInvalidGsub(t *testing.T) {
	subtables := []gtab.Subtable{
		&gtab.Gsub1_1{
			Cov:   coverage.Set{1: true, 5000: true},
			Delta: 5000,
		},
		&gtab.Gsub1_2{
			Cov:                coverage.Table{1: 0, 5000: 1},
			SubstituteGlyphIDs: []glyph.ID{5001, 2},
		},
		&gtab.Gsub3_1{
			Cov:        coverage.Table{1: 0, 5000: 1},
			Alternates: [][]glyph.ID{{5002}, {2}},
		},
		&gtab.Gsub4_1{
			Cov: coverage.Table{1: 0, 5000: 1},
			Repl: [][]gtab.Ligature{
				{{In: []glyph.ID{5003}, Out: 2}, {In: []glyph.ID{1}, Out: 5004}},
				{{In: []glyph.ID{1}, Out: 2}},
			},
		},
	}

	for _, subtable := range subtables {
		f := makeNameTestFont()
		f.Gsub = &gtab.Info{
			LookupList: gtab.LookupList{
				&gtab.LookupTable{Subtables: []gtab.Subtable{subtable}},
			},
		}

		names := f.MakeGlyphNames()
		if len(names) != 3 {
			t.Fatalf("%T: got %d names, want 3", subtable, len(names))
		}
	}
}
