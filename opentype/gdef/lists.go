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
	"slices"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
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

	offsets, err := p.Budget.AllocSlice[uint16](glyphCount)
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

	// AttachPoint tables reached through several offsets are read once
	cache := readcache.New[[]uint16]()
	points, err := p.Budget.AllocSlice[[]uint16](len(offsets))
	if err != nil {
		return nil, err
	}
	for i, off := range offsets {
		if off == 0 {
			continue
		}
		points[i], err = cache.Get(pos+int64(off), func(pos int64) ([]uint16, error) {
			if err := p.SeekPos(pos); err != nil {
				return nil, err
			}
			pointCount, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			pts, err := p.Budget.AllocSlice[uint16](int(pointCount))
			if err != nil {
				return nil, err
			}
			for k := range pts {
				pts[k], err = p.ReadUint16()
				if err != nil {
					return nil, err
				}
			}
			return pts, nil
		})
		if err != nil {
			return nil, err
		}
	}

	return &AttachList{Cov: cov, Points: points}, nil
}

// attachListLayout holds the positions of the tables of an attachment point
// list.
type attachListLayout struct {
	covBytes []byte
	offs     []uint16
	pool     *offsetpool.Pool
}

// layout computes the positions of the AttachPoint tables.  Tables with
// identical encodings share one copy.  An error is returned if a table
// cannot be reached through its Offset16.
func (al *AttachList) layout() (*attachListLayout, error) {
	coverageOffset := 4 + 2*len(al.Points)
	if coverageOffset > 0xFFFF {
		return nil, fmt.Errorf("attachment point coverage offset %d out of range", coverageOffset)
	}
	covBytes := al.Cov.Encode()

	pool := offsetpool.New("attachment point", coverageOffset+len(covBytes))
	offs := make([]uint16, len(al.Points))
	for i, pts := range al.Points {
		if pts == nil {
			continue
		}
		var err error
		offs[i], err = pool.AddFunc(offsetpool.SliceKey(pts), func() []byte {
			enc := make([]byte, 2, 2+2*len(pts))
			enc[0] = byte(len(pts) >> 8)
			enc[1] = byte(len(pts))
			for _, pt := range pts {
				enc = append(enc, byte(pt>>8), byte(pt))
			}
			return enc
		})
		if err != nil {
			return nil, err
		}
	}
	return &attachListLayout{covBytes: covBytes, offs: offs, pool: pool}, nil
}

func (l *attachListLayout) encodeLen() int {
	return 4 + 2*len(l.offs) + len(l.covBytes) + l.pool.Len()
}

func (l *attachListLayout) append(buf []byte) []byte {
	glyphCount := len(l.offs)
	coverageOffset := 4 + 2*glyphCount
	buf = append(buf,
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(glyphCount>>8), byte(glyphCount))
	for _, off := range l.offs {
		buf = append(buf, byte(off>>8), byte(off))
	}
	buf = append(buf, l.covBytes...)
	return append(buf, l.pool.Bytes()...)
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

	offsets, err := p.Budget.AllocSlice[uint16](ligGlyphCount)
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

	// LigGlyph and CaretValue tables reached through several offsets are
	// read once
	ligGlyphs := readcache.New[[]CaretValue]()
	caretValues := readcache.New[CaretValue]()
	carets, err := p.Budget.AllocSlice[[]CaretValue](len(offsets))
	if err != nil {
		return nil, err
	}
	for i, off := range offsets {
		if off == 0 {
			continue
		}
		carets[i], err = ligGlyphs.Get(pos+int64(off), func(ligGlyphPos int64) ([]CaretValue, error) {
			if err := p.SeekPos(ligGlyphPos); err != nil {
				return nil, err
			}
			caretCount, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			caretOffsets, err := p.Budget.AllocSlice[uint16](int(caretCount))
			if err != nil {
				return nil, err
			}
			for k := range caretOffsets {
				caretOffsets[k], err = p.ReadUint16()
				if err != nil {
					return nil, err
				}
			}
			row, err := p.Budget.AllocSlice[CaretValue](int(caretCount))
			if err != nil {
				return nil, err
			}
			for k, co := range caretOffsets {
				row[k], err = caretValues.Get(ligGlyphPos+int64(co), func(pos int64) (CaretValue, error) {
					return readCaretValue(p, pos)
				})
				if err != nil {
					return nil, err
				}
			}
			return row, nil
		})
		if err != nil {
			return nil, err
		}
	}

	return &LigCaretList{Cov: cov, Carets: carets}, nil
}

// ligCaretListLayout holds the positions of the tables of a ligature caret
// list.
type ligCaretListLayout struct {
	covBytes []byte
	offs     []uint16
	pool     *offsetpool.Pool
}

// layout computes the positions of the LigGlyph tables.  Tables with
// identical encodings share one copy, and so do the CaretValue tables
// within a LigGlyph table.  An error is returned if a table cannot be
// reached through its Offset16.
func (lcl *LigCaretList) layout() (*ligCaretListLayout, error) {
	coverageOffset := 4 + 2*len(lcl.Carets)
	if coverageOffset > 0xFFFF {
		return nil, fmt.Errorf("ligature caret coverage offset %d out of range", coverageOffset)
	}
	covBytes := lcl.Cov.Encode()

	pool := offsetpool.New("ligature glyph", coverageOffset+len(covBytes))
	offs := make([]uint16, len(lcl.Carets))
	// LigGlyph tables with the same caret values are encoded once
	var keys offsetpool.SeqKeys[CaretValue]
	for i, row := range lcl.Carets {
		if row == nil {
			continue
		}
		var encErr error
		off, err := pool.AddFunc(keys.Key(offsetpool.SliceKey(row), len(row), slices.Values(row)), func() []byte {
			var enc []byte
			enc, encErr = encodeLigGlyph(row)
			return enc
		})
		if encErr != nil {
			return nil, encErr
		}
		if err != nil {
			return nil, err
		}
		offs[i] = off
	}
	return &ligCaretListLayout{covBytes: covBytes, offs: offs, pool: pool}, nil
}

// encodeLigGlyph encodes a LigGlyph table.
func encodeLigGlyph(row []CaretValue) ([]byte, error) {
	pool := offsetpool.New("caret value", 2+2*len(row))
	offs := make([]uint16, len(row))
	for i, cv := range row {
		var err error
		offs[i], err = pool.AddFunc(cv, func() []byte { return cv.append(nil) })
		if err != nil {
			return nil, err
		}
	}
	buf := make([]byte, 2, 2+2*len(row)+pool.Len())
	buf[0] = byte(len(row) >> 8)
	buf[1] = byte(len(row))
	for _, off := range offs {
		buf = append(buf, byte(off>>8), byte(off))
	}
	return append(buf, pool.Bytes()...), nil
}

func (l *ligCaretListLayout) encodeLen() int {
	return 4 + 2*len(l.offs) + len(l.covBytes) + l.pool.Len()
}

func (l *ligCaretListLayout) append(buf []byte) []byte {
	ligGlyphCount := len(l.offs)
	coverageOffset := 4 + 2*ligGlyphCount
	buf = append(buf,
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(ligGlyphCount>>8), byte(ligGlyphCount))
	for _, off := range l.offs {
		buf = append(buf, byte(off>>8), byte(off))
	}
	buf = append(buf, l.covBytes...)
	return append(buf, l.pool.Bytes()...)
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
