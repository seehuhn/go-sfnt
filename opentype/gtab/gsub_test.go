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

package gtab

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/parser"
)

func TestGsub1_2EncodeOverflow(t *testing.T) {
	// a subtable whose coverage offset would exceed the uint16 range must
	// be rejected rather than silently truncated
	l := &Gsub1_2{
		Cov:                coverage.Table{},
		SubstituteGlyphIDs: make([]glyph.ID, 0x8000),
	}
	defer func() {
		if recover() == nil {
			t.Error("expected panic on oversized subtable")
		}
	}()
	l.encode()
}

func FuzzGsub1_1(f *testing.F) {
	l := &Gsub1_1{
		Cov:   map[glyph.ID]bool{3: true},
		Delta: 26,
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 1, readGsub1_1, data)
	})
}

func FuzzGsub1_2(f *testing.F) {
	l := &Gsub1_2{
		Cov:                map[glyph.ID]int{2: 0, 3: 1},
		SubstituteGlyphIDs: []glyph.ID{6, 7},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 2, readGsub1_2, data)
	})
}

// TestGsub2_1DropsEmptySequence checks that an on-disk multiple substitution
// with one empty replacement (glyphCount == 0) is normalized away on read:
// the empty entry is dropped, the corresponding coverage glyph is removed,
// and the resulting struct round-trips through encode().
func TestGsub2_1DropsEmptySequence(t *testing.T) {
	data := []byte{
		0x00, 0x01, // format
		0x00, 0x10, // coverageOffset = 16
		0x00, 0x02, // sequenceCount = 2
		0x00, 0x0a, // sequenceOffsets[0] = 10
		0x00, 0x0e, // sequenceOffsets[1] = 14
		// sequence 0 at offset 10: valid (glyphCount=1, gid=10)
		0x00, 0x01, 0x00, 0x0a,
		// sequence 1 at offset 14: empty
		0x00, 0x00,
		// coverage at offset 16: format-1, gids {1, 2}
		0x00, 0x01, 0x00, 0x02, 0x00, 0x01, 0x00, 0x02,
	}
	st1 := readSubtable(t, readGsub2_1, data)
	g1 := st1.(*Gsub2_1)
	for i, repl := range g1.Repl {
		if len(repl) == 0 {
			t.Fatalf("Repl[%d] still empty after read", i)
		}
	}

	st2 := readSubtable(t, readGsub2_1, g1.encode())
	if d := cmp.Diff(st1, st2); d != "" {
		t.Errorf("read-write-read mismatch (-first +second):\n%s", d)
	}
}

// TestGsub3_1DropsEmptyAlternateSet — analogous round-trip for Gsub3_1.
func TestGsub3_1DropsEmptyAlternateSet(t *testing.T) {
	data := []byte{
		0x00, 0x01, // format
		0x00, 0x10, // coverageOffset = 16
		0x00, 0x02, // alternateSetCount = 2
		0x00, 0x0a, // alternateSetOffsets[0] = 10
		0x00, 0x0e, // alternateSetOffsets[1] = 14
		// alternate set 0 at offset 10: valid (glyphCount=1, gid=10)
		0x00, 0x01, 0x00, 0x0a,
		// alternate set 1 at offset 14: empty
		0x00, 0x00,
		// coverage at offset 16: format-1, gids {1, 2}
		0x00, 0x01, 0x00, 0x02, 0x00, 0x01, 0x00, 0x02,
	}
	st1 := readSubtable(t, readGsub3_1, data)
	g1 := st1.(*Gsub3_1)
	for i, alt := range g1.Alternates {
		if len(alt) == 0 {
			t.Fatalf("Alternates[%d] still empty after read", i)
		}
	}

	st2 := readSubtable(t, readGsub3_1, g1.encode())
	if d := cmp.Diff(st1, st2); d != "" {
		t.Errorf("read-write-read mismatch (-first +second):\n%s", d)
	}
}

// TestDropEmptyEntries directly exercises the helper for a few shapes
// not covered by the round-trip tests, including all-empty and
// no-empty inputs as well as renumbering of trailing entries.
func TestDropEmptyEntries(t *testing.T) {
	cases := []struct {
		name    string
		cov     coverage.Table
		items   [][]glyph.ID
		wantCov coverage.Table
		wantOut [][]glyph.ID
	}{
		{
			name:    "no empty",
			cov:     coverage.Table{1: 0, 2: 1},
			items:   [][]glyph.ID{{10}, {11, 12}},
			wantCov: coverage.Table{1: 0, 2: 1},
			wantOut: [][]glyph.ID{{10}, {11, 12}},
		},
		{
			name:    "first empty",
			cov:     coverage.Table{1: 0, 2: 1},
			items:   [][]glyph.ID{{}, {11}},
			wantCov: coverage.Table{2: 0},
			wantOut: [][]glyph.ID{{11}},
		},
		{
			name:    "last empty",
			cov:     coverage.Table{1: 0, 2: 1},
			items:   [][]glyph.ID{{10}, {}},
			wantCov: coverage.Table{1: 0},
			wantOut: [][]glyph.ID{{10}},
		},
		{
			name:    "middle empty renumbers",
			cov:     coverage.Table{1: 0, 2: 1, 3: 2},
			items:   [][]glyph.ID{{10}, {}, {12}},
			wantCov: coverage.Table{1: 0, 3: 1},
			wantOut: [][]glyph.ID{{10}, {12}},
		},
		{
			name:    "multiple empty",
			cov:     coverage.Table{1: 0, 2: 1, 3: 2},
			items:   [][]glyph.ID{{}, {11}, {}},
			wantCov: coverage.Table{2: 0},
			wantOut: [][]glyph.ID{{11}},
		},
		{
			name:    "all empty",
			cov:     coverage.Table{1: 0, 2: 1},
			items:   [][]glyph.ID{{}, {}},
			wantCov: coverage.Table{},
			wantOut: [][]glyph.ID{},
		},
		{
			name:    "empty input",
			cov:     coverage.Table{},
			items:   [][]glyph.ID{},
			wantCov: coverage.Table{},
			wantOut: [][]glyph.ID{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCov, gotOut := dropEmptyEntries(tc.cov, tc.items)
			if d := cmp.Diff(tc.wantCov, gotCov); d != "" {
				t.Errorf("cov mismatch (-want +got):\n%s", d)
			}
			if d := cmp.Diff(tc.wantOut, gotOut); d != "" {
				t.Errorf("items mismatch (-want +got):\n%s", d)
			}
		})
	}
}

// readSubtable invokes a subtable reader on the given bytes, skipping the
// 2-byte format/version prefix at the start.
func readSubtable(t *testing.T, read func(*parser.Parser, int64) (Subtable, error), data []byte) Subtable {
	t.Helper()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	st, err := read(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func FuzzGsub2_1(f *testing.F) {
	l := &Gsub2_1{
		Cov: map[glyph.ID]int{2: 0, 3: 1},
		Repl: [][]glyph.ID{
			{4, 5},
			{1, 2, 3},
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 1, readGsub2_1, data)
	})
}

func FuzzGsub3_1(f *testing.F) {
	l := &Gsub3_1{
		Cov: map[glyph.ID]int{1: 0, 2: 1},
		Alternates: [][]glyph.ID{
			{3, 4},
			{5, 6, 7},
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 3, 1, readGsub3_1, data)
	})
}

// TestGsub4_1ZeroComponentCount checks that a ligature with
// componentCount == 0 (which would underflow uint16 in the original
// allocation calculation) is rejected at read time.
func TestGsub4_1ZeroComponentCount(t *testing.T) {
	data := []byte{
		0x00, 0x01, // version
		0x00, 0x10, // coverageOffset = 16
		0x00, 0x01, // ligatureSetCount = 1
		0x00, 0x08, // ligatureSetOffsets[0] = 8
		// ligature set at offset 8
		0x00, 0x01, // ligatureCount = 1
		0x00, 0x04, // ligatureOffsets[0] = 4 (relative to ligature set)
		// ligature at offset 12
		0x00, 0x0a, // ligatureGlyph = 10
		0x00, 0x00, // componentCount = 0 — INVALID
		// coverage at offset 16 (format-1, count=1, gid=1)
		0x00, 0x01, 0x00, 0x01, 0x00, 0x01,
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	if _, err := readGsub4_1(p, 0); err == nil {
		t.Errorf("expected error for componentCount == 0")
	}
}

func FuzzGsub4_1(f *testing.F) {
	l := &Gsub4_1{
		Cov: map[glyph.ID]int{1: 0, 2: 1},
		Repl: [][]Ligature{
			{
				{In: []glyph.ID{1, 2, 3}, Out: 10},
				{In: []glyph.ID{1, 2}, Out: 11},
				{In: []glyph.ID{1}, Out: 12},
			},
			{
				{In: []glyph.ID{1, 2}, Out: 13},
				{In: []glyph.ID{1}, Out: 14},
			},
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 4, 1, readGsub4_1, data)
	})
}

func FuzzGsub8_1(f *testing.F) {
	l := &Gsub8_1{
		Input:              coverage.Table{1: 0},
		SubstituteGlyphIDs: []glyph.ID{2},
	}
	f.Add(l.encode())
	l = &Gsub8_1{
		Input: coverage.Table{1: 0},
		Backtrack: []coverage.Table{
			{1: 0, 2: 1, 4: 2},
			{1: 0, 3: 1, 4: 2},
		},
		Lookahead: []coverage.Table{
			{2: 0, 3: 1, 10: 2},
			{3: 0},
			{3: 0, 4: 1},
		},
		SubstituteGlyphIDs: []glyph.ID{2},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 8, 1, readGsub8_1, data)
	})
}
