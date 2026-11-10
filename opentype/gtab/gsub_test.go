// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

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

// checkSharedRoundTrip reads a subtable whose offsets share tables, and
// checks that it can be written back without growing and that reading the
// output gives the same subtable.
func checkSharedRoundTrip(t *testing.T, read func(*parser.Parser, int64) (Subtable, error), data []byte) Subtable {
	t.Helper()
	sub := readSubtable(t, read, data)
	enc := sub.encode()
	if len(enc) != sub.encodeLen() {
		t.Fatalf("encode/encodeLen mismatch: %d vs %d", len(enc), sub.encodeLen())
	}
	if len(enc) > len(data) {
		t.Errorf("re-encoded subtable has %d bytes, input has %d", len(enc), len(data))
	}
	sub2 := readSubtable(t, read, enc)
	if diff := cmp.Diff(sub, sub2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	return sub
}

// TestGsub2_1SharedSequences checks that a subtable whose sequence offsets
// all reach the same Sequence table can be written back.  Without sharing,
// the output would need more than 64 KiB.
func TestGsub2_1SharedSequences(t *testing.T) {
	const n = 4000 // covered glyphs, all sharing one sequence
	const m = 400  // glyphs in the shared sequence
	seqPos := 6 + 2*n
	covPos := seqPos + 2 + 2*m
	data := &bytes.Buffer{}
	writeU16(data, 1) // format
	writeU16(data, uint16(covPos))
	writeU16(data, n)
	for range n {
		writeU16(data, uint16(seqPos))
	}
	writeU16(data, m)
	for j := range m {
		writeU16(data, uint16(j+1))
	}
	writeU16(data, 2) // coverage format 2: glyphs 1..n
	writeU16(data, 1)
	writeU16(data, 1)
	writeU16(data, n)
	writeU16(data, 0)

	sub := checkSharedRoundTrip(t, readGsub2_1, data.Bytes())
	if got := len(sub.(*Gsub2_1).Repl); got != n {
		t.Errorf("got %d sequences, want %d", got, n)
	}
}

// TestGsub2_1SequenceSharing checks that only sequences with identical
// content share a table.
func TestGsub2_1SequenceSharing(t *testing.T) {
	l := &Gsub2_1{
		Cov:  coverage.Table{1: 0, 2: 1, 3: 2},
		Repl: [][]glyph.ID{{5, 6}, {5}, {5, 6}},
	}
	enc := l.encode()
	// header 12, sequences 6+4, coverage 10
	if len(enc) != 32 || l.encodeLen() != 32 {
		t.Errorf("got %d bytes (encodeLen %d), want 32", len(enc), l.encodeLen())
	}
	sub := readSubtable(t, readGsub2_1, enc)
	if diff := cmp.Diff(Subtable(l), sub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
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

// TestGsubLongSubtables checks that a GSUB subtable longer than 64 KiB
// encodes and reads back unchanged, as long as every offset it stores fits in
// a uint16.  Each of these subtables ends in a coverage table which carries no
// offsets of its own, so only the offset reaching that table is bounded; the
// table itself may run past the limit.
func TestGsubLongSubtables(t *testing.T) {
	// glyphs spaced two apart, so the coverage table cannot collapse them
	// into a range and grows with the number of entries
	gid := func(i int) glyph.ID { return glyph.ID(2*i + 1) }
	cov := func(n int) coverage.Table {
		c := coverage.Table{}
		for i := range n {
			c[gid(i)] = i
		}
		return c
	}

	cases := []struct {
		name string
		make func() Subtable
		read func(*parser.Parser, int64) (Subtable, error)
	}{
		{"Gsub1_2", func() Subtable {
			const n = 32000
			repl := make([]glyph.ID, n)
			for i := range repl {
				repl[i] = gid(i) + 1
			}
			return &Gsub1_2{Cov: cov(n), SubstituteGlyphIDs: repl}
		}, readGsub1_2},

		{"Gsub2_1", func() Subtable {
			const n = 10000
			repl := make([][]glyph.ID, n)
			for i := range repl {
				repl[i] = []glyph.ID{gid(i) + 1}
			}
			return &Gsub2_1{Cov: cov(n), Repl: repl}
		}, readGsub2_1},

		{"Gsub3_1", func() Subtable {
			const n = 10000
			alt := make([][]glyph.ID, n)
			for i := range alt {
				alt[i] = []glyph.ID{gid(i) + 1}
			}
			return &Gsub3_1{Cov: cov(n), Alternates: alt}
		}, readGsub3_1},

		{"Gsub4_1", func() Subtable {
			const n = 5400
			repl := make([][]Ligature, n)
			for i := range repl {
				repl[i] = []Ligature{{In: []glyph.ID{gid(i) + 1}, Out: gid(i)}}
			}
			return &Gsub4_1{Cov: cov(n), Repl: repl}
		}, readGsub4_1},

		{"Gsub8_1", func() Subtable {
			// the lookahead coverage reached by the last offset is the one
			// which pushes the subtable past the limit
			const n = 2000
			repl := make([]glyph.ID, n)
			for i := range repl {
				repl[i] = gid(i) + 1
			}
			return &Gsub8_1{
				Input:              cov(n),
				Backtrack:          []coverage.Table{cov(10)},
				Lookahead:          []coverage.Table{cov(10), cov(30000)},
				SubstituteGlyphIDs: repl,
			}
		}, readGsub8_1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l1 := c.make()

			data := l1.encode()
			if len(data) != l1.encodeLen() {
				t.Errorf("encode/encodeLen mismatch: %d vs %d", len(data), l1.encodeLen())
			}
			if len(data) <= 0xFFFF {
				t.Fatalf("subtable is only %d bytes, the case does not test long subtables",
					len(data))
			}

			p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			if err := p.Discard(2); err != nil {
				t.Fatal(err)
			}
			l2, err := c.read(p, 0)
			if err != nil {
				t.Fatal(err)
			}
			if d := cmp.Diff(l1, l2); d != "" {
				t.Errorf("round trip (-want +got):\n%s", d)
			}
		})
	}
}

// TestGsub3_1SharedAlternateSets checks that a subtable whose alternate set
// offsets all reach the same AlternateSet table can be written back.
// Without sharing, the output would need more than 64 KiB.
func TestGsub3_1SharedAlternateSets(t *testing.T) {
	const n = 4000 // covered glyphs, all sharing one alternate set
	const m = 400  // glyphs in the shared alternate set
	setPos := 6 + 2*n
	covPos := setPos + 2 + 2*m
	data := &bytes.Buffer{}
	writeU16(data, 1) // format
	writeU16(data, uint16(covPos))
	writeU16(data, n)
	for range n {
		writeU16(data, uint16(setPos))
	}
	writeU16(data, m)
	for j := range m {
		writeU16(data, uint16(j+1))
	}
	writeU16(data, 2) // coverage format 2: glyphs 1..n
	writeU16(data, 1)
	writeU16(data, 1)
	writeU16(data, n)
	writeU16(data, 0)

	sub := checkSharedRoundTrip(t, readGsub3_1, data.Bytes())
	if got := len(sub.(*Gsub3_1).Alternates); got != n {
		t.Errorf("got %d alternate sets, want %d", got, n)
	}
}

// TestGsub3_1AlternateSetSharing checks that only alternate sets with
// identical content share a table.
func TestGsub3_1AlternateSetSharing(t *testing.T) {
	l := &Gsub3_1{
		Cov:        coverage.Table{1: 0, 2: 1, 3: 2},
		Alternates: [][]glyph.ID{{5, 6}, {5}, {5, 6}},
	}
	// header 12, alternate sets 6+4, coverage 10
	if enc := l.encode(); len(enc) != 32 || l.encodeLen() != 32 {
		t.Errorf("got %d bytes (encodeLen %d), want 32", len(enc), l.encodeLen())
	}
	sub := readSubtable(t, readGsub3_1, l.encode())
	if diff := cmp.Diff(Subtable(l), sub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestGsub3_1LayoutOverflow(t *testing.T) {
	l := &Gsub3_1{Cov: coverage.Table{}}
	for i := range 0x4000 {
		l.Cov[glyph.ID(i)] = i
		l.Alternates = append(l.Alternates, []glyph.ID{glyph.ID(i)})
	}
	if _, err := l.layout(); err == nil {
		t.Error("expected layout error")
	}
	assertPanics(t, func() { l.encode() })
}

// TestGsub4_1SharedLigatureSets checks that a subtable whose ligature set
// offsets all reach one LigatureSet, and whose ligature offsets all reach one
// Ligature, can be read and written back.  Without sharing, the output would
// need more than 64 KiB.
func TestGsub4_1SharedLigatureSets(t *testing.T) {
	const n = 1000 // covered glyphs, all sharing one ligature set
	const m = 100  // ligatures in the shared set, all sharing one table
	setPos := 6 + 2*n
	ligOff := 2 + 2*m
	covPos := setPos + ligOff + 10
	data := &bytes.Buffer{}
	writeU16(data, 1) // format
	writeU16(data, uint16(covPos))
	writeU16(data, n)
	for range n {
		writeU16(data, uint16(setPos))
	}
	writeU16(data, m)
	for range m {
		writeU16(data, uint16(ligOff))
	}
	writeU16(data, 9) // ligature glyph
	writeU16(data, 4) // component count
	writeU16(data, 1)
	writeU16(data, 2)
	writeU16(data, 3)
	writeU16(data, 2) // coverage format 2: glyphs 1..n
	writeU16(data, 1)
	writeU16(data, 1)
	writeU16(data, n)
	writeU16(data, 0)

	sub := checkSharedRoundTrip(t, readGsub4_1, data.Bytes())
	g := sub.(*Gsub4_1)
	if len(g.Repl) != n || len(g.Repl[0]) != m {
		t.Errorf("got %d sets of %d ligatures, want %d of %d",
			len(g.Repl), len(g.Repl[0]), n, m)
	}
}

// TestGsub4_1Sharing checks that only identical tables share, both for
// LigatureSets and for Ligatures within a set.
func TestGsub4_1Sharing(t *testing.T) {
	a := Ligature{In: []glyph.ID{2}, Out: 9}
	b := Ligature{In: []glyph.ID{3}, Out: 9}
	l := &Gsub4_1{
		Cov:  coverage.Table{1: 0, 2: 1, 3: 2},
		Repl: [][]Ligature{{a, b, a}, {a}, {a, b, a}},
	}
	// header 12; set {a,b,a}: 2+6+2*6=20; set {a}: 2+2+6=10; coverage 10
	if enc := l.encode(); len(enc) != 52 || l.encodeLen() != 52 {
		t.Errorf("got %d bytes (encodeLen %d), want 52", len(enc), l.encodeLen())
	}
	sub := readSubtable(t, readGsub4_1, l.encode())
	if diff := cmp.Diff(Subtable(l), sub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestGsub4_1LayoutOverflow(t *testing.T) {
	// distinct ligatures within one set
	var set []Ligature
	for i := range 0x4000 {
		set = append(set, Ligature{Out: glyph.ID(i)})
	}
	l := &Gsub4_1{Cov: coverage.Table{1: 0}, Repl: [][]Ligature{set}}
	if _, err := l.layout(); err == nil {
		t.Error("expected layout error for ligature offsets")
	}
	assertPanics(t, func() { l.encode() })

	// distinct ligature sets
	l = &Gsub4_1{Cov: coverage.Table{}}
	for i := range 0x4000 {
		l.Cov[glyph.ID(i)] = i
		l.Repl = append(l.Repl, []Ligature{{Out: glyph.ID(i)}})
	}
	if _, err := l.layout(); err == nil {
		t.Error("expected layout error for set offsets")
	}
	assertPanics(t, func() { l.encodeLen() })
}

// TestGsub8_1SharedCoverage checks that a subtable whose backtrack and
// lookahead offsets all reach the same coverage table can be written back.
// Without sharing, the output would need more than 64 KiB.
func TestGsub8_1SharedCoverage(t *testing.T) {
	const nb = 2000 // backtrack coverage tables
	const na = 1000 // lookahead coverage tables
	const k = 30    // glyphs in the shared coverage table
	covLen := 4 + 2*k
	hdr := 10 + 2*(nb+na+k)
	sharedPos := hdr + covLen // after the input coverage
	data := []byte{
		0, 1, // format
		byte(hdr >> 8), byte(hdr),
		byte(nb >> 8), byte(nb & 0xFF),
	}
	for range nb {
		data = append(data, byte(sharedPos>>8), byte(sharedPos))
	}
	data = append(data, byte(na>>8), byte(na&0xFF))
	for range na {
		data = append(data, byte(sharedPos>>8), byte(sharedPos))
	}
	data = append(data, 0, k)
	for j := range k {
		data = append(data, 0, byte(100+j)) // substitutes
	}
	// input coverage: glyphs 1..k, then the shared coverage: glyphs 1..k
	for range 2 {
		data = append(data, 0, 1, 0, k)
		for j := range k {
			data = append(data, 0, byte(j+1))
		}
	}

	sub := checkSharedRoundTrip(t, readGsub8_1, data)
	g := sub.(*Gsub8_1)
	if len(g.Backtrack) != nb || len(g.Lookahead) != na {
		t.Errorf("got %d+%d coverage tables, want %d+%d",
			len(g.Backtrack), len(g.Lookahead), nb, na)
	}
}

// TestGsub8_1CoverageSharing checks that only identical coverage tables
// share, including the input coverage.
func TestGsub8_1CoverageSharing(t *testing.T) {
	l := &Gsub8_1{
		Input:              coverage.Table{1: 0, 2: 1},
		Backtrack:          []coverage.Table{{1: 0, 2: 1}, {3: 0}},
		Lookahead:          []coverage.Table{{3: 0}, {4: 0}},
		SubstituteGlyphIDs: []glyph.ID{7, 8},
	}
	// header 10+2*(2+2+2)=22; coverages {1,2}: 8, {3}: 6, {4}: 6
	if enc := l.encode(); len(enc) != 42 || l.encodeLen() != 42 {
		t.Errorf("got %d bytes (encodeLen %d), want 42", len(enc), l.encodeLen())
	}
	sub := readSubtable(t, readGsub8_1, l.encode())
	if diff := cmp.Diff(Subtable(l), sub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestGsub8_1LayoutOverflow(t *testing.T) {
	l := &Gsub8_1{Input: coverage.Table{}}
	for i := range 0x2000 {
		l.Backtrack = append(l.Backtrack, coverage.Table{glyph.ID(i): 0})
	}
	if _, err := l.layout(); err == nil {
		t.Error("expected layout error")
	}
	assertPanics(t, func() { l.encode() })

	l = &Gsub8_1{Input: coverage.Table{}}
	for i := range 0x2000 {
		l.Lookahead = append(l.Lookahead, coverage.Table{glyph.ID(i): 0})
	}
	if _, err := l.layout(); err == nil {
		t.Error("expected layout error for lookahead")
	}

	l = &Gsub8_1{Input: coverage.Table{}, SubstituteGlyphIDs: make([]glyph.ID, 0x8000)}
	if _, err := l.layout(); err == nil {
		t.Error("expected layout error for input")
	}
}

func TestGsub1_1Layout(t *testing.T) {
	l := &Gsub1_1{Cov: coverage.Set{1: true, 2: true}, Delta: 3}
	enc := l.encode()
	if len(enc) != l.encodeLen() {
		t.Errorf("encode/encodeLen mismatch: %d vs %d", len(enc), l.encodeLen())
	}
	sub := readSubtable(t, readGsub1_1, enc)
	if diff := cmp.Diff(Subtable(l), sub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestGsub8_1PrunedInputSharesBacktrack checks that pruning the input
// coverage does not change a backtrack coverage stored at the same position.
func TestGsub8_1PrunedInputSharesBacktrack(t *testing.T) {
	data := &bytes.Buffer{}
	writeU16(data, 1)  // format
	writeU16(data, 14) // input coverage offset
	writeU16(data, 1)  // one backtrack coverage
	writeU16(data, 14)
	writeU16(data, 0) // no lookahead
	writeU16(data, 1) // one substitute
	writeU16(data, 7)
	writeU16(data, 1) // coverage format 1: glyphs 1, 2, 3
	writeU16(data, 3)
	writeU16(data, 1)
	writeU16(data, 2)
	writeU16(data, 3)

	sub := readSubtable(t, readGsub8_1, data.Bytes()).(*Gsub8_1)
	if want := (coverage.Table{1: 0}); !cmp.Equal(sub.Input, want) {
		t.Errorf("input coverage %v, want %v", sub.Input, want)
	}
	if want := (coverage.Table{1: 0, 2: 1, 3: 2}); !cmp.Equal(sub.Backtrack[0], want) {
		t.Errorf("backtrack coverage %v, want %v", sub.Backtrack[0], want)
	}
}
