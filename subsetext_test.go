// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/debug"
)

// TestSubsetLargeCMap checks that subsetting keeps a large format 6 subtable
// in place, including its character codes.
func TestSubsetLargeCMap(t *testing.T) {
	font := debug.MakeSimpleFont()
	numGlyphs := font.NumGlyphs()

	const first = 0x20
	gids := largeGids(largeFormat6Entries)
	for _, g := range gids {
		if int(g) >= numGlyphs {
			t.Fatal("test font too small")
		}
	}

	all := make([]glyph.ID, numGlyphs)
	for i := range all {
		all[i] = glyph.ID(i)
	}

	type subtable struct {
		format  uint16
		mapping map[uint32]glyph.ID
	}
	cases := []struct {
		name  string
		table cmap.Table
		want  map[cmap.Key]subtable
	}{
		{"format6/unicode-bmp",
			cmap.Table{k31: makeFormat6(first, gids)},
			map[cmap.Key]subtable{k31: {6, mappingOf(first, gids, numGlyphs)}}},
		{"format6/mac",
			cmap.Table{k10: makeFormat6(first, gids)},
			map[cmap.Key]subtable{k10: {6, mappingOf(first, gids, numGlyphs)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			font.CMapTable = tc.table
			subset, err := font.Subset(all)
			if err != nil {
				t.Fatal(err)
			}
			got := map[cmap.Key]subtable{}
			for key, data := range subset.CMapTable {
				format, mapping := cmapMapping(t, key, data)
				got[key] = subtable{format, mapping}
			}
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(subtable{})); diff != "" {
				t.Errorf("wrong cmap (-want +got):\n%s", diff)
			}
		})
	}
}
