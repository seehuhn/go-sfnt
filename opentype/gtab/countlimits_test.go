// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"testing"

	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/markarray"
)

// tooMany is one more than a 16-bit count can hold.
const tooMany = 0x10000

// checkLayoutError checks that the layout of l fails.
func checkLayoutError(t *testing.T, l Subtable) {
	t.Helper()
	if _, err := l.layout(); err == nil {
		t.Error("layout succeeded")
	}
}

// alternatingClasses returns a class definition table which cannot be
// encoded, since it assigns a class to every glyph and each glyph starts a
// new range.
func alternatingClasses() classdef.Table {
	cd := classdef.Table{}
	for i := range tooMany {
		cd[glyph.ID(i)] = uint16(i%2 + 1)
	}
	return cd
}

func TestGpos1_2TooManyRecords(t *testing.T) {
	checkLayoutError(t, &Gpos1_2{
		Cov:    coverage.Table{},
		Adjust: make([]*GposValueRecord, tooMany),
	})
}

func TestGpos2_1TooManyPairs(t *testing.T) {
	adj := map[glyph.Pair]*PairAdjust{}
	for i := range tooMany {
		adj[glyph.Pair{Left: 0, Right: glyph.ID(i)}] = &PairAdjust{
			First: &GposValueRecord{XAdvance: 1},
		}
	}
	checkLayoutError(t, &Gpos2_1{Adjust: adj})
}

func TestMarkAttachTooManyMarkClasses(t *testing.T) {
	marks := []markarray.Record{{Class: 0xFFFF, Table: anchor.Table{}}}
	t.Run("Gpos4_1", func(t *testing.T) {
		checkLayoutError(t, &Gpos4_1{
			MarkCov:   coverage.Table{1: 0},
			BaseCov:   coverage.Table{},
			MarkArray: marks,
		})
	})
	t.Run("Gpos5_1", func(t *testing.T) {
		checkLayoutError(t, &Gpos5_1{
			MarkCov:   coverage.Table{1: 0},
			LigCov:    coverage.Table{},
			MarkArray: marks,
		})
	})
	t.Run("Gpos6_1", func(t *testing.T) {
		checkLayoutError(t, &Gpos6_1{
			Mark1Cov:   coverage.Table{1: 0},
			Mark2Cov:   coverage.Table{},
			Mark1Array: marks,
		})
	})
}

func TestAnchorMatrixTooManyRows(t *testing.T) {
	t.Run("Gpos4_1", func(t *testing.T) {
		checkLayoutError(t, &Gpos4_1{
			MarkCov:   coverage.Table{},
			BaseCov:   coverage.Table{},
			BaseArray: make([][]*anchor.Table, tooMany),
		})
	})
	t.Run("Gpos5_1", func(t *testing.T) {
		checkLayoutError(t, &Gpos5_1{
			MarkCov:  coverage.Table{},
			LigCov:   coverage.Table{1: 0},
			LigArray: [][][]*anchor.Table{make([][]*anchor.Table, tooMany)},
		})
	})
	t.Run("Gpos6_1", func(t *testing.T) {
		checkLayoutError(t, &Gpos6_1{
			Mark1Cov:   coverage.Table{},
			Mark2Cov:   coverage.Table{},
			Mark2Array: make([][]*anchor.Table, tooMany),
		})
	})
}

func TestGsubSequencesTooLong(t *testing.T) {
	long := make([]glyph.ID, tooMany)
	t.Run("Gsub2_1", func(t *testing.T) {
		checkLayoutError(t, &Gsub2_1{
			Cov:  coverage.Table{1: 0},
			Repl: [][]glyph.ID{long},
		})
	})
	t.Run("Gsub3_1", func(t *testing.T) {
		checkLayoutError(t, &Gsub3_1{
			Cov:        coverage.Table{1: 0},
			Alternates: [][]glyph.ID{long},
		})
	})
	t.Run("Gsub4_1", func(t *testing.T) {
		checkLayoutError(t, &Gsub4_1{
			Cov:  coverage.Table{1: 0},
			Repl: [][]Ligature{{{In: long[:tooMany-1], Out: 2}}},
		})
	})
}

func TestRulesTooLong(t *testing.T) {
	glyphs := make([]glyph.ID, tooMany)
	classes := make([]uint16, tooMany)
	actions := make([]SeqLookup, tooMany)
	cov := coverage.Table{1: 0}
	cd := classdef.Table{1: 1}

	seq := func(input []glyph.ID, actions []SeqLookup) Subtable {
		return &SeqContext1{Cov: cov, Rules: [][]*SeqRule{{{Input: input, Actions: actions}}}}
	}
	classSeq := func(input []uint16, actions []SeqLookup) Subtable {
		return &SeqContext2{Cov: cov, Input: cd,
			Rules: [][]*ClassSeqRule{nil, {{Input: input, Actions: actions}}}}
	}
	chained := func(b, i, a []glyph.ID, actions []SeqLookup) Subtable {
		return &ChainedSeqContext1{Cov: cov, Rules: [][]*ChainedSeqRule{{{
			Backtrack: b, Input: i, Lookahead: a, Actions: actions}}}}
	}
	chainedClass := func(b, i, a []uint16, actions []SeqLookup) Subtable {
		return &ChainedSeqContext2{Cov: cov, Backtrack: cd, Input: cd, Lookahead: cd,
			Rules: [][]*ChainedClassSeqRule{nil, {{
				Backtrack: b, Input: i, Lookahead: a, Actions: actions}}}}
	}

	cases := map[string]Subtable{
		"SeqContext1 input":            seq(glyphs[:0xFFFF], nil),
		"SeqContext1 actions":          seq(nil, actions),
		"SeqContext2 input":            classSeq(classes[:0xFFFF], nil),
		"SeqContext2 actions":          classSeq(nil, actions),
		"ChainedSeqContext1 backtrack": chained(glyphs, nil, nil, nil),
		"ChainedSeqContext1 input":     chained(nil, glyphs[:0xFFFF], nil, nil),
		"ChainedSeqContext1 lookahead": chained(nil, nil, glyphs, nil),
		"ChainedSeqContext1 actions":   chained(nil, nil, nil, actions),
		"ChainedSeqContext2 backtrack": chainedClass(classes, nil, nil, nil),
		"ChainedSeqContext2 input":     chainedClass(nil, classes[:0xFFFF], nil, nil),
		"ChainedSeqContext2 lookahead": chainedClass(nil, nil, classes, nil),
		"ChainedSeqContext2 actions":   chainedClass(nil, nil, nil, actions),
		"SeqContext3 actions":          &SeqContext3{Actions: actions},
		"ChainedSeqContext3 actions":   &ChainedSeqContext3{Actions: actions},
	}
	for name, l := range cases {
		t.Run(name, func(t *testing.T) {
			checkLayoutError(t, l)
		})
	}
}

func TestClassDefTooLarge(t *testing.T) {
	cd := alternatingClasses()
	cov := coverage.Table{1: 0}
	t.Run("Gpos2_2", func(t *testing.T) {
		checkLayoutError(t, &Gpos2_2{
			Cov:    coverage.Set{1: true},
			Class1: cd,
			Class2: classdef.Table{},
		})
	})
	t.Run("SeqContext2", func(t *testing.T) {
		checkLayoutError(t, &SeqContext2{Cov: cov, Input: cd})
	})
	t.Run("ChainedSeqContext2", func(t *testing.T) {
		checkLayoutError(t, &ChainedSeqContext2{Cov: cov, Input: cd})
	})
}

func TestFeatureTooManyLookups(t *testing.T) {
	info := FeatureListInfo{{Tag: "liga", Lookups: make([]LookupIndex, tooMany)}}
	if _, err := info.encode(); err == nil {
		t.Error("encode succeeded")
	}
}

func TestFeatureTooManyCharacters(t *testing.T) {
	info := FeatureListInfo{{
		Tag:    "cv01",
		Params: &FeatureParamsCharacterVariants{Characters: make([]rune, tooMany)},
	}}
	if _, err := info.encode(); err == nil {
		t.Error("encode succeeded")
	}
}

func TestLangSysTooManyFeatures(t *testing.T) {
	info := ScriptListInfo{
		language.MustParse("und-Latn"): {Optional: make([]FeatureIndex, tooMany)},
	}
	if _, err := info.encode(); err == nil {
		t.Error("encode succeeded")
	}
}

func TestFeatureVariationsTooMany(t *testing.T) {
	cases := map[string]FeatureVariationRecord{
		"conditions":    {Conditions: make([]Condition, tooMany)},
		"substitutions": {Substitutions: make([]FeatureSubstitution, tooMany)},
		"lookups": {Substitutions: []FeatureSubstitution{
			{Lookups: make([]LookupIndex, tooMany)},
		}},
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := encodeFeatureVariations([]FeatureVariationRecord{rec})
			if err == nil {
				t.Error("encode succeeded")
			}
		})
	}
}

func TestLookupListTooManyLookups(t *testing.T) {
	lookup := &LookupTable{Meta: &LookupMetaInfo{LookupType: 1}}
	ll := make(LookupList, 1<<14)
	for i := range ll {
		ll[i] = lookup
	}
	if _, err := ll.encode(); err == nil {
		t.Error("encode succeeded")
	}
}

func TestLookupTooManySubtables(t *testing.T) {
	sub := &Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1}
	subtables := make([]Subtable, 1<<14)
	for i := range subtables {
		subtables[i] = sub
	}
	ll := LookupList{{
		Meta:      &LookupMetaInfo{LookupType: 1},
		Subtables: subtables,
	}}
	if _, err := ll.encode(); err == nil {
		t.Error("encode succeeded")
	}
}
