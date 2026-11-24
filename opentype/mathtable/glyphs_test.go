// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
)

func TestCheckGlyphs(t *testing.T) {
	const bad = glyph.ID(77)
	good := []Variant{{Glyph: 1, Advance: 10}}
	tests := map[string]*Info{
		"ItalicsCorrection": {ItalicsCorrection: map[glyph.ID]Value{bad: {V: 5}}},
		"TopAccent":         {TopAccent: map[glyph.ID]Value{bad: {V: 5}}},
		"ExtendedShapes":    {ExtendedShapes: coverage.Set{bad: true}},
		"Kerns": {Kerns: map[glyph.ID]*KernInfo{
			bad: {TopRight: &Kern{Kerns: []Value{{V: 1}}}}}},
		"Vertical key": {Vertical: map[glyph.ID]*Construction{
			bad: {Variants: good}}},
		"Horizontal key": {Horizontal: map[glyph.ID]*Construction{
			bad: {Variants: good}}},
		"Vertical variant": {Vertical: map[glyph.ID]*Construction{
			1: {Variants: []Variant{{Glyph: 1}, {Glyph: bad}}}}},
		"Horizontal variant": {Horizontal: map[glyph.ID]*Construction{
			1: {Variants: []Variant{{Glyph: bad}}}}},
		"Vertical part": {Vertical: map[glyph.ID]*Construction{
			1: {Assembly: &Assembly{Parts: []Part{{Glyph: 2}, {Glyph: bad}}}}}},
		"Horizontal part": {Horizontal: map[glyph.ID]*Construction{
			1: {Variants: good, Assembly: &Assembly{Parts: []Part{{Glyph: bad}}}}}},
	}
	for name, info := range tests {
		t.Run(name, func(t *testing.T) {
			if err := info.CheckGlyphs(4); err == nil {
				t.Error("out-of-range glyph not detected")
			}
			if err := info.CheckGlyphs(100); err != nil {
				t.Error(err)
			}
		})
	}

	if err := (&Info{}).CheckGlyphs(0); err != nil {
		t.Error(err)
	}
}

func TestLimitGlyphs(t *testing.T) {
	info := &Info{
		TopAccent: map[glyph.ID]Value{1: {V: 5}, 7: {V: 6}},
		Vertical: map[glyph.ID]*Construction{
			2: {Variants: []Variant{{Glyph: 2, Advance: 10}, {Glyph: 8, Advance: 20}}},
		},
	}

	if got := info.LimitGlyphs(9); got != info {
		t.Error("table without out-of-range glyphs was copied")
	}

	got := info.LimitGlyphs(5)
	if err := got.CheckGlyphs(5); err != nil {
		t.Error(err)
	}
	want := &Info{
		TopAccent: map[glyph.ID]Value{1: {V: 5}},
		Vertical: map[glyph.ID]*Construction{
			2: {Variants: []Variant{{Glyph: 2, Advance: 10}}},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong result (-want +got):\n%s", diff)
	}
	if err := info.CheckGlyphs(5); err == nil {
		t.Error("receiver modified")
	}

	if (*Info)(nil).LimitGlyphs(5) != nil {
		t.Error("nil receiver gave non-nil result")
	}
}
