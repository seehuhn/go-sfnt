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

// Package anchor encodes and decodes OpenType "Anchor Tables".
package anchor

import (
	"fmt"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
)

// Table is an OpenType "Anchor Table".
//
// The encoded format is chosen from the optional fields: a non-nil XDev or
// YDev selects format 3, otherwise a non-nil ContourPoint selects format 2,
// otherwise format 1 is used.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#anchor-tables
type Table struct {
	X, Y funit.Int16

	// ContourPoint, if non-nil, is the index of a glyph contour point that
	// coincides with the anchor (format 2).
	ContourPoint *uint16

	// XDev and YDev, if non-nil, hold Device or VariationIndex tables that
	// adjust the anchor coordinates for the current size or instance
	// (format 3).
	XDev, YDev *device.Table
}

// Read reads an anchor table from the given parser.
func Read(p *parser.Parser, pos int64) (Table, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return Table{}, err
	}

	buf, err := p.ReadBytes(6)
	if err != nil {
		return Table{}, err
	}

	format := uint16(buf[0])<<8 | uint16(buf[1])
	res := Table{
		X: funit.Int16(buf[2])<<8 | funit.Int16(buf[3]),
		Y: funit.Int16(buf[4])<<8 | funit.Int16(buf[5]),
	}

	switch format {
	case 1:
		// coordinates only
	case 2:
		cp, err := p.ReadUint16()
		if err != nil {
			return Table{}, err
		}
		res.ContourPoint = &cp
	case 3:
		buf, err := p.ReadBytes(4)
		if err != nil {
			return Table{}, err
		}
		xDevOffset := uint16(buf[0])<<8 | uint16(buf[1])
		yDevOffset := uint16(buf[2])<<8 | uint16(buf[3])
		if xDevOffset != 0 {
			res.XDev, err = device.Read(p, pos+int64(xDevOffset))
			if err != nil {
				return Table{}, err
			}
		}
		if yDevOffset != 0 {
			res.YDev, err = device.Read(p, pos+int64(yDevOffset))
			if err != nil {
				return Table{}, err
			}
		}
	default:
		return Table{}, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/anchor",
			Reason:    fmt.Sprintf("invalid anchor table format %d", format),
		}
	}

	return res, nil
}

// EncodeLen returns the number of bytes in the binary representation of the
// anchor table.
func (rec Table) EncodeLen() int {
	switch {
	case rec.XDev != nil || rec.YDev != nil:
		total := 10
		if rec.XDev != nil {
			total += rec.XDev.EncodeLen()
		}
		if rec.YDev != nil {
			total += rec.YDev.EncodeLen()
		}
		return total
	case rec.ContourPoint != nil:
		return 8
	default:
		return 6
	}
}

// Append appends the binary representation of the Anchor Table to buf.
func (rec Table) Append(buf []byte) []byte {
	switch {
	case rec.XDev != nil || rec.YDev != nil:
		// format 3: device offsets are relative to the anchor table start
		var xDevOffset, yDevOffset int
		next := 10
		if rec.XDev != nil {
			xDevOffset = next
			next += rec.XDev.EncodeLen()
		}
		if rec.YDev != nil {
			yDevOffset = next
		}
		buf = append(buf,
			0, 3, // anchorFormat
			byte(rec.X>>8), byte(rec.X),
			byte(rec.Y>>8), byte(rec.Y),
			byte(xDevOffset>>8), byte(xDevOffset),
			byte(yDevOffset>>8), byte(yDevOffset),
		)
		if rec.XDev != nil {
			buf = append(buf, rec.XDev.Encode()...)
		}
		if rec.YDev != nil {
			buf = append(buf, rec.YDev.Encode()...)
		}
		return buf
	case rec.ContourPoint != nil:
		return append(buf,
			0, 2, // anchorFormat
			byte(rec.X>>8), byte(rec.X),
			byte(rec.Y>>8), byte(rec.Y),
			byte(*rec.ContourPoint>>8), byte(*rec.ContourPoint),
		)
	default:
		return append(buf,
			0, 1, // anchorFormat
			byte(rec.X>>8), byte(rec.X),
			byte(rec.Y>>8), byte(rec.Y),
		)
	}
}
