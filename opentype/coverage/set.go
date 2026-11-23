// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package coverage

import (
	"maps"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// Set is a coverage table, but with the coverage indices omitted.
type Set map[glyph.ID]bool

// Glyphs returned the glyphs covered by the Set, in order of increasing glyph ID.
func (set Set) Glyphs() []glyph.ID {
	glyphs := slices.Sorted(maps.Keys(set))
	return glyphs
}

// ToTable converts the Set to a Coverage table.
func (set Set) ToTable() Table {
	glyphs := set.Glyphs()
	table := make(Table, len(glyphs))
	for i, gid := range glyphs {
		table[gid] = i
	}
	return table
}

// ReadSet reads a coverage table as the set of glyphs which the table
// lists.
func ReadSet(p *parser.Parser, pos int64) (Set, error) {
	set := make(Set)
	err := readEntries(p, pos, setEntrySize, func(gid glyph.ID, _ int) {
		set[gid] = true
	})
	if err != nil {
		return nil, err
	}
	return set, nil
}
