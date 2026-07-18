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
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/geom/matrix"

	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/variation"

	"seehuhn.de/go/membudget"
)

// oneAxisStore builds an item variation store with a single region peaking at
// the +1 end of one axis, selected by vsindex 0 (one delta column).
func oneAxisStore() *variation.ItemVariationStore {
	f := variation.F2Dot14FromFloat
	return &variation.ItemVariationStore{
		Regions: []variation.Region{
			{{Start: f(0), Peak: f(1), End: f(1)}},
		},
		Data: []*variation.ItemVariationData{
			{RegionIndexes: []uint16{0}, Deltas: [][]int32{}},
		},
	}
}

// buildVarCFF2 returns a one-axis CFF2 font with a blended glyph, a blended
// stem pair that inverts at +1, and a blended private dict.
func buildVarCFF2() *OutlinesCFF2 {
	notdef := &GlyphCFF2{
		Cmds: []GlyphOpCFF2{{Op: OpMoveTo, Args: []Blend{{Default: 0}, {Default: 0}}}},
	}
	// a glyph whose outline and a hstem pair both vary; at +1 the stem pair
	// inverts (100 stays, 60 -> -20), so instancing must swap it.
	g := &GlyphCFF2{
		HStem: []Blend{
			{Default: 100},
			{Default: 60, Deltas: []float64{-80}},
		},
		Cmds: []GlyphOpCFF2{
			{Op: OpMoveTo, Args: []Blend{{Default: 0}, {Default: 0}}},
			{Op: OpHintMask, Args: []Blend{{Default: 0x80}}},
			{Op: OpLineTo, Args: []Blend{
				{Default: 100, Deltas: []float64{50}},  // x: 100 -> 150
				{Default: 200, Deltas: []float64{-30}}, // y: 200 -> 170
			}},
			{Op: OpCurveTo, Args: []Blend{
				{Default: 10}, {Default: 10},
				{Default: 20}, {Default: 20},
				{Default: 30, Deltas: []float64{5}}, {Default: 40},
			}},
		},
	}
	priv := &PrivateCFF2{
		BlueValues: []Blend{{Default: 0}, {Default: 12, Deltas: []float64{3}}},
		BlueScale:  Blend{Default: 0.04},
		BlueShift:  Blend{Default: 7},
		BlueFuzz:   Blend{Default: 1},
		StdHW:      Blend{Default: 50, Deltas: []float64{10}},
		StdVW:      Blend{Default: 80},
	}
	return &OutlinesCFF2{
		Glyphs:   []*GlyphCFF2{notdef, g},
		Widths:   []float64{500, 650},
		Private:  []*PrivateCFF2{priv},
		VarStore: oneAxisStore(),
	}
}

// TestInstanceDefaultIdentity instances at the default (all-zero) coordinates
// and checks that every glyph's outline, width and stem count are unchanged
// from the CFF2 default instance.
func TestInstanceDefaultIdentity(t *testing.T) {
	o := buildVarCFF2()
	coords := []variation.F2Dot14{0} // default instance

	static, err := o.Instance(coords, nil)
	if err != nil {
		t.Fatal(err)
	}

	if static.NumGlyphs() != o.NumGlyphs() {
		t.Fatalf("glyph count = %d, want %d", static.NumGlyphs(), o.NumGlyphs())
	}

	for gid := range o.Glyphs {
		want := collectPath(o.Path(glyph.ID(gid)))
		got := collectPath(static.Path(glyph.ID(gid)))
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("glyph %d path (-want +got):\n%s", gid, diff)
		}
		if got, want := static.Glyphs[gid].Width, o.Widths[gid]; got != want {
			t.Errorf("glyph %d width = %v, want %v", gid, got, want)
		}
		if got, want := len(static.Glyphs[gid].HStem), len(o.Glyphs[gid].HStem); got != want {
			t.Errorf("glyph %d hstem count = %d, want %d", gid, got, want)
		}
	}

	if !static.IsCIDKeyed() {
		t.Error("instanced outlines should be CID-keyed")
	}
	if got := static.ROS; got.Registry != "Adobe" || got.Ordering != "Identity" || got.Supplement != 0 {
		t.Errorf("ROS = %+v, want Adobe-Identity-0", got)
	}
	for gid, c := range static.GIDToCID {
		if int(c) != gid {
			t.Errorf("GIDToCID[%d] = %d, want %d", gid, c, gid)
		}
	}
}

// TestInstanceAtPlusOne instances at the +1 end of the axis and checks the
// evaluated coordinates against hand-computed blend values, including the
// inverted-stem swap and the blended private dict.
func TestInstanceAtPlusOne(t *testing.T) {
	o := buildVarCFF2()
	coords := []variation.F2Dot14{variation.F2Dot14FromFloat(1)}

	static, err := o.Instance(coords, nil)
	if err != nil {
		t.Fatal(err)
	}

	g := static.Glyphs[1]

	// stems: (100, 60-80=-20) -> swapped to (-20, 100)
	wantStem := []float64{-20, 100}
	if diff := cmp.Diff(wantStem, g.HStem); diff != "" {
		t.Errorf("hstem (-want +got):\n%s", diff)
	}

	// the lineto varies: x 100->150, y 200->170; curveto endpoint x 30->35.
	wantCmds := []GlyphOp{
		{Op: OpMoveTo, Args: []float64{0, 0}},
		{Op: OpHintMask, Args: []float64{0x80}},
		{Op: OpLineTo, Args: []float64{150, 170}},
		{Op: OpCurveTo, Args: []float64{10, 10, 20, 20, 35, 40}},
	}
	if diff := cmp.Diff(wantCmds, g.Cmds); diff != "" {
		t.Errorf("cmds (-want +got):\n%s", diff)
	}

	// private dict: BlueValues[1] 12->15, StdHW 50->60.
	p := static.Private[0]
	if got := p.BlueValues; len(got) != 2 || got[0] != 0 || got[1] != 15 {
		t.Errorf("blue values = %v, want [0 15]", got)
	}
	if p.StdHW != 60 {
		t.Errorf("StdHW = %v, want 60", p.StdHW)
	}
}

// TestInstanceNilVarStore instances a hand-built font with no variation store;
// every blend collapses to its default value.
func TestInstanceNilVarStore(t *testing.T) {
	o := &OutlinesCFF2{
		Glyphs: []*GlyphCFF2{{
			Cmds: []GlyphOpCFF2{
				{Op: OpMoveTo, Args: []Blend{{Default: 0}, {Default: 0}}},
				{Op: OpLineTo, Args: []Blend{{Default: 100}, {Default: 200}}},
			},
		}},
		Widths:  []float64{400},
		Private: []*PrivateCFF2{newPrivateCFF2()},
	}
	static, err := o.Instance(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := static.Glyphs[0].Width; got != 400 {
		t.Errorf("width = %v, want 400", got)
	}
	want := collectPath(o.Path(0))
	got := collectPath(static.Path(0))
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("path (-want +got):\n%s", diff)
	}
}

// TestInstanceExplicitWidths passes an explicit width slice and checks it takes
// precedence, with negatives clamped to zero.
func TestInstanceExplicitWidths(t *testing.T) {
	o := buildVarCFF2()
	static, err := o.Instance([]variation.F2Dot14{0}, []float64{700, -5})
	if err != nil {
		t.Fatal(err)
	}
	if got := static.Glyphs[0].Width; got != 700 {
		t.Errorf("glyph 0 width = %v, want 700", got)
	}
	if got := static.Glyphs[1].Width; got != 0 {
		t.Errorf("glyph 1 width = %v, want 0 (clamped)", got)
	}
}

// TestInstanceCFF1RoundTrip writes an instanced font through the CFF1 writer
// and reads it back, checking that the outlines and widths survive.
func TestInstanceCFF1RoundTrip(t *testing.T) {
	o := buildVarCFF2()
	static, err := o.Instance([]variation.F2Dot14{variation.F2Dot14FromFloat(1)}, nil)
	if err != nil {
		t.Fatal(err)
	}

	font := &Font{
		FontInfo: &type1.FontInfo{
			FontName:   "InstanceTest",
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		},
		Outlines: static,
	}

	var buf bytes.Buffer
	if err := font.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<26))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got.NumGlyphs() != static.NumGlyphs() {
		t.Fatalf("glyph count = %d, want %d", got.NumGlyphs(), static.NumGlyphs())
	}
	for gid := range static.Glyphs {
		want := collectPath(static.Path(glyph.ID(gid)))
		have := collectPath(got.Path(glyph.ID(gid)))
		if diff := cmp.Diff(want, have); diff != "" {
			t.Errorf("glyph %d path after round trip (-want +got):\n%s", gid, diff)
		}
		if w1, w2 := static.Glyphs[gid].Width, got.Glyphs[gid].Width; w1 != w2 {
			t.Errorf("glyph %d width = %v, want %v", gid, w2, w1)
		}
	}
}
