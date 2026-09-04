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
	"math"
	"testing"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
)

// TestGlyphHeight checks that glyphHeight reports the height of a glyph's ink
// in font design units.  Outlines are not necessarily drawn on the design
// grid, so each font below places its ink at a known fraction of the em using
// a different coordinate system, and the expected result is that fraction of
// UnitsPerEm.
func TestGlyphHeight(t *testing.T) {
	// a CID-keyed CFF font gives each glyph its own matrix
	cidKeyed := cidKeyedFont()

	glyfGlyph := &glyf.Glyph{}
	glyfGlyph.URy = 1400

	tests := []struct {
		name string
		font *Font
		gid  glyph.ID
		want funit.Int16
	}{
		{
			// ink height 1400 on a 2048-unit charstring grid is 0.68359 em,
			// or 683.59 units of the 1000-unit design grid
			name: "cff",
			font: &Font{
				UnitsPerEm: 1000,
				FontMatrix: emGrid(2048),
				Outlines: &cff.Outlines{
					Glyphs:   []*cff.Glyph{cffBox(1400)},
					Private:  []*type1.PrivateDict{{}},
					FDSelect: func(glyph.ID) int { return 0 },
				},
			},
			want: 684,
		},
		{
			// ink height 1024 on a 2048-unit charstring grid is half the em
			name: "cff2",
			font: &Font{
				UnitsPerEm: 1000,
				FontMatrix: emGrid(2048),
				Outlines: &cff.OutlinesCFF2{
					Glyphs:   []*cff.GlyphCFF2{cff2Box(1024)},
					Private:  []*cff.PrivateCFF2{{}},
					FDSelect: func(glyph.ID) int { return 0 },
				},
			},
			want: 500,
		},
		{
			// glyf outlines are always drawn on the design grid
			name: "glyf",
			font: &Font{
				UnitsPerEm: 2048,
				FontMatrix: emGrid(2048),
				Outlines:   &glyf.Outlines{Glyphs: glyf.Glyphs{glyfGlyph}},
			},
			want: 1400,
		},
		{
			name: "cff-cid-full-em",
			font: cidKeyed,
			gid:  0,
			want: 1000,
		},
		{
			name: "cff-cid-half-em",
			font: cidKeyed,
			gid:  1,
			want: 500,
		},
		{
			// a matrix read from a malformed font can scale the ink far
			// beyond the design grid
			name: "huge-matrix",
			font: cffFontWithMatrix(matrix.Matrix{1e6, 0, 0, 1e6, 0, 0}),
			want: math.MaxInt16,
		},
		{
			// with the y axis flipped the ink hangs from the baseline, so
			// its top edge sits at height zero
			name: "flipped-matrix",
			font: cffFontWithMatrix(emGrid(-2048)),
			want: 0,
		},
		{
			// an infinite matrix entry makes the transformed outline
			// degenerate
			name: "infinite-matrix",
			font: cffFontWithMatrix(matrix.Matrix{1, 0, 0, math.Inf(1), 0, 0}),
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.font.glyphHeight(tc.gid); got != tc.want {
				t.Errorf("glyph height = %d, want %d", got, tc.want)
			}
		})
	}
}
