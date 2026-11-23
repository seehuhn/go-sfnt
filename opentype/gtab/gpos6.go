// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

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
	Mark1Cov coverage.Table
	Mark2Cov coverage.Table

	// Mark1Array is indexed by mark1 coverage index.  Each Class must be
	// less than the number of mark classes.
	Mark1Array []markarray.Record

	// Mark2Array is indexed by mark2 coverage index, then by mark class.
	// All rows must have the same length, which gives the number of mark
	// classes.  If Mark2Array is empty, the number of mark classes is one
	// more than the largest Class in Mark1Array.  Nil entries are absent.
	Mark2Array [][]*anchor.Table
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

	mark1Listing, err := coverage.ReadListing(p, subtablePos+mark1CoverageOffset)
	if err != nil {
		return nil, err
	}
	mark2Listing, err := coverage.ReadListing(p, subtablePos+mark2CoverageOffset)
	if err != nil {
		return nil, err
	}

	mark1Array, err := markarray.Read(p, anchors, subtablePos+mark1ArrayOffset, mark1Listing.DataLen())
	if err != nil {
		return nil, err
	}
	mark1Cov, mark1Array := coverage.Align(mark1Listing, mark1Array)
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
	mark2Count = uint16(min(int(mark2Count), mark2Listing.DataLen()))
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
	mark2Cov, mark2Array := coverage.Align(mark2Listing, mark2Array)

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

// countMarkClasses returns the markClassCount for this subtable.  The
// encoder requires every row in Mark2Array to have width markClassCount,
// and every Mark1Array record's Class to be less than markClassCount.
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

// layout implements the [Subtable] interface.  The mark1 and mark2
// coverage tables are stored once if they are identical.
func (l *Gpos6_1) layout() (subtableLayout, error) {
	if err := checkCoverage("Gpos6_1", l.Mark1Cov, len(l.Mark1Array)); err != nil {
		return nil, err
	}
	if err := checkCoverage("Gpos6_1", l.Mark2Cov, len(l.Mark2Array)); err != nil {
		return nil, err
	}
	markClassCount := l.countMarkClasses()
	marks1, err := newMarkArray("Gpos6_1", l.Mark1Array)
	if err != nil {
		return nil, err
	}
	marks2, err := newAnchorMatrix("Gpos6_1", l.Mark2Array, markClassCount)
	if err != nil {
		return nil, err
	}
	lay, err := newMarkAttach("Gpos6_1", l.Mark1Cov, l.Mark2Cov, markClassCount, marks1, marks2)
	if err != nil {
		return nil, err
	}
	return lay, nil
}
