// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/text/language"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/vmtx"
	"seehuhn.de/go/sfnt/vorg"
)

// TestWriteTrueTypePDFPreservesNames checks that WriteTrueTypePDF retains
// glyph names from the "post" table when they are present in the source
// font.  Downstream consumers (text extraction in particular) rely on
// these names to recover text from symbolic TrueType embeds without a
// /ToUnicode CMap.
func TestWriteTrueTypePDFPreservesNames(t *testing.T) {
	src, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	srcOutlines := src.Outlines.(*glyf.Outlines)
	if len(srcOutlines.Names) == 0 {
		t.Fatal("test precondition: source font carries no glyph names")
	}

	var buf bytes.Buffer
	if _, err := src.WriteTrueTypePDF(&buf); err != nil {
		t.Fatal(err)
	}

	dstData := buf.Bytes()
	dst, err := Read(bytes.NewReader(dstData), parser.NewBudget(int64(len(dstData))))
	if err != nil {
		t.Fatal(err)
	}
	dstOutlines := dst.Outlines.(*glyf.Outlines)
	if len(dstOutlines.Names) != len(srcOutlines.Names) {
		t.Fatalf("glyph name count: got %d, want %d",
			len(dstOutlines.Names), len(srcOutlines.Names))
	}
	for gid, name := range srcOutlines.Names {
		if dstOutlines.Names[gid] != name {
			t.Errorf("gid %d: got %q, want %q",
				gid, dstOutlines.Names[gid], name)
		}
	}
}

// TestWriteTrueTypePDFRejectsNameCountMismatch checks that writing a font
// whose glyph name slice does not match the glyph count returns an error
// rather than emitting a malformed "post" table.
func TestWriteTrueTypePDFRejectsNameCountMismatch(t *testing.T) {
	src, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	outlines := src.Outlines.(*glyf.Outlines)
	outlines.Names = outlines.Names[:len(outlines.Names)-1]

	var buf bytes.Buffer
	if _, err := src.WriteTrueTypePDF(&buf); err == nil {
		t.Fatal("expected error for glyph name / glyph count mismatch")
	}
}

// TestWriteTrueTypePDFOmitsNamelessPost checks that no "post" table is
// written when the source font has no glyph names — the original
// minimisation behaviour.
func TestWriteTrueTypePDFOmitsNamelessPost(t *testing.T) {
	src, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	src.Outlines.(*glyf.Outlines).Names = nil

	var buf bytes.Buffer
	if _, err := src.WriteTrueTypePDF(&buf); err != nil {
		t.Fatal(err)
	}

	dstData := buf.Bytes()
	dst, err := Read(bytes.NewReader(dstData), parser.NewBudget(int64(len(dstData))))
	if err != nil {
		t.Fatal(err)
	}
	if names := dst.Outlines.(*glyf.Outlines).Names; names != nil {
		t.Errorf("expected nil glyph names, got %d entries", len(names))
	}
}

// TestWriteLargeGpos checks that a GPOS table whose offsets do not all fit
// in the uint16 the format uses survives a read-write-read cycle.  The writer
// reorders the layout and inserts extension records to bring every target
// back within reach; class-pair positioning subtables are what reach that
// size in real fonts, so they are what the cases below build.
//
// Each subtable here fits in 64 KiB on its own; only the lookup list built
// from them puts a lookup table, or a subtable's distance from its lookup
// table, out of reach.  A subtable which outgrows the limit by itself is the
// encoder's own problem, and is covered in the gtab package.
func TestWriteLargeGpos(t *testing.T) {
	cases := []struct {
		name      string
		lookups   int
		subtables int
		classes   int
	}{
		{"subtables out of reach of their lookup", 1, 3, 130},
		{"lookup tables out of reach", 6, 1, 130},
		{"both out of reach", 4, 2, 130},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testLargeGpos(t, c.lookups, c.subtables, c.classes)
		})
	}
}

func testLargeGpos(t *testing.T, lookups, subtables, classes int) {
	t.Helper()

	f1, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	f1.Gpos = largeGpos(lookups, subtables, classes)

	buf := &bytes.Buffer{}
	if _, err := f1.Write(buf); err != nil {
		t.Fatal(err)
	}

	body := buf.Bytes()
	f2, err := Read(bytes.NewReader(body), parser.NewBudget(int64(len(body))))
	if err != nil {
		t.Fatal(err)
	}

	// The encoding is the fixpoint that matters here: the lookup list has to
	// come back with its subtables in the same shape, whatever reordering and
	// extension records the writer needed to fit the offsets.
	before := f1.Gpos.Encode()
	after := f2.Gpos.Encode()

	// Guard against a vacuous test: each case exists to drive the layout past
	// what 16-bit offsets reach, which is only true while the writer answers
	// it with extension records.
	if n := countExtensionLookups(before); n == 0 {
		t.Fatalf("GPOS is %d bytes but needed no extension record", len(before))
	}

	if !bytes.Equal(before, after) {
		t.Errorf("encoded GPOS differs; lookup lists (-before +after):\n%s",
			cmp.Diff(f1.Gpos.LookupList, f2.Gpos.LookupList))
	}
}

// countExtensionLookups reports how many lookups of an encoded GPOS table use
// an extension record (lookup type 9) to reach their subtables.
func countExtensionLookups(b []byte) int {
	lookupList := int(binary.BigEndian.Uint16(b[8:]))
	n := int(binary.BigEndian.Uint16(b[lookupList:]))
	count := 0
	for i := range n {
		off := lookupList + int(binary.BigEndian.Uint16(b[lookupList+2+2*i:]))
		if binary.BigEndian.Uint16(b[off:]) == 9 {
			count++
		}
	}
	return count
}

// largeGpos builds a GPOS table of the given shape, each subtable a class-pair
// adjustment with classes*classes entries.
func largeGpos(lookups, subtables, classes int) *gtab.Info {
	adjust := make([][]*gtab.PairAdjust, classes)
	for i := range adjust {
		adjust[i] = make([]*gtab.PairAdjust, classes)
		for j := range adjust[i] {
			adjust[i][j] = &gtab.PairAdjust{
				First: &gtab.GposValueRecord{XAdvance: funit.Int16(i - j)},
			}
		}
	}

	ll := make(gtab.LookupList, lookups)
	for i := range ll {
		ss := make([]gtab.Subtable, subtables)
		for j := range ss {
			ss[j] = &gtab.Gpos2_2{
				Cov:    coverage.Set{glyph.ID(1): true},
				Class1: classdef.Table{glyph.ID(1): 1},
				Class2: classdef.Table{glyph.ID(1): 1},
				Adjust: adjust,
			}
		}
		ll[i] = &gtab.LookupTable{
			Meta:      &gtab.LookupMetaInfo{LookupType: 2},
			Subtables: ss,
		}
	}

	features := make(gtab.FeatureListInfo, lookups)
	idx := make([]gtab.FeatureIndex, lookups)
	for i := range features {
		features[i] = &gtab.Feature{Tag: "kern", Lookups: []gtab.LookupIndex{gtab.LookupIndex(i)}}
		idx[i] = gtab.FeatureIndex(i)
	}

	return &gtab.Info{
		ScriptList: gtab.ScriptListInfo{
			language.MustParse("und-Latn"): {Required: 0xFFFF, Optional: idx},
		},
		FeatureList: features,
		LookupList:  ll,
	}
}

// TestWriteRejectsUnitsPerEm checks that the writer refuses a units per em
// value the "head" table cannot store.  Writing it would silently truncate
// the value and leave every metric in the font scaled wrongly.
func TestWriteRejectsUnitsPerEm(t *testing.T) {
	cases := []struct {
		upm uint16
		ok  bool
	}{
		{0, false},
		{15, false},
		{16, true},
		{1000, true},
		{16384, true},
		{16385, false},
		{65535, false},
	}
	for _, c := range cases {
		f, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
		if err != nil {
			t.Fatal(err)
		}
		f.UnitsPerEm = c.upm

		_, err = f.Write(io.Discard)
		if (err == nil) != c.ok {
			t.Errorf("UnitsPerEm %d: got error %v, want ok=%v", c.upm, err, c.ok)
		}
		_, err = f.WriteTrueTypePDF(io.Discard)
		if (err == nil) != c.ok {
			t.Errorf("UnitsPerEm %d (PDF): got error %v, want ok=%v", c.upm, err, c.ok)
		}
	}
}

// TestUnitsPerEmFrom checks the sources UnitsPerEm is taken from when the head
// table gives nothing usable.
func TestUnitsPerEmFrom(t *testing.T) {
	cases := []struct {
		name string
		head *head.Info
		m    matrix.Matrix
		want float64
	}{
		{"head wins", &head.Info{UnitsPerEm: 2048}, matrix.Scale(0.001, 0.001), 2048},
		{"head unusable", &head.Info{UnitsPerEm: 1}, matrix.Scale(0.001, 0.001), 1000},
		{"no head", nil, matrix.Scale(0.0005, 0.0005), 2000},
		{"no matrix", nil, matrix.Matrix{}, 1000},
		{"matrix too coarse", nil, matrix.Scale(0.5, 0.5), 1000},
		{"matrix too fine", nil, matrix.Scale(1e-6, 1e-6), 1000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unitsPerEmFrom(c.head, c.m); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestReadRepairsUnitsPerEm checks that a units per em value outside the range
// the "head" table allows is replaced on read.  Every glyph metric is measured
// in these units, so leaving the value in place spreads infinities and NaNs
// through the font.
func TestReadRepairsUnitsPerEm(t *testing.T) {
	for _, upm := range []uint16{0, 15, 16385} {
		body := patchUnitsPerEm(t, goregular.TTF, upm)

		f, err := Read(bytes.NewReader(body), parser.NewBudget(int64(len(body))))
		if err != nil {
			t.Fatal(err)
		}
		if f.UnitsPerEm != 1000 {
			t.Errorf("UnitsPerEm %d: got %d, want 1000", upm, f.UnitsPerEm)
		}
		w := f.WidthsPDF()[1]
		if math.IsNaN(w) || math.IsInf(w, 0) {
			t.Errorf("UnitsPerEm %d: glyph width is %v", upm, w)
		}
	}
}

// patchUnitsPerEm returns a copy of an sfnt file with the unitsPerEm field of
// the "head" table replaced.
func patchUnitsPerEm(t *testing.T, body []byte, upm uint16) []byte {
	t.Helper()

	res := bytes.Clone(body)
	h, err := header.Read(bytes.NewReader(res))
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := h.Toc["head"]
	if !ok {
		t.Fatal("no head table")
	}
	binary.BigEndian.PutUint16(res[rec.Offset+18:], upm)
	return res
}

// TestWriteRejectsWidth checks that the writer refuses an advance width the
// "hmtx" table cannot hold, instead of storing a different width.
func TestWriteRejectsWidth(t *testing.T) {
	cases := []struct {
		width float64
		ok    bool
	}{
		{0, true},
		{500, true},
		{65535, true},
		{65535.25, true},
		{65535.5, false},
		{-0.25, true},
		{-0.5, false},
		{-500, false},
	}
	for _, c := range cases {
		glyphs := []*cff.Glyph{cff.NewGlyph(".notdef", c.width)}
		font := &Font{
			FontName:   "Test",
			UnitsPerEm: 1000,
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
			Outlines: &cff.Outlines{
				Glyphs:   glyphs,
				Private:  []*type1.PrivateDict{{}},
				FDSelect: func(glyph.ID) int { return 0 },
				Encoding: cff.StandardEncoding(glyphs),
			},
		}
		_, err := font.Write(io.Discard)
		if (err == nil) != c.ok {
			t.Errorf("width %g: got error %v, want ok=%v", c.width, err, c.ok)
		}
	}
}

// TestWriteSparseEncoding checks that a CFF built-in encoding in which
// unencoded glyphs sit between encoded ones survives a write/read cycle.
func TestWriteSparseEncoding(t *testing.T) {
	var glyphs []*cff.Glyph
	for _, name := range []string{".notdef", "alpha", "unused", "beta", "gamma"} {
		glyphs = append(glyphs, cff.NewGlyph(name, 500))
	}
	encoding := make([]glyph.ID, 256)
	encoding['a'] = 1
	encoding['b'] = 3
	encoding['g'] = 4
	encoding['G'] = 4
	font := &Font{
		FontName:   "Test",
		UnitsPerEm: 1000,
		FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		Outlines: &cff.Outlines{
			Glyphs:   glyphs,
			Private:  []*type1.PrivateDict{{}},
			FDSelect: func(glyph.ID) int { return 0 },
			Encoding: encoding,
		},
	}

	buf := &bytes.Buffer{}
	_, err := font.Write(buf)
	if err != nil {
		t.Fatal(err)
	}
	font2, err := Read(bytes.NewReader(buf.Bytes()), parser.NewBudget(int64(buf.Len())))
	if err != nil {
		t.Fatal(err)
	}

	got := font2.Outlines.(*cff.Outlines).Encoding
	if diff := cmp.Diff(encoding, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// tablesOf returns the tables of a font file.
func tablesOf(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r := bytes.NewReader(data)
	dir, err := header.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]byte, len(dir.Toc))
	for name := range dir.Toc {
		body, err := dir.ReadTableBytes(r, name)
		if err != nil {
			t.Fatal(err)
		}
		tables[name] = body
	}
	return tables
}

// TestWriteVerticalTables checks which vertical tables the writers emit.
func TestWriteVerticalTables(t *testing.T) {
	glyfFont := ttfFixture(t)
	cffFont := cffFixture()
	type want struct{ vhea, vmtx, vorg bool }
	cases := []struct {
		name  string
		font  *Font
		write func(f *Font, buf *bytes.Buffer) error
		want  want
	}{
		{"glyf", glyfFont, writeFull, want{true, true, false}},
		{"cff", cffFont, writeFull, want{true, true, true}},
		{"glyf PDF", glyfFont, func(f *Font, buf *bytes.Buffer) error {
			_, err := f.WriteTrueTypePDF(buf)
			return err
		}, want{}},
		{"cff PDF", cffFont, func(f *Font, buf *bytes.Buffer) error {
			return f.WriteOpenTypeCFFPDF(buf)
		}, want{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.font.Clone()
			f.FontName = "Test-Regular"
			f.Vertical = verticalFixture(f)
			buf := &bytes.Buffer{}
			if err := c.write(f, buf); err != nil {
				t.Fatal(err)
			}
			tables := tablesOf(t, buf.Bytes())
			_, vhea := tables["vhea"]
			_, vmtx := tables["vmtx"]
			_, vorg := tables["VORG"]
			if got := (want{vhea, vmtx, vorg}); got != c.want {
				t.Errorf("tables (vhea, vmtx, VORG) = %v, want %v", got, c.want)
			}
		})
	}

	// no vertical metrics, no vertical tables
	buf := &bytes.Buffer{}
	if err := writeFull(cffFont, buf); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vhea", "vmtx", "VORG"} {
		if _, ok := tablesOf(t, buf.Bytes())[name]; ok {
			t.Errorf("font without vertical metrics has a %q table", name)
		}
	}
}

func writeFull(f *Font, buf *bytes.Buffer) error {
	_, err := f.Write(buf)
	return err
}

// TestWriteRejectsVertical checks that Write refuses vertical metrics which
// the "vmtx" table cannot hold.
func TestWriteRejectsVertical(t *testing.T) {
	for _, c := range []struct {
		name   string
		modify func(v *VerticalMetrics)
	}{
		{"short Heights", func(v *VerticalMetrics) { v.Heights = v.Heights[1:] }},
		{"long OriginY", func(v *VerticalMetrics) { v.OriginY = append(v.OriginY, 0) }},
		{"nil OriginY", func(v *VerticalMetrics) { v.OriginY = nil }},
		// glyph 0 has its top at 700, so the bearing is below -32768
		{"tsb out of range", func(v *VerticalMetrics) { v.OriginY[0] = math.MinInt16 }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := cffFixture()
			f.Vertical = verticalFixture(f)
			c.modify(f.Vertical)
			if _, err := f.Write(io.Discard); err == nil {
				t.Error("invalid vertical metrics accepted")
			}
		})
	}
}

// TestWriteVerticalTSB checks that the top side bearings are
// measured from the glyph tops, and that the vertical origins of a CFF font
// are recorded in the "VORG" table.
func TestWriteVerticalTSB(t *testing.T) {
	f := cffFixture()
	f.Vertical = verticalFixture(f)
	buf := &bytes.Buffer{}
	if _, err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	tables := tablesOf(t, buf.Bytes())

	info, err := vmtx.Decode(tables["vhea"], tables["vmtx"])
	if err != nil {
		t.Fatal(err)
	}
	origins, err := vorg.Decode(tables["VORG"])
	if err != nil {
		t.Fatal(err)
	}
	tops := []int{700, 500, 650, 0, 720, 300}
	for i, top := range tops {
		y := f.Vertical.OriginY[i]
		if got, want := int(info.TSB[i]), int(y)-top; got != want {
			t.Errorf("glyph %d: tsb = %d, want %d", i, got, want)
		}
		if got := origins.Get(glyph.ID(i)); got != y {
			t.Errorf("glyph %d: VORG origin = %d, want %d", i, got, y)
		}
	}
	if diff := cmp.Diff(f.Vertical.Heights, info.Heights); diff != "" {
		t.Errorf("heights (-want +got):\n%s", diff)
	}
}
