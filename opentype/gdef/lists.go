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

package gdef

import (
	"fmt"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
)

// AttachList is an OpenType "Attachment Point List Table".  It records, for
// selected glyphs, the contour points used to attach marks.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gdef#attachment-point-list-table
type AttachList struct {
	Cov coverage.Table // maps glyph ID to an index into Points
	// Points holds the attachment contour point indices for each covered
	// glyph, in increasing order, indexed by coverage index.  A nil entry
	// means the glyph has no attachment point list.
	Points [][]uint16
}

func readAttachList(p *parser.Parser, pos int64) (*AttachList, error) {
	if err := p.SeekPos(pos); err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	glyphCount := int(buf[2])<<8 | int(buf[3])

	offsets, err := membudget.AllocSlice[uint16](p.Budget, glyphCount)
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	cov, err := coverage.Read(p, pos+coverageOffset)
	if err != nil {
		return nil, err
	}
	if len(offsets) > len(cov) {
		offsets = offsets[:len(cov)]
	} else if len(offsets) < len(cov) {
		cov.Prune(len(offsets))
	}

	points, err := membudget.AllocSlice[[]uint16](p.Budget, len(offsets))
	if err != nil {
		return nil, err
	}
	for i, off := range offsets {
		if off == 0 {
			continue
		}
		if err := p.SeekPos(pos + int64(off)); err != nil {
			return nil, err
		}
		pointCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		pts, err := membudget.AllocSlice[uint16](p.Budget, int(pointCount))
		if err != nil {
			return nil, err
		}
		for k := range pts {
			pts[k], err = p.ReadUint16()
			if err != nil {
				return nil, err
			}
		}
		points[i] = pts
	}

	return &AttachList{Cov: cov, Points: points}, nil
}

func (al *AttachList) encodeLen() int {
	total := 4 + 2*len(al.Points) + al.Cov.EncodeLen()
	for _, pts := range al.Points {
		if pts != nil {
			total += 2 + 2*len(pts)
		}
	}
	return total
}

func (al *AttachList) append(buf []byte) []byte {
	glyphCount := len(al.Points)
	coverageOffset := 4 + 2*glyphCount
	covBytes := al.Cov.Encode()

	pos := coverageOffset + len(covBytes)
	offsets := make([]int, glyphCount)
	for i, pts := range al.Points {
		if pts == nil {
			continue
		}
		offsets[i] = pos
		pos += 2 + 2*len(pts)
	}

	buf = append(buf,
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(glyphCount>>8), byte(glyphCount))
	for _, off := range offsets {
		buf = append(buf, byte(off>>8), byte(off))
	}
	buf = append(buf, covBytes...)
	for _, pts := range al.Points {
		if pts == nil {
			continue
		}
		buf = append(buf, byte(len(pts)>>8), byte(len(pts)))
		for _, pt := range pts {
			buf = append(buf, byte(pt>>8), byte(pt))
		}
	}
	return buf
}

// LigCaretList is an OpenType "Ligature Caret List Table".  It records the
// caret positions used to divide ligature glyphs for text selection.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gdef#ligature-caret-list-table
type LigCaretList struct {
	Cov coverage.Table // maps glyph ID to an index into Carets
	// Carets holds the caret values for each covered ligature glyph, indexed
	// by coverage index.  A nil entry means the glyph has no caret list.
	Carets [][]CaretValue
}

// CaretValue is an OpenType "Caret Value Table".  The encoded format is chosen
// from the optional fields: a non-nil Device selects format 3, otherwise a
// non-nil ContourPoint selects format 2, otherwise format 1 is used.
type CaretValue struct {
	// Coordinate is the caret position in design units (formats 1 and 3).
	Coordinate funit.Int16

	// ContourPoint, if non-nil, is the index of a glyph contour point whose
	// position gives the caret (format 2).
	ContourPoint *uint16

	// Device, if non-nil, is a Device or VariationIndex table adjusting
	// Coordinate for the current size or instance (format 3).
	Device *device.Table
}

func readLigCaretList(p *parser.Parser, pos int64) (*LigCaretList, error) {
	if err := p.SeekPos(pos); err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	ligGlyphCount := int(buf[2])<<8 | int(buf[3])

	offsets, err := membudget.AllocSlice[uint16](p.Budget, ligGlyphCount)
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		offsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	cov, err := coverage.Read(p, pos+coverageOffset)
	if err != nil {
		return nil, err
	}
	if len(offsets) > len(cov) {
		offsets = offsets[:len(cov)]
	} else if len(offsets) < len(cov) {
		cov.Prune(len(offsets))
	}

	carets, err := membudget.AllocSlice[[]CaretValue](p.Budget, len(offsets))
	if err != nil {
		return nil, err
	}
	for i, off := range offsets {
		if off == 0 {
			continue
		}
		ligGlyphPos := pos + int64(off)
		if err := p.SeekPos(ligGlyphPos); err != nil {
			return nil, err
		}
		caretCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		caretOffsets, err := membudget.AllocSlice[uint16](p.Budget, int(caretCount))
		if err != nil {
			return nil, err
		}
		for k := range caretOffsets {
			caretOffsets[k], err = p.ReadUint16()
			if err != nil {
				return nil, err
			}
		}
		row, err := membudget.AllocSlice[CaretValue](p.Budget, int(caretCount))
		if err != nil {
			return nil, err
		}
		for k, co := range caretOffsets {
			row[k], err = readCaretValue(p, ligGlyphPos+int64(co))
			if err != nil {
				return nil, err
			}
		}
		carets[i] = row
	}

	return &LigCaretList{Cov: cov, Carets: carets}, nil
}

func (lcl *LigCaretList) encodeLen() int {
	total := 4 + 2*len(lcl.Carets) + lcl.Cov.EncodeLen()
	for _, row := range lcl.Carets {
		if row == nil {
			continue
		}
		total += 2 + 2*len(row)
		for _, cv := range row {
			total += cv.encodeLen()
		}
	}
	return total
}

func (lcl *LigCaretList) append(buf []byte) []byte {
	ligGlyphCount := len(lcl.Carets)
	coverageOffset := 4 + 2*ligGlyphCount
	covBytes := lcl.Cov.Encode()

	pos := coverageOffset + len(covBytes)
	offsets := make([]int, ligGlyphCount)
	for i, row := range lcl.Carets {
		if row == nil {
			continue
		}
		offsets[i] = pos
		pos += 2 + 2*len(row)
		for _, cv := range row {
			pos += cv.encodeLen()
		}
	}

	buf = append(buf,
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(ligGlyphCount>>8), byte(ligGlyphCount))
	for _, off := range offsets {
		buf = append(buf, byte(off>>8), byte(off))
	}
	buf = append(buf, covBytes...)
	for _, row := range lcl.Carets {
		if row == nil {
			continue
		}
		buf = append(buf, byte(len(row)>>8), byte(len(row)))
		caretOff := 2 + 2*len(row)
		for _, cv := range row {
			buf = append(buf, byte(caretOff>>8), byte(caretOff))
			caretOff += cv.encodeLen()
		}
		for _, cv := range row {
			buf = cv.append(buf)
		}
	}
	return buf
}

func readCaretValue(p *parser.Parser, pos int64) (CaretValue, error) {
	if err := p.SeekPos(pos); err != nil {
		return CaretValue{}, err
	}
	buf, err := p.ReadBytes(4)
	if err != nil {
		return CaretValue{}, err
	}
	format := uint16(buf[0])<<8 | uint16(buf[1])
	field := uint16(buf[2])<<8 | uint16(buf[3])

	switch format {
	case 1:
		return CaretValue{Coordinate: funit.Int16(field)}, nil
	case 2:
		cp := field
		return CaretValue{ContourPoint: &cp}, nil
	case 3:
		deviceOffset, err := p.ReadUint16()
		if err != nil {
			return CaretValue{}, err
		}
		res := CaretValue{Coordinate: funit.Int16(field)}
		if deviceOffset != 0 {
			res.Device, err = device.Read(p, pos+int64(deviceOffset))
			if err != nil {
				return CaretValue{}, err
			}
		}
		return res, nil
	default:
		return CaretValue{}, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gdef",
			Reason:    fmt.Sprintf("invalid caret value format %d", format),
		}
	}
}

func (cv CaretValue) encodeLen() int {
	if cv.Device != nil {
		return 6 + cv.Device.EncodeLen()
	}
	return 4
}

func (cv CaretValue) append(buf []byte) []byte {
	switch {
	case cv.Device != nil:
		// format 3: the device offset is relative to the caret value start
		buf = append(buf,
			0, 3,
			byte(cv.Coordinate>>8), byte(cv.Coordinate),
			0, 6) // deviceOffset
		return append(buf, cv.Device.Encode()...)
	case cv.ContourPoint != nil:
		return append(buf,
			0, 2,
			byte(*cv.ContourPoint>>8), byte(*cv.ContourPoint))
	default:
		return append(buf,
			0, 1,
			byte(cv.Coordinate>>8), byte(cv.Coordinate))
	}
}
