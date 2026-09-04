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

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyph"
)

// rectClose reports whether two rectangles agree to within tol.
func rectClose(a, b rect.Rect, tol float64) bool {
	return math.Abs(a.LLx-b.LLx) <= tol && math.Abs(a.LLy-b.LLy) <= tol &&
		math.Abs(a.URx-b.URx) <= tol && math.Abs(a.URy-b.URy) <= tol
}

// emGrid returns the font matrix for outlines drawn on a grid of n units
// per em.
func emGrid(n float64) matrix.Matrix {
	return matrix.Matrix{1 / n, 0, 0, 1 / n, 0, 0}
}

// cffBox returns a CFF glyph whose ink is a box of the given height,
// measured in charstring units.
func cffBox(height float64) *cff.Glyph {
	g := cff.NewGlyph("box", 2*height)
	g.MoveTo(0, 0)
	g.LineTo(height, 0)
	g.LineTo(height, height)
	return g
}

// cff2Box returns a CFF2 glyph whose ink is a box of the given height,
// measured in charstring units.
func cff2Box(height float64) *cff.GlyphCFF2 {
	b := func(v float64) cff.Blend { return cff.Blend{Default: v} }
	return &cff.GlyphCFF2{Cmds: []cff.GlyphOpCFF2{
		{Op: cff.OpMoveTo, Args: []cff.Blend{b(0), b(0)}},
		{Op: cff.OpLineTo, Args: []cff.Blend{b(height), b(0)}},
		{Op: cff.OpLineTo, Args: []cff.Blend{b(height), b(height)}},
	}}
}

// cidKeyedFont returns a CID-keyed CFF font with two glyphs of identical
// outlines, drawn on grids of 1000 and 2000 units per em.  The first glyph
// therefore covers the whole em, the second only half of it.
func cidKeyedFont() *Font {
	return &Font{
		UnitsPerEm: 1000,
		FontMatrix: matrix.Identity,
		Outlines: &cff.Outlines{
			Glyphs:       []*cff.Glyph{cffBox(1000), cffBox(1000)},
			Private:      []*type1.PrivateDict{{}, {}},
			FDSelect:     func(gid glyph.ID) int { return int(gid) },
			FontMatrices: []matrix.Matrix{emGrid(1000), emGrid(2000)},
			ROS:          &cid.SystemInfo{Registry: "Test", Ordering: "Test"},
			GIDToCID:     []cid.CID{0, 1},
		},
	}
}

// cffFontWithMatrix returns a font with a single CFF glyph, using the given
// font matrix in place of a well-formed one.
func cffFontWithMatrix(fm matrix.Matrix) *Font {
	return &Font{
		UnitsPerEm: 1000,
		FontMatrix: fm,
		Outlines: &cff.Outlines{
			Glyphs:   []*cff.Glyph{cffBox(1400)},
			Private:  []*type1.PrivateDict{{}},
			FDSelect: func(glyph.ID) int { return 0 },
		},
	}
}
