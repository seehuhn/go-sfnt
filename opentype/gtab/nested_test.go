// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
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
		next := l.apply(ctx, test.before, len(in))
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

	for b.Loop() {
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
		next := l.apply(ctx, test.before, len(in))
		if next != test.after {
			t.Errorf("Apply(%d) = %d, want %d", test.before, next, test.after)
		}
	}
}

// TestSeqContext2ClassOutOfRange checks that a covered glyph whose input class
// is beyond the rule-set list does not panic.  readSeqContext2 truncates the
// rule-set list to numClasses but never pads it up, so the ClassDef can return
// a class >= len(Rules).
func TestSeqContext2ClassOutOfRange(t *testing.T) {
	in := []glyph.Info{{GID: 5}}
	l := &SeqContext2{
		Cov:   map[glyph.ID]int{5: 0},
		Input: classdef.Table{5: 2}, // class 2, but only one rule set
		Rules: [][]*ClassSeqRule{
			{{Input: []uint16{}}},
		},
	}
	ctx := &Context{seq: in, keep: makeDebugKeepFunc()}
	if next := l.apply(ctx, 0, len(in)); next != -1 {
		t.Errorf("apply = %d, want -1", next)
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

	for b.Loop() {
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
		next := l.apply(ctx, test.before, len(in))
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

	for b.Loop() {
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

// TestChainedSeqContext3 checks that apply records the matched input
// positions, including for input glyphs past the first.  The returned
// position alone does not exercise this, so the test inspects the InputPos
// list pushed onto the stack.
func TestChainedSeqContext3(t *testing.T) {
	// glyph 99 is a mark, so it is skipped between the input glyphs 3 and 4
	in := []glyph.Info{{GID: 1}, {GID: 2}, {GID: 3}, {GID: 99}, {GID: 4}, {GID: 5}}
	l := &ChainedSeqContext3{
		Backtrack: []coverage.Set{{1: true}},
		Input: []coverage.Set{
			{2: true},
			{3: true},
			{4: true},
		},
		Lookahead: []coverage.Set{{5: true}},
		Actions: []SeqLookup{
			{SequenceIndex: 1, LookupListIndex: 7},
			{SequenceIndex: 2, LookupListIndex: 8},
		},
	}
	keep := makeDebugKeepFunc()
	ctx := &Context{seq: in, keep: keep}

	next := l.apply(ctx, 1, len(in))
	if next != 5 {
		t.Errorf("apply returned %d, want 5", next)
	}
	if len(ctx.stack) != 1 {
		t.Fatalf("expected one nested entry, got %d", len(ctx.stack))
	}
	if diff := cmp.Diff([]int{1, 2, 4}, ctx.stack[0].InputPos); diff != "" {
		t.Errorf("InputPos mismatch (-want +got):\n%s", diff)
	}
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
		next := l.apply(ctx, test.before, len(in))
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

	for b.Loop() {
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

	for b.Loop() {
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

	for b.Loop() {
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
	for i := range glyph.ID(256) {
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
	for i := range glyph.ID(256) {
		if k.Keep(i) != (i < 50) {
			t.Errorf("Keep(%d) = %v, want %v", i, k.Keep(i), i < 50)
		}
	}
}

type debugNestedLookup struct {
	matchPos []int
	actions  []SeqLookup
}

func (l *debugNestedLookup) apply(ctx *Context, a, b int) int {
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

func (l *debugNestedLookup) fits() error {
	return nil
}

func (l *debugNestedLookup) encodeLen() int {
	panic("unreachable")
}

func (l *debugNestedLookup) encode() []byte {
	panic("unreachable")
}

// TestContextSubtableOffsetOverflow checks that the context subtables refuse a
// layout whose offsets would not fit the uint16 the format stores them in,
// instead of writing a truncated offset which points at the wrong data.
func TestContextSubtableOffsetOverflow(t *testing.T) {
	// Rule sets and coverage tables must differ, since identical tables
	// are stored only once.  A rule set of a few glyphs is small enough
	// that thousands are needed to push the offsets out of range.
	rule := func(i int) *SeqRule {
		return &SeqRule{Input: []glyph.ID{glyph.ID(i)}, Actions: []SeqLookup{{SequenceIndex: 0}}}
	}
	chained := func(i int) *ChainedSeqRule {
		return &ChainedSeqRule{
			Input:   []glyph.ID{glyph.ID(i)},
			Actions: []SeqLookup{{SequenceIndex: 0}},
		}
	}
	// a coverage table of 10000 glyphs two apart encodes to about 20 KiB, so
	// a handful of them reach past the limit
	wide := func(k int) coverage.Set {
		set := coverage.Set{}
		for i := range 10000 {
			set[glyph.ID(2*i+1+k)] = true
		}
		return set
	}
	wideSet := wide(0)

	cases := []struct {
		name string
		make func() Subtable
	}{
		{"SeqContext1", func() Subtable {
			const n = 5000
			cov := coverage.Table{}
			rules := make([][]*SeqRule, n)
			for i := range rules {
				cov[glyph.ID(i+1)] = i
				rules[i] = []*SeqRule{rule(i)}
			}
			return &SeqContext1{Cov: cov, Rules: rules}
		}},
		{"SeqContext3", func() Subtable {
			return &SeqContext3{
				Input:   []coverage.Set{wide(0), wide(1), wide(2), wide(3), wide(4)},
				Actions: []SeqLookup{{SequenceIndex: 0}},
			}
		}},
		{"ChainedSeqContext1", func() Subtable {
			const n = 5000
			cov := coverage.Table{}
			rules := make([][]*ChainedSeqRule, n)
			for i := range rules {
				cov[glyph.ID(i+1)] = i
				rules[i] = []*ChainedSeqRule{chained(i)}
			}
			return &ChainedSeqContext1{Cov: cov, Rules: rules}
		}},
		// A subtable whose rule sets are all absent still stores the offsets
		// in its header, and those are the ones which run away here.  The
		// checks alongside the rule sets never run for these.
		{"ChainedSeqContext1 without rules", func() Subtable {
			const n = 40000 // 6 + 2*n puts the coverage table out of reach
			return &ChainedSeqContext1{
				Cov:   coverage.Table{1: 0},
				Rules: make([][]*ChainedSeqRule, n),
			}
		}},
		{"ChainedSeqContext2 without rules", func() Subtable {
			cd := classdef.Table{}
			for i := range 20000 {
				cd[glyph.ID(2*i+1)] = uint16(i%300 + 1)
			}
			shifted := func(k uint16) classdef.Table {
				res := classdef.Table{}
				for g, c := range cd {
					res[g] = c + k
				}
				return res
			}
			return &ChainedSeqContext2{
				Cov:       wideSet.ToTable(),
				Backtrack: shifted(1),
				Input:     shifted(2),
				Lookahead: shifted(3),
				Rules:     make([][]*ChainedClassSeqRule, 4),
			}
		}},

		{"ChainedSeqContext3", func() Subtable {
			return &ChainedSeqContext3{
				Backtrack: []coverage.Set{wide(0), wide(1)},
				Input:     []coverage.Set{wide(2), wide(3)},
				Lookahead: []coverage.Set{wide(4), wide(5)},
				Actions:   []SeqLookup{{SequenceIndex: 0}},
			}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := c.make()
			assertPanics(t, func() { l.encodeLen() })
			assertPanics(t, func() { l.encode() })
		})
	}
}

func appendWords(buf []byte, xs ...int) []byte {
	for _, x := range xs {
		buf = append(buf, byte(x>>8), byte(x))
	}
	return buf
}

// sharedCoverage returns a format 1 coverage table for 1000 glyphs two
// apart.
func sharedCoverage() []byte {
	const n = 1000
	data := appendWords(nil, 1, n)
	for i := range n {
		data = appendWords(data, 2*i+1)
	}
	return data
}

// TestSeqContext1SharedRuleSets checks that a subtable whose rule set
// offsets all reach the same rule set can be written back.  Without
// sharing, the output would need more than 64 KiB.
func TestSeqContext1SharedRuleSets(t *testing.T) {
	const n = 400 // covered glyphs, all sharing one rule set
	const m = 100 // rules in the shared rule set
	setPos := 6 + 2*n
	const ruleLen = 4 + 2*2 + 4 // two further input glyphs, one action
	covPos := setPos + 2 + 2*m + m*ruleLen

	data := appendWords(nil, 1, covPos, n)
	for range n {
		data = appendWords(data, setPos)
	}
	data = appendWords(data, m)
	for j := range m {
		data = appendWords(data, 2+2*m+j*ruleLen)
	}
	for j := range m {
		data = appendWords(data, 3, 1, j+2, j+3, 0, 1)
	}
	data = appendWords(data, 2, 1, 1, n, 0) // glyphs 1..n

	sub := checkSharedRoundTrip(t, readSeqContext1, data)
	l := sub.(*SeqContext1)
	if len(l.Rules) != n {
		t.Errorf("got %d rule sets, want %d", len(l.Rules), n)
	}
	if n*(2+2*m+m*ruleLen) <= 0xFFFF {
		t.Fatal("test data too small to need sharing")
	}
}

// TestSeqContext3SharedCoverage checks that a subtable whose coverage
// offsets all reach the same table can be written back.
func TestSeqContext3SharedCoverage(t *testing.T) {
	const g = 200
	covPos := 6 + 2*g + 4
	cov := sharedCoverage()
	if g*len(cov) <= 0xFFFF {
		t.Fatal("test data too small to need sharing")
	}

	data := appendWords(nil, 3, g, 1)
	for range g {
		data = appendWords(data, covPos)
	}
	data = appendWords(data, 0, 1) // action
	data = append(data, cov...)

	sub := checkSharedRoundTrip(t, readSeqContext3, data)
	if got := len(sub.(*SeqContext3).Input); got != g {
		t.Errorf("got %d coverage tables, want %d", got, g)
	}
}

// TestChainedSeqContext3SharedCoverage checks that a subtable whose
// coverage offsets all reach the same table can be written back.
func TestChainedSeqContext3SharedCoverage(t *testing.T) {
	const g = 100 // coverage tables in each of the three lists
	covPos := 10 + 3*2*g + 4
	cov := sharedCoverage()
	if 3*g*len(cov) <= 0xFFFF {
		t.Fatal("test data too small to need sharing")
	}

	data := []byte{0, 3}
	for range 3 {
		data = appendWords(data, g)
		for range g {
			data = appendWords(data, covPos)
		}
	}
	data = appendWords(data, 1, 0, 1) // action
	data = append(data, cov...)

	sub := checkSharedRoundTrip(t, readChainedSeqContext3, data)
	l := sub.(*ChainedSeqContext3)
	if len(l.Backtrack) != g || len(l.Input) != g || len(l.Lookahead) != g {
		t.Errorf("wrong coverage counts %d, %d, %d", len(l.Backtrack), len(l.Input), len(l.Lookahead))
	}
}

// checkEncoded checks the size of the encoding of a subtable and that
// reading it back gives the same subtable.
func checkEncoded(t *testing.T, l Subtable, read func(*parser.Parser, int64) (Subtable, error), wantLen int) {
	t.Helper()
	enc := l.encode()
	if len(enc) != wantLen || l.encodeLen() != wantLen {
		t.Errorf("got %d bytes (encodeLen %d), want %d", len(enc), l.encodeLen(), wantLen)
	}
	sub := readSubtable(t, read, enc)
	if diff := cmp.Diff(l, sub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestContextRuleSharing checks that only tables with identical content
// share storage in the subtables with rule sets.
func TestContextRuleSharing(t *testing.T) {
	action := []SeqLookup{{SequenceIndex: 0, LookupListIndex: 1}}
	cov := coverage.Table{1: 0, 2: 1, 3: 2}
	covLen := cov.EncodeLen()
	cd := classdef.Table{1: 1, 2: 2, 3: 2}
	cdLen := cd.AppendLen()

	t.Run("SeqContext1", func(t *testing.T) {
		a := func() *SeqRule { return &SeqRule{Input: []glyph.ID{5}, Actions: action} }
		b := &SeqRule{Input: []glyph.ID{6}, Actions: action}
		l := &SeqContext1{
			Cov:   cov,
			Rules: [][]*SeqRule{{a()}, {a()}, {a(), a()}},
		}
		// header 12, rule sets (2+2+10) and (2+4+10), coverage
		checkEncoded(t, l, readSeqContext1, 12+14+16+covLen)

		l.Rules[1] = []*SeqRule{b}
		checkEncoded(t, l, readSeqContext1, 12+14+14+16+covLen)
	})

	t.Run("SeqContext2", func(t *testing.T) {
		a := func() *ClassSeqRule { return &ClassSeqRule{Input: []uint16{1}, Actions: action} }
		b := &ClassSeqRule{Input: []uint16{2}, Actions: action}
		l := &SeqContext2{
			Cov:   cov,
			Input: cd,
			Rules: [][]*ClassSeqRule{{a()}, {a()}, {a(), a()}},
		}
		// header 8+6, rule sets (2+2+10) and (2+4+10), coverage, classes
		checkEncoded(t, l, readSeqContext2, 14+14+16+covLen+cdLen)

		l.Rules[0] = []*ClassSeqRule{b}
		checkEncoded(t, l, readSeqContext2, 14+14+14+16+covLen+cdLen)
	})

	t.Run("ChainedSeqContext1", func(t *testing.T) {
		a := func() *ChainedSeqRule {
			return &ChainedSeqRule{
				Backtrack: []glyph.ID{1},
				Input:     []glyph.ID{2},
				Lookahead: []glyph.ID{3},
				Actions:   action,
			}
		}
		b := &ChainedSeqRule{
			Backtrack: []glyph.ID{4},
			Input:     []glyph.ID{2},
			Lookahead: []glyph.ID{3},
			Actions:   action,
		}
		l := &ChainedSeqContext1{
			Cov:   cov,
			Rules: [][]*ChainedSeqRule{{a()}, {a()}, {a(), a()}},
		}
		// header 12, coverage, rule sets (2+2+18) and (2+4+18)
		checkEncoded(t, l, readChainedSeqContext1, 12+covLen+22+24)

		l.Rules[1] = []*ChainedSeqRule{b}
		checkEncoded(t, l, readChainedSeqContext1, 12+covLen+22+22+24)
	})

	t.Run("ChainedSeqContext2", func(t *testing.T) {
		a := func() *ChainedClassSeqRule {
			return &ChainedClassSeqRule{
				Backtrack: []uint16{1},
				Input:     []uint16{2},
				Lookahead: []uint16{1},
				Actions:   action,
			}
		}
		b := &ChainedClassSeqRule{
			Backtrack: []uint16{2},
			Input:     []uint16{2},
			Lookahead: []uint16{1},
			Actions:   action,
		}
		l := &ChainedSeqContext2{
			Cov:       cov,
			Backtrack: cd,
			Input:     cd,
			Lookahead: cd,
			Rules:     [][]*ChainedClassSeqRule{{a()}, {a()}, {a(), a()}},
		}
		// header 12+6, coverage, one class definition table shared by
		// all three, rule sets (2+2+18) and (2+4+18)
		checkEncoded(t, l, readChainedSeqContext2, 18+covLen+cdLen+22+24)

		l.Lookahead = classdef.Table{1: 2, 2: 2, 3: 2}
		l.Rules[1] = []*ChainedClassSeqRule{b}
		want := 18 + covLen + cdLen + l.Lookahead.AppendLen() + 22 + 22 + 24
		checkEncoded(t, l, readChainedSeqContext2, want)
	})
}

// sharedRuleSet returns a rule set of m rules, as the offset table followed
// by the rules.  The rules are produced by rule(j).
func sharedRuleSet(m int, rule func(j int) []byte) []byte {
	var rules []byte
	data := appendWords(nil, m)
	for j := range m {
		r := rule(j)
		data = appendWords(data, 2+2*m+len(rules))
		rules = append(rules, r...)
	}
	return append(data, rules...)
}

// glyphListCoverage returns a format 1 coverage table for glyphs 1 to n.
func glyphListCoverage(n int) []byte {
	data := appendWords(nil, 1, n)
	for i := range n {
		data = appendWords(data, i+1)
	}
	return data
}

// identityClassDef returns a format 2 class definition which maps glyph i
// to class i, for i = 1 to n.
func identityClassDef(n int) []byte {
	data := appendWords(nil, 2, n)
	for i := 1; i <= n; i++ {
		data = appendWords(data, i, i, i)
	}
	return data
}

// TestSharedRuleSets checks that subtables whose rule set offsets all reach
// the same rule set can be read with the default budget and written back.
// Without sharing, the Go structures would exceed the budget and the output
// would need more than 64 KiB.
func TestSharedRuleSets(t *testing.T) {
	const n = 300 // rule sets, all the same
	const m = 100 // rules in the rule set

	// rule: 3 input glyphs, 1 action
	classRule := func(j int) []byte {
		return appendWords(nil, 3, 1, j, j+1, 0, 1)
	}
	const classRuleLen = 12
	// rule: 2 backtrack, 3 input, 1 lookahead glyphs, 1 action
	chainedRule := func(j int) []byte {
		return appendWords(nil, 2, 1, 2, 3, j, j+1, 1, 4, 1, 0, 1)
	}
	const chainedRuleLen = 22
	if n*(2+2*m+m*classRuleLen) <= 0xFFFF || n*(2+2*m+m*chainedRuleLen) <= 0xFFFF {
		t.Fatal("test data too small to need sharing")
	}

	t.Run("SeqContext2", func(t *testing.T) {
		set := sharedRuleSet(m, classRule)
		cov := glyphListCoverage(n)
		cd := identityClassDef(n)
		setPos := 8 + 2*n
		covPos := setPos + len(set)
		cdPos := covPos + len(cov)

		data := appendWords(nil, 2, covPos, cdPos, n)
		for range n {
			data = appendWords(data, setPos)
		}
		data = append(data, set...)
		data = append(data, cov...)
		data = append(data, cd...)

		sub := checkSharedRoundTrip(t, readSeqContext2, data)
		l := sub.(*SeqContext2)
		if len(l.Rules) != n || len(l.Rules[0]) != m {
			t.Errorf("wrong shape %d x %d", len(l.Rules), len(l.Rules[0]))
		}
	})

	t.Run("ChainedSeqContext1", func(t *testing.T) {
		set := sharedRuleSet(m, chainedRule)
		cov := glyphListCoverage(n)
		setPos := 6 + 2*n
		covPos := setPos + len(set)

		data := appendWords(nil, 1, covPos, n)
		for range n {
			data = appendWords(data, setPos)
		}
		data = append(data, set...)
		data = append(data, cov...)

		sub := checkSharedRoundTrip(t, readChainedSeqContext1, data)
		l := sub.(*ChainedSeqContext1)
		if len(l.Rules) != n || len(l.Rules[0]) != m {
			t.Errorf("wrong shape %d x %d", len(l.Rules), len(l.Rules[0]))
		}
	})

	t.Run("ChainedSeqContext2", func(t *testing.T) {
		set := sharedRuleSet(m, chainedRule)
		cov := glyphListCoverage(n)
		cd := identityClassDef(n)
		setPos := 12 + 2*n
		covPos := setPos + len(set)
		cdPos := covPos + len(cov)

		// one class definition serves as backtrack, input and lookahead
		data := appendWords(nil, 2, covPos, cdPos, cdPos, cdPos, n)
		for range n {
			data = appendWords(data, setPos)
		}
		data = append(data, set...)
		data = append(data, cov...)
		data = append(data, cd...)

		sub := checkSharedRoundTrip(t, readChainedSeqContext2, data)
		l := sub.(*ChainedSeqContext2)
		if len(l.Rules) != n || len(l.Rules[0]) != m {
			t.Errorf("wrong shape %d x %d", len(l.Rules), len(l.Rules[0]))
		}
	})
}

// TestSharedRules checks that rule sets which differ but whose offsets
// reach the same rules can be read and written back.
func TestSharedRules(t *testing.T) {
	// set A holds rules r1 and r2, set B holds r2 only
	r1 := appendWords(nil, 2, 1, 7, 0, 1)
	r2 := appendWords(nil, 2, 1, 8, 0, 1)
	setA := 10 // header: format, coverage, count, two offsets
	setB := setA + 6
	r1Pos := setB + 4
	r2Pos := r1Pos + len(r1)
	covPos := r2Pos + len(r2)
	data := appendWords(nil, 1, covPos, 2, setA, setB)
	data = appendWords(data, 2, r1Pos-setA, r2Pos-setA)
	data = appendWords(data, 1, r2Pos-setB)
	data = append(data, r1...)
	data = append(data, r2...)
	data = append(data, glyphListCoverage(2)...)

	sub := readSubtable(t, readSeqContext1, data)
	sub2 := readSubtable(t, readSeqContext1, sub.encode())
	if diff := cmp.Diff(sub, sub2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	l := sub.(*SeqContext1)
	if len(l.Rules) != 2 || len(l.Rules[0]) != 2 || len(l.Rules[1]) != 1 || l.Rules[0][1] != l.Rules[1][0] {
		t.Errorf("rules not shared: %v", l.Rules)
	}
}

// TestContextCoverageSharing checks that only identical coverage tables
// share storage in the coverage based subtables.
func TestContextCoverageSharing(t *testing.T) {
	action := []SeqLookup{{SequenceIndex: 0, LookupListIndex: 1}}
	s := coverage.Set{1: true, 2: true}
	u := coverage.Set{1: true, 3: true}
	covLen := s.ToTable().EncodeLen()

	l3 := &SeqContext3{
		Input:   []coverage.Set{s, u, {1: true, 2: true}},
		Actions: action,
	}
	// header 6+6+4, two coverage tables
	checkEncoded(t, l3, readSeqContext3, 16+2*covLen)

	c3 := &ChainedSeqContext3{
		Backtrack: []coverage.Set{s},
		Input:     []coverage.Set{u, {1: true, 2: true}},
		Lookahead: []coverage.Set{u},
		Actions:   action,
	}
	// header 10+8+4, two coverage tables
	checkEncoded(t, c3, readChainedSeqContext3, 22+2*covLen)
}

// TestContextLayoutErrors checks that layout reports an offset which does
// not fit, at each stage where one can occur.
func TestContextLayoutErrors(t *testing.T) {
	action := []SeqLookup{{SequenceIndex: 0, LookupListIndex: 1}}
	hugeSet := coverage.Set{}
	for i := range 35000 {
		hugeSet[glyph.ID(2*i+1)] = true
	}
	huge := hugeSet.ToTable() // over 64 KiB
	hugeSet2 := coverage.Set{}
	hugeClasses := classdef.Table{}
	for i := range 35000 {
		hugeSet2[glyph.ID(2*i)] = true
		hugeClasses[glyph.ID(2*i)] = 1
	}
	const n = 6000
	distinct := func(i int) []glyph.ID { return []glyph.ID{glyph.ID(i)} }

	t.Run("SeqContext1 rule set", func(t *testing.T) {
		rules := make([]*SeqRule, n)
		for i := range rules {
			rules[i] = &SeqRule{Input: distinct(i), Actions: action}
		}
		l := &SeqContext1{Cov: coverage.Table{1: 0}, Rules: [][]*SeqRule{rules}}
		if _, err := l.layout(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("SeqContext2 rule set", func(t *testing.T) {
		rules := make([]*ClassSeqRule, n)
		for i := range rules {
			rules[i] = &ClassSeqRule{Input: []uint16{uint16(i)}, Actions: action}
		}
		l := &SeqContext2{Rules: [][]*ClassSeqRule{rules}}
		if _, err := l.layout(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("SeqContext2 two large tables", func(t *testing.T) {
		l := &SeqContext2{Cov: huge, Input: hugeClasses}
		if _, err := l.layout(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("ChainedSeqContext1 rule set", func(t *testing.T) {
		rules := make([]*ChainedSeqRule, n)
		for i := range rules {
			rules[i] = &ChainedSeqRule{Input: distinct(i), Actions: action}
		}
		l := &ChainedSeqContext1{Cov: coverage.Table{1: 0}, Rules: [][]*ChainedSeqRule{rules}}
		if _, err := l.layout(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("ChainedSeqContext2 rule set", func(t *testing.T) {
		rules := make([]*ChainedClassSeqRule, n)
		for i := range rules {
			rules[i] = &ChainedClassSeqRule{Input: []uint16{uint16(i)}, Actions: action}
		}
		l := &ChainedSeqContext2{Rules: [][]*ChainedClassSeqRule{rules}}
		if _, err := l.layout(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("ChainedSeqContext2 two large tables", func(t *testing.T) {
		l := &ChainedSeqContext2{
			Cov:       huge,
			Backtrack: hugeClasses,
			Rules:     [][]*ChainedClassSeqRule{{{Actions: action}}},
		}
		if _, err := l.layout(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("SeqContext3", func(t *testing.T) {
		l := &SeqContext3{Input: []coverage.Set{hugeSet, hugeSet}}
		if _, err := l.layout(); err != nil {
			t.Fatal("unexpected error for shared tables")
		}
		l = &SeqContext3{Input: []coverage.Set{hugeSet, hugeSet2, {1: true}}}
		if _, err := l.layout(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("ChainedSeqContext3", func(t *testing.T) {
		one := coverage.Set{1: true}
		two := coverage.Set{2: true}
		cases := []*ChainedSeqContext3{
			{Backtrack: []coverage.Set{hugeSet, hugeSet2}, Input: []coverage.Set{one}},
			{Backtrack: []coverage.Set{hugeSet}, Input: []coverage.Set{hugeSet}},
			{Backtrack: []coverage.Set{one}, Input: []coverage.Set{hugeSet, hugeSet2}},
			{Backtrack: []coverage.Set{hugeSet}, Input: []coverage.Set{hugeSet2}, Lookahead: []coverage.Set{two}},
		}
		for i, l := range cases {
			_, err := l.layout()
			if (err != nil) != (i != 1) {
				t.Errorf("case %d: unexpected result %v", i, err)
			}
		}
	})
}

// TestLayoutCoverageOrder checks subtables which can only be encoded if the
// coverage table comes before the per-glyph tables, or only if it comes
// after them.
func TestLayoutCoverageOrder(t *testing.T) {
	action := []SeqLookup{{SequenceIndex: 0, LookupListIndex: 1}}
	long := make([]glyph.ID, 40000)

	// 40 kB of header and 40 kB of coverage, with one shared rule set
	const n = 20000
	wideSet := coverage.Set{}
	for i := range n {
		wideSet[glyph.ID(2*i+1)] = true
	}
	shared := []*ChainedSeqRule{{
		Backtrack: []glyph.ID{},
		Input:     []glyph.ID{},
		Lookahead: []glyph.ID{},
		Actions:   action,
	}}
	wideRules := make([][]*ChainedSeqRule, n)
	for i := range wideRules {
		wideRules[i] = shared
	}

	cases := []struct {
		name string
		sub  Subtable
		read func(*parser.Parser, int64) (Subtable, error)
	}{
		{"Gsub2_1", &Gsub2_1{Cov: coverage.Table{1: 0}, Repl: [][]glyph.ID{long}}, readGsub2_1},
		{"Gsub3_1", &Gsub3_1{Cov: coverage.Table{1: 0}, Alternates: [][]glyph.ID{long}}, readGsub3_1},
		{"Gsub4_1", &Gsub4_1{Cov: coverage.Table{1: 0}, Repl: [][]Ligature{{{In: long, Out: 1}}}}, readGsub4_1},
		{"SeqContext1", &SeqContext1{
			Cov:   coverage.Table{1: 0},
			Rules: [][]*SeqRule{{{Input: long, Actions: action}}},
		}, readSeqContext1},
		{"SeqContext2", &SeqContext2{
			Cov:   coverage.Table{1: 0},
			Input: classdef.Table{},
			Rules: [][]*ClassSeqRule{{{Input: make([]uint16, 40000), Actions: action}}},
		}, readSeqContext2},
		{"ChainedSeqContext1", &ChainedSeqContext1{
			Cov:   wideSet.ToTable(),
			Rules: wideRules,
		}, readChainedSeqContext1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkFitsRoundTrip(t, c.sub, c.read)
		})
	}
}

// checkFitsRoundTrip checks that sub can be encoded and that reading the
// encoding gives sub back.
func checkFitsRoundTrip(t *testing.T, sub Subtable, read func(*parser.Parser, int64) (Subtable, error)) {
	t.Helper()
	if err := sub.fits(); err != nil {
		t.Fatal(err)
	}
	data := sub.encode()
	if len(data) != sub.encodeLen() {
		t.Errorf("encodeLen() = %d, want %d", sub.encodeLen(), len(data))
	}
	got := readSubtable(t, read, data)
	if diff := cmp.Diff(sub, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestLayoutLargestLast checks subtables which can only be encoded if a
// table of more than 64 KiB is stored after the tables which follow it in
// header order.
func TestLayoutLargestLast(t *testing.T) {
	action := []SeqLookup{{SequenceIndex: 0, LookupListIndex: 1}}
	hugeSet := coverage.Set{}
	for i := range 35000 {
		hugeSet[glyph.ID(2*i+1)] = true
	}
	huge := hugeSet.ToTable() // over 64 KiB
	one := coverage.Set{1: true}
	two := coverage.Set{2: true}

	long := make([]glyph.ID, 40000)
	longRule := &SeqRule{Input: long, Actions: action}

	// one pair set of about 80 kB, followed by a small one
	manyPairs := map[glyph.Pair]*PairAdjust{
		{Left: 2, Right: 1}: {First: &GposValueRecord{XAdvance: 1}},
	}
	for i := range 20000 {
		manyPairs[glyph.Pair{Left: 1, Right: glyph.ID(i)}] = &PairAdjust{
			First: &GposValueRecord{XAdvance: 2},
		}
	}

	// a mark array of about 64 kB
	manyMarksCov := coverage.Table{}
	manyMarks := make([]markarray.Record, 16000)
	for i := range manyMarks {
		manyMarksCov[glyph.ID(i+10)] = i
		manyMarks[i].Table.X = 3
	}

	// a LigatureAttach table of 64 KiB
	manyComponents := make([][]*anchor.Table, 32767)
	for i := range manyComponents {
		manyComponents[i] = []*anchor.Table{nil}
	}
	shortRule := &SeqRule{Input: []glyph.ID{}, Actions: action}

	cases := []struct {
		name string
		sub  Subtable
		read func(*parser.Parser, int64) (Subtable, error)
	}{
		{"SeqContext1 rule", &SeqContext1{
			Cov:   coverage.Table{1: 0},
			Rules: [][]*SeqRule{{longRule, shortRule}},
		}, readSeqContext1},
		{"SeqContext1 rule set", &SeqContext1{
			Cov:   coverage.Table{1: 0, 2: 1},
			Rules: [][]*SeqRule{{longRule}, {shortRule}},
		}, readSeqContext1},
		{"SeqContext2", &SeqContext2{
			Cov:   huge,
			Input: classdef.Table{1: 1},
			Rules: [][]*ClassSeqRule{{{Input: []uint16{}, Actions: action}}},
		}, readSeqContext2},
		{"SeqContext3", &SeqContext3{
			Input:   []coverage.Set{hugeSet, one},
			Actions: action,
		}, readSeqContext3},
		{"ChainedSeqContext2", &ChainedSeqContext2{
			Cov:       huge,
			Backtrack: classdef.Table{1: 1},
			Input:     classdef.Table{},
			Lookahead: classdef.Table{2: 1},
			Rules: [][]*ChainedClassSeqRule{{{
				Backtrack: []uint16{},
				Input:     []uint16{},
				Lookahead: []uint16{},
				Actions:   action,
			}}},
		}, readChainedSeqContext2},
		{"ChainedSeqContext3 backtrack", &ChainedSeqContext3{
			Backtrack: []coverage.Set{hugeSet, one},
			Input:     []coverage.Set{one},
			Lookahead: []coverage.Set{},
			Actions:   action,
		}, readChainedSeqContext3},
		{"ChainedSeqContext3 input", &ChainedSeqContext3{
			Backtrack: []coverage.Set{one},
			Input:     []coverage.Set{hugeSet, two},
			Lookahead: []coverage.Set{},
			Actions:   action,
		}, readChainedSeqContext3},
		{"ChainedSeqContext3 lookahead", &ChainedSeqContext3{
			Backtrack: []coverage.Set{hugeSet},
			Input:     []coverage.Set{hugeSet},
			Lookahead: []coverage.Set{two},
			Actions:   action,
		}, readChainedSeqContext3},
		{"Gsub8_1", &Gsub8_1{
			Input:              coverage.Table{1: 0},
			Backtrack:          []coverage.Table{huge, {2: 0}},
			Lookahead:          []coverage.Table{},
			SubstituteGlyphIDs: []glyph.ID{3},
		}, readGsub8_1},
		{"Gsub2_1", &Gsub2_1{
			Cov:  coverage.Table{1: 0, 2: 1},
			Repl: [][]glyph.ID{long, {1}},
		}, readGsub2_1},
		{"Gsub3_1", &Gsub3_1{
			Cov:        coverage.Table{1: 0, 2: 1},
			Alternates: [][]glyph.ID{long, {1}},
		}, readGsub3_1},
		{"Gsub4_1 ligature set", &Gsub4_1{
			Cov:  coverage.Table{1: 0, 2: 1},
			Repl: [][]Ligature{{{In: long, Out: 1}}, {{In: []glyph.ID{5}, Out: 2}}},
		}, readGsub4_1},
		{"Gsub4_1 ligature", &Gsub4_1{
			Cov:  coverage.Table{1: 0},
			Repl: [][]Ligature{{{In: long, Out: 1}, {In: []glyph.ID{5}, Out: 2}}},
		}, readGsub4_1},
		{"Gpos2_1", &Gpos2_1{Adjust: manyPairs}, readGpos2_1},
		{"Gpos4_1", &Gpos4_1{
			MarkCov:   manyMarksCov,
			BaseCov:   coverage.Table{1: 0},
			MarkArray: manyMarks,
			BaseArray: [][]*anchor.Table{{{X: 1}}},
		}, readGpos4_1},
		{"Gpos5_1", &Gpos5_1{
			MarkCov:   coverage.Table{3: 0},
			LigCov:    coverage.Table{1: 0, 2: 1},
			MarkArray: []markarray.Record{{X: 1}},
			LigArray:  [][][]*anchor.Table{manyComponents, {{{X: 2}}}},
		}, readGpos5_1},
		{"Gpos2_2", &Gpos2_2{
			Cov:    hugeSet,
			Class1: classdef.Table{1: 1},
			Class2: classdef.Table{2: 1},
			Adjust: [][]*PairAdjust{
				{{First: &GposValueRecord{}}, {First: &GposValueRecord{}}},
				{{First: &GposValueRecord{}}, {First: &GposValueRecord{XAdvance: 10}}},
			},
		}, readGpos2_2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkFitsRoundTrip(t, c.sub, c.read)
		})
	}
}
