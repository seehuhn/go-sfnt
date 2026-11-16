// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gdef

import (
	"testing"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
)

// tooMany is one more than a 16-bit count can hold.
const tooMany = 0x10000

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

func TestClassDefTooLarge(t *testing.T) {
	t.Run("glyph classes", func(t *testing.T) {
		table := &Table{GlyphClass: alternatingClasses()}
		if _, err := table.Encode(); err == nil {
			t.Error("encode succeeded")
		}
	})
	t.Run("mark attachment classes", func(t *testing.T) {
		table := &Table{MarkAttachClass: alternatingClasses()}
		if _, err := table.Encode(); err == nil {
			t.Error("encode succeeded")
		}
	})
}

func TestAttachListTooManyPoints(t *testing.T) {
	al := &AttachList{
		Cov:    coverage.Table{1: 0},
		Points: [][]uint16{make([]uint16, tooMany)},
	}
	if _, err := al.layout(); err == nil {
		t.Error("layout succeeded")
	}
}
