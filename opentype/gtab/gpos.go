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
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
	"seehuhn.de/go/sfnt/parser"
)

// mustFit returns the layout of a subtable, and panics if the subtable
// cannot be encoded.  The encoders use it because their input comes from
// the API, which is trusted.
func mustFit[T any](lay T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("sfnt/gtab: %v; split the lookup into smaller subtables", err))
	}
	return lay
}

// fitOffset16 returns off as an Offset16, or an error if it does not fit.
func fitOffset16(name string, off int) (uint16, error) {
	if off > 0xFFFF {
		return 0, fmt.Errorf("%s offset %d out of range", name, off)
	}
	return uint16(off), nil
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
	devices := readcache.New[*device.Table]()
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	valueFormat := decodeValueFormat(buf[2:])
	valueRecord, err := readValueRecord(p, valueFormat, subtablePos, devices)
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

// gpos1_1Layout is the encoded layout of a [Gpos1_1] subtable: the header
// and value record, the Device tables, and the coverage table.
type gpos1_1Layout struct {
	format uint16
	dev    [4]int // pool indices of the Device tables
	cov    int    // pool index of the coverage table
	pool   devicePool
	size   int
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *Gpos1_1) layout() (*gpos1_1Layout, error) {
	format := l.Adjust.getFormat()
	devStart := 6 + valueRecordEncodeLen(format)
	pool := newDevicePool("Gpos1_1", devStart)
	dev, err := pool.addRecord(l.Adjust)
	if err != nil {
		return nil, err
	}
	cov, err := pool.Add(l.Cov.Encode())
	if err != nil {
		return nil, err
	}
	return &gpos1_1Layout{format: format, dev: dev, cov: cov, pool: pool, size: devStart + pool.Len()}, nil
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos1_1) encodeLen() int {
	return mustFit(l.layout()).size
}

// encode implements the [Subtable] interface.
func (l *Gpos1_1) encode() []byte {
	lay := mustFit(l.layout())
	offs, data := lay.pool.Place()
	buf := make([]byte, 0, lay.size)
	buf = appendUint16(buf, 1) // format
	buf = appendUint16(buf, int(offs[lay.cov]))
	buf = appendUint16(buf, int(lay.format))
	buf = append(buf, l.Adjust.encode(lay.format, deviceOffsets(offs, lay.dev))...)
	return append(buf, data...)
}

// Gpos1_2 is a Single Adjustment Positioning Subtable (GPOS type 1, format 2).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#single-adjustment-positioning-format-2-array-of-positioning-values
//
// A nil entry in Adjust means that the glyph is not adjusted.  All entries
// share one encoded format, so if any entry is non-nil, the nil entries are
// encoded as zero records and read back as non-nil.
type Gpos1_2 struct {
	Cov    coverage.Table
	Adjust []*GposValueRecord // indexed by coverage index
}

func readGpos1_2(p *parser.Parser, subtablePos int64) (Subtable, error) {
	devices := readcache.New[*device.Table]()
	buf, err := p.ReadBytes(6)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	valueFormat := decodeValueFormat(buf[2:])
	valueCount := int(buf[4])<<8 | int(buf[5])
	valueRecords, err := p.Budget.AllocSlice[*GposValueRecord](valueCount)
	if err != nil {
		return nil, err
	}
	for i := range valueRecords {
		valueRecords[i], err = readValueRecord(p, valueFormat, subtablePos, devices)
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

// sharedFormat returns the format shared by all value records of the subtable.
func (l *Gpos1_2) sharedFormat() uint16 {
	var b formatBuilder
	for _, adj := range l.Adjust {
		b.add(adj)
	}
	return b.format()
}

// gpos1_2Layout is the encoded layout of a [Gpos1_2] subtable: the header
// and value records, the Device tables, and the coverage table.
type gpos1_2Layout struct {
	format uint16
	dev    [][4]int // pool indices of the Device tables, for each record
	cov    int      // pool index of the coverage table
	pool   devicePool
	size   int
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *Gpos1_2) layout() (*gpos1_2Layout, error) {
	format := l.sharedFormat()
	devStart := 8 + valueRecordEncodeLen(format)*len(l.Adjust)
	pool := newDevicePool("Gpos1_2", devStart)
	dev := make([][4]int, len(l.Adjust))
	for i, adj := range l.Adjust {
		var err error
		dev[i], err = pool.addRecord(adj)
		if err != nil {
			return nil, err
		}
	}
	cov, err := pool.Add(l.Cov.Encode())
	if err != nil {
		return nil, err
	}
	return &gpos1_2Layout{format: format, dev: dev, cov: cov, pool: pool, size: devStart + pool.Len()}, nil
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos1_2) encodeLen() int {
	return mustFit(l.layout()).size
}

// encode implements the [Subtable] interface.
func (l *Gpos1_2) encode() []byte {
	lay := mustFit(l.layout())
	offs, data := lay.pool.Place()
	buf := make([]byte, 0, lay.size)
	buf = appendUint16(buf, 2) // format
	buf = appendUint16(buf, int(offs[lay.cov]))
	buf = appendUint16(buf, int(lay.format))
	buf = appendUint16(buf, len(l.Adjust))
	for i, adj := range l.Adjust {
		buf = append(buf, adj.encode(lay.format, deviceOffsets(offs, lay.dev[i]))...)
	}
	return append(buf, data...)
}

// Gpos2_1 is a Pair Adjustment Positioning Subtable (format 1).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#pair-adjustment-positioning-format-1-adjustments-for-glyph-pairs
type Gpos2_1 struct {
	Adjust map[glyph.Pair]*PairAdjust

	// PositionSecond records whether the subtable has value records for
	// the second glyph of each pair.  If it is true, a match consumes both
	// glyphs and a nil Second counts as a zero record.  If it is false, a
	// match consumes only the first glyph, so that the second glyph can
	// start the next match, and Second must be nil in all pairs.
	PositionSecond bool
}

// PairAdjust represents information from a PairValueRecord table.
//
// This is used in [Gpos2_1] and [Gpos2_2] subtables.
//
// A nil value record means that the glyph is not adjusted.  Whether a match
// consumes the second glyph is set for the whole subtable, by its
// PositionSecond field.  All first records of a subtable share one encoded
// format, as do all second records.  If any record in the group is non-nil,
// the nil records are encoded as zero records and read back as non-nil.
type PairAdjust struct {
	First, Second *GposValueRecord
}

// apply implements the [Subtable] interface.
func (l *Gpos2_1) apply(ctx *Context, a, b int) int {
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
	adj, ok := l.Adjust[glyph.Pair{Left: g1.GID, Right: g2.GID}]
	if !ok {
		return -1
	}

	adj.First.Apply(&seq[a])
	if !l.PositionSecond {
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
	valueFormat1 := decodeValueFormat(buf[2:])
	valueFormat2 := decodeValueFormat(buf[4:])
	pairSetCount := int(buf[6])<<8 | int(buf[7])

	pairSetOffsets, err := p.Budget.AllocSlice[uint16](pairSetCount)
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

	devices := readcache.New[*device.Table]()
	type pair struct {
		second glyph.ID
		adjust *PairAdjust
	}
	readPairSet := func(pos int64) ([]pair, error) {
		err := p.SeekPos(pos)
		if err != nil {
			return nil, err
		}
		pairValueCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		pairs, err := p.Budget.AllocSlice[pair](int(pairValueCount))
		if err != nil {
			return nil, err
		}
		adjusts, err := p.Budget.AllocSlice[PairAdjust](int(pairValueCount))
		if err != nil {
			return nil, err
		}
		for j := range pairs {
			secondGlyph, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			// value-record device offsets are measured from the start of
			// the PairSet table, not the subtable
			first, err := readValueRecord(p, valueFormat1, pos, devices)
			if err != nil {
				return nil, err
			}
			second, err := readValueRecord(p, valueFormat2, pos, devices)
			if err != nil {
				return nil, err
			}
			adjusts[j] = PairAdjust{First: first, Second: second}
			pairs[j] = pair{glyph.ID(secondGlyph), &adjusts[j]}
		}
		return pairs, nil
	}

	// The result is a flat map, so a pair set used by several first glyphs
	// is expanded once per glyph.  The map entries are charged per
	// reference, the pair set itself once.
	pairSets := readcache.New[[]pair]()
	res := &Gpos2_1{
		Adjust:         map[glyph.Pair]*PairAdjust{},
		PositionSecond: valueFormat2 != 0,
	}
	for first, i := range cov {
		pairs, err := pairSets.Get(subtablePos+int64(pairSetOffsets[i]), readPairSet)
		if err != nil {
			return nil, err
		}
		// rough estimate for the map entries: 24 bytes each
		if err := p.Budget.ChargeN(len(pairs), 24); err != nil {
			return nil, err
		}
		for _, pr := range pairs {
			res.Adjust[glyph.Pair{Left: first, Right: pr.second}] = pr.adjust
		}
	}
	return res, nil
}

// CovAndAdjust is a convenience function which returns the coverage table and
// the adjustments.
func (l *Gpos2_1) CovAndAdjust() (coverage.Table, []map[glyph.ID]*PairAdjust) {
	seen := make(map[glyph.ID]bool)
	for pair := range l.Adjust {
		seen[pair.Left] = true
	}

	firstGids := slices.Sorted(maps.Keys(seen))
	cov := coverage.Table{}
	adjust := make([]map[glyph.ID]*PairAdjust, len(firstGids))
	for i, gid := range firstGids {
		cov[gid] = i
		adjust[i] = map[glyph.ID]*PairAdjust{}
	}

	for pair, adj := range l.Adjust {
		adjust[cov[pair.Left]][pair.Right] = adj
	}

	return cov, adjust
}

// pairFormats returns the formats shared by the first and second value
// records of a pair adjustment subtable.  It panics if positionSecond is
// false but a pair has a second value record, since such a record cannot be
// encoded.
func pairFormats(name string, pairs iter.Seq[*PairAdjust], positionSecond bool) (uint16, uint16) {
	var b1, b2 formatBuilder
	for v := range pairs {
		if v.Second != nil && !positionSecond {
			panic(name + ": Second set but PositionSecond is false")
		}
		b1.add(v.First)
		b2.add(v.Second)
	}
	// with PositionSecond set, the second records are present even if
	// all of them are nil
	b2.hasRecord = positionSecond
	return b1.format(), b2.format()
}

// sharedFormats returns the formats shared by the first and second value
// records of all pairs.
func (l *Gpos2_1) sharedFormats() (uint16, uint16) {
	return pairFormats("Gpos2_1", maps.Values(l.Adjust), l.PositionSecond)
}

// pairSetLayout is the encoded layout of a [Gpos2_1] subtable.  The pairs are
// grouped into PairSet tables, and pair sets with identical content are
// written once and shared between all first glyphs which use them.
type pairSetLayout struct {
	cov       coverage.Table
	sets      []map[glyph.ID]*PairAdjust // the distinct pair sets
	keys      [][]glyph.ID               // sorted second glyphs, for each set
	setOf     []int                      // set index, for each coverage index
	format1   uint16
	format2   uint16
	setPos    []int // subtable offset of each pair set
	pairDev   [][]pairDevOffsets
	devices   []byte // the Device tables, which follow the pair sets
	covBytes  []byte
	covOffset int
	covFirst  bool // whether the coverage table precedes the Device tables
	size      int
}

// pairDevOffsets holds the Device table offsets of one pair, relative to the
// start of the enclosing PairSet table.
type pairDevOffsets struct {
	first, second [4]uint16
}

// layout groups the pairs into pair sets, sharing pair sets with identical
// content, and computes every offset.  An error is returned if an offset does
// not fit into an Offset16.
func (l *Gpos2_1) layout() (*pairSetLayout, error) {
	cov, adjust := l.CovAndAdjust()
	format1, format2 := l.sharedFormats()
	res := &pairSetLayout{
		cov:     cov,
		setOf:   make([]int, len(adjust)),
		format1: format1,
		format2: format2,
	}
	seen := make(map[string]int)
	devIDs := &deviceIDs{}
	for i, adj := range adjust {
		keys := slices.Sorted(maps.Keys(adj))
		id := pairSetID(adj, keys, format1, format2, devIDs)
		idx, ok := seen[id]
		if !ok {
			idx = len(res.sets)
			seen[id] = idx
			res.sets = append(res.sets, adj)
			res.keys = append(res.keys, keys)
		}
		res.setOf[i] = idx
	}

	// The pair sets and the Device tables they point at come before the
	// coverage table, so that the offsets pointing at them stay small.  The
	// coverage table is reached through a single offset and carries none of
	// its own, so it may extend past the 64 KiB an offset can cover.  If the
	// coverage table cannot be reached this way, it is stored between the
	// pair sets.
	res.covBytes = cov.Encode()
	err := res.place(false)
	if err != nil {
		err = res.place(true)
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

// place computes the offsets of the pair sets, the Device tables and the
// coverage table.  The Device tables are addressed from the start of each
// pair set and follow all of them.  If covFirst is set, the coverage table
// is stored with the pair sets, otherwise after the Device tables.
func (ps *pairSetLayout) place(covFirst bool) error {
	n := len(ps.sets)
	sizes := make([]int, n, n+1)
	for i := range ps.sets {
		sizes[i] = ps.pairSetLen(i)
	}
	if covFirst {
		sizes = append(sizes, len(ps.covBytes))
	}
	pos := ps.pairSetsStart()
	offs, ok := offsetpool.Place(pos, sizes)
	if !ok {
		return errors.New("Gpos2_1 offset out of range")
	}
	for _, size := range sizes {
		pos += size
	}

	pool := newDevicePool("Gpos2_1", pos)
	indices := make([][]pairDevIndices, n)
	for i, adj := range ps.sets {
		row := make([]pairDevIndices, len(ps.keys[i]))
		for j, g := range ps.keys[i] {
			v := adj[g]
			var err error
			if row[j].first, err = pool.addRecord(v.First); err != nil {
				return err
			}
			if row[j].second, err = pool.addRecord(v.Second); err != nil {
				return err
			}
		}
		indices[i] = row
	}

	ps.setPos = offs[:n]
	ps.covFirst = covFirst
	ps.size = pos + pool.Len()
	if covFirst {
		ps.covOffset = offs[n]
	} else {
		if ps.size > 0xFFFF {
			return fmt.Errorf("Gpos2_1 offset %d out of range", ps.size)
		}
		ps.covOffset = ps.size
		ps.size += len(ps.covBytes)
	}

	var devOffs []uint16
	devOffs, ps.devices = pool.Place()
	ps.pairDev = make([][]pairDevOffsets, n)
	for i, row := range indices {
		// device offsets are written relative to the PairSet table start
		base := uint16(ps.setPos[i])
		ps.pairDev[i] = make([]pairDevOffsets, len(row))
		for j, dev := range row {
			first := deviceOffsets(devOffs, dev.first)
			second := deviceOffsets(devOffs, dev.second)
			for k := range first {
				if first[k] != 0 {
					ps.pairDev[i][j].first[k] = first[k] - base
				}
				if second[k] != 0 {
					ps.pairDev[i][j].second[k] = second[k] - base
				}
			}
		}
	}
	return nil
}

// pairSetID returns a string which identifies the encoded content of a pair
// set: two pair sets with the same ID encode to the same bytes, including the
// Device tables they refer to.
func pairSetID(adj map[glyph.ID]*PairAdjust, keys []glyph.ID, format1, format2 uint16, devIDs *deviceIDs) string {
	var buf []byte
	addRecord := func(vr *GposValueRecord, format uint16) {
		buf = append(buf, vr.encode(format, [4]uint16{})...)
		for _, d := range vr.deviceTables() {
			buf = binary.AppendUvarint(buf, devIDs.id(d))
		}
	}
	for _, g := range keys {
		v := adj[g]
		buf = append(buf, byte(g>>8), byte(g))
		addRecord(v.First, format1)
		addRecord(v.Second, format2)
	}
	return string(buf)
}

// deviceIDs numbers Device tables by their encoding.  Each table is encoded
// only once, since the readers share one Go value between all records which
// reach the same Device table.
type deviceIDs struct {
	byPtr map[*device.Table]uint64
	byEnc map[string]uint64
}

// id returns a number which identifies the encoding of d.  Tables with equal
// encodings get the same number, and nil gets 0.
func (ids *deviceIDs) id(d *device.Table) uint64 {
	if d == nil {
		return 0
	}
	if id, ok := ids.byPtr[d]; ok {
		return id
	}
	if ids.byPtr == nil {
		ids.byPtr = map[*device.Table]uint64{}
		ids.byEnc = map[string]uint64{}
	}
	enc := string(d.Encode())
	id, ok := ids.byEnc[enc]
	if !ok {
		id = uint64(len(ids.byEnc)) + 1
		ids.byEnc[enc] = id
	}
	ids.byPtr[d] = id
	return id
}

// pairSetsStart returns the subtable offset of the first pair set.
func (ps *pairSetLayout) pairSetsStart() int {
	return 10 + 2*len(ps.setOf)
}

// pairSetLen returns the encoded length of the pair set with index i.
func (ps *pairSetLayout) pairSetLen(i int) int {
	recLen := 2 + valueRecordEncodeLen(ps.format1) + valueRecordEncodeLen(ps.format2)
	return 2 + recLen*len(ps.sets[i])
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos2_1) encodeLen() int {
	return mustFit(l.layout()).size
}

// encode implements the [Subtable] interface.
func (l *Gpos2_1) encode() []byte {
	ps := mustFit(l.layout())

	buf := make([]byte, 0, ps.size)
	buf = appendUint16(buf, 1) // format
	buf = appendUint16(buf, ps.covOffset)
	buf = appendUint16(buf, int(ps.format1))
	buf = appendUint16(buf, int(ps.format2))
	buf = appendUint16(buf, len(ps.setOf))
	for _, idx := range ps.setOf {
		buf = appendUint16(buf, ps.setPos[idx])
	}

	// the pair sets, and the coverage table if it is stored with them, in
	// order of increasing offset; index len(ps.sets) is the coverage table
	n := len(ps.sets)
	pos := ps.setPos
	if ps.covFirst {
		pos = append(slices.Clone(pos), ps.covOffset)
	}
	order := make([]int, len(pos))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(i, j int) int { return cmp.Compare(pos[i], pos[j]) })
	for _, i := range order {
		if i == n {
			buf = append(buf, ps.covBytes...)
			continue
		}
		adj := ps.sets[i]
		buf = appendUint16(buf, len(adj))
		for j, secondGlyph := range ps.keys[i] {
			v := adj[secondGlyph]
			buf = appendUint16(buf, int(secondGlyph))
			buf = append(buf, v.First.encode(ps.format1, ps.pairDev[i][j].first)...)
			buf = append(buf, v.Second.encode(ps.format2, ps.pairDev[i][j].second)...)
		}
	}
	buf = append(buf, ps.devices...)
	if !ps.covFirst {
		buf = append(buf, ps.covBytes...)
	}
	return buf
}

// Gpos2_2 is a Pair Adjustment Positioning Subtable (format 2).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#pair-adjustment-positioning-format-2-class-pair-adjustment
type Gpos2_2 struct {
	Cov            coverage.Set
	Class1, Class2 classdef.Table
	Adjust         [][]*PairAdjust // indexed by class1 index, then class2 index

	// PositionSecond records whether the subtable has value records for
	// the second glyph of each pair, as described for [Gpos2_1].
	PositionSecond bool
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
	if !l.PositionSecond {
		return p
	}
	adj.Second.Apply(&seq[p])
	return p + 1
}

func readGpos2_2(p *parser.Parser, subtablePos int64) (Subtable, error) {
	devices := readcache.New[*device.Table]()
	buf, err := p.ReadBytes(14)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	valueFormat1 := decodeValueFormat(buf[2:])
	valueFormat2 := decodeValueFormat(buf[4:])
	classDef1Offset := int64(buf[6])<<8 | int64(buf[7])
	classDef2Offset := int64(buf[8])<<8 | int64(buf[9])
	class1Count := uint16(buf[10])<<8 | uint16(buf[11])
	class2Count := uint16(buf[12])<<8 | uint16(buf[13])

	numRecords := int(class1Count) * int(class2Count)
	if numRecords >= 65536 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    "GPOS2.2 table too large",
		}
	}
	records, err := p.Budget.AllocSlice[*PairAdjust](numRecords)
	if err != nil {
		return nil, err
	}
	pairAdjusts, err := p.Budget.AllocSlice[PairAdjust](numRecords)
	if err != nil {
		return nil, err
	}
	for i := range numRecords {
		first, err := readValueRecord(p, valueFormat1, subtablePos, devices)
		if err != nil {
			return nil, err
		}
		second, err := readValueRecord(p, valueFormat2, subtablePos, devices)
		if err != nil {
			return nil, err
		}
		pairAdjusts[i] = PairAdjust{First: first, Second: second}
		records[i] = &pairAdjusts[i]
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

	adjust, err := p.Budget.AllocSlice[[]*PairAdjust](int(class1Count))
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(class1Count); i++ {
		adjust[i] = records[i*int(class2Count) : (i+1)*int(class2Count)]
	}

	return &Gpos2_2{
		Cov:            cov,
		Class1:         classDef1,
		Class2:         classDef2,
		Adjust:         adjust,
		PositionSecond: valueFormat2 != 0,
	}, nil
}

// countClasses returns the class1Count and class2Count for this subtable.  It
// also validates the structural invariant the encoder depends on: every row in
// Adjust must have width class2Count.  The records are written as one
// rectangular block whose size the following offsets are measured from, so a
// ragged row would displace the coverage table, the two classdefs and every
// Device table.  Both encodeLen and encode call this, so inconsistent input is
// caught on the first pass instead of leaving encodeLen happily returning a
// size for unencodable data.
func (l *Gpos2_2) countClasses() (class1Count, class2Count int) {
	class1Count = len(l.Adjust)
	if class1Count > 0 {
		class2Count = len(l.Adjust[0])
	}
	for _, row := range l.Adjust {
		if len(row) != class2Count {
			panic("Gpos2_2: inconsistent Adjust row width")
		}
	}
	return class1Count, class2Count
}

// sharedFormats returns the formats shared by the first and second value
// records of all class pairs.
func (l *Gpos2_2) sharedFormats() (uint16, uint16) {
	cells := func(yield func(*PairAdjust) bool) {
		for _, row := range l.Adjust {
			for _, v := range row {
				if !yield(v) {
					return
				}
			}
		}
	}
	return pairFormats("Gpos2_2", cells, l.PositionSecond)
}

// gpos2_2Layout is the encoded layout of a [Gpos2_2] subtable: the header
// and class records, followed by the Device tables, the coverage table and
// the two class definitions.  Identical tables are stored once.
type gpos2_2Layout struct {
	cov                      coverage.Table
	format1, format2         uint16
	class1Count, class2Count int
	cellDev                  [][]pairDevIndices
	tables                   [3]int // pool indices of the coverage and class definition tables
	pool                     devicePool
	size                     int
}

// pairDevIndices holds the pool indices of the Device tables of one class
// pair.
type pairDevIndices struct {
	first, second [4]int
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *Gpos2_2) layout() (*gpos2_2Layout, error) {
	valueFormat1, valueFormat2 := l.sharedFormats()
	recLen := valueRecordEncodeLen(valueFormat1) + valueRecordEncodeLen(valueFormat2)
	class1Count, class2Count := l.countClasses()
	if class1Count > 0xFFFF || class2Count > 0xFFFF {
		return nil, errors.New("Gpos2_2 class count out of range")
	}

	// The Device tables come before the coverage table and the two
	// classdefs, so that the many offsets pointing into them stay small.
	// The other tables are reached through one offset each, so the last of
	// them may extend past the 64 KiB the offsets can reach.
	devStart := 16 + class1Count*class2Count*recLen
	pool := newDevicePool("Gpos2_2", devStart)
	cellDev := make([][]pairDevIndices, class1Count)
	for ci, row := range l.Adjust {
		cellDev[ci] = make([]pairDevIndices, len(row))
		for cj, adj := range row {
			first, err := pool.addRecord(adj.First)
			if err != nil {
				return nil, err
			}
			second, err := pool.addRecord(adj.Second)
			if err != nil {
				return nil, err
			}
			cellDev[ci][cj] = pairDevIndices{first: first, second: second}
		}
	}

	cov := l.Cov.ToTable()
	var tables [3]int
	for i, enc := range [][]byte{cov.Encode(), l.Class1.Append(nil), l.Class2.Append(nil)} {
		var err error
		tables[i], err = pool.Add(enc)
		if err != nil {
			return nil, err
		}
	}
	return &gpos2_2Layout{
		cov:         cov,
		format1:     valueFormat1,
		format2:     valueFormat2,
		class1Count: class1Count,
		class2Count: class2Count,
		cellDev:     cellDev,
		tables:      tables,
		pool:        pool,
		size:        devStart + pool.Len(),
	}, nil
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos2_2) encodeLen() int {
	return mustFit(l.layout()).size
}

// encode implements the [Subtable] interface.
func (l *Gpos2_2) encode() []byte {
	lay := mustFit(l.layout())
	offs, data := lay.pool.Place()

	res := make([]byte, 0, lay.size)
	res = appendUint16(res, 2) // format
	res = appendUint16(res, int(offs[lay.tables[0]]))
	res = appendUint16(res, int(lay.format1))
	res = appendUint16(res, int(lay.format2))
	res = appendUint16(res, int(offs[lay.tables[1]]))
	res = appendUint16(res, int(offs[lay.tables[2]]))
	res = appendUint16(res, lay.class1Count)
	res = appendUint16(res, lay.class2Count)
	for ci, row := range l.Adjust {
		for cj, adj := range row {
			dev := lay.cellDev[ci][cj]
			res = append(res, adj.First.encode(lay.format1, deviceOffsets(offs, dev.first))...)
			res = append(res, adj.Second.encode(lay.format2, deviceOffsets(offs, dev.second))...)
		}
	}
	return append(res, data...)
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
	Entry *anchor.Table // nil if absent
	Exit  *anchor.Table // nil if absent
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
			if prevRec.Exit != nil && rec.Entry != nil {
				seq[a].YOffset = prevGlyph.YOffset + prevRec.Exit.Y - rec.Entry.Y
			}
		}
	}
	if a < b-1 {
		nextGlyph := seq[a+1] // TODO(voss): use ctx.Keep?
		next, ok := l.Cov[nextGlyph.GID]
		if ok {
			nextRec := l.Records[next]
			if rec.Exit != nil && nextRec.Entry != nil {
				seq[a].Advance = seq[a].XOffset + rec.Exit.X - nextGlyph.XOffset - nextRec.Entry.X
			}
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

	offsets, err := p.Budget.AllocSlice[uint16](2 * entryExitCount)
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	anchors := anchor.NewReader(p)
	records, err := p.Budget.AllocSlice[EntryExitRecord](entryExitCount)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if offsets[2*i] != 0 {
			records[i].Entry, err = anchors.Read(subtablePos + int64(offsets[2*i]))
			if err != nil {
				return nil, err
			}
		}
		if offsets[2*i+1] != 0 {
			records[i].Exit, err = anchors.Read(subtablePos + int64(offsets[2*i+1]))
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

// gpos3_1Layout is the encoded layout of a [Gpos3_1] subtable: the header
// and entry/exit offsets, the anchors, and the coverage table.
type gpos3_1Layout struct {
	anchors [][2]int // pool indices of the entry and exit anchors
	cov     int      // pool index of the coverage table
	pool    anchorPool
	size    int
}

// layout computes the encoded layout of the subtable.  Cursive attachment
// reuses join points heavily, so the same anchor commonly appears on many
// glyphs; identical anchors are stored once.  An error is returned if an
// offset does not fit into an Offset16.
func (l *Gpos3_1) layout() (*gpos3_1Layout, error) {
	base := 6 + 4*len(l.Records)
	pool := newAnchorPool("Gpos3_1", base)
	anchors := make([][2]int, len(l.Records))
	for i, rec := range l.Records {
		var err error
		if anchors[i][0], err = pool.add(rec.Entry); err != nil {
			return nil, err
		}
		if anchors[i][1], err = pool.add(rec.Exit); err != nil {
			return nil, err
		}
	}
	cov, err := pool.Add(l.Cov.Encode())
	if err != nil {
		return nil, err
	}
	return &gpos3_1Layout{anchors: anchors, cov: cov, pool: pool, size: base + pool.Len()}, nil
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos3_1) encodeLen() int {
	return mustFit(l.layout()).size
}

// encode implements the [Subtable] interface.
func (l *Gpos3_1) encode() []byte {
	lay := mustFit(l.layout())
	offs, data := lay.pool.Place()

	res := make([]byte, 0, lay.size)
	res = appendUint16(res, 1) // format
	res = appendUint16(res, int(offs[lay.cov]))
	res = appendUint16(res, len(l.Records))
	for _, a := range lay.anchors {
		res = appendOffsets(res, offs, a[:])
	}
	return append(res, data...)
}

// fits implements the [Subtable] interface.
func (l *Gpos1_1) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *Gpos1_2) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *Gpos2_1) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *Gpos2_2) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *Gpos3_1) fits() error {
	_, err := l.layout()
	return err
}
