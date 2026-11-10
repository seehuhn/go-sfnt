// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cmap

import "seehuhn.de/go/sfnt/glyph"

// TODO(voss): add a way to iterate over CMap Subtables

// Subtable represents a decoded cmap subtable.
type Subtable interface {
	// Lookup returns the glyph index for the given rune.
	// If the rune is not found, Lookup returns 0 (corresponding to the ".notdef" glyph).
	//
	// TODO(voss): change lookup to map uint32 or int to glyph.ID?
	//
	// TODO(voss): change to return (glyph.ID, bool)?
	Lookup(r rune) glyph.ID

	// Encode returns the binary form of the subtable.
	Encode(language uint16) []byte

	// CodeRange returns the smallest and largest code point in the subtable.
	CodeRange() (low, high rune)
}

// From the font files on my laptop, I extracted all cmap subtables
// and removed duplicates.  The following table is the result.
//
//    count | format |
//   -------+--------+-----------------------------------
//     1668 |    4   | Segment mapping to delta values
//      625 |    6   | Trimmed table mapping
//      554 |   12   | Segmented coverage
//      226 |    0   | Byte encoding table
//       54 |   14   | Unicode Variation Sequences
//       47 |    2   | High-byte mapping through table
//        2 |   10   | Trimmed array
//        1 |    8   | mixed 16-bit and 32-bit coverage
//        1 |   13   | Many-to-one range mappings

var decoders = map[uint16]func([]byte, func(int) rune) (Subtable, error){
	0:  decodeFormat0,
	2:  notImplemented, // TODO(voss): implement
	4:  decodeFormat4,
	6:  decodeFormat6,
	8:  notImplemented, // TODO(voss): implement
	10: notImplemented, // TODO(voss): implement
	12: decodeFormat12,
	13: notImplemented, // TODO(voss): implement
	14: notImplemented, // TODO(voss): implement
}

func notImplemented([]byte, func(int) rune) (Subtable, error) {
	return nil, errUnsupportedCmapFormat
}

func unicode(code int) rune {
	return rune(code)
}
