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
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/rect"

	"seehuhn.de/go/postscript/cid"
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

// Path returns the glyph outline as a path.Path iterator.
// This converts CFF glyph commands to path commands.
func (o *Outlines) Path(gid glyph.ID) path.Path {
	if int(gid) >= len(o.Glyphs) || o.Glyphs[gid] == nil {
		return func(yield func(path.Command, []path.Point) bool) {}
	}

	cffGlyph := o.Glyphs[gid]

	return func(yield func(path.Command, []path.Point) bool) {
		var buf [3]path.Point

		for _, cmd := range cffGlyph.Cmds {
			switch cmd.Op {
			case OpMoveTo:
				if len(cmd.Args) >= 2 {
					buf[0] = path.Point{X: cmd.Args[0], Y: cmd.Args[1]}
					if !yield(path.CmdMoveTo, buf[:1]) {
						return
					}
				}
			case OpLineTo:
				if len(cmd.Args) >= 2 {
					buf[0] = path.Point{X: cmd.Args[0], Y: cmd.Args[1]}
					if !yield(path.CmdLineTo, buf[:1]) {
						return
					}
				}
			case OpCurveTo:
				if len(cmd.Args) >= 6 {
					buf[0] = path.Point{X: cmd.Args[0], Y: cmd.Args[1]} // control point 1
					buf[1] = path.Point{X: cmd.Args[2], Y: cmd.Args[3]} // control point 2
					buf[2] = path.Point{X: cmd.Args[4], Y: cmd.Args[5]} // end point
					if !yield(path.CmdCubeTo, buf[:3]) {
						return
					}
				}
			}
		}

		// CFF glyphs are implicitly closed
		if !yield(path.CmdClose, nil) {
			return
		}
	}
}

// GlyphBBox computes the bounding box of a glyph, after the matrix M has been
// applied to the glyph outline.
//
// If the glyph is blank, the zero rectangle is returned.
func (o *Outlines) GlyphBBox(M matrix.Matrix, gid glyph.ID) rect.Rect {
	return o.Path(gid).Transform([6]float64(M)).BBox()
}

// GlyphBBoxPDF computes the bounding box of a glyph in PDF glyph space units
// (1/1000th of a text space unit).
// The font matrix M is applied to the glyph outline.
//
// If the glyph is blank, the zero rectangle is returned.
func (o *Outlines) GlyphBBoxPDF(M matrix.Matrix, gid glyph.ID) (bbox rect.Rect) {
	if o.IsCIDKeyed() {
		M = o.FontMatrices[o.FDSelect(gid)].Mul(M)
	}
	M = M.Mul(matrix.Scale(1000, 1000))

	return o.GlyphBBox(M, gid)
}

func (o *Outlines) IsBlank(gid glyph.ID) bool {
	if int(gid) >= len(o.Glyphs) {
		gid = 0 // .notdef
	}
	return len(o.Glyphs[gid].Cmds) == 0
}
