// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package coverage reads and writes OpenType "Coverage Tables".
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#coverage-table
package coverage

import (
	"maps"
	"math/bits"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/internal/mapfilter"
	"seehuhn.de/go/sfnt/parser"
)

// Table represents an OpenType "Coverage Table".
// A Coverage table defines a unique index value, the Coverage Index, for each covered glyph.
// The Coverage Indexes are sequential, from 0 to the number of covered glyphs minus 1.
// The map from glyph ID to Coverage Index must be strictly monotonic.
type Table map[glyph.ID]int

// Contains returns true if the given glyph ID is covered by the table.
func (table Table) Contains(gid glyph.ID) bool {
	_, ok := table[gid]
	return ok
}

// Glyphs returns the glyphs covered by the table in increasing order.
func (table Table) Glyphs() []glyph.ID {
	keys := slices.Sorted(maps.Keys(table))
	return keys
}

// Valid reports whether the coverage indices of the table are exactly
// 0, 1, ..., len(table)-1, assigned in order of increasing glyph ID.
func (table Table) Valid() bool {
	// The covered glyphs are visited in increasing order via a bit set,
	// which lives on the stack, so that the check allocates nothing.
	var covered [1 << 16 / 64]uint64
	for gid := range table {
		covered[gid/64] |= 1 << (gid % 64)
	}
	rank := 0
	for i, word := range covered {
		for word != 0 {
			gid := glyph.ID(i*64 + bits.TrailingZeros64(word))
			if table[gid] != rank {
				return false
			}
			rank++
			word &= word - 1
		}
	}
	return true
}

// Limit returns the table without the glyphs numGlyphs and above.  The
// second result reports whether any glyph was removed.  If not, the table
// itself is returned, otherwise the result is a copy.  The remaining glyphs
// keep their coverage indices.  If the table is valid, so is the result.
func (table Table) Limit(numGlyphs int) (Table, bool) {
	// In a valid table, the glyphs removed have the largest coverage indices.
	return mapfilter.Keys(table, func(gid glyph.ID) bool { return int(gid) < numGlyphs })
}

// Filter returns the part of a coverage table, and of the data indexed by
// its coverage indices, for which keep returns true.  The function keep is
// called once for each covered glyph, with the glyph and its data item.
// The table cov
// must be valid, as reported by [Table.Valid], and the slice data must have
// one item for each covered glyph.
//
// The glyphs which are left get the coverage indices 0, 1, ... in order of
// glyph ID, and the data items are moved to match.  The last result reports
// whether any glyph was removed.  If not, cov and data themselves are
// returned.  Otherwise the results are new values, and the arguments are not
// modified.
func Filter[T any](cov Table, data []T, keep func(gid glyph.ID, item T) bool) (Table, []T, bool) {
	// indexed by coverage index, allocated when the first glyph is dropped
	var drop []bool
	numDropped := 0
	for gid, idx := range cov {
		if !keep(gid, data[idx]) {
			if drop == nil {
				drop = make([]bool, len(cov))
			}
			drop[idx] = true
			numDropped++
		}
	}
	if drop == nil {
		return cov, data, false
	}

	glyphs := make([]glyph.ID, len(cov))
	for gid, idx := range cov {
		glyphs[idx] = gid
	}
	n := len(cov) - numDropped
	newCov := make(Table, n)
	newData := make([]T, 0, n)
	for idx, gid := range glyphs {
		if !drop[idx] {
			newCov[gid] = len(newData)
			newData = append(newData, data[idx])
		}
	}
	return newCov, newData, true
}

// ToSet returns the set of glyphs covered by the table.
func (table Table) ToSet() Set {
	set := make(Set, len(table))
	for gid := range table {
		set[gid] = true
	}
	return set
}

// Read reads a coverage table for which the font holds no data per covered
// glyph.  The glyphs get the coverage indices 0, 1, ... in order of glyph
// ID.  For a coverage table with data, use [ReadListing] and [Align]
// instead.
func Read(p *parser.Parser, pos int64) (Table, error) {
	l, err := ReadListing(p, pos)
	if err != nil {
		return nil, err
	}
	return l.Table(), nil
}

func (table Table) encInfo() ([]glyph.ID, int, int) {
	if !table.Valid() {
		panic("invalid coverage table")
	}
	rev := make([]glyph.ID, len(table))
	for gid, i := range table {
		rev[i] = gid
	}

	format1Length := 4 + 2*len(table)

	rangeCount := 0
	prev := 0xFFFF
	for _, gid := range rev {
		if int(gid) != prev+1 {
			rangeCount++
		}
		prev = int(gid)
	}
	format2Length := 4 + 6*rangeCount

	return rev, format1Length, format2Length
}

// EncodeLen returns the number of bytes in the binary representation of the
// coverage table.
func (table Table) EncodeLen() int {
	_, format1Length, format2Length := table.encInfo()
	if format1Length <= format2Length {
		return format1Length
	}
	return format2Length
}

// Encode returns the binary representation of the coverage table.
func (table Table) Encode() []byte {
	rev, format1Length, format2Length := table.encInfo()

	if format1Length <= format2Length {
		buf := make([]byte, format1Length)
		buf[0] = 0
		buf[1] = 1
		buf[2] = byte(len(rev) >> 8)
		buf[3] = byte(len(rev))
		for i, gid := range rev {
			buf[4+2*i] = byte(gid >> 8)
			buf[4+2*i+1] = byte(gid)
		}
		return buf
	}

	rangeCount := (format2Length - 4) / 6

	buf := make([]byte, 4, format2Length)
	buf[0] = 0
	buf[1] = 2
	buf[2] = byte(rangeCount >> 8)
	buf[3] = byte(rangeCount)
	var startGlyphID glyph.ID
	var startCoverageIndex int
	prev := 0xFFFF
	for i, gid := range rev {
		if int(gid) != prev+1 {
			if i > 0 {
				buf = append(buf,
					byte(startGlyphID>>8), byte(startGlyphID),
					byte(prev>>8), byte(prev),
					byte(startCoverageIndex>>8), byte(startCoverageIndex))
			}
			startGlyphID = gid
			startCoverageIndex = i
		}
		prev = int(gid)
	}
	buf = append(buf,
		byte(startGlyphID>>8), byte(startGlyphID),
		byte(prev>>8), byte(prev),
		byte(startCoverageIndex>>8), byte(startCoverageIndex))
	return buf
}
