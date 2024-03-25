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
	"seehuhn.de/go/sfnt/opentype/gdef"
)

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
		var nested []SeqLookup
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
							Cov:   coverage.Set{1: true},
							Delta: 1,
						},
					},
				},
			},
		}
		seq := []glyph.Info{
			{GID: 1}, {GID: 1}, {GID: 1}, {GID: 1}, {GID: 1}, {GID: 1}, {GID: 1},
		}
		e := NewContext(info.LookupList, nil, []LookupIndex{0})
		seq = e.Apply(seq)
		var out []glyph.ID
		for _, g := range seq {
			out = append(out, g.GID)
		}
		if diff := cmp.Diff(test.out, out); diff != "" {
			t.Error(diff)
		}
	}
}

func TestSeqContext1(t *testing.T) {
	in := []glyph.Info{{GID: 1}, {GID: 2}, {GID: 3}, {GID: 4}, {GID: 99}, {GID: 5}}
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
	keep := makeDebugKeepFunc()

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
	ctx := &Context{seq: in, keep: keep}
	for _, test := range cases {
		next := l.Apply(ctx, test.before, len(in))
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, next, test.after)
		}
	}
}

func BenchmarkSeqContext1(b *testing.B) {
	l0 := &Gsub1_1{
		Cov:   coverage.Set{1: true, 2: true},
		Delta: 1,
	}
	l1 := &SeqContext1{
		Cov: map[glyph.ID]int{1: 0, 2: 1},
		Rules: [][]*SeqRule{
			{ // seq = 1, ...
				{
					Input:   []glyph.ID{1, 2, 2},
					Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
				{
					Input:   []glyph.ID{2, 2, 1},
					Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
			},
			{ // seq = 2, ...
				{
					Input:   []glyph.ID{2, 1, 1},
					Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
				{
					Input:   []glyph.ID{1, 1, 2},
					Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
			},
		},
	}
	ll := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{l0},
		},
		{
			Meta:      &LookupMetaInfo{LookupType: 5},
			Subtables: []Subtable{l1},
		},
	}
	var seq []glyph.Info
	ctx := NewContext(ll, nil, []LookupIndex{1})

	for _, gid := range []glyph.ID{1, 2, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2} {
		seq = append(seq, glyph.Info{GID: gid})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Apply(seq)
	}
}

func FuzzSeqContext1(f *testing.F) {
	sub := &SeqContext1{}
	f.Add(sub.encode())
	sub.Cov = coverage.Table{3: 0, 5: 1}
	sub.Rules = [][]*SeqRule{
		{},
		{},
	}
	f.Add(sub.encode())
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
	f.Add(sub.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 1, readSeqContext1, data)
	})
}

func TestSeqContext2(t *testing.T) {
	in := []glyph.Info{{GID: 1}, {GID: 2}, {GID: 3}, {GID: 4}, {GID: 99}, {GID: 5}}
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
	keep := makeDebugKeepFunc()

	cases := []struct {
		before, after int
	}{
		{0, -1}, // not in coverage table
		{1, 5},  // matches class0, class1, class0, also skips 99
		{2, -1}, // no match for class1, class0, class1
		{3, 6},  // matches 4, [99,] 5
		{5, -1}, // not in coverage table
	}
	ctx := &Context{seq: in, keep: keep}
	for _, test := range cases {
		next := l.Apply(ctx, test.before, len(in))
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, next, test.after)
		}
	}
}

func BenchmarkSeqContext2(b *testing.B) {
	l0 := &Gsub1_1{
		Cov:   coverage.Set{1: true, 2: true},
		Delta: 1,
	}
	l1 := &SeqContext2{
		Cov:   map[glyph.ID]int{1: 0, 2: 1},
		Input: classdef.Table{1: 1, 2: 1},
		Rules: [][]*ClassSeqRule{
			{ // seq = 1, ...
				{
					Input:   []uint16{1, 2, 2},
					Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
				{
					Input:   []uint16{2, 2, 1},
					Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
			},
			{ // seq = 2, ...
				{
					Input:   []uint16{2, 1, 1},
					Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
				{
					Input:   []uint16{1, 1, 2},
					Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
			},
		},
	}
	ll := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{l0},
		},
		{
			Meta:      &LookupMetaInfo{LookupType: 5},
			Subtables: []Subtable{l1},
		},
	}
	var seq []glyph.Info
	ctx := NewContext(ll, nil, []LookupIndex{1})

	for _, gid := range []glyph.ID{1, 2, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2} {
		seq = append(seq, glyph.Info{GID: gid})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Apply(seq)
	}
}

func FuzzSeqContext2(f *testing.F) {
	sub := &SeqContext2{}
	f.Add(sub.encode())
	sub.Cov = coverage.Table{3: 0, 5: 1}
	sub.Rules = [][]*ClassSeqRule{
		{},
		{},
	}
	f.Add(sub.encode())
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
	f.Add(sub.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 2, readSeqContext2, data)
	})
}

func TestSeqContext3(t *testing.T) {
	in := []glyph.Info{{GID: 1}, {GID: 2}, {GID: 3}, {GID: 4}, {GID: 99}, {GID: 5}}
	l := &SeqContext3{
		Input: []coverage.Set{
			{1: true, 3: true, 4: true},
			{2: true, 4: true, 5: true},
			{3: true, 5: true},
		},
	}
	keep := makeDebugKeepFunc()

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
	ctx := &Context{seq: in, keep: keep}
	for _, test := range cases {
		next := l.Apply(ctx, test.before, len(in))
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, next, test.after)
		}
	}
}

func BenchmarkSeqContext3(b *testing.B) {
	l0 := &Gsub1_1{
		Cov:   coverage.Set{1: true, 2: true},
		Delta: 1,
	}
	l1 := &SeqContext3{
		Input: []coverage.Set{
			{1: true},
			{2: true},
		},
		Actions: []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
	}
	ll := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{l0},
		},
		{
			Meta:      &LookupMetaInfo{LookupType: 5},
			Subtables: []Subtable{l1},
		},
	}
	var seq []glyph.Info
	ctx := NewContext(ll, nil, []LookupIndex{1})

	for _, gid := range []glyph.ID{1, 2, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2} {
		seq = append(seq, glyph.Info{GID: gid})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Apply(seq)
	}
}

func FuzzSeqContext3(f *testing.F) {
	sub := &SeqContext3{}
	f.Add(sub.encode())
	sub.Input = append(sub.Input, coverage.Set{3: true, 4: true})
	sub.Actions = []SeqLookup{
		{SequenceIndex: 0, LookupListIndex: 1},
		{SequenceIndex: 1, LookupListIndex: 5},
		{SequenceIndex: 0, LookupListIndex: 4},
	}
	f.Add(sub.encode())
	sub.Input = append(sub.Input, coverage.Set{1: true, 3: true, 5: true})
	f.Add(sub.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 3, readSeqContext3, data)
	})
}

func TestChainedSeqContext1(t *testing.T) {
	in := []glyph.Info{
		{GID: 1}, {GID: 99}, {GID: 2}, {GID: 99}, {GID: 3}, {GID: 4}, {GID: 99}, {GID: 5},
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
	keep := makeDebugKeepFunc()

	cases := []struct {
		before, after int
	}{
		{0, -1},
		{1, -1},
		{2, -1},
		{3, -1},
		{4, 7}, // matches [1, 2,] 3, 4, [5], also skips 99
	}
	ctx := &Context{seq: in, keep: keep}
	for _, test := range cases {
		next := l.Apply(ctx, test.before, len(in))
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, next, test.after)
		}
	}
}

func BenchmarkChainedSeqContext1(b *testing.B) {
	l0 := &Gsub1_1{
		Cov:   coverage.Set{1: true, 2: true},
		Delta: 1,
	}
	l1 := &ChainedSeqContext1{
		Cov: map[glyph.ID]int{1: 0, 2: 1},
		Rules: [][]*ChainedSeqRule{
			{ // seq = 1, ...
				{
					Input:     []glyph.ID{1, 2},
					Lookahead: []glyph.ID{2},
					Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
				{
					Backtrack: []glyph.ID{1},
					Input:     []glyph.ID{2, 2},
					Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
			},
			{ // seq = 2, ...
				{
					Backtrack: []glyph.ID{1},
					Input:     []glyph.ID{2, 1},
					Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
				{
					Input:     []glyph.ID{1, 1},
					Lookahead: []glyph.ID{2},
					Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
			},
		},
	}
	ll := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{l0},
		},
		{
			Meta:      &LookupMetaInfo{LookupType: 5},
			Subtables: []Subtable{l1},
		},
	}
	var seq []glyph.Info
	ctx := NewContext(ll, nil, []LookupIndex{1})

	for _, gid := range []glyph.ID{1, 2, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2} {
		seq = append(seq, glyph.Info{GID: gid})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Apply(seq)
	}
}

func FuzzChainedSeqContext1(f *testing.F) {
	sub := &ChainedSeqContext1{}
	f.Add(sub.encode())
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
	f.Add(sub.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 1, readChainedSeqContext1, data)
	})
}

func BenchmarkChainedSeqContext2(b *testing.B) {
	l0 := &Gsub1_1{
		Cov:   coverage.Set{1: true, 2: true},
		Delta: 1,
	}
	l1 := &ChainedSeqContext2{
		Cov:       map[glyph.ID]int{1: 0, 2: 1},
		Backtrack: classdef.Table{1: 1, 2: 2},
		Input:     classdef.Table{1: 1, 2: 1},
		Lookahead: classdef.Table{1: 1, 2: 1},
		Rules: [][]*ChainedClassSeqRule{
			{ // seq = 1, ...
				{
					Input:     []uint16{1, 2},
					Lookahead: []uint16{2},
					Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
				{
					Backtrack: []uint16{1},
					Input:     []uint16{2, 2},
					Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
			},
			{ // seq = 2, ...
				{
					Backtrack: []uint16{1},
					Input:     []uint16{2, 1},
					Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
				{
					Input:     []uint16{1, 1},
					Lookahead: []uint16{2},
					Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
				},
			},
		},
	}
	ll := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{l0},
		},
		{
			Meta:      &LookupMetaInfo{LookupType: 5},
			Subtables: []Subtable{l1},
		},
	}
	var seq []glyph.Info
	ctx := NewContext(ll, nil, []LookupIndex{1})

	for _, gid := range []glyph.ID{1, 2, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2} {
		seq = append(seq, glyph.Info{GID: gid})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Apply(seq)
	}
}

func FuzzChainedSeqContext2(f *testing.F) {
	sub := &ChainedSeqContext2{}
	f.Add(sub.encode())
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
	f.Add(sub.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 2, readChainedSeqContext2, data)
	})
}

func BenchmarkChainedSeqContext3(b *testing.B) {
	l0 := &Gsub1_1{
		Cov:   coverage.Set{1: true, 2: true},
		Delta: 1,
	}
	l1 := &ChainedSeqContext3{
		Backtrack: []coverage.Set{{1: true}},
		Input:     []coverage.Set{{1: true}, {2: true}},
		Lookahead: []coverage.Set{{2: true}},
		Actions:   []SeqLookup{{SequenceIndex: 1, LookupListIndex: 0}},
	}
	ll := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{l0},
		},
		{
			Meta:      &LookupMetaInfo{LookupType: 5},
			Subtables: []Subtable{l1},
		},
	}
	var seq []glyph.Info
	ctx := NewContext(ll, nil, []LookupIndex{1})

	for _, gid := range []glyph.ID{1, 2, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2, 2, 1, 1, 2} {
		seq = append(seq, glyph.Info{GID: gid})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Apply(seq)
	}
}

func FuzzChainedSeqContext3(f *testing.F) {
	sub := &ChainedSeqContext3{}
	f.Add(sub.encode())
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
	f.Add(sub.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 3, readChainedSeqContext3, data)
	})
}

// makeDebugKeepFunc returns a KeepFunc which keeps glyphs with GID < 50,
// and ignores all glyphs 50, ..., 255.
func makeDebugKeepFunc() *keepFunc {
	class := classdef.Table{}
	for i := glyph.ID(0); i < 256; i++ {
		if i < 50 {
			class[i] = gdef.GlyphClassBase
		} else {
			class[i] = gdef.GlyphClassMark
		}
	}
	gdef := &gdef.Table{GlyphClass: class}
	meta := &LookupMetaInfo{LookupFlags: IgnoreMarks}
	return &keepFunc{Gdef: gdef, Meta: meta}
}

func TestDebugKeepFunc(t *testing.T) {
	k := makeDebugKeepFunc()
	for i := glyph.ID(0); i < 256; i++ {
		if k.Keep(i) != (i < 50) {
			t.Errorf("Keep(%d) = %v, want %v", i, k.Keep(i), i < 50)
		}
	}
}

type debugNestedLookup struct {
	matchPos []int
	actions  []SeqLookup
}

func (l *debugNestedLookup) Apply(ctx *Context, a, b int) int {
	if a != 0 {
		ctx.seq[a].GID = 3
		return a + 1
	}

	next := l.matchPos[len(l.matchPos)-1] + 1
	ctx.stack = append(ctx.stack, &nested{
		InputPos: l.matchPos,
		Actions:  l.actions,
		EndPos:   next,
	})
	return next
}

func (l *debugNestedLookup) encodeLen() int {
	panic("unreachable")
}

func (l *debugNestedLookup) encode() []byte {
	panic("unreachable")
}
