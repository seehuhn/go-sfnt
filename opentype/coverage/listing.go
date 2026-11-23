// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package coverage

import (
	"fmt"
	"iter"
	"maps"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// Listing is a coverage table as stored in a font file: the covered glyphs,
// each with the coverage index which the file gives to it.  The data which a
// font holds for the glyphs of a coverage table is stored in the order of
// these indices.
//
// A valid file numbers the glyphs 0, 1, ... in order of glyph ID, as a
// [Table] does.  In a damaged file the indices can have gaps, can repeat
// and can be out of order.  The function [Align] converts a listing and the
// data read with it into a [Table] and data which belong together.
//
// A listing is not modified after it has been read, so that one listing can
// be used for all references to the same coverage table.
type Listing struct {
	// table covers the listed glyphs, with the coverage indices 0, 1, ...
	// in order of glyph ID.
	table Table

	// glyphs and index are nil if the file gives every glyph the index
	// which the glyph has in table.  Otherwise glyphs holds the covered
	// glyphs in increasing order, and index holds the coverage index
	// which the file gives to each of them.
	glyphs []glyph.ID
	index  []int

	dataLen int
}

// ReadListing reads a coverage table, keeping the coverage indices of the
// file.  A glyph which the file lists more than once keeps the first of its
// indices.
func ReadListing(p *parser.Parser, pos int64) (*Listing, error) {
	// first coverage index of each glyph
	first := make(map[glyph.ID]int)

	// whether the file lists the glyphs in increasing order, with the
	// indices 0, 1, ...
	regular := true
	prev := -1

	err := readEntries(p, pos, tableEntrySize, func(gid glyph.ID, idx int) {
		if int(gid) <= prev || idx != len(first) {
			regular = false
		}
		prev = int(gid)
		if _, listed := first[gid]; !listed {
			first[gid] = idx
		}
	})
	if err != nil {
		return nil, err
	}
	if regular {
		return &Listing{table: first, dataLen: len(first)}, nil
	}

	// the table is built a second time, with the glyph list and the indices
	n := len(first)
	if err := p.Budget.ChargeN(n, tableEntrySize+2+8); err != nil {
		return nil, err
	}
	l := &Listing{
		table:  make(Table, n),
		glyphs: slices.Sorted(maps.Keys(first)),
		index:  make([]int, n),
	}
	moved := false
	for i, gid := range l.glyphs {
		idx := first[gid]
		l.table[gid] = i
		l.index[i] = idx
		l.dataLen = max(l.dataLen, idx+1)
		if idx != i {
			moved = true
		}
	}
	if !moved {
		// the indices of the file agree with the order of glyph IDs
		l.glyphs, l.index = nil, nil
	}
	return l, nil
}

// Table returns the coverage table for the glyphs of the listing.  This is
// the table to use where no data is stored for the covered glyphs.  All
// calls return the same table.
func (l *Listing) Table() Table {
	return l.table
}

// DataLen returns the number of data items which the listing refers to.
// This is one more than the largest coverage index, or zero if the listing
// covers no glyph.
func (l *Listing) DataLen() int {
	return l.dataLen
}

// All returns the covered glyphs, each with the coverage index which the
// file gives to it.  The order is not specified.
func (l *Listing) All() iter.Seq2[glyph.ID, int] {
	return func(yield func(glyph.ID, int) bool) {
		for gid, i := range l.table {
			if l.index != nil {
				i = l.index[i]
			}
			if !yield(gid, i) {
				return
			}
		}
	}
}

// Align returns the coverage table and the data for the glyphs of a listing
// which have data.  The argument data holds the data items in the order of
// the file, so that the item for a glyph is found at the coverage index
// which the listing gives to the glyph.  In the result, the item for a glyph
// is found at the coverage index which the returned table gives to the
// glyph.  Glyphs whose coverage index lies beyond the end of data are left
// out, and so are the items which belong to no glyph.
//
// If nothing needs to be moved, the table is the one which [Listing.Table]
// returns and the slice is the start of data.  Otherwise the results are new
// values, which have at most as many entries as the listing has glyphs.
// Neither the listing nor data is modified.
func Align[T any](l *Listing, data []T) (Table, []T) {
	if l.index == nil {
		n := len(l.table)
		if len(data) >= n {
			return l.table, data[:n]
		}
		// the glyphs with the largest glyph IDs have no data
		cov := make(Table, len(data))
		for gid, i := range l.table {
			if i < len(data) {
				cov[gid] = i
			}
		}
		return cov, data
	}

	numKept := 0
	for _, idx := range l.index {
		if idx < len(data) {
			numKept++
		}
	}
	cov := make(Table, numKept)
	// an empty result is empty in the same way as data
	res := data[:0:0]
	for i, gid := range l.glyphs {
		if idx := l.index[i]; idx < len(data) {
			cov[gid] = len(res)
			res = append(res, data[idx])
		}
	}
	return cov, res
}

// Budget charges for each glyph listed in a coverage table.
const (
	// covers the hashtable and value overhead of a map entry
	tableEntrySize = 24

	// approximates the amortised hashtable overhead for map[glyph.ID]bool,
	// where each entry carries only a few bytes
	setEntrySize = 16
)

// readEntries reads the coverage table at pos.  It calls add for each glyph
// which the table lists, in the order of the file, together with the
// coverage index which the table gives to the glyph.  The budget is charged
// entrySize bytes for each glyph listed.
//
// The glyphs can come in any order, and a glyph can be listed more than
// once.
func readEntries(p *parser.Parser, pos int64, entrySize int, add func(gid glyph.ID, idx int)) error {
	err := p.SeekPos(pos)
	if err != nil {
		return err
	}

	format, err := p.ReadUint16()
	if err != nil {
		return err
	}

	switch format {
	case 1: // Coverage Format 1
		glyphCount, err := p.ReadUint16()
		if err != nil {
			return err
		}
		if err := p.Budget.ChargeN(int(glyphCount), entrySize); err != nil {
			return err
		}
		for i := range int(glyphCount) {
			gid, err := p.ReadUint16()
			if err != nil {
				return err
			}
			add(glyph.ID(gid), i)
		}

	case 2: // Coverage Format 2
		rangeCount, err := p.ReadUint16()
		if err != nil {
			return err
		}
		for range int(rangeCount) {
			buf, err := p.ReadBytes(6)
			if err != nil {
				return err
			}
			startGlyphID := int(buf[0])<<8 | int(buf[1])
			endGlyphID := int(buf[2])<<8 | int(buf[3])
			startCoverageIndex := int(buf[4])<<8 | int(buf[5])
			if endGlyphID < startGlyphID {
				// the range lists no glyph
				continue
			}
			// Each range is charged up front.  This also bounds the work
			// for ranges which overlap, where many of the glyphs listed
			// are not new.
			if err := p.Budget.ChargeN(endGlyphID-startGlyphID+1, entrySize); err != nil {
				return err
			}
			for gid := startGlyphID; gid <= endGlyphID; gid++ {
				add(glyph.ID(gid), startCoverageIndex+gid-startGlyphID)
			}
		}

	default:
		return &parser.NotSupportedError{
			SubSystem: "sfnt/opentype/coverage",
			Feature:   fmt.Sprintf("coverage format %d", format),
		}
	}

	return nil
}
