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

package debug

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/stat"
	"seehuhn.de/go/sfnt/variation"
)

func testBudget() *membudget.Budget { return parser.NewBudget(1 << 20) }

// varGlyphs returns the glyf outlines and advance widths of the fixture.
func varGlyphs(f *sfnt.Font) (glyf.Glyphs, []funit.Uint16) {
	o := f.Outlines.(*glyf.Outlines)
	return o.Glyphs, o.Widths
}

// simpleContour unpacks a simple glyph's single contour.
func simpleContour(t *testing.T, g *glyf.Glyph) []glyf.Point {
	t.Helper()
	sg, ok := g.Data.(glyf.SimpleGlyph)
	if !ok {
		t.Fatalf("glyph is not simple: %T", g.Data)
	}
	su, err := sg.Unpack()
	if err != nil {
		t.Fatal(err)
	}
	if len(su.Contours) != 1 {
		t.Fatalf("expected 1 contour, got %d", len(su.Contours))
	}
	return su.Contours[0]
}

// TestMakeVarFontRoundTrip runs the whole synthetic font through
// Write -> Read -> Write and checks the variation tables survive.
func TestMakeVarFontRoundTrip(t *testing.T) {
	src := MakeVarFont()

	buf1 := &bytes.Buffer{}
	if _, err := src.Write(buf1); err != nil {
		t.Fatal(err)
	}
	b1 := buf1.Bytes()

	f2, err := sfnt.Read(bytes.NewReader(b1), parser.NewBudget(int64(len(b1))))
	if err != nil {
		t.Fatal(err)
	}

	// name resolution
	if got := f2.VariationAxes(); len(got) != 2 ||
		got[0].Name != "Weight" || got[1].Name != "Width" {
		t.Errorf("axis names: %+v", got)
	}
	inst := f2.NamedInstances()
	if len(inst) != 1 || inst[0].Name != "Bold Narrow" ||
		inst[0].PostScriptName != "QuireVar-BoldNarrow" {
		t.Errorf("named instances: %+v", inst)
	}
	if inst[0].Coordinates["wght"] != 700 || inst[0].Coordinates["wdth"] != 75 {
		t.Errorf("instance coordinates: %+v", inst[0].Coordinates)
	}
	if f2.VariationsPostScriptName != "QuireVar" {
		t.Errorf("VariationsPostScriptName = %q", f2.VariationsPostScriptName)
	}
	if f2.Stat.DesignAxes[0].Name != "Weight" ||
		f2.Stat.AxisValues[0].(*stat.Format1).Name != "Regular" ||
		f2.Stat.AxisValues[1].(*stat.Format2).Name != "Normal" {
		t.Error("STAT names not resolved")
	}
	if !f2.IsVariable() {
		t.Error("f2 is not variable")
	}

	// name-independent tables must survive Read unchanged
	if diff := cmp.Diff(src.Avar, f2.Avar); diff != "" {
		t.Errorf("avar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Gvar, f2.Gvar); diff != "" {
		t.Errorf("gvar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Cvar, f2.Cvar); diff != "" {
		t.Errorf("cvar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Hvar, f2.Hvar); diff != "" {
		t.Errorf("hvar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Mvar, f2.Mvar); diff != "" {
		t.Errorf("mvar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Gdef.ItemVarStore, f2.Gdef.ItemVarStore); diff != "" {
		t.Errorf("GDEF item variation store mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Gsub.Variations, f2.Gsub.Variations); diff != "" {
		t.Errorf("GSUB feature variations mismatch (-src +f2):\n%s", diff)
	}

	// second write must be byte-identical (fixpoint)
	buf2 := &bytes.Buffer{}
	if _, err := f2.Write(buf2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, buf2.Bytes()) {
		t.Errorf("second write not byte-identical: %d vs %d bytes", len(b1), buf2.Len())
	}
}

// TestMakeVarFontGvarDefault checks that applying gvar at the default
// coordinates reproduces the original outlines and advances exactly.
func TestMakeVarFontGvarDefault(t *testing.T) {
	f := MakeVarFont()
	glyphs, widths := varGlyphs(f)
	coords := []variation.F2Dot14{0, 0}

	for gid := glyph.ID(0); int(gid) < len(glyphs); gid++ {
		res, err := f.Gvar.Apply(glyphs, widths, gid, coords, testBudget(), nil)
		if err != nil {
			t.Fatalf("gid %d: %v", gid, err)
		}
		if diff := cmp.Diff(glyphs[gid], res.Glyph); diff != "" {
			t.Errorf("gid %d outline changed at default (-orig +applied):\n%s", gid, diff)
		}
		if got, want := res.Advance, widths[gid]; got != want {
			t.Errorf("gid %d advance = %d, want %d", gid, got, want)
		}
	}
}

// TestMakeVarFontExpect checks the hand-computed values in [MakeVarFontExpect]
// against the variation tables of the fixture at the wght=900/wdth=75 instance.
func TestMakeVarFontExpect(t *testing.T) {
	f := MakeVarFont()
	exp := MakeVarFontExpect()
	glyphs, widths := varGlyphs(f)
	coords := exp.InstanceCoords

	// IUP glyph instanced outline
	resIUP, err := f.Gvar.Apply(glyphs, widths, VarGidIUP, coords, testBudget(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(exp.IUPPoints, simpleContour(t, resIUP.Glyph)); diff != "" {
		t.Errorf("IUP glyph points (-want +got):\n%s", diff)
	}

	// two-tuple glyph instanced outline
	resTwo, err := f.Gvar.Apply(glyphs, widths, VarGidTwo, coords, testBudget(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(exp.TwoPoints, simpleContour(t, resTwo.Glyph)); diff != "" {
		t.Errorf("two-tuple glyph points (-want +got):\n%s", diff)
	}

	// HVAR advances
	rectAdv := uint16(int(widths[VarGidRect]) + int(f.Hvar.AdvanceDelta(VarGidRect, coords)))
	if rectAdv != uint16(exp.RectAdvanceInstance) {
		t.Errorf("Rect HVAR advance = %d, want %d", rectAdv, exp.RectAdvanceInstance)
	}
	compAdv := uint16(int(widths[VarGidComp]) + int(f.Hvar.AdvanceDelta(VarGidComp, coords)))
	if compAdv != uint16(exp.CompAdvanceInstance) {
		t.Errorf("Comp HVAR advance = %d, want %d", compAdv, exp.CompAdvanceInstance)
	}
	if d := f.Hvar.AdvanceDelta(VarGidNotdef, coords); d != 0 {
		t.Errorf("notdef HVAR advance delta = %v, want 0", d)
	}

	// MVAR deltas
	if d, ok := f.Mvar.Delta("hasc", coords); !ok || d != exp.HascDelta {
		t.Errorf("hasc delta = %v (ok=%v), want %v", d, ok, exp.HascDelta)
	}
	if d, ok := f.Mvar.Delta("cpht", coords); !ok || d != exp.CphtDelta {
		t.Errorf("cpht delta = %v (ok=%v), want %v", d, ok, exp.CphtDelta)
	}

	// cvar cvt values
	cvt := f.Outlines.(*glyf.Outlines).Tables["cvt "]
	if got := decodeCVT(cvt); !equalInt16(got, exp.CVTDefault) {
		t.Errorf("default cvt = %v, want %v", got, exp.CVTDefault)
	}
	varied := f.Cvar.Apply(cvt, coords)
	if got := decodeCVT(varied); !equalInt16(got, exp.CVTInstance) {
		t.Errorf("instanced cvt = %v, want %v", got, exp.CVTInstance)
	}

	// FeatureVariations substitution trigger
	rec := &f.Gsub.Variations[0]
	if !rec.Matches(coords) {
		t.Error("FeatureVariations should match at the instance")
	}
	if rec.Matches([]variation.F2Dot14{0, 0}) {
		t.Error("FeatureVariations should not match at the default")
	}
	belowThreshold := []variation.F2Dot14{exp.SubstThreshold - 1, 0}
	if rec.Matches(belowThreshold) {
		t.Error("FeatureVariations should not match just below the threshold")
	}

	// GDEF/GPOS VariationIndex delta
	got := f.Gdef.ItemVarStore.Evaluate(0, 0, coords)
	if got != exp.KernDeltaInstance {
		t.Errorf("GDEF store delta = %v, want %v", got, exp.KernDeltaInstance)
	}
}

func decodeCVT(data []byte) []int16 {
	out := make([]int16, len(data)/2)
	for i := range out {
		out[i] = int16(binary.BigEndian.Uint16(data[2*i:]))
	}
	return out
}

func equalInt16(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
