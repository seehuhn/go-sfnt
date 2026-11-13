// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt_test

import (
	"bytes"
	"encoding/binary"
	"maps"
	"math"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/vmtx"
	"seehuhn.de/go/sfnt/vorg"
)

// cmapGlyphs returns the glyph IDs used by the subtables of the font,
// together with the number of subtables which could be decoded.
func cmapGlyphs(t *testing.T, font *sfnt.Font) (gids []glyph.ID, decoded int) {
	t.Helper()
	for key, data := range font.CMapTable {
		c, err := cmap.Table{key: data}.GetNoLang(key.PlatformID, key.EncodingID)
		if err != nil {
			continue
		}
		decoded++
		switch c := c.(type) {
		case *cmap.Format0:
			for _, g := range c.Data {
				gids = append(gids, glyph.ID(g))
			}
		case cmap.Format4:
			for _, g := range c {
				gids = append(gids, g)
			}
		case *cmap.Format6:
			gids = append(gids, c.GlyphIDs...)
		case cmap.Format12:
			for _, g := range c {
				gids = append(gids, g)
			}
		default:
			t.Fatalf("unexpected subtable type %T", c)
		}
	}
	return gids, decoded
}

// checkSubsetAll subsets the font to all glyphs reachable via the cmap.
func checkSubsetAll(t *testing.T, font *sfnt.Font) {
	t.Helper()

	gids, _ := cmapGlyphs(t, font)
	for _, g := range gids {
		if int(g) >= font.NumGlyphs() {
			t.Fatalf("cmap refers to glyph %d, font has %d glyphs", g, font.NumGlyphs())
		}
	}
	slices.Sort(gids)
	gids = slices.Compact(gids)
	if len(gids) == 0 || gids[0] != 0 {
		gids = slices.Insert(gids, 0, 0)
	}
	if _, err := font.Subset(gids); err != nil {
		t.Fatal(err)
	}
}

// TestReadDropsCMapGlyphsOutOfRange checks that cmap mappings to glyphs
// outside the font are dropped on read.
func TestReadDropsCMapGlyphsOutOfRange(t *testing.T) {
	good := cmap.Format4{'A': 4, 'B': 5}
	bad := cmap.Format4{'A': 4, 'B': 5, 'C': 5000}

	var f0 cmap.Format0
	f0.Data['A'] = 4
	f0.Data['B'] = 200 // font has fewer than 200 glyphs

	cases := []struct {
		name string
		key  cmap.Key
		data []byte
		want map[uint16]glyph.ID // expected content after reading, for format 4
	}{
		{"format4", cmap.Key{PlatformID: 3, EncodingID: 1}, bad.Encode(0), good},
		{"format12", cmap.Key{PlatformID: 3, EncodingID: 10},
			cmap.Format12{'A': 4, 'B': 60000}.Encode(0), nil},
		{"format0", cmap.Key{PlatformID: 1, EncodingID: 0}, f0.Encode(0), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			font := debug.MakeSimpleFont()
			if font.NumGlyphs() >= 200 {
				t.Fatal("test font too large")
			}
			font.CMapTable = cmap.Table{tc.key: tc.data}

			buf := &bytes.Buffer{}
			if _, err := font.Write(buf); err != nil {
				t.Fatal(err)
			}
			font1 := readBack(t, buf.Bytes())
			if _, n := cmapGlyphs(t, font1); n != 1 {
				t.Fatalf("expected one decodable subtable, got %d", n)
			}
			checkSubsetAll(t, font1)

			if tc.want != nil {
				c, err := font1.CMapTable.GetNoLang(tc.key.PlatformID, tc.key.EncodingID)
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(tc.want, map[uint16]glyph.ID(c.(cmap.Format4))); diff != "" {
					t.Errorf("wrong cmap (-want +got):\n%s", diff)
				}
			}

			// read-write-read is stable
			buf.Reset()
			if _, err := font1.Write(buf); err != nil {
				t.Fatal(err)
			}
			font2 := readBack(t, buf.Bytes())
			if diff := cmp.Diff(font1.CMapTable, font2.CMapTable); diff != "" {
				t.Errorf("cmap not stable (-1 +2):\n%s", diff)
			}
		})
	}
}

// TestReadKeepsValidCMap checks that a valid cmap table is passed through
// unchanged.
func TestReadKeepsValidCMap(t *testing.T) {
	font := debug.MakeSimpleFont()

	buf := &bytes.Buffer{}
	if _, err := font.Write(buf); err != nil {
		t.Fatal(err)
	}
	font1 := readBack(t, buf.Bytes())
	buf.Reset()
	if _, err := font1.Write(buf); err != nil {
		t.Fatal(err)
	}
	font2 := readBack(t, buf.Bytes())
	if diff := cmp.Diff(font1.CMapTable, font2.CMapTable); diff != "" {
		t.Errorf("cmap not stable (-1 +2):\n%s", diff)
	}
	checkSubsetAll(t, font1)
}

// TestReadCompositeOutOfRange checks that composite glyphs referring to
// missing glyphs do not break Subset.
func TestReadCompositeOutOfRange(t *testing.T) {
	font, err := sfnt.Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	outlines := font.Outlines.(*glyf.Outlines)
	outlines.Glyphs[5] = &glyf.Glyph{
		Data: glyf.CompositeGlyph{Components: []glyf.GlyphComponent{
			{Flags: glyf.FlagArg1And2AreWords | glyf.FlagArgsAreXYValues, GlyphIndex: 7, Data: []byte{0, 0, 0, 0}},
			{Flags: glyf.FlagArg1And2AreWords | glyf.FlagArgsAreXYValues, GlyphIndex: 60000, Data: []byte{0, 0, 0, 0}},
		}},
	}
	font.CMapTable = cmap.Table{{PlatformID: 3, EncodingID: 1}: cmap.Format4{'A': 5}.Encode(0)}

	buf := &bytes.Buffer{}
	if _, err := font.Write(buf); err != nil {
		t.Fatal(err)
	}
	font1 := readBack(t, buf.Bytes())
	checkSubsetAll(t, font1)
}

// TestReadGsubOutOfRange checks that GSUB rules producing missing glyphs
// are dropped on read and do not break Subset.
func TestReadGsubOutOfRange(t *testing.T) {
	font := debug.MakeSimpleFont()
	font.Gsub = &gtab.Info{
		ScriptList: gtab.ScriptListInfo{},
		FeatureList: gtab.FeatureListInfo{
			{Tag: "test", Lookups: []gtab.LookupIndex{0, 1}},
		},
		LookupList: gtab.LookupList{
			{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gsub1_1{Cov: coverage.Set{4: true}, Delta: 5000},
				},
			},
			{
				Meta: &gtab.LookupMetaInfo{LookupType: 4},
				Subtables: []gtab.Subtable{
					&gtab.Gsub4_1{
						Cov:  coverage.Table{4: 0},
						Repl: [][]gtab.Ligature{{{In: []glyph.ID{5}, Out: 6000}}},
					},
				},
			},
		},
	}
	buf := &bytes.Buffer{}
	if _, err := font.Write(buf); err != nil {
		t.Fatal(err)
	}
	font1 := readBack(t, buf.Bytes())
	checkSubsetAll(t, font1)

	// Reading drops the rules, together with the subtables they leave
	// empty, and the result survives a second round trip.
	for i, lookup := range font1.Gsub.LookupList {
		if len(lookup.Subtables) != 0 {
			t.Errorf("lookup %d: invalid rules were kept", i)
		}
	}
	buf.Reset()
	if _, err := font1.Write(buf); err != nil {
		t.Fatal(err)
	}
	font2 := readBack(t, buf.Bytes())
	if diff := cmp.Diff(font1.Gsub, font2.Gsub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// makeFormat6 builds a format 6 subtable from the glyph IDs of consecutive
// character codes, starting at first.
func makeFormat6(first uint16, gids []uint16) []byte {
	data := []byte{0, 6, 0, 0, 0, 0, byte(first >> 8), byte(first), byte(len(gids) >> 8), byte(len(gids))}
	for _, g := range gids {
		data = append(data, byte(g>>8), byte(g))
	}
	data[2], data[3] = byte(len(data)>>8), byte(len(data))
	return data
}

// largeGids returns n glyph IDs, all below 9, which form no long runs.
// Format 4 must store most of them in its glyph ID array, so that more than
// about 32000 of them do not fit into a format 4 subtable.
func largeGids(n int) []uint16 {
	gids := make([]uint16, n)
	for i := range gids {
		gids[i] = uint16(1 + i%2) // no runs, so segments store values
		if i%4000 < 8 {
			gids[i] = uint16(1 + i%4000) // a run, which ends the segment
		}
	}
	return gids
}

// cmapMapping decodes a subtable without translating character codes and
// returns its format together with the mapping it describes.
func cmapMapping(t *testing.T, key cmap.Key, data []byte) (uint16, map[uint32]glyph.ID) {
	t.Helper()
	c, err := cmap.Table{key: data}.GetNoLang(key.PlatformID, key.EncodingID)
	if err != nil {
		t.Fatal(err)
	}
	res := map[uint32]glyph.ID{}
	switch c := c.(type) {
	case cmap.Format4:
		for code, g := range c {
			res[uint32(code)] = g
		}
	case *cmap.Format6:
		for i, g := range c.GlyphIDs {
			if g != 0 {
				res[uint32(c.FirstCode)+uint32(i)] = g
			}
		}
	case cmap.Format12:
		maps.Copy(res, c)
	default:
		t.Fatalf("unexpected subtable type %T", c)
	}
	return uint16(data[0])<<8 | uint16(data[1]), res
}

// mappingOf returns the mapping from first+i to gids[i], leaving out glyph
// IDs which are 0 or at least numGlyphs.
func mappingOf(first uint32, gids []uint16, numGlyphs int) map[uint32]glyph.ID {
	res := map[uint32]glyph.ID{}
	for i, g := range gids {
		if g != 0 && int(g) < numGlyphs {
			res[first+uint32(i)] = glyph.ID(g)
		}
	}
	return res
}

var (
	k03 = cmap.Key{PlatformID: 0, EncodingID: 3}
	k10 = cmap.Key{PlatformID: 1, EncodingID: 0}
	k31 = cmap.Key{PlatformID: 3, EncodingID: 1}
)

// largeFormat6Entries is the largest number of entries a format 6 subtable
// can have, given its 16-bit length field.
const largeFormat6Entries = (0xFFFF - 10) / 2

// TestReadDropsCMapGlyphsLarge checks that a large format 6 subtable, whose
// mapping would not fit into a format 4 subtable, is repaired in place.
func TestReadDropsCMapGlyphsLarge(t *testing.T) {
	const first = 0x20

	cases := []struct {
		name string
		key  cmap.Key
	}{
		{"unicode-bmp", k31},
		{"unicode-platform", k03},
		{"mac", k10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gids := largeGids(largeFormat6Entries)
			gids[20001] = 5000 // out of range
			data := makeFormat6(first, gids)

			font := debug.MakeSimpleFont()
			font.CMapTable = cmap.Table{tc.key: data}

			buf := &bytes.Buffer{}
			if _, err := font.Write(buf); err != nil {
				t.Fatal(err)
			}
			font1 := readBack(t, buf.Bytes())

			if len(font1.CMapTable) != 1 {
				t.Fatalf("expected one subtable, got %d", len(font1.CMapTable))
			}
			got, ok := font1.CMapTable[tc.key]
			if !ok {
				t.Fatalf("no subtable for %v", tc.key)
			}
			format, mapping := cmapMapping(t, tc.key, got)
			if format != 6 {
				t.Errorf("got format %d, want 6", format)
			}
			if len(got) != len(data) {
				t.Errorf("repaired subtable has %d bytes, want %d", len(got), len(data))
			}
			want := mappingOf(first, gids, font1.NumGlyphs())
			if diff := cmp.Diff(want, mapping); diff != "" {
				t.Errorf("wrong cmap (-want +got):\n%s", diff)
			}
			checkSubsetAll(t, font1)
		})
	}
}

// verticalFonts returns a TrueType and a CFF font with vertical metrics.
func verticalFonts(t testing.TB) map[string]*sfnt.Font {
	glyfFont, err := sfnt.Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	glyfFont.Vertical = sfnt.VerticalFixture(glyfFont)
	cffFont := debug.MakeSimpleFont()
	cffFont.Vertical = sfnt.VerticalFixture(cffFont)
	return map[string]*sfnt.Font{"glyf": glyfFont, "cff": cffFont}
}

// TestReadVerticalRoundTrip checks that vertical metrics survive two
// write/read cycles, and that the second write reproduces the first.
func TestReadVerticalRoundTrip(t *testing.T) {
	for name, font := range verticalFonts(t) {
		t.Run(name, func(t *testing.T) {
			data1 := writeFont(t, font)
			font1 := readBack(t, data1)
			if diff := cmp.Diff(font.Vertical, font1.Vertical); diff != "" {
				t.Errorf("first read (-want +got):\n%s", diff)
			}
			checkRoundTrip(t, font1)
		})
	}
}

// checkRoundTrip checks that font1, which was read from a file, reads back
// unchanged after a write, and that writing it again gives the same file.
func checkRoundTrip(t *testing.T, font1 *sfnt.Font) {
	t.Helper()
	data2 := writeFont(t, font1)
	font2 := readBack(t, data2)
	if diff := cmp.Diff(font1, font2, fontCmpOptions(font1)...); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	if data3 := writeFont(t, font2); !bytes.Equal(data2, data3) {
		t.Error("second write differs from the first")
	}
}

func writeFont(t *testing.T, font *sfnt.Font) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	if _, err := font.Write(buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestReadVerticalWithoutVORG checks that the vertical origins of a CFF font
// without a "VORG" table are derived from the top side bearings, and that the
// table is written on the next write.
func TestReadVerticalWithoutVORG(t *testing.T) {
	font := verticalFonts(t)["cff"]
	data := editTables(t, font, func(tables map[string][]byte) {
		delete(tables, "VORG")
	})
	font1 := readBack(t, data)
	if diff := cmp.Diff(font.Vertical, font1.Vertical); diff != "" {
		t.Errorf("vertical metrics (-want +got):\n%s", diff)
	}
	checkRoundTrip(t, font1)
}

// TestReadVerticalVORG checks where the vertical origins of a font come from
// when the "VORG" table disagrees with the top side bearings.
func TestReadVerticalVORG(t *testing.T) {
	const y = 900
	vorgData := (&vorg.Table{Default: y}).Encode()
	for _, c := range []struct {
		name    string
		vorg    []byte
		useVORG bool
	}{
		{"glyf", vorgData, false},
		{"cff", vorgData, true},
		{"cff", vorgData[:6], false}, // malformed, ignored
		{"cff", []byte{0, 2, 0, 0, 0, 0, 0, 0}, false},
	} {
		font := verticalFonts(t)[c.name]
		data := editTables(t, font, func(tables map[string][]byte) {
			tables["VORG"] = c.vorg
		})
		got := readBack(t, data)
		if got.Vertical == nil {
			t.Fatalf("%s: vertical metrics dropped", c.name)
		}
		for gid, origin := range got.Vertical.OriginY {
			want := font.Vertical.OriginY[gid]
			if c.useVORG {
				want = y
			}
			if origin != want {
				t.Errorf("%s: glyph %d has origin %d, want %d", c.name, gid, origin, want)
				break
			}
		}
		checkRoundTrip(t, got)
	}
}

// TestReadVerticalRepairs checks the repairs made when reading vertical
// metrics.
func TestReadVerticalRepairs(t *testing.T) {
	for name, font := range verticalFonts(t) {
		t.Run(name, func(t *testing.T) {
			n := font.NumGlyphs()

			// a caret slope of (0, 0) becomes (0, 1); an old "vhea" version
			// and excess "vmtx" entries are accepted
			data := editTables(t, font, func(tables map[string][]byte) {
				vhea := bytes.Clone(tables["vhea"])
				binary.BigEndian.PutUint32(vhea[0:], 0x00010000)
				binary.BigEndian.PutUint16(vhea[18:], 0)
				binary.BigEndian.PutUint16(vhea[20:], 0)
				tables["vhea"] = vhea
				tables["vmtx"] = append(bytes.Clone(tables["vmtx"]), 0, 1, 0, 2, 0, 3)
			})
			got := readBack(t, data)
			if got.Vertical == nil {
				t.Fatal("vertical metrics dropped")
			}
			v := got.Vertical
			if v.CaretSlopeRise != 0 || v.CaretSlopeRun != 1 {
				t.Errorf("caret slope = (%d, %d), want (0, 1)", v.CaretSlopeRise, v.CaretSlopeRun)
			}
			if len(v.Heights) != n || len(v.OriginY) != n {
				t.Errorf("vertical metrics for %d/%d glyphs, want %d", len(v.Heights), len(v.OriginY), n)
			}
			if diff := cmp.Diff(font.Vertical.OriginY, v.OriginY); diff != "" {
				t.Errorf("origins (-want +got):\n%s", diff)
			}
			checkRoundTrip(t, got)
		})
	}
}

// TestReadVerticalDropped checks that vertical metrics which cannot be used
// are dropped without failing the read.
func TestReadVerticalDropped(t *testing.T) {
	type edit func(font *sfnt.Font, tables map[string][]byte)
	vmtxWith := func(n int, tsb funit.Int16) []byte {
		info := &vmtx.Info{
			Heights: make([]funit.Uint16, n),
			TSB:     make([]funit.Int16, n),
		}
		for i := range n {
			info.Heights[i] = 1000
			info.TSB[i] = tsb
		}
		_, data := info.Encode()
		return data
	}
	cases := []struct {
		name  string
		fonts []string
		edit  edit
	}{
		{"no vhea", []string{"glyf", "cff"}, func(_ *sfnt.Font, tables map[string][]byte) {
			delete(tables, "vhea")
		}},
		{"no vmtx", []string{"glyf", "cff"}, func(_ *sfnt.Font, tables map[string][]byte) {
			delete(tables, "vmtx")
		}},
		{"short vhea", []string{"glyf", "cff"}, func(_ *sfnt.Font, tables map[string][]byte) {
			tables["vhea"] = tables["vhea"][:30]
		}},
		{"bad vhea version", []string{"glyf", "cff"}, func(_ *sfnt.Font, tables map[string][]byte) {
			vhea := bytes.Clone(tables["vhea"])
			binary.BigEndian.PutUint32(vhea[0:], 0x00020000)
			tables["vhea"] = vhea
		}},
		{"odd vmtx length", []string{"glyf", "cff"}, func(_ *sfnt.Font, tables map[string][]byte) {
			tables["vmtx"] = append(bytes.Clone(tables["vmtx"]), 0)
		}},
		{"short vmtx", []string{"glyf", "cff"}, func(font *sfnt.Font, tables map[string][]byte) {
			tables["vmtx"] = vmtxWith(font.NumGlyphs()-1, 0)
		}},
		// the glyph tops are positive, so these origins lie above 32767
		{"origin out of range", []string{"glyf"}, func(font *sfnt.Font, tables map[string][]byte) {
			tables["vmtx"] = vmtxWith(font.NumGlyphs(), math.MaxInt16)
		}},
		{"origin out of range", []string{"cff"}, func(font *sfnt.Font, tables map[string][]byte) {
			tables["vmtx"] = vmtxWith(font.NumGlyphs(), math.MaxInt16)
			delete(tables, "VORG")
		}},
		// a "VORG" origin far below the glyph tops gives a top side bearing
		// below -32768
		{"tsb out of range", []string{"cff"}, func(_ *sfnt.Font, tables map[string][]byte) {
			tables["VORG"] = (&vorg.Table{Default: math.MinInt16}).Encode()
		}},
	}
	fonts := verticalFonts(t)
	for _, c := range cases {
		for _, name := range c.fonts {
			t.Run(c.name+"/"+name, func(t *testing.T) {
				font := fonts[name]
				data := editTables(t, font, func(tables map[string][]byte) {
					c.edit(font, tables)
				})
				got := readBack(t, data)
				if got.Vertical != nil {
					t.Error("unusable vertical metrics kept")
				}
				checkRoundTrip(t, got)
			})
		}
	}
}

// TestReadVerticalCFF2 checks that the vertical metrics of static and
// variable CFF2 fonts are written with a "VORG" table and survive a
// write/read cycle.
func TestReadVerticalCFF2(t *testing.T) {
	for name, font := range map[string]*sfnt.Font{
		"static":   makeCFF2Font(),
		"variable": debug.MakeVarCFF2Font(),
	} {
		t.Run(name, func(t *testing.T) {
			font.Vertical = sfnt.VerticalFixture(font)
			data1 := writeFont(t, font)

			r := bytes.NewReader(data1)
			dir, err := header.Read(r)
			if err != nil {
				t.Fatal(err)
			}
			vorgData, err := dir.ReadTableBytes(r, "VORG")
			if err != nil {
				t.Fatal(err)
			}
			origins, err := vorg.Decode(vorgData)
			if err != nil {
				t.Fatal(err)
			}
			for gid, y := range font.Vertical.OriginY {
				if got := origins.Get(glyph.ID(gid)); got != y {
					t.Errorf("glyph %d: VORG origin = %d, want %d", gid, got, y)
				}
			}

			font1 := readBack(t, data1)
			if diff := cmp.Diff(font.Vertical, font1.Vertical); diff != "" {
				t.Errorf("first read (-want +got):\n%s", diff)
			}
			checkRoundTrip(t, font1)
		})
	}
}
