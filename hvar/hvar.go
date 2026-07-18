// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package hvar reads and writes the "HVAR" table, which stores horizontal
// metrics variation data (advance width, left side bearing, right side
// bearing) for a variable font.
// https://learn.microsoft.com/en-us/typography/opentype/spec/hvar
//
// LsbMap and RsbMap decode and re-encode for round-trip fidelity only: this
// library regenerates "hmtx" left side bearings from glyph bounding boxes on
// write, so neither map is ever consulted when producing output.
package hvar

import (
	"errors"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// headerSize is the fixed size of the HVAR header.
const headerSize = 20

var (
	errUnsupportedVersion = errors.New("hvar: unsupported table version")
	errMissingStore       = errors.New("hvar: item variation store offset is zero")
)

// Table represents the contents of an "HVAR" table.
type Table struct {
	// Store holds the shared delta values for advance widths and, if
	// present, side bearings.
	Store *variation.ItemVariationStore

	// AdvanceMap maps a glyph ID to a delta-set index for advance-width
	// deltas.  A nil AdvanceMap means the implicit mapping applies: outer
	// index 0, inner index equal to the glyph ID.
	AdvanceMap *variation.DeltaSetIndexMap

	// LsbMap maps a glyph ID to a delta-set index for left-side-bearing
	// deltas.  A nil LsbMap means no left-side-bearing variation data is
	// present.
	LsbMap *variation.DeltaSetIndexMap

	// RsbMap maps a glyph ID to a delta-set index for right-side-bearing
	// deltas.  A nil RsbMap means no right-side-bearing variation data is
	// present.
	RsbMap *variation.DeltaSetIndexMap
}

// Read reads and decodes an "HVAR" table.  Allocations are charged against
// budget.  The item variation store is mandatory; a table whose store offset
// is zero is malformed and yields an error.
func Read(r parser.ReadSeekSizer, budget *membudget.Budget) (*Table, error) {
	p := parser.New(r, budget)

	buf, err := p.ReadBytes(headerSize)
	if err != nil {
		return nil, err
	}
	majorVersion := u16(buf, 0)
	storeOffset := u32(buf, 4)
	advanceOffset := u32(buf, 8)
	lsbOffset := u32(buf, 12)
	rsbOffset := u32(buf, 16)

	if majorVersion != 1 {
		return nil, errUnsupportedVersion
	}
	if storeOffset == 0 {
		return nil, errMissingStore
	}

	t := &Table{}
	t.Store, err = variation.ReadItemVariationStore(p, int64(storeOffset))
	if err != nil {
		return nil, err
	}
	if advanceOffset != 0 {
		t.AdvanceMap, err = variation.ReadDeltaSetIndexMap(p, int64(advanceOffset))
		if err != nil {
			return nil, err
		}
	}
	if lsbOffset != 0 {
		t.LsbMap, err = variation.ReadDeltaSetIndexMap(p, int64(lsbOffset))
		if err != nil {
			return nil, err
		}
	}
	if rsbOffset != 0 {
		t.RsbMap, err = variation.ReadDeltaSetIndexMap(p, int64(rsbOffset))
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

// Encode returns the binary form of the HVAR table.  The item variation
// store is always written, even when Store is nil (as an empty store), since
// a zero store offset is not permitted.  The output is deterministic.
func (t *Table) Encode() []byte {
	store := t.Store
	if store == nil {
		store = &variation.ItemVariationStore{}
	}
	storeBytes := store.Encode()

	pos := headerSize
	storeOffset := pos
	pos += len(storeBytes)

	var advanceBytes, lsbBytes, rsbBytes []byte
	var advanceOffset, lsbOffset, rsbOffset int
	if t.AdvanceMap != nil {
		advanceBytes = t.AdvanceMap.Encode()
		advanceOffset = pos
		pos += len(advanceBytes)
	}
	if t.LsbMap != nil {
		lsbBytes = t.LsbMap.Encode()
		lsbOffset = pos
		pos += len(lsbBytes)
	}
	if t.RsbMap != nil {
		rsbBytes = t.RsbMap.Encode()
		rsbOffset = pos
		pos += len(rsbBytes)
	}

	buf := make([]byte, 0, pos)
	buf = appendU16(buf, 1) // majorVersion
	buf = appendU16(buf, 0) // minorVersion
	buf = appendU32(buf, uint32(storeOffset))
	buf = appendU32(buf, uint32(advanceOffset))
	buf = appendU32(buf, uint32(lsbOffset))
	buf = appendU32(buf, uint32(rsbOffset))
	buf = append(buf, storeBytes...)
	buf = append(buf, advanceBytes...)
	buf = append(buf, lsbBytes...)
	buf = append(buf, rsbBytes...)
	return buf
}

// AdvanceDelta returns the unrounded advance-width delta for glyph gid at
// the normalized axis coordinates coords.  Callers apply any required
// rounding.  A nil Store yields 0.
func (t *Table) AdvanceDelta(gid glyph.ID, coords []variation.F2Dot14) float64 {
	var outer, inner uint16
	if t.AdvanceMap != nil {
		outer, inner = t.AdvanceMap.Lookup(uint32(gid))
	} else {
		inner = uint16(gid)
	}
	if t.Store == nil {
		return 0
	}
	return t.Store.Evaluate(outer, inner, coords)
}

func u16(b []byte, i int) uint16 {
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func u32(b []byte, i int) uint32 {
	return uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
}

func appendU16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

func appendU32(buf []byte, v uint32) []byte {
	return append(buf, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
