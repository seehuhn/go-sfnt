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

package cff

import (
	"fmt"
	"io"
	"sort"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// FDSelectFn maps glyphID values to private dicts in Font.Info.Private.
type FDSelectFn func(glyph.ID) int

// maxFontDICTs is the largest number of Font DICTs a font can have.  The
// FDSelect formats defined by CFF store the Font DICT index in a single byte.
// CFF2 additionally allows a 16-bit index, but the same bound is used there
// since real fonts need only a handful of Font DICTs.
const maxFontDICTs = 256

// readFDSelect reads an FDSelect table.  Format 4, which is CFF2-only, is
// accepted only when allowFormat4 is set.
func readFDSelect(p *parser.Parser, nGlyphs, nPrivate int, allowFormat4 bool) (FDSelectFn, error) {
	format, err := p.ReadUint8()
	if err != nil {
		return nil, err
	}

	switch format {
	case 0:
		buf := make([]uint8, nGlyphs)
		_, err := io.ReadFull(p, buf)
		if err != nil {
			return nil, err
		}
		for i := range nGlyphs {
			if int(buf[i]) >= nPrivate {
				return nil, invalidSince("FDSelect out of range")
			}
		}
		return func(gid glyph.ID) int {
			return int(buf[gid])
		}, nil
	case 3:
		nRanges, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		if nGlyphs > 0 && nRanges == 0 {
			return nil, invalidSince("no FDSelect data found")
		}

		var end []glyph.ID
		var fdIdx []uint8

		prev := uint16(0)
		for i := 0; i < int(nRanges); i++ {
			first, err := p.ReadUint16()
			if err != nil {
				return nil, err
			} else if i > 0 && first <= prev || i == 0 && first != 0 {
				return nil, invalidSince("FDSelect is invalid")
			}
			fd, err := p.ReadUint8()
			if err != nil {
				return nil, err
			} else if int(fd) >= nPrivate {
				return nil, invalidSince("FDSelect out of range")
			}
			if i > 0 {
				end = append(end, glyph.ID(first))
			}
			fdIdx = append(fdIdx, fd)
			prev = first
		}
		sentinel, err := p.ReadUint16()
		if err != nil {
			return nil, err
		} else if int(sentinel) != nGlyphs {
			return nil, invalidSince("wrong FDSelect sentinel")
		}
		end = append(end, glyph.ID(nGlyphs))

		return func(gid glyph.ID) int {
			idx := sort.Search(int(nRanges),
				func(i int) bool { return gid < end[i] })
			return int(fdIdx[idx])
		}, nil
	case 4:
		if !allowFormat4 {
			return nil, unsupported("FDSelect format 4")
		}
		nRanges, err := p.ReadUint32()
		if err != nil {
			return nil, err
		}
		if nGlyphs > 0 && nRanges == 0 {
			return nil, invalidSince("no FDSelect data found")
		}
		// bound nRanges before allocating: each range needs six bytes
		if int64(nRanges) > p.Size()-p.Pos() {
			return nil, invalidSince("FDSelect nRanges exceeds input size")
		}

		var end []uint32
		var fdIdx []uint16

		prev := uint32(0)
		for i := 0; i < int(nRanges); i++ {
			first, err := p.ReadUint32()
			if err != nil {
				return nil, err
			} else if i > 0 && first <= prev || i == 0 && first != 0 {
				return nil, invalidSince("FDSelect is invalid")
			}
			fd, err := p.ReadUint16()
			if err != nil {
				return nil, err
			} else if int(fd) >= nPrivate {
				return nil, invalidSince("FDSelect out of range")
			}
			if i > 0 {
				end = append(end, first)
			}
			fdIdx = append(fdIdx, fd)
			prev = first
		}
		sentinel, err := p.ReadUint32()
		if err != nil {
			return nil, err
		} else if int64(sentinel) != int64(nGlyphs) {
			return nil, invalidSince("wrong FDSelect sentinel")
		}
		end = append(end, uint32(nGlyphs))

		return func(gid glyph.ID) int {
			idx := sort.Search(int(nRanges),
				func(i int) bool { return uint32(gid) < end[i] })
			return int(fdIdx[idx])
		}, nil
	default:
		return nil, unsupported(fmt.Sprintf("FDSelect format %d", format))
	}
}

// encode returns the binary form of the FDSelect map, choosing the smaller
// of formats 0 and 3.  Font DICT indices outside the range [0, nFonts)
// cannot be represented and cause an error.
func (fdSelect FDSelectFn) encode(nGlyphs, nFonts int) ([]byte, error) {
	for i := range nGlyphs {
		if fd := fdSelect(glyph.ID(i)); fd < 0 || fd >= nFonts {
			return nil, invalidSince("FDSelect out of range")
		}
	}

	format0Length := nGlyphs + 1

	buf := []byte{3, 0, 0}
	var currendFD int
	nSeg := 0
	for i := range nGlyphs {
		fd := fdSelect(glyph.ID(i))
		if i > 0 && fd == currendFD {
			continue
		}
		// new segment
		if len(buf)+3+2 >= format0Length {
			goto useFormat0
		}
		buf = append(buf, byte(i>>8), byte(i), byte(fd))
		nSeg++
		currendFD = fd
	}
	buf = append(buf, byte(nGlyphs>>8), byte(nGlyphs))
	buf[1], buf[2] = byte(nSeg>>8), byte(nSeg)
	return buf, nil

useFormat0:
	buf = make([]byte, nGlyphs+1)
	for i := range nGlyphs {
		buf[i+1] = byte(fdSelect(glyph.ID(i)))
	}
	return buf, nil
}

// encodeFormat4 encodes the FDSelect map as FDSelect format 4, which uses
// uint32 range boundaries and uint16 FontDICT indices.  This format is
// valid only in CFF2.
func (fdSelect FDSelectFn) encodeFormat4(nGlyphs int) []byte {
	buf := []byte{4, 0, 0, 0, 0} // format + nRanges placeholder
	currentFD := 0
	nSeg := 0
	for i := range nGlyphs {
		fd := fdSelect(glyph.ID(i))
		if i > 0 && fd == currentFD {
			continue
		}
		// new range
		buf = append(buf,
			byte(i>>24), byte(i>>16), byte(i>>8), byte(i), // first (uint32)
			byte(fd>>8), byte(fd)) // fd (uint16)
		nSeg++
		currentFD = fd
	}
	buf = append(buf, byte(nGlyphs>>24), byte(nGlyphs>>16), byte(nGlyphs>>8), byte(nGlyphs)) // sentinel
	buf[1], buf[2], buf[3], buf[4] = byte(nSeg>>24), byte(nSeg>>16), byte(nSeg>>8), byte(nSeg)
	return buf
}
