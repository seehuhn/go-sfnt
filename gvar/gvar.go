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

// Package gvar reads and writes the "gvar" table, which stores the glyph
// outline variation data of a variable font.
// https://learn.microsoft.com/en-us/typography/opentype/spec/gvar
//
// Decode keeps each glyph's variation data as an opaque byte block and
// parses it only on demand via [Table.Unpack].  Delta application (IUP,
// phantom points) is not part of this package.
package gvar

import (
	"errors"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/variation"
)

// headerSize is the fixed size of the gvar header, up to and excluding the
// glyph offset array.
const headerSize = 20

// longOffsetsFlag marks the long (32-bit) glyph offset format.
const longOffsetsFlag = 0x0001

var (
	errShortTable         = errors.New("gvar: table too short")
	errUnsupportedVersion = errors.New("gvar: unsupported table version")
	errBadAxisCount       = errors.New("gvar: invalid axis count")
	errBadOffsets         = errors.New("gvar: invalid glyph offsets")
	errTooManyGlyphs      = errors.New("gvar: too many glyphs")
	errTooManyShared      = errors.New("gvar: too many shared tuples")
	errTooLarge           = errors.New("gvar: table too large")
	errGIDOutOfRange      = errors.New("gvar: glyph index out of range")
)

// Table represents the contents of a "gvar" table.
type Table struct {
	// AxisCount is the number of variation axes.  It must match the font's
	// "fvar" axis count.
	AxisCount int

	// SharedTuples holds the shared peak tuples, each a vector of AxisCount
	// normalized coordinates.  Individual glyph variations may reference
	// these by index instead of embedding their own peak.
	SharedTuples [][]variation.F2Dot14

	// PerGlyph holds one raw variation-data block per glyph, in glyph order.
	// Its length is the glyph count recorded in the table, which need not
	// equal the font's real glyph count.
	PerGlyph []GlyphData
}

// GlyphData is one glyph's raw glyphVariationData block.  The block is
// retained verbatim and parsed on demand by [Table.Unpack].  A nil Data
// means the glyph has no variation data.
type GlyphData struct {
	Data []byte
}

// Decode parses a "gvar" table.  Allocations are charged against budget.
//
// The retained per-glyph blocks alias the input slice; the caller must not
// modify data while the returned table is in use.  Decode does not parse the
// per-glyph tuple data; use [Table.Unpack] for that.
//
// The glyph count recorded in the table is not validated against the font's
// real glyph count; Decode records whatever the table states.
func Decode(data []byte, budget *membudget.Budget) (*Table, error) {
	if err := budget.Charge(headerSize); err != nil {
		return nil, err
	}
	if len(data) < headerSize {
		return nil, errShortTable
	}

	majorVersion := u16(data, 0)
	axisCount := int(u16(data, 4))
	sharedTupleCount := int(u16(data, 6))
	sharedTuplesOffset := int(u32(data, 8))
	glyphCount := int(u16(data, 12))
	flags := u16(data, 14)
	dataArrayOffset := int(u32(data, 16))

	if majorVersion != 1 {
		return nil, errUnsupportedVersion
	}
	if axisCount > variation.MaxAxisCount {
		return nil, errBadAxisCount
	}

	longOffsets := flags&longOffsetsFlag != 0
	offsetSize := 2
	if longOffsets {
		offsetSize = 4
	}

	// the glyph offset array follows the header; bound it against the input
	// size before allocating anything
	nOffsets := glyphCount + 1
	if headerSize+nOffsets*offsetSize > len(data) {
		return nil, errShortTable
	}

	offsets, err := membudget.AllocSlice[int](budget, nOffsets)
	if err != nil {
		return nil, err
	}
	prev := 0
	for i := range offsets {
		var off int
		if longOffsets {
			off = int(u32(data, headerSize+i*4))
		} else {
			off = int(u16(data, headerSize+i*2)) * 2
		}
		if off < prev {
			return nil, errBadOffsets
		}
		offsets[i] = off
		prev = off
	}

	if dataArrayOffset > len(data) || dataArrayOffset+offsets[glyphCount] > len(data) {
		return nil, errShortTable
	}

	t := &Table{AxisCount: axisCount}

	if sharedTupleCount > 0 {
		need := sharedTupleCount * axisCount * 2
		if sharedTuplesOffset < 0 || sharedTuplesOffset+need > len(data) {
			return nil, errShortTable
		}
		t.SharedTuples, err = membudget.AllocSlice[[]variation.F2Dot14](budget, sharedTupleCount)
		if err != nil {
			return nil, err
		}
		p := sharedTuplesOffset
		for i := range t.SharedTuples {
			if axisCount == 0 {
				continue
			}
			tup, err := membudget.AllocSlice[variation.F2Dot14](budget, axisCount)
			if err != nil {
				return nil, err
			}
			for j := range tup {
				tup[j] = variation.F2Dot14(int16(u16(data, p)))
				p += 2
			}
			t.SharedTuples[i] = tup
		}
	}

	if glyphCount > 0 {
		t.PerGlyph, err = membudget.AllocSlice[GlyphData](budget, glyphCount)
		if err != nil {
			return nil, err
		}
		for i := range t.PerGlyph {
			start := offsets[i]
			end := offsets[i+1]
			if end > start {
				t.PerGlyph[i].Data = data[dataArrayOffset+start : dataArrayOffset+end]
			}
		}
	}

	return t, nil
}

// Encode returns the binary form of the gvar table.  Retained per-glyph
// blocks are re-emitted unchanged; only the header, offset array and offset
// format are recomputed.
//
// The output is deterministic.  Glyph blocks are laid out contiguously and
// the short (16-bit) offset format is used whenever every offset is even and
// the data fits; otherwise the long (32-bit) format is used.  A Decode /
// Encode / Decode cycle preserves the raw blocks byte for byte.
func (t *Table) Encode() ([]byte, error) {
	glyphCount := len(t.PerGlyph)
	if glyphCount > 0xFFFF {
		return nil, errTooManyGlyphs
	}
	if t.AxisCount < 0 || t.AxisCount > variation.MaxAxisCount {
		return nil, errBadAxisCount
	}
	if len(t.SharedTuples) > 0xFFFF {
		return nil, errTooManyShared
	}

	// cumulative offsets with exact block lengths; no padding is inserted so
	// that each block's range is exactly its data and re-decode is faithful
	nOffsets := glyphCount + 1
	offsets := make([]int, nOffsets)
	pos := 0
	for i := range t.PerGlyph {
		offsets[i] = pos
		pos += len(t.PerGlyph[i].Data)
	}
	offsets[glyphCount] = pos
	dataLen := pos

	// short offsets need every offset even (stored halved) and in range
	allEven := true
	for _, off := range offsets {
		if off%2 != 0 {
			allEven = false
			break
		}
	}
	short := allEven && dataLen <= 0x1FFFE

	offsetSize := 4
	flags := uint16(longOffsetsFlag)
	if short {
		offsetSize = 2
		flags = 0
	}

	offsetArrayLen := nOffsets * offsetSize
	sharedLen := len(t.SharedTuples) * t.AxisCount * 2
	sharedTuplesOffset := headerSize + offsetArrayLen
	dataArrayOffset := sharedTuplesOffset + sharedLen

	total := dataArrayOffset + dataLen
	if int64(total) > 0xFFFFFFFF {
		return nil, errTooLarge
	}

	out := make([]byte, 0, total)
	out = appendU16(out, 1) // majorVersion
	out = appendU16(out, 0) // minorVersion
	out = appendU16(out, uint16(t.AxisCount))
	out = appendU16(out, uint16(len(t.SharedTuples)))
	out = appendU32(out, uint32(sharedTuplesOffset))
	out = appendU16(out, uint16(glyphCount))
	out = appendU16(out, flags)
	out = appendU32(out, uint32(dataArrayOffset))

	for _, off := range offsets {
		if short {
			out = appendU16(out, uint16(off/2))
		} else {
			out = appendU32(out, uint32(off))
		}
	}

	for _, tup := range t.SharedTuples {
		for j := range t.AxisCount {
			var v variation.F2Dot14
			if j < len(tup) {
				v = tup[j]
			}
			out = appendU16(out, uint16(v))
		}
	}

	for i := range t.PerGlyph {
		out = append(out, t.PerGlyph[i].Data...)
	}

	return out, nil
}

// Unpack parses one glyph's tuple variations.  nPoints is the number of
// deltable points and must include the four phantom points.  A glyph with no
// variation data yields a nil slice and no error; an out-of-range gid is an
// error.  Allocations are charged against budget.
func (t *Table) Unpack(gid glyph.ID, nPoints int, budget *membudget.Budget) ([]variation.TupleVariation, error) {
	if int(gid) >= len(t.PerGlyph) {
		return nil, errGIDOutOfRange
	}
	data := t.PerGlyph[gid].Data
	if data == nil {
		return nil, nil
	}
	return variation.DecodeTupleData(data, t.AxisCount, 2, nPoints, true, budget)
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
