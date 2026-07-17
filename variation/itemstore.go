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

package variation

import (
	"errors"
	"math"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
)

// longWordsFlag marks an ItemVariationData whose word deltas are 32-bit
// and remaining deltas 16-bit.  Without it, word deltas are 16-bit and
// the rest 8-bit.
const longWordsFlag = 0x8000

// ItemVariationStore holds a shared set of variation regions together
// with the delta values that apply within them.  It is the common delta
// container used by the HVAR, MVAR, GDEF and CFF2 tables.
type ItemVariationStore struct {
	Regions []Region
	Data    []*ItemVariationData
}

// ItemVariationData is one subtable of an [ItemVariationStore].
// RegionIndexes selects, per delta column, the region in the parent
// store.  Deltas is indexed [inner index][region column]; every row has
// len(RegionIndexes) entries.
type ItemVariationData struct {
	RegionIndexes []uint16
	Deltas        [][]int32
}

// ReadItemVariationStore reads a format 1 item variation store located at
// the absolute file position pos.  Allocations are charged against the
// parser's budget.
func ReadItemVariationStore(p *parser.Parser, pos int64) (*ItemVariationStore, error) {
	if err := p.SeekPos(pos); err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(8)
	if err != nil {
		return nil, err
	}
	format := uint16(buf[0])<<8 | uint16(buf[1])
	if format != 1 {
		return nil, errors.New("variation: unsupported item variation store format")
	}
	regionListOffset := uint32(buf[2])<<24 | uint32(buf[3])<<16 | uint32(buf[4])<<8 | uint32(buf[5])
	dataCount := int(uint16(buf[6])<<8 | uint16(buf[7]))

	if err := checkCount(p, dataCount, 4); err != nil {
		return nil, err
	}
	dataOffsets, err := membudget.AllocSlice[uint32](p.Budget, dataCount)
	if err != nil {
		return nil, err
	}
	for i := range dataOffsets {
		dataOffsets[i], err = p.ReadUint32()
		if err != nil {
			return nil, err
		}
	}

	s := &ItemVariationStore{}
	if regionListOffset != 0 {
		s.Regions, err = readRegionList(p, pos+int64(regionListOffset))
		if err != nil {
			return nil, err
		}
	}

	if dataCount == 0 {
		return s, nil
	}
	s.Data, err = membudget.AllocSlice[*ItemVariationData](p.Budget, dataCount)
	if err != nil {
		return nil, err
	}
	for i := range s.Data {
		if dataOffsets[i] == 0 {
			s.Data[i] = &ItemVariationData{}
			continue
		}
		s.Data[i], err = readItemVariationData(p, pos+int64(dataOffsets[i]), len(s.Regions))
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func readRegionList(p *parser.Parser, pos int64) ([]Region, error) {
	if err := p.SeekPos(pos); err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	axisCount := int(uint16(buf[0])<<8 | uint16(buf[1]))
	regionCount := int(uint16(buf[2])<<8 | uint16(buf[3]))
	if axisCount > MaxAxisCount {
		return nil, errors.New("variation: too many variation axes")
	}
	if regionCount == 0 {
		return nil, nil
	}

	// each region axis is 3 int16 values
	if err := checkCount(p, regionCount*axisCount, 6); err != nil {
		return nil, err
	}
	if err := p.Budget.ChargeN(regionCount*axisCount, 4); err != nil {
		return nil, err
	}

	regions, err := membudget.AllocSlice[Region](p.Budget, regionCount)
	if err != nil {
		return nil, err
	}
	for i := range regions {
		axes, err := membudget.AllocSlice[RegionAxis](p.Budget, axisCount)
		if err != nil {
			return nil, err
		}
		for j := range axes {
			start, err := ReadF2Dot14(p)
			if err != nil {
				return nil, err
			}
			peak, err := ReadF2Dot14(p)
			if err != nil {
				return nil, err
			}
			end, err := ReadF2Dot14(p)
			if err != nil {
				return nil, err
			}
			axes[j] = RegionAxis{Start: start, Peak: peak, End: end}
		}
		regions[i] = axes
	}
	return regions, nil
}

func readItemVariationData(p *parser.Parser, pos int64, regionCount int) (*ItemVariationData, error) {
	if err := p.SeekPos(pos); err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(6)
	if err != nil {
		return nil, err
	}
	itemCount := int(uint16(buf[0])<<8 | uint16(buf[1]))
	wordDeltaCount := uint16(buf[2])<<8 | uint16(buf[3])
	regionIndexCount := int(uint16(buf[4])<<8 | uint16(buf[5]))

	long := wordDeltaCount&longWordsFlag != 0
	wordCount := int(wordDeltaCount & 0x7FFF)
	if wordCount > regionIndexCount {
		return nil, errors.New("variation: word delta count exceeds region count")
	}

	if err := checkCount(p, regionIndexCount, 2); err != nil {
		return nil, err
	}
	var regionIndexes []uint16
	if regionIndexCount > 0 {
		regionIndexes, err = membudget.AllocSlice[uint16](p.Budget, regionIndexCount)
		if err != nil {
			return nil, err
		}
	}
	for i := range regionIndexes {
		idx, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		if int(idx) >= regionCount {
			return nil, errors.New("variation: region index out of range")
		}
		regionIndexes[i] = idx
	}

	wideBytes, narrowBytes := 2, 1
	if long {
		wideBytes, narrowBytes = 4, 2
	}
	rowSize := wordCount*wideBytes + (regionIndexCount-wordCount)*narrowBytes

	if err := checkCount(p, itemCount, rowSize); err != nil {
		return nil, err
	}
	if err := p.Budget.ChargeN(itemCount*regionIndexCount, 4); err != nil {
		return nil, err
	}

	var deltas [][]int32
	if itemCount > 0 {
		deltas, err = membudget.AllocSlice[[]int32](p.Budget, itemCount)
		if err != nil {
			return nil, err
		}
	}
	for i := range deltas {
		var row []int32
		if regionIndexCount > 0 {
			row, err = membudget.AllocSlice[int32](p.Budget, regionIndexCount)
			if err != nil {
				return nil, err
			}
		}
		for j := range row {
			var v int32
			if j < wordCount {
				v, err = readSigned(p, wideBytes)
			} else {
				v, err = readSigned(p, narrowBytes)
			}
			if err != nil {
				return nil, err
			}
			row[j] = v
		}
		deltas[i] = row
	}

	return &ItemVariationData{RegionIndexes: regionIndexes, Deltas: deltas}, nil
}

// readSigned reads a big-endian signed integer of n bytes (1, 2, or 4).
func readSigned(p *parser.Parser, n int) (int32, error) {
	switch n {
	case 1:
		v, err := p.ReadUint8()
		return int32(int8(v)), err
	case 2:
		v, err := p.ReadInt16()
		return int32(v), err
	default:
		return p.ReadInt32()
	}
}

// checkCount reports an error if count elements of elemSize bytes each
// cannot fit within the input file, guarding against oversized counts in
// malformed tables before any allocation.
func checkCount(p *parser.Parser, count, elemSize int) error {
	if count < 0 {
		return errors.New("variation: negative count")
	}
	if int64(count)*int64(elemSize) > p.Size() {
		return errors.New("variation: count exceeds input size")
	}
	return nil
}

// Evaluate returns the interpolated delta for the given outer and inner
// index at the normalized axis coordinates coords.  The result is a raw
// floating-point value; callers apply any required rounding.  An
// out-of-range outer or inner index yields 0.
func (s *ItemVariationStore) Evaluate(outer, inner uint16, coords []F2Dot14) float64 {
	if int(outer) >= len(s.Data) {
		return 0
	}
	d := s.Data[int(outer)]
	if int(inner) >= len(d.Deltas) {
		return 0
	}
	row := d.Deltas[int(inner)]
	var sum float64
	for j, idx := range d.RegionIndexes {
		if j >= len(row) || int(idx) >= len(s.Regions) {
			continue
		}
		sum += float64(row[j]) * s.Regions[idx].Scalar(coords)
	}
	return sum
}

// ivdLayout captures the encoding decisions for one ItemVariationData.
type ivdLayout struct {
	long      bool
	wordCount int
	rowSize   int
}

func analyzeItemVariationData(d *ItemVariationData) ivdLayout {
	ric := len(d.RegionIndexes)
	need := make([]int, ric) // 0: fits int8, 1: fits int16, 2: needs int32
	for _, row := range d.Deltas {
		for j := 0; j < ric && j < len(row); j++ {
			v := row[j]
			switch {
			case v < math.MinInt16 || v > math.MaxInt16:
				need[j] = 2
			case v < math.MinInt8 || v > math.MaxInt8:
				if need[j] < 1 {
					need[j] = 1
				}
			}
		}
	}
	maxNeed := 0
	for _, n := range need {
		if n > maxNeed {
			maxNeed = n
		}
	}
	long := maxNeed == 2
	threshold := 1
	if long {
		threshold = 2
	}
	wordCount := 0
	for j := 0; j < ric; j++ {
		if need[j] >= threshold {
			wordCount = j + 1
		}
	}
	wideBytes, narrowBytes := 2, 1
	if long {
		wideBytes, narrowBytes = 4, 2
	}
	rowSize := wordCount*wideBytes + (ric-wordCount)*narrowBytes
	return ivdLayout{long: long, wordCount: wordCount, rowSize: rowSize}
}

// storeLayout holds the computed byte layout of an ItemVariationStore.
type storeLayout struct {
	total            int
	axisCount        int
	regionListOffset int
	dataOffsets      []int
	ivds             []ivdLayout
}

func (s *ItemVariationStore) layout() storeLayout {
	axisCount := 0
	for _, r := range s.Regions {
		if len(r) > axisCount {
			axisCount = len(r)
		}
	}

	total := 8 + 4*len(s.Data)
	regionListOffset := total
	total += 4 + len(s.Regions)*axisCount*6

	dataOffsets := make([]int, len(s.Data))
	ivds := make([]ivdLayout, len(s.Data))
	for i, d := range s.Data {
		dataOffsets[i] = total
		ivds[i] = analyzeItemVariationData(d)
		total += 6 + 2*len(d.RegionIndexes) + len(d.Deltas)*ivds[i].rowSize
	}

	return storeLayout{
		total:            total,
		axisCount:        axisCount,
		regionListOffset: regionListOffset,
		dataOffsets:      dataOffsets,
		ivds:             ivds,
	}
}

// EncodeLen returns the number of bytes [ItemVariationStore.Encode]
// produces.
func (s *ItemVariationStore) EncodeLen() int {
	return s.layout().total
}

// Encode returns the binary form of the item variation store as a format
// 1 table.  The delta column widths and the LONG_WORDS flag are chosen
// minimally, and the output is deterministic.
func (s *ItemVariationStore) Encode() []byte {
	l := s.layout()
	buf := make([]byte, 0, l.total)

	// header
	buf = appendU16(buf, 1) // format
	buf = appendU32(buf, uint32(l.regionListOffset))
	buf = appendU16(buf, uint16(len(s.Data)))
	for _, off := range l.dataOffsets {
		buf = appendU32(buf, uint32(off))
	}

	// region list
	buf = appendU16(buf, uint16(l.axisCount))
	buf = appendU16(buf, uint16(len(s.Regions)))
	for _, r := range s.Regions {
		for j := 0; j < l.axisCount; j++ {
			var axis RegionAxis
			if j < len(r) {
				axis = r[j]
			}
			buf = appendU16(buf, uint16(axis.Start))
			buf = appendU16(buf, uint16(axis.Peak))
			buf = appendU16(buf, uint16(axis.End))
		}
	}

	// item variation data subtables
	for i, d := range s.Data {
		lay := l.ivds[i]
		wordDeltaCount := uint16(lay.wordCount)
		if lay.long {
			wordDeltaCount |= longWordsFlag
		}
		buf = appendU16(buf, uint16(len(d.Deltas)))
		buf = appendU16(buf, wordDeltaCount)
		buf = appendU16(buf, uint16(len(d.RegionIndexes)))
		for _, idx := range d.RegionIndexes {
			buf = appendU16(buf, idx)
		}
		wideBytes, narrowBytes := 2, 1
		if lay.long {
			wideBytes, narrowBytes = 4, 2
		}
		for _, row := range d.Deltas {
			for j := range d.RegionIndexes {
				var v int32
				if j < len(row) {
					v = row[j]
				}
				if j < lay.wordCount {
					buf = appendSigned(buf, v, wideBytes)
				} else {
					buf = appendSigned(buf, v, narrowBytes)
				}
			}
		}
	}

	return buf
}

func appendU16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

func appendU32(buf []byte, v uint32) []byte {
	return append(buf, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// appendSigned appends v as a big-endian signed integer of n bytes.
func appendSigned(buf []byte, v int32, n int) []byte {
	switch n {
	case 1:
		return append(buf, byte(v))
	case 2:
		return append(buf, byte(v>>8), byte(v))
	default:
		return appendU32(buf, uint32(v))
	}
}
