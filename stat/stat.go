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

// Package stat reads and writes the "STAT" table, which records style
// attributes used to relate variable-font instances and named font
// families.
// https://learn.microsoft.com/en-us/typography/opentype/spec/stat
package stat

import (
	"errors"
	"fmt"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// designAxisRecordSize is the strict-write size of one design axis
// record; a larger declared size on read is permitted, with the extra
// bytes skipped.
const designAxisRecordSize = 8

// headerSize is the strict-write size of the STAT header, version 1.2
// (including elidedFallbackNameID).
const headerSize = 20

// minHeaderSize is the size of the fixed part of the header shared by
// all minor versions, up to and including offsetToAxisValueOffsets.
const minHeaderSize = 18

// Table represents the contents of a "STAT" table.
type Table struct {
	DesignAxes []DesignAxis // in axis-record order
	AxisValues []AxisValue

	// ElidedFallbackNameID is the "name" table entry used when no axis
	// value applies and the elided-fallback style name should be shown.
	ElidedFallbackNameID uint16

	// ElidedFallbackName is the resolved elided fallback name.  It is
	// informational, filled in by higher layers, and ignored by
	// [Table.Encode].
	ElidedFallbackName string
}

// DesignAxis describes a single design axis used by the axis values
// below.
type DesignAxis struct {
	Tag      string // four-character axis identifier
	NameID   uint16 // "name" table entry for the axis name
	Ordering uint16 // relative ordering of the axis for UI purposes

	// Name is the resolved axis name.  It is informational, filled in
	// by higher layers, and ignored by [Table.Encode].
	Name string
}

// AxisValue is one entry of a STAT table's axis value array.  The
// concrete type is one of [*Format1], [*Format2], [*Format3], or
// [*Format4].
type AxisValue interface{ isAxisValue() }

// Format1 associates a single axis coordinate with a style name.
type Format1 struct {
	AxisIndex uint16 // index into [Table.DesignAxes]
	Flags     uint16
	NameID    uint16  // "name" table entry for the value's name
	Value     float64 // axis coordinate

	// Name is the resolved value name.  It is informational, filled in
	// by higher layers, and ignored by [Table.Encode].
	Name string
}

func (*Format1) isAxisValue() {}

// Format2 associates a range of axis coordinates with a style name.
type Format2 struct {
	AxisIndex         uint16
	Flags             uint16
	NameID            uint16
	Nominal, Min, Max float64
	Name              string
}

func (*Format2) isAxisValue() {}

// Format3 associates a single axis coordinate with a style name, and
// links it to a coordinate on the same axis considered equivalent for
// style-linking (e.g. Bold linked to Regular).
type Format3 struct {
	AxisIndex          uint16
	Flags              uint16
	NameID             uint16
	Value, LinkedValue float64
	Name               string
}

func (*Format3) isAxisValue() {}

// Format4 associates a combination of coordinates on multiple axes with
// a single style name.
type Format4 struct {
	Flags  uint16
	NameID uint16
	Name   string
	Values []AxisValueEntry
}

func (*Format4) isAxisValue() {}

// AxisValueEntry is one axis/value pair within a [Format4] axis value.
type AxisValueEntry struct {
	AxisIndex uint16
	Value     float64
}

// Read reads and decodes a "STAT" table.  Allocations are charged
// against budget.  A malformed table yields an error.  Axis value
// records using an unrecognized format are skipped permissively.
func Read(r parser.ReadSeekSizer, budget *membudget.Budget) (*Table, error) {
	p := parser.New(r, budget)

	buf, err := p.ReadBytes(minHeaderSize)
	if err != nil {
		return nil, err
	}
	majorVersion := u16(buf, 0)
	minorVersion := u16(buf, 2)
	designAxisSize := int(u16(buf, 4))
	designAxisCount := int(u16(buf, 6))
	designAxesOffset := int64(u32(buf, 8))
	axisValueCount := int(u16(buf, 12))
	offsetToAxisValueOffsets := int64(u32(buf, 14))

	if majorVersion != 1 || minorVersion > 2 {
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/stat",
			Feature:   fmt.Sprintf("STAT table version %d.%d", majorVersion, minorVersion),
		}
	}

	var elidedFallbackNameID uint16
	if minorVersion >= 1 {
		elidedFallbackNameID, err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	t := &Table{ElidedFallbackNameID: elidedFallbackNameID}

	if designAxisCount > 0 {
		if designAxisSize < designAxisRecordSize {
			return nil, errors.New("stat: design axis size too small")
		}
		// bounds check before any allocation
		end := designAxesOffset + int64(designAxisCount)*int64(designAxisSize)
		if designAxesOffset < 0 || end > p.Size() {
			return nil, errors.New("stat: design axes out of bounds")
		}

		t.DesignAxes, err = membudget.AllocSlice[DesignAxis](budget, designAxisCount)
		if err != nil {
			return nil, err
		}
		for i := range t.DesignAxes {
			pos := designAxesOffset + int64(i)*int64(designAxisSize)
			if err := p.SeekPos(pos); err != nil {
				return nil, err
			}
			rec, err := p.ReadBytes(designAxisRecordSize)
			if err != nil {
				return nil, err
			}
			t.DesignAxes[i] = DesignAxis{
				Tag:      string(rec[0:4]),
				NameID:   u16(rec, 4),
				Ordering: u16(rec, 6),
			}
		}
	}

	if axisValueCount > 0 {
		// bounds check before any allocation
		end := offsetToAxisValueOffsets + int64(axisValueCount)*2
		if offsetToAxisValueOffsets < 0 || end > p.Size() {
			return nil, errors.New("stat: axis value offsets out of bounds")
		}
		if err := p.SeekPos(offsetToAxisValueOffsets); err != nil {
			return nil, err
		}
		offs, err := membudget.AllocSlice[uint16](budget, axisValueCount)
		if err != nil {
			return nil, err
		}
		for i := range offs {
			offs[i], err = p.ReadUint16()
			if err != nil {
				return nil, err
			}
		}

		axisValues, err := membudget.AllocSlice[AxisValue](budget, axisValueCount)
		if err != nil {
			return nil, err
		}
		n := 0
		for _, off := range offs {
			av, err := readAxisValue(p, offsetToAxisValueOffsets+int64(off))
			if err != nil {
				return nil, err
			}
			if av == nil {
				// unrecognized format: dropped permissively
				continue
			}
			axisValues[n] = av
			n++
		}
		if n > 0 {
			t.AxisValues = axisValues[:n]
		}
	}

	return t, nil
}

// readAxisValue reads a single axis value table at pos.  It returns a
// nil AxisValue and a nil error for an unrecognized format, signalling
// that the caller should skip the record.
func readAxisValue(p *parser.Parser, pos int64) (AxisValue, error) {
	if err := p.SeekPos(pos); err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(8)
	if err != nil {
		return nil, err
	}
	format := u16(buf, 0)
	// the second header field is axisIndex for formats 1-3, but
	// axisCount for format 4
	field2 := u16(buf, 2)
	flags := u16(buf, 4)
	nameID := u16(buf, 6)

	switch format {
	case 1:
		axisIndex := field2
		v, err := variation.ReadFixed(p)
		if err != nil {
			return nil, err
		}
		return &Format1{AxisIndex: axisIndex, Flags: flags, NameID: nameID, Value: v.Float64()}, nil
	case 2:
		axisIndex := field2
		nominal, err := variation.ReadFixed(p)
		if err != nil {
			return nil, err
		}
		min, err := variation.ReadFixed(p)
		if err != nil {
			return nil, err
		}
		max, err := variation.ReadFixed(p)
		if err != nil {
			return nil, err
		}
		return &Format2{
			AxisIndex: axisIndex, Flags: flags, NameID: nameID,
			Nominal: nominal.Float64(), Min: min.Float64(), Max: max.Float64(),
		}, nil
	case 3:
		axisIndex := field2
		value, err := variation.ReadFixed(p)
		if err != nil {
			return nil, err
		}
		linked, err := variation.ReadFixed(p)
		if err != nil {
			return nil, err
		}
		return &Format3{
			AxisIndex: axisIndex, Flags: flags, NameID: nameID,
			Value: value.Float64(), LinkedValue: linked.Float64(),
		}, nil
	case 4:
		axisCount := int(field2)
		if err := checkCount(p, axisCount, 6); err != nil {
			return nil, err
		}
		var values []AxisValueEntry
		if axisCount > 0 {
			values, err = membudget.AllocSlice[AxisValueEntry](p.Budget, axisCount)
			if err != nil {
				return nil, err
			}
		}
		for i := range values {
			rec, err := p.ReadBytes(6)
			if err != nil {
				return nil, err
			}
			val := variation.Fixed(int32(u32(rec, 2)))
			values[i] = AxisValueEntry{AxisIndex: u16(rec, 0), Value: val.Float64()}
		}
		return &Format4{Flags: flags, NameID: nameID, Values: values}, nil
	default:
		return nil, nil
	}
}

// checkCount reports an error if count elements of elemSize bytes each
// cannot fit within the remaining input, guarding against oversized
// counts in malformed tables before any allocation.
func checkCount(p *parser.Parser, count, elemSize int) error {
	if count < 0 {
		return errors.New("stat: negative count")
	}
	if int64(count)*int64(elemSize) > p.Size() {
		return errors.New("stat: count exceeds input size")
	}
	return nil
}

// Encode returns the binary form of the STAT table.  The output always
// uses version 1.2, a design axis record size of 8, and a deterministic
// layout: unrecognized axis value formats read by [Read] are not
// represented and so cannot round-trip.
func (t *Table) Encode() []byte {
	designAxesOffset := headerSize
	total := designAxesOffset + len(t.DesignAxes)*designAxisRecordSize
	offsetToAxisValueOffsets := total
	total += 2 * len(t.AxisValues)

	avOffsets := make([]int, len(t.AxisValues))
	pos := total
	for i, av := range t.AxisValues {
		avOffsets[i] = pos - offsetToAxisValueOffsets
		pos += axisValueEncodeLen(av)
	}
	total = pos

	buf := make([]byte, 0, total)
	buf = appendU16(buf, 1)                                // majorVersion
	buf = appendU16(buf, 2)                                // minorVersion
	buf = appendU16(buf, designAxisRecordSize)             // designAxisSize
	buf = appendU16(buf, uint16(len(t.DesignAxes)))        // designAxisCount
	buf = appendU32(buf, uint32(designAxesOffset))         // designAxesOffset
	buf = appendU16(buf, uint16(len(t.AxisValues)))        // axisValueCount
	buf = appendU32(buf, uint32(offsetToAxisValueOffsets)) // offsetToAxisValueOffsets
	buf = appendU16(buf, t.ElidedFallbackNameID)

	for _, a := range t.DesignAxes {
		buf = appendTag(buf, a.Tag)
		buf = appendU16(buf, a.NameID)
		buf = appendU16(buf, a.Ordering)
	}

	for _, off := range avOffsets {
		buf = appendU16(buf, uint16(off))
	}

	for _, av := range t.AxisValues {
		buf = appendAxisValue(buf, av)
	}

	return buf
}

// axisValueEncodeLen returns the number of bytes [appendAxisValue]
// writes for av.
func axisValueEncodeLen(av AxisValue) int {
	switch v := av.(type) {
	case *Format1:
		return 12
	case *Format2:
		return 20
	case *Format3:
		return 16
	case *Format4:
		return 8 + 6*len(v.Values)
	default:
		panic(fmt.Sprintf("stat: unknown axis value type %T", av))
	}
}

// appendAxisValue appends the binary form of av to buf.
func appendAxisValue(buf []byte, av AxisValue) []byte {
	switch v := av.(type) {
	case *Format1:
		buf = appendU16(buf, 1)
		buf = appendU16(buf, v.AxisIndex)
		buf = appendU16(buf, v.Flags)
		buf = appendU16(buf, v.NameID)
		buf = appendU32(buf, uint32(variation.FixedFromFloat(v.Value)))
	case *Format2:
		buf = appendU16(buf, 2)
		buf = appendU16(buf, v.AxisIndex)
		buf = appendU16(buf, v.Flags)
		buf = appendU16(buf, v.NameID)
		buf = appendU32(buf, uint32(variation.FixedFromFloat(v.Nominal)))
		buf = appendU32(buf, uint32(variation.FixedFromFloat(v.Min)))
		buf = appendU32(buf, uint32(variation.FixedFromFloat(v.Max)))
	case *Format3:
		buf = appendU16(buf, 3)
		buf = appendU16(buf, v.AxisIndex)
		buf = appendU16(buf, v.Flags)
		buf = appendU16(buf, v.NameID)
		buf = appendU32(buf, uint32(variation.FixedFromFloat(v.Value)))
		buf = appendU32(buf, uint32(variation.FixedFromFloat(v.LinkedValue)))
	case *Format4:
		buf = appendU16(buf, 4)
		buf = appendU16(buf, uint16(len(v.Values)))
		buf = appendU16(buf, v.Flags)
		buf = appendU16(buf, v.NameID)
		for _, e := range v.Values {
			buf = appendU16(buf, e.AxisIndex)
			buf = appendU32(buf, uint32(variation.FixedFromFloat(e.Value)))
		}
	default:
		panic(fmt.Sprintf("stat: unknown axis value type %T", av))
	}
	return buf
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

// appendTag appends a four-character tag, padding with spaces or
// truncating as needed.
func appendTag(buf []byte, tag string) []byte {
	var b [4]byte
	copy(b[:], "    ")
	copy(b[:], tag)
	return append(buf, b[:]...)
}
