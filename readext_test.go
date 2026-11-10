// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt_test

import (
	"bytes"
	"maps"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/parser"
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
