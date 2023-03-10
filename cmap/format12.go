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

package cmap

import (
	"errors"
	"sort"

	"seehuhn.de/go/sfnt/glyph"
)

// format12 represents a format 12 cmap subtable.
// https://docs.microsoft.com/en-us/typography/opentype/spec/cmap#format-12-segmented-coverage
type format12 []format12segment

type format12segment struct {
	StartCharCode rune
	EndCharCode   rune
	StartGlyphID  glyph.ID
}

func decodeFormat12(data []byte, code2rune func(c int) rune) (Subtable, error) {
	if code2rune != nil {
		return nil, errors.New("cmap/format12: code2rune not supported")
	}

	if len(data) < 16 {
		return nil, errMalformedSubtable
	}

	nSegments := uint32(data[12])<<24 | uint32(data[13])<<16 | uint32(data[14])<<8 | uint32(data[15])
	if len(data) != 16+int(nSegments)*12 || nSegments > 1e6 {
		return nil, errMalformedSubtable
	}

	segments := make(format12, nSegments)
	prevEnd := rune(-1)
	for i := uint32(0); i < nSegments; i++ {
		base := 16 + i*12
		segments[i].StartCharCode = rune(data[base])<<24 | rune(data[base+1])<<16 | rune(data[base+2])<<8 | rune(data[base+3])
		segments[i].EndCharCode = rune(data[base+4])<<24 | rune(data[base+5])<<16 | rune(data[base+6])<<8 | rune(data[base+7])
		startGlyphID := uint32(data[base+8])<<24 | uint32(data[base+9])<<16 | uint32(data[base+10])<<8 | uint32(data[base+11])

		if segments[i].StartCharCode <= prevEnd ||
			segments[i].EndCharCode < segments[i].StartCharCode ||
			startGlyphID > 0x10_FFFF ||
			startGlyphID+uint32(segments[i].EndCharCode-segments[i].StartCharCode) > 0x10_FFFF {
			return nil, errMalformedSubtable
		}
		segments[i].StartGlyphID = glyph.ID(startGlyphID)

		prevEnd = segments[i].EndCharCode
	}

	return segments, nil
}

func (cmap format12) Encode(language uint16) []byte {
	nSegments := len(cmap)
	l := uint32(16 + nSegments*12)
	out := make([]byte, l)
	copy(out, []byte{
		0, 12, 0, 0,
		byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l),
		0, 0, byte(language >> 8), byte(language),
		byte(nSegments >> 24), byte(nSegments >> 16), byte(nSegments >> 8), byte(nSegments),
	})
	for i := 0; i < nSegments; i++ {
		base := 16 + i*12
		out[base] = byte(cmap[i].StartCharCode >> 24)
		out[base+1] = byte(cmap[i].StartCharCode >> 16)
		out[base+2] = byte(cmap[i].StartCharCode >> 8)
		out[base+3] = byte(cmap[i].StartCharCode)
		out[base+4] = byte(cmap[i].EndCharCode >> 24)
		out[base+5] = byte(cmap[i].EndCharCode >> 16)
		out[base+6] = byte(cmap[i].EndCharCode >> 8)
		out[base+7] = byte(cmap[i].EndCharCode)
		// out[base+8] = 0
		// out[base+9] = 0
		out[base+10] = byte(cmap[i].StartGlyphID >> 8)
		out[base+11] = byte(cmap[i].StartGlyphID)
	}
	return out
}

func (cmap format12) Lookup(code rune) glyph.ID {
	idx := sort.Search(len(cmap), func(i int) bool {
		return code <= cmap[i].EndCharCode
	})
	if idx == len(cmap) || cmap[idx].StartCharCode > code {
		return 0
	}
	return cmap[idx].StartGlyphID + glyph.ID(code-cmap[idx].StartCharCode)
}

func (cmap format12) CodeRange() (low, high rune) {
	if len(cmap) == 0 {
		return 0, 0
	}
	return cmap[0].StartCharCode, cmap[len(cmap)-1].EndCharCode
}
