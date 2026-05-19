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

// Package device encodes and decodes OpenType "Device Tables" and
// "VariationIndex Tables".  The two share a single offset slot in
// outer structures (e.g. GPOS value records) and are distinguished by
// the deltaFormat field.
//
// https://learn.microsoft.com/en-us/typography/opentype/spec/chapter2#device-and-variationindex-tables
package device

import (
	"fmt"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
)

// Table represents a Device table (per-ppem hinting deltas) or a
// VariationIndex table (item-variation-store reference).
type Table struct {
	// StartSize and EndSize bracket the ppem range a Device table
	// applies to.  They are unused for VariationIndex tables.
	StartSize, EndSize uint16

	// Deltas holds the per-ppem corrections of a Device table, one
	// entry per ppem from StartSize to EndSize inclusive.  The values
	// fit the bit width selected by DeltaFormat.
	Deltas []int8

	// OuterIndex and InnerIndex select a row in the item variation
	// store for a VariationIndex table.  Unused for Device tables.
	OuterIndex, InnerIndex uint16

	// DeltaFormat selects the encoding:
	//   1, 2, 3 — Device table with 2/4/8-bit packed deltas
	//   0x8000  — VariationIndex
	DeltaFormat uint16
}

// VariationIndexFormat is the deltaFormat value that selects a
// VariationIndex table.
const VariationIndexFormat = 0x8000

// IsVariationIndex reports whether t encodes a VariationIndex table.
func (t *Table) IsVariationIndex() bool {
	return t.DeltaFormat == VariationIndexFormat
}

// Read reads a Device or VariationIndex table starting at pos.
func Read(p *parser.Parser, pos int64) (*Table, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(6)
	if err != nil {
		return nil, err
	}
	first := uint16(buf[0])<<8 | uint16(buf[1])
	second := uint16(buf[2])<<8 | uint16(buf[3])
	deltaFormat := uint16(buf[4])<<8 | uint16(buf[5])

	t := &Table{DeltaFormat: deltaFormat}
	switch deltaFormat {
	case VariationIndexFormat:
		t.OuterIndex = first
		t.InnerIndex = second
		return t, nil
	case 1, 2, 3:
		t.StartSize = first
		t.EndSize = second
		if t.EndSize < t.StartSize {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/device",
				Reason:    "endSize before startSize",
			}
		}
		count := int(t.EndSize) - int(t.StartSize) + 1
		bitsPerDelta := int(1) << uint(deltaFormat)
		deltasPerWord := 16 / bitsPerDelta
		wordsNeeded := (count + deltasPerWord - 1) / deltasPerWord
		words, err := membudget.AllocSlice[uint16](p.Budget, wordsNeeded)
		if err != nil {
			return nil, err
		}
		for i := range words {
			w, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			words[i] = w
		}
		deltas, err := membudget.AllocSlice[int8](p.Budget, count)
		if err != nil {
			return nil, err
		}
		mask := uint16(1<<uint(bitsPerDelta)) - 1
		shiftLeft := uint(8 - bitsPerDelta)
		for i := range deltas {
			subIdx := i % deltasPerWord
			shift := uint(16 - bitsPerDelta*(subIdx+1))
			raw := (words[i/deltasPerWord] >> shift) & mask
			deltas[i] = int8(raw<<shiftLeft) >> shiftLeft
		}
		t.Deltas = deltas
		return t, nil
	default:
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/device",
			Reason:    fmt.Sprintf("invalid deltaFormat 0x%04x", deltaFormat),
		}
	}
}

// bitsPerDelta returns the number of bits used to encode each delta
// of a Device table.  It panics if DeltaFormat is not one of 1, 2, 3
// — callers must handle the VariationIndex case before invoking.
func (t *Table) bitsPerDelta() int {
	if t.DeltaFormat < 1 || t.DeltaFormat > 3 {
		panic(fmt.Sprintf("device: invalid DeltaFormat 0x%04x", t.DeltaFormat))
	}
	return 1 << t.DeltaFormat
}

// EncodeLen returns the number of bytes Encode will produce.
func (t *Table) EncodeLen() int {
	if t.IsVariationIndex() {
		return 6
	}
	bitsPerDelta := t.bitsPerDelta()
	deltasPerWord := 16 / bitsPerDelta
	wordsNeeded := (len(t.Deltas) + deltasPerWord - 1) / deltasPerWord
	return 6 + 2*wordsNeeded
}

// Encode returns the binary representation of the table.
func (t *Table) Encode() []byte {
	if t.IsVariationIndex() {
		return []byte{
			byte(t.OuterIndex >> 8), byte(t.OuterIndex),
			byte(t.InnerIndex >> 8), byte(t.InnerIndex),
			byte(t.DeltaFormat >> 8), byte(t.DeltaFormat),
		}
	}
	bitsPerDelta := t.bitsPerDelta()
	deltasPerWord := 16 / bitsPerDelta
	count := len(t.Deltas)
	wordsNeeded := (count + deltasPerWord - 1) / deltasPerWord

	res := make([]byte, 0, 6+2*wordsNeeded)
	res = append(res,
		byte(t.StartSize>>8), byte(t.StartSize),
		byte(t.EndSize>>8), byte(t.EndSize),
		byte(t.DeltaFormat>>8), byte(t.DeltaFormat),
	)

	mask := uint16(1<<uint(bitsPerDelta)) - 1
	for w := range wordsNeeded {
		var word uint16
		for k := range deltasPerWord {
			i := w*deltasPerWord + k
			if i >= count {
				break
			}
			shift := uint(16 - bitsPerDelta*(k+1))
			word |= (uint16(t.Deltas[i]) & mask) << shift
		}
		res = append(res, byte(word>>8), byte(word))
	}
	return res
}
