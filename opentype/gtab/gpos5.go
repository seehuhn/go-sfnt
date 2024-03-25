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
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

// Gpos5_1 is a Mark-to-Ligature Attachment Positioning Subtable (format 1)
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
	offsets := make([]uint16, ligCount)
	for i := range offsets {
		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	ligArray := make([][][]anchor.Table, ligCount)
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
		ligAttach := make([][]anchor.Table, componentCount)

		for j := 0; j < int(componentCount); j++ {
			row := make([]anchor.Table, markClassCount)
			for j := range row {
				if offsets[j] == 0 {
					continue
				}
				row[j], err = anchor.Read(p, ligAttachPos+int64(offsets[j]))
				if err != nil {
					return nil, err
				}
			}
			ligAttach[i] = row
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

// Apply implements the [Subtable] interface.
func (l *Gpos5_1) Apply(ctx *Context, a, b int) int {
	// TODO(voss): implement this
	return -1
}

// encode implements the [Subtable] interface.
func (l *Gpos5_1) encode() []byte {
	panic("not implemented")
}

// encodeLen implements the [Subtable] interface.
func (l *Gpos5_1) encodeLen() int {
	panic("not implemented")
}
