// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cmap

import (
	"fmt"

	"seehuhn.de/go/sfnt/glyph"
)

// decodeFormat0 decodes a format 0 cmap subtable.
//
// If code2rune is nil, the result is a [*Format0], indexed by the character
// codes of the subtable.  Otherwise the codes are translated by code2rune
// and the result is a [Format4], like for the other byte-encoded formats.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/cmap#format-0-byte-encoding-table
func decodeFormat0(data []byte, code2rune func(c int) rune) (Subtable, error) {
	data = data[6:]
	if len(data) != 256 {
		return nil, fmt.Errorf("cmap: format 0: expected 256 bytes, got %d", len(data))
	}

	if code2rune != nil {
		res := make(Format4)
		for code, gid := range data {
			if gid != 0 {
				res[uint16(code2rune(code))] = glyph.ID(gid)
			}
		}
		return res, nil
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
	if r < 0 || r > 255 {
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
