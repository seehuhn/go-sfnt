// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2024  Jochen Voss <voss@seehuhn.de>
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
	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

// Gpos5_1 is a Mark-to-Ligature Attachment Positioning Subtable (format 1).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#lookup-type-5-mark-to-ligature-attachment-positioning-subtable
type Gpos5_1 struct {
	MarkCov   coverage.Table
	LigCov    coverage.Table
	MarkArray []markarray.Record // indexed by mark coverage index
	LigArray  [][][]anchor.Table // indexed by (ligature coverage index, ligature component, mark class)
}

func readGpos5_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(10)
	if err != nil {
		return nil, err
	}
	markCoverageOffset := int64(buf[0])<<8 | int64(buf[1])
	ligCoverageOffset := int64(buf[2])<<8 | int64(buf[3])
	markClassCount := int(buf[4])<<8 | int(buf[5])
	markArrayOffset := int64(buf[6])<<8 | int64(buf[7])
	ligArrayOffset := int64(buf[8])<<8 | int64(buf[9])

	markCov, err := coverage.Read(p, subtablePos+markCoverageOffset)
	if err != nil {
		return nil, err
	}
	ligCov, err := coverage.Read(p, subtablePos+ligCoverageOffset)
	if err != nil {
		return nil, err
	}

	markArray, err := markarray.Read(p, subtablePos+markArrayOffset, len(markCov))
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

	ligArrayPos := subtablePos + ligArrayOffset
	err = p.SeekPos(ligArrayPos)
	if err != nil {
		return nil, err
	}

	// Read the "LigatureArray Table"
	ligCount, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if int(ligCount) > len(ligCov) {
		ligCount = uint16(len(ligCov))
	} else {
		ligCov.Prune(int(ligCount))
	}
	// Array of offsets to LigatureAttach tables.  Offsets are from beginning
	// of LigatureArray table, ordered by ligatureCoverage index.
	offsets, err := membudget.AllocSlice[uint16](p.Budget, int(ligCount))
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	ligArray, err := membudget.AllocSlice[[][]anchor.Table](p.Budget, int(ligCount))
	if err != nil {
		return nil, err
	}
	for i := range ligArray {
		ligAttachPos := ligArrayPos + int64(offsets[i])
		err = p.SeekPos(ligAttachPos)
		if err != nil {
			return nil, err
		}

		componentCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		numAnchorOffsets := uint(componentCount) * uint(markClassCount)
		if numAnchorOffsets > (65536-2)/2 {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "GPOS5.1 table too large",
			}
		}
		anchorOffsets, err := membudget.AllocSlice[uint16](p.Budget, int(numAnchorOffsets))
		if err != nil {
			return nil, err
		}
		for k := range anchorOffsets {
			anchorOffsets[k], err = p.ReadUint16()
			if err != nil {
				return nil, err
			}
		}

		ligAttach, err := membudget.AllocSlice[[]anchor.Table](p.Budget, int(componentCount))
		if err != nil {
			return nil, err
		}
		for j := range ligAttach {
			row, err := membudget.AllocSlice[anchor.Table](p.Budget, int(markClassCount))
			if err != nil {
				return nil, err
			}
			for k := range row {
				if anchorOffsets[k] == 0 {
					continue
				}
				row[k], err = anchor.Read(p, ligAttachPos+int64(anchorOffsets[k]))
				if err != nil {
					return nil, err
				}
			}
			ligAttach[j] = row
			anchorOffsets = anchorOffsets[markClassCount:]
		}

		ligArray[i] = ligAttach
	}

	return &Gpos5_1{
		MarkCov:   markCov,
		LigCov:    ligCov,
		MarkArray: markArray,
		LigArray:  ligArray,
	}, nil
}

// apply implements the [Subtable] interface.
func (l *Gpos5_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	markIdx, ok := l.MarkCov[seq[a].GID]
	if !ok {
		return -1
	}
	markRecord := l.MarkArray[markIdx]

	if a == 0 {
		return -1
	}
	p := a - 1
	var ligIdx int
	for p >= 0 {
		ligIdx, ok = l.LigCov[seq[p].GID]
		if ok {
			break
		}
		p--
	}
	if p < 0 {
		return -1
	}

	components := l.LigArray[ligIdx]
	if len(components) == 0 {
		return -1
	}
	// Attach to the component recorded on the mark during ligature
	// substitution; a mark not tagged for this ligature defaults to the last
	// component.
	// TODO(voss): nested ligatures and multiple substitution do not yet
	// propagate component information.
	comp := len(components) - 1
	if seq[a].LigID != 0 && seq[a].LigID == seq[p].LigID && int(seq[a].LigComp) < len(components) {
		comp = int(seq[a].LigComp)
	}
	ligRecord := components[comp][markRecord.Class]
	if ligRecord.IsEmpty() {
		return -1
	}

	dx := ligRecord.X - markRecord.X
	dy := ligRecord.Y - markRecord.Y
	for i := p; i < a; i++ {
		dx -= seq[i].Advance
	}
	seq[a].XOffset = dx
	seq[a].YOffset = dy
	return a + 1
}

// countMarkClasses returns the markClassCount for this subtable.  It also
// validates the structural invariants the encoder depends on: every row
// in every LigArray ligature must have width markClassCount, and every
// MarkArray record's Class must be < markClassCount.  Both encodeLen and
// encode call this, so inconsistent input is caught on the first pass
// instead of leaving encodeLen happily returning a size for unencodable
// data.
func (l *Gpos5_1) countMarkClasses() int {
	var count int
	derived := false
	for _, lig := range l.LigArray {
		if len(lig) > 0 {
			count = len(lig[0])
			derived = true
			break
		}
	}
	if !derived {
		var maxClass uint16
		for _, rec := range l.MarkArray {
			if rec.Class > maxClass {
				maxClass = rec.Class
			}
		}
		count = int(maxClass) + 1
	}
	for _, lig := range l.LigArray {
		for _, row := range lig {
			if len(row) != count {
				panic("Gpos5_1: inconsistent LigArray row width")
			}
		}
	}
	for _, rec := range l.MarkArray {
		if int(rec.Class) >= count {
			panic("Gpos5_1: mark class out of range")
		}
	}
	return count
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos5_1) encodeLen() int {
	markClassCount := l.countMarkClasses()
	total := 12
	total += l.MarkCov.EncodeLen()
	total += l.LigCov.EncodeLen()
	total += 2 + (4+6)*len(l.MarkArray)
	total += 2 + 2*len(l.LigArray)
	for _, lig := range l.LigArray {
		total += 2 + 2*len(lig)*markClassCount
		for _, row := range lig {
			for _, rec := range row {
				if !rec.IsEmpty() {
					total += 6
				}
			}
		}
	}
	return total
}

// encode implements the [Subtable] interface.
func (l *Gpos5_1) encode() []byte {
	markCount := len(l.MarkArray)
	ligCount := len(l.LigArray)
	markClassCount := l.countMarkClasses()

	total := 12
	markCoverageOffset := total
	total += l.MarkCov.EncodeLen()
	ligCoverageOffset := total
	total += l.LigCov.EncodeLen()
	markArrayOffset := total
	total += 2 + (4+6)*markCount
	ligArrayOffset := total

	// lig array section: count + per-lig offsets + per-LigatureAttach blocks
	ligArrayLen := 2 + 2*ligCount
	ligAttachOffs := make([]uint16, ligCount)
	for i, lig := range l.LigArray {
		ligAttachOffs[i] = uint16(ligArrayLen)
		ligArrayLen += 2 + 2*len(lig)*markClassCount
		for _, row := range lig {
			for _, rec := range row {
				if !rec.IsEmpty() {
					ligArrayLen += 6
				}
			}
		}
	}
	total += ligArrayLen
	checkSubtableSize16("Gpos5_1", total)

	res := make([]byte, 0, total)
	res = append(res,
		0, 1, // posFormat
		byte(markCoverageOffset>>8), byte(markCoverageOffset),
		byte(ligCoverageOffset>>8), byte(ligCoverageOffset),
		byte(markClassCount>>8), byte(markClassCount),
		byte(markArrayOffset>>8), byte(markArrayOffset),
		byte(ligArrayOffset>>8), byte(ligArrayOffset),
	)

	res = append(res, l.MarkCov.Encode()...)
	res = append(res, l.LigCov.Encode()...)

	// mark array
	res = append(res,
		byte(markCount>>8), byte(markCount),
	)
	offs := 2 + 4*markCount
	for _, rec := range l.MarkArray {
		res = append(res,
			byte(rec.Class>>8), byte(rec.Class),
			byte(offs>>8), byte(offs),
		)
		offs += 6
	}
	for _, rec := range l.MarkArray {
		res = rec.Append(res)
	}

	// lig array
	res = append(res,
		byte(ligCount>>8), byte(ligCount),
	)
	for _, off := range ligAttachOffs {
		res = append(res, byte(off>>8), byte(off))
	}
	for _, lig := range l.LigArray {
		componentCount := len(lig)
		res = append(res, byte(componentCount>>8), byte(componentCount))
		anchorOff := 2 + 2*componentCount*markClassCount
		for _, row := range lig {
			for _, rec := range row {
				if rec.IsEmpty() {
					res = append(res, 0, 0)
					continue
				}
				res = append(res, byte(anchorOff>>8), byte(anchorOff))
				anchorOff += 6
			}
		}
		for _, row := range lig {
			for _, rec := range row {
				if rec.IsEmpty() {
					continue
				}
				res = rec.Append(res)
			}
		}
	}

	return res
}
