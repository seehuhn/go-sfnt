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
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/parser"
)

// The following is a list of all OpenType lookup types.
var (
	_ Subtable = (*Gsub1_1)(nil)
	_ Subtable = (*Gsub1_2)(nil)
	_ Subtable = (*Gsub2_1)(nil)
	_ Subtable = (*Gsub3_1)(nil)
	_ Subtable = (*Gsub4_1)(nil)
	_ Subtable = (*Gsub8_1)(nil)

	_ Subtable = (*Gpos1_1)(nil)
	_ Subtable = (*Gpos1_2)(nil)
	_ Subtable = (*Gpos2_1)(nil)
	_ Subtable = (*Gpos2_2)(nil)
	_ Subtable = (*Gpos3_1)(nil)
	_ Subtable = (*Gpos4_1)(nil)
	_ Subtable = (*Gpos5_1)(nil)
	_ Subtable = (*Gpos6_1)(nil)

	_ Subtable = (*SeqContext1)(nil)
	_ Subtable = (*SeqContext2)(nil)
	_ Subtable = (*SeqContext3)(nil)
	_ Subtable = (*ChainedSeqContext1)(nil)
	_ Subtable = (*ChainedSeqContext2)(nil)
	_ Subtable = (*ChainedSeqContext3)(nil)

	_ Subtable = (*debugNestedLookup)(nil)
	_ Subtable = (*dummySubtable)(nil)
)

// TestLookupFlags tests that the lookup flags cause the correct glyphs to be
// ignored.
func TestLookupFlags(t *testing.T) {
	// The glyphs used in our test.
	const (
		repl glyph.ID = iota + 1 // We set this up as a ligature between the first and last glyph in the sequence.

		// The following glyphs are can be ignored by the lookup flags.
		A
		B
		C
		mark1
		mark2
		mark3
		mark4
		lig1
		lig2
	)

	// Assign glyphs to the various classes.
	gdefTable := &gdef.Table{
		GlyphClass: classdef.Table{
			A:     gdef.GlyphClassBase,
			B:     gdef.GlyphClassBase,
			C:     gdef.GlyphClassBase,
			mark1: gdef.GlyphClassMark,
			mark2: gdef.GlyphClassMark,
			mark3: gdef.GlyphClassMark,
			mark4: gdef.GlyphClassMark,
			lig1:  gdef.GlyphClassLigature,
			lig2:  gdef.GlyphClassLigature,
		},
		MarkAttachClass: classdef.Table{
			mark1: 1,
			mark2: 2,
			mark3: 2,
			mark4: 1,
		},
		MarkGlyphSets: []coverage.Set{
			{mark1: true, mark2: true},
			{mark1: true, mark3: true},
		},
	}

	type testCase struct {
		in          []glyph.ID
		flags       LookupFlags
		set         uint16
		shouldMerge bool
	}
	cases := []testCase{
		{in: []glyph.ID{A, B}, flags: 0, shouldMerge: true},
		{in: []glyph.ID{A, A, B}, flags: 0, shouldMerge: false},
		{in: []glyph.ID{A, mark1, B}, flags: 0, shouldMerge: false},
		{in: []glyph.ID{A, repl, B}, flags: 0, shouldMerge: false},

		{in: []glyph.ID{mark1, mark2}, flags: IgnoreBaseGlyphs, shouldMerge: true},
		{in: []glyph.ID{mark1, A, B, mark2}, flags: IgnoreBaseGlyphs, shouldMerge: true},
		{in: []glyph.ID{mark1, lig1, mark1}, flags: IgnoreBaseGlyphs, shouldMerge: false},
		{in: []glyph.ID{mark1, lig1, mark2}, flags: IgnoreBaseGlyphs, shouldMerge: false},
		{in: []glyph.ID{A, B}, flags: IgnoreBaseGlyphs, shouldMerge: false},
		{in: []glyph.ID{A, B, C}, flags: IgnoreBaseGlyphs, shouldMerge: false},

		{in: []glyph.ID{mark1, mark2}, flags: IgnoreLigatures, shouldMerge: true},
		{in: []glyph.ID{mark1, lig1, lig2, mark2}, flags: IgnoreLigatures, shouldMerge: true},
		{in: []glyph.ID{lig1, lig2}, flags: IgnoreLigatures, shouldMerge: false},

		{in: []glyph.ID{A, B}, flags: IgnoreMarks, shouldMerge: true},
		{in: []glyph.ID{A, mark1, mark2, B}, flags: IgnoreMarks, shouldMerge: true},
		{in: []glyph.ID{mark1, mark2}, flags: IgnoreMarks, shouldMerge: false},

		// mark filtering set 0 keeps mark1 and mark2, and ignores mark3, mark4
		{in: []glyph.ID{mark1, mark2}, flags: UseMarkFilteringSet, set: 0, shouldMerge: true},
		{in: []glyph.ID{mark1, mark3, mark2}, flags: UseMarkFilteringSet, set: 0, shouldMerge: true},
		{in: []glyph.ID{mark1, mark3, mark3, mark1}, flags: UseMarkFilteringSet, set: 0, shouldMerge: true},
		{in: []glyph.ID{mark1, mark3, mark2, mark3, mark1}, flags: UseMarkFilteringSet, set: 0, shouldMerge: false},

		// mark filtering set 1 keeps mark1 and mark3, and ignores mark2, mark4
		{in: []glyph.ID{mark1, mark3}, flags: UseMarkFilteringSet, set: 1, shouldMerge: true},
		{in: []glyph.ID{mark1, mark2, mark3}, flags: UseMarkFilteringSet, set: 1, shouldMerge: true},
		{in: []glyph.ID{mark1, mark2, mark2, mark1}, flags: UseMarkFilteringSet, set: 1, shouldMerge: true},
		{in: []glyph.ID{mark1, mark2, mark3, mark2, mark1}, flags: UseMarkFilteringSet, set: 1, shouldMerge: false},

		// attachment type 1 ignores mark2, mark3.  All other glyphs, including mark1 and mark4, are kept.
		{in: []glyph.ID{mark1, mark1}, flags: 1 << 8, shouldMerge: true},
		{in: []glyph.ID{mark1, mark2, mark1}, flags: 1 << 8, shouldMerge: true},
		{in: []glyph.ID{mark1, mark4, mark1}, flags: 1 << 8, shouldMerge: false},
		{in: []glyph.ID{mark1, A, mark1}, flags: 1 << 8, shouldMerge: false},

		// attachment type 2 ignores mark1, mark4.  All other glyphs, including mark2 and mark3, are kept.
		{in: []glyph.ID{mark2, mark3}, flags: 2 << 8, shouldMerge: true},
		{in: []glyph.ID{mark2, mark1, mark4, mark3}, flags: 2 << 8, shouldMerge: true},
		{in: []glyph.ID{mark2, mark3, mark2}, flags: 2 << 8, shouldMerge: false},
		{in: []glyph.ID{mark2, A, mark2}, flags: 2 << 8, shouldMerge: false},

		// Finally, we test a few combinations of flags:
		{in: []glyph.ID{A, B}, flags: IgnoreMarks | IgnoreLigatures, shouldMerge: true},
		{in: []glyph.ID{A, mark1, lig2, B}, flags: IgnoreMarks | IgnoreLigatures, shouldMerge: true},
		{in: []glyph.ID{A, B, C}, flags: IgnoreMarks | IgnoreLigatures, shouldMerge: false},
		{in: []glyph.ID{mark1, A, mark2, B, mark3}, flags: IgnoreBaseGlyphs | UseMarkFilteringSet, set: 1, shouldMerge: true},
		{in: []glyph.ID{mark1, A, mark3, B, mark1}, flags: IgnoreBaseGlyphs | UseMarkFilteringSet, set: 1, shouldMerge: false},
		{in: []glyph.ID{mark2, mark3}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: true},
		{in: []glyph.ID{mark2, A, mark3}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: true},
		{in: []glyph.ID{mark2, mark1, mark3}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: true},
		{in: []glyph.ID{mark2, A, B, C, mark1, mark4, mark3}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: true},
		{in: []glyph.ID{mark2, A, mark4, mark3}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: true},
		{in: []glyph.ID{mark2, mark2}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: true},
		{in: []glyph.ID{mark2, lig1, mark2}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: false},
		{in: []glyph.ID{mark2, mark3, mark2}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: false},
		{in: []glyph.ID{mark2, repl, mark2}, flags: IgnoreBaseGlyphs | (2 << 8), shouldMerge: false},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("c%02d", i), func(t *testing.T) {
			// Construct a lookup table that combines the first and last glyph
			// in the sequence into `repl` and which has the specified flags
			// and mark filtering set.
			lookupTable := &LookupTable{
				Meta: &LookupMetaInfo{
					LookupType:       4, // ligature substitution
					LookupFlags:      c.flags,
					MarkFilteringSet: c.set,
				},
				Subtables: []Subtable{
					&Gsub4_1{ // combine the pair of the first and last glyph into repl
						Cov: coverage.Table{c.in[0]: 0},
						Repl: [][]Ligature{
							{{In: []glyph.ID{c.in[len(c.in)-1]}, Out: repl}},
						},
					},
				},
			}
			lookupList := LookupList{lookupTable}

			seq := make([]glyph.Info, len(c.in))
			for i, g := range c.in {
				seq[i].GID = g
			}
			e := NewContext(lookupList, gdefTable, []LookupIndex{0})
			seq = e.Apply(seq)

			hasMerged := seq[0].GID == repl
			if hasMerged && !c.shouldMerge {
				t.Errorf("test %d: lookup flags %v/0x%02x: merged when it should not",
					i, c.in, c.flags)
			} else if !hasMerged && c.shouldMerge {
				t.Errorf("test %d: lookup flags %v/0x%02x: did not merge when it should",
					i, c.in, c.flags)
			}
		})
	}
}

func FuzzLookupList(f *testing.F) {
	l := LookupList{
		&LookupTable{
			Meta: &LookupMetaInfo{},
			Subtables: []Subtable{
				dummySubtable{},
			},
		},
	}
	f.Add(l.encode())

	l = LookupList{
		&LookupTable{
			Meta: &LookupMetaInfo{
				LookupType:  4,
				LookupFlags: UseMarkFilteringSet,
			},
			Subtables: []Subtable{
				dummySubtable{1, 2, 3, 4},
			},
		},
	}
	f.Add(l.encode())

	l = LookupList{
		&LookupTable{
			Meta: &LookupMetaInfo{
				LookupType: 1,
			},
			Subtables: []Subtable{
				dummySubtable{0},
				dummySubtable{1},
				dummySubtable{2},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{
				LookupType:       2,
				LookupFlags:      UseMarkFilteringSet,
				MarkFilteringSet: 7,
			},
			Subtables: []Subtable{
				dummySubtable{3, 4},
				dummySubtable{5, 6},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{
				LookupType: 3,
			},
			Subtables: []Subtable{
				dummySubtable{7, 8, 9},
			},
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data1 []byte) {
		p := parser.New(bytes.NewReader(data1), parser.NewBudget(int64(len(data1))))
		l1, err := readLookupList(p, 0, readDummySubtable)
		if err != nil {
			return
		}

		data2 := l1.encode()

		p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
		l2, err := readLookupList(p, 0, readDummySubtable)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(l1, l2) {
			fmt.Printf("A % x\n", data1)
			fmt.Printf("B % x\n", data2)
			fmt.Println(l1)
			fmt.Println(l2)
			t.Fatal("different")
		}
	})
}

func readDummySubtable(p *parser.Parser, pos int64, info *LookupMetaInfo) (Subtable, error) {
	if info.LookupType > 32 {
		return nil, errors.New("invalid type for dummy lookup")
	}
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}
	res := make(dummySubtable, info.LookupType)
	_, err = p.Read(res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

type dummySubtable []byte

func (st dummySubtable) apply(ctx *Context, a, b int) int {
	return -1
}

func (st dummySubtable) encodeLen() int {
	return len(st)
}

func (st dummySubtable) encode() []byte {
	return []byte(st)
}

// TestLookupListLayout checks the contract of [LookupList.encode] for lookup
// lists which do not fit the 16-bit offsets their own format uses: a reader
// gets back exactly the list which was encoded.
//
// How the encoder achieves this is its own business — it may reorder the
// lookups and route subtables through extension records — but the lookup
// types, the subtables and their order must all survive, and a second encode
// of what the reader returns must reproduce the same bytes.
func TestLookupListLayout(t *testing.T) {
	cases := []struct {
		name              string
		lookups, subtable int
		glyphs            int
	}{
		{"within reach", 2, 1, 100},
		{"subtables out of reach of their lookup", 1, 3, 12000},
		{"lookup tables out of reach", 6, 1, 12000},
		{"both out of reach", 4, 2, 12000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := largeLookupList(c.lookups, c.subtable, c.glyphs)

			data := want.encode()
			big := len(data) > 0xFFFF
			if big != (c.glyphs > 1000) {
				t.Fatalf("lookup list is %d bytes; the case does not test what it means to",
					len(data))
			}

			got := decodeLookupList(t, data)
			if d := cmp.Diff(want, got); d != "" {
				t.Errorf("round trip (-want +got):\n%s", d)
			}

			if again := got.encode(); !bytes.Equal(data, again) {
				t.Errorf("re-encoding the decoded list gives %d bytes, want the same %d",
					len(again), len(data))
			}
		})
	}
}

// TestLookupListSubtableOrder checks that reordering the layout leaves the
// lookups and their subtables where the caller put them: a lookup list is
// addressed by index, and applying a feature to lookup 3 must still reach the
// third lookup after a layout which moved its bytes elsewhere in the table.
func TestLookupListSubtableOrder(t *testing.T) {
	// six lookups of distinct sizes, so the layout has a genuine choice about
	// which to move and which to replace
	ll := make(LookupList, 6)
	for i := range ll {
		ll[i] = &LookupTable{
			Meta:      &LookupMetaInfo{LookupType: 1, LookupFlags: LookupFlags(i)},
			Subtables: []Subtable{bigGsub(2000*(i+1), glyph.ID(100*i))},
		}
	}

	data := ll.encode()
	if len(data) <= 0xFFFF {
		t.Fatalf("lookup list is only %d bytes", len(data))
	}
	got := decodeLookupList(t, data)

	if len(got) != len(ll) {
		t.Fatalf("got %d lookups, want %d", len(got), len(ll))
	}
	for i := range ll {
		if got[i].Meta.LookupFlags != ll[i].Meta.LookupFlags {
			t.Errorf("lookup %d has flags %d, want %d",
				i, got[i].Meta.LookupFlags, ll[i].Meta.LookupFlags)
		}
		want := ll[i].Subtables[0].(*Gsub1_2)
		sub, ok := got[i].Subtables[0].(*Gsub1_2)
		if !ok {
			t.Errorf("lookup %d holds a %T, want *Gsub1_2", i, got[i].Subtables[0])
			continue
		}
		if d := cmp.Diff(want, sub); d != "" {
			t.Errorf("lookup %d subtable (-want +got):\n%s", i, d)
		}
	}
}

// decodeLookupList reads back an encoded lookup list of GSUB subtables.
func decodeLookupList(t *testing.T, data []byte) LookupList {
	t.Helper()

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	ll, err := readLookupList(p, 0, readGsubSubtable)
	if err != nil {
		t.Fatal(err)
	}
	return ll
}

// bigGsub returns a single substitution subtable covering n glyphs from first.
// Its encoded size is a little over 4*n bytes.
func bigGsub(n int, first glyph.ID) *Gsub1_2 {
	cov := coverage.Table{}
	repl := make([]glyph.ID, n)
	for i := range repl {
		cov[first+glyph.ID(2*i)] = i
		repl[i] = first + glyph.ID(2*i) + 1
	}
	return &Gsub1_2{Cov: cov, SubstituteGlyphIDs: repl}
}

// largeLookupList builds a lookup list of the given shape.  The lookups differ
// in size, so that the layout has to choose between them rather than finding
// them interchangeable.
func largeLookupList(lookups, subtables, glyphs int) LookupList {
	ll := make(LookupList, lookups)
	for i := range ll {
		ss := make([]Subtable, subtables)
		for j := range ss {
			ss[j] = bigGsub(glyphs+100*i+10*j, glyph.ID(1))
		}
		ll[i] = &LookupTable{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: ss,
		}
	}
	return ll
}

// TestLookupListMovesLargestLookup covers a lookup list whose first lookup
// dwarfs the rest: every lookup after it starts out beyond the reach of the
// offset addressing it, yet each of its own subtables is close enough to its
// lookup table that extension records are not called for.  Making room by
// rearranging the layout is then the only way to encode the list, and the
// encoder is required to find it — a lookup list of this shape is not an
// error.
func TestLookupListMovesLargestLookup(t *testing.T) {
	// two subtables of just under 64 KiB each: together past the reach of an
	// Offset16, individually within it
	ll := LookupList{
		&LookupTable{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{bigGsub(16000, 1), bigGsub(16000, 33000)},
		},
		lookupOf(bigGsub(50, 1)),
		lookupOf(bigGsub(50, 201)),
		lookupOf(bigGsub(50, 401)),
	}
	first, second := ll[0].Subtables[0].encodeLen(), ll[0].Subtables[1].encodeLen()
	if first+second <= 0xFFFF {
		t.Fatalf("the first lookup is only %d bytes; it does not dominate", first+second)
	}
	if 6+first > 0xFFFF {
		t.Fatalf("the second subtable sits at %d and would need an extension record",
			6+first)
	}

	data := ll.encode()
	got := decodeLookupList(t, data)
	if d := cmp.Diff(ll, got); d != "" {
		t.Errorf("round trip (-want +got):\n%s", d)
	}
}

// lookupOf wraps a single subtable in a lookup table.
func lookupOf(st Subtable) *LookupTable {
	return &LookupTable{
		Meta:      &LookupMetaInfo{LookupType: 1},
		Subtables: []Subtable{st},
	}
}
