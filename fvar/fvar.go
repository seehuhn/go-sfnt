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

// Package fvar reads and writes the "fvar" table, which describes the
// variation axes and named instances of a variable font.
// https://learn.microsoft.com/en-us/typography/opentype/spec/fvar
package fvar

import (
	"errors"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// axisSize is the fixed size of a VariationAxisRecord in bytes.
const axisSize = 20

// noNameID is the sentinel meaning "no PostScript name" for an instance.
const noNameID = 0xFFFF

// hiddenAxisFlag marks an axis that should not be exposed in a user
// interface.
const hiddenAxisFlag = 0x0001

// Table represents the contents of an "fvar" table.
type Table struct {
	Axes      []Axis
	Instances []Instance
}

// Axis describes a single variation axis.  The Min, Default and Max
// values are in the axis' user-facing coordinate scale.
type Axis struct {
	Tag     string  // four-character axis identifier
	Min     float64 // minimum user coordinate
	Default float64 // default user coordinate
	Max     float64 // maximum user coordinate
	Hidden  bool    // axis is hidden from user interfaces
	NameID  uint16  // "name" table entry for the axis name

	// Name is the resolved axis name.  It is informational, filled in by
	// higher layers, and ignored by [Table.Encode].
	Name string
}

// Instance describes a named instance: a specific point in the design
// space with an associated name.
type Instance struct {
	NameID           uint16    // "name" table entry for the instance name
	PostScriptNameID uint16    // "name" table entry for the PostScript name; 0xFFFF if absent
	Flags            uint16    // instance flags
	Coordinates      []float64 // user coordinates, one per axis

	// Name and PostScriptName are resolved names.  They are
	// informational, filled in by higher layers, and ignored by
	// [Table.Encode].
	Name           string
	PostScriptName string
}

// Read reads and decodes an "fvar" table.  Allocations are charged
// against budget.  A malformed table yields an error.
func Read(r parser.ReadSeekSizer, budget *membudget.Budget) (*Table, error) {
	p := parser.New(r, budget)

	buf, err := p.ReadBytes(16)
	if err != nil {
		return nil, err
	}
	majorVersion := uint16(buf[0])<<8 | uint16(buf[1])
	axesArrayOffset := int64(uint16(buf[4])<<8 | uint16(buf[5]))
	axisCount := int(uint16(buf[8])<<8 | uint16(buf[9]))
	storedAxisSize := int(uint16(buf[10])<<8 | uint16(buf[11]))
	instanceCount := int(uint16(buf[12])<<8 | uint16(buf[13]))
	instanceSize := int(uint16(buf[14])<<8 | uint16(buf[15]))

	if majorVersion != 1 {
		return nil, errors.New("fvar: unsupported table version")
	}
	if axisCount > variation.MaxAxisCount {
		return nil, errors.New("fvar: too many axes")
	}
	if storedAxisSize < axisSize {
		return nil, errors.New("fvar: axis size too small")
	}
	minInstanceSize := 4 + 4*axisCount
	if instanceCount > 0 && instanceSize < minInstanceSize {
		return nil, errors.New("fvar: instance size too small")
	}

	// bounds check before any allocation
	end := axesArrayOffset + int64(axisCount)*int64(storedAxisSize) +
		int64(instanceCount)*int64(instanceSize)
	if axesArrayOffset < 16 || end > p.Size() {
		return nil, errors.New("fvar: table too short")
	}

	t := &Table{}

	if axisCount > 0 {
		t.Axes, err = membudget.AllocSlice[Axis](budget, axisCount)
		if err != nil {
			return nil, err
		}
	}
	for i := range t.Axes {
		if err := p.SeekPos(axesArrayOffset + int64(i)*int64(storedAxisSize)); err != nil {
			return nil, err
		}
		rec, err := p.ReadBytes(axisSize)
		if err != nil {
			return nil, err
		}
		tag := string(rec[0:4])
		minV := variation.Fixed(int32(uint32(rec[4])<<24 | uint32(rec[5])<<16 | uint32(rec[6])<<8 | uint32(rec[7])))
		defV := variation.Fixed(int32(uint32(rec[8])<<24 | uint32(rec[9])<<16 | uint32(rec[10])<<8 | uint32(rec[11])))
		maxV := variation.Fixed(int32(uint32(rec[12])<<24 | uint32(rec[13])<<16 | uint32(rec[14])<<8 | uint32(rec[15])))
		flags := uint16(rec[16])<<8 | uint16(rec[17])
		nameID := uint16(rec[18])<<8 | uint16(rec[19])
		t.Axes[i] = Axis{
			Tag:     tag,
			Min:     minV.Float64(),
			Default: defV.Float64(),
			Max:     maxV.Float64(),
			Hidden:  flags&hiddenAxisFlag != 0,
			NameID:  nameID,
		}
	}

	if instanceCount > 0 {
		t.Instances, err = membudget.AllocSlice[Instance](budget, instanceCount)
		if err != nil {
			return nil, err
		}
	}
	hasPS := instanceSize >= minInstanceSize+2
	instancesStart := axesArrayOffset + int64(axisCount)*int64(storedAxisSize)
	for i := range t.Instances {
		if err := p.SeekPos(instancesStart + int64(i)*int64(instanceSize)); err != nil {
			return nil, err
		}
		head, err := p.ReadBytes(4)
		if err != nil {
			return nil, err
		}
		inst := Instance{
			NameID:           uint16(head[0])<<8 | uint16(head[1]),
			Flags:            uint16(head[2])<<8 | uint16(head[3]),
			PostScriptNameID: noNameID,
		}
		if axisCount > 0 {
			inst.Coordinates, err = membudget.AllocSlice[float64](budget, axisCount)
			if err != nil {
				return nil, err
			}
		}
		for j := range inst.Coordinates {
			v, err := variation.ReadFixed(p)
			if err != nil {
				return nil, err
			}
			inst.Coordinates[j] = v.Float64()
		}
		if hasPS {
			ps, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			if ps != 0 {
				inst.PostScriptNameID = ps
			}
		}
		t.Instances[i] = inst
	}

	return t, nil
}

// hasPostScriptNames reports whether any instance carries a PostScript
// name identifier.
func (t *Table) hasPostScriptNames() bool {
	for i := range t.Instances {
		ps := t.Instances[i].PostScriptNameID
		if ps != noNameID && ps != 0 {
			return true
		}
	}
	return false
}

// Encode returns the binary form of the fvar table.  The output is
// deterministic: the PostScript-name variant of the instance record is
// written if and only if some instance has a PostScript name.
func (t *Table) Encode() []byte {
	axisCount := len(t.Axes)
	longInstance := t.hasPostScriptNames()
	instanceSize := 4 + 4*axisCount
	if longInstance {
		instanceSize += 2
	}

	total := 16 + axisCount*axisSize + len(t.Instances)*instanceSize
	buf := make([]byte, 0, total)

	// header
	buf = appendU16(buf, 1)  // majorVersion
	buf = appendU16(buf, 0)  // minorVersion
	buf = appendU16(buf, 16) // axesArrayOffset
	buf = appendU16(buf, 2)  // reserved
	buf = appendU16(buf, uint16(axisCount))
	buf = appendU16(buf, axisSize)
	buf = appendU16(buf, uint16(len(t.Instances)))
	buf = appendU16(buf, uint16(instanceSize))

	// axes
	for i := range t.Axes {
		a := &t.Axes[i]
		buf = appendTag(buf, a.Tag)
		buf = appendU32(buf, uint32(variation.FixedFromFloat(a.Min)))
		buf = appendU32(buf, uint32(variation.FixedFromFloat(a.Default)))
		buf = appendU32(buf, uint32(variation.FixedFromFloat(a.Max)))
		var flags uint16
		if a.Hidden {
			flags |= hiddenAxisFlag
		}
		buf = appendU16(buf, flags)
		buf = appendU16(buf, a.NameID)
	}

	// instances
	for i := range t.Instances {
		inst := &t.Instances[i]
		buf = appendU16(buf, inst.NameID)
		buf = appendU16(buf, inst.Flags)
		for j := range axisCount {
			var c float64
			if j < len(inst.Coordinates) {
				c = inst.Coordinates[j]
			}
			buf = appendU32(buf, uint32(variation.FixedFromFloat(c)))
		}
		if longInstance {
			ps := inst.PostScriptNameID
			if ps == 0 {
				ps = noNameID
			}
			buf = appendU16(buf, ps)
		}
	}

	return buf
}

// Normalize converts a set of user-scale axis coordinates to normalized
// F2Dot14 coordinates, one per axis in axis order.  coords is keyed by
// axis tag; axes absent from the map use their default value.  An
// unknown tag yields an error.  This applies the fvar normalization step
// only; any avar adjustment is applied separately.
func (t *Table) Normalize(coords map[string]float64) ([]variation.F2Dot14, error) {
	for tag := range coords {
		if !t.hasAxis(tag) {
			return nil, errors.New("fvar: unknown axis tag")
		}
	}

	result := make([]variation.F2Dot14, len(t.Axes))
	for i := range t.Axes {
		a := &t.Axes[i]
		c, ok := coords[a.Tag]
		if !ok {
			c = a.Default
		}
		result[i] = variation.F2Dot14FromFloat(normalizeAxis(c, a.Min, a.Default, a.Max))
	}
	return result, nil
}

// hasAxis reports whether the table contains an axis with the given tag.
func (t *Table) hasAxis(tag string) bool {
	for i := range t.Axes {
		if t.Axes[i].Tag == tag {
			return true
		}
	}
	return false
}

// normalizeAxis maps a single user coordinate to the [-1, 1] normalized
// range, following the fvar normalization rules.
func normalizeAxis(c, min, def, max float64) float64 {
	// clamp to the axis range
	if c < min {
		c = min
	}
	if c > max {
		c = max
	}

	switch {
	case c < def:
		if def == min {
			return 0
		}
		return (c - def) / (def - min)
	case c > def:
		if max == def {
			return 0
		}
		return (c - def) / (max - def)
	default:
		return 0
	}
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
