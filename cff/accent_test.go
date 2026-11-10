// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cff

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/psenc"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

func accentTestGlyphs() []*Glyph {
	notdef := &Glyph{Name: ".notdef", Width: 100}

	a := &Glyph{Name: "A", Width: 600}
	a.MoveTo(0, 0)
	a.LineTo(300, 700)
	a.LineTo(600, 0)

	acute := &Glyph{Name: "acute", Width: 200}
	acute.MoveTo(10, 800)
	acute.CurveTo(20, 810, 30, 820, 40, 830)

	aacute := &Glyph{Name: "Aacute", Width: 650}

	return []*Glyph{notdef, a, acute, aacute}
}

var aacuteRef = &accentRef{dx: 100, dy: 50,
	base: psenc.StandardEncodingRev["A"], accent: psenc.StandardEncodingRev["acute"]}

func TestResolveAccents(t *testing.T) {
	glyphs := accentTestGlyphs()
	refs := map[int]*accentRef{3: aacuteRef}
	if err := resolveAccents(glyphs, refs, parser.NewBudget(0)); err != nil {
		t.Fatal(err)
	}

	want := &Glyph{Name: "Aacute", Width: 650}
	want.MoveTo(0, 0)
	want.LineTo(300, 700)
	want.LineTo(600, 0)
	want.MoveTo(110, 850)
	want.CurveTo(120, 860, 130, 870, 140, 880)
	if diff := cmp.Diff(want, glyphs[3]); diff != "" {
		t.Errorf("composition mismatch (-want +got):\n%s", diff)
	}

	// the components are unchanged
	orig := accentTestGlyphs()
	if diff := cmp.Diff(orig[:3], glyphs[:3]); diff != "" {
		t.Errorf("components modified (-want +got):\n%s", diff)
	}
}

func TestResolveAccentsMissing(t *testing.T) {
	cases := map[string]*accentRef{
		"missing accent": {base: 65, accent: psenc.StandardEncodingRev["grave"]},
		"missing base":   {base: psenc.StandardEncodingRev["B"], accent: 194},
		"unused code":    {base: 0, accent: 194},
	}
	for name, ref := range cases {
		t.Run(name, func(t *testing.T) {
			glyphs := accentTestGlyphs()
			if err := resolveAccents(glyphs, map[int]*accentRef{3: ref}, parser.NewBudget(0)); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(accentTestGlyphs(), glyphs); diff != "" {
				t.Errorf("glyphs changed (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveAccentsOutOfRange(t *testing.T) {
	glyphs := accentTestGlyphs()
	ref := *aacuteRef
	ref.dx = 40000 // shifted accent exceeds the encodable range
	if err := resolveAccents(glyphs, map[int]*accentRef{3: &ref}, parser.NewBudget(0)); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(accentTestGlyphs(), glyphs); diff != "" {
		t.Errorf("glyphs changed (-want +got):\n%s", diff)
	}
}

func TestResolveAccentsNested(t *testing.T) {
	// glyph 3 uses glyph 4 as its base, and glyph 4 is itself a composite
	glyphs := accentTestGlyphs()
	glyphs = append(glyphs, &Glyph{Name: "B", Width: 600})
	ref3 := &accentRef{dx: 100, dy: 50,
		base: psenc.StandardEncodingRev["B"], accent: psenc.StandardEncodingRev["acute"]}
	ref4 := &accentRef{dx: 5, dy: 5,
		base: psenc.StandardEncodingRev["A"], accent: psenc.StandardEncodingRev["acute"]}
	refs := map[int]*accentRef{3: ref3, 4: ref4}
	if err := resolveAccents(glyphs, refs, parser.NewBudget(0)); err != nil {
		t.Fatal(err)
	}

	// glyph 3 is composed from the empty decode of glyph 4
	want := &Glyph{Name: "Aacute", Width: 650}
	want.MoveTo(110, 850)
	want.CurveTo(120, 860, 130, 870, 140, 880)
	if diff := cmp.Diff(want, glyphs[3]); diff != "" {
		t.Errorf("nested composition mismatch (-want +got):\n%s", diff)
	}
}

func TestResolveAccentsHints(t *testing.T) {
	mk := func(compHStem []float64) []*Glyph {
		glyphs := accentTestGlyphs()
		glyphs[1].HStem = []float64{0, 20}
		glyphs[1].VStem = []float64{10, 30}
		glyphs[1].Cmds = append([]GlyphOp{{Op: OpHintMask, Args: []float64{0xC0}}}, glyphs[1].Cmds...)
		glyphs[2].Cmds = append([]GlyphOp{{Op: OpHintMask, Args: []float64{0x80}}}, glyphs[2].Cmds...)
		glyphs[3].HStem = compHStem
		return glyphs
	}

	// base hints are copied if the composite has none
	glyphs := mk(nil)
	if err := resolveAccents(glyphs, map[int]*accentRef{3: aacuteRef}, parser.NewBudget(0)); err != nil {
		t.Fatal(err)
	}
	g := glyphs[3]
	if diff := cmp.Diff([]float64{0, 20}, g.HStem); diff != "" {
		t.Errorf("hstem mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]float64{10, 30}, g.VStem); diff != "" {
		t.Errorf("vstem mismatch (-want +got):\n%s", diff)
	}
	var masks []GlyphOp
	for _, cmd := range g.Cmds {
		if isMaskOp(cmd.Op) {
			masks = append(masks, cmd)
		}
	}
	if diff := cmp.Diff([]GlyphOp{{Op: OpHintMask, Args: []float64{0xC0}}}, masks); diff != "" {
		t.Errorf("masks mismatch (-want +got):\n%s", diff)
	}
	glyphs[1].HStem[0] = 99
	if g.HStem[0] != 0 {
		t.Errorf("hints share memory with the base")
	}

	// the composite's own hints are kept
	glyphs = mk([]float64{5, 15})
	if err := resolveAccents(glyphs, map[int]*accentRef{3: aacuteRef}, parser.NewBudget(0)); err != nil {
		t.Fatal(err)
	}
	g = glyphs[3]
	if diff := cmp.Diff([]float64{5, 15}, g.HStem); diff != "" {
		t.Errorf("hstem mismatch (-want +got):\n%s", diff)
	}
	if len(g.VStem) != 0 {
		t.Errorf("unexpected vstem %v", g.VStem)
	}
	for _, cmd := range g.Cmds {
		if isMaskOp(cmd.Op) {
			t.Errorf("unexpected hint mask in composite with own stems")
		}
	}
}

func TestResolveAccentsBudget(t *testing.T) {
	glyphs := accentTestGlyphs()
	err := resolveAccents(glyphs, map[int]*accentRef{3: aacuteRef}, membudget.New(8))
	if !errors.Is(err, membudget.ErrExceeded) {
		t.Errorf("got %v, want ErrExceeded", err)
	}
}

// TestReadAccent writes a font in which one glyph is a four-argument endchar
// composite, and checks that reading gives the composed outline, which then
// survives a write-read cycle.
func TestReadAccent(t *testing.T) {
	glyphs := accentTestGlyphs()
	f := &Font{
		FontInfo: &type1.FontInfo{
			FontName:   "Test-Regular",
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		},
		Outlines: &Outlines{
			Glyphs: glyphs,
			Private: []*type1.PrivateDict{{
				BlueScale: type1.DefaultBlueScale,
				BlueShift: type1.DefaultBlueShift,
				BlueFuzz:  type1.DefaultBlueFuzz,
			}},
			FDSelect: fdSelectSimple,
			Encoding: make([]glyph.ID, 256),
		},
	}
	charStrings, defWidths, nomWidths, err := f.encodeCharStrings()
	if err != nil {
		t.Fatal(err)
	}
	// no width operand, so the glyph gets the default width
	charStrings[3] = cs(10, 20, int(aacuteRef.base), int(aacuteRef.accent), t2endchar)
	buf := &bytes.Buffer{}
	if err := f.writeCharStrings(buf, charStrings, defWidths, nomWidths); err != nil {
		t.Fatal(err)
	}

	f1, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(int64(10*buf.Len())))
	if err != nil {
		t.Fatal(err)
	}

	want := &Glyph{Name: "Aacute", Width: defWidths[0]}
	want.MoveTo(0, 0)
	want.LineTo(300, 700)
	want.LineTo(600, 0)
	want.MoveTo(20, 820)
	want.CurveTo(30, 830, 40, 840, 50, 850)
	if diff := cmp.Diff(want, f1.Glyphs[3]); diff != "" {
		t.Errorf("composed glyph mismatch (-want +got):\n%s", diff)
	}

	f2 := writeRead(t, f1)
	if diff := cmp.Diff(f1.Glyphs, f2.Glyphs); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}
