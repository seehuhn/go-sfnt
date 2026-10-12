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
	"unicode"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/hmtx"
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
					BlueValues: []float64{-10, 0, 700, 800},
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

	// its CFF2 counterpart: two item variation data subtables of different
	// width, an intermediate region, two Font DICTs, stem hints behind a
	// hintmask and cubic curves
	buf.Reset()
	_, err = debug.MakeVarCFF2Font().Write(buf)
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

		if diff := cmp.Diff(font1, font2, fontCmpOptions(font1)...); diff != "" {
			t.Errorf("different (-old +new):\n%s", diff)
		}
	})
}

// fontCmpOptions returns the options for comparing two fonts read from either
// end of a write-read cycle.  ref supplies the glyph count the comparison is
// measured against; the two fonts agree on it, or the comparison reports that
// first.
func fontCmpOptions(ref *sfnt.Font) cmp.Options {
	cmpFDSelectFn := cmp.Comparer(func(fn1, fn2 cff.FDSelectFn) bool {
		for gid := range ref.NumGlyphs() {
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
	return cmp.Options{cmpFDSelectFn, cmpFloat}
}

// The PostScript name must survive embedding in a PDF file: the TrueType
// writer records it in a minimal "name" table, the CFF writer in the CFF
// header.  Without this, a font read back out of a PDF file has lost its name.
func TestPostScriptNamePDFRoundTrip(t *testing.T) {
	const psName = "ABCDEF+Quire-Regular"

	t.Run("glyf", func(t *testing.T) {
		font := debug.MakeVarFont()
		font.FontName = psName

		buf := &bytes.Buffer{}
		if _, err := font.WriteTrueTypePDF(buf); err != nil {
			t.Fatal(err)
		}
		font2 := readBack(t, buf.Bytes())
		if got := font2.FontName; got != psName {
			t.Errorf("PostScript name = %q, want %q", got, psName)
		}
	})

	t.Run("cff", func(t *testing.T) {
		font := debug.MakeSimpleFont()
		font.FontName = psName

		buf := &bytes.Buffer{}
		if err := font.WriteOpenTypeCFFPDF(buf); err != nil {
			t.Fatal(err)
		}
		font2 := readBack(t, buf.Bytes())
		if got := font2.FontName; got != psName {
			t.Errorf("PostScript name = %q, want %q", got, psName)
		}
	})
}

// A font file can carry a PostScript name the "name" table entry is not
// allowed to hold: a Windows record is UTF-16 and takes any string, and other
// tools write such names.  Reading such a file, writing it out and reading it
// again must give the same name back.  Where the font cannot carry the name,
// the second read would otherwise answer with the derived name, which stands
// for a different font.
func TestPostScriptNameRoundTrip(t *testing.T) {
	psNames := []string{
		"Quire-Regular",
		"ABCDEF+Quire-Regular",

		// the "name" table cannot hold these
		"Grüße-Regular",
		"宋体-Regular",
	}
	for _, tc := range []struct {
		label string
		// makeFile writes a font file naming the font psName, using whichever
		// place that kind of font keeps its name in
		makeFile func(t *testing.T, psName string) []byte
		// whether that place can hold a name the "name" table cannot
		carries bool
	}{
		{
			// A "name" table is the only place a TrueType font can name
			// itself, and this library will not write a name entry 6 may not
			// hold; other tools do, so the entry is installed directly.
			label: "glyf",
			makeFile: func(t *testing.T, psName string) []byte {
				return fontWithPostScriptName(t, debug.MakeVarFont(), psName)
			},
			carries: false,
		},
		{
			// A CFF Name INDEX takes any PostScript font name, so the writer
			// can produce this file itself.
			label: "cff",
			makeFile: func(t *testing.T, psName string) []byte {
				font := debug.MakeSimpleFont()
				font.FontName = psName
				buf := &bytes.Buffer{}
				if _, err := font.Write(buf); err != nil {
					t.Fatal(err)
				}
				return buf.Bytes()
			},
			carries: true,
		},
	} {
		for _, psName := range psNames {
			t.Run(tc.label+"/"+psName, func(t *testing.T) {
				font1 := readBack(t, tc.makeFile(t, psName))

				buf := &bytes.Buffer{}
				if _, err := font1.Write(buf); err != nil {
					t.Fatal(err)
				}
				font2 := readBack(t, buf.Bytes())

				if got, want := font2.FontName, font1.FontName; got != want {
					t.Errorf("the two reads disagree: %q and %q", want, got)
				}

				want := psName
				if !tc.carries && !isASCII(psName) {
					// the name cannot be carried, so it is dropped rather than
					// cut down to the characters which fit
					want = ""
				}
				if got := font1.FontName; got != want {
					t.Errorf("PostScript name = %q, want %q", got, want)
				}
			})
		}
	}
}

// isASCII reports whether s consists of ASCII characters only.
func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// fontWithPostScriptName writes font to a font file whose "name" table gives
// psName as the PostScript name.  The name goes into the Windows record, which
// is UTF-16 and takes any string; this library will not write a name the entry
// is not allowed to hold, but other tools do.
func fontWithPostScriptName(t *testing.T, font *sfnt.Font, psName string) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	if _, err := font.Write(buf); err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(buf.Bytes())
	hdr, err := header.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]byte, len(hdr.Toc))
	for tag := range hdr.Toc {
		data, err := hdr.ReadTableBytes(r, tag)
		if err != nil {
			t.Fatal(err)
		}
		tables[tag] = data
	}

	nameInfo, err := name.Decode(tables["name"])
	if err != nil {
		t.Fatal(err)
	}
	nameInfo.Windows["en-US"].PostScriptName = psName
	tables["name"] = nameInfo.Encode(1)

	out := &bytes.Buffer{}
	if _, err := header.Write(out, hdr.ScalerType, tables); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
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
	font.FontName = "ABCDEF+Quire-Regular"

	buf := &bytes.Buffer{}
	if _, err := font.WriteTrueTypePDF(buf, "name", nameData); err != nil {
		t.Fatal(err)
	}

	font2 := readBack(t, buf.Bytes())
	if got := font2.FontName; got != "QuireRegulardraft" {
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
		glyfFont.FontName = psName
		if _, err := glyfFont.Write(&bytes.Buffer{}); err == nil {
			t.Errorf("Write(%q) = nil, want error", psName)
		}
		if _, err := glyfFont.WriteTrueTypePDF(&bytes.Buffer{}); err == nil {
			t.Errorf("WriteTrueTypePDF(%q) = nil, want error", psName)
		}

		cffFont := debug.MakeSimpleFont()
		cffFont.FontName = psName
		if err := cffFont.WriteOpenTypeCFFPDF(&bytes.Buffer{}); err == nil {
			t.Errorf("WriteOpenTypeCFFPDF(%q) = nil, want error", psName)
		}
	}
}

// The names generated for a variable-font instance are built from the "fvar"
// axis tags, which are four arbitrary bytes read from the font file.  A tag
// which is not already a plain identifier is replaced rather than filtered:
// removing the offending bytes can map two tags onto the same string, and the
// name would then no longer tell the axes apart.
func TestInstanceNameSanitized(t *testing.T) {
	font := debug.MakeVarFont()
	font.FamilyName = "Quire Var"
	font.VariationsPostScriptName = ""
	// two tags which filtering would reduce to the same string
	font.Fvar.Axes[0].Tag = "a b\x00"
	font.Fvar.Axes[1].Tag = "ab\x00 "
	font.Fvar.Instances = nil

	inst, err := font.Instantiate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Write(&bytes.Buffer{}); err != nil {
		t.Errorf("instance is not writable: %v", err)
	}
	if got, want := inst.FontName, "QuireVar_400X0_100X1"; got != want {
		t.Errorf("instance name = %q, want %q", got, want)
	}
}

// A substitute for an unusable axis tag must not collide with a tag the font
// already uses, or the generated name would again fail to tell the axes apart.
func TestInstanceNameTagClash(t *testing.T) {
	font := debug.MakeVarFont()
	font.FamilyName = "Quire Var"
	font.VariationsPostScriptName = ""
	font.Fvar.Axes[0].Tag = "a b\x00" // needs a substitute, would be "X0"
	font.Fvar.Axes[1].Tag = "X0"      // already uses that name
	font.Fvar.Instances = nil

	inst, err := font.Instantiate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inst.FontName, "QuireVar_400XX0_100X0"; got != want {
		t.Errorf("instance name = %q, want %q", got, want)
	}
}

// A CFF Name INDEX and "name" table entry 6 are separate strings which a font
// may give different values: one CFF font program may be shared between
// several fonts, so its name belongs to only one of them.  Entry 6 names this
// font, so reading prefers it.
func TestFontNamePrefersNameTableEntry6(t *testing.T) {
	font := debug.MakeSimpleFont()
	font.FontName = "Shared-Regular"

	font2 := readBack(t, fontWithPostScriptName(t, font, "Quire-Regular"))

	if got, want := font2.FontName, "Quire-Regular"; got != want {
		t.Errorf("PostScript name = %q, want %q", got, want)
	}
}

// Entry 6 cannot hold every PostScript name, and a font whose name it cannot
// hold has no entry 6 at all.  The CFF Name INDEX is then the only record of
// the name, as it is in an OpenType/CFF font embedded in a PDF file, where the
// "name" table need not be present.
func TestFontNameFallsBackToCFFNameIndex(t *testing.T) {
	font := debug.MakeSimpleFont()
	font.FontName = "宋体-Regular"

	buf := &bytes.Buffer{}
	if _, err := font.Write(buf); err != nil {
		t.Fatal(err)
	}

	if got, want := readBack(t, buf.Bytes()).FontName, "宋体-Regular"; got != want {
		t.Errorf("PostScript name = %q, want %q", got, want)
	}
}

// Where entry 6 holds a name it is not allowed to hold, the CFF Name INDEX is
// the better record: reading is permissive, so the file is accepted, but the
// entry is not taken at face value.
func TestFontNameSkipsMalformedEntry6(t *testing.T) {
	font := debug.MakeSimpleFont()
	font.FontName = "Quire-Regular"

	font2 := readBack(t, fontWithPostScriptName(t, font, "Quire Regular (draft)"))

	if got, want := font2.FontName, "Quire-Regular"; got != want {
		t.Errorf("PostScript name = %q, want %q", got, want)
	}
}

// Entry 6 of the "name" table may hold at most 63 characters.  A longer name
// is dropped rather than truncated, since a truncated name stands for a
// different font.
func TestFontNameTooLongForNameTable(t *testing.T) {
	long := strings.Repeat("x", 100)

	glyf := readBack(t, fontWithPostScriptName(t, debug.MakeVarFont(), long))
	if got := glyf.FontName; got != "" {
		t.Errorf("glyf: PostScript name = %q, want %q", got, "")
	}

	// a CFF Name INDEX has room for it
	cffFont := debug.MakeSimpleFont()
	cffFont.FontName = long
	buf := &bytes.Buffer{}
	if _, err := cffFont.Write(buf); err != nil {
		t.Fatal(err)
	}
	if got := readBack(t, buf.Bytes()).FontName; got != long {
		t.Errorf("cff: PostScript name = %q, want %d x's", got, len(long))
	}
}

// The variations name prefix belongs to a variable font.  Writing it to a font
// with no variations would lose it, since reading only looks for it there.
func TestVariationsNameNeedsVariations(t *testing.T) {
	font := debug.MakeSimpleFont()
	font.VariationsPostScriptName = "Quire"

	if _, err := font.Write(&bytes.Buffer{}); err == nil {
		t.Error("Write with a variations name on a static font = nil, want error")
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

// widthFile describes an OpenType font file with CFF or CFF2 outlines, for
// testing how advance widths are read.
type widthFile struct {
	// the top-level font matrix
	matrix matrix.Matrix

	// the per-FD font matrices, one per glyph; nil for a name-keyed CFF font
	fds []matrix.Matrix

	// the charstring advance widths, one per glyph (CFF only)
	widths []float64

	// the "head" units per em; 0 for a file without a "head" table
	upm uint16

	// the "hmtx" advance widths; nil for a file without an "hmtx" table
	hmtx []funit.Uint16

	// whether the outlines are CFF2 rather than CFF
	cff2 bool
}

// encode assembles the font file described by spec.
func (spec *widthFile) encode(t *testing.T) []byte {
	t.Helper()

	tables := make(map[string][]byte)
	buf := &bytes.Buffer{}
	if spec.cff2 {
		n := len(spec.hmtx)
		glyphs := make([]*cff.GlyphCFF2, n)
		private := make([]*cff.PrivateCFF2, max(len(spec.fds), 1))
		for i := range glyphs {
			glyphs[i] = &cff.GlyphCFF2{}
		}
		for i := range private {
			private[i] = &cff.PrivateCFF2{}
		}
		fdSelect := func(glyph.ID) int { return 0 }
		if spec.fds != nil {
			fdSelect = func(gid glyph.ID) int { return int(gid) }
		}
		font := &cff.FontCFF2{
			FontMatrix: spec.matrix,
			OutlinesCFF2: &cff.OutlinesCFF2{
				Glyphs:       glyphs,
				Private:      private,
				FDSelect:     fdSelect,
				FontMatrices: spec.fds,
			},
		}
		if err := font.Write(buf); err != nil {
			t.Fatal(err)
		}
		tables["CFF2"] = buf.Bytes()
	} else {
		n := len(spec.widths)
		glyphs := make([]*cff.Glyph, n)
		for i, w := range spec.widths {
			name := ".notdef"
			if i > 0 {
				name = string(rune('a' + i - 1))
			}
			glyphs[i] = cff.NewGlyph(name, w)
		}
		o := &cff.Outlines{
			Glyphs:   glyphs,
			Private:  []*type1.PrivateDict{{}},
			FDSelect: func(glyph.ID) int { return 0 },
		}
		if spec.fds != nil {
			o.Private = make([]*type1.PrivateDict, n)
			for i := range o.Private {
				o.Private[i] = &type1.PrivateDict{}
			}
			o.FDSelect = func(gid glyph.ID) int { return int(gid) }
			o.FontMatrices = spec.fds
			o.ROS = &cid.SystemInfo{Registry: "Test", Ordering: "Test"}
			o.GIDToCID = make([]cid.CID, n)
			for i := range o.GIDToCID {
				o.GIDToCID[i] = cid.CID(i)
			}
		} else {
			o.Encoding = cff.StandardEncoding(glyphs)
		}
		font := &cff.Font{
			FontInfo: &type1.FontInfo{FontName: "Test", FontMatrix: spec.matrix},
			Outlines: o,
		}
		if err := font.Write(buf); err != nil {
			t.Fatal(err)
		}
		tables["CFF "] = buf.Bytes()
	}

	if spec.upm != 0 {
		tables["head"] = (&head.Info{UnitsPerEm: spec.upm}).Encode()
	}
	if spec.hmtx != nil {
		info := &hmtx.Info{
			Widths:       spec.hmtx,
			GlyphExtents: make([]funit.Rect16, len(spec.hmtx)),
		}
		tables["hhea"], tables["hmtx"] = info.Encode()
	}

	out := &bytes.Buffer{}
	if _, err := header.Write(out, header.ScalerTypeCFF, tables); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestWidthRoundTrip checks that the advance widths of fonts with CFF and
// CFF2 outlines survive a read-write-read cycle, for font matrices which do
// not match the units per em of the font, or which are so large or so small
// that the "hmtx" table cannot hold the widths they imply.
func TestWidthRoundTrip(t *testing.T) {
	scale := func(s float64) matrix.Matrix { return matrix.Matrix{s, 0, 0, s, 0, 0} }
	cases := []struct {
		name string
		spec widthFile
		want []float64 // the widths after the first read, in glyph design units
	}{
		{
			name: "matching matrix",
			spec: widthFile{matrix: scale(0.001), widths: []float64{500, 668}},
			want: []float64{500, 668},
		},
		{
			name: "fractional charstring widths",
			spec: widthFile{matrix: scale(0.001), widths: []float64{500.25, 667.75}},
			want: []float64{500.25, 667.75},
		},
		{
			name: "negative charstring width",
			spec: widthFile{matrix: scale(0.001), widths: []float64{-500, 668}},
			want: []float64{0, 668},
		},
		{
			// the charstring widths are measured against the font matrix,
			// the hmtx table against the "head" units per em
			name: "matrix inconsistent with head",
			spec: widthFile{matrix: scale(0.001), widths: []float64{500, 501}, upm: 2048},
			want: []float64{500, 501},
		},
		{
			name: "matrix inconsistent with head, with hmtx",
			spec: widthFile{matrix: scale(0.001), widths: []float64{500, 501}, upm: 2048, hmtx: []funit.Uint16{1024, 1025}},
			want: []float64{500, 1025 / 2.048},
		},
		{
			// 1/0.0003 is not a whole number of units per em
			name: "units per em from matrix",
			spec: widthFile{matrix: scale(0.0003), widths: []float64{500, 1000}, hmtx: []funit.Uint16{150, 300}},
			want: []float64{cff.RoundWidth(150 / (0.0003 * 3333)), cff.RoundWidth(300 / (0.0003 * 3333))},
		},
		{
			// a matrix close to the default must not be mistaken for it
			name: "matrix near the default",
			spec: widthFile{matrix: scale(1.0 / 1001), widths: []float64{500, 600}, upm: 1001, hmtx: []funit.Uint16{500, 600}},
			want: []float64{500, 600},
		},
		{
			name: "CID-keyed, matrix near the default",
			spec: widthFile{matrix: scale(1.000001), fds: []matrix.Matrix{scale(0.001), scale(0.000999999)}, widths: []float64{500, 600}},
			want: []float64{500, 600},
		},
		{
			// the fuzzer found this one: the widths are 2*10^17 units per em
			name: "huge matrix",
			spec: widthFile{matrix: scale(3.0303030303e+11), widths: []float64{668, 668}},
			want: []float64{0, 0},
		},
		{
			// no multiple of 2^-16 maps to an hmtx width of 500
			name: "huge matrix, with hmtx",
			spec: widthFile{matrix: scale(1e10), widths: []float64{668, 668}, hmtx: []funit.Uint16{0, 500}},
			want: []float64{0, 0},
		},
		{
			// the charstring widths map to an hmtx width of 0 and are kept
			name: "tiny matrix",
			spec: widthFile{matrix: scale(1e-200), widths: []float64{668, 668}},
			want: []float64{668, 668},
		},
		{
			// the design width for an hmtx width of 500 is too large to store
			name: "tiny matrix, with hmtx",
			spec: widthFile{matrix: scale(1e-200), widths: []float64{668, 668}, hmtx: []funit.Uint16{0, 500}},
			want: []float64{668, 0},
		},
		{
			name: "mirrored matrix",
			spec: widthFile{matrix: matrix.Matrix{-0.001, 0, 0, 0.001, 0, 0}, widths: []float64{500, -500}},
			want: []float64{0, -500},
		},
		{
			name: "mirrored matrix, with hmtx",
			spec: widthFile{matrix: matrix.Matrix{-0.001, 0, 0, 0.001, 0, 0}, widths: []float64{500, 500}, hmtx: []funit.Uint16{500, 600}},
			want: []float64{-500, -600},
		},
		{
			// the advance has no horizontal component
			name: "rotated matrix",
			spec: widthFile{matrix: matrix.Matrix{0, 0.001, -0.001, 0, 0, 0}, widths: []float64{500, 600}, hmtx: []funit.Uint16{0, 600}},
			want: []float64{500, 0},
		},
		{
			name: "shear overflows",
			spec: widthFile{matrix: matrix.Matrix{0.001, 1e200, 1e200, 0.001, 0, 0}, widths: []float64{500, 600}},
			want: []float64{500, 600},
		},
		{
			name: "CID-keyed, FDs at different scales",
			spec: widthFile{matrix: matrix.Identity, fds: []matrix.Matrix{scale(0.001), scale(0.0005)}, widths: []float64{500, 1001}},
			want: []float64{500, 1001},
		},
		{
			name: "CID-keyed, composition overflows",
			spec: widthFile{matrix: scale(1e200), fds: []matrix.Matrix{scale(0.001), scale(1e200)}, widths: []float64{500, 600}},
			want: []float64{0, 0},
		},
		{
			name: "CFF2, matching matrix",
			spec: widthFile{cff2: true, matrix: scale(0.001), hmtx: []funit.Uint16{500, 668}},
			want: []float64{500, 668},
		},
		{
			name: "CFF2, matrix inconsistent with head",
			spec: widthFile{cff2: true, matrix: scale(0.001), upm: 2048, hmtx: []funit.Uint16{1024, 1025}},
			want: []float64{500, 1025 / 2.048},
		},
		{
			name: "CFF2, huge matrix",
			spec: widthFile{cff2: true, matrix: scale(1e10), hmtx: []funit.Uint16{0, 500}},
			want: []float64{0, 0},
		},
		{
			name: "CFF2, tiny matrix",
			spec: widthFile{cff2: true, matrix: scale(1e-200), hmtx: []funit.Uint16{0, 500}},
			want: []float64{0, 0},
		},
		{
			name: "CFF2, composition overflows",
			spec: widthFile{cff2: true, matrix: scale(1e200), fds: []matrix.Matrix{scale(1e200), scale(1e200)}, hmtx: []funit.Uint16{0, 500}},
			want: []float64{0, 0},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			font1 := readBack(t, c.spec.encode(t))

			if diff := cmp.Diff(c.want, font1.Widths(), fontCmpOptions(font1)...); diff != "" {
				t.Errorf("widths (-want +got):\n%s", diff)
			}

			buf := &bytes.Buffer{}
			if _, err := font1.Write(buf); err != nil {
				t.Fatal(err)
			}
			font2 := readBack(t, buf.Bytes())
			if diff := cmp.Diff(font1, font2, fontCmpOptions(font1)...); diff != "" {
				t.Errorf("round trip failed (-old +new):\n%s", diff)
			}
		})
	}
}
