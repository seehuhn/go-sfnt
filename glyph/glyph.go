// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package glyph contains types for representing glyphs.
package glyph

import "seehuhn.de/go/postscript/funit"

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
// All dimensions are in the font's glyph space units.
type Info struct {
	GID      ID
	Text     []rune
	XOffset  funit.Int16 // TODO(voss): convert to float64
	YOffset  funit.Int16 // TODO(voss): convert to float64
	Advance  funit.Int16 // horizontal advance; TODO(voss): convert to float64
	YAdvance funit.Int16 // vertical advance, used only in vertical layout

	// LigID identifies the ligature a glyph belongs to.  It is zero for glyphs
	// that are not part of a ligature, and a non-zero value shared by a
	// ligature glyph and the marks attached to its components.  The value is
	// assigned during ligature substitution and is used to attach marks to the
	// correct ligature component.
	LigID uint16

	// LigComp is the index of the ligature component a mark attaches to,
	// meaningful only when LigID is non-zero.
	LigComp uint16
}
