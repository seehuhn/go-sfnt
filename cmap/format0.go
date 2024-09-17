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
	"fmt"

	"seehuhn.de/go/sfnt/glyph"
)

// decodeFormat0 decodes a format 0 cmap subtable.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/cmap#format-0-byte-encoding-table
func decodeFormat0(data []byte, code2rune func(c int) rune) (Subtable, error) {
	if code2rune == nil {
		code2rune = unicode
	}

	data = data[6:]
	if len(data) != 256 {
		return nil, fmt.Errorf("cmap: format 0: expected 256 bytes, got %d", len(data))
	}

	res := &Format0{}
	copy(res.Data[:], data)

	return res, nil
}

type Format0 struct {
	Data [256]byte
}

// Lookup returns the glyph index for the given rune.
// If the rune is not found, Lookup returns 0 (corresponding to the ".notdef" glyph).
func (cmap *Format0) Lookup(r rune) glyph.ID {
	if r > 255 {
		return 0
	}
	return glyph.ID(cmap.Data[r])
}

// Encode returns the binary form of the subtable.
func (cmap *Format0) Encode(language uint16) []byte {
	L := 2 + 2 + 2 + 256
	buf := make([]byte, 0, L)
	buf = append(buf,
		0, 0, // format
		byte(L>>8), byte(L), // length
		byte(language>>8), byte(language), // language
	)
	buf = append(buf, cmap.Data[:]...)
	return buf
}

// CodeRange returns the smallest and largest code point in the subtable.
func (cmap *Format0) CodeRange() (low rune, high rune) {
	return 0, 255
}
