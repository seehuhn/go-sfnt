// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/mathtable"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

func readGoRegular(t *testing.T) *sfnt.Font {
	t.Helper()
	f, err := sfnt.Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func writeAndRead(t *testing.T, f *sfnt.Font) *sfnt.Font {
	t.Helper()
	buf := &bytes.Buffer{}
	if _, err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	f2, err := sfnt.Read(bytes.NewReader(buf.Bytes()), parser.NewBudget(int64(buf.Len())))
	if err != nil {
		t.Fatal(err)
	}
	return f2
}

// testMath returns a MATH table for a font with at least 20 glyphs.
func testMath() *mathtable.Info {
	info := &mathtable.Info{
		ItalicsCorrection:   map[glyph.ID]mathtable.Value{4: {V: 30}},
		TopAccent:           map[glyph.ID]mathtable.Value{4: {V: 250}},
		ExtendedShapes:      map[glyph.ID]bool{10: true},
		MinConnectorOverlap: 20,
		Kerns: map[glyph.ID]*mathtable.KernInfo{
			4: {TopRight: &mathtable.Kern{
				Heights: []mathtable.Value{{V: 400}},
				Kerns:   []mathtable.Value{{V: -50}, {V: 10}},
			}},
		},
		Vertical: map[glyph.ID]*mathtable.Construction{
			5: {
				Variants: []mathtable.Variant{{Glyph: 5, Advance: 700}, {Glyph: 6, Advance: 900}, {Glyph: 7, Advance: 1200}},
				Assembly: &mathtable.Assembly{
					Parts: []mathtable.Part{
						{Glyph: 8, EndConnector: 100, FullAdvance: 500},
						{Glyph: 9, StartConnector: 100, EndConnector: 100, FullAdvance: 300, Extender: true},
						{Glyph: 10, StartConnector: 100, FullAdvance: 500},
					},
				},
			},
		},
		Horizontal: map[glyph.ID]*mathtable.Construction{
			11: {Variants: []mathtable.Variant{{Glyph: 11, Advance: 600}, {Glyph: 12, Advance: 1000}}},
		},
	}
	info.Constants.ScriptPercentScaleDown = 70
	info.Constants.ScriptScriptPercentScaleDown = 50
	info.Constants.AxisHeight = mathtable.Value{V: 250}
	info.Constants.FractionRuleThickness = mathtable.Value{V: 40}
	return info
}

// TestMathRoundTrip checks that a font keeps its MATH table when written
// and read back.
func TestMathRoundTrip(t *testing.T) {
	f := readGoRegular(t)
	f.Math = testMath()

	f2 := writeAndRead(t, f)
	if diff := cmp.Diff(f.Math, f2.Math); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestMathReadDropsMissingGlyphs checks that MATH data for glyphs beyond
// the end of the font is dropped on read, and that the result is stable.
func TestMathReadDropsMissingGlyphs(t *testing.T) {
	f := readGoRegular(t)
	n := glyph.ID(f.NumGlyphs())
	info := testMath()
	info.ItalicsCorrection[n] = mathtable.Value{V: 1}
	info.Vertical[5].Variants = append(info.Vertical[5].Variants, mathtable.Variant{Glyph: n + 1, Advance: 2000})
	info.Horizontal[n+2] = &mathtable.Construction{Variants: []mathtable.Variant{{Glyph: 11, Advance: 600}}}
	f.Math = info

	f2 := writeAndRead(t, f)
	for gid := range f2.Math.ItalicsCorrection {
		if gid >= n {
			t.Errorf("italics correction for missing glyph %d kept", gid)
		}
	}
	for gid, c := range f2.Math.Horizontal {
		if gid >= n {
			t.Errorf("construction for missing glyph %d kept", gid)
		}
		for _, v := range c.Variants {
			if v.Glyph >= n {
				t.Errorf("variant %d of glyph %d kept", v.Glyph, gid)
			}
		}
	}
	for _, v := range f2.Math.Vertical[5].Variants {
		if v.Glyph >= n {
			t.Errorf("variant %d of glyph 5 kept", v.Glyph)
		}
	}

	f3 := writeAndRead(t, f2)
	if diff := cmp.Diff(f2.Math, f3.Math); diff != "" {
		t.Errorf("read-write-read not stable (-want +got):\n%s", diff)
	}
}

// TestMathSubset checks that subsetting keeps the size variants and
// assembly parts of the selected glyphs, with renumbered glyph IDs.
func TestMathSubset(t *testing.T) {
	f := readGoRegular(t)
	f.Math = testMath()

	sub, err := f.Subset([]glyph.ID{0, 5})
	if err != nil {
		t.Fatal(err)
	}
	// glyph 5 with variants 6, 7 and parts 8, 9, 10
	if got := sub.NumGlyphs(); got != 7 {
		t.Errorf("subset has %d glyphs, want 7", got)
	}

	old := f.Math.Vertical[5]
	c := sub.Math.Vertical[1]
	if c == nil || len(c.Variants) != len(old.Variants) || c.Assembly == nil ||
		len(c.Assembly.Parts) != len(old.Assembly.Parts) {
		t.Fatalf("construction not carried over: %+v", c)
	}
	for i, v := range c.Variants {
		if v.Advance != old.Variants[i].Advance {
			t.Errorf("variant %d: advance %d, want %d", i, v.Advance, old.Variants[i].Advance)
		}
		if sub.GlyphWidth(v.Glyph) != f.GlyphWidth(old.Variants[i].Glyph) {
			t.Errorf("variant %d refers to the wrong glyph", i)
		}
	}
	for i, p := range c.Assembly.Parts {
		if sub.GlyphWidth(p.Glyph) != f.GlyphWidth(old.Assembly.Parts[i].Glyph) {
			t.Errorf("part %d refers to the wrong glyph", i)
		}
	}
	// glyph 10 is an extended shape and an assembly part
	if len(sub.Math.ExtendedShapes) != 1 {
		t.Errorf("got %d extended shapes, want 1", len(sub.Math.ExtendedShapes))
	}
	// glyph 4 and 11 are not in the subset
	if sub.Math.ItalicsCorrection != nil || sub.Math.Kerns != nil || sub.Math.Horizontal != nil {
		t.Error("data for glyphs outside the subset kept")
	}

	sub2 := writeAndRead(t, sub)
	if diff := cmp.Diff(sub.Math, sub2.Math); diff != "" {
		t.Errorf("round trip of subset failed (-want +got):\n%s", diff)
	}
}

// TestMathInstantiate checks that instancing a variable font resolves the
// VariationIndex tables of the MATH table.
func TestMathInstantiate(t *testing.T) {
	f := debug.MakeVarFont()

	// rebuild the GDEF store, keeping the row used by GPOS at (0, 0)
	f2 := variation.F2Dot14FromFloat
	regionWght := variation.Region{{Start: 0, Peak: f2(1), End: f2(1)}, {}}
	regionWdth := variation.Region{{}, {Start: f2(-1), Peak: f2(-1), End: 0}}
	b := variation.NewStoreBuilder()
	outer, inner, err := b.Add([]variation.Region{regionWght, regionWdth}, []int32{25, -10})
	if err != nil {
		t.Fatal(err)
	}
	if outer != 0 || inner != 0 {
		t.Fatalf("first row at (%d, %d), want (0, 0)", outer, inner)
	}
	outer, inner, err = b.Add([]variation.Region{regionWght}, []int32{100})
	if err != nil {
		t.Fatal(err)
	}
	f.Gdef.ItemVarStore = b.Build()
	vi := &device.Table{OuterIndex: outer, InnerIndex: inner, DeltaFormat: device.VariationIndexFormat}

	info := &mathtable.Info{
		Vertical: map[glyph.ID]*mathtable.Construction{
			debug.VarGidRect: {
				Variants: []mathtable.Variant{{Glyph: debug.VarGidTwo, Advance: 900}},
				Assembly: &mathtable.Assembly{
					ItalicsCorrection: mathtable.Value{V: 10, Device: vi},
					Parts:             []mathtable.Part{{Glyph: debug.VarGidIUP, FullAdvance: 700}},
				},
			},
		},
	}
	info.Constants.AxisHeight = mathtable.Value{V: 250, Device: vi}
	f.Math = info

	// the variable font itself round-trips with its VariationIndex tables
	fv := writeAndRead(t, f)
	if diff := cmp.Diff(f.Math, fv.Math); diff != "" {
		t.Errorf("variable font round trip failed (-want +got):\n%s", diff)
	}

	cases := []struct {
		wght       float64
		axisHeight funit.Int16
		italics    funit.Int16
	}{
		{400, 250, 10},
		{900, 350, 110},
	}
	for _, c := range cases {
		inst, err := f.Instantiate(map[string]float64{"wght": c.wght})
		if err != nil {
			t.Fatal(err)
		}
		got := inst.Math.Constants.AxisHeight
		if got.V != c.axisHeight || got.Device != nil {
			t.Errorf("wght=%g: AxisHeight = %+v, want %d", c.wght, got, c.axisHeight)
		}
		got = inst.Math.Vertical[debug.VarGidRect].Assembly.ItalicsCorrection
		if got.V != c.italics || got.Device != nil {
			t.Errorf("wght=%g: italics correction = %+v, want %d", c.wght, got, c.italics)
		}
		writeAndRead(t, inst)
	}
}

// TestMathWriteMissingVariation checks that a font is not written when a
// MATH VariationIndex table has no variation data to refer to.
func TestMathWriteMissingVariation(t *testing.T) {
	f := readGoRegular(t)
	f.Gdef = nil
	f.Math = testMath()
	f.Math.Constants.AxisHeight.Device = &device.Table{DeltaFormat: device.VariationIndexFormat}

	if _, err := f.Write(&bytes.Buffer{}); err == nil {
		t.Error("font with dangling VariationIndex written")
	}
}

// withRawMath returns the encoding of f with its "MATH" table replaced by
// the encoding of info.  This bypasses the checks of [sfnt.Font.Write].
func withRawMath(t *testing.T, f *sfnt.Font, info *mathtable.Info) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	if _, err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(buf.Bytes())
	dir, err := header.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]byte, len(dir.Toc)+1)
	for name := range dir.Toc {
		tables[name], err = dir.ReadTableBytes(r, name)
		if err != nil {
			t.Fatal(err)
		}
	}
	tables["MATH"], err = info.Encode()
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	if _, err := header.Write(out, dir.ScalerType, tables); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestMathReadDropsMissingVariation checks that MATH VariationIndex tables
// which refer to missing variation data are dropped on read, and that the
// result is stable.
func TestMathReadDropsMissingVariation(t *testing.T) {
	noGdef := readGoRegular(t)
	noGdef.Gdef = nil
	shortStore := debug.MakeVarFont()
	cases := []struct {
		name string
		f    *sfnt.Font
	}{
		{"no GDEF", noGdef},
		{"missing row", shortStore},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := testMath()
			vi := &device.Table{OuterIndex: 0, InnerIndex: 0xFFF0, DeltaFormat: device.VariationIndexFormat}
			info.Constants.AxisHeight.Device = vi
			info.ItalicsCorrection[4] = mathtable.Value{V: 30, Device: vi}

			data := withRawMath(t, c.f, info)
			f2, err := sfnt.Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			if err != nil {
				t.Fatal(err)
			}
			if f2.Math == nil {
				t.Fatal("MATH table dropped")
			}
			if d := f2.Math.Constants.AxisHeight.Device; d != nil {
				t.Errorf("dangling VariationIndex kept: %+v", d)
			}
			if d := f2.Math.ItalicsCorrection[4].Device; d != nil {
				t.Errorf("dangling VariationIndex kept: %+v", d)
			}

			f3 := writeAndRead(t, f2)
			if diff := cmp.Diff(f2.Math, f3.Math); diff != "" {
				t.Errorf("read-write-read not stable (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMathSubsetNested checks that subsetting follows chains of size
// variants: the variants of a variant are kept, too.
func TestMathSubsetNested(t *testing.T) {
	f := readGoRegular(t)
	info := testMath()
	// glyph 6 is a variant of glyph 5, and has variants of its own
	info.Vertical[6] = &mathtable.Construction{
		Variants: []mathtable.Variant{{Glyph: 13, Advance: 950}},
		Assembly: &mathtable.Assembly{Parts: []mathtable.Part{{Glyph: 14, FullAdvance: 400}}},
	}
	f.Math = info

	sub, err := f.Subset([]glyph.ID{0, 5})
	if err != nil {
		t.Fatal(err)
	}
	// glyph 5, variants 6, 7, parts 8, 9, 10, and from glyph 6: 13, 14
	if got := sub.NumGlyphs(); got != 9 {
		t.Errorf("subset has %d glyphs, want 9", got)
	}

	c := sub.Math.Vertical[sub.Math.Vertical[1].Variants[1].Glyph]
	if c == nil || len(c.Variants) != 1 || c.Assembly == nil {
		t.Fatalf("construction of the variant not carried over: %+v", c)
	}
	if sub.GlyphWidth(c.Variants[0].Glyph) != f.GlyphWidth(13) {
		t.Error("nested variant refers to the wrong glyph")
	}
	if sub.GlyphWidth(c.Assembly.Parts[0].Glyph) != f.GlyphWidth(14) {
		t.Error("nested assembly part refers to the wrong glyph")
	}

	sub2 := writeAndRead(t, sub)
	if diff := cmp.Diff(sub.Math, sub2.Math); diff != "" {
		t.Errorf("round trip of subset failed (-want +got):\n%s", diff)
	}
}
