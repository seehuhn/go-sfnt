// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package markarray contains types for representing OpenType mark array tables.
package markarray

import (
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/parser"
)

// Record is a mark record in a Mark Array Table.
// Each mark record defines the class of the mark and its anchor point.
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#mark-array-table
type Record struct {
	Class uint16
	anchor.Table
}

// Read reads a Mark Array Table from the given parser.
// If there are more than numMarks entries in the table, the remaining entries
// are ignored.  Anchor tables are read via anchors, so that they can be
// shared with other structures read by the same Reader.
func Read(p *parser.Parser, anchors *anchor.Reader, pos int64, numMarks int) ([]Record, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	markCount, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if int(markCount) > numMarks {
		markCount = uint16(numMarks)
	}

	res, err := p.Budget.AllocSlice[Record](int(markCount))
	if err != nil {
		return nil, err
	}
	offsets, err := p.Budget.AllocSlice[uint16](int(markCount))
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(markCount); i++ {
		res[i].Class, err = p.ReadUint16()
		if err != nil {
			return nil, err
		}

		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	for i, offs := range offsets {
		a, err := anchors.Read(pos + int64(offs))
		if err != nil {
			return nil, err
		}
		res[i].Table = *a
	}
	return res, nil
}
