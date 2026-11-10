// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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

package gtab

import (
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

// Gpos6_1 is a Mark-to-Mark Attachment Positioning Subtable (format 1).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#lookup-type-6-mark-to-mark-attachment-positioning-subtable
type Gpos6_1 struct {
	Mark1Cov   coverage.Table
	Mark2Cov   coverage.Table
	Mark1Array []markarray.Record // indexed by mark1 coverage index
	Mark2Array [][]*anchor.Table  // indexed by mark2 coverage index, then by mark class; nil entries are absent
}

func readGpos6_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(10)
	if err != nil {
		return nil, err
	}
	anchors := anchor.NewReader(p)

	mark1CoverageOffset := int64(buf[0])<<8 | int64(buf[1])
	mark2CoverageOffset := int64(buf[2])<<8 | int64(buf[3])
	markClassCount := int(buf[4])<<8 | int(buf[5])
	mark1ArrayOffset := int64(buf[6])<<8 | int64(buf[7])
	mark2ArrayOffset := int64(buf[8])<<8 | int64(buf[9])

	mark1Cov, err := coverage.Read(p, subtablePos+mark1CoverageOffset)
	if err != nil {
		return nil, err
	}
	mark2Cov, err := coverage.Read(p, subtablePos+mark2CoverageOffset)
	if err != nil {
		return nil, err
	}

	mark1Array, err := markarray.Read(p, anchors, subtablePos+mark1ArrayOffset, len(mark1Cov))
	if err != nil {
		return nil, err
	}
	if len(mark1Cov) > len(mark1Array) {
		mark1Cov.Prune(len(mark1Array))
	} else {
		mark1Array = mark1Array[:len(mark1Cov)]
	}
	for _, rec := range mark1Array {
		if int(rec.Class) >= markClassCount {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "mark class out of range",
			}
		}
	}

	mark2ArrayPos := subtablePos + mark2ArrayOffset
	err = p.SeekPos(mark2ArrayPos)
	if err != nil {
		return nil, err
	}

	mark2Count, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if int(mark2Count) > len(mark2Cov) {
		mark2Count = uint16(len(mark2Cov))
	} else {
		mark2Cov.Prune(int(mark2Count))
	}
	numOffsets := uint(mark2Count) * uint(markClassCount)
	if numOffsets > (65536-6-2)/2 {
		// Offsets are 16-bit from mark2ArrayPos, and there must still be
		// space for at least one anchor table.
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    "GPOS6.1 table too large",
		}
	}
	offsets, err := p.Budget.AllocSlice[uint16](int(numOffsets))
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	mark2Array, err := p.Budget.AllocSlice[[]*anchor.Table](int(mark2Count))
	if err != nil {
		return nil, err
	}
	for i := range mark2Array {
		row, err := p.Budget.AllocSlice[*anchor.Table](markClassCount)
		if err != nil {
			return nil, err
		}
		for j := range row {
			if offsets[j] == 0 {
				continue
			}
			a, err := anchors.Read(mark2ArrayPos + int64(offsets[j]))
			if err != nil {
				return nil, err
			}
			row[j] = a
		}
		mark2Array[i] = row
		offsets = offsets[markClassCount:]
	}

	return &Gpos6_1{
		Mark1Cov:   mark1Cov,
		Mark2Cov:   mark2Cov,
		Mark1Array: mark1Array,
		Mark2Array: mark2Array,
	}, nil
}

// apply implements the [Subtable] interface.
func (l *Gpos6_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	mark1Idx, ok := l.Mark1Cov[seq[a].GID]
	if !ok {
		return -1
	}
	mark1Record := l.Mark1Array[mark1Idx]

	if a == 0 {
		return -1
	}
	p := a - 1
	var mark2Idx int
	for p >= 0 {
		mark2Idx, ok = l.Mark2Cov[seq[p].GID]
		if ok {
			break
		}
		p--
	}
	if p < 0 {
		return -1
	}
	mark2Record := l.Mark2Array[mark2Idx][mark1Record.Class]
	if mark2Record == nil {
		// TODO(voss): verify that this is what others do, too.
		return -1
	}

	dx := mark2Record.X - mark1Record.X
	dy := mark2Record.Y - mark1Record.Y
	for i := p; i < a; i++ {
		dx -= seq[i].Advance
	}
	seq[a].XOffset = dx
	seq[a].YOffset = dy
	return a + 1
}

// countMarkClasses returns the markClassCount for this subtable.  It also
// validates the structural invariants the encoder depends on: every row
// in Mark2Array must have width markClassCount, and every Mark1Array
// record's Class must be < markClassCount.  It panics
// if these are violated.
func (l *Gpos6_1) countMarkClasses() int {
	var count int
	if len(l.Mark2Array) > 0 {
		count = len(l.Mark2Array[0])
	} else {
		var maxClass uint16
		for _, rec := range l.Mark1Array {
			if rec.Class > maxClass {
				maxClass = rec.Class
			}
		}
		count = int(maxClass) + 1
	}
	for _, row := range l.Mark2Array {
		if len(row) != count {
			panic("Gpos6_1: inconsistent Mark2Array row width")
		}
	}
	for _, rec := range l.Mark1Array {
		if int(rec.Class) >= count {
			panic("Gpos6_1: mark class out of range")
		}
	}
	return count
}

// layout computes the encoded layout of the subtable.  The mark1 and mark2
// coverage tables are stored once if they are identical.  An error is
// returned if an offset does not fit into an Offset16.
func (l *Gpos6_1) layout() (*markAttach, error) {
	markClassCount := l.countMarkClasses()
	marks1, err := newMarkArray("Gpos6_1", l.Mark1Array)
	if err != nil {
		return nil, err
	}
	marks2, err := newAnchorMatrix("Gpos6_1", l.Mark2Array, markClassCount)
	if err != nil {
		return nil, err
	}
	return newMarkAttach("Gpos6_1", l.Mark1Cov, l.Mark2Cov, markClassCount, marks1, marks2)
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos6_1) encodeLen() int {
	return mustFit(l.layout()).size()
}

// encode implements the [Subtable] interface.
func (l *Gpos6_1) encode() []byte {
	lay := mustFit(l.layout())
	return lay.append(make([]byte, 0, lay.size()))
}

// fits implements the [Subtable] interface.
func (l *Gpos6_1) fits() error {
	_, err := l.layout()
	return err
}
