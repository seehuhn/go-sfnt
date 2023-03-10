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

// Package glyph contains types for representing glyphs.
package glyph

import "seehuhn.de/go/sfnt/funit"

// ID is used to enumerate the glyphs in a font.  The first glyph
// has index 0 and is used to indicate a missing character (usually rendered
// as an empty box).
type ID uint16

// Pair represents two consecutive glyphs, specified by a pair of
// character codes.  This is sometimes used for ligatures and kerning
// information.
type Pair struct {
	Left  ID
	Right ID
}

// Info contains layout information for a single glyph.
type Info struct {
	Gid     ID
	Text    []rune
	XOffset funit.Int16
	YOffset funit.Int16
	Advance funit.Int16
}

// Seq is a sequence of glyphs.
type Seq []Info

func (s Seq) Runes() []rune {
	var runes []rune
	for _, g := range s {
		runes = append(runes, g.Text...)
	}
	return runes
}

// AdvanceWidth returns the total advance width of the sequence.
func (s Seq) AdvanceWidth() funit.Int {
	var width funit.Int
	for _, g := range s {
		width += funit.Int(g.Advance)
	}
	return width
}
