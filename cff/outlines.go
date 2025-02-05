// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2025  Jochen Voss <voss@seehuhn.de>
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
	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/rect"

	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
)

// Outlines stores the glyph data of a CFF font.
//
// There are two cases:
//   - For a simple font, Encoding is used, and ROS, GIDToCID, and FontMatrices
//     must be nil.  In this case FDSelect always returns 0.
//   - For CID-keyed fonts, ROS, GIDToCID, and FontMatrices are in used,
//     and Encoding must be nil.
type Outlines struct {
	Glyphs []*Glyph

	// Private stores the private dictionaries of the font.
	// The length of this slice must be at least one.
	// For a simple font the length is exactly one.
	Private []*type1.PrivateDict

	// FDSelect determines which private dictionary is used for each glyph.
	// For a simple font, this function always returns 0.
	FDSelect FDSelectFn

	// Encoding lists the glyphs corresponding to the 256 one-byte character
	// codes.  If present, the length of this slice must be 256.  Entries
	// for unused character codes must be set to 0.
	//
	// This is only used for simple fonts.
	Encoding []glyph.ID

	// ROS specifies the character collection of the font, using Adobe's
	// Registry, Ordering, Supplement system.
	//
	// This is only used for CID-keyed fonts.
	ROS *cid.SystemInfo

	// GIDToCID lists the character identifiers corresponding to the glyphs.
	// When present, the first entry (corresponding to the .notdef glyph) must
	// be 0.
	//
	// This is only used for CID-keyed fonts.
	GIDToCID []cid.CID

	// FontMatrices lists the font matrices corresponding to each private
	// dictionary.  The matrices are applied before the font matrix from
	// the Font.FontInfo structure.
	//
	// This is only used for CID-keyed fonts.
	FontMatrices []matrix.Matrix
}

// IsCIDKeyed returns true if the font is a CID-keyed font.
func (o *Outlines) IsCIDKeyed() bool {
	return o.ROS != nil
}

// BuiltinEncoding returns the built-in encoding of the font.
// For simple CFF fonts, the result is a slice of length 256.
// For CIDFonts, the result is nil.
func (o *Outlines) BuiltinEncoding() []string {
	if len(o.Encoding) != 256 {
		return nil
	}
	res := make([]string, 256)
	for i, gid := range o.Encoding {
		if gid <= 0 || int(gid) >= len(o.Glyphs) {
			res[i] = ".notdef"
		} else {
			res[i] = o.Glyphs[gid].Name
		}
	}
	return res
}

// NumGlyphs returns the number of glyphs in the font.
func (o *Outlines) NumGlyphs() int {
	return len(o.Glyphs)
}

// BBox returns the font bounding box.
//
// TODO(voss): remove
func (o *Outlines) BBox() (bbox funit.Rect16) {
	first := true
	for _, glyph := range o.Glyphs {
		glyphBox := glyph.Extent()
		if glyphBox.IsZero() {
			continue
		}
		if first {
			bbox = glyphBox
			first = false
		} else {
			bbox.Extend(glyphBox)
		}
	}
	return bbox
}

// GlyphBBoxPDF computes the bounding box of a glyph in PDF glyph space units
// (1/1000th of a text space unit).
// The font matrix fm is applied to the glyph outline.
//
// If the glyph is blank, the zero rectangle is returned.
func (o *Outlines) GlyphBBoxPDF(fm matrix.Matrix, gid glyph.ID) (bbox rect.Rect) {
	g := o.Glyphs[gid]

	var M matrix.Matrix
	if o.IsCIDKeyed() {
		M = o.FontMatrices[o.FDSelect(gid)].Mul(fm)
	} else {
		M = fm
	}
	M = M.Mul(matrix.Scale(1000, 1000))

	first := true
cmdLoop:
	for _, cmd := range g.Cmds {
		var x, y float64
		switch cmd.Op {
		case OpMoveTo, OpLineTo:
			x = cmd.Args[0]
			y = cmd.Args[1]
		case OpCurveTo:
			x = cmd.Args[4]
			y = cmd.Args[5]
		default:
			continue cmdLoop
		}

		x, y = M.Apply(x, y)

		if first || x < bbox.LLx {
			bbox.LLx = x
		}
		if first || x > bbox.URx {
			bbox.URx = x
		}
		if first || y < bbox.LLy {
			bbox.LLy = y
		}
		if first || y > bbox.URy {
			bbox.URy = y
		}
		first = false
	}

	return bbox
}
