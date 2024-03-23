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
	// _ Subtable = (*Gpos5_1)(nil) // TODO(voss): implement this
	_ Subtable = (*Gpos6_1)(nil)

	_ Subtable = (*SeqContext1)(nil)
	_ Subtable = (*SeqContext2)(nil)
	_ Subtable = (*SeqContext3)(nil)
	_ Subtable = (*ChainedSeqContext1)(nil)
	_ Subtable = (*ChainedSeqContext2)(nil)
	_ Subtable = (*ChainedSeqContext3)(nil)

	_ Subtable = notImplementedGposSubtable{}
	_ Subtable = (*debugNestedLookup)(nil)
	_ Subtable = (*dummySubTable)(nil)
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
			e := lookupList.NewContext([]LookupIndex{0}, gdefTable)
			seq = e.ApplyAll(seq)

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
				dummySubTable{},
			},
		},
	}
	f.Add(l.encode(999))

	l = LookupList{
		&LookupTable{
			Meta: &LookupMetaInfo{
				LookupType:  4,
				LookupFlags: UseMarkFilteringSet,
			},
			Subtables: []Subtable{
				dummySubTable{1, 2, 3, 4},
			},
		},
	}
	f.Add(l.encode(999))

	l = LookupList{
		&LookupTable{
			Meta: &LookupMetaInfo{
				LookupType: 1,
			},
			Subtables: []Subtable{
				dummySubTable{0},
				dummySubTable{1},
				dummySubTable{2},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{
				LookupType:       2,
				LookupFlags:      UseMarkFilteringSet,
				MarkFilteringSet: 7,
			},
			Subtables: []Subtable{
				dummySubTable{3, 4},
				dummySubTable{5, 6},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{
				LookupType: 3,
			},
			Subtables: []Subtable{
				dummySubTable{7, 8, 9},
			},
		},
	}
	f.Add(l.encode(999))

	f.Fuzz(func(t *testing.T, data1 []byte) {
		p := parser.New(bytes.NewReader(data1))
		l1, err := readLookupList(p, 0, readDummySubtable)
		if err != nil {
			return
		}

		data2 := l1.encode(999)

		p = parser.New(bytes.NewReader(data2))
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
	res := make(dummySubTable, info.LookupType)
	_, err = p.Read(res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

type dummySubTable []byte

func (st dummySubTable) Apply(ctx *Context, a, b int) int {
	return -1
}

func (st dummySubTable) EncodeLen() int {
	return len(st)
}

func (st dummySubTable) Encode() []byte {
	return []byte(st)
}
