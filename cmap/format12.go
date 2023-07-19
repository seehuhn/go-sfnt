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

	"golang.org/x/exp/maps"
	"seehuhn.de/go/sfnt/glyph"
)

// Format12 represents a format 12 cmap subtable.
// https://docs.microsoft.com/en-us/typography/opentype/spec/cmap#format-12-segmented-coverage
//
// The binary encoding is most efficient, if consecutive code points are mapped
// to consecutive glyph IDs.
type Format12 map[uint32]glyph.ID

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

	cmap := Format12{}

	var size uint32
	var prevEnd uint32
	for i := uint32(0); i < nSegments; i++ {
		base := 16 + i*12
		startCharCode := uint32(data[base])<<24 | uint32(data[base+1])<<16 | uint32(data[base+2])<<8 | uint32(data[base+3])
		endCharCode := uint32(data[base+4])<<24 | uint32(data[base+5])<<16 | uint32(data[base+6])<<8 | uint32(data[base+7])
		startGlyphID := uint32(data[base+8])<<24 | uint32(data[base+9])<<16 | uint32(data[base+10])<<8 | uint32(data[base+11])

		if (i > 0 && startCharCode <= prevEnd) ||
			endCharCode < startCharCode ||
			endCharCode == 0xFFFF_FFFF || // avoid integer overflow in the loop below
			startGlyphID > 0x10_FFFF ||
			startGlyphID+(endCharCode-startCharCode) > 0x10_FFFF {
			return nil, errMalformedSubtable
		}
		prevEnd = endCharCode

		size += endCharCode - startCharCode + 1
		if size > 65536 {
			// avoid excessive memory allocation from malformed subtables
			return nil, errMalformedSubtable
		}

		for c := startCharCode; c <= endCharCode; c++ {
			cmap[c] = glyph.ID(startGlyphID + c - startCharCode)
		}
	}

	return cmap, nil
}

func (cmap Format12) Encode(language uint16) []byte {
	var ss []format12segment
	keys := maps.Keys(cmap)
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	segStart := 0
	for i := 1; i < len(keys); i++ {
		if keys[i] != keys[i-1]+1 || cmap[keys[i]] != cmap[keys[i-1]]+1 {
			ss = append(ss, format12segment{
				StartCharCode: keys[segStart],
				EndCharCode:   keys[i-1],
				StartGlyphID:  cmap[keys[segStart]],
			})
			segStart = i
		}
	}
	if len(keys) > 0 {
		ss = append(ss, format12segment{
			StartCharCode: keys[segStart],
			EndCharCode:   keys[len(keys)-1],
			StartGlyphID:  cmap[keys[segStart]],
		})
	}

	nSegments := len(ss)
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
		out[base] = byte(ss[i].StartCharCode >> 24)
		out[base+1] = byte(ss[i].StartCharCode >> 16)
		out[base+2] = byte(ss[i].StartCharCode >> 8)
		out[base+3] = byte(ss[i].StartCharCode)
		out[base+4] = byte(ss[i].EndCharCode >> 24)
		out[base+5] = byte(ss[i].EndCharCode >> 16)
		out[base+6] = byte(ss[i].EndCharCode >> 8)
		out[base+7] = byte(ss[i].EndCharCode)
		// out[base+8] = 0
		// out[base+9] = 0
		out[base+10] = byte(ss[i].StartGlyphID >> 8)
		out[base+11] = byte(ss[i].StartGlyphID)
	}
	return out
}

func (cmap Format12) Lookup(code rune) glyph.ID {
	return cmap[uint32(code)]
}

func (cmap Format12) CodeRange() (low, high rune) {
	first := true
	for c := range cmap {
		cr := rune(c)
		if first || cr < low {
			low = cr
		}
		if first || cr > high {
			high = cr
		}
		first = false
	}
	return
}

type format12segment struct {
	StartCharCode uint32
	EndCharCode   uint32
	StartGlyphID  glyph.ID
}
