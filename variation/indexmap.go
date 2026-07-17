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

package variation

import (
	"errors"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
)

// DeltaSetIndexMap maps a value index to a delta-set (outer, inner)
// coordinate in an [ItemVariationStore].  Each entry packs the pair as
// outer<<16 | inner.  An absent map is represented by a nil pointer at
// the call site.
type DeltaSetIndexMap struct {
	Map []uint32
}

// ReadDeltaSetIndexMap reads a delta-set index map (format 0 or 1)
// located at the absolute file position pos.  Allocations are charged
// against the parser's budget.
func ReadDeltaSetIndexMap(p *parser.Parser, pos int64) (*DeltaSetIndexMap, error) {
	if err := p.SeekPos(pos); err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(2)
	if err != nil {
		return nil, err
	}
	format := buf[0]
	entryFormat := buf[1]

	var mapCount int
	switch format {
	case 0:
		n, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		mapCount = int(n)
	case 1:
		n, err := p.ReadUint32()
		if err != nil {
			return nil, err
		}
		if int64(n) > p.Size() {
			return nil, errors.New("variation: map count exceeds input size")
		}
		mapCount = int(n)
	default:
		return nil, errors.New("variation: unsupported delta set index map format")
	}

	innerBits := int(entryFormat&0x0F) + 1
	entrySize := int((entryFormat>>4)&0x03) + 1

	if err := checkCount(p, mapCount, entrySize); err != nil {
		return nil, err
	}
	m := &DeltaSetIndexMap{}
	if mapCount == 0 {
		return m, nil
	}
	m.Map, err = membudget.AllocSlice[uint32](p.Budget, mapCount)
	if err != nil {
		return nil, err
	}
	innerMask := uint32(1)<<innerBits - 1
	for i := range m.Map {
		raw, err := p.ReadBytes(entrySize)
		if err != nil {
			return nil, err
		}
		var entry uint32
		for _, b := range raw {
			entry = entry<<8 | uint32(b)
		}
		inner := entry & innerMask
		outer := entry >> innerBits
		m.Map[i] = (outer&0xFFFF)<<16 | (inner & 0xFFFF)
	}
	return m, nil
}

// Lookup returns the delta-set coordinate for value index i.  Indices at
// or beyond the end of the map clamp to the last entry.  An empty map
// yields (0, 0).
func (m *DeltaSetIndexMap) Lookup(i uint32) (outer, inner uint16) {
	if len(m.Map) == 0 {
		return 0, 0
	}
	if i >= uint32(len(m.Map)) {
		i = uint32(len(m.Map) - 1)
	}
	v := m.Map[i]
	return uint16(v >> 16), uint16(v)
}

// Encode returns the binary form of the delta-set index map.  It selects
// the minimal entry format and, for large maps, format 1; the output is
// deterministic.
func (m *DeltaSetIndexMap) Encode() []byte {
	maxInner := uint32(0)
	for _, v := range m.Map {
		if inner := v & 0xFFFF; inner > maxInner {
			maxInner = inner
		}
	}
	innerBits := max(bitCount(maxInner), 1)

	maxEntry := uint32(0)
	for _, v := range m.Map {
		outer := v >> 16
		inner := v & 0xFFFF
		entry := outer<<innerBits | inner
		if entry > maxEntry {
			maxEntry = entry
		}
	}
	entrySize := max((bitCount(maxEntry)+7)/8, 1)

	entryFormat := byte((innerBits - 1) | ((entrySize - 1) << 4))

	var buf []byte
	if len(m.Map) > 0xFFFF {
		buf = append(buf, 1, entryFormat)
		buf = appendU32(buf, uint32(len(m.Map)))
	} else {
		buf = append(buf, 0, entryFormat)
		buf = appendU16(buf, uint16(len(m.Map)))
	}
	for _, v := range m.Map {
		outer := v >> 16
		inner := v & 0xFFFF
		entry := outer<<innerBits | inner
		for b := entrySize - 1; b >= 0; b-- {
			buf = append(buf, byte(entry>>(8*b)))
		}
	}
	return buf
}

// bitCount returns the number of bits needed to represent v, and 0 for v == 0.
func bitCount(v uint32) int {
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}
