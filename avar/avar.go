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

// Package avar reads and writes the "avar" table, which adjusts the
// normalized variation coordinates of a variable font.
// https://learn.microsoft.com/en-us/typography/opentype/spec/avar
package avar

import (
	"errors"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// Table represents the contents of an "avar" table.
//
// A version 1 table is decoded into SegmentMaps, one map per axis.  Any
// other version is kept verbatim in Raw and reported as unsupported by
// [Table.IsSupported]; such a table is re-emitted unchanged by
// [Table.Encode].
type Table struct {
	SegmentMaps []SegmentMap
	Raw         []byte
}

// SegmentMap is the piecewise-linear remapping for one axis, given as a
// sequence of control points ordered by From.
type SegmentMap []AxisValueMap

// AxisValueMap maps one normalized coordinate value to another.
type AxisValueMap struct {
	From variation.F2Dot14
	To   variation.F2Dot14
}

// Read reads and decodes an "avar" table.  Allocations are charged
// against budget.  A version other than 1.0 is retained verbatim (see
// [Table]).
func Read(r parser.ReadSeekSizer, budget *membudget.Budget) (*Table, error) {
	p := parser.New(r, budget)

	buf, err := p.ReadBytes(8)
	if err != nil {
		return nil, err
	}
	majorVersion := uint16(buf[0])<<8 | uint16(buf[1])
	minorVersion := uint16(buf[2])<<8 | uint16(buf[3])
	axisCount := int(uint16(buf[6])<<8 | uint16(buf[7]))

	if majorVersion != 1 || minorVersion != 0 {
		return readRaw(p, budget)
	}

	if axisCount > variation.MaxAxisCount {
		return nil, errors.New("avar: too many axes")
	}

	t := &Table{}
	if axisCount > 0 {
		t.SegmentMaps, err = membudget.AllocSlice[SegmentMap](budget, axisCount)
		if err != nil {
			return nil, err
		}
	}
	for i := range t.SegmentMaps {
		count, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		n := int(count)
		if int64(n)*4 > p.Size() {
			return nil, errors.New("avar: segment map too large")
		}
		if n == 0 {
			continue
		}
		seg, err := membudget.AllocSlice[AxisValueMap](budget, n)
		if err != nil {
			return nil, err
		}
		for j := range seg {
			from, err := variation.ReadF2Dot14(p)
			if err != nil {
				return nil, err
			}
			to, err := variation.ReadF2Dot14(p)
			if err != nil {
				return nil, err
			}
			seg[j] = AxisValueMap{From: from, To: to}
		}
		t.SegmentMaps[i] = seg
	}

	return t, nil
}

// readRaw retains the whole table verbatim for an unsupported version.
func readRaw(p *parser.Parser, budget *membudget.Budget) (*Table, error) {
	size := p.Size()
	if size < 0 {
		return nil, errors.New("avar: negative size")
	}
	raw, err := membudget.AllocSlice[byte](budget, int(size))
	if err != nil {
		return nil, err
	}
	if err := p.SeekPos(0); err != nil {
		return nil, err
	}
	if _, err := p.Read(raw); err != nil {
		return nil, err
	}
	return &Table{Raw: raw}, nil
}

// IsSupported reports whether the table is a decoded version 1 table.
// It returns false for a table whose version was not understood and kept
// as raw bytes.
func (t *Table) IsSupported() bool {
	return t.Raw == nil
}

// Encode returns the binary form of the avar table.  An unsupported
// table (see [Table]) is re-emitted from its raw bytes unchanged.
func (t *Table) Encode() []byte {
	if t.Raw != nil {
		return t.Raw
	}

	total := 8
	for _, seg := range t.SegmentMaps {
		total += 2 + 4*len(seg)
	}
	buf := make([]byte, 0, total)

	// header
	buf = appendU16(buf, 1) // majorVersion
	buf = appendU16(buf, 0) // minorVersion
	buf = appendU16(buf, 0) // reserved
	buf = appendU16(buf, uint16(len(t.SegmentMaps)))

	for _, seg := range t.SegmentMaps {
		buf = appendU16(buf, uint16(len(seg)))
		for _, m := range seg {
			buf = appendU16(buf, uint16(m.From))
			buf = appendU16(buf, uint16(m.To))
		}
	}

	return buf
}

// Map applies the axis-value maps to the normalized coordinates coords,
// returning a new slice of the same length.  An axis without a segment
// map, or with an empty one, is left unchanged.
func (t *Table) Map(coords []variation.F2Dot14) []variation.F2Dot14 {
	result := make([]variation.F2Dot14, len(coords))
	for i, c := range coords {
		if i < len(t.SegmentMaps) {
			result[i] = mapValue(t.SegmentMaps[i], c)
		} else {
			result[i] = c
		}
	}
	return result
}

// mapValue applies a single segment map to one coordinate.  An empty map
// is the identity; coordinates outside the map's range clamp to the
// nearest control point's To value.  The result is clamped to [-1, 1].
func mapValue(seg SegmentMap, c variation.F2Dot14) variation.F2Dot14 {
	if len(seg) == 0 {
		return c
	}

	v := c.Float64()
	if v <= seg[0].From.Float64() {
		return clamp(seg[0].To.Float64())
	}
	for i := 1; i < len(seg); i++ {
		f1 := seg[i].From.Float64()
		if v <= f1 {
			f0 := seg[i-1].From.Float64()
			t0 := seg[i-1].To.Float64()
			t1 := seg[i].To.Float64()
			if f1 == f0 {
				return clamp(t1)
			}
			frac := (v - f0) / (f1 - f0)
			return clamp(t0 + frac*(t1-t0))
		}
	}
	return clamp(seg[len(seg)-1].To.Float64())
}

// clamp restricts v to [-1, 1] and rounds it to F2Dot14.
func clamp(v float64) variation.F2Dot14 {
	if v < -1 {
		v = -1
	}
	if v > 1 {
		v = 1
	}
	return variation.F2Dot14FromFloat(v)
}

func appendU16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}
