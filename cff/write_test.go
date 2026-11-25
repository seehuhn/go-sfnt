// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cff

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/geom/matrix"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// makeLargeFont returns a name-keyed font with n glyphs.  All glyph names
// except ".notdef" are non-standard, so each of them needs its own SID.
func makeLargeFont(n int) *Font {
	glyphs := make([]*Glyph, n)
	for i := range glyphs {
		g := &Glyph{Name: fmt.Sprintf("uni%05X", i), Width: 100}
		g.MoveTo(0, 0)
		g.LineTo(10, 0)
		glyphs[i] = g
	}
	if n > 0 {
		glyphs[0] = &Glyph{Name: ".notdef", Width: 100}
	}

	return &Font{
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
}

// TestWriteTooManyGlyphs checks that fonts which exceed the format limits are
// rejected instead of being written in truncated form.
func TestWriteTooManyGlyphs(t *testing.T) {
	// The CharStrings INDEX stores the glyph count in a 16-bit field.
	f := makeLargeFont(maxGlyphs + 1)
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}

	// Every glyph name needs a SID, and SIDs above maxSID are reserved.
	f = makeLargeFont(maxGlyphs)
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteNameKeyedPrivateCount checks that name-keyed fonts must have
// exactly one private dictionary.  The Top DICT holds a single Private
// entry, so any further dictionaries would be written but never referenced,
// and reading the font back would silently lose them.
func TestWriteNameKeyedPrivateCount(t *testing.T) {
	f := makeLargeFont(3)
	f.Private = append(f.Private, &type1.PrivateDict{
		BlueScale: type1.DefaultBlueScale,
		BlueShift: type1.DefaultBlueShift,
		BlueFuzz:  type1.DefaultBlueFuzz,
	})
	if err := f.Write(io.Discard); err == nil {
		t.Error("font with two private dicts: expected an error")
	}

	f.Private = nil
	if err := f.Write(io.Discard); err == nil {
		t.Error("font with no private dict: expected an error")
	}
}

// makeManyFDFont returns a CID-keyed font with numFDs Font DICTs, using a
// separate Font DICT for each glyph.
func makeManyFDFont(numFDs int) *Font {
	glyphs := make([]*Glyph, numFDs)
	gidToCID := make([]cid.CID, numFDs)
	private := make([]*type1.PrivateDict, numFDs)
	fontMatrices := make([]matrix.Matrix, numFDs)
	for i := range glyphs {
		g := &Glyph{Width: 100}
		g.MoveTo(0, 0)
		g.LineTo(10, 0)
		glyphs[i] = g
		gidToCID[i] = cid.CID(i)
		private[i] = &type1.PrivateDict{
			BlueScale: type1.DefaultBlueScale,
			BlueShift: type1.DefaultBlueShift,
			BlueFuzz:  type1.DefaultBlueFuzz,
		}
		fontMatrices[i] = matrix.Identity
	}

	return &Font{
		FontInfo: &type1.FontInfo{
			FontName:   "Test-Regular",
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		},
		Outlines: &Outlines{
			Glyphs:       glyphs,
			Private:      private,
			FontMatrices: fontMatrices,
			FDSelect:     func(gid glyph.ID) int { return int(gid) },
			ROS: &cid.SystemInfo{
				Registry: "Adobe",
				Ordering: "Identity",
			},
			GIDToCID: gidToCID,
		},
	}
}

// TestWriteTooManyFontDICTs checks that fonts with more Font DICTs than
// FDSelect can address are rejected.  Without the check the Font DICT index
// would be truncated to eight bits and the font could not be read back.
func TestWriteTooManyFontDICTs(t *testing.T) {
	f := makeManyFDFont(maxFontDICTs)
	buf := &bytes.Buffer{}
	if err := f.Write(buf); err != nil {
		t.Fatalf("largest allowed font rejected: %v", err)
	}
	out, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<26))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for gid := range maxFontDICTs {
		if fd := out.FDSelect(glyph.ID(gid)); fd != gid {
			t.Fatalf("FDSelect(%d) = %d, want %d", gid, fd, gid)
		}
	}

	f = makeManyFDFont(maxFontDICTs + 1)
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteFDSelectRange checks that Font DICT indices returned by FDSelect
// are validated when the font is written.  Without the check, out-of-range
// indices would be truncated to eight bits and Read would reject the output.
func TestWriteFDSelectRange(t *testing.T) {
	for _, bad := range []int{-1, 2, 300} {
		f := makeManyFDFont(2)
		f.FDSelect = func(glyph.ID) int { return bad }
		if err := f.Write(io.Discard); err == nil {
			t.Errorf("FDSelect returning %d: expected an error", bad)
		}
	}
}

// TestSelectWidthsUniform checks the width heuristic for fonts in which
// every glyph has the same advance width.  No glyph then encodes a width
// relative to nominalWidthX, but the value is still written to the private
// dict and must be a valid number.
func TestSelectWidthsUniform(t *testing.T) {
	f := makeLargeFont(3)
	def, nom, err := selectWidths(f.Glyphs)
	if err != nil {
		t.Fatal(err)
	}
	if def != 100 {
		t.Errorf("got default width %v, want 100", def)
	}
	if math.IsInf(nom, 0) || math.IsNaN(nom) {
		t.Errorf("got nominal width %v, want a finite number", nom)
	}
}

// TestWriteFractionalUnderline checks that fractional underline metrics
// survive a write-read cycle.  The DICT encoding supports real numbers, so
// the values must not be rounded to integers.
func TestWriteFractionalUnderline(t *testing.T) {
	f := makeLargeFont(2)
	f.FontInfo.UnderlinePosition = -100.5
	f.FontInfo.UnderlineThickness = 42.25

	buf := &bytes.Buffer{}
	if err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	out, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<26))
	if err != nil {
		t.Fatal(err)
	}

	if got := out.FontInfo.UnderlinePosition; got != -100.5 {
		t.Errorf("got underline position %v, want -100.5", got)
	}
	if got := out.FontInfo.UnderlineThickness; got != 42.25 {
		t.Errorf("got underline thickness %v, want 42.25", got)
	}
}

// TestWriteCharStringLength checks that glyphs whose charstring exceeds the
// 65535-byte limit of Type 2 charstring interpreters are rejected.
func TestWriteCharStringLength(t *testing.T) {
	f := makeLargeFont(1)
	g := f.Glyphs[0]
	g.MoveTo(0, 0)
	for i := range 40000 {
		g.LineTo(float64(i%100), float64(i%37))
	}
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteCFF2CharStringLength checks the corresponding limit for CFF2.
func TestWriteCFF2CharStringLength(t *testing.T) {
	cmds := make([]GlyphOpCFF2, 0, 40001)
	cmds = append(cmds, GlyphOpCFF2{Op: OpMoveTo, Args: blendArgs(0, 0)})
	for i := range 40000 {
		x := float64(i % 100)
		y := float64(i % 37)
		cmds = append(cmds, GlyphOpCFF2{Op: OpLineTo, Args: blendArgs(x, y)})
	}

	f := &FontCFF2{
		FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		OutlinesCFF2: &OutlinesCFF2{
			Glyphs:  []*GlyphCFF2{{Cmds: cmds}},
			Private: []*PrivateCFF2{{}},
		},
	}
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteCFF2FDSelectRange checks that Font DICT indices returned by
// FDSelect are validated when a CFF2 font is written.  Without the check the
// FDSelect table would contain indices which do not match the private dicts
// used for encoding the charstrings.
func TestWriteCFF2FDSelectRange(t *testing.T) {
	for _, bad := range []int{-1, 2, 300} {
		f := &FontCFF2{
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
			OutlinesCFF2: &OutlinesCFF2{
				Glyphs:   []*GlyphCFF2{{}, {}},
				Private:  []*PrivateCFF2{{}, {}},
				FDSelect: func(glyph.ID) int { return bad },
			},
		}
		if err := f.Write(io.Discard); err == nil {
			t.Errorf("FDSelect returning %d: expected an error", bad)
		}
	}
}

// TestWriteCFF2TooManyGlyphs checks that a CFF2 font with more glyphs than
// the maxp table can count is refused.  ReadCFF2 rejects such fonts, so
// writing them would break the round trip.
func TestWriteCFF2TooManyGlyphs(t *testing.T) {
	glyphs := make([]*GlyphCFF2, maxGlyphs+1)
	for i := range glyphs {
		glyphs[i] = &GlyphCFF2{}
	}
	f := &FontCFF2{
		FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		OutlinesCFF2: &OutlinesCFF2{
			Glyphs:   glyphs,
			Private:  []*PrivateCFF2{{}},
			FDSelect: func(glyph.ID) int { return 0 },
		},
	}
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteCFF2TooManyFontDICTs checks the corresponding limit for CFF2.
// ReadCFF2 rejects such fonts, so writing them would break the round trip.
func TestWriteCFF2TooManyFontDICTs(t *testing.T) {
	const numFDs = maxFontDICTs + 1
	glyphs := make([]*GlyphCFF2, numFDs)
	private := make([]*PrivateCFF2, numFDs)
	for i := range glyphs {
		glyphs[i] = &GlyphCFF2{}
		private[i] = &PrivateCFF2{}
	}

	f := &FontCFF2{
		FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		OutlinesCFF2: &OutlinesCFF2{
			Glyphs:   glyphs,
			Private:  private,
			FDSelect: func(gid glyph.ID) int { return int(gid) },
		},
	}
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// fontWithMatrices returns a font with two glyphs and the given font
// matrices.  If fds is nil the font is name-keyed, otherwise it is CID-keyed
// with glyph i using FD i.
func fontWithMatrices(top matrix.Matrix, fds []matrix.Matrix) *Font {
	f := makeLargeFont(2)
	f.FontInfo.FontMatrix = top
	if fds != nil {
		f.Glyphs[1].Name = ""
		f.Private = []*type1.PrivateDict{{}, {}}
		f.FDSelect = func(gid glyph.ID) int { return int(gid) }
		f.FontMatrices = fds
		f.ROS = &cid.SystemInfo{Registry: "Test", Ordering: "Test"}
		f.GIDToCID = []cid.CID{0, 1}
		f.Encoding = nil
	}
	return f
}

// writeReadMalformed writes f without checking its font matrices and reads it
// back, for testing the repair of font files.
func writeReadMalformed(t *testing.T, f *Font) *Font {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := f.write(buf); err != nil {
		t.Fatal(err)
	}
	res, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(int64(10*buf.Len())))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// writeRead writes f and reads it back.
func writeRead(t *testing.T, f *Font) *Font {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	res, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(int64(10*buf.Len())))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestSelectWidths checks the choice of defaultWidthX and nominalWidthX.
func TestSelectWidths(t *testing.T) {
	cases := []struct {
		name   string
		widths []float64
		def    float64
		nom    float64
	}{
		{
			// the two remaining widths both lie within 107 of 593
			name:   "mode",
			widths: []float64{500, 500, 600, 700},
			def:    500,
			nom:    593,
		},
		{
			name:   "default tie",
			widths: []float64{600, 500},
			def:    500,
			nom:    493,
		},
		{
			// the fractional widths need the long operand regardless
			name:   "fractional widths are not defaults",
			widths: []float64{500.5, 500.5, 500.5, 7},
			def:    7,
			nom:    0,
		},
		{
			name:   "no integer widths",
			widths: []float64{600.5, 500.5},
			def:    0,
			nom:    0,
		},
		{
			// 0 is out of range, so the nearest admissible value is used
			name:   "no integer widths, wide spread",
			widths: []float64{40000.5, 40100.5},
			def:    0,
			nom:    7333,
		},
		{
			// the fractional widths must not pull the nominal width away
			// from the integer width 800
			name:   "fractional widths not scored",
			widths: []float64{700, 700, 800, 500.5, 500.5, 500.5},
			def:    700,
			nom:    693,
		},
		{
			// one-byte counts tie; 4893 has two widths within 1131
			name:   "two-byte tie-break",
			widths: []float64{100, 100, 100, 1000, 5000, 5300},
			def:    100,
			nom:    4893,
		},
		{
			name:   "all default",
			widths: []float64{100, 100},
			def:    100,
			nom:    0,
		},
		{
			// the nominal width must be at least 70000-32767
			name:   "wide spread",
			widths: []float64{0, 0, 70000, 32800},
			def:    0,
			nom:    37233,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			glyphs := make([]*Glyph, len(c.widths))
			for i, w := range c.widths {
				glyphs[i] = &Glyph{Width: w}
			}
			def, nom, err := selectWidths(glyphs)
			if err != nil {
				t.Fatal(err)
			}
			if def != c.def || nom != c.nom {
				t.Errorf("got default %v, nominal %v, want %v, %v", def, nom, c.def, c.nom)
			}
		})
	}
}

// TestSelectWidthsError checks that widths the charstrings cannot hold are
// rejected.
func TestSelectWidthsError(t *testing.T) {
	cases := []struct {
		name   string
		widths []float64
	}{
		{"off the grid", []float64{0.3}},
		{"too large", []float64{1 << 31}},
		{"infinite", []float64{math.Inf(1)}},
		{"NaN", []float64{math.NaN()}},
		{"spread too far", []float64{0, 0, 1, 70000}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			glyphs := make([]*Glyph, len(c.widths))
			for i, w := range c.widths {
				glyphs[i] = &Glyph{Width: w}
			}
			if _, _, err := selectWidths(glyphs); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// TestWritePerFDWidths checks that defaultWidthX and nominalWidthX are
// chosen per private dictionary, so that the glyphs of one dictionary do
// not constrain the widths of another.
func TestWritePerFDWidths(t *testing.T) {
	f := makeManyFDFont(4)
	widths := []float64{0, 100000, 100100.5, 100200}
	fds := []int{0, 1, 1, 1}
	for i, g := range f.Glyphs {
		g.Width = widths[i]
	}
	f.FDSelect = func(gid glyph.ID) int { return fds[gid] }

	buf := &bytes.Buffer{}
	if err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	out, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<26))
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range out.Glyphs {
		if g.Width != widths[i] {
			t.Errorf("glyph %d: got width %v, want %v", i, g.Width, widths[i])
		}
	}
}

// TestWriteRejectsCoordinate checks that an outline coordinate outside the
// range of a charstring number is reported, instead of being stored wrongly.
func TestWriteRejectsCoordinate(t *testing.T) {
	f := makeLargeFont(2)
	if err := f.Write(io.Discard); err != nil {
		t.Fatal(err)
	}
	f.Glyphs[1].LineTo(70000, 0)
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// topDictOf writes f and returns the decoded top DICT of the result.
func topDictOf(t *testing.T, f *Font) cffDict {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	p := parser.New(bytes.NewReader(buf.Bytes()), membudget.New(int64(10*buf.Len())))
	if err := p.SeekPos(int64(buf.Bytes()[2])); err != nil { // hdrSize
		t.Fatal(err)
	}
	if _, err := readIndex(p); err != nil { // names
		t.Fatal(err)
	}
	topDicts, err := readIndex(p)
	if err != nil {
		t.Fatal(err)
	}
	stringIndex, err := readIndex(p)
	if err != nil {
		t.Fatal(err)
	}
	ss := &cffStrings{data: make([]string, len(stringIndex))}
	for i, s := range stringIndex {
		ss.data[i] = string(s)
	}
	d, err := decodeDict(topDicts[0], ss)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// boxGlyph returns a glyph which fills the given rectangle.
func boxGlyph(name string, x0, y0, x1, y1 float64) *Glyph {
	g := &Glyph{Name: name, Width: 500}
	g.MoveTo(x0, y0)
	g.LineTo(x1, y0)
	g.LineTo(x1, y1)
	g.LineTo(x0, y1)
	return g
}

// bboxOperands returns the four FontBBox operands of a top DICT.
func bboxOperands(t *testing.T, d cffDict) [4]float64 {
	t.Helper()
	vals := d[opFontBBox]
	if len(vals) != 4 {
		t.Fatalf("FontBBox has %d operands", len(vals))
	}
	var res [4]float64
	for i, v := range vals {
		switch x := v.(type) {
		case int32:
			res[i] = float64(x)
		case float64:
			res[i] = x
		default:
			t.Fatalf("operand %d has type %T", i, v)
		}
	}
	return res
}

// statesNoBBox reports whether a top DICT gives no font bounding box, either
// by omitting FontBBox or by giving the default 0 0 0 0.
func statesNoBBox(d cffDict) bool {
	for _, v := range d[opFontBBox] {
		if v != int32(0) && v != 0.0 {
			return false
		}
	}
	return true
}

func TestWriteFontBBoxSimple(t *testing.T) {
	f := makeLargeFont(3)
	f.Glyphs[1] = boxGlyph("a", -20.5, 0, 300, 700.25)
	f.Glyphs[2] = boxGlyph("b", 10, -200.5, 450.5, 500)

	want := [4]float64{-21, -201, 451, 701}
	if diff := cmp.Diff(want, bboxOperands(t, topDictOf(t, f))); diff != "" {
		t.Errorf("wrong FontBBox (-want +got):\n%s", diff)
	}
}

func TestWriteFontBBoxCID(t *testing.T) {
	f := fontWithMatrices(matrix.Identity, []matrix.Matrix{
		{1.0 / 2048, 0, 0, 1.0 / 2048, 0, 0},
		{0.001, 0, 0, 0.001, 0, 0},
	})
	f.Glyphs[0] = boxGlyph("", -100, -300, 1000, 1500)
	f.Glyphs[1] = boxGlyph("", 50, -250.5, 800, 900)

	// glyph 0 spans (-48.83, -146.48)-(488.28, 732.42) in 1000/em space
	want := [4]float64{-49, -251, 800, 900}
	if diff := cmp.Diff(want, bboxOperands(t, topDictOf(t, f))); diff != "" {
		t.Errorf("wrong FontBBox (-want +got):\n%s", diff)
	}
}

func TestWriteFontBBoxEmpty(t *testing.T) {
	for _, fds := range [][]matrix.Matrix{nil, {matrix.Identity, matrix.Identity}} {
		f := fontWithMatrices(matrix.Matrix{0.001, 0, 0, 0.001, 0, 0}, fds)
		f.Glyphs[0] = &Glyph{Name: ".notdef", Width: 100}
		f.Glyphs[1] = &Glyph{Name: "a", Width: 100}
		if fds != nil {
			f.Glyphs[0].Name = ""
			f.Glyphs[1].Name = ""
		}
		if !statesNoBBox(topDictOf(t, f)) {
			t.Errorf("FontBBox given for font without ink (CID %v)", fds != nil)
		}
	}
}

// TestWriteFontBBoxLarge checks the box of a CID-keyed font whose
// coordinates, in 1000/em space, are too large for integer operands.
func TestWriteFontBBoxLarge(t *testing.T) {
	f := fontWithMatrices(matrix.Identity, []matrix.Matrix{
		matrix.Scale(1e200, 1e200), matrix.Scale(1e200, 1e200),
	})
	f.Glyphs[0] = boxGlyph("", 0, 0, 100, 100)
	f.Glyphs[1] = boxGlyph("", -50, -50, 0, 0)

	want := f.FontBBoxPDF().Rounded()
	got := bboxOperands(t, topDictOf(t, f))
	if diff := cmp.Diff([4]float64{want.LLx, want.LLy, want.URx, want.URy}, got); diff != "" {
		t.Errorf("wrong FontBBox (-want +got):\n%s", diff)
	}
}

// TestWriteFontBBoxUnrepresentable checks that a CID-keyed font whose box,
// in 1000/em space, exceeds the range of real operands is written without
// stating a box.
func TestWriteFontBBoxUnrepresentable(t *testing.T) {
	f := fontWithMatrices(matrix.Identity, []matrix.Matrix{
		matrix.Scale(1e300, 1e300), matrix.Scale(1e300, 1e300),
	})
	f.Glyphs[0] = boxGlyph("", 0, 0, 100, 100)
	f.Glyphs[1] = boxGlyph("", -50, -50, 0, 0)

	if !statesNoBBox(topDictOf(t, f)) {
		t.Error("FontBBox given for a box beyond the range of reals")
	}
}
