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

// Package gdef reads and writes OpenType "GDEF" tables.
package gdef

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"slices"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// Table contains the parsed GDEF table.
// https://docs.microsoft.com/en-us/typography/opentype/spec/GDEF
type Table struct {
	GlyphClass      classdef.Table // class definition table for glyph type
	AttachList      *AttachList    // attachment point list table, or nil
	LigCaretList    *LigCaretList  // ligature caret list table, or nil
	MarkAttachClass classdef.Table // class definition table for mark attachment type
	MarkGlyphSets   []coverage.Set // table of mark glyph set definitions

	// ItemVarStore holds the shared delta values referenced by
	// device/VariationIndex tables elsewhere in the font, or nil if
	// absent (GDEF 1.3).
	ItemVarStore *variation.ItemVariationStore
}

// IsMark returns true if it is known that the glyph represents a mark character.
func (table *Table) IsMark(gid glyph.ID) bool {
	if table == nil || table.GlyphClass == nil {
		return false
	}
	return table.GlyphClass[gid] == GlyphClassMark
}

// Read reads the GDEF table from r.  Allocations are charged against budget.
func Read(r parser.ReadSeekSizer, budget *membudget.Budget) (*Table, error) {
	p := parser.New(r, budget)
	buf, err := p.ReadBytes(12)
	if err != nil {
		return nil, err
	}
	majorVersion := uint16(buf[0])<<8 | uint16(buf[1])
	minorVersion := uint16(buf[2])<<8 | uint16(buf[3])
	if majorVersion != 1 || (minorVersion != 0 && minorVersion != 2 && minorVersion != 3) {
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/opentype/gdef",
			Feature:   fmt.Sprintf("GDEF table version %d.%d", majorVersion, minorVersion),
		}
	}
	glyphClassDefOffset := uint16(buf[4])<<8 | uint16(buf[5])
	attachListOffset := uint16(buf[6])<<8 | uint16(buf[7])
	ligCaretListOffset := uint16(buf[8])<<8 | uint16(buf[9])
	markAttachClassDefOffset := uint16(buf[10])<<8 | uint16(buf[11])
	var markGlyphSetsDefOffset uint16
	if minorVersion >= 2 {
		markGlyphSetsDefOffset, err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}
	var itemVarStoreOffset uint32
	if minorVersion >= 3 {
		itemVarStoreOffset, err = p.ReadUint32()
		if err != nil {
			return nil, err
		}
	}

	table := &Table{}

	if glyphClassDefOffset != 0 {
		table.GlyphClass, err = classdef.Read(p, int64(glyphClassDefOffset))
		if err != nil {
			return nil, err
		}
	}

	var lists listLayouts
	if attachListOffset != 0 {
		table.AttachList, err = readAttachList(p, int64(attachListOffset))
		if err != nil {
			return nil, err
		}
		// an optional list which cannot be written is dropped
		lists.attach, err = table.AttachList.layout()
		if err != nil {
			table.AttachList = nil
			lists.attach = nil
		}
	}
	if ligCaretListOffset != 0 {
		table.LigCaretList, err = readLigCaretList(p, int64(ligCaretListOffset))
		if err != nil {
			return nil, err
		}
		// an optional list which cannot be written is dropped
		lists.ligCaret, err = table.LigCaretList.layout()
		if err != nil {
			table.LigCaretList = nil
			lists.ligCaret = nil
		}
	}

	if markAttachClassDefOffset != 0 {
		table.MarkAttachClass, err = classdef.Read(p, int64(markAttachClassDefOffset))
		if err != nil {
			return nil, err
		}
	}

	if markGlyphSetsDefOffset != 0 {
		pos := int64(markGlyphSetsDefOffset)
		err = p.SeekPos(pos)
		if err != nil {
			return nil, err
		}
		buf, err := p.ReadBytes(4)
		if err != nil {
			return nil, err
		}
		format := uint16(buf[0])<<8 | uint16(buf[1])
		if format != 1 {
			return nil, &parser.NotSupportedError{
				SubSystem: "sfnt/opentype/gdef",
				Feature:   fmt.Sprintf("mark glyph sets format %d", format),
			}
		}
		markGlyphSetCount := uint16(buf[2])<<8 | uint16(buf[3])
		coverageOffsets, err := p.Budget.AllocSlice[uint32](int(markGlyphSetCount))
		if err != nil {
			return nil, err
		}
		for i := range coverageOffsets {
			coverageOffsets[i], err = p.ReadUint32()
			if err != nil {
				return nil, err
			}
		}

		table.MarkGlyphSets, err = p.Budget.AllocSlice[coverage.Set](int(markGlyphSetCount))
		if err != nil {
			return nil, err
		}
		for i := range table.MarkGlyphSets {
			table.MarkGlyphSets[i], err = coverage.ReadSet(p, pos+int64(coverageOffsets[i]))
			if err != nil {
				return nil, err
			}
		}
	}

	if itemVarStoreOffset != 0 {
		table.ItemVarStore, err = variation.ReadItemVariationStore(p, int64(itemVarStoreOffset))
		if err != nil {
			return nil, err
		}
	}

	// optional tables which cannot be written back are dropped
	table.dropUnreachable(lists)

	return table, nil
}

// headerLen returns the length and the version of the GDEF header.  Version
// 1.2 adds the mark glyph sets offset, version 1.3 the item variation store
// offset.
func (table *Table) headerLen() (int, uint32) {
	switch {
	case table.ItemVarStore != nil:
		return 18, 0x00010003
	case table.MarkGlyphSets != nil:
		return 14, 0x00010002
	default:
		return 12, 0x00010000
	}
}

// subtable is a table reached through an Offset16 field of the GDEF header.
type subtable struct {
	size   int // encoded length, or -1 if absent
	append func([]byte) []byte
}

// listLayouts holds the encoded layouts of the attachment point list and
// the ligature caret list.  The layout of an absent list is nil.
type listLayouts struct {
	attach   *attachListLayout
	ligCaret *ligCaretListLayout
}

// layoutLists computes the layouts of the two lists, and panics if a list
// cannot be encoded.
func (table *Table) layoutLists() listLayouts {
	var lists listLayouts
	if table.AttachList != nil {
		lists.attach = mustFit(table.AttachList.layout())
	}
	if table.LigCaretList != nil {
		lists.ligCaret = mustFit(table.LigCaretList.layout())
	}
	return lists
}

// subtables returns the tables reached through Offset16 fields of the
// header, in the order of the header fields.
func (table *Table) subtables(lists listLayouts) []subtable {
	res := make([]subtable, 5)
	for i := range res {
		res[i].size = -1
	}
	if cd := table.GlyphClass; cd != nil {
		res[0] = subtable{cd.AppendLen(), cd.Append}
	}
	if lay := lists.attach; lay != nil {
		res[1] = subtable{lay.encodeLen(), lay.append}
	}
	if lay := lists.ligCaret; lay != nil {
		res[2] = subtable{lay.encodeLen(), lay.append}
	}
	if cd := table.MarkAttachClass; cd != nil {
		res[3] = subtable{cd.AppendLen(), cd.Append}
	}
	if sets := table.MarkGlyphSets; sets != nil {
		size := 4 + 4*len(sets)
		for _, set := range sets {
			size += set.ToTable().EncodeLen()
		}
		res[4] = subtable{size, func(buf []byte) []byte {
			return appendMarkGlyphSets(buf, sets)
		}}
	}
	return res
}

// appendMarkGlyphSets appends a MarkGlyphSets table to buf.
func appendMarkGlyphSets(buf []byte, sets []coverage.Set) []byte {
	n := len(sets)
	buf = append(buf,
		0, 1, // format
		byte(n>>8), byte(n))
	covs := make([]coverage.Table, n)
	offs := 4 + 4*n
	for i, set := range sets {
		covs[i] = set.ToTable()
		buf = append(buf,
			byte(offs>>24), byte(offs>>16), byte(offs>>8), byte(offs))
		offs += covs[i].EncodeLen()
	}
	for _, cov := range covs {
		buf = append(buf, cov.Encode()...)
	}
	return buf
}

// sizes returns the encoded lengths of the subtables.
func sizes(subtables []subtable) []int {
	res := make([]int, len(subtables))
	for i, st := range subtables {
		res[i] = st.size
	}
	return res
}

// dropUnreachable drops tables which cannot be reached through the Offset16
// fields of the encoded header.  The tables are considered in the order of
// the header fields, and each is kept if it can be stored together with the
// tables kept before it.
func (table *Table) dropUnreachable(lists listLayouts) {
	hdr, _ := table.headerLen()
	all := sizes(table.subtables(lists))
	kept := make([]int, len(all))
	for i := range kept {
		kept[i] = -1
	}
	for i, size := range all {
		if size < 0 {
			continue
		}
		kept[i] = size
		if _, ok := offsetpool.Place(hdr, kept); ok {
			continue
		}
		kept[i] = -1
		switch i {
		case 0:
			table.GlyphClass = nil
		case 1:
			table.AttachList = nil
		case 2:
			table.LigCaretList = nil
		case 3:
			table.MarkAttachClass = nil
		case 4:
			table.MarkGlyphSets = nil
		}
	}
}

// Encode converts the GDEF table to its binary form.
func (table *Table) Encode() []byte {
	hdr, version := table.headerLen()
	subtables := table.subtables(table.layoutLists())
	offs, ok := offsetpool.Place(hdr, sizes(subtables))
	if !ok {
		panic("sfnt/gdef: tables too large for GDEF header")
	}

	total := hdr
	for _, st := range subtables {
		total += max(st.size, 0)
	}
	itemVarStoreOffset := total
	if table.ItemVarStore != nil {
		total += table.ItemVarStore.EncodeLen()
	}

	buf := make([]byte, hdr, total)
	binary.BigEndian.PutUint32(buf, version)
	// the version 1.0 header has no MarkGlyphSets field
	for i, off := range offs[:min(len(offs), (hdr-4)/2)] {
		binary.BigEndian.PutUint16(buf[4+2*i:], uint16(off))
	}
	if table.ItemVarStore != nil {
		binary.BigEndian.PutUint32(buf[14:], uint32(itemVarStoreOffset))
	}

	// the subtables in order of increasing offset
	order := make([]int, 0, len(subtables))
	for i, st := range subtables {
		if st.size >= 0 {
			order = append(order, i)
		}
	}
	slices.SortFunc(order, func(i, j int) int { return cmp.Compare(offs[i], offs[j]) })
	for _, i := range order {
		buf = subtables[i].append(buf)
	}

	if table.ItemVarStore != nil {
		buf = append(buf, table.ItemVarStore.Encode()...)
	}
	return buf
}

// mustFit returns the layout of a table and panics if the table cannot be
// encoded.  The encoders use it because their input comes from the API,
// which is trusted.
func mustFit[T any](lay T, err error) T {
	if err != nil {
		panic("sfnt/gdef: " + err.Error())
	}
	return lay
}

// Possible values for the GlyphClass field.
// https://docs.microsoft.com/en-us/typography/opentype/spec/GDEF#glyph-class-definition-table
const (
	GlyphClassBase      = 1
	GlyphClassLigature  = 2
	GlyphClassMark      = 3
	GlyphClassComponent = 4
)
