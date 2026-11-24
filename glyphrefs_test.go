// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/language"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/opentype/mathtable"
)

// simpleInfo returns a GSUB or GPOS table with one feature and one lookup.
func simpleInfo(typ uint16, st gtab.Subtable) *gtab.Info {
	return &gtab.Info{
		ScriptList: gtab.ScriptListInfo{
			language.MustParse("und-Latn"): {Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: gtab.FeatureListInfo{{Tag: "liga", Lookups: []gtab.LookupIndex{0}}},
		LookupList: gtab.LookupList{{
			Meta:      &gtab.LookupMetaInfo{LookupType: typ},
			Subtables: []gtab.Subtable{st},
		}},
	}
}

// writeReadBack writes a font and reads it back.
func writeReadBack(t *testing.T, font *sfnt.Font) *sfnt.Font {
	t.Helper()
	buf := &bytes.Buffer{}
	if _, err := font.Write(buf); err != nil {
		t.Fatal(err)
	}
	return readBack(t, buf.Bytes())
}

// markSetGsub returns a GSUB table with a lookup which uses the given
// mark filtering set.
func markSetGsub(set uint16) *gtab.Info {
	info := simpleInfo(1, &gtab.Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1})
	meta := info.LookupList[0].Meta
	meta.LookupFlags = gtab.UseMarkFilteringSet
	meta.MarkFilteringSet = set
	return info
}

// markSetGpos returns a GPOS table with a lookup which uses the given
// mark filtering set.
func markSetGpos(set uint16) *gtab.Info {
	info := simpleInfo(1, &gtab.Gpos1_1{
		Cov:    coverage.Table{1: 0},
		Adjust: &gtab.GposValueRecord{XAdvance: 1},
	})
	meta := info.LookupList[0].Meta
	meta.LookupFlags = gtab.UseMarkFilteringSet
	meta.MarkFilteringSet = set
	return info
}

func TestWriteRejectsInvalidReferences(t *testing.T) {
	missing := func(f *sfnt.Font) glyph.ID { return glyph.ID(f.NumGlyphs() + 10) }

	tests := []struct {
		name string
		edit func(f *sfnt.Font)
	}{
		{"math glyph", func(f *sfnt.Font) {
			f.Math = &mathtable.Info{
				ItalicsCorrection: map[glyph.ID]mathtable.Value{missing(f): {V: 5}},
			}
		}},
		{"ligature output", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(4, &gtab.Gsub4_1{
				Cov:  coverage.Table{1: 0},
				Repl: [][]gtab.Ligature{{{In: []glyph.ID{2}, Out: missing(f)}}},
			})
		}},
		{"feature lookup", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(1, &gtab.Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1})
			f.Gsub.FeatureList[0].Lookups = []gtab.LookupIndex{5}
		}},
		{"nested lookup", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(5, &gtab.SeqContext3{
				Input:   []coverage.Set{{1: true}},
				Actions: []gtab.SeqLookup{{LookupListIndex: 7}},
			})
		}},
		{"lookup type", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(1, &gtab.Gsub4_1{
				Cov:  coverage.Table{1: 0},
				Repl: [][]gtab.Ligature{{{In: []glyph.ID{2}, Out: 3}}},
			})
		}},
		{"coverage indices", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(4, &gtab.Gsub4_1{
				Cov: coverage.Table{1: 0, 2: 5},
				Repl: [][]gtab.Ligature{
					{{In: []glyph.ID{2}, Out: 3}},
					{{In: []glyph.ID{2}, Out: 3}},
				},
			})
		}},
		{"mark filtering set without GDEF", func(f *sfnt.Font) {
			f.Gsub = markSetGsub(0)
		}},
		{"mark filtering set out of range", func(f *sfnt.Font) {
			f.Gdef = &gdef.Table{MarkGlyphSets: []coverage.Set{{1: true}}}
			f.Gsub = markSetGsub(1)
		}},
		{"second value record without PositionSecond", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(2, &gtab.Gpos2_1{
				Adjust: map[glyph.Pair]*gtab.PairAdjust{
					{Left: 1, Right: 2}: {Second: &gtab.GposValueRecord{XAdvance: 1}},
				},
			})
		}},
		{"class pair rows of different width", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(2, &gtab.Gpos2_2{
				Cov:    coverage.Set{1: true},
				Class1: classdef.Table{1: 1},
				Class2: classdef.Table{2: 1},
				Adjust: [][]*gtab.PairAdjust{{nil, nil}, {nil}},
			})
		}},
		{"mark class out of range", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(4, &gtab.Gpos4_1{
				MarkCov:   coverage.Table{1: 0},
				BaseCov:   coverage.Table{2: 0},
				MarkArray: []markarray.Record{{Class: 1}},
				BaseArray: [][]*anchor.Table{{nil}},
			})
		}},
		{"empty replacement sequence", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(2, &gtab.Gsub2_1{
				Cov:  coverage.Table{1: 0},
				Repl: [][]glyph.ID{{}},
			})
		}},
		{"empty alternate set", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(3, &gtab.Gsub3_1{
				Cov:        coverage.Table{1: 0},
				Alternates: [][]glyph.ID{{}},
			})
		}},
		{"language without OpenType tag", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(1, &gtab.Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1})
			f.Gsub.ScriptList = gtab.ScriptListInfo{
				language.MustParse("tlh-Latn"): {Optional: []gtab.FeatureIndex{0}},
			}
		}},
		{"gdef attach list shape", func(f *sfnt.Font) {
			f.Gdef = &gdef.Table{AttachList: &gdef.AttachList{
				Cov:    coverage.Table{99: 0, 5: 1},
				Points: [][]uint16{{1}},
			}}
		}},
		{"gdef ligature caret list shape", func(f *sfnt.Font) {
			f.Gdef = &gdef.Table{LigCaretList: &gdef.LigCaretList{
				Cov:    coverage.Table{99: 0, 5: 1},
				Carets: [][]gdef.CaretValue{{{}}},
			}}
		}},
		{"gsub1_2 data length", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(1, &gtab.Gsub1_2{Cov: coverage.Table{1: 0, 2: 1}})
		}},
		{"gsub2_1 data length", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(2, &gtab.Gsub2_1{Cov: coverage.Table{1: 0}})
		}},
		{"gsub3_1 data length", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(3, &gtab.Gsub3_1{Cov: coverage.Table{1: 0}})
		}},
		{"gsub4_1 data length", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(4, &gtab.Gsub4_1{Cov: coverage.Table{1: 0}})
		}},
		{"gsub8_1 data length", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(8, &gtab.Gsub8_1{Input: coverage.Table{1: 0}})
		}},
		{"gpos1_2 data length", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(1, &gtab.Gpos1_2{Cov: coverage.Table{1: 0}})
		}},
		{"gpos3_1 data length", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(3, &gtab.Gpos3_1{Cov: coverage.Table{1: 0, 2: 1}})
		}},
		{"gpos4_1 data length", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(4, &gtab.Gpos4_1{MarkCov: coverage.Table{1: 0}})
		}},
		{"gpos5_1 data length", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(5, &gtab.Gpos5_1{MarkCov: coverage.Table{1: 0}})
		}},
		{"gpos6_1 data length", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(6, &gtab.Gpos6_1{Mark1Cov: coverage.Table{1: 0}})
		}},
		{"seq context 1 data length", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(5, &gtab.SeqContext1{Cov: coverage.Table{1: 0}})
		}},
		{"chained seq context 1 data length", func(f *sfnt.Font) {
			f.Gsub = simpleInfo(6, &gtab.ChainedSeqContext1{Cov: coverage.Table{1: 0}})
		}},
		{"gpos context glyph", func(f *sfnt.Font) {
			f.Gpos = simpleInfo(7, &gtab.SeqContext3{
				Input: []coverage.Set{{missing(f): true}},
			})
		}},
		{"gdef glyph class", func(f *sfnt.Font) {
			f.Gdef = &gdef.Table{
				GlyphClass: classdef.Table{missing(f): gdef.GlyphClassMark},
			}
		}},
		{"gdef mark glyph set", func(f *sfnt.Font) {
			f.Gdef = &gdef.Table{
				MarkGlyphSets: []coverage.Set{{missing(f): true}},
			}
		}},
		{"gpos mark filtering set out of range", func(f *sfnt.Font) {
			f.Gdef = &gdef.Table{MarkGlyphSets: []coverage.Set{{1: true}}}
			f.Gpos = markSetGpos(1)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			font := debug.MakeSimpleFont()
			tc.edit(font)
			if _, err := font.Write(&bytes.Buffer{}); err == nil {
				t.Error("invalid font was written")
			}
		})
	}
}

// A feature may list a lookup which does not exist.  Reading drops the
// index, and the result can be written and read again.
func TestReadDropsInvalidFeatureLookups(t *testing.T) {
	font := debug.MakeSimpleFont()
	gsub := simpleInfo(1, &gtab.Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1})
	data, err := gsub.Encode(gtab.TypeGsub)
	if err != nil {
		t.Fatal(err)
	}

	// patch the single lookup index of the feature table, which follows
	// the feature parameters offset and the lookup count
	data = bytes.Clone(data)
	featureListOffset := int(data[6])<<8 | int(data[7])
	n := int(data[featureListOffset])<<8 | int(data[featureListOffset+1])
	if n != 1 {
		t.Fatalf("unexpected feature count %d", n)
	}
	featureOffset := featureListOffset + int(data[featureListOffset+6])<<8 + int(data[featureListOffset+7])
	data[featureOffset+4], data[featureOffset+5] = 0, 5

	font1 := readBack(t, editTables(t, font, func(tables map[string][]byte) {
		tables["GSUB"] = data
	}))

	font2 := writeReadBack(t, font1)
	if diff := cmp.Diff(font1.Gsub, font2.Gsub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// A lookup may select a mark filtering set which the GDEF table does not
// have.  Reading clears the selection, and the result can be written and read
// again.
func TestReadClearsInvalidMarkFilteringSet(t *testing.T) {
	font := debug.MakeSimpleFont()
	font.Gdef = &gdef.Table{MarkGlyphSets: []coverage.Set{{1: true}}}
	font.Gsub = markSetGsub(0)

	// replace the GDEF table by one without mark glyph sets
	plain, err := (&gdef.Table{}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	font1 := readBack(t, editTables(t, font, func(tables map[string][]byte) {
		tables["GDEF"] = plain
	}))

	font2 := writeReadBack(t, font1)
	if diff := cmp.Diff(font1.Gsub, font2.Gsub); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// A GDEF table may list glyphs which the font does not have.  Reading drops
// them, and the result can be written and read again.
func TestReadGdefOutOfRange(t *testing.T) {
	font := debug.MakeSimpleFont()
	n := glyph.ID(font.NumGlyphs())
	table := &gdef.Table{
		GlyphClass: classdef.Table{
			1:      gdef.GlyphClassBase,
			2:      gdef.GlyphClassMark,
			n + 10: gdef.GlyphClassMark,
		},
		AttachList: &gdef.AttachList{
			Cov:    coverage.Table{1: 0, n + 1: 1},
			Points: [][]uint16{{1}, {2}},
		},
		LigCaretList: &gdef.LigCaretList{
			Cov:    coverage.Table{3: 0, n + 2: 1},
			Carets: [][]gdef.CaretValue{{{Coordinate: 100}}, {{Coordinate: 200}}},
		},
		MarkAttachClass: classdef.Table{2: 1, n + 3: 2},
		MarkGlyphSets:   []coverage.Set{{2: true, n + 4: true}, {n + 5: true}},
	}
	data, err := table.Encode()
	if err != nil {
		t.Fatal(err)
	}
	font1 := readBack(t, editTables(t, font, func(tables map[string][]byte) {
		tables["GDEF"] = data
	}))

	font2 := writeReadBack(t, font1)
	if diff := cmp.Diff(font1.Gdef, font2.Gdef); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// A GPOS table may refer to glyphs which the font does not have.  Reading
// drops the references, and the result can be written and read again.
func TestReadGposOutOfRange(t *testing.T) {
	font := debug.MakeSimpleFont()
	n := glyph.ID(font.NumGlyphs())
	gpos := &gtab.Info{
		ScriptList: gtab.ScriptListInfo{
			language.MustParse("und-Latn"): {Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: gtab.FeatureListInfo{
			{Tag: "kern", Lookups: []gtab.LookupIndex{0, 1}},
		},
		LookupList: gtab.LookupList{
			{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gpos1_1{
						Cov:    coverage.Table{n + 3: 0},
						Adjust: &gtab.GposValueRecord{XAdvance: 1},
					},
				},
			},
			{
				Meta: &gtab.LookupMetaInfo{LookupType: 2},
				Subtables: []gtab.Subtable{
					&gtab.Gpos2_1{
						Adjust: map[glyph.Pair]*gtab.PairAdjust{
							{Left: 1, Right: 2}:      {First: &gtab.GposValueRecord{XAdvance: 10}},
							{Left: 1, Right: n + 10}: {First: &gtab.GposValueRecord{XAdvance: 20}},
							{Left: n + 11, Right: 2}: {First: &gtab.GposValueRecord{XAdvance: 30}},
						},
					},
				},
			},
		},
	}
	data, err := gpos.Encode(gtab.TypeGpos)
	if err != nil {
		t.Fatal(err)
	}
	font1 := readBack(t, editTables(t, font, func(tables map[string][]byte) {
		tables["GPOS"] = data
	}))

	font2 := writeReadBack(t, font1)
	if diff := cmp.Diff(font1.Gpos, font2.Gpos); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}
