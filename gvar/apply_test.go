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

package gvar

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/variation"
)

func b8(v int8) byte { return byte(v) }

func makeSimple(contours ...[]glyf.Point) *glyf.Glyph {
	su := &glyf.SimpleUnpacked{Contours: make([]glyf.Contour, len(contours))}
	for i, c := range contours {
		su.Contours[i] = glyf.Contour(c)
	}
	g := su.AsGlyph()
	return &g
}

func outlinePoints(t *testing.T, g *glyf.Glyph) []glyf.Point {
	t.Helper()
	sg, ok := g.Data.(glyf.SimpleGlyph)
	if !ok {
		t.Fatalf("not a simple glyph: %T", g.Data)
	}
	su, err := sg.Unpack()
	if err != nil {
		t.Fatal(err)
	}
	var out []glyf.Point
	for _, c := range su.Contours {
		out = append(out, c...)
	}
	return out
}

func mustEncode(t *testing.T, tuples []variation.TupleVariation, axisCount, nPoints int) []byte {
	t.Helper()
	b, err := variation.EncodeTupleData(tuples, axisCount, 2, nPoints, nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestApplySimpleIUP builds a 1-axis, 1-glyph gvar table by hand: a square
// glyph with an all-points tuple and a subset tuple requiring IUP for two
// untouched points, then checks the instance outline at three coordinates.
func TestApplySimpleIUP(t *testing.T) {
	// square: p0=(0,0) p1=(100,0) p2=(100,100) p3=(0,100)
	glyphs := glyf.Glyphs{makeSimple([]glyf.Point{
		{X: 0, Y: 0, OnCurve: true},
		{X: 100, Y: 0, OnCurve: true},
		{X: 100, Y: 100, OnCurve: true},
		{X: 0, Y: 100, OnCurve: true},
	})}
	widths := []funit.Uint16{200}

	const nPoints = 8 // 4 outline + 4 phantom
	tuples := []variation.TupleVariation{
		{ // all points; phantom p5 grows the advance by 50
			Peak:   []variation.F2Dot14{0x4000},
			Deltas: []int32{20, 20, 20, 20, 0, 50, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{ // subset {1,3}; x deltas 30 and 10, points 0 and 2 inferred by IUP
			Peak:   []variation.F2Dot14{0x4000},
			Points: []uint16{1, 3},
			Deltas: []int32{30, 10, 0, 0},
		},
	}
	tbl := &Table{
		AxisCount: 1,
		PerGlyph:  []GlyphData{{Data: mustEncode(t, tuples, 1, nPoints)}},
	}

	tests := []struct {
		coord       variation.F2Dot14
		wantPoints  []glyf.Point
		wantAdvance funit.Uint16
	}{
		{ // coord 0: neither tuple active, outline and advance unchanged
			coord: 0,
			wantPoints: []glyf.Point{
				{X: 0, Y: 0, OnCurve: true},
				{X: 100, Y: 0, OnCurve: true},
				{X: 100, Y: 100, OnCurve: true},
				{X: 0, Y: 100, OnCurve: true},
			},
			wantAdvance: 200,
		},
		{ // coord 1.0: s=1 for both.  IUP gives point0 +10, point2 +30.
			// total dx: p0 30, p1 50, p2 50, p3 30
			coord: 0x4000,
			wantPoints: []glyf.Point{
				{X: 30, Y: 0, OnCurve: true},
				{X: 150, Y: 0, OnCurve: true},
				{X: 150, Y: 100, OnCurve: true},
				{X: 30, Y: 100, OnCurve: true},
			},
			wantAdvance: 250,
		},
		{ // coord 0.5: s=0.5 for both, halved deltas
			coord: 0x2000,
			wantPoints: []glyf.Point{
				{X: 15, Y: 0, OnCurve: true},
				{X: 125, Y: 0, OnCurve: true},
				{X: 125, Y: 100, OnCurve: true},
				{X: 15, Y: 100, OnCurve: true},
			},
			wantAdvance: 225,
		},
	}

	for _, tt := range tests {
		res, err := tbl.Apply(glyphs, widths, 0, []variation.F2Dot14{tt.coord}, testBudget(), nil)
		if err != nil {
			t.Fatalf("coord %d: %v", tt.coord, err)
		}
		if res.Advance != tt.wantAdvance {
			t.Errorf("coord %d: advance = %d, want %d", tt.coord, res.Advance, tt.wantAdvance)
		}
		got := outlinePoints(t, res.Glyph)
		if diff := cmp.Diff(tt.wantPoints, got); diff != "" {
			t.Errorf("coord %d: outline mismatch (-want +got):\n%s", tt.coord, diff)
		}
	}
}

// TestApplyComposite exercises component offset adjustment, int8->int16
// widening, flag preservation and the untouched point-matching component.
func TestApplyComposite(t *testing.T) {
	comp0 := glyf.GlyphComponent{ // XY offsets, byte form
		Flags:      glyf.FlagArgsAreXYValues,
		GlyphIndex: 1,
		Data:       []byte{b8(100), b8(-50)},
	}
	comp1 := glyf.GlyphComponent{ // XY offsets, byte form at the int8 boundary
		Flags:      glyf.FlagArgsAreXYValues,
		GlyphIndex: 2,
		Data:       []byte{b8(127), b8(0)},
	}
	comp2 := glyf.GlyphComponent{ // point matching, must be left untouched
		Flags:      0,
		GlyphIndex: 3,
		Data:       []byte{b8(3), b8(4)},
	}
	composite := &glyf.Glyph{
		Rect16: funit.Rect16{LLx: 0, LLy: 0, URx: 500, URy: 700},
		Data: glyf.CompositeGlyph{
			Components: []glyf.GlyphComponent{comp0, comp1, comp2},
		},
	}
	glyphs := glyf.Glyphs{composite}
	widths := []funit.Uint16{300}

	const nPoints = 7 // 3 components + 4 phantom
	tuples := []variation.TupleVariation{
		{
			Peak: []variation.F2Dot14{0x4000},
			// x: comp0 +5, comp1 +1, comp2 +100 (ignored), phantom p4 +25
			// y: comp0 +3, comp1 0, comp2 +100 (ignored)
			Deltas: []int32{5, 1, 100, 0, 25, 0, 0, 3, 0, 100, 0, 0, 0, 0},
		},
	}
	tbl := &Table{
		AxisCount: 1,
		PerGlyph:  []GlyphData{{Data: mustEncode(t, tuples, 1, nPoints)}},
	}

	res, err := tbl.Apply(glyphs, widths, 0, []variation.F2Dot14{0x4000}, testBudget(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Advance != 325 {
		t.Errorf("advance = %d, want 325", res.Advance)
	}

	cg := res.Glyph.Data.(glyf.CompositeGlyph)

	// composite header bbox left unchanged
	if res.Glyph.Rect16 != composite.Rect16 {
		t.Errorf("bbox changed: %v", res.Glyph.Rect16)
	}

	// comp0: byte form retained, args 105 and -47
	if got, want := cg.Components[0].Flags, glyf.FlagArgsAreXYValues; got != want {
		t.Errorf("comp0 flags = %v, want %v", got, want)
	}
	if diff := cmp.Diff([]byte{b8(105), b8(-47)}, cg.Components[0].Data); diff != "" {
		t.Errorf("comp0 data (-want +got):\n%s", diff)
	}

	// comp1: widened to words, args 128 and 0
	if got, want := cg.Components[1].Flags, glyf.FlagArgsAreXYValues|glyf.FlagArg1And2AreWords; got != want {
		t.Errorf("comp1 flags = %v, want %v", got, want)
	}
	if diff := cmp.Diff([]byte{0x00, 0x80, 0x00, 0x00}, cg.Components[1].Data); diff != "" {
		t.Errorf("comp1 data (-want +got):\n%s", diff)
	}

	// comp2: point matching, untouched
	if got, want := cg.Components[2].Flags, glyf.ComponentFlag(0); got != want {
		t.Errorf("comp2 flags = %v, want %v", got, want)
	}
	if diff := cmp.Diff([]byte{b8(3), b8(4)}, cg.Components[2].Data); diff != "" {
		t.Errorf("comp2 data (-want +got):\n%s", diff)
	}
}

// TestApplyEmptyGlyph checks that a nil glyph still varies its advance width.
func TestApplyEmptyGlyph(t *testing.T) {
	glyphs := glyf.Glyphs{nil}
	widths := []funit.Uint16{200}

	const nPoints = 4 // phantoms only
	tuples := []variation.TupleVariation{
		{
			Peak:   []variation.F2Dot14{0x4000},
			Deltas: []int32{0, 50, 0, 0, 0, 0, 0, 0}, // phantom p1 - p0 = 50
		},
	}
	tbl := &Table{
		AxisCount: 1,
		PerGlyph:  []GlyphData{{Data: mustEncode(t, tuples, 1, nPoints)}},
	}

	res, err := tbl.Apply(glyphs, widths, 0, []variation.F2Dot14{0x4000}, testBudget(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Glyph != nil {
		t.Errorf("empty glyph produced a non-nil outline")
	}
	if res.Advance != 250 {
		t.Errorf("advance = %d, want 250", res.Advance)
	}
}

// TestApplyNegativeHalfTie checks that accumulated deltas landing on an exact
// negative half-integer tie round with OpenType's otRound convention (toward
// +Inf), not Go's math.Round (away from zero): -3.5 must round to -3, and a
// component offset delta of -0.5 must round to 0.
func TestApplyNegativeHalfTie(t *testing.T) {
	t.Run("outline point", func(t *testing.T) {
		// square: p0=(0,0) p1=(100,0) p2=(100,100) p3=(0,100)
		glyphs := glyf.Glyphs{makeSimple([]glyf.Point{
			{X: 0, Y: 0, OnCurve: true},
			{X: 100, Y: 0, OnCurve: true},
			{X: 100, Y: 100, OnCurve: true},
			{X: 0, Y: 100, OnCurve: true},
		})}
		widths := []funit.Uint16{200}

		const nPoints = 8 // 4 outline + 4 phantom
		tuples := []variation.TupleVariation{
			{ // all points; at coord 0.5 (scalar 0.5) p0.x delta -7 gives dx=-3.5
				Peak:   []variation.F2Dot14{0x4000},
				Deltas: []int32{-7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		}
		tbl := &Table{
			AxisCount: 1,
			PerGlyph:  []GlyphData{{Data: mustEncode(t, tuples, 1, nPoints)}},
		}

		res, err := tbl.Apply(glyphs, widths, 0, []variation.F2Dot14{0x2000}, testBudget(), nil)
		if err != nil {
			t.Fatal(err)
		}
		got := outlinePoints(t, res.Glyph)
		if got[0].X != -3 {
			t.Errorf("p0.X = %d, want -3 (otRound(-3.5)), math.Round would give -4", got[0].X)
		}
	})

	t.Run("composite offset", func(t *testing.T) {
		comp0 := glyf.GlyphComponent{
			Flags:      glyf.FlagArgsAreXYValues,
			GlyphIndex: 1,
			Data:       []byte{b8(0), b8(0)},
		}
		composite := &glyf.Glyph{
			Rect16: funit.Rect16{LLx: 0, LLy: 0, URx: 500, URy: 700},
			Data:   glyf.CompositeGlyph{Components: []glyf.GlyphComponent{comp0}},
		}
		glyphs := glyf.Glyphs{composite}
		widths := []funit.Uint16{300}

		const nPoints = 5 // 1 component + 4 phantom
		tuples := []variation.TupleVariation{
			{ // at coord 0.5 (scalar 0.5) comp0.x delta -1 gives dx=-0.5
				Peak:   []variation.F2Dot14{0x4000},
				Deltas: []int32{-1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		}
		tbl := &Table{
			AxisCount: 1,
			PerGlyph:  []GlyphData{{Data: mustEncode(t, tuples, 1, nPoints)}},
		}

		res, err := tbl.Apply(glyphs, widths, 0, []variation.F2Dot14{0x2000}, testBudget(), nil)
		if err != nil {
			t.Fatal(err)
		}
		cg := res.Glyph.Data.(glyf.CompositeGlyph)
		if diff := cmp.Diff([]byte{b8(0), b8(0)}, cg.Components[0].Data); diff != "" {
			t.Errorf("comp0 data (-want +got), otRound(-0.5) should give offset 0:\n%s", diff)
		}
	})
}

// TestApplyWorkBudget checks that an exhausted work budget returns ErrWorkLimit.
func TestApplyWorkBudget(t *testing.T) {
	glyphs := glyf.Glyphs{makeSimple([]glyf.Point{
		{X: 0, Y: 0, OnCurve: true},
		{X: 100, Y: 0, OnCurve: true},
		{X: 50, Y: 100, OnCurve: true},
	})}
	widths := []funit.Uint16{100}

	const nPoints = 7 // 3 outline + 4 phantom
	tuples := []variation.TupleVariation{
		{Peak: []variation.F2Dot14{0x4000}, Deltas: make([]int32, 2*nPoints)},
	}
	// give a non-zero delta so the tuple is meaningful
	tuples[0].Deltas[0] = 10
	tbl := &Table{
		AxisCount: 1,
		PerGlyph:  []GlyphData{{Data: mustEncode(t, tuples, 1, nPoints)}},
	}

	wb := int64(1) // one active tuple charges nPoints = 7 > 1
	_, err := tbl.Apply(glyphs, widths, 0, []variation.F2Dot14{0x4000}, testBudget(), &wb)
	if !errors.Is(err, ErrWorkLimit) {
		t.Fatalf("err = %v, want ErrWorkLimit", err)
	}

	// a generous budget succeeds and is decremented
	wb = 1000
	if _, err := tbl.Apply(glyphs, widths, 0, []variation.F2Dot14{0x4000}, testBudget(), &wb); err != nil {
		t.Fatal(err)
	}
	if wb != 1000-nPoints {
		t.Errorf("work budget = %d, want %d", wb, 1000-nPoints)
	}
}
