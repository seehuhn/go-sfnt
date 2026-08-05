// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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
	"math"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/name"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/parser"
)

// readBack reads a font file the test has just written.
func readBack(t *testing.T, data []byte) *sfnt.Font {
	t.Helper()
	font, err := sfnt.Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	return font
}

// swapFvarAxisNameIDs patches the "fvar" table's first two axes so their
// stored NameIDs are swapped, in place, in an otherwise valid font produced
// by [sfnt.Font.Write].  data must contain a "fvar" table with at least two
// axes and canonical NameIDs (256, 257, ...) in axis order; the table
// shrinks and grows by nothing, so no other offsets in data change.
func swapFvarAxisNameIDs(tb testing.TB, data []byte) []byte {
	tb.Helper()

	data = bytes.Clone(data)
	rr := bytes.NewReader(data)
	dir, err := header.Read(rr)
	if err != nil {
		tb.Fatal(err)
	}
	rec, ok := dir.Toc["fvar"]
	if !ok {
		tb.Fatal("font has no fvar table")
	}
	fd, err := dir.TableReader(rr, "fvar")
	if err != nil {
		tb.Fatal(err)
	}
	tab, err := fvar.Read(fd, parser.NewBudget(int64(rec.Length)))
	if err != nil {
		tb.Fatal(err)
	}
	if len(tab.Axes) < 2 {
		tb.Fatal("font has fewer than two fvar axes")
	}
	if tab.Axes[0].NameID != 256 || tab.Axes[1].NameID != 257 {
		tb.Fatalf("fvar axes are not canonically numbered: %+v", tab.Axes[:2])
	}
	tab.Axes[0].NameID, tab.Axes[1].NameID = 257, 256

	patched := tab.Encode()
	if len(patched) != int(rec.Length) {
		tb.Fatalf("patched fvar table changed size: %d vs %d", len(patched), rec.Length)
	}
	copy(data[rec.Offset:int(rec.Offset)+len(patched)], patched)
	return data
}

func TestGetFontInfo(t *testing.T) {
	font := debug.MakeSimpleFont()
	font.Trademark = "test trademark notice"
	font.Copyright = "(c) 2022 test copyright notice"

	fontInfo1 := font.GetFontInfo()

	cffFont1 := &cff.Font{
		FontInfo: fontInfo1,
		Outlines: font.Outlines.(*cff.Outlines),
	}
	buf := &bytes.Buffer{}
	err := cffFont1.Write(buf)
	if err != nil {
		t.Fatal(err)
	}
	cffData := buf.Bytes()

	cffFont2, err := cff.Read(bytes.NewReader(cffData), parser.NewBudget(int64(len(cffData))))
	if err != nil {
		t.Fatal(err)
	}
	fontInfo2 := cffFont2.FontInfo

	if d := cmp.Diff(fontInfo1, fontInfo2); d != "" {
		t.Errorf("font info differs: (-got +want)\n%s", d)
	}
}

func FuzzFont(f *testing.F) {
	f.Add(goregular.TTF)
	f.Add(gobolditalic.TTF)

	fontInfo := debug.MakeSimpleFont()
	buf := &bytes.Buffer{}
	_, err := fontInfo.Write(buf)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(buf.Bytes())

	g0 := cff.NewGlyph(".notdef", 777)
	g0.MoveTo(0, 0)
	g0.LineTo(600, 0)
	g0.LineTo(600, 600)
	g0.LineTo(0, 600)
	g1 := cff.NewGlyph("A", 900)
	g1.MoveTo(50, 50)
	g1.LineTo(850, 50)
	g1.LineTo(850, 850)
	g1.LineTo(50, 850)
	gg := []*cff.Glyph{g0, g1}
	fontInfo = &sfnt.Font{
		FamilyName:         "Test",
		Width:              os2.WidthNormal,
		Weight:             os2.WeightNormal,
		UnitsPerEm:         1234,
		Ascent:             800,
		Descent:            -200,
		LineGap:            100,
		CapHeight:          400,
		XHeight:            200,
		ItalicAngle:        -12.5,
		UnderlinePosition:  -100,
		UnderlineThickness: -50,
		Outlines: &cff.Outlines{
			Glyphs: gg,
			Private: []*type1.PrivateDict{
				{
					BlueValues: []funit.Int16{-10, 0, 700, 800},
					StdHW:      70,
					StdVW:      70,
				},
			},
			FDSelect: func(glyph.ID) int { return 0 },
			Encoding: cff.StandardEncoding(gg),
		},
	}
	buf.Reset()
	_, err = fontInfo.Write(buf)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(buf.Bytes())

	// a synthetic variable font exercising every variation table
	buf.Reset()
	_, err = makeVariableFont(f).Write(buf)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(buf.Bytes())

	// a minimal CFF2 font
	buf.Reset()
	_, err = makeCFF2Font().Write(buf)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(buf.Bytes())

	// the hand-computable synthetic variable font from internal/debug
	buf.Reset()
	_, err = debug.MakeVarFont().Write(buf)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(buf.Bytes())

	// a variable font whose stored fvar axis NameIDs are swapped relative to
	// the canonical order Write assigns from axis position (256, 257
	// becomes 257, 256); a Critical review of d6a89a7 found that Read kept
	// the stored IDs verbatim while Write always reassigns canonically,
	// breaking the read-write-read invariant for any font not already using
	// canonical numbering.
	buf.Reset()
	_, err = debug.MakeVarFont().Write(buf)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(swapFvarAxisNameIDs(f, buf.Bytes()))

	f.Fuzz(func(t *testing.T, data []byte) {
		font1, err := sfnt.Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}

		buf := &bytes.Buffer{}
		_, err = font1.Write(buf)
		if err != nil {
			t.Fatal(err)
		}

		font2Data := buf.Bytes()
		font2, err := sfnt.Read(bytes.NewReader(font2Data), parser.NewBudget(int64(len(font2Data))))
		if err != nil {
			t.Fatal(err)
		}

		cmpFDSelectFn := cmp.Comparer(func(fn1, fn2 cff.FDSelectFn) bool {
			for gid := 0; gid < font1.NumGlyphs(); gid++ {
				if fn1(glyph.ID(gid)) != fn2(glyph.ID(gid)) {
					return false
				}
			}
			return true
		})
		cmpFloat := cmp.Comparer(func(x1, x2 float64) bool {
			d := math.Max(math.Abs(x1), math.Abs(x2)) * 1e-8
			return math.Abs(x2-x1) <= d
		})
		// CFF glyph widths are constrained to the hmtx table's precision, which
		// stores advance widths as integers in font design units (UnitsPerEm).
		// At UnitsPerEm != 1000 a CFF-glyph-space width therefore round-trips to
		// the nearest representable value, so compare the quantised hmtx widths
		// rather than the raw glyph-space widths.
		upm := float64(font1.UnitsPerEm)
		toHmtx := func(w float64) int {
			// mirror makeHmtx: quantise and clamp to the UFWORD range
			return int(max(0, min(math.Round(w*upm/1000), 0xFFFF)))
		}
		cmpGlyphWidth := cmp.Comparer(func(g1, g2 *cff.Glyph) bool {
			if g1.Name != g2.Name {
				return false
			}
			return toHmtx(g1.Width) == toHmtx(g2.Width)
		})
		cmpUnexported := cmpopts.IgnoreUnexported(sfnt.Font{})
		if diff := cmp.Diff(font1, font2, cmpFDSelectFn, cmpFloat, cmpGlyphWidth, cmpUnexported); diff != "" {
			t.Errorf("different (-old +new):\n%s", diff)
		}
		// the PostScript name lives in an unexported field, which the diff
		// above ignores
		if name1, name2 := font1.PostScriptName(), font2.PostScriptName(); name1 != name2 {
			t.Errorf("PostScript name %q -> %q", name1, name2)
		}
	})
}

// The PostScript name must survive embedding in a PDF file: the TrueType
// writer records it in a minimal "name" table, the CFF writer in the CFF
// header.  Without this, a font read back out of a PDF file has lost its name.
func TestPostScriptNamePDFRoundTrip(t *testing.T) {
	const psName = "ABCDEF+Quire-Regular"

	t.Run("glyf", func(t *testing.T) {
		font := debug.MakeVarFont()
		font.SetPostScriptName(psName)

		buf := &bytes.Buffer{}
		if _, err := font.WriteTrueTypePDF(buf); err != nil {
			t.Fatal(err)
		}
		font2 := readBack(t, buf.Bytes())
		if got := font2.PostScriptName(); got != psName {
			t.Errorf("PostScript name = %q, want %q", got, psName)
		}
	})

	t.Run("cff", func(t *testing.T) {
		font := debug.MakeSimpleFont()
		font.SetPostScriptName(psName)

		buf := &bytes.Buffer{}
		if err := font.WriteOpenTypeCFFPDF(buf); err != nil {
			t.Fatal(err)
		}
		font2 := readBack(t, buf.Bytes())
		if got := font2.PostScriptName(); got != psName {
			t.Errorf("PostScript name = %q, want %q", got, psName)
		}
	})
}

// Reading is permissive: a "name" table holding a name no PostScript name may
// contain is repaired rather than rejected, and the repaired font can be
// written again.
func TestPostScriptNameSanitizedOnRead(t *testing.T) {
	nameTable := &name.Table{PostScriptName: "Quire Regular (draft)\x00"}
	nameData := (&name.Info{
		Windows: name.Tables{"en-US": nameTable},
	}).Encode(1)

	font := debug.MakeVarFont()
	font.SetPostScriptName("ABCDEF+Quire-Regular")

	buf := &bytes.Buffer{}
	if _, err := font.WriteTrueTypePDF(buf, "name", nameData); err != nil {
		t.Fatal(err)
	}

	font2 := readBack(t, buf.Bytes())
	if got := font2.PostScriptName(); got != "QuireRegulardraft" {
		t.Errorf("PostScript name = %q, want %q", got, "QuireRegulardraft")
	}
	if _, err := font2.Write(&bytes.Buffer{}); err != nil {
		t.Errorf("repaired font is not writable: %v", err)
	}
}

// Writing is strict: an invalid name installed through the API is rejected by
// every writer rather than being silently corrected.
func TestPostScriptNameWriteRejected(t *testing.T) {
	bad := []string{
		"Quire Regular",
		"Quire(Regular)",
		"Quire/Regular",
		strings.Repeat("x", 128), // one over the length limit
	}
	for _, psName := range bad {
		glyfFont := debug.MakeVarFont()
		glyfFont.SetPostScriptName(psName)
		if _, err := glyfFont.Write(&bytes.Buffer{}); err == nil {
			t.Errorf("Write(%q) = nil, want error", psName)
		}
		if _, err := glyfFont.WriteTrueTypePDF(&bytes.Buffer{}); err == nil {
			t.Errorf("WriteTrueTypePDF(%q) = nil, want error", psName)
		}

		cffFont := debug.MakeSimpleFont()
		cffFont.SetPostScriptName(psName)
		if err := cffFont.WriteOpenTypeCFFPDF(&bytes.Buffer{}); err == nil {
			t.Errorf("WriteOpenTypeCFFPDF(%q) = nil, want error", psName)
		}
	}
}

// The names generated for a variable-font instance are built from the "fvar"
// axis tags, which are four arbitrary bytes read from the font file.
func TestInstanceNameSanitized(t *testing.T) {
	font := debug.MakeVarFont()
	font.FamilyName = "Quire Var"
	font.VariationsPostScriptName = ""
	for i := range font.Fvar.Axes {
		font.Fvar.Axes[i].Tag = "a b\x00"
	}
	font.Fvar.Instances = nil

	inst, err := font.Instantiate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Write(&bytes.Buffer{}); err != nil {
		t.Errorf("instance is not writable: %v", err)
	}
}

// A complete font file also carries the variations name prefix and the
// PostScript names of the named instances; those must be valid too.
func TestWriteRejectsInvalidInstanceNames(t *testing.T) {
	font := debug.MakeVarFont()
	font.VariationsPostScriptName = "Quire Var"
	if _, err := font.Write(&bytes.Buffer{}); err == nil {
		t.Error("Write with invalid variations name prefix = nil, want error")
	}

	font = debug.MakeVarFont()
	font.Fvar.Instances[0].PostScriptName = "Quire (Regular)"
	if _, err := font.Write(&bytes.Buffer{}); err == nil {
		t.Error("Write with invalid instance name = nil, want error")
	}
}
