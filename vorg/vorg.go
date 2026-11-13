// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package vorg reads and writes "VORG" tables.
// https://learn.microsoft.com/en-us/typography/opentype/spec/vorg
//
// The table gives the y coordinate of the vertical origin of glyphs in fonts
// with CFF outlines.  Most glyphs share one default value; the table lists
// the exceptions.
package vorg

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
)

// Table contains the information from a "VORG" table.
type Table struct {
	// Default is the y coordinate of the vertical origin for all glyphs
	// not listed in Origins.
	Default funit.Int16

	// Origins maps glyph IDs to their vertical origin.  Entries which
	// are equal to Default are redundant and are not written to the table.
	Origins map[glyph.ID]funit.Int16
}

const (
	headerLength = 8
	recordLength = 4
)

// Decode extracts the vertical origins from a "VORG" table.
// If the table lists a glyph more than once, the first entry is used.
// Entries equal to the default are dropped.
func Decode(data []byte) (*Table, error) {
	if len(data) < headerLength {
		return nil, errors.New("VORG table too short")
	}
	major := binary.BigEndian.Uint16(data)
	if major != 1 {
		return nil, fmt.Errorf("unsupported VORG version %d", major)
	}
	t := &Table{
		Default: funit.Int16(binary.BigEndian.Uint16(data[4:])),
	}
	n := int(binary.BigEndian.Uint16(data[6:]))
	records := data[headerLength:]
	if len(records) < n*recordLength {
		return nil, errors.New("VORG table truncated")
	}

	seen := make(map[glyph.ID]bool)
	for i := range n {
		rec := records[i*recordLength:]
		gid := glyph.ID(binary.BigEndian.Uint16(rec))
		y := funit.Int16(binary.BigEndian.Uint16(rec[2:]))
		if seen[gid] {
			continue
		}
		seen[gid] = true
		if y == t.Default {
			continue
		}
		if t.Origins == nil {
			t.Origins = make(map[glyph.ID]funit.Int16)
		}
		t.Origins[gid] = y
	}
	return t, nil
}

// Encode returns the binary form of the table.  The records are sorted by
// glyph ID, and entries equal to Default are omitted.
func (t *Table) Encode() []byte {
	gids := make([]glyph.ID, 0, len(t.Origins))
	for gid, y := range t.Origins {
		if y != t.Default {
			gids = append(gids, gid)
		}
	}
	slices.Sort(gids)

	data := make([]byte, headerLength+recordLength*len(gids))
	binary.BigEndian.PutUint16(data, 1) // majorVersion
	binary.BigEndian.PutUint16(data[4:], uint16(t.Default))
	binary.BigEndian.PutUint16(data[6:], uint16(len(gids)))
	for i, gid := range gids {
		rec := data[headerLength+i*recordLength:]
		binary.BigEndian.PutUint16(rec, uint16(gid))
		binary.BigEndian.PutUint16(rec[2:], uint16(t.Origins[gid]))
	}
	return data
}

// Get returns the vertical origin of a glyph.
func (t *Table) Get(gid glyph.ID) funit.Int16 {
	if y, ok := t.Origins[gid]; ok {
		return y
	}
	return t.Default
}

// FromOrigins builds a table from a list of vertical origins, one per glyph.
// The most frequent value becomes the default.  If several values are equally
// frequent, the smallest one is used.
func FromOrigins(origins []funit.Int16) *Table {
	count := make(map[funit.Int16]int)
	for _, y := range origins {
		count[y]++
	}
	t := &Table{}
	best := 0
	for y, c := range count {
		if c > best || c == best && y < t.Default {
			best = c
			t.Default = y
		}
	}

	for i, y := range origins {
		if y == t.Default {
			continue
		}
		if t.Origins == nil {
			t.Origins = make(map[glyph.ID]funit.Int16)
		}
		t.Origins[glyph.ID(i)] = y
	}
	return t
}
