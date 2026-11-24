// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/text/language"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

// TestLimitGsubGlyphs checks that substitutions producing glyphs
// outside the font are removed, while valid substitutions are kept.
func TestLimitGsubGlyphs(t *testing.T) {
	const n = 10 // glyphs 0, ..., 9 exist
	subtables := []Subtable{
		&Gsub1_1{Cov: coverage.Set{1: true, 2: true, 8: true}, Delta: 2},
		&Gsub1_2{
			Cov:                coverage.Table{1: 0, 2: 1, 3: 2},
			SubstituteGlyphIDs: []glyph.ID{4, n, 5},
		},
		&Gsub2_1{
			Cov:  coverage.Table{1: 0, 2: 1},
			Repl: [][]glyph.ID{{3, n + 1}, {4, 5}},
		},
		&Gsub3_1{
			Cov:        coverage.Table{1: 0, 2: 1, 3: 2},
			Alternates: [][]glyph.ID{{n, 4, n + 1, 5}, {n}, {6}},
		},
		&Gsub4_1{
			Cov: coverage.Table{1: 0, 2: 1},
			Repl: [][]Ligature{
				{{In: []glyph.ID{2}, Out: n}, {In: []glyph.ID{3}, Out: 6}},
				{{In: []glyph.ID{n}, Out: n + 2}},
			},
		},
		&Gsub8_1{
			Input:              coverage.Table{1: 0, 2: 1},
			Backtrack:          []coverage.Table{{3: 0}},
			SubstituteGlyphIDs: []glyph.ID{n, 4},
		},
	}
	// subtables which are left empty are removed, others are kept
	valid := &Gsub1_2{Cov: coverage.Table{1: 0}, SubstituteGlyphIDs: []glyph.ID{2}}
	context := &SeqContext1{Cov: coverage.Table{}}
	mixed := []Subtable{
		&Gsub1_1{Cov: coverage.Set{9: true}, Delta: 1},
		valid,
		&Gsub1_2{Cov: coverage.Table{1: 0}, SubstituteGlyphIDs: []glyph.ID{n}},
		&Gsub2_1{Cov: coverage.Table{1: 0}, Repl: [][]glyph.ID{{n}}},
		&Gsub3_1{Cov: coverage.Table{1: 0}, Alternates: [][]glyph.ID{{n}}},
		&Gsub4_1{Cov: coverage.Table{1: 0}, Repl: [][]Ligature{{{Out: n}}}},
		&Gsub8_1{Input: coverage.Table{1: 0}, SubstituteGlyphIDs: []glyph.ID{n}},
		context,
	}

	info := &Info{LookupList: LookupList{
		{Meta: &LookupMetaInfo{}, Subtables: subtables},
		{Meta: &LookupMetaInfo{}, Subtables: mixed},
	}}

	info = info.LimitGlyphs(n)

	if got := info.LookupList[1].Subtables; len(got) != 2 || got[0] != valid || got[1] != context {
		t.Errorf("wrong subtables kept: %v", got)
	}

	want := []Subtable{
		&Gsub1_1{Cov: coverage.Set{1: true, 2: true}, Delta: 2},
		&Gsub1_2{
			Cov:                coverage.Table{1: 0, 3: 1},
			SubstituteGlyphIDs: []glyph.ID{4, 5},
		},
		&Gsub2_1{
			Cov:  coverage.Table{2: 0},
			Repl: [][]glyph.ID{{4, 5}},
		},
		&Gsub3_1{
			Cov:        coverage.Table{1: 0, 3: 1},
			Alternates: [][]glyph.ID{{4, 5}, {6}},
		},
		&Gsub4_1{
			Cov:  coverage.Table{1: 0},
			Repl: [][]Ligature{{{In: []glyph.ID{3}, Out: 6}}},
		},
		&Gsub8_1{
			Input:              coverage.Table{2: 0},
			Backtrack:          []coverage.Table{{3: 0}},
			SubstituteGlyphIDs: []glyph.ID{4},
		},
	}
	if diff := cmp.Diff(want, []Subtable(info.LookupList[0].Subtables)); diff != "" {
		t.Errorf("wrong subtables (-want +got):\n%s", diff)
	}
}

// TestLimitGsubGlyphsUnchanged checks that a table without invalid glyphs
// is returned as it is.
func TestLimitGsubGlyphsUnchanged(t *testing.T) {
	sub := &Gsub3_1{
		Cov:        coverage.Table{1: 0, 2: 1},
		Alternates: [][]glyph.ID{{3, 4}, {5}},
	}
	info := &Info{LookupList: LookupList{{
		Meta:      &LookupMetaInfo{},
		Subtables: []Subtable{sub},
	}}}

	if got := info.LimitGlyphs(6); got != info {
		t.Error("table without invalid glyphs was replaced")
	}
}

// limitTestCases lists subtables which refer to glyphs outside a font with
// ten glyphs, together with the subtables expected from LimitGlyphs.
var limitTestCases = []struct {
	name       string
	tp         Type
	lookupType uint16
	in, want   Subtable
}{
	{
		name: "Gsub1_2 covered glyph", tp: TypeGsub, lookupType: 1,
		in: &Gsub1_2{
			Cov:                coverage.Table{1: 0, 12: 1},
			SubstituteGlyphIDs: []glyph.ID{2, 3},
		},
		want: &Gsub1_2{
			Cov:                coverage.Table{1: 0},
			SubstituteGlyphIDs: []glyph.ID{2},
		},
	},
	{
		name: "Gsub4_1 component", tp: TypeGsub, lookupType: 4,
		in: &Gsub4_1{
			Cov: coverage.Table{1: 0},
			Repl: [][]Ligature{{
				{In: []glyph.ID{12}, Out: 5},
				{In: []glyph.ID{3}, Out: 6},
			}},
		},
		want: &Gsub4_1{
			Cov:  coverage.Table{1: 0},
			Repl: [][]Ligature{{{In: []glyph.ID{3}, Out: 6}}},
		},
	},
	{
		name: "Gsub8_1 context", tp: TypeGsub, lookupType: 8,
		in: &Gsub8_1{
			Input:              coverage.Table{1: 0},
			Backtrack:          []coverage.Table{{2: 0, 12: 1}},
			Lookahead:          []coverage.Table{{3: 0}, {4: 0, 13: 1}},
			SubstituteGlyphIDs: []glyph.ID{5},
		},
		want: &Gsub8_1{
			Input:              coverage.Table{1: 0},
			Backtrack:          []coverage.Table{{2: 0}},
			Lookahead:          []coverage.Table{{3: 0}, {4: 0}},
			SubstituteGlyphIDs: []glyph.ID{5},
		},
	},
	{
		name: "Gpos1_1", tp: TypeGpos, lookupType: 1,
		in: &Gpos1_1{
			Cov:    coverage.Table{1: 0, 12: 1},
			Adjust: &GposValueRecord{XAdvance: 10},
		},
		want: &Gpos1_1{
			Cov:    coverage.Table{1: 0},
			Adjust: &GposValueRecord{XAdvance: 10},
		},
	},
	{
		name: "Gpos1_2", tp: TypeGpos, lookupType: 1,
		in: &Gpos1_2{
			Cov:    coverage.Table{1: 0, 5: 1, 12: 2},
			Adjust: []*GposValueRecord{{XAdvance: 10}, {XAdvance: 20}, {XAdvance: 30}},
		},
		want: &Gpos1_2{
			Cov:    coverage.Table{1: 0, 5: 1},
			Adjust: []*GposValueRecord{{XAdvance: 10}, {XAdvance: 20}},
		},
	},
	{
		name: "Gpos2_1", tp: TypeGpos, lookupType: 2,
		in: &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{
			{Left: 1, Right: 2}:  {First: &GposValueRecord{XAdvance: 10}},
			{Left: 1, Right: 12}: {First: &GposValueRecord{XAdvance: 20}},
			{Left: 12, Right: 1}: {First: &GposValueRecord{XAdvance: 30}},
		}},
		want: &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{
			{Left: 1, Right: 2}: {First: &GposValueRecord{XAdvance: 10}},
		}},
	},
	{
		name: "Gpos2_2", tp: TypeGpos, lookupType: 2,
		in: &Gpos2_2{
			Cov:    coverage.Set{1: true, 12: true},
			Class1: classdef.Table{1: 1, 12: 1},
			Class2: classdef.Table{2: 1, 13: 1},
			Adjust: [][]*PairAdjust{
				{{First: &GposValueRecord{}}, {First: &GposValueRecord{}}},
				{{First: &GposValueRecord{}}, {First: &GposValueRecord{XAdvance: 10}}},
			},
		},
		want: &Gpos2_2{
			Cov:    coverage.Set{1: true},
			Class1: classdef.Table{1: 1},
			Class2: classdef.Table{2: 1},
			Adjust: [][]*PairAdjust{
				{{First: &GposValueRecord{}}, {First: &GposValueRecord{}}},
				{{First: &GposValueRecord{}}, {First: &GposValueRecord{XAdvance: 10}}},
			},
		},
	},
	{
		name: "Gpos3_1", tp: TypeGpos, lookupType: 3,
		in: &Gpos3_1{
			Cov:     coverage.Table{1: 0, 12: 1},
			Records: []EntryExitRecord{{}, {}},
		},
		want: &Gpos3_1{
			Cov:     coverage.Table{1: 0},
			Records: []EntryExitRecord{{}},
		},
	},
	{
		name: "Gpos4_1", tp: TypeGpos, lookupType: 4,
		in: &Gpos4_1{
			MarkCov:   coverage.Table{1: 0, 12: 1},
			BaseCov:   coverage.Table{2: 0, 3: 1, 13: 2},
			MarkArray: []markarray.Record{{}, {}},
			BaseArray: [][]*anchor.Table{{nil}, {nil}, {nil}},
		},
		want: &Gpos4_1{
			MarkCov:   coverage.Table{1: 0},
			BaseCov:   coverage.Table{2: 0, 3: 1},
			MarkArray: []markarray.Record{{}},
			BaseArray: [][]*anchor.Table{{nil}, {nil}},
		},
	},
	{
		name: "Gpos5_1", tp: TypeGpos, lookupType: 5,
		in: &Gpos5_1{
			MarkCov:   coverage.Table{1: 0, 12: 1},
			LigCov:    coverage.Table{2: 0, 13: 1},
			MarkArray: []markarray.Record{{}, {}},
			LigArray:  [][][]*anchor.Table{{{nil}}, {{nil}, {nil}}},
		},
		want: &Gpos5_1{
			MarkCov:   coverage.Table{1: 0},
			LigCov:    coverage.Table{2: 0},
			MarkArray: []markarray.Record{{}},
			LigArray:  [][][]*anchor.Table{{{nil}}},
		},
	},
	{
		name: "Gpos6_1", tp: TypeGpos, lookupType: 6,
		in: &Gpos6_1{
			Mark1Cov:   coverage.Table{1: 0, 12: 1},
			Mark2Cov:   coverage.Table{2: 0, 13: 1},
			Mark1Array: []markarray.Record{{}, {}},
			Mark2Array: [][]*anchor.Table{{nil}, {nil}},
		},
		want: &Gpos6_1{
			Mark1Cov:   coverage.Table{1: 0},
			Mark2Cov:   coverage.Table{2: 0},
			Mark1Array: []markarray.Record{{}},
			Mark2Array: [][]*anchor.Table{{nil}},
		},
	},
	{
		name: "SeqContext1", tp: TypeGsub, lookupType: 5,
		in: &SeqContext1{
			Cov: coverage.Table{1: 0, 12: 1},
			Rules: [][]*SeqRule{
				{{Input: []glyph.ID{2}}, {Input: []glyph.ID{12}}},
				{{Input: []glyph.ID{3}}},
			},
		},
		want: &SeqContext1{
			Cov:   coverage.Table{1: 0},
			Rules: [][]*SeqRule{{{Input: []glyph.ID{2}}}},
		},
	},
	{
		name: "SeqContext2", tp: TypeGpos, lookupType: 7,
		in: &SeqContext2{
			Cov:   coverage.Table{1: 0, 12: 1},
			Input: classdef.Table{1: 1, 2: 1, 12: 1},
			Rules: [][]*ClassSeqRule{nil, {{Input: []uint16{1}}}},
		},
		want: &SeqContext2{
			Cov:   coverage.Table{1: 0},
			Input: classdef.Table{1: 1, 2: 1},
			Rules: [][]*ClassSeqRule{nil, {{Input: []uint16{1}}}},
		},
	},
	{
		// classes 2 and 4 are left without glyphs
		name: "SeqContext2 emptied class", tp: TypeGsub, lookupType: 5,
		in: &SeqContext2{
			Cov:   coverage.Table{1: 0, 2: 1},
			Input: classdef.Table{1: 1, 12: 2, 2: 3, 13: 4},
			Rules: [][]*ClassSeqRule{
				nil,
				{{Input: []uint16{3}}, {Input: []uint16{2}}},
				{{Input: []uint16{1}}},
				{{Input: []uint16{4}}, {Input: []uint16{3, 2}}},
				{{Input: []uint16{1}}},
			},
		},
		want: &SeqContext2{
			Cov:   coverage.Table{1: 0, 2: 1},
			Input: classdef.Table{1: 1, 2: 3},
			Rules: [][]*ClassSeqRule{nil, {{Input: []uint16{3}}}, nil, nil},
		},
	},
	{
		name: "SeqContext3", tp: TypeGsub, lookupType: 5,
		in: &SeqContext3{
			Input: []coverage.Set{{1: true, 12: true}, {2: true}},
		},
		want: &SeqContext3{
			Input: []coverage.Set{{1: true}, {2: true}},
		},
	},
	{
		name: "ChainedSeqContext1", tp: TypeGsub, lookupType: 6,
		in: &ChainedSeqContext1{
			Cov: coverage.Table{1: 0, 12: 1},
			Rules: [][]*ChainedSeqRule{
				{
					{Backtrack: []glyph.ID{12}},
					{Input: []glyph.ID{12}},
					{Lookahead: []glyph.ID{12}},
					{Backtrack: []glyph.ID{2}, Input: []glyph.ID{3}, Lookahead: []glyph.ID{4}},
				},
				{{Input: []glyph.ID{3}}},
			},
		},
		want: &ChainedSeqContext1{
			Cov: coverage.Table{1: 0},
			Rules: [][]*ChainedSeqRule{{
				{Backtrack: []glyph.ID{2}, Input: []glyph.ID{3}, Lookahead: []glyph.ID{4}},
			}},
		},
	},
	{
		name: "ChainedSeqContext2", tp: TypeGpos, lookupType: 8,
		in: &ChainedSeqContext2{
			Cov:       coverage.Table{1: 0, 12: 1},
			Backtrack: classdef.Table{2: 1, 13: 1},
			Input:     classdef.Table{1: 1, 12: 1},
			Lookahead: classdef.Table{3: 1, 14: 1},
			Rules:     [][]*ChainedClassSeqRule{nil, {{Backtrack: []uint16{1}}}},
		},
		want: &ChainedSeqContext2{
			Cov:       coverage.Table{1: 0},
			Backtrack: classdef.Table{2: 1},
			Input:     classdef.Table{1: 1},
			Lookahead: classdef.Table{3: 1},
			Rules:     [][]*ChainedClassSeqRule{nil, {{Backtrack: []uint16{1}}}},
		},
	},
	{
		// class 2 is left without glyphs in each class definition
		name: "ChainedSeqContext2 emptied class", tp: TypeGsub, lookupType: 6,
		in: &ChainedSeqContext2{
			Cov:       coverage.Table{1: 0},
			Backtrack: classdef.Table{3: 1, 12: 2},
			Input:     classdef.Table{1: 1, 13: 2},
			Lookahead: classdef.Table{4: 1, 14: 2},
			Rules: [][]*ChainedClassSeqRule{
				nil,
				{
					{Backtrack: []uint16{2}},
					{Input: []uint16{2}},
					{Lookahead: []uint16{2}},
					{Backtrack: []uint16{1}, Input: []uint16{1}, Lookahead: []uint16{1}},
				},
				{{Input: []uint16{1}}},
			},
		},
		want: &ChainedSeqContext2{
			Cov:       coverage.Table{1: 0},
			Backtrack: classdef.Table{3: 1},
			Input:     classdef.Table{1: 1},
			Lookahead: classdef.Table{4: 1},
			Rules: [][]*ChainedClassSeqRule{
				nil,
				{{Backtrack: []uint16{1}, Input: []uint16{1}, Lookahead: []uint16{1}}},
			},
		},
	},
	{
		name: "ChainedSeqContext3", tp: TypeGsub, lookupType: 6,
		in: &ChainedSeqContext3{
			Backtrack: []coverage.Set{{2: true, 12: true}},
			Input:     []coverage.Set{{1: true, 13: true}},
			Lookahead: []coverage.Set{{3: true, 14: true}},
		},
		want: &ChainedSeqContext3{
			Backtrack: []coverage.Set{{2: true}},
			Input:     []coverage.Set{{1: true}},
			Lookahead: []coverage.Set{{3: true}},
		},
	},
}

// TestLimitGlyphs checks, for each kind of subtable, that the rules which
// refer to a missing glyph are removed, that the result passes the check
// which the original table fails, and that the result survives a write/read
// cycle.
func TestLimitGlyphs(t *testing.T) {
	const n = 10 // glyphs 0, ..., 9 exist
	for _, tc := range limitTestCases {
		t.Run(tc.name, func(t *testing.T) {
			info := &Info{
				ScriptList: ScriptListInfo{
					language.MustParse("und-Latn-x-latn"): {Optional: []FeatureIndex{0}},
				},
				FeatureList: FeatureListInfo{{Tag: "test", Lookups: []LookupIndex{0}}},
				LookupList: LookupList{{
					Meta:      &LookupMetaInfo{LookupType: tc.lookupType},
					Subtables: []Subtable{tc.in},
				}},
			}
			if err := info.CheckGlyphs(n); err == nil {
				t.Error("missing glyph not detected")
			}

			limited := info.LimitGlyphs(n)
			if err := limited.CheckGlyphs(n); err != nil {
				t.Error(err)
			}
			want := []Subtable{tc.want}
			if diff := cmp.Diff(want, limited.LookupList[0].Subtables); diff != "" {
				t.Errorf("wrong subtables (-want +got):\n%s", diff)
			}

			data, err := limited.Encode(tc.tp)
			if err != nil {
				t.Fatal(err)
			}
			again, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))), tc.tp)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(limited, again, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("round trip failed (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLimitGlyphsRemovesEmptySubtables checks that subtables in which every
// rule refers to a missing glyph are removed from their lookup.
func TestLimitGlyphsRemovesEmptySubtables(t *testing.T) {
	const n = 10 // glyphs 0, ..., 9 exist
	cov := coverage.Table{12: 0}
	set := coverage.Set{12: true}
	marks := []markarray.Record{{}}
	empty := []Subtable{
		&Gpos1_1{Cov: cov},
		&Gpos1_2{Cov: cov, Adjust: []*GposValueRecord{{}}},
		&Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{{Left: 1, Right: 12}: {}}},
		&Gpos2_2{Cov: set},
		&Gpos3_1{Cov: cov, Records: []EntryExitRecord{{}}},
		&Gpos4_1{
			MarkCov: coverage.Table{1: 0}, MarkArray: marks,
			BaseCov: cov, BaseArray: [][]*anchor.Table{{nil}},
		},
		&Gpos5_1{
			MarkCov: cov, MarkArray: marks,
			LigCov: coverage.Table{1: 0}, LigArray: [][][]*anchor.Table{{{nil}}},
		},
		&Gpos6_1{
			Mark1Cov: cov, Mark1Array: marks,
			Mark2Cov: coverage.Table{1: 0}, Mark2Array: [][]*anchor.Table{{nil}},
		},
		&SeqContext1{Cov: cov, Rules: [][]*SeqRule{nil}},
		&SeqContext1{
			Cov:   coverage.Table{1: 0},
			Rules: [][]*SeqRule{{{Input: []glyph.ID{12}}}},
		},
		&SeqContext2{Cov: cov},
		&SeqContext2{
			Cov:   coverage.Table{1: 0},
			Input: classdef.Table{1: 1, 12: 2},
			Rules: [][]*ClassSeqRule{nil, {{Input: []uint16{2}}}},
		},
		&SeqContext3{Input: []coverage.Set{{1: true}, set}},
		&ChainedSeqContext1{Cov: cov, Rules: [][]*ChainedSeqRule{nil}},
		&ChainedSeqContext1{
			Cov:   coverage.Table{1: 0},
			Rules: [][]*ChainedSeqRule{{{Lookahead: []glyph.ID{12}}}},
		},
		&ChainedSeqContext2{Cov: cov},
		&ChainedSeqContext2{
			Cov:       coverage.Table{1: 0},
			Input:     classdef.Table{1: 1},
			Lookahead: classdef.Table{12: 1},
			Rules:     [][]*ChainedClassSeqRule{nil, {{Lookahead: []uint16{1}}}},
		},
		&ChainedSeqContext3{Backtrack: []coverage.Set{set}, Input: []coverage.Set{{1: true}}},
		&ChainedSeqContext3{Input: []coverage.Set{set}},
		&ChainedSeqContext3{Input: []coverage.Set{{1: true}}, Lookahead: []coverage.Set{set}},
		&Gsub8_1{
			Input:              coverage.Table{1: 0},
			Backtrack:          []coverage.Table{cov},
			SubstituteGlyphIDs: []glyph.ID{2},
		},
		&Gsub8_1{
			Input:              coverage.Table{1: 0},
			Lookahead:          []coverage.Table{cov},
			SubstituteGlyphIDs: []glyph.ID{2},
		},
	}
	for _, st := range empty {
		keep := &Gpos1_1{Cov: coverage.Table{1: 0}}
		info := &Info{LookupList: LookupList{{
			Meta:      &LookupMetaInfo{},
			Subtables: []Subtable{st, keep},
		}}}
		got := info.LimitGlyphs(n).LookupList[0].Subtables
		if diff := cmp.Diff([]Subtable{keep}, got); diff != "" {
			t.Errorf("%T: wrong subtables (-want +got):\n%s", st, diff)
		}
	}
}

// singleLookup returns a table whose only lookup holds the given subtables.
func singleLookup(lookupType uint16, subtables ...Subtable) *Info {
	return &Info{
		ScriptList: ScriptListInfo{
			language.MustParse("und-Latn-x-latn"): {Optional: []FeatureIndex{0}},
		},
		FeatureList: FeatureListInfo{{Tag: "test", Lookups: []LookupIndex{0}}},
		LookupList: LookupList{{
			Meta:      &LookupMetaInfo{LookupType: lookupType},
			Subtables: subtables,
		}},
	}
}

// writeRead encodes a table and reads it back.
func writeRead(t *testing.T, info *Info, tp Type) (*Info, []byte) {
	t.Helper()
	data, err := info.Encode(tp)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))), tp)
	if err != nil {
		t.Fatal(err)
	}
	return again, data
}

// sameMap reports whether a and b are the same map.
func sameMap[K comparable, V any](a, b map[K]V) bool {
	return offsetpool.MapKey(a) == offsetpool.MapKey(b)
}

// sameSlice reports whether a and b are the same slice.
func sameSlice[T any](a, b []T) bool {
	return offsetpool.SliceKey(a) == offsetpool.SliceKey(b)
}

// TestLimitGlyphsKeepsSharing checks, for the kinds of values which a
// subtable can use more than once, that all uses of a value get the same
// value in the limited table.
func TestLimitGlyphsKeepsSharing(t *testing.T) {
	const n = 10 // glyphs 0, ..., 9 exist
	limit := func(t *testing.T, st Subtable) Subtable {
		t.Helper()
		info := &Info{LookupList: LookupList{{
			Meta:      &LookupMetaInfo{},
			Subtables: []Subtable{st},
		}}}
		got := info.LimitGlyphs(n).LookupList[0].Subtables
		if len(got) != 1 || got[0] == st {
			t.Fatalf("subtable not limited: %v", got)
		}
		return got[0]
	}
	check := func(t *testing.T, shared bool) {
		t.Helper()
		if !shared {
			t.Error("value is not shared")
		}
	}

	set := coverage.Set{1: true, 12: true}
	cov := coverage.Table{2: 0, 12: 1}
	classes := classdef.Table{1: 1, 2: 2, 12: 3}

	t.Run("alternates", func(t *testing.T) {
		alts := []glyph.ID{3, 12, 4}
		st := limit(t, &Gsub3_1{
			Cov:        coverage.Table{1: 0, 2: 1},
			Alternates: [][]glyph.ID{alts, alts},
		}).(*Gsub3_1)
		check(t, sameSlice(st.Alternates[0], st.Alternates[1]))
	})
	t.Run("ligature sets", func(t *testing.T) {
		ligs := []Ligature{{In: []glyph.ID{3}, Out: 4}, {In: []glyph.ID{12}, Out: 5}}
		st := limit(t, &Gsub4_1{
			Cov:  coverage.Table{1: 0, 2: 1},
			Repl: [][]Ligature{ligs, ligs},
		}).(*Gsub4_1)
		check(t, sameSlice(st.Repl[0], st.Repl[1]))
	})
	t.Run("coverage tables", func(t *testing.T) {
		st := limit(t, &Gsub8_1{
			Input:              coverage.Table{1: 0},
			Backtrack:          []coverage.Table{cov, cov},
			Lookahead:          []coverage.Table{cov},
			SubstituteGlyphIDs: []glyph.ID{3},
		}).(*Gsub8_1)
		check(t, sameMap(st.Backtrack[0], st.Backtrack[1]))
		check(t, sameMap(st.Backtrack[0], st.Lookahead[0]))
	})
	t.Run("glyph sets", func(t *testing.T) {
		st := limit(t, &SeqContext3{Input: []coverage.Set{set, set}}).(*SeqContext3)
		check(t, sameMap(st.Input[0], st.Input[1]))
	})
	t.Run("class definitions", func(t *testing.T) {
		st := limit(t, &Gpos2_2{
			Cov:    coverage.Set{1: true},
			Class1: classes,
			Class2: classes,
			Adjust: [][]*PairAdjust{{{}, {}, {}}, {{}, {}, {}}, {{}, {}, {}}},
		}).(*Gpos2_2)
		check(t, sameMap(st.Class1, st.Class2))
	})
	t.Run("rule sets", func(t *testing.T) {
		rules := []*SeqRule{{Input: []glyph.ID{3}}, {Input: []glyph.ID{12}}}
		st := limit(t, &SeqContext1{
			Cov:   coverage.Table{1: 0, 2: 1},
			Rules: [][]*SeqRule{rules, rules},
		}).(*SeqContext1)
		check(t, sameSlice(st.Rules[0], st.Rules[1]))
	})
	t.Run("chained rule sets", func(t *testing.T) {
		rules := []*ChainedSeqRule{{Input: []glyph.ID{3}}, {Lookahead: []glyph.ID{12}}}
		st := limit(t, &ChainedSeqContext1{
			Cov:   coverage.Table{1: 0, 2: 1},
			Rules: [][]*ChainedSeqRule{rules, rules},
		}).(*ChainedSeqContext1)
		check(t, sameSlice(st.Rules[0], st.Rules[1]))
	})
	t.Run("class rule sets", func(t *testing.T) {
		// class 3 is left without glyphs
		rules := []*ClassSeqRule{{Input: []uint16{2}}, {Input: []uint16{3}}}
		st := limit(t, &SeqContext2{
			Cov:   coverage.Table{1: 0, 2: 1},
			Input: classes,
			Rules: [][]*ClassSeqRule{nil, rules, rules},
		}).(*SeqContext2)
		check(t, len(st.Rules[1]) == 1 && sameSlice(st.Rules[1], st.Rules[2]))
	})
	t.Run("chained class rule sets", func(t *testing.T) {
		// class 3 is left without glyphs
		rules := []*ChainedClassSeqRule{{Backtrack: []uint16{2}}, {Lookahead: []uint16{3}}}
		st := limit(t, &ChainedSeqContext2{
			Cov:       coverage.Table{1: 0, 2: 1},
			Backtrack: classes,
			Input:     classes,
			Lookahead: classes,
			Rules:     [][]*ChainedClassSeqRule{nil, rules, rules},
		}).(*ChainedSeqContext2)
		check(t, sameMap(st.Backtrack, st.Input) && sameMap(st.Input, st.Lookahead))
		check(t, len(st.Rules[1]) == 1 && sameSlice(st.Rules[1], st.Rules[2]))
	})
	t.Run("subtables", func(t *testing.T) {
		st := &Gsub1_2{Cov: coverage.Table{1: 0, 2: 1}, SubstituteGlyphIDs: []glyph.ID{3, 12}}
		lookup := &LookupTable{Meta: &LookupMetaInfo{}, Subtables: []Subtable{st, st}}
		info := &Info{LookupList: LookupList{lookup, lookup}}
		ll := info.LimitGlyphs(n).LookupList
		check(t, ll[0] == ll[1] && ll[0] != lookup)
		check(t, ll[0].Subtables[0] == ll[0].Subtables[1])
	})
}

// TestLimitGlyphsSharedCoverage checks a subtable which uses one large glyph
// set at many positions of its context.  The set must be shared in the
// limited table, too: a table with a separate set for every position would
// need memory far beyond the size of the original.
func TestLimitGlyphsSharedCoverage(t *testing.T) {
	const (
		numGlyphs = 60000
		numUses   = 10000 // in the backtrack and in the lookahead sequence
	)
	set := make(coverage.Set, numGlyphs)
	for gid := range numGlyphs {
		set[glyph.ID(gid)] = true
	}
	context := make([]coverage.Set, numUses)
	for i := range context {
		context[i] = set
	}
	info := singleLookup(6, &ChainedSeqContext3{
		Backtrack: context,
		Input:     []coverage.Set{set},
		Lookahead: context,
	})
	_, data := writeRead(t, info, TypeGsub)

	if err := info.CheckGlyphs(numGlyphs); err != nil {
		t.Error(err)
	}
	if info.LimitGlyphs(numGlyphs) != info {
		t.Error("table without missing glyphs was replaced")
	}
	if err := info.CheckGlyphs(numGlyphs - 1); err == nil {
		t.Error("missing glyph not detected")
	}

	const n = 10 // glyphs 0, ..., 9 exist
	want := coverage.Set{}
	for gid := range n {
		want[glyph.ID(gid)] = true
	}
	checkShared := func(info *Info) {
		t.Helper()
		if err := info.CheckGlyphs(n); err != nil {
			t.Error(err)
		}
		if info.LimitGlyphs(n) != info {
			t.Error("limited table was replaced")
		}
		st := info.LookupList[0].Subtables[0].(*ChainedSeqContext3)
		if len(st.Backtrack) != numUses || len(st.Input) != 1 || len(st.Lookahead) != numUses {
			t.Fatalf("wrong context lengths %d, %d, %d",
				len(st.Backtrack), len(st.Input), len(st.Lookahead))
		}
		shared := st.Input[0]
		if diff := cmp.Diff(want, shared); diff != "" {
			t.Errorf("wrong glyph set (-want +got):\n%s", diff)
		}
		for _, sets := range [][]coverage.Set{st.Backtrack, st.Lookahead} {
			for _, s := range sets {
				if !sameMap(s, shared) {
					t.Fatal("glyph set is not shared")
				}
			}
		}
	}

	limited := info.LimitGlyphs(n)
	checkShared(limited)
	again, limitedData := writeRead(t, limited, TypeGsub)
	checkShared(again)
	if len(limitedData) > len(data) {
		t.Errorf("limited table has %d bytes, original has %d", len(limitedData), len(data))
	}
}

// TestLimitGlyphsSharedLigatures checks a ligature subtable where many
// glyphs share one ligature set, and the ligatures of the set share one long
// list of components.  Visiting the components once for every glyph and
// ligature would take time far beyond the size of the table, and the limited
// table must share what the original shares.
func TestLimitGlyphsSharedLigatures(t *testing.T) {
	const (
		numFirst = 20000 // glyphs which share the ligature set
		numLigs  = 20000 // ligatures which share the components
		numComp  = 50000
		lastGID  = 59999 // the last glyph used by the valid ligatures
		badGID   = 65000
	)
	components := make([]glyph.ID, numComp)
	for i := range components {
		components[i] = glyph.ID(lastGID - i)
	}
	ligs := make([]Ligature, numLigs, numLigs+1)
	for i := range ligs {
		ligs[i] = Ligature{In: components, Out: 7}
	}
	ligs = append(ligs, Ligature{In: []glyph.ID{1}, Out: badGID})
	cov := make(coverage.Table, numFirst)
	repl := make([][]Ligature, numFirst)
	for i := range numFirst {
		cov[glyph.ID(i+1)] = i
		repl[i] = ligs
	}
	info := singleLookup(4, &Gsub4_1{Cov: cov, Repl: repl})
	_, data := writeRead(t, info, TypeGsub)

	if err := info.CheckGlyphs(badGID + 1); err != nil {
		t.Error(err)
	}
	if info.LimitGlyphs(badGID+1) != info {
		t.Error("table without missing glyphs was replaced")
	}
	if err := info.CheckGlyphs(badGID); err == nil {
		t.Error("missing glyph not detected")
	}

	const n = lastGID + 1
	checkShared := func(info *Info) {
		t.Helper()
		if err := info.CheckGlyphs(n); err != nil {
			t.Error(err)
		}
		if info.LimitGlyphs(n) != info {
			t.Error("limited table was replaced")
		}
		st := info.LookupList[0].Subtables[0].(*Gsub4_1)
		if len(st.Cov) != numFirst || len(st.Repl) != numFirst {
			t.Fatalf("wrong number of ligature sets: %d, %d", len(st.Cov), len(st.Repl))
		}
		shared := st.Repl[0]
		for _, set := range st.Repl {
			if !sameSlice(set, shared) {
				t.Fatal("ligature set is not shared")
			}
		}
		if len(shared) != numLigs {
			t.Fatalf("wrong number of ligatures: %d", len(shared))
		}
		if !slices.Equal(shared[0].In, components) {
			t.Error("wrong components")
		}
		for _, lig := range shared {
			if !sameSlice(lig.In, shared[0].In) || lig.Out != 7 {
				t.Fatal("ligature is not shared")
			}
		}
	}

	limited := info.LimitGlyphs(n)
	checkShared(limited)
	again, limitedData := writeRead(t, limited, TypeGsub)
	checkShared(again)
	if len(limitedData) > len(data) {
		t.Errorf("limited table has %d bytes, original has %d", len(limitedData), len(data))
	}
}

// TestLimitGlyphsEncodable checks that a table which was read from a file
// can still be written once glyphs are removed.  Here the removed glyphs
// split the coverage table of the subtable into many ranges, so that the
// subtable no longer fits its offsets.
func TestLimitGlyphsEncodable(t *testing.T) {
	const (
		numCovered = 30000
		n          = 60000 // number of glyphs in the font
	)
	shared := []glyph.ID{5}
	missing := []glyph.ID{65000}
	long := make([]glyph.ID, 9999)
	for i := range long {
		long[i] = glyph.ID(i)
	}
	cov := make(coverage.Table, numCovered)
	alternates := make([][]glyph.ID, numCovered)
	for i := range numCovered {
		gid := glyph.ID(i + 1)
		cov[gid] = i
		switch {
		case gid%6 == 0:
			alternates[i] = missing
		case i == 0:
			alternates[i] = long
		default:
			alternates[i] = shared
		}
	}
	info, _ := writeRead(t, singleLookup(3, &Gsub3_1{Cov: cov, Alternates: alternates}), TypeGsub)
	if len(info.LookupList[0].Subtables) != 1 {
		t.Fatal("subtable not read")
	}

	limited := info.LimitGlyphs(n)
	if err := limited.CheckGlyphs(n); err != nil {
		t.Error(err)
	}
	again, _ := writeRead(t, limited, TypeGsub)
	if diff := cmp.Diff(limited, again); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	if again.LimitGlyphs(n) != again {
		t.Error("limited table was replaced")
	}
}

// TestLimitGlyphsHeaderOffsets checks that a table which was read from a
// file can still be written if removing glyphs makes the lookup list longer.
// Here the script list and the feature list are so long that the header
// reaches the original lookup list, but not a longer one.
func TestLimitGlyphsHeaderOffsets(t *testing.T) {
	const (
		numCovered = 10000
		n          = 60000 // number of glyphs in the font
	)
	cov := make(coverage.Table, numCovered)
	subst := make([]glyph.ID, numCovered)
	for i := range numCovered {
		gid := glyph.ID(i + 1)
		cov[gid] = i
		subst[i] = 5
		if gid%5 == 0 {
			subst[i] = 65000
		}
	}
	info := singleLookup(1, &Gsub1_2{Cov: cov, SubstituteGlyphIDs: subst})
	for tag := range info.ScriptList {
		info.ScriptList[tag].Optional = make([]FeatureIndex, 20000)
	}
	info.FeatureList[0].Lookups = make([]LookupIndex, 20000)
	info, _ = writeRead(t, info, TypeGsub)
	if len(info.LookupList[0].Subtables) != 1 {
		t.Fatal("subtable not read")
	}

	limited := info.LimitGlyphs(n)
	if err := limited.CheckGlyphs(n); err != nil {
		t.Error(err)
	}
	again, _ := writeRead(t, limited, TypeGsub)
	if diff := cmp.Diff(limited, again); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	if again.LimitGlyphs(n) != again {
		t.Error("limited table was replaced")
	}
}

// TestLimitGlyphsGsub1_1Wraps checks that the glyph a Gsub1_1 subtable
// produces is the sum of the input glyph and the delta modulo 65536, and
// that this is the glyph which must exist.
func TestLimitGlyphsGsub1_1Wraps(t *testing.T) {
	const n = 10 // glyphs 0, ..., 9 exist

	// glyph 3 becomes glyph 2, and glyph 0 becomes the missing glyph 65535
	info := singleLookup(1, &Gsub1_1{Cov: coverage.Set{0: true, 3: true}, Delta: 0xFFFF})
	seq := NewContext(info.LookupList, nil, []LookupIndex{0}).Apply([]glyph.Info{{GID: 3}})
	if len(seq) != 1 || seq[0].GID != 2 {
		t.Errorf("wrong substitution: %v", seq)
	}
	if err := info.CheckGlyphs(n); err == nil {
		t.Error("missing glyph not detected")
	}
	want := []Subtable{&Gsub1_1{Cov: coverage.Set{3: true}, Delta: 0xFFFF}}
	limited := info.LimitGlyphs(n)
	if diff := cmp.Diff(want, limited.LookupList[0].Subtables); diff != "" {
		t.Errorf("wrong subtables (-want +got):\n%s", diff)
	}

	// all substitutions stay inside the font
	info = singleLookup(1, &Gsub1_1{Cov: coverage.Set{3: true, 9: true}, Delta: 0xFFFF})
	if err := info.CheckGlyphs(n); err != nil {
		t.Error(err)
	}
	if info.LimitGlyphs(n) != info {
		t.Error("table without missing glyphs was replaced")
	}
}

// FuzzLimitGlyphs checks that a table read from a file, with the glyphs
// which a font lacks removed, can be written and read back unchanged.
func FuzzLimitGlyphs(f *testing.F) {
	for _, tc := range limitTestCases {
		data, err := singleLookup(tc.lookupType, tc.in).Encode(tc.tp)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data, tc.tp == TypeGpos, uint16(9))
	}

	f.Fuzz(func(t *testing.T, data1 []byte, gpos bool, lastGlyph uint16) {
		tp := Type(TypeGsub)
		if gpos {
			tp = TypeGpos
		}
		n := int(lastGlyph) + 1 // number of glyphs in the font
		info, err := Read(bytes.NewReader(data1), parser.NewBudget(int64(len(data1))), tp)
		if err != nil {
			return
		}
		if _, err := info.Encode(tp); err != nil {
			t.Fatalf("table cannot be encoded as read: %v", err)
		}

		limited := info.LimitGlyphs(n)
		if err := limited.CheckGlyphs(n); err != nil {
			t.Fatal(err)
		}
		if limited.LimitGlyphs(n) != limited {
			t.Error("limited table needs further limiting")
		}

		data2, err := limited.Encode(tp)
		if err != nil {
			t.Fatal(err)
		}
		// The decoded table fits the budget of the original input.  A more
		// compact encoding of the same data must not get a smaller one.
		budget := parser.NewBudget(int64(max(len(data1), len(data2))))
		again, err := Read(bytes.NewReader(data2), budget, tp)
		if err != nil {
			t.Fatal(err)
		}
		if again.LimitGlyphs(n) != again {
			t.Error("table read back needs further limiting")
		}
		// DeepEqual compares values which are used many times only once
		if !reflect.DeepEqual(limited, again) {
			t.Errorf("round trip failed (-want +got):\n%s", cmp.Diff(limited, again))
		}
	})
}
