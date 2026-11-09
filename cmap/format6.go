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

import "seehuhn.de/go/sfnt/glyph"

// maxFormat6Entries is the largest number of glyph IDs for which the
// length of a format 6 subtable fits into its 16-bit length field.
const maxFormat6Entries = (0xFFFF - 10) / 2

// decodeFormat6 decodes a format 6 cmap subtable.
//
// If code2rune is nil, the result is a [*Format6], indexed by the character
// codes of the subtable.  Otherwise the codes are translated by code2rune
// and the result is a [Format4], like for the other byte-encoded formats.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/cmap#format-6-trimmed-table-mapping
func decodeFormat6(data []byte, code2rune func(c int) rune) (Subtable, error) {
	if len(data) < 10 {
		return nil, errMalformedSubtable
	}
	firstCode := int(data[6])<<8 | int(data[7])
	count := int(data[8])<<8 | int(data[9])
	if count > maxFormat6Entries {
		// too long for the 16-bit length field
		return nil, errMalformedSubtable
	}

	// some fonts have an excess 0x0000 at the end of the table
	if len(data) == 10+2*count+2 && data[10+2*count] == 0 && data[10+2*count+1] == 0 {
		data = data[:10+2*count]
	}

	if len(data) != 10+2*count {
		return nil, errMalformedSubtable
	}
	data = data[10:]

	// character codes are limited to 16 bits
	count = min(count, 0x10000-firstCode)

	gids := make([]glyph.ID, count)
	for i := range gids {
		gids[i] = glyph.ID(data[2*i])<<8 | glyph.ID(data[2*i+1])
	}

	if code2rune == nil {
		return &Format6{FirstCode: uint16(firstCode), GlyphIDs: gids}, nil
	}

	res := make(Format4)
	for i, gid := range gids {
		if gid != 0 {
			res[uint16(code2rune(i+firstCode))] = gid
		}
	}
	return res, nil
}

// Format6 represents a format 6 cmap subtable, which maps a single range of
// consecutive character codes.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/cmap#format-6-trimmed-table-mapping
type Format6 struct {
	// FirstCode is the first character code covered by the subtable.
	FirstCode uint16

	// GlyphIDs gives the glyph for each character code, starting at
	// FirstCode.  Glyph ID 0 marks an unmapped code.
	//
	// FirstCode+len(GlyphIDs) must not exceed 0x10000, and the subtable
	// can hold at most 32762 entries.
	GlyphIDs []glyph.ID
}

// Lookup returns the glyph index for the given rune.
// If the rune is not found, Lookup returns 0 (corresponding to the ".notdef" glyph).
func (cmap *Format6) Lookup(r rune) glyph.ID {
	i := int(r) - int(cmap.FirstCode)
	if i < 0 || i >= len(cmap.GlyphIDs) {
		return 0
	}
	return cmap.GlyphIDs[i]
}

// Encode returns the binary form of the subtable.  The method panics if the
// subtable exceeds the limits given for [Format6.GlyphIDs].
func (cmap *Format6) Encode(language uint16) []byte {
	n := len(cmap.GlyphIDs)
	if n > maxFormat6Entries || int(cmap.FirstCode)+n > 0x10000 {
		panic("cmap: too many entries for a format 6 subtable")
	}
	L := 10 + 2*n
	buf := make([]byte, 0, L)
	buf = append(buf,
		0, 6, // format
		byte(L>>8), byte(L), // length
		byte(language>>8), byte(language), // language
		byte(cmap.FirstCode>>8), byte(cmap.FirstCode), // firstCode
		byte(n>>8), byte(n), // entryCount
	)
	for _, gid := range cmap.GlyphIDs {
		buf = append(buf, byte(gid>>8), byte(gid))
	}
	return buf
}

// CodeRange returns the smallest and largest code point in the subtable.
// For an empty subtable, both values are 0.
func (cmap *Format6) CodeRange() (low, high rune) {
	if len(cmap.GlyphIDs) == 0 {
		return 0, 0
	}
	low = rune(cmap.FirstCode)
	return low, low + rune(len(cmap.GlyphIDs)) - 1
}
