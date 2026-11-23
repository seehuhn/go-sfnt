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
	MarkCov coverage.Table
	BaseCov coverage.Table

	// MarkArray is indexed by mark coverage index.  Each Class must be
	// less than the number of mark classes.
	MarkArray []markarray.Record

	// BaseArray is indexed by base coverage index, then by mark class.
	// All rows must have the same length, which gives the number of mark
	// classes.  If BaseArray is empty, the number of mark classes is one
	// more than the largest Class in MarkArray.  Nil entries are absent.
	BaseArray [][]*anchor.Table
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

	markListing, err := coverage.ReadListing(p, subtablePos+markCoverageOffset)
	if err != nil {
		return nil, err
	}
	baseListing, err := coverage.ReadListing(p, subtablePos+baseCoverageOffset)
	if err != nil {
		return nil, err
	}

	markArray, err := markarray.Read(p, anchors, subtablePos+markArrayOffset, markListing.DataLen())
	if err != nil {
		return nil, err
	}
	markCov, markArray := coverage.Align(markListing, markArray)
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
	baseCount = uint16(min(int(baseCount), baseListing.DataLen()))
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
	baseCov, baseArray := coverage.Align(baseListing, baseArray)

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

// countMarkClasses returns the markClassCount for this subtable.  The
// encoder requires every row in BaseArray to have width markClassCount,
// and every MarkArray record's Class to be less than markClassCount.
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

// layout implements the [Subtable] interface.  The mark and base coverage
// tables are stored once if they are identical.
func (l *Gpos4_1) layout() (subtableLayout, error) {
	if err := checkCoverage("Gpos4_1", l.MarkCov, len(l.MarkArray)); err != nil {
		return nil, err
	}
	if err := checkCoverage("Gpos4_1", l.BaseCov, len(l.BaseArray)); err != nil {
		return nil, err
	}
	markClassCount := l.countMarkClasses()
	marks, err := newMarkArray("Gpos4_1", l.MarkArray)
	if err != nil {
		return nil, err
	}
	bases, err := newAnchorMatrix("Gpos4_1", l.BaseArray, markClassCount)
	if err != nil {
		return nil, err
	}
	lay, err := newMarkAttach("Gpos4_1", l.MarkCov, l.BaseCov, markClassCount, marks, bases)
	if err != nil {
		return nil, err
	}
	return lay, nil
}
