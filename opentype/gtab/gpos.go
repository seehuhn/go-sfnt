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
	"fmt"
	"maps"
	"slices"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/parser"
)

// checkSubtableSize16 panics if total exceeds the uint16 range.
//
// All internal offsets in a GPOS or GSUB subtable (coverage tables,
// classdefs, mark/base/lig array offsets, Device-table offsets, …) are
// stored as Offset16 values relative to the subtable start.  Once the
// laid-out subtable grows past 0xFFFF bytes, those offsets truncate
// silently and the encoded bytes point at the wrong data.  Callers must
// split the subtable themselves; the library has no way to do it for
// them.
func checkSubtableSize16(name string, total int) {
	if total > 0xFFFF {
		panic(fmt.Sprintf("sfnt/gtab: %s subtable too large (%d bytes); "+
			"split the lookup so each subtable fits in 64 KiB", name, total))
	}
}

// readGposSubtable reads a GPOS subtable.
// This function can be used as the SubtableReader argument to readLookupList().
func readGposSubtable(p *parser.Parser, pos int64, meta *LookupMetaInfo) (Subtable, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	format, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}

	reader, ok := gposReaders[10*meta.LookupType+format]
	if !ok {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason: fmt.Sprintf("unknown GPOS subtable format %d.%d",
				meta.LookupType, format),
		}
	}
	return reader(p, pos)
}

// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#gsubLookupTypeEnum
var gposReaders = map[uint16]func(p *parser.Parser, pos int64) (Subtable, error){
	1_1: readGpos1_1,
	1_2: readGpos1_2,
	2_1: readGpos2_1,
	2_2: readGpos2_2,
	3_1: readGpos3_1,
	4_1: readGpos4_1,
	5_1: readGpos5_1,
	6_1: readGpos6_1,
	7_1: readSeqContext1,
	7_2: readSeqContext2,
	7_3: readSeqContext3,
	8_1: readChainedSeqContext1,
	8_2: readChainedSeqContext2,
	8_3: readChainedSeqContext3,
	9_1: readExtensionSubtable,
}

// Gpos1_1 is a Single Adjustment Positioning Subtable (GPOS type 1, format 1).
// If specifies a single adjustment to be applied to all glyphs in the
// coverage table.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#single-adjustment-positioning-format-1-single-positioning-value
type Gpos1_1 struct {
	Cov    coverage.Table
	Adjust *GposValueRecord
}

func readGpos1_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	valueFormat := uint16(buf[2])<<8 | uint16(buf[3])
	valueRecord, err := readValueRecord(p, valueFormat, subtablePos)
	if err != nil {
		return nil, err
	}
	cov, err := coverage.Read(p, subtablePos+coverageOffset)
	if err != nil {
		return nil, err
	}
	res := &Gpos1_1{
		Cov:    cov,
		Adjust: valueRecord,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *Gpos1_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq

	_, ok := l.Cov[seq[a].GID]
	if !ok {
		return -1
	}

	l.Adjust.Apply(&seq[a])
	return a + 1
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos1_1) encodeLen() int {
	format := l.Adjust.getFormat()
	devStart := 6 + valueRecordEncodeLen(format) + l.Cov.EncodeLen()
	pool := newDevicePool(devStart)
	pool.addAll(l.Adjust)
	return devStart + pool.len()
}

// encode implements the [Subtable] interface.
func (l *Gpos1_1) encode() []byte {
	format := l.Adjust.getFormat()
	coverageOffs := 6 + valueRecordEncodeLen(format)
	devStart := coverageOffs + l.Cov.EncodeLen()

	pool := newDevicePool(devStart)
	var devOffsets [4]uint16
	for i, d := range l.Adjust.deviceTables() {
		devOffsets[i] = pool.add(d)
	}
	total := devStart + pool.len()
	checkSubtableSize16("Gpos1_1", total)

	buf := make([]byte, 0, total)
	buf = append(buf,
		0, 1, // format
		byte(coverageOffs>>8), byte(coverageOffs),
		byte(format>>8), byte(format),
	)
	buf = append(buf, l.Adjust.encode(format, devOffsets)...)
	buf = append(buf, l.Cov.Encode()...)
	buf = append(buf, pool.bytes()...)
	return buf
}

// Gpos1_2 is a Single Adjustment Positioning Subtable (GPOS type 1, format 2).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#single-adjustment-positioning-format-2-array-of-positioning-values
type Gpos1_2 struct {
	Cov    coverage.Table
	Adjust []*GposValueRecord // indexed by coverage index
}

func readGpos1_2(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(6)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	valueFormat := uint16(buf[2])<<8 | uint16(buf[3])
	valueCount := int(buf[4])<<8 | int(buf[5])
	valueRecords, err := membudget.AllocSlice[*GposValueRecord](p.Budget, valueCount)
	if err != nil {
		return nil, err
	}
	for i := range valueRecords {
		valueRecords[i], err = readValueRecord(p, valueFormat, subtablePos)
		if err != nil {
			return nil, err
		}
	}
	cov, err := coverage.Read(p, subtablePos+coverageOffset)
	if err != nil {
		return nil, err
	}

	if len(valueRecords) > len(cov) {
		valueRecords = valueRecords[:len(cov)]
	} else if len(valueRecords) < len(cov) {
		cov.Prune(len(valueRecords))
	}

	res := &Gpos1_2{
		Cov:    cov,
		Adjust: valueRecords,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *Gpos1_2) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	idx, ok := l.Cov[seq[a].GID]
	if !ok {
		return -1
	}
	l.Adjust[idx].Apply(&seq[a])
	return a + 1
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos1_2) encodeLen() int {
	var valueFormat uint16
	for _, adj := range l.Adjust {
		valueFormat |= adj.getFormat()
	}
	devStart := 8 + valueRecordEncodeLen(valueFormat)*len(l.Adjust) + l.Cov.EncodeLen()
	pool := newDevicePool(devStart)
	pool.addAll(l.Adjust...)
	return devStart + pool.len()
}

// encode implements the [Subtable] interface.
func (l *Gpos1_2) encode() []byte {
	var valueFormat uint16
	for _, adj := range l.Adjust {
		valueFormat |= adj.getFormat()
	}
	valueCount := len(l.Adjust)
	vrLen := valueRecordEncodeLen(valueFormat)

	coverageOffset := 8 + vrLen*valueCount
	devStart := coverageOffset + l.Cov.EncodeLen()

	pool := newDevicePool(devStart)
	perRecOffs := make([][4]uint16, valueCount)
	for i, adj := range l.Adjust {
		for k, d := range adj.deviceTables() {
			perRecOffs[i][k] = pool.add(d)
		}
	}
	total := devStart + pool.len()
	checkSubtableSize16("Gpos1_2", total)

	buf := make([]byte, 0, total)
	buf = append(buf,
		0, 2, // format
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(valueFormat>>8), byte(valueFormat),
		byte(valueCount>>8), byte(valueCount),
	)
	for i, adj := range l.Adjust {
		buf = append(buf, adj.encode(valueFormat, perRecOffs[i])...)
	}
	buf = append(buf, l.Cov.Encode()...)
	buf = append(buf, pool.bytes()...)
	return buf
}

// Gpos2_1 is a Pair Adjustment Positioning Subtable (format 1).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#pair-adjustment-positioning-format-1-adjustments-for-glyph-pairs
type Gpos2_1 map[glyph.Pair]*PairAdjust

// PairAdjust represents information from a PairValueRecord table.
//
// This is used in [Gpos2_1] and [Gpos2_2] subtables.
type PairAdjust struct {
	First, Second *GposValueRecord
}

// apply implements the [Subtable] interface.
func (l Gpos2_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	p := a + 1
	for p < b && !keep.Keep(seq[p].GID) {
		p++
	}
	if p >= b {
		return -1
	}

	g1 := seq[a]
	g2 := seq[p]
	adj, ok := l[glyph.Pair{Left: g1.GID, Right: g2.GID}]
	if !ok {
		return -1
	}

	adj.First.Apply(&seq[a])
	if adj.Second == nil {
		return p
	}
	adj.Second.Apply(&seq[p])
	return p + 1
}

func readGpos2_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(8)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	valueFormat1 := uint16(buf[2])<<8 | uint16(buf[3])
	valueFormat2 := uint16(buf[4])<<8 | uint16(buf[5])
	pairSetCount := int(buf[6])<<8 | int(buf[7])

	pairSetOffsets, err := membudget.AllocSlice[uint16](p.Budget, pairSetCount)
	if err != nil {
		return nil, err
	}
	for i := range pairSetOffsets {
		pairSetOffsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	cov, err := coverage.Read(p, subtablePos+coverageOffset)
	if err != nil {
		return nil, err
	}

	if len(pairSetOffsets) > len(cov) {
		pairSetOffsets = pairSetOffsets[:len(cov)]
	} else if len(pairSetOffsets) < len(cov) {
		cov.Prune(len(pairSetOffsets))
	}

	adjust, err := membudget.AllocSlice[map[glyph.ID]*PairAdjust](p.Budget, len(pairSetOffsets))
	if err != nil {
		return nil, err
	}
	for i, offset := range pairSetOffsets {
		err = p.SeekPos(subtablePos + int64(offset))
		if err != nil {
			return nil, err
		}
		pairValueCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		// charge the map: rough estimate ~16 bytes per entry plus
		// bucket overhead.  A flat 24 bytes per entry is generous.
		if err := p.Budget.Charge(int(pairValueCount) * 24); err != nil {
			return nil, err
		}
		adj := make(map[glyph.ID]*PairAdjust, pairValueCount)
		for j := 0; j < int(pairValueCount); j++ {
			secondGlyph, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			first, err := readValueRecord(p, valueFormat1, subtablePos)
			if err != nil {
				return nil, err
			}
			second, err := readValueRecord(p, valueFormat2, subtablePos)
			if err != nil {
				return nil, err
			}
			adj[glyph.ID(secondGlyph)] = &PairAdjust{
				First:  first,
				Second: second,
			}
		}
		adjust[i] = adj
	}

	res := Gpos2_1{}
	for first, i := range cov {
		for second, a := range adjust[i] {
			res[glyph.Pair{Left: first, Right: second}] = a
		}
	}
	return res, nil
}

// CovAndAdjust is a convenience function which returns the coverage table and
// the adjustments.
func (l Gpos2_1) CovAndAdjust() (coverage.Table, []map[glyph.ID]*PairAdjust) {
	seen := make(map[glyph.ID]bool)
	for pair := range l {
		seen[pair.Left] = true
	}

	firstGids := slices.Sorted(maps.Keys(seen))
	cov := coverage.Table{}
	adjust := make([]map[glyph.ID]*PairAdjust, len(firstGids))
	for i, gid := range firstGids {
		cov[gid] = i
		adjust[i] = map[glyph.ID]*PairAdjust{}
	}

	for pair := range l {
		adjust[cov[pair.Left]][pair.Right] = l[pair]
	}

	return cov, adjust
}

// encodeLen implements the [Subtable] interface.
func (l Gpos2_1) encodeLen() int {
	cov, adjust := l.CovAndAdjust()

	var valueFormat1, valueFormat2 uint16
	for _, adj := range adjust {
		for _, v := range adj {
			valueFormat1 |= v.First.getFormat()
			valueFormat2 |= v.Second.getFormat()
		}
	}
	vr1Len := valueRecordEncodeLen(valueFormat1)
	vr2Len := valueRecordEncodeLen(valueFormat2)

	devStart := 10 + 2*len(adjust) + cov.EncodeLen()
	for _, adj := range adjust {
		devStart += 2 + (2+vr1Len+vr2Len)*len(adj)
	}
	pool := newDevicePool(devStart)
	for _, adj := range adjust {
		for _, v := range adj {
			pool.addAll(v.First, v.Second)
		}
	}
	return devStart + pool.len()
}

// encode implements the [Subtable] interface.
func (l Gpos2_1) encode() []byte {
	cov, adjust := l.CovAndAdjust()

	pairSetCount := len(adjust)
	var valueFormat1, valueFormat2 uint16
	for _, adj := range adjust {
		for _, v := range adj {
			valueFormat1 |= v.First.getFormat()
			valueFormat2 |= v.Second.getFormat()
		}
	}
	vr1Len := valueRecordEncodeLen(valueFormat1)
	vr2Len := valueRecordEncodeLen(valueFormat2)

	headerLen := 10 + 2*pairSetCount
	coverageOffset := headerLen
	covLen := cov.EncodeLen()
	pairSetOffsets := make([]uint16, pairSetCount)
	pos := coverageOffset + covLen
	pairKeys := make([][]glyph.ID, pairSetCount)
	for i, adj := range adjust {
		pairSetOffsets[i] = uint16(pos)
		pos += 2 + (2+vr1Len+vr2Len)*len(adj)
		pairKeys[i] = slices.Sorted(maps.Keys(adj))
	}
	devStart := pos
	pool := newDevicePool(devStart)
	type devOff struct {
		first, second [4]uint16
	}
	pairDev := make([][]devOff, pairSetCount)
	for i, adj := range adjust {
		row := make([]devOff, len(pairKeys[i]))
		for j, g := range pairKeys[i] {
			v := adj[g]
			for k, d := range v.First.deviceTables() {
				row[j].first[k] = pool.add(d)
			}
			for k, d := range v.Second.deviceTables() {
				row[j].second[k] = pool.add(d)
			}
		}
		pairDev[i] = row
	}
	total := devStart + pool.len()
	checkSubtableSize16("Gpos2_1", total)

	buf := make([]byte, 0, total)
	buf = append(buf,
		0, 1, // format
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(valueFormat1>>8), byte(valueFormat1),
		byte(valueFormat2>>8), byte(valueFormat2),
		byte(pairSetCount>>8), byte(pairSetCount),
	)
	for _, offset := range pairSetOffsets {
		buf = append(buf, byte(offset>>8), byte(offset))
	}

	buf = append(buf, cov.Encode()...)

	for i, adj := range adjust {
		pairValueCount := len(adj)
		buf = append(buf, byte(pairValueCount>>8), byte(pairValueCount))

		for j, secondGlyph := range pairKeys[i] {
			v := adj[secondGlyph]
			buf = append(buf, byte(secondGlyph>>8), byte(secondGlyph))
			buf = append(buf, v.First.encode(valueFormat1, pairDev[i][j].first)...)
			buf = append(buf, v.Second.encode(valueFormat2, pairDev[i][j].second)...)
		}
	}
	buf = append(buf, pool.bytes()...)

	return buf
}

// Gpos2_2 is a Pair Adjustment Positioning Subtable (format 2).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#pair-adjustment-positioning-format-2-class-pair-adjustment
type Gpos2_2 struct {
	Cov            coverage.Set
	Class1, Class2 classdef.Table
	Adjust         [][]*PairAdjust // indexed by class1 index, then class2 index
}

// apply implements the [Subtable] interface.
func (l *Gpos2_2) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	g1 := seq[a]
	_, ok := l.Cov[g1.GID]
	if !ok {
		return -1
	}

	p := a + 1
	for p < b && !keep.Keep(seq[p].GID) {
		p++
	}
	if p >= b {
		return -1
	}
	g2 := seq[p]

	class1 := l.Class1[g1.GID]
	if int(class1) >= len(l.Adjust) {
		return -1
	}
	row := l.Adjust[class1]
	class2 := l.Class2[g2.GID]
	if int(class2) >= len(row) {
		return -1
	}
	adj := row[class2]

	adj.First.Apply(&seq[a])
	if adj.Second == nil {
		return p
	}
	adj.Second.Apply(&seq[p])
	return p + 1
}

func readGpos2_2(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(14)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	valueFormat1 := uint16(buf[2])<<8 | uint16(buf[3])
	valueFormat2 := uint16(buf[4])<<8 | uint16(buf[5])
	classDef1Offset := int64(buf[6])<<8 | int64(buf[7])
	classDef2Offset := int64(buf[8])<<8 | int64(buf[9])
	class1Count := uint16(buf[10])<<8 | uint16(buf[11])
	class2Count := uint16(buf[12])<<8 | uint16(buf[13])

	numRecords := int(class1Count) * int(class2Count)
	if numRecords >= 65536 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    "GPOS2.1 table too large",
		}
	}
	records, err := membudget.AllocSlice[*PairAdjust](p.Budget, numRecords)
	if err != nil {
		return nil, err
	}
	for i := range numRecords {
		first, err := readValueRecord(p, valueFormat1, subtablePos)
		if err != nil {
			return nil, err
		}
		second, err := readValueRecord(p, valueFormat2, subtablePos)
		if err != nil {
			return nil, err
		}
		records[i] = &PairAdjust{
			First:  first,
			Second: second,
		}
	}

	cov, err := coverage.ReadSet(p, subtablePos+coverageOffset)
	if err != nil {
		return nil, err
	}

	classDef1, err := classdef.Read(p, subtablePos+classDef1Offset)
	if err != nil {
		return nil, err
	}
	classDef2, err := classdef.Read(p, subtablePos+classDef2Offset)
	if err != nil {
		return nil, err
	}

	adjust, err := membudget.AllocSlice[[]*PairAdjust](p.Budget, int(class1Count))
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(class1Count); i++ {
		adjust[i] = records[i*int(class2Count) : (i+1)*int(class2Count)]
	}

	return &Gpos2_2{
		Cov:    cov,
		Class1: classDef1,
		Class2: classDef2,
		Adjust: adjust,
	}, nil
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos2_2) encodeLen() int {
	var valueFormat1, valueFormat2 uint16
	for _, adj := range l.Adjust {
		for _, v := range adj {
			valueFormat1 |= v.First.getFormat()
			valueFormat2 |= v.Second.getFormat()
		}
	}
	recLen := valueRecordEncodeLen(valueFormat1) + valueRecordEncodeLen(valueFormat2)

	class1Count := len(l.Adjust)
	var class2Count int
	if class1Count > 0 {
		class2Count = len(l.Adjust[0])
	}

	devStart := 16 + class1Count*class2Count*recLen
	devStart += l.Cov.ToTable().EncodeLen()
	devStart += l.Class1.AppendLen()
	devStart += l.Class2.AppendLen()
	pool := newDevicePool(devStart)
	for _, row := range l.Adjust {
		for _, adj := range row {
			pool.addAll(adj.First, adj.Second)
		}
	}
	return devStart + pool.len()
}

// encode implements the [Subtable] interface.
func (l *Gpos2_2) encode() []byte {
	var valueFormat1, valueFormat2 uint16
	for _, adj := range l.Adjust {
		for _, v := range adj {
			valueFormat1 |= v.First.getFormat()
			valueFormat2 |= v.Second.getFormat()
		}
	}
	recLen := valueRecordEncodeLen(valueFormat1) + valueRecordEncodeLen(valueFormat2)

	class1Count := len(l.Adjust)
	var class2Count int
	if class1Count > 0 {
		class2Count = len(l.Adjust[0])
	}

	recordsLen := class1Count * class2Count * recLen
	coverageOffset := 16 + recordsLen
	covLen := l.Cov.ToTable().EncodeLen()
	classDef1Offset := coverageOffset + covLen
	cls1Len := l.Class1.AppendLen()
	classDef2Offset := classDef1Offset + cls1Len
	cls2Len := l.Class2.AppendLen()
	devStart := classDef2Offset + cls2Len

	pool := newDevicePool(devStart)
	type devOff struct {
		first, second [4]uint16
	}
	cellDev := make([][]devOff, class1Count)
	for ci, row := range l.Adjust {
		cellDev[ci] = make([]devOff, len(row))
		for cj, adj := range row {
			for k, d := range adj.First.deviceTables() {
				cellDev[ci][cj].first[k] = pool.add(d)
			}
			for k, d := range adj.Second.deviceTables() {
				cellDev[ci][cj].second[k] = pool.add(d)
			}
		}
	}
	total := devStart + pool.len()
	checkSubtableSize16("Gpos2_2", total)

	res := make([]byte, 0, total)
	res = append(res,
		0, 2, // posFormat
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(valueFormat1>>8), byte(valueFormat1),
		byte(valueFormat2>>8), byte(valueFormat2),
		byte(classDef1Offset>>8), byte(classDef1Offset),
		byte(classDef2Offset>>8), byte(classDef2Offset),
		byte(class1Count>>8), byte(class1Count),
		byte(class2Count>>8), byte(class2Count),
	)
	for ci, row := range l.Adjust {
		for cj, adj := range row {
			res = append(res, adj.First.encode(valueFormat1, cellDev[ci][cj].first)...)
			res = append(res, adj.Second.encode(valueFormat2, cellDev[ci][cj].second)...)
		}
	}
	res = append(res, l.Cov.ToTable().Encode()...)
	res = l.Class1.Append(res)
	res = l.Class2.Append(res)
	res = append(res, pool.bytes()...)

	return res
}

// Gpos3_1 is a Cursive Attachment Positioning subtable (format 1).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#cursive-attachment-positioning-format1-cursive-attachment
type Gpos3_1 struct {
	Cov     coverage.Table
	Records []EntryExitRecord // indexed by coverage index
}

// EntryExitRecord is an OpenType EntryExitRecord table, for use in [Gpos3_1]
// subtables.  The Exit anchor point of a glyph is aligned with the Entry anchor
// point of the following glyph.
type EntryExitRecord struct {
	Entry anchor.Table
	Exit  anchor.Table
}

// apply implements the [Subtable] interface.
func (l *Gpos3_1) apply(ctx *Context, a, b int) int {
	// TODO(voss): this is only correct if the RIGHT_TO_LEFT flag is not set.

	seq := ctx.seq

	idx, ok := l.Cov[seq[a].GID]
	if !ok {
		return -1
	}
	rec := l.Records[idx]
	if a > 0 {
		prevGlyph := seq[a-1] // TODO(voss): use ctx.Keep?
		prev, ok := l.Cov[prevGlyph.GID]
		if ok {
			prevRec := l.Records[prev]
			seq[a].YOffset = prevGlyph.YOffset + prevRec.Exit.Y - rec.Entry.Y
		}
	}
	if a < b-1 {
		nextGlyph := seq[a+1] // TODO(voss): use ctx.Keep?
		next, ok := l.Cov[nextGlyph.GID]
		if ok {
			nextRec := l.Records[next]
			seq[a].Advance = seq[a].XOffset + rec.Exit.X - nextGlyph.XOffset - nextRec.Entry.X
		}
	}

	return a + 1
}

func readGpos3_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	entryExitCount := int(buf[2])<<8 | int(buf[3])

	offsets, err := membudget.AllocSlice[uint16](p.Budget, 2*entryExitCount)
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	records, err := membudget.AllocSlice[EntryExitRecord](p.Budget, entryExitCount)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if offsets[2*i] != 0 {
			records[i].Entry, err = anchor.Read(p, subtablePos+int64(offsets[2*i]))
			if err != nil {
				return nil, err
			}
		}
		if offsets[2*i+1] != 0 {
			records[i].Exit, err = anchor.Read(p, subtablePos+int64(offsets[2*i+1]))
			if err != nil {
				return nil, err
			}
		}
	}

	cov, err := coverage.Read(p, subtablePos+coverageOffset)
	if err != nil {
		return nil, err
	}

	if entryExitCount > len(cov) {
		records = records[:len(cov)]
	} else if entryExitCount < len(cov) {
		cov.Prune(entryExitCount)
	}

	return &Gpos3_1{
		Cov:     cov,
		Records: records,
	}, nil
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos3_1) encodeLen() int {
	total := 6
	total += 4 * len(l.Records)
	for _, rec := range l.Records {
		if !rec.Entry.IsEmpty() {
			total += 6
		}
		if !rec.Exit.IsEmpty() {
			total += 6
		}
	}
	total += l.Cov.EncodeLen()
	return total
}

// encode implements the [Subtable] interface.
func (l *Gpos3_1) encode() []byte {
	total := 6
	entryExitCount := len(l.Records)
	total += 4 * entryExitCount
	entryOffs := make([]uint16, entryExitCount)
	exitOffs := make([]uint16, entryExitCount)
	for i, rec := range l.Records {
		if !rec.Entry.IsEmpty() {
			entryOffs[i] = uint16(total)
			total += 6
		}
		if !rec.Exit.IsEmpty() {
			exitOffs[i] = uint16(total)
			total += 6
		}
	}
	coverageOffset := total
	total += l.Cov.EncodeLen()
	checkSubtableSize16("Gpos3_1", total)

	res := make([]byte, 0, total)

	res = append(res,
		0, 1, // posFormat
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(entryExitCount>>8), byte(entryExitCount),
	)
	for i := range entryExitCount {
		res = append(res,
			byte(entryOffs[i]>>8), byte(entryOffs[i]),
			byte(exitOffs[i]>>8), byte(exitOffs[i]),
		)
	}
	for i := range entryExitCount {
		if entryOffs[i] != 0 {
			res = l.Records[i].Entry.Append(res)
		}
		if exitOffs[i] != 0 {
			res = l.Records[i].Exit.Append(res)
		}
	}

	res = append(res, l.Cov.Encode()...)

	return res
}
