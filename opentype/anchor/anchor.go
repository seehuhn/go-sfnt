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
	"bytes"
	"fmt"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
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
	return NewReader(p).read(pos)
}

// A Reader reads anchor tables and the Device tables they refer to.
// Tables at the same file position are read once and shared between all
// references, so the result of [Reader.Read] must not be modified.
type Reader struct {
	p       *parser.Parser
	anchors *readcache.Cache[*Table]
	devices *readcache.Cache[*device.Table]
	fit     map[[2]*device.Table]bool
}

// NewReader returns a Reader which reads from p.
func NewReader(p *parser.Parser) *Reader {
	return &Reader{
		p:       p,
		anchors: readcache.New[*Table](),
		devices: readcache.New[*device.Table](),
		fit:     map[[2]*device.Table]bool{},
	}
}

// Read reads the anchor table at pos.  The returned table is shared with
// all other reads at the same position.
func (r *Reader) Read(pos int64) (*Table, error) {
	return r.anchors.Get(pos, func(pos int64) (*Table, error) {
		t, err := r.read(pos)
		if err != nil {
			return nil, err
		}
		return &t, nil
	})
}

func (r *Reader) read(pos int64) (Table, error) {
	p := r.p
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
			res.XDev, err = r.devices.Get(pos+int64(xDevOffset), r.readDevice)
			if err != nil {
				return Table{}, err
			}
		}
		if yDevOffset != 0 {
			res.YDev, err = r.devices.Get(pos+int64(yDevOffset), r.readDevice)
			if err != nil {
				return Table{}, err
			}
		}
		// Two Device tables can be too large to both be reached from the
		// anchor.  The YDev table is dropped in this case, so that the
		// anchor can be written back.
		if !r.fits(&res) {
			res.YDev = nil
		}
	default:
		return Table{}, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/anchor",
			Reason:    fmt.Sprintf("invalid anchor table format %d", format),
		}
	}

	return res, nil
}

func (r *Reader) readDevice(pos int64) (*device.Table, error) {
	return device.Read(r.p, pos)
}

// maxDevStart is the largest length of the part of a format 3 anchor table
// before the second Device table, which keeps the offset of the second
// table within the range of an Offset16.
const maxDevStart = 0xFFFF

// fits reports whether the anchor can be encoded.  This fails only if both
// Device tables are present, differ, and are each so long that the other one
// cannot be reached after it.
//
// The result is remembered for each pair of Device tables, since many
// anchors may refer to the same shared tables.
func (r *Reader) fits(rec *Table) bool {
	x, y := rec.XDev, rec.YDev
	if x == nil || y == nil || x == y {
		return true
	}
	if 10+min(x.EncodeLen(), y.EncodeLen()) <= maxDevStart {
		return true
	}
	key := [2]*device.Table{x, y}
	ok, seen := r.fit[key]
	if !seen {
		ok = bytes.Equal(x.Encode(), y.Encode())
		r.fit[key] = ok
	}
	return ok
}

// format3Layout returns the encoded Device tables of a format 3 anchor and
// the offsets of the two tables from the start of the anchor.  If both
// tables encode to the same bytes, they are stored once.  Otherwise the
// shorter table is stored first, so that the offset of the second table is
// as small as possible.  The function panics if the second table cannot be
// reached through its Offset16.
func (rec Table) format3Layout() (xEnc, yEnc []byte, xOff, yOff int) {
	if rec.XDev != nil {
		xEnc = rec.XDev.Encode()
		xOff = 10
	}
	if rec.YDev != nil {
		yEnc = rec.YDev.Encode()
		yOff = 10
	}
	switch {
	case xEnc == nil || yEnc == nil:
		// at most one table
	case bytes.Equal(xEnc, yEnc):
		yEnc = nil
	case len(yEnc) < len(xEnc):
		xOff = 10 + len(yEnc)
	default:
		yOff = 10 + len(xEnc)
	}
	if xOff > maxDevStart || yOff > maxDevStart {
		panic("sfnt/anchor: Device tables too large for one anchor")
	}
	return xEnc, yEnc, xOff, yOff
}

// EncodeLen returns the number of bytes in the binary representation of the
// anchor table.
func (rec Table) EncodeLen() int {
	switch {
	case rec.XDev != nil || rec.YDev != nil:
		xEnc, yEnc, _, _ := rec.format3Layout()
		return 10 + len(xEnc) + len(yEnc)
	case rec.ContourPoint != nil:
		return 8
	default:
		return 6
	}
}

// Append appends the binary representation of the Anchor Table to buf.
//
// If XDev and YDev encode to the same bytes, the Device table is written
// once and both offsets point at it.
func (rec Table) Append(buf []byte) []byte {
	switch {
	case rec.XDev != nil || rec.YDev != nil:
		// format 3: device offsets are relative to the anchor table start
		xEnc, yEnc, xDevOffset, yDevOffset := rec.format3Layout()
		buf = append(buf,
			0, 3, // anchorFormat
			byte(rec.X>>8), byte(rec.X),
			byte(rec.Y>>8), byte(rec.Y),
			byte(xDevOffset>>8), byte(xDevOffset),
			byte(yDevOffset>>8), byte(yDevOffset),
		)
		if yDevOffset < xDevOffset {
			buf = append(buf, yEnc...)
			return append(buf, xEnc...)
		}
		buf = append(buf, xEnc...)
		return append(buf, yEnc...)
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
