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

package cff

import (
	"math"
	"testing"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/funit"
)

func TestGlyphExtentCurve(t *testing.T) {
	// A cubic curve from (0,0) to (300,0) whose control points sit at y=200:
	// the extent must include the curve, which bulges well above the
	// endpoints.  The endpoint-only computation would report URy=0.
	g := &Glyph{
		Name: "curve",
		Cmds: []GlyphOp{
			{Op: OpMoveTo, Args: []float64{0, 0}},
			{Op: OpCurveTo, Args: []float64{100, 200, 200, 200, 300, 0}},
		},
	}

	got := g.Extent()
	want := funit.Rect16{LLx: 0, LLy: 0, URx: 300, URy: 200}
	if got != want {
		t.Errorf("Extent() = %v, want %v", got, want)
	}

	// Extent must agree with GlyphBBox (the matrix-aware sibling) under the
	// identity transform, so the two bounding-box methods stay consistent.
	o := &Outlines{Glyphs: []*Glyph{g}}
	if bbox := rectToFunit(o.GlyphBBox(matrix.Identity, 0)); bbox != got {
		t.Errorf("Extent() = %v, but GlyphBBox(identity) rounds to %v", got, bbox)
	}
}

func TestGlyphExtentBlank(t *testing.T) {
	g := &Glyph{Name: "blank"}
	if got := g.Extent(); got != (funit.Rect16{}) {
		t.Errorf("blank glyph Extent() = %v, want zero rectangle", got)
	}
}

func rectToFunit(r rect.Rect) funit.Rect16 {
	return funit.Rect16{
		LLx: funit.Int16(math.Floor(r.LLx)),
		LLy: funit.Int16(math.Floor(r.LLy)),
		URx: funit.Int16(math.Ceil(r.URx)),
		URy: funit.Int16(math.Ceil(r.URy)),
	}
}
