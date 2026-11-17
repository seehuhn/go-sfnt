// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package classdef reads and writes OpenType "Class Definition Tables".
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#classDefTbl
package classdef

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// Table contains the information from an OpenType "Class Definition Table".
// All glyphs not assigned to a class fall into Class 0.
type Table map[glyph.ID]uint16

// NumClasses returns the number of classes in the table.
// The count includes the zero class.
func (info Table) NumClasses() int {
	maxClass := uint16(0)
	for _, class := range info {
		if class > maxClass {
			maxClass = class
		}
	}
	return int(maxClass) + 1
}

// Glyphs returns the glyphs for each non-zero class in the Table.
// The first entry of the returned slice, corresponding to class 0,
// is always nil.
func (info Table) Glyphs() [][]glyph.ID {
	numClasses := info.NumClasses()
	glyphs := make([][]glyph.ID, numClasses)
	for gid, cls := range info {
		if cls == 0 {
			continue
		}
		glyphs[cls] = append(glyphs[cls], gid)
	}
	for i := 1; i < numClasses; i++ {
		slices.Sort(glyphs[i])
	}
	return glyphs
}

// Read reads and decodes an OpenType "Class Definition Table".
func Read(p *parser.Parser, pos int64) (Table, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	version, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	switch version {
	case 1:
		data, err := p.ReadBytes(4)
		if err != nil {
			return nil, err
		}
		startGlyphID := glyph.ID(data[0])<<8 | glyph.ID(data[1])
		glyphCount := int(data[2])<<8 | int(data[3])

		// class values past glyph ID 0xFFFF refer to no glyph and are ignored
		numValid := min(glyphCount, 0x10000-int(startGlyphID))

		if err := p.Budget.Charge(numValid * 24); err != nil {
			return nil, err
		}
		res := make(Table, numValid)
		for i := range numValid {
			classValue, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			if classValue != 0 {
				res[startGlyphID+glyph.ID(i)] = classValue
			}
		}
		return res, nil

	case 2:
		classRangeCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}

		// Each range contributes a start and an end event.  Sweeping over
		// the events in glyph order, a glyph keeps its class if all ranges
		// covering it agree.  Glyphs where the ranges assign different
		// classes are dropped, so that they fall into class 0.
		events, err := p.Budget.AllocSlice[rangeEvent](2 * int(classRangeCount))
		if err != nil {
			return nil, err
		}
		events = events[:0]
		for range int(classRangeCount) {
			data, err := p.ReadBytes(6)
			if err != nil {
				return nil, err
			}
			startGlyphID := int(data[0])<<8 | int(data[1])
			endGlyphID := int(data[2])<<8 | int(data[3])
			classValue := uint16(data[4])<<8 | uint16(data[5])

			if endGlyphID < startGlyphID {
				// an inverted range covers no glyphs
				continue
			}
			events = append(events,
				rangeEvent{pos: startGlyphID, class: classValue, delta: 1},
				rangeEvent{pos: endGlyphID + 1, class: classValue, delta: -1})
		}
		slices.SortFunc(events, func(a, b rangeEvent) int {
			return cmp.Compare(a.pos, b.pos)
		})

		// number of open ranges for each class
		if err := p.Budget.ChargeN(int(classRangeCount), 24); err != nil {
			return nil, err
		}
		active := make(map[uint16]int)

		res := Table{}
		for i := 0; i < len(events); {
			pos := events[i].pos
			for i < len(events) && events[i].pos == pos {
				e := events[i]
				active[e.class] += e.delta
				if active[e.class] == 0 {
					delete(active, e.class)
				}
				i++
			}
			if len(active) != 1 {
				continue
			}
			// Here i < len(events), since the end event of every open
			// range is still to come.
			var classValue uint16
			for c := range active {
				classValue = c
			}
			if classValue == 0 {
				continue
			}
			next := events[i].pos
			if err := p.Budget.Charge((next - pos) * 24); err != nil {
				return nil, err
			}
			for gid := pos; gid < next; gid++ {
				res[glyph.ID(gid)] = classValue
			}
		}
		return res, nil

	default:
		return nil, &parser.NotSupportedError{
			SubSystem: "opentype/classdef",
			Feature:   fmt.Sprintf("class definition table version %d", version),
		}
	}
}

// rangeEvent marks the start or the end of a format 2 class range.
// For an end event, pos is the first glyph after the range.
type rangeEvent struct {
	pos   int
	class uint16
	delta int
}

type encInfo struct {
	minGid, maxGid glyph.ID
	format1Size    int
	format2Size    int

	// useFormat1 is true if format 1 is the smaller representation.  Format
	// 1 cannot represent a table which extends over all 65536 glyph IDs,
	// since its glyph count is a 16-bit field.
	useFormat1 bool
}

func (info Table) getEncInfo() *encInfo {
	minGid := glyph.ID(0xFFFF)
	maxGid := glyph.ID(0)
	for key := range info {
		if key < minGid {
			minGid = key
		}
		if key > maxGid {
			maxGid = key
		}
	}

	numGlyphs := int(maxGid) - int(minGid) + 1
	format1Size := 6 + 2*numGlyphs
	format1OK := numGlyphs <= 0xFFFF

	// counting stops early once format 1 is known to be smaller
	segCount := 0
	segStart := -1
	var segClass uint16
	for i := int(minGid); i <= int(maxGid) && (!format1OK || 4+6*segCount < format1Size); i++ {
		class := info[glyph.ID(i)]

		if segStart >= 0 && class != segClass {
			segCount++
			segStart = -1
		}
		if segStart == -1 {
			if class != 0 {
				segStart = i
				segClass = class
			}
		}
	}
	if segStart >= 0 {
		segCount++
	}

	format2Size := 4 + 6*segCount

	return &encInfo{
		minGid:      minGid,
		maxGid:      maxGid,
		format1Size: format1Size,
		format2Size: format2Size,
		useFormat1:  format1OK && format1Size <= format2Size,
	}
}

// AppendLen returns the size of the binary table representation, for a
// table which [Table.Append] can encode.
func (info Table) AppendLen() int {
	if len(info) == 0 {
		return 4
	}
	encInfo := info.getEncInfo()
	if encInfo.useFormat1 {
		return encInfo.format1Size
	}
	return encInfo.format2Size
}

// Append appends the binary table representation to the given buffer.  An
// error is returned if the table cannot be represented, which happens when
// the classes extend over all 65536 glyph IDs and form more than 65535
// ranges.
func (info Table) Append(buf []byte) ([]byte, error) {
	if len(info) == 0 {
		return append(buf, 0, 2, 0, 0), nil
	}

	encInfo := info.getEncInfo()

	if encInfo.useFormat1 {
		count := encInfo.maxGid - encInfo.minGid + 1
		buf = append(buf,
			0, 1,
			byte(encInfo.minGid>>8), byte(encInfo.minGid),
			byte(count>>8), byte(count))
		for i := 0; i < int(count); i++ {
			class := info[encInfo.minGid+glyph.ID(i)]
			buf = append(buf, byte(class>>8), byte(class))
		}
		return buf, nil
	}

	segCount := (encInfo.format2Size - 4) / 6
	if segCount > 0xFFFF {
		return nil, errors.New("class definition table too large")
	}
	buf = append(buf, 0, 2, byte(segCount>>8), byte(segCount))
	segStart := -1
	var segClass uint16
	for i := int(encInfo.minGid); i <= int(encInfo.maxGid); i++ {
		class := info[glyph.ID(i)]

		if segStart >= 0 && class != segClass {
			buf = append(buf,
				byte(segStart>>8), byte(segStart),
				byte((i-1)>>8), byte(i-1),
				byte(segClass>>8), byte(segClass))
			segStart = -1
		}
		if segStart == -1 {
			if class != 0 {
				segStart = i
				segClass = class
			}
		}
	}
	if segStart >= 0 {
		buf = append(buf,
			byte(segStart>>8), byte(segStart),
			byte(encInfo.maxGid>>8), byte(encInfo.maxGid),
			byte(segClass>>8), byte(segClass))
	}

	return buf, nil
}
