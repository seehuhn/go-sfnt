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

// Package cvar reads and writes the "cvar" table, which stores variation
// data for the entries of a font's "cvt " table.
// https://learn.microsoft.com/en-us/typography/opentype/spec/cvar
//
// Unlike "gvar", "cvar" has no shared-tuples array: every tuple must embed
// its own peak.  It does, however, support the SHARED_POINT_NUMBERS
// mechanism for point numbers, where the point numbers here select CVT
// indices rather than glyph outline points.
package cvar

import (
	"errors"
	"math"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/variation"
)

// headerSize is the fixed size of the cvar header, up to and excluding the
// tuple variation headers.
const headerSize = 8

var (
	errShortTable         = errors.New("cvar: table too short")
	errUnsupportedVersion = errors.New("cvar: unsupported table version")
	errBadAxisCount       = errors.New("cvar: invalid axis count")
	errBadOffset          = errors.New("cvar: invalid data offset")
	errMissingPeak        = errors.New("cvar: tuple has no embedded peak")
)

// Table represents the contents of a "cvar" table.
type Table struct {
	// AxisCount is the number of variation axes.  It must match the font's
	// "fvar" axis count.
	AxisCount int

	// Tuples holds the tuple variations, each a single-dimensional set of
	// deltas for a subset (or all) of the "cvt " table's entries.  Every
	// tuple embeds its own peak; a nil Peak is not writable.
	Tuples []variation.TupleVariation
}

// Decode parses a "cvar" table.  cvtCount is the number of entries in the
// font's "cvt " table, i.e. the number of deltable CVT indices.  Allocations
// are charged against budget.
//
// Per spec, every tuple in a "cvar" table must embed its own peak; a tuple
// header lacking the embedded-peak flag is malformed.  Decode is permissive
// and silently drops such a tuple rather than failing the whole table.
func Decode(data []byte, axisCount, cvtCount int, budget *membudget.Budget) (*Table, error) {
	if err := budget.Charge(headerSize); err != nil {
		return nil, err
	}
	if len(data) < headerSize {
		return nil, errShortTable
	}
	if axisCount < 0 || axisCount > variation.MaxAxisCount {
		return nil, errBadAxisCount
	}

	majorVersion := u16(data, 0)
	dataOffsetAbs := int(u16(data, 6))

	if majorVersion != 1 {
		return nil, errUnsupportedVersion
	}
	// dataOffset is measured from the start of the cvar table, but the
	// tuple-variation headers (and hence the region [4, dataOffsetAbs))
	// begin only after the 4-byte version fields.
	if dataOffsetAbs < headerSize || dataOffsetAbs > len(data) {
		return nil, errBadOffset
	}

	// build the block variation.DecodeTupleData expects: it starts at the
	// tupleVariationCount field and measures dataOffset relative to that
	// same position, 4 bytes later than the cvar table's own convention.
	sub, err := membudget.AllocSlice[byte](budget, len(data)-4)
	if err != nil {
		return nil, err
	}
	copy(sub, data[4:])
	relOffset := dataOffsetAbs - 4
	sub[2] = byte(relOffset >> 8)
	sub[3] = byte(relOffset)

	tuples, err := variation.DecodeTupleData(sub, axisCount, 1, cvtCount, true, budget)
	if err != nil {
		return nil, err
	}

	// cvar requires an embedded peak on every tuple; drop any that lack one
	kept := tuples[:0]
	for _, tv := range tuples {
		if tv.Peak == nil {
			continue
		}
		kept = append(kept, tv)
	}
	if len(kept) == 0 {
		kept = nil
	}

	return &Table{AxisCount: axisCount, Tuples: kept}, nil
}

// Encode returns the binary form of the cvar table.  cvtCount is the number
// of entries in the font's "cvt " table.  Every tuple must have a non-nil
// Peak; Encode returns an error otherwise.  The output is deterministic.
func (t *Table) Encode(cvtCount int) ([]byte, error) {
	if t.AxisCount < 0 || t.AxisCount > variation.MaxAxisCount {
		return nil, errBadAxisCount
	}
	for i := range t.Tuples {
		if t.Tuples[i].Peak == nil {
			return nil, errMissingPeak
		}
	}

	block, err := variation.EncodeTupleData(t.Tuples, t.AxisCount, 1, cvtCount, nil)
	if err != nil {
		return nil, err
	}
	// block[2:4] holds a dataOffset relative to block's own start (position
	// 4 of the cvar table); the cvar dataOffset field counts from the cvar
	// table start instead, 4 bytes further out.
	relOffset := int(u16(block, 2))
	absOffset := relOffset + 4
	if absOffset > math.MaxUint16 {
		return nil, errors.New("cvar: table too large")
	}

	buf := make([]byte, 0, 4+len(block))
	buf = appendU16(buf, 1) // majorVersion
	buf = appendU16(buf, 0) // minorVersion
	buf = append(buf, block[0], block[1])
	buf = appendU16(buf, uint16(absOffset))
	buf = append(buf, block[4:]...)
	return buf, nil
}

// Apply returns a new "cvt " table with the tuple deltas applied at the
// normalized axis coordinates coords.  cvt holds the input table's raw
// bytes: big-endian int16 FWORD entries, with an optional trailing odd byte
// preserved verbatim.  For each tuple whose interpolation weight is
// non-zero, its deltas are scaled by that weight and accumulated per CVT
// index; the accumulated deltas are rounded once and clamped to int16 before
// being added to the original values.
//
// A CVT index referenced by a tuple that falls outside the range covered by
// cvt is silently ignored.
func (t *Table) Apply(cvt []byte, coords []variation.F2Dot14) []byte {
	n := len(cvt) / 2

	deltas := make([]float64, n)
	for i := range t.Tuples {
		tv := &t.Tuples[i]
		s := tv.Scalar(coords, nil)
		if s == 0 {
			continue
		}
		if tv.Points == nil {
			m := min(n, len(tv.Deltas))
			for p := range m {
				deltas[p] += s * float64(tv.Deltas[p])
			}
			continue
		}
		for k, pn := range tv.Points {
			if k >= len(tv.Deltas) {
				break
			}
			p := int(pn)
			if p >= n {
				continue
			}
			deltas[p] += s * float64(tv.Deltas[k])
		}
	}

	out := make([]byte, len(cvt))
	for i := range n {
		orig := int16(u16(cvt, 2*i))
		v := clampInt16(variation.OTRound(float64(orig) + deltas[i]))
		out[2*i] = byte(uint16(v) >> 8)
		out[2*i+1] = byte(uint16(v))
	}
	if len(cvt)%2 == 1 {
		out[len(out)-1] = cvt[len(cvt)-1]
	}
	return out
}

func clampInt16(v float64) int16 {
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return int16(v)
}

func u16(b []byte, i int) uint16 {
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func appendU16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}
