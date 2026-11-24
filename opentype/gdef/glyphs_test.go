// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gdef

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/parser"
)

func glyphsTestTable() *Table {
	cp := uint16(3)
	return &Table{
		GlyphClass:      classdef.Table{1: GlyphClassBase, 2: GlyphClassMark, 50: GlyphClassMark},
		MarkAttachClass: classdef.Table{2: 1, 60: 2},
		AttachList: &AttachList{
			Cov:    coverage.Table{1: 0, 40: 1, 70: 2},
			Points: [][]uint16{{1}, {2}, {3}},
		},
		LigCaretList: &LigCaretList{
			Cov:    coverage.Table{5: 0, 6: 1, 80: 2},
			Carets: [][]CaretValue{{{Coordinate: 10}}, {{ContourPoint: &cp}}, {{Coordinate: 30}}},
		},
		MarkGlyphSets: []coverage.Set{{1: true, 90: true}, {2: true}, {99: true}},
	}
}

func TestLimitGlyphs(t *testing.T) {
	table := glyphsTestTable()
	if err := table.CheckGlyphs(10); err == nil {
		t.Fatal("out-of-range glyphs not detected")
	}
	if err := table.CheckGlyphs(100); err != nil {
		t.Fatal(err)
	}
	if table.LimitGlyphs(100) != table {
		t.Error("table without out-of-range glyphs was copied")
	}

	limited := table.LimitGlyphs(10)
	if err := limited.CheckGlyphs(10); err != nil {
		t.Error(err)
	}
	if limited.LimitGlyphs(10) != limited {
		t.Error("limiting is not stable")
	}
	if len(table.GlyphClass) != 3 || len(table.AttachList.Cov) != 3 {
		t.Error("receiver was modified")
	}

	want := glyphsTestTable()
	want.GlyphClass = classdef.Table{1: GlyphClassBase, 2: GlyphClassMark}
	want.MarkAttachClass = classdef.Table{2: 1}
	want.AttachList = &AttachList{Cov: coverage.Table{1: 0}, Points: [][]uint16{{1}}}
	want.LigCaretList.Cov = coverage.Table{5: 0, 6: 1}
	want.LigCaretList.Carets = want.LigCaretList.Carets[:2]
	want.MarkGlyphSets = []coverage.Set{{1: true}, {2: true}, {}}
	if diff := cmp.Diff(want, limited); diff != "" {
		t.Errorf("wrong result (-want +got):\n%s", diff)
	}

	if (*Table)(nil).LimitGlyphs(3) != nil {
		t.Error("nil table changed")
	}
}

func TestLimitGlyphsRenumbers(t *testing.T) {
	table := &Table{AttachList: &AttachList{
		Cov:    coverage.Table{1: 0, 50: 1, 60: 2},
		Points: [][]uint16{{1}, {2}, {3}},
	}}
	got := table.LimitGlyphs(55).AttachList
	want := &AttachList{Cov: coverage.Table{1: 0, 50: 1}, Points: [][]uint16{{1}, {2}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong result (-want +got):\n%s", diff)
	}

	table = &Table{AttachList: &AttachList{Cov: coverage.Table{70: 0}, Points: [][]uint16{{1}}}}
	if table.LimitGlyphs(10).AttachList != nil {
		t.Error("empty list not removed")
	}
}

func TestCheckGlyphsFields(t *testing.T) {
	cp := uint16(1)
	tests := map[string]*Table{
		"GlyphClass":      {GlyphClass: classdef.Table{glyph.ID(77): 1}},
		"MarkAttachClass": {MarkAttachClass: classdef.Table{glyph.ID(77): 1}},
		"AttachList": {AttachList: &AttachList{
			Cov: coverage.Table{77: 0}, Points: [][]uint16{{1}}}},
		"LigCaretList": {LigCaretList: &LigCaretList{
			Cov: coverage.Table{77: 0}, Carets: [][]CaretValue{{{ContourPoint: &cp}}}}},
		"MarkGlyphSets": {MarkGlyphSets: []coverage.Set{{77: true}}},
	}
	for name, table := range tests {
		t.Run(name, func(t *testing.T) {
			if err := table.CheckGlyphs(4); err == nil {
				t.Error("out-of-range glyph not detected")
			}
			if err := table.CheckGlyphs(100); err != nil {
				t.Error(err)
			}
			if err := table.LimitGlyphs(4).CheckGlyphs(4); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestEncodeInvalidCoverage(t *testing.T) {
	cp := uint16(1)
	tests := map[string]*Table{
		"attach": {AttachList: &AttachList{
			Cov: coverage.Table{1: 0, 2: 5}, Points: [][]uint16{{1}, {2}}}},
		"attach order": {AttachList: &AttachList{
			Cov: coverage.Table{1: 1, 2: 0}, Points: [][]uint16{{1}, {2}}}},
		"attach length": {AttachList: &AttachList{
			Cov: coverage.Table{1: 0, 2: 1}, Points: [][]uint16{{1}}}},
		"caret": {LigCaretList: &LigCaretList{
			Cov: coverage.Table{1: 0, 2: 5}, Carets: [][]CaretValue{{{ContourPoint: &cp}}, nil}}},
		"caret length": {LigCaretList: &LigCaretList{
			Cov: coverage.Table{1: 0, 2: 1}, Carets: [][]CaretValue{nil}}},
	}
	for name, table := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := table.Encode(); err == nil {
				t.Error("invalid table encoded")
			}
		})
	}
}

func TestLimitGlyphsRoundTrip(t *testing.T) {
	data := mustEncode(t, glyphsTestTable())
	table, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	limited := table.LimitGlyphs(10)
	data2 := mustEncode(t, limited)
	table2, err := Read(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(limited, table2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}
