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
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

// Gpos5_1 is a Mark-to-Ligature Attachment Positioning Subtable (format 1).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#lookup-type-5-mark-to-ligature-attachment-positioning-subtable
type Gpos5_1 struct {
	MarkCov   coverage.Table
	LigCov    coverage.Table
	MarkArray []markarray.Record  // indexed by mark coverage index
	LigArray  [][][]*anchor.Table // indexed by (ligature coverage index, ligature component, mark class); nil entries are absent
}

func readGpos5_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(10)
	if err != nil {
		return nil, err
	}
	anchors := anchor.NewReader(p)

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
	offsets, err := p.Budget.AllocSlice[uint16](int(ligCount))
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	ligArray, err := p.Budget.AllocSlice[[][]*anchor.Table](int(ligCount))
	if err != nil {
		return nil, err
	}
	readLigAttach := func(ligAttachPos int64) ([][]*anchor.Table, error) {
		err := p.SeekPos(ligAttachPos)
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
		anchorOffsets, err := p.Budget.AllocSlice[uint16](int(numAnchorOffsets))
		if err != nil {
			return nil, err
		}
		for k := range anchorOffsets {
			anchorOffsets[k], err = p.ReadUint16()
			if err != nil {
				return nil, err
			}
		}

		ligAttach, err := p.Budget.AllocSlice[[]*anchor.Table](int(componentCount))
		if err != nil {
			return nil, err
		}
		for j := range ligAttach {
			row, err := p.Budget.AllocSlice[*anchor.Table](markClassCount)
			if err != nil {
				return nil, err
			}
			for k := range row {
				if anchorOffsets[k] == 0 {
					continue
				}
				row[k], err = anchors.Read(ligAttachPos + int64(anchorOffsets[k]))
				if err != nil {
					return nil, err
				}
			}
			ligAttach[j] = row
			anchorOffsets = anchorOffsets[markClassCount:]
		}
		return ligAttach, nil
	}
	ligAttachCache := readcache.New[[][]*anchor.Table]()
	for i := range ligArray {
		ligArray[i], err = ligAttachCache.Get(ligArrayPos+int64(offsets[i]), readLigAttach)
		if err != nil {
			return nil, err
		}
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
	if ligRecord == nil {
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
// MarkArray record's Class must be < markClassCount.  It panics
// if these are violated.
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

// ligArray is the layout of a LigatureArray table: a ligature count, one
// offset per ligature, and the LigatureAttach tables.  Each LigatureAttach
// table addresses its anchors from its own start, so it is encoded on its
// own; identical tables are stored once.
type ligArray struct {
	ligs []int // pool indices of the LigatureAttach tables
	pool *offsetpool.Deferred
}

func newLigArray(name string, ligs [][][]*anchor.Table, markClassCount int) (*ligArray, error) {
	pool := offsetpool.NewDeferred(name, 2+2*len(ligs))
	indices := make([]int, len(ligs))
	// LigatureAttach tables with the same anchors are encoded once; the key
	// includes the row lengths, so that rows of the wrong length are
	// still told apart
	var keys offsetpool.SeqKeys[any]
	for i, lig := range ligs {
		var encErr error
		key := keys.Key(offsetpool.SliceKey(lig), len(lig)*(1+markClassCount), func(yield func(any) bool) {
			for _, row := range lig {
				if !yield(len(row)) {
					return
				}
				for _, a := range row {
					if !yield(a) {
						return
					}
				}
			}
		})
		idx, err := pool.AddFunc(key, func() []byte {
			m, err := newAnchorMatrix(name, lig, markClassCount)
			if err != nil {
				encErr = err
				return nil
			}
			return m.append(make([]byte, 0, m.size()))
		})
		if encErr != nil {
			return nil, encErr
		}
		if err != nil {
			return nil, err
		}
		indices[i] = idx
	}
	return &ligArray{ligs: indices, pool: pool}, nil
}

// size returns the encoded length of the array.
func (a *ligArray) size() int { return 2 + 2*len(a.ligs) + a.pool.Len() }

// append appends the encoded array to buf.
func (a *ligArray) append(buf []byte) []byte {
	offs, data := a.pool.Place()
	buf = appendUint16(buf, len(a.ligs))
	buf = appendOffsets(buf, offs, a.ligs)
	return append(buf, data...)
}

// layout computes the encoded layout of the subtable.  The mark and ligature
// coverage tables are stored once if they are identical, and so are
// identical LigatureAttach tables.  An error is returned if an offset does
// not fit into an Offset16.
func (l *Gpos5_1) layout() (*markAttach, error) {
	markClassCount := l.countMarkClasses()
	marks, err := newMarkArray("Gpos5_1", l.MarkArray)
	if err != nil {
		return nil, err
	}
	ligs, err := newLigArray("Gpos5_1", l.LigArray, markClassCount)
	if err != nil {
		return nil, err
	}
	return newMarkAttach("Gpos5_1", l.MarkCov, l.LigCov, markClassCount, marks, ligs)
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos5_1) encodeLen() int {
	return mustFit(l.layout()).size()
}

// encode implements the [Subtable] interface.
func (l *Gpos5_1) encode() []byte {
	lay := mustFit(l.layout())
	return lay.append(make([]byte, 0, lay.size()))
}

// fits implements the [Subtable] interface.
func (l *Gpos5_1) fits() error {
	_, err := l.layout()
	return err
}
