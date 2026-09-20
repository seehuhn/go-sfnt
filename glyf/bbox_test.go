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

package glyf

import (
	"math"
	"testing"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
)

// compositeOf builds a composite glyph with a deliberately wrong stored box,
// so that a test can only pass if the box was recomputed.
func compositeOf(components ...GlyphComponent) *Glyph {
	return &Glyph{
		Rect16: funit.Rect16{LLx: -1, LLy: -1, URx: 1, URy: 1},
		Data:   CompositeGlyph{Components: components},
	}
}

func checkBBoxes(t *testing.T, gg Glyphs, want []funit.Rect16) {
	t.Helper()
	for gid, box := range want {
		if got := gg[gid].Rect16; got != box {
			t.Errorf("glyph %d: got %+v, want %+v", gid, got, box)
		}
	}
}

// TestCompositeBBoxUnion checks that the box of a composite glyph is the union
// of its components' boxes, moved by the component offsets.
func TestCompositeBBoxUnion(t *testing.T) {
	gg := Glyphs{
		squareGlyph(100),
		compositeOf(offsetComponent(0, 0, 0), offsetComponent(0, 150, 0)),
	}
	gg.RecomputeCompositeBBoxes()
	checkBBoxes(t, gg, []funit.Rect16{
		{URx: 100, URy: 100},
		{URx: 250, URy: 100},
	})
}

// TestCompositeBBoxNested checks that a composite component is computed
// before the glyph which uses it, even when the parent has the lower glyph
// ID.
func TestCompositeBBoxNested(t *testing.T) {
	gg := Glyphs{
		squareGlyph(100),
		compositeOf(offsetComponent(2, 0, 0), offsetComponent(2, 150, 0)), // outer
		compositeOf(offsetComponent(0, 0, 0)),                             // inner
	}
	gg.RecomputeCompositeBBoxes()
	checkBBoxes(t, gg, []funit.Rect16{
		{URx: 100, URy: 100},
		{URx: 250, URy: 100},
		{URx: 100, URy: 100},
	})
}

// TestCompositeBBoxChain checks a chain of composite glyphs which each
// reference the next one twice.  Expanding such a chain grows exponentially
// with its depth; computing the boxes bottom-up does not, so a chain far
// deeper than any real font finishes and yields the exact boxes.
func TestCompositeBBoxChain(t *testing.T) {
	const depth = 64

	gg := make(Glyphs, depth+1)
	gg[depth] = squareGlyph(100)
	for i := depth - 1; i >= 0; i-- {
		next := glyph.ID(i + 1)
		gg[i] = compositeOf(offsetComponent(next, 0, 0), offsetComponent(next, 10, 0))
	}

	gg.RecomputeCompositeBBoxes()

	want := make([]funit.Rect16, depth+1)
	for i := range want {
		want[i] = funit.Rect16{URx: funit.Int16(100 + 10*(depth-i)), URy: 100}
	}
	checkBBoxes(t, gg, want)
}

// TestCompositeBBoxCycle checks that reference cycles, which valid fonts do
// not contain, terminate and give a well-defined result: a component which
// refers back to a glyph still being computed contributes that glyph's stored
// box.
func TestCompositeBBoxCycle(t *testing.T) {
	stored := funit.Rect16{URx: 100, URy: 100}
	withBox := func(g *Glyph) *Glyph {
		g.Rect16 = stored
		return g
	}

	gg := Glyphs{
		withBox(compositeOf(offsetComponent(0, 10, 0))), // refers to itself
		withBox(compositeOf(offsetComponent(2, 10, 0))), // a -> b
		withBox(compositeOf(offsetComponent(1, 10, 0))), // b -> a
		squareGlyph(100),
		withBox(compositeOf(offsetComponent(3, 0, 0), offsetComponent(4, 50, 0))), // outline, then itself
	}
	gg.RecomputeCompositeBBoxes()
	checkBBoxes(t, gg, []funit.Rect16{
		{LLx: 10, URx: 110, URy: 100},
		{LLx: 20, URx: 120, URy: 100}, // b was computed inside a, from a's stored box
		{LLx: 10, URx: 110, URy: 100},
		{URx: 100, URy: 100},
		{URx: 150, URy: 100},
	})
}

// TestCompositeBBoxBlankChild checks that components which leave no marks
// contribute nothing, and that a composite glyph made only of such components
// ends up blank rather than keeping its stored box.
func TestCompositeBBoxBlankChild(t *testing.T) {
	blank := &Glyph{Data: SimpleGlyph{NumContours: 0, Encoded: []byte{0, 0}}}
	gg := Glyphs{
		compositeOf(offsetComponent(1, 10, 0), offsetComponent(2, 20, 0), offsetComponent(99, 30, 0)),
		nil,
		blank,
	}
	gg.RecomputeCompositeBBoxes()
	checkBBoxes(t, gg, []funit.Rect16{{}})
}

// TestCompositeBBoxRotated checks that the box of a transformed component is
// rounded outwards, so that it still contains the transformed outline.
func TestCompositeBBoxRotated(t *testing.T) {
	rotated := (&ComponentUnpacked{Child: 0, Trfm: matrix.Rotate(math.Pi / 4)}).Pack()
	gg := Glyphs{
		squareGlyph(100),
		compositeOf(rotated),
	}
	gg.RecomputeCompositeBBoxes()
	// the square's corners map to (0,0), (±70.7, 70.7) and (0, 141.4)
	checkBBoxes(t, gg, []funit.Rect16{
		{URx: 100, URy: 100},
		{LLx: -71, LLy: 0, URx: 71, URy: 142},
	})
}

// TestCompositeBBoxSaturation checks that a transformed box which leaves the
// int16 range is clamped, corner by corner, while the corners still in range
// stay exact.
func TestCompositeBBoxSaturation(t *testing.T) {
	full := &Glyph{
		Rect16: funit.Rect16{LLx: -32768, LLy: -32768, URx: 32767, URy: 32767},
		Data:   SimpleGlyph{NumContours: 0, Encoded: []byte{0, 0}},
	}
	scaled := (&ComponentUnpacked{
		Child: 0,
		Trfm:  matrix.Matrix{1.99, 0, 0, 1.99, 32767, -32768},
	}).Pack()
	gg := Glyphs{full, compositeOf(scaled)}
	gg.RecomputeCompositeBBoxes()

	// the stored scale is 32604/16384, so -32768 scales to exactly -65208
	checkBBoxes(t, gg, []funit.Rect16{
		{LLx: -32768, LLy: -32768, URx: 32767, URy: 32767},
		{LLx: -32441, LLy: -32768, URx: 32767, URy: 32439},
	})
}

// TestCompositeBBoxTruncatedComponent checks that a component whose data is
// too short for its flags is skipped, as it is when the outline is drawn.
func TestCompositeBBoxTruncatedComponent(t *testing.T) {
	truncated := GlyphComponent{Flags: FlagArgsAreXYValues | FlagArg1And2AreWords, Data: []byte{0}}
	gg := Glyphs{
		squareGlyph(100),
		compositeOf(truncated, offsetComponent(0, 150, 0)),
	}
	gg.RecomputeCompositeBBoxes()
	checkBBoxes(t, gg, []funit.Rect16{
		{URx: 100, URy: 100},
		{LLx: 150, URx: 250, URy: 100},
	})
}
