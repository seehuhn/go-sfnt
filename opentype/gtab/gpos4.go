// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

// Gpos4_1 is a Mark-to-Base Attachment Positioning Subtable (format 1).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#mark-to-base-attachment-positioning-format-1-mark-to-base-attachment-point
type Gpos4_1 struct {
	MarkCov   coverage.Table
	BaseCov   coverage.Table
	MarkArray []markarray.Record // indexed by mark coverage index
	BaseArray [][]*anchor.Table  // indexed by base coverage index, then by mark class; nil entries are absent
}

func readGpos4_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(10)
	if err != nil {
		return nil, err
	}
	anchors := anchor.NewReader(p)

	markCoverageOffset := int64(buf[0])<<8 | int64(buf[1])
	baseCoverageOffset := int64(buf[2])<<8 | int64(buf[3])
	markClassCount := int(buf[4])<<8 | int(buf[5])
	markArrayOffset := int64(buf[6])<<8 | int64(buf[7])
	baseArrayOffset := int64(buf[8])<<8 | int64(buf[9])

	markCov, err := coverage.Read(p, subtablePos+markCoverageOffset)
	if err != nil {
		return nil, err
	}
	baseCov, err := coverage.Read(p, subtablePos+baseCoverageOffset)
	if err != nil {
		return nil, err
	}

	markArray, err := markarray.Read(p, anchors, subtablePos+markArrayOffset, len(markCov))
	if err != nil {
		return nil, err
	}
	if len(markCov) > len(markArray) {
		markCov.Prune(len(markArray))
	} else {
		markArray = markArray[:len(markCov)]
	}
	for _, rec := range markArray {
		if int(rec.Class) >= markClassCount {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "mark class out of range",
			}
		}
	}

	baseArrayPos := subtablePos + baseArrayOffset
	err = p.SeekPos(baseArrayPos)
	if err != nil {
		return nil, err
	}

	baseCount, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if int(baseCount) > len(baseCov) {
		baseCount = uint16(len(baseCov))
	} else {
		baseCov.Prune(int(baseCount))
	}
	numOffsets := uint(baseCount) * uint(markClassCount)
	if numOffsets > (65536-6-2)/2 {
		// Offsets are 16-bit from baseArrayPos, and there must still be
		// space for at least one anchor table.
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    "GPOS4.1 table too large",
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

	baseArray, err := p.Budget.AllocSlice[[]*anchor.Table](int(baseCount))
	if err != nil {
		return nil, err
	}
	for i := range baseArray {
		row, err := p.Budget.AllocSlice[*anchor.Table](markClassCount)
		if err != nil {
			return nil, err
		}
		for j := range row {
			if offsets[j] == 0 {
				continue
			}
			a, err := anchors.Read(baseArrayPos + int64(offsets[j]))
			if err != nil {
				return nil, err
			}
			row[j] = a
		}
		baseArray[i] = row
		offsets = offsets[markClassCount:]
	}

	return &Gpos4_1{
		MarkCov:   markCov,
		BaseCov:   baseCov,
		MarkArray: markArray,
		BaseArray: baseArray,
	}, nil
}

// apply implements the [Subtable] interface.
func (l *Gpos4_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq

	// TODO(voss): does this apply to the base or the mark?
	markIdx, ok := l.MarkCov[seq[a].GID]
	if !ok {
		return -1
	}
	markRecord := l.MarkArray[markIdx]

	if a == 0 {
		return -1
	}
	p := a - 1
	var baseIdx int
	for p >= 0 {
		baseIdx, ok = l.BaseCov[seq[p].GID]
		if ok {
			break
		}
		p--
	}
	if p < 0 {
		return -1
	}
	baseRecord := l.BaseArray[baseIdx][markRecord.Class]
	if baseRecord == nil {
		// TODO(voss): verify that this is what others do, too.
		return -1
	}

	dx := baseRecord.X - markRecord.X
	dy := baseRecord.Y - markRecord.Y
	for i := p; i < a; i++ {
		dx -= seq[i].Advance
	}
	seq[a].XOffset = dx
	seq[a].YOffset = dy
	return a + 1
}

// countMarkClasses returns the markClassCount for this subtable.  It also
// validates the structural invariants the encoder depends on: every row in
// BaseArray must have width markClassCount, and every MarkArray record's
// Class must be < markClassCount.  It panics
// if these are violated.
func (l *Gpos4_1) countMarkClasses() int {
	var count int
	if len(l.BaseArray) > 0 {
		count = len(l.BaseArray[0])
	} else {
		var maxClass uint16
		for _, rec := range l.MarkArray {
			if rec.Class > maxClass {
				maxClass = rec.Class
			}
		}
		count = int(maxClass) + 1
	}
	for _, row := range l.BaseArray {
		if len(row) != count {
			panic("Gpos4_1: inconsistent BaseArray row width")
		}
	}
	for _, rec := range l.MarkArray {
		if int(rec.Class) >= count {
			panic("Gpos4_1: mark class out of range")
		}
	}
	return count
}

// layout computes the encoded layout of the subtable.  The mark and base
// coverage tables are stored once if they are identical.  An error is returned
// if an offset does not fit into an Offset16.
func (l *Gpos4_1) layout() (*markAttach, error) {
	markClassCount := l.countMarkClasses()
	marks, err := newMarkArray("Gpos4_1", l.MarkArray)
	if err != nil {
		return nil, err
	}
	bases, err := newAnchorMatrix("Gpos4_1", l.BaseArray, markClassCount)
	if err != nil {
		return nil, err
	}
	return newMarkAttach("Gpos4_1", l.MarkCov, l.BaseCov, markClassCount, marks, bases)
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos4_1) encodeLen() int {
	return mustFit(l.layout()).size()
}

// encode implements the [Subtable] interface.
func (l *Gpos4_1) encode() []byte {
	lay := mustFit(l.layout())
	return lay.append(make([]byte, 0, lay.size()))
}

// fits implements the [Subtable] interface.
func (l *Gpos4_1) fits() error {
	_, err := l.layout()
	return err
}
