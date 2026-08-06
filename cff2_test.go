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

package sfnt_test

import (
	"bytes"
	"io"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/vec"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/hvar"
	"seehuhn.de/go/sfnt/internal/testfonts"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// makeCFF2Font builds a minimal, non-variable CFF2 sfnt font for round-trip
// testing.
func makeCFF2Font() *sfnt.Font {
	b := func(v float64) cff.Blend { return cff.Blend{Default: v} }
	box := func(w, h float64) *cff.GlyphCFF2 {
		return &cff.GlyphCFF2{Cmds: []cff.GlyphOpCFF2{
			{Op: cff.OpMoveTo, Args: []cff.Blend{b(0), b(0)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b(w), b(0)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b(w), b(h)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b(0), b(h)}},
		}}
	}

	glyphs := []*cff.GlyphCFF2{box(500, 700), box(600, 650), box(550, 680)}
	o := &cff.OutlinesCFF2{
		Glyphs:   glyphs,
		Widths:   []float64{600, 700, 650},
		Private:  []*cff.PrivateCFF2{{}},
		FDSelect: func(glyph.ID) int { return 0 },
	}
	return &sfnt.Font{
		FamilyName:         "CFF2Test",
		Width:              os2.WidthNormal,
		Weight:             os2.WeightNormal,
		UnitsPerEm:         1000,
		Ascent:             700,
		Descent:            -300,
		LineGap:            100,
		CapHeight:          700,
		XHeight:            500,
		UnderlinePosition:  -100,
		UnderlineThickness: 50,
		FontMatrix:         matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		Outlines:           o,
	}
}

// cff2RoundTripOpts returns cmp options tolerant of the small float differences
// that CFF2 encoding and hmtx quantisation introduce.
func cff2RoundTripOpts(nGlyphs int) []cmp.Option {
	floatClose := func(x, y float64) bool {
		d := math.Abs(x - y)
		m := math.Max(math.Abs(x), math.Abs(y))
		return d <= math.Max(1e-6, m*1e-6)
	}
	return []cmp.Option{
		cmp.Comparer(func(a, b cff.Blend) bool {
			if !floatClose(a.Default, b.Default) {
				return false
			}
			n := max(len(a.Deltas), len(b.Deltas))
			for i := range n {
				var av, bv float64
				if i < len(a.Deltas) {
					av = a.Deltas[i]
				}
				if i < len(b.Deltas) {
					bv = b.Deltas[i]
				}
				if !floatClose(av, bv) {
					return false
				}
			}
			return true
		}),
		cmp.Comparer(floatClose),
		cmp.Comparer(func(fn1, fn2 cff.FDSelectFn) bool {
			for gid := range nGlyphs {
				a, b := 0, 0
				if fn1 != nil {
					a = fn1(glyph.ID(gid))
				}
				if fn2 != nil {
					b = fn2(glyph.ID(gid))
				}
				if a != b {
					return false
				}
			}
			return true
		}),
	}
}

func TestCFF2SfntRoundTrip(t *testing.T) {
	f0 := makeCFF2Font()

	var buf1 bytes.Buffer
	if _, err := f0.Write(&buf1); err != nil {
		t.Fatalf("write 0: %v", err)
	}

	font1, err := sfnt.Read(bytes.NewReader(buf1.Bytes()), parser.NewBudget(int64(buf1.Len())))
	if err != nil {
		t.Fatalf("read 1: %v", err)
	}
	if !font1.IsCFF2() {
		t.Fatal("font is not CFF2 after read")
	}
	if font1.IsCFF() || font1.AsCFF() != nil {
		t.Error("CFF2 font reports CFF outlines")
	}
	if font1.AsCFF2() == nil {
		t.Error("AsCFF2 returned nil for a CFF2 font")
	}

	var buf2 bytes.Buffer
	if _, err := font1.Write(&buf2); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	font2, err := sfnt.Read(bytes.NewReader(buf2.Bytes()), parser.NewBudget(int64(buf2.Len())))
	if err != nil {
		t.Fatalf("read 2: %v", err)
	}

	opts := cff2RoundTripOpts(font1.NumGlyphs())
	if diff := cmp.Diff(font1.Outlines, font2.Outlines, opts...); diff != "" {
		t.Errorf("outlines round trip (-first +second):\n%s", diff)
	}

	// second write is byte-identical
	var buf3 bytes.Buffer
	if _, err := font2.Write(&buf3); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if !bytes.Equal(buf2.Bytes(), buf3.Bytes()) {
		t.Errorf("write not a byte fixpoint: %d vs %d bytes", buf2.Len(), buf3.Len())
	}
}

func TestCFF2WidthsRoundTrip(t *testing.T) {
	f0 := makeCFF2Font()
	var buf bytes.Buffer
	if _, err := f0.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	font, err := sfnt.Read(bytes.NewReader(buf.Bytes()), parser.NewBudget(int64(buf.Len())))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	want := []float64{600, 700, 650}
	got := font.Widths()
	if len(got) != len(want) {
		t.Fatalf("got %d widths, want %d", len(got), len(want))
	}
	for i, w := range want {
		if math.Abs(got[i]-w) > 0.5 {
			t.Errorf("glyph %d: width %v, want %v", i, got[i], w)
		}
		if pw := font.GlyphWidthPDF(glyph.ID(i)); math.Abs(pw-w) > 0.5 {
			t.Errorf("glyph %d: PDF width %v, want %v", i, pw, w)
		}
	}
}

// TestWritePDFRejectsCFF2 verifies the PDF-minimal writers reject CFF2 outlines
// rather than panicking.
func TestWritePDFRejectsCFF2(t *testing.T) {
	f := makeCFF2Font()
	if err := f.WriteOpenTypeCFFPDF(io.Discard); err == nil {
		t.Error("WriteOpenTypeCFFPDF accepted CFF2 outlines")
	}
	if _, err := f.WriteTrueTypePDF(io.Discard); err == nil {
		t.Error("WriteTrueTypePDF accepted CFF2 outlines")
	}
}

// TestCFF2AdobeVF exercises the read path on a real variable CFF2 font.
func TestCFF2AdobeVF(t *testing.T) {
	path := testfonts.Path(t, "AdobeVFPrototype.otf")

	font, err := sfnt.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !font.IsCFF2() {
		t.Fatal("font is not CFF2")
	}
	if !font.IsVariable() {
		t.Fatal("font is not variable")
	}
	if len(font.VariationAxes()) == 0 {
		t.Error("no variation axes")
	}
	if font.Gvar != nil {
		t.Error("gvar must not be read for a CFF2 font")
	}

	// several glyphs yield non-empty default outlines
	nonEmpty := 0
	for gid := 1; gid < font.NumGlyphs() && gid < 200; gid++ {
		nMarks := 0
		for range font.Outlines.Path(glyph.ID(gid)) {
			nMarks++
		}
		if nMarks > 0 {
			nonEmpty++
		}
	}
	if nonEmpty == 0 {
		t.Error("no glyph produced a non-empty outline")
	}

	// some glyph has a sensible PDF advance width
	maxWidth := 0.0
	for gid := range font.NumGlyphs() {
		if w := font.GlyphWidthPDF(glyph.ID(gid)); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth <= 0 {
		t.Errorf("no positive PDF glyph width found")
	}

	// model round-trips through Write/Read
	var buf bytes.Buffer
	if _, err := font.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	font2, err := sfnt.Read(bytes.NewReader(buf.Bytes()), parser.NewBudget(int64(buf.Len())))
	if err != nil {
		t.Fatalf("read 2: %v", err)
	}
	if !font2.IsCFF2() {
		t.Error("round-tripped font is not CFF2")
	}
	if font.NumGlyphs() != font2.NumGlyphs() {
		t.Errorf("glyph count changed: %d -> %d", font.NumGlyphs(), font2.NumGlyphs())
	}
}

// makeVarCFF2Font builds a minimal variable CFF2 sfnt font: one wght axis, a
// box glyph that widens at the +1 end, and an HVAR table that widens its
// advance.  Used by the CFF2 arm of Instantiate and as a FuzzInstantiate seed.
func makeVarCFF2Font() *sfnt.Font {
	f2 := variation.F2Dot14FromFloat
	peak := variation.Region{{Start: f2(0), Peak: f2(1), End: f2(1)}}

	b := func(v float64) cff.Blend { return cff.Blend{Default: v} }
	bv := func(v, d float64) cff.Blend { return cff.Blend{Default: v, Deltas: []float64{d}} }

	notdef := &cff.GlyphCFF2{Cmds: []cff.GlyphOpCFF2{
		{Op: cff.OpMoveTo, Args: []cff.Blend{b(0), b(0)}},
	}}
	// right edge 500 -> 600 at the +1 end
	box := &cff.GlyphCFF2{Cmds: []cff.GlyphOpCFF2{
		{Op: cff.OpMoveTo, Args: []cff.Blend{b(0), b(0)}},
		{Op: cff.OpLineTo, Args: []cff.Blend{bv(500, 100), b(0)}},
		{Op: cff.OpLineTo, Args: []cff.Blend{bv(500, 100), b(700)}},
		{Op: cff.OpLineTo, Args: []cff.Blend{b(0), b(700)}},
	}}

	o := &cff.OutlinesCFF2{
		Glyphs:   []*cff.GlyphCFF2{notdef, box},
		Widths:   []float64{600, 550},
		Private:  []*cff.PrivateCFF2{{}},
		FDSelect: func(glyph.ID) int { return 0 },
		VarStore: &variation.ItemVariationStore{
			Regions: []variation.Region{peak},
			Data: []*variation.ItemVariationData{
				{RegionIndexes: []uint16{0}, Deltas: [][]int32{}},
			},
		},
	}

	font := &sfnt.Font{
		FamilyName:         "CFF2Var",
		Width:              os2.WidthNormal,
		Weight:             os2.WeightNormal,
		UnitsPerEm:         1000,
		Ascent:             700,
		Descent:            -300,
		LineGap:            100,
		CapHeight:          700,
		XHeight:            500,
		UnderlinePosition:  -100,
		UnderlineThickness: 50,
		FontMatrix:         matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		Outlines:           o,
	}
	font.Fvar = &fvar.Table{
		Axes: []fvar.Axis{{Tag: "wght", Min: 100, Default: 400, Max: 900, Name: "Weight"}},
	}
	// advance of the box glyph 550 -> 600 at the +1 end
	font.Hvar = &hvar.Table{
		Store: &variation.ItemVariationStore{
			Regions: []variation.Region{peak},
			Data: []*variation.ItemVariationData{
				{RegionIndexes: []uint16{0}, Deltas: [][]int32{{0}, {50}}},
			},
		},
		AdvanceMap: &variation.DeltaSetIndexMap{Map: []uint32{0, 1}},
	}
	return font
}

// TestInstantiateCFF2 exercises the CFF2 arm of Font.Instantiate on a
// hand-built variable CFF2 font, at the default and at the wght=900 extreme.
func TestInstantiateCFF2(t *testing.T) {
	f := makeVarCFF2Font()

	check := func(t *testing.T, inst *sfnt.Font, wantRightEdge, wantAdvance float64) {
		t.Helper()
		if inst.IsVariable() {
			t.Error("instance is still variable")
		}
		if !inst.IsCFF() || inst.AsCFF() == nil {
			t.Fatal("instance is not CFF")
		}
		if inst.IsCFF2() {
			t.Error("instance still reports CFF2")
		}
		outlines := inst.AsCFF().Outlines
		if !outlines.IsCIDKeyed() {
			t.Error("instanced CFF is not CID-keyed")
		}
		bbox := outlines.Path(1).BBox()
		if math.Abs(bbox.URx-wantRightEdge) > 0.5 {
			t.Errorf("box right edge = %v, want %v", bbox.URx, wantRightEdge)
		}
		if got := outlines.Glyphs[1].Width; math.Abs(got-wantAdvance) > 0.5 {
			t.Errorf("box advance = %v, want %v", got, wantAdvance)
		}

		// write/read round-trip stays a static CFF font
		var buf bytes.Buffer
		if _, err := inst.Write(&buf); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := sfnt.Read(bytes.NewReader(buf.Bytes()), parser.NewBudget(int64(buf.Len())))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if got.IsVariable() {
			t.Error("round-tripped instance is variable")
		}
	}

	t.Run("default", func(t *testing.T) {
		inst, err := f.Instantiate(map[string]float64{})
		if err != nil {
			t.Fatal(err)
		}
		check(t, inst, 500, 550)
	})

	t.Run("wght900", func(t *testing.T) {
		inst, err := f.Instantiate(map[string]float64{"wght": 900})
		if err != nil {
			t.Fatal(err)
		}
		check(t, inst, 600, 600)
	})
}

// makeVarCFF2FontNonUnitMatrix is a variant of makeVarCFF2Font whose font
// matrix scale (0.002) is not 1/UnitsPerEm (1/1000 = 0.001), so design units
// and hmtx/UnitsPerEm units diverge.  It carries a single glyph (the box) and
// an HVAR delta of 100 raw hmtx units at the +1 end, used to check that
// instanceCFF2 converts the HVAR delta into design units before adding it to
// the design-unit base width, rather than assuming the two units coincide.
func makeVarCFF2FontNonUnitMatrix() *sfnt.Font {
	f2 := variation.F2Dot14FromFloat
	peak := variation.Region{{Start: f2(0), Peak: f2(1), End: f2(1)}}

	b := func(v float64) cff.Blend { return cff.Blend{Default: v} }
	box := &cff.GlyphCFF2{Cmds: []cff.GlyphOpCFF2{
		{Op: cff.OpMoveTo, Args: []cff.Blend{b(0), b(0)}},
		{Op: cff.OpLineTo, Args: []cff.Blend{b(500), b(0)}},
		{Op: cff.OpLineTo, Args: []cff.Blend{b(500), b(700)}},
		{Op: cff.OpLineTo, Args: []cff.Blend{b(0), b(700)}},
	}}

	o := &cff.OutlinesCFF2{
		Glyphs:   []*cff.GlyphCFF2{box},
		Widths:   []float64{550}, // design units
		Private:  []*cff.PrivateCFF2{{}},
		FDSelect: func(glyph.ID) int { return 0 },
	}

	font := &sfnt.Font{
		FamilyName: "CFF2VarNonUnit",
		Width:      os2.WidthNormal,
		Weight:     os2.WeightNormal,
		UnitsPerEm: 1000,
		Ascent:     700,
		Descent:    -300,
		// scale 0.002, not 1/UnitsPerEm (0.001): design units and
		// hmtx/UnitsPerEm units are related by a factor of 2, not 1.
		FontMatrix: matrix.Matrix{0.002, 0, 0, 0.002, 0, 0},
		Outlines:   o,
	}
	font.Fvar = &fvar.Table{
		Axes: []fvar.Axis{{Tag: "wght", Min: 100, Default: 400, Max: 900, Name: "Weight"}},
	}
	// HVAR delta at the +1 end: 100 raw hmtx/UnitsPerEm units.
	font.Hvar = &hvar.Table{
		Store: &variation.ItemVariationStore{
			Regions: []variation.Region{peak},
			Data: []*variation.ItemVariationData{
				{RegionIndexes: []uint16{0}, Deltas: [][]int32{{100}}},
			},
		},
	}
	return font
}

// TestInstantiateCFF2HVARUnitConversion checks that instanceCFF2 converts the
// HVAR advance delta into CFF2 design units before combining it with the
// design-unit base width, for a font whose font matrix scale is not
// 1/UnitsPerEm.
//
// Setup: UnitsPerEm = 1000, FontMatrix scale = 0.002 (so
// GlyphAdvanceScale*UnitsPerEm = 0.002*1000 = 2, not 1).  Base width (design
// units) = 550.  HVAR delta at wght=900 (norm 1.0, peaking the only region)
// is 100 raw hmtx/UnitsPerEm units, i.e. 100/2 = 50 design units.  The
// instanced width must therefore be 550 + 50 = 600 design units, not the
// unit-mismatched 550 + 100 = 650 a direct addition would produce.
func TestInstantiateCFF2HVARUnitConversion(t *testing.T) {
	f := makeVarCFF2FontNonUnitMatrix()

	inst, err := f.Instantiate(map[string]float64{"wght": 900})
	if err != nil {
		t.Fatal(err)
	}
	outlines := inst.AsCFF().Outlines
	const want = 600.0
	if got := outlines.Glyphs[0].Width; math.Abs(got-want) > 1e-6 {
		t.Errorf("box advance = %v, want %v (550 design units + 100/2 design units)", got, want)
	}
}

// TestInstantiateCFF2AdobeVF instances the real Adobe variable CFF2 prototype
// at the default and at wght=900, checking the result is a usable static CFF
// font that round-trips.
func TestInstantiateCFF2AdobeVF(t *testing.T) {
	path := testfonts.Path(t, "AdobeVFPrototype.otf")
	f, err := sfnt.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !f.IsCFF2() || !f.IsVariable() {
		t.Fatal("font is not a variable CFF2 font")
	}

	check := func(t *testing.T, coords map[string]float64) {
		t.Helper()
		inst, err := f.Instantiate(coords)
		if err != nil {
			t.Fatal(err)
		}
		if inst.IsVariable() {
			t.Error("instance is still variable")
		}
		if !inst.IsCFF() || inst.AsCFF() == nil {
			t.Fatal("instance is not CFF")
		}
		if inst.Hvar != nil || inst.Mvar != nil || inst.Fvar != nil {
			t.Error("instance retains variation tables")
		}

		nonEmpty, maxWidth := 0, 0.0
		for gid := range inst.NumGlyphs() {
			for range inst.Outlines.Path(glyph.ID(gid)) {
				nonEmpty++
				break
			}
			if w := inst.GlyphWidthPDF(glyph.ID(gid)); w > maxWidth {
				maxWidth = w
			}
		}
		if nonEmpty == 0 {
			t.Error("no glyph produced a non-empty outline")
		}
		if maxWidth <= 0 {
			t.Error("no positive PDF glyph width")
		}

		var buf bytes.Buffer
		if _, err := inst.Write(&buf); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := sfnt.Read(bytes.NewReader(buf.Bytes()), parser.NewBudget(int64(buf.Len())))
		if err != nil {
			t.Fatalf("re-read: %v", err)
		}
		if got.IsVariable() {
			t.Error("round-tripped instance is variable")
		}
		if !got.IsCFF() {
			t.Error("round-tripped instance is not CFF")
		}
	}

	t.Run("defaults", func(t *testing.T) { check(t, map[string]float64{}) })
	t.Run("wght900", func(t *testing.T) { check(t, map[string]float64{"wght": 900}) })
}

// pathStep is a materialised path.Path step, for use with cmp.Diff.
type pathStep struct {
	Cmd path.Command
	Pts []vec.Vec2
}

func collectPathSteps(p path.Path) []pathStep {
	var steps []pathStep
	for cmd, pts := range p {
		steps = append(steps, pathStep{Cmd: cmd, Pts: append([]vec.Vec2(nil), pts...)})
	}
	return steps
}

// TestConvertCFF2Static checks the static (non-variable) arm of ConvertCFF2:
// the outlines convert to CFF, the resulting paths match the CFF2 default
// instance exactly (no blends to evaluate), and the result round-trips.
func TestConvertCFF2Static(t *testing.T) {
	f := makeCFF2Font()
	orig := f.Outlines.(*cff.OutlinesCFF2)

	out, err := f.ConvertCFF2()
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsCFF() || out.AsCFF() == nil {
		t.Fatal("converted font is not CFF")
	}
	if out.IsCFF2() {
		t.Error("converted font still reports CFF2")
	}
	if got, want := out.FontName, f.FontName; got != want {
		t.Errorf("PostScriptName = %q, want %q (no axes pinned)", got, want)
	}

	converted := out.AsCFF().Outlines
	for gid := range orig.Glyphs {
		want := collectPathSteps(orig.Path(glyph.ID(gid)))
		got := collectPathSteps(converted.Path(glyph.ID(gid)))
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("gid %d path differs from the CFF2 default path (-want +got):\n%s", gid, diff)
		}
	}

	var buf bytes.Buffer
	if _, err := out.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	got2, err := sfnt.Read(bytes.NewReader(buf.Bytes()), parser.NewBudget(int64(buf.Len())))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !got2.IsCFF() {
		t.Error("round-tripped font is not CFF")
	}

	// the receiver is left unchanged
	if !f.IsCFF2() {
		t.Error("receiver no longer reports CFF2 outlines")
	}
}

// TestConvertCFF2Variable checks that the variable arm of ConvertCFF2 is
// exactly Instantiate(nil): all axes pinned at their default values.
func TestConvertCFF2Variable(t *testing.T) {
	f := makeVarCFF2Font()
	nGlyphs := len(f.Outlines.(*cff.OutlinesCFF2).Glyphs)

	got, err := f.ConvertCFF2()
	if err != nil {
		t.Fatal(err)
	}
	want, err := f.Instantiate(nil)
	if err != nil {
		t.Fatal(err)
	}

	opts := append(cff2RoundTripOpts(nGlyphs), cmpopts.IgnoreUnexported(sfnt.Font{}))
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Errorf("ConvertCFF2() != Instantiate(nil) (-want +got):\n%s", diff)
	}
}

// TestConvertCFF2NotCFF2 checks that ConvertCFF2 rejects fonts that do not
// use CFF2 outlines.
func TestConvertCFF2NotCFF2(t *testing.T) {
	f := &sfnt.Font{Outlines: &glyf.Outlines{}}
	if _, err := f.ConvertCFF2(); err == nil {
		t.Error("expected an error for glyf outlines")
	}
}
