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

	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
)

// TestGlyphBBoxDesignUnits checks that GlyphBBox reports a glyph's bounding
// box in font design units, whatever coordinate system the outlines use.
func TestGlyphBBoxDesignUnits(t *testing.T) {
	glyfGlyph := &glyf.Glyph{}
	glyfGlyph.URy = 1400

	cidKeyed := cidKeyedFont()

	tests := []struct {
		name string
		font *Font
		gid  glyph.ID
		want rect.Rect
	}{
		{
			// a 1400-unit box on a 2048-unit charstring grid covers
			// 0.68359 em, or 683.59375 units of the 1000-unit design grid
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
			want: rect.Rect{URx: 683.59375, URy: 683.59375},
		},
		{
			// a 1024-unit box on a 2048-unit charstring grid covers half
			// the em
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
			want: rect.Rect{URx: 500, URy: 500},
		},
		{
			// glyf outlines are always drawn on the design grid
			name: "glyf",
			font: &Font{
				UnitsPerEm: 2048,
				FontMatrix: emGrid(2048),
				Outlines:   &glyf.Outlines{Glyphs: glyf.Glyphs{glyfGlyph}},
			},
			want: rect.Rect{URy: 1400},
		},
		{
			name: "cff-cid-full-em",
			font: cidKeyed,
			gid:  0,
			want: rect.Rect{URx: 1000, URy: 1000},
		},
		{
			name: "cff-cid-half-em",
			font: cidKeyed,
			gid:  1,
			want: rect.Rect{URx: 500, URy: 500},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.font.GlyphBBox(tc.gid)
			if !rectClose(got, tc.want, 1e-9) {
				t.Errorf("glyph bbox = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFontBBoxDesignUnits checks that FontBBox encloses every glyph and is
// reported in font design units.
func TestFontBBoxDesignUnits(t *testing.T) {
	f := cidKeyedFont()

	// the first glyph covers the whole em, the second only half of it
	want := rect.Rect{URx: 1000, URy: 1000}
	if got := f.FontBBox(); !rectClose(got, want, 1e-9) {
		t.Errorf("font bbox = %v, want %v", got, want)
	}
}

// TestFontBBoxIsExactOnDesignGrid checks that glyf outlines, which are drawn
// on the design grid, are reported without rounding error whatever the units
// per em.  Scaling through a different grid would leave the coordinates just
// off an integer, and the outward rounding in bboxRect16 would then grow the
// stored box by a whole unit.
func TestFontBBoxIsExactOnDesignGrid(t *testing.T) {
	want := funit.Rect16{LLx: -37, LLy: -1999, URx: 1801, URy: 1999}

	// 1200 and 1800 are the interesting cases: 1000/unitsPerEm is not
	// representable, so a round trip through PDF glyph space is inexact
	for _, unitsPerEm := range []uint16{1000, 1024, 1200, 1800, 2000, 2048} {
		g := &glyf.Glyph{}
		g.LLx, g.LLy, g.URx, g.URy = want.LLx, want.LLy, want.URx, want.URy
		f := &Font{
			UnitsPerEm: unitsPerEm,
			FontMatrix: emGrid(float64(unitsPerEm)),
			Outlines:   &glyf.Outlines{Glyphs: glyf.Glyphs{g}},
		}
		if got := bboxRect16(f.FontBBox()); got != want {
			t.Errorf("unitsPerEm %d: font bbox = %v, want %v",
				unitsPerEm, got, want)
		}
	}
}

// TestFontBBoxPDF checks that FontBBoxPDF reports the font bounding box in
// PDF glyph space units, independently of the design grid.
func TestFontBBoxPDF(t *testing.T) {
	g := &glyf.Glyph{}
	g.URx, g.URy = 1024, 1024
	f := &Font{
		UnitsPerEm: 2048,
		FontMatrix: emGrid(2048),
		Outlines:   &glyf.Outlines{Glyphs: glyf.Glyphs{g}},
	}

	// the ink covers half the em, i.e. 500 PDF glyph space units
	want := rect.Rect{URx: 500, URy: 500}
	if got := f.FontBBoxPDF(); !rectClose(got, want, 1e-9) {
		t.Errorf("font bbox = %v, want %v", got, want)
	}
}
