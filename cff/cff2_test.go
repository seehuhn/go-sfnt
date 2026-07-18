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
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/vec"

	"seehuhn.de/go/sfnt/glyph"
)

// blendArgs wraps plain coordinate values, optionally with deltas, as Blend
// arguments for a CFF2 glyph command.
func blendArgs(vals ...float64) []Blend {
	res := make([]Blend, len(vals))
	for i, v := range vals {
		res[i] = Blend{Default: v}
	}
	return res
}

func TestBlendAt(t *testing.T) {
	type testCase struct {
		blend   Blend
		scalars []float64
		want    float64
	}
	cases := []testCase{
		// nil deltas: does not vary
		{Blend{Default: 5}, []float64{0.5, 0.25}, 5},
		{Blend{Default: 5}, nil, 5},
		// exact match
		{Blend{Default: 10, Deltas: []float64{2, 4}}, []float64{0.5, 0.25}, 10 + 1 + 1},
		// len(scalars) > len(Deltas): extra scalars ignored
		{Blend{Default: 10, Deltas: []float64{2}}, []float64{0.5, 0.25}, 11},
		// len(scalars) < len(Deltas): missing scalars ignored (permissive)
		{Blend{Default: 10, Deltas: []float64{2, 4}}, []float64{0.5}, 11},
		// no scalars
		{Blend{Default: 10, Deltas: []float64{2, 4}}, nil, 10},
	}
	for i, c := range cases {
		got := c.blend.At(c.scalars)
		if got != c.want {
			t.Errorf("case %d: got %v, want %v", i, got, c.want)
		}
	}
}

// collectPath materialises a path iterator into command/point slices.
type pathStep struct {
	Cmd path.Command
	Pts []vec.Vec2
}

func collectPath(p path.Path) []pathStep {
	var steps []pathStep
	for cmd, pts := range p {
		clone := append([]vec.Vec2(nil), pts...)
		steps = append(steps, pathStep{Cmd: cmd, Pts: clone})
	}
	return steps
}

// TestPathMatchesCFF1 builds a two-glyph CFF2 outline (one glyph blended) and
// checks its default-instance Path matches the equivalent CFF1 outline built
// from the Default values.
func TestPathMatchesCFF1(t *testing.T) {
	// CFF1: a triangle and a curve
	cff1 := []*Glyph{
		{Cmds: []GlyphOp{
			{Op: OpMoveTo, Args: []float64{0, 0}},
			{Op: OpLineTo, Args: []float64{100, 0}},
			{Op: OpLineTo, Args: []float64{50, 100}},
		}},
		{Cmds: []GlyphOp{
			{Op: OpMoveTo, Args: []float64{10, 20}},
			{Op: OpCurveTo, Args: []float64{30, 40, 50, 60, 70, 80}},
		}},
	}

	// CFF2 mirror: glyph 1 carries deltas which must not affect Path
	cff2 := []*GlyphCFF2{
		{Cmds: []GlyphOpCFF2{
			{Op: OpMoveTo, Args: blendArgs(0, 0)},
			{Op: OpLineTo, Args: blendArgs(100, 0)},
			{Op: OpLineTo, Args: blendArgs(50, 100)},
		}},
		{Cmds: []GlyphOpCFF2{
			{Op: OpMoveTo, Args: []Blend{{Default: 10, Deltas: []float64{5}}, {Default: 20}}},
			{Op: OpCurveTo, Args: []Blend{
				{Default: 30, Deltas: []float64{-7}}, {Default: 40},
				{Default: 50}, {Default: 60},
				{Default: 70}, {Default: 80},
			}},
		}},
	}

	o1 := &Outlines{Glyphs: cff1}
	o2 := &OutlinesCFF2{Glyphs: cff2}

	for gid := range 2 {
		want := collectPath(o1.Path(glyph.ID(gid)))
		got := collectPath(o2.Path(glyph.ID(gid)))
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("gid %d path mismatch (-cff1 +cff2):\n%s", gid, diff)
		}
	}
}

// TestGlyphBBoxIgnoresDeltas checks the default-instance bbox depends only on
// the Default values.
func TestGlyphBBoxIgnoresDeltas(t *testing.T) {
	withDeltas := &OutlinesCFF2{Glyphs: []*GlyphCFF2{
		{Cmds: []GlyphOpCFF2{
			{Op: OpMoveTo, Args: []Blend{{Default: 0, Deltas: []float64{100}}, {Default: 0}}},
			{Op: OpLineTo, Args: []Blend{{Default: 200, Deltas: []float64{999}}, {Default: 0}}},
			{Op: OpLineTo, Args: blendArgs(100, 300)},
		}},
	}}
	noDeltas := &OutlinesCFF2{Glyphs: []*GlyphCFF2{
		{Cmds: []GlyphOpCFF2{
			{Op: OpMoveTo, Args: blendArgs(0, 0)},
			{Op: OpLineTo, Args: blendArgs(200, 0)},
			{Op: OpLineTo, Args: blendArgs(100, 300)},
		}},
	}}

	a := withDeltas.GlyphBBox(matrix.Identity, 0)
	b := noDeltas.GlyphBBox(matrix.Identity, 0)
	if a != b {
		t.Errorf("bbox depends on deltas: %v != %v", a, b)
	}
}

func TestOutlinesCFF2Basics(t *testing.T) {
	o := &OutlinesCFF2{Glyphs: []*GlyphCFF2{
		{}, // .notdef, blank
		{Cmds: []GlyphOpCFF2{{Op: OpMoveTo, Args: blendArgs(0, 0)}}},
	}}

	if o.NumGlyphs() != 2 {
		t.Errorf("NumGlyphs = %d, want 2", o.NumGlyphs())
	}
	if !o.IsBlank(0) {
		t.Error("glyph 0 should be blank")
	}
	if o.IsBlank(1) {
		t.Error("glyph 1 should not be blank")
	}
	// out-of-range gid maps to .notdef (blank)
	if !o.IsBlank(99) {
		t.Error("out-of-range gid should map to blank .notdef")
	}
	// out-of-range Path maps to glyph 0 (blank -> no steps)
	if steps := collectPath(o.Path(99)); len(steps) != 0 {
		t.Errorf("out-of-range Path: got %d steps, want 0", len(steps))
	}
}

// TestOutlinesCFF2Empty verifies that an OutlinesCFF2 with no glyphs (e.g.
// the zero value) does not panic and behaves as if every glyph were blank.
func TestOutlinesCFF2Empty(t *testing.T) {
	o := &OutlinesCFF2{}

	if !o.IsBlank(5) {
		t.Error("IsBlank on empty Glyphs should be true")
	}
	if steps := collectPath(o.Path(5)); len(steps) != 0 {
		t.Errorf("Path on empty Glyphs: got %d steps, want 0", len(steps))
	}
	if bbox := o.GlyphBBox(matrix.Identity, 5); !bbox.IsZero() {
		t.Errorf("GlyphBBox on empty Glyphs: got %v, want zero rect", bbox)
	}
}

// TestGlyphMatrixNilFDSelect verifies that GlyphMatrix uses FontMatrices[0]
// when FDSelect is nil, instead of discarding it.
func TestGlyphMatrixNilFDSelect(t *testing.T) {
	top := matrix.Matrix{0.002, 0, 0, 0.002, 0, 0}
	fd0 := matrix.Matrix{2, 0, 0, 2, 0, 0} // non-identity
	o := &OutlinesCFF2{
		FDSelect:     nil,
		FontMatrices: []matrix.Matrix{fd0},
	}
	got := o.GlyphMatrix(top, 0)
	want := fd0.Mul(top)
	if got != want {
		t.Errorf("GlyphMatrix with nil FDSelect: got %v, want %v", got, want)
	}
}

func TestNewPrivateCFF2Defaults(t *testing.T) {
	p := newPrivateCFF2()
	if p.BlueScale.Default != 0.039625 {
		t.Errorf("BlueScale = %v, want 0.039625", p.BlueScale.Default)
	}
	if p.BlueShift.Default != 7 {
		t.Errorf("BlueShift = %v, want 7", p.BlueShift.Default)
	}
	if p.BlueFuzz.Default != 1 {
		t.Errorf("BlueFuzz = %v, want 1", p.BlueFuzz.Default)
	}
	if p.ExpansionFactor.Default != 0.06 {
		t.Errorf("ExpansionFactor = %v, want 0.06", p.ExpansionFactor.Default)
	}
	if p.LanguageGroup != 0 {
		t.Errorf("LanguageGroup = %v, want 0", p.LanguageGroup)
	}
}

func TestFontCFF2Clone(t *testing.T) {
	orig := &FontCFF2{
		FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		OutlinesCFF2: &OutlinesCFF2{
			Glyphs:       []*GlyphCFF2{{}, {}},
			Widths:       []float64{100, 200},
			Private:      []*PrivateCFF2{newPrivateCFF2()},
			FontMatrices: []matrix.Matrix{matrix.Identity},
		},
	}
	clone := orig.Clone()

	if clone == orig || clone.OutlinesCFF2 == orig.OutlinesCFF2 {
		t.Fatal("Clone must return fresh structs")
	}
	// top-level slices must be independent
	clone.Glyphs[0] = &GlyphCFF2{Cmds: []GlyphOpCFF2{{Op: OpMoveTo, Args: blendArgs(1, 2)}}}
	if orig.Glyphs[0] == clone.Glyphs[0] {
		t.Error("Glyphs slice not copied")
	}
	clone.Widths[0] = 999
	if orig.Widths[0] == 999 {
		t.Error("Widths slice not copied")
	}
	if clone.FontMatrix != orig.FontMatrix {
		t.Error("FontMatrix not preserved")
	}
}
