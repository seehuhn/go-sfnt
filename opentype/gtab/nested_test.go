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
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
)

type debugNestedLookup struct {
	matchPos []int
	actions  SeqLookups
}

func (l *debugNestedLookup) Apply(_ keepGlyphFn, seq []glyph.Info, a, b int) *Match {
	if a != 0 {
		return &Match{
			InputPos: []int{a},
			Replace: []glyph.Info{
				{Gid: 3},
			},
			Next: a + 1,
		}
	}
	return &Match{
		InputPos: l.matchPos,
		Actions:  l.actions,
		Next:     l.matchPos[len(l.matchPos)-1] + 1,
	}
}

func (l *debugNestedLookup) EncodeLen() int {
	panic("unreachable")
}

func (l *debugNestedLookup) Encode() []byte {
	panic("unreachable")
}

// TestNestedSimple tests that the nested lookup works as expected
// when the nested lookups are single glyph substitutions.
func TestNestedSimple(t *testing.T) {
	type testCase struct {
		sequenceIndex []int
		out           []glyph.ID
	}
	cases := []testCase{
		{[]int{0}, []glyph.ID{2, 1, 1, 1, 1, 3, 3}},
		{[]int{1}, []glyph.ID{1, 1, 2, 1, 1, 3, 3}},
		{[]int{2}, []glyph.ID{1, 1, 1, 1, 2, 3, 3}},
		{[]int{3}, []glyph.ID{1, 1, 1, 1, 1, 3, 3}},
		{[]int{1, 2}, []glyph.ID{1, 1, 2, 1, 2, 3, 3}},
		{[]int{1, 3}, []glyph.ID{1, 1, 2, 1, 1, 3, 3}},
	}
	for _, test := range cases {
		var nested SeqLookups
		for _, seqenceIndex := range test.sequenceIndex {
			nested = append(nested, SeqLookup{
				SequenceIndex:   uint16(seqenceIndex),
				LookupListIndex: 1,
			})
		}
		info := &Info{
			LookupList: LookupList{
				{
					Meta: &LookupMetaInfo{},
					Subtables: []Subtable{
						&debugNestedLookup{
							matchPos: []int{0, 2, 4},
							actions:  nested,
						},
					},
				},
				{ // 1 -> 2
					Meta: &LookupMetaInfo{
						LookupType: 1,
					},
					Subtables: []Subtable{
						&Gsub1_1{
							Cov:   coverage.Table{1: 0},
							Delta: 1,
						},
					},
				},
			},
		}
		seq := []glyph.Info{
			{Gid: 1}, {Gid: 1}, {Gid: 1}, {Gid: 1}, {Gid: 1}, {Gid: 1}, {Gid: 1},
		}
		seq = info.LookupList.ApplyLookup(seq, 0, nil)
		var out []glyph.ID
		for _, g := range seq {
			out = append(out, g.Gid)
		}
		if diff := cmp.Diff(test.out, out); diff != "" {
			t.Error(diff)
		}
	}
}

func TestSeqContext1(t *testing.T) {
	in := []glyph.Info{{Gid: 1}, {Gid: 2}, {Gid: 3}, {Gid: 4}, {Gid: 99}, {Gid: 5}}
	l := &SeqContext1{
		Cov: map[glyph.ID]int{2: 0, 3: 1, 4: 2},
		Rules: [][]*SeqRule{
			{ // seq = 2, ...
				{Input: []glyph.ID{2}},
				{Input: []glyph.ID{3, 4, 6}},
				{Input: []glyph.ID{3, 4}},
				{Input: []glyph.ID{3, 4, 5}}, // does not match since it comes last
			},
			{ // seq = 3, ...
				{Input: []glyph.ID{3}},
				{Input: []glyph.ID{5}},
				{Input: []glyph.ID{4, 5, 6}},
			},
			{ // seq = 4, ...
				{Input: []glyph.ID{5, 6}},
				{Input: []glyph.ID{4}},
				{Input: []glyph.ID{5}},
			},
		},
	}
	keep := func(g glyph.ID) bool { return g < 50 }

	cases := []struct {
		before, after int
	}{
		{0, -1},
		{1, 5}, // matches 2, 3, 4, also skips 99
		{2, -1},
		{3, 6}, // matches 4, [99,] 5
		{4, -1},
		{5, -1},
	}
	for _, test := range cases {
		m := l.Apply(keep, in, test.before, len(in))
		next := -1
		if m != nil {
			next = m.Next
		}
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, m.Next, test.after)
		}
	}
}

func TestSeqContext2(t *testing.T) {
	in := []glyph.Info{{Gid: 1}, {Gid: 2}, {Gid: 3}, {Gid: 4}, {Gid: 99}, {Gid: 5}}
	l := &SeqContext2{
		Cov:   map[glyph.ID]int{2: 0, 3: 1, 4: 2, 99: 3},
		Input: classdef.Table{1: 1, 3: 1, 5: 1},
		Rules: [][]*ClassSeqRule{
			{ // seq = class0, ...
				{Input: []uint16{1, 0}},
				{Input: []uint16{1}},
			},
			{ // seq = class1, ...
				{Input: []uint16{1}},
				{Input: []uint16{0, 1, 0}},
			},
		},
	}
	keep := func(g glyph.ID) bool { return g < 50 }

	cases := []struct {
		before, after int
	}{
		{0, -1}, // not in coverage table
		{1, 5},  // matches class0, class1, class0, also skips 99
		{2, -1}, // no match for class1, class0, class1
		{3, 6},  // matches 4, [99,] 5
		{4, -1}, // keep returns false
		{5, -1}, // not in coverage table
	}
	for _, test := range cases {
		m := l.Apply(keep, in, test.before, len(in))
		next := -1
		if keep(in[test.before].Gid) && m != nil {
			next = m.Next
		}
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, m.Next, test.after)
		}
	}
}

func TestSeqContext3(t *testing.T) {
	in := []glyph.Info{{Gid: 1}, {Gid: 2}, {Gid: 3}, {Gid: 4}, {Gid: 99}, {Gid: 5}}
	l := &SeqContext3{
		Input: []coverage.Table{
			{1: 0, 3: 1, 4: 2},
			{2: 0, 4: 1, 5: 2},
			{3: 0, 5: 1},
		},
	}
	keep := func(g glyph.ID) bool { return g < 50 }

	cases := []struct {
		before, after int
	}{
		{0, 3}, // matches 1, 2, 3
		{1, -1},
		{2, 6}, // matches 3, 4, [99,] 5
		{3, -1},
		{4, -1},
		{5, -1},
	}
	for _, test := range cases {
		m := l.Apply(keep, in, test.before, len(in))
		next := -1
		if m != nil {
			next = m.Next
		}
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, m.Next, test.after)
		}
	}
}

func TestChainedSeqContext1(t *testing.T) {
	in := []glyph.Info{
		{Gid: 1}, {Gid: 99}, {Gid: 2}, {Gid: 99}, {Gid: 3}, {Gid: 4}, {Gid: 99}, {Gid: 5},
	}
	l := &ChainedSeqContext1{
		Cov: map[glyph.ID]int{2: 0, 3: 1, 4: 2},
		Rules: [][]*ChainedSeqRule{
			{ // seq = 2, ...
				{
					Input: []glyph.ID{2},
				},
				{
					Input:     []glyph.ID{3, 4},
					Lookahead: []glyph.ID{99},
				},
				{
					Input:     []glyph.ID{3, 4, 5},
					Backtrack: []glyph.ID{2},
				},
			},
			{ // seq = 3, ...
				{
					Input:     []glyph.ID{4},
					Lookahead: []glyph.ID{5},
					Backtrack: []glyph.ID{2, 1},
				},
			},
			{ // seq = 4, ...
			},
		},
	}
	keep := func(g glyph.ID) bool { return g < 50 }

	cases := []struct {
		before, after int
	}{
		{0, -1},
		{1, -1},
		{2, -1},
		{3, -1},
		{4, 7}, // matches [1, 2,] 3, 4, [5], also skips 99
	}
	for _, test := range cases {
		m := l.Apply(keep, in, test.before, len(in))
		next := -1
		if m != nil {
			next = m.Next
		}
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, m.Next, test.after)
		}
	}
}

func FuzzSeqContext1(f *testing.F) {
	sub := &SeqContext1{}
	f.Add(sub.Encode())
	sub.Cov = coverage.Table{3: 0, 5: 1}
	sub.Rules = [][]*SeqRule{
		{},
		{},
	}
	f.Add(sub.Encode())
	sub.Rules = [][]*SeqRule{
		{
			{
				Input: []glyph.ID{4},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 1},
					{SequenceIndex: 1, LookupListIndex: 5},
					{SequenceIndex: 0, LookupListIndex: 4},
				},
			},
		},
		{
			{
				Input: []glyph.ID{6, 7},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 2},
				},
			},
			{
				Input: []glyph.ID{6},
				Actions: []SeqLookup{
					{SequenceIndex: 2, LookupListIndex: 1},
					{SequenceIndex: 1, LookupListIndex: 2},
					{SequenceIndex: 0, LookupListIndex: 3},
				},
			},
		},
	}
	f.Add(sub.Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 1, readSeqContext1, data)
	})
}

func FuzzSeqContext2(f *testing.F) {
	sub := &SeqContext2{}
	f.Add(sub.Encode())
	sub.Cov = coverage.Table{3: 0, 5: 1}
	sub.Rules = [][]*ClassSeqRule{
		{},
		{},
	}
	f.Add(sub.Encode())
	sub.Rules = [][]*ClassSeqRule{
		{
			{
				Input: []uint16{4},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 1},
					{SequenceIndex: 1, LookupListIndex: 5},
					{SequenceIndex: 0, LookupListIndex: 4},
				},
			},
		},
		{
			{
				Input: []uint16{6, 7},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 2},
				},
			},
			{
				Input: []uint16{6},
				Actions: []SeqLookup{
					{SequenceIndex: 2, LookupListIndex: 1},
					{SequenceIndex: 1, LookupListIndex: 2},
					{SequenceIndex: 0, LookupListIndex: 3},
				},
			},
		},
	}
	f.Add(sub.Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 2, readSeqContext2, data)
	})
}

func FuzzSeqContext3(f *testing.F) {
	sub := &SeqContext3{}
	f.Add(sub.Encode())
	sub.Input = append(sub.Input, coverage.Table{3: 0, 4: 1})
	sub.Actions = []SeqLookup{
		{SequenceIndex: 0, LookupListIndex: 1},
		{SequenceIndex: 1, LookupListIndex: 5},
		{SequenceIndex: 0, LookupListIndex: 4},
	}
	f.Add(sub.Encode())
	sub.Input = append(sub.Input, coverage.Table{1: 0, 3: 1, 5: 2})
	f.Add(sub.Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 3, readSeqContext3, data)
	})
}

func FuzzChainedSeqContext1(f *testing.F) {
	sub := &ChainedSeqContext1{}
	f.Add(sub.Encode())
	sub.Cov = coverage.Table{1: 0, 3: 1}
	sub.Rules = [][]*ChainedSeqRule{
		{
			{
				Backtrack: []glyph.ID{},
				Input:     []glyph.ID{1},
				Lookahead: []glyph.ID{2, 3},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 1},
					{SequenceIndex: 0, LookupListIndex: 2},
				},
			},
			{
				Backtrack: []glyph.ID{4, 5, 6},
				Input:     []glyph.ID{7, 8},
				Lookahead: []glyph.ID{9},
				Actions: []SeqLookup{
					{SequenceIndex: 1, LookupListIndex: 0},
				},
			},
			{
				Backtrack: []glyph.ID{10, 11},
				Input:     []glyph.ID{12},
				Lookahead: []glyph.ID{},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 1000},
				},
			},
		},
		{
			{
				Backtrack: []glyph.ID{},
				Input:     []glyph.ID{13},
				Lookahead: []glyph.ID{},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 1},
					{SequenceIndex: 0, LookupListIndex: 2},
					{SequenceIndex: 0, LookupListIndex: 3},
				},
			},
		},
	}
	f.Add(sub.Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 1, readChainedSeqContext1, data)
	})
}

func FuzzChainedSeqContext2(f *testing.F) {
	sub := &ChainedSeqContext2{}
	f.Add(sub.Encode())
	sub.Cov = coverage.Table{1: 0, 3: 1}
	sub.Backtrack = classdef.Table{2: 1, 3: 1, 4: 2}
	sub.Input = classdef.Table{3: 1, 4: 2}
	sub.Lookahead = classdef.Table{3: 1, 4: 2, 5: 2}
	sub.Rules = [][]*ChainedClassSeqRule{
		{
			{
				Backtrack: []uint16{},
				Input:     []uint16{1},
				Lookahead: []uint16{2, 3},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 1},
					{SequenceIndex: 0, LookupListIndex: 2},
				},
			},
			{
				Backtrack: []uint16{4, 5, 6},
				Input:     []uint16{7, 8},
				Lookahead: []uint16{9},
				Actions: []SeqLookup{
					{SequenceIndex: 1, LookupListIndex: 0},
				},
			},
			{
				Backtrack: []uint16{10, 11},
				Input:     []uint16{12},
				Lookahead: []uint16{},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 1000},
				},
			},
		},
		{
			{
				Backtrack: []uint16{},
				Input:     []uint16{13},
				Lookahead: []uint16{},
				Actions: []SeqLookup{
					{SequenceIndex: 0, LookupListIndex: 1},
					{SequenceIndex: 0, LookupListIndex: 2},
					{SequenceIndex: 0, LookupListIndex: 3},
				},
			},
		},
	}
	f.Add(sub.Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 2, readChainedSeqContext2, data)
	})
}

func FuzzChainedSeqContext3(f *testing.F) {
	sub := &ChainedSeqContext3{}
	f.Add(sub.Encode())
	sub.Backtrack = []coverage.Set{
		{1: true, 3: true},
	}
	sub.Input = []coverage.Set{
		{2: true, 3: true},
		{3: true, 4: true},
	}
	sub.Lookahead = []coverage.Set{
		{4: true, 5: true, 6: true},
	}
	sub.Actions = []SeqLookup{
		{SequenceIndex: 0, LookupListIndex: 1},
		{SequenceIndex: 0, LookupListIndex: 2},
	}
	f.Add(sub.Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 3, readChainedSeqContext3, data)
	})
}
