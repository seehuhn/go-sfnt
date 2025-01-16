// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2021  Jochen Voss <voss@seehuhn.de>
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
	"math"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
)

// TODO(voss): implement support for font matrices

// Font stores a CFF font.
type Font struct {
	*type1.FontInfo
	*Outlines
}

// Clone creates a new font, consisting of shallow copies of the
// FontInfo and Outlines fields.
func (cff *Font) Clone() *Font {
	fontInfo := *cff.FontInfo
	outlines := *cff.Outlines
	return &Font{
		FontInfo: &fontInfo,
		Outlines: &outlines,
	}
}

// FontBBoxPDF computes the bounding box of the font in PDF glyph space units
// (1/1000th of a text space units).
func (cff *Font) FontBBoxPDF() rect.Rect {
	var bbox rect.Rect
	for gid := range cff.Glyphs {
		glyphBox := cff.Outlines.GlyphBBoxPDF(cff.FontInfo.FontMatrix, glyph.ID(gid))
		if glyphBox.IsZero() {
			continue
		}
		if bbox.IsZero() {
			bbox = glyphBox
		} else {
			bbox.Extend(glyphBox)
		}
	}
	return bbox
}

// Widths returns the widths of all glyphs in CFF glyph space units.
func (cff *Font) Widths() []float64 {
	res := make([]float64, len(cff.Glyphs))
	for i, glyph := range cff.Glyphs {
		res[i] = glyph.Width
	}
	return res
}

// WidthsPDF returns the advance widths of the glyphs in the font,
// in PDF glyph space units (1/1000th of a text space unit).
func (cff *Font) WidthsPDF() []float64 {
	widths := make([]float64, cff.NumGlyphs())
	q := cff.FontMatrix[0] * 1000
	for gid, glyph := range cff.Glyphs {
		widths[gid] = float64(glyph.Width) * q
	}
	return widths
}

// WidthsMapPDF returns a map from glyph names to advance widths in PDF text
// space units.
//
// If the font uses CIDFont operators, nil is returned (because there are no
// glyph names).
func (cff *Font) WidthsMapPDF() map[string]float64 {
	if cff.IsCIDKeyed() {
		return nil
	}

	q := cff.FontMatrix[0]
	if math.Abs(cff.FontMatrix[3]) > 1e-6 {
		q -= cff.FontMatrix[1] * cff.FontMatrix[2] / cff.FontMatrix[3]
	}
	q *= 1000

	widths := make(map[string]float64)
	for _, glyph := range cff.Glyphs {
		widths[glyph.Name] = float64(glyph.Width) * q
	}
	return widths
}

// GlyphWidthPDF returns the advance width of a glyph in PDF glyph space units.
func (cff *Font) GlyphWidthPDF(gid glyph.ID) float64 {
	var fm matrix.Matrix
	if cff.IsCIDKeyed() {
		fm = cff.FontMatrices[cff.FDSelect(gid)].Mul(cff.FontInfo.FontMatrix)
	} else {
		fm = cff.FontInfo.FontMatrix
	}

	q := fm[0]
	if math.Abs(fm[3]) > 1e-6 {
		q -= fm[1] * fm[2] / fm[3]
	}

	return cff.Glyphs[gid].Width * (q * 1000)
}

// CIDSystemInfo describes a character collection covered by a font.
// A character collection implies an encoding which maps Character IDs to glyphs.
//
// See section 5.11.2 of the PLRM.
type CIDSystemInfo struct {
	Registry   string
	Ordering   string
	Supplement int32
}

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
	// codes.  If presnt, the length of this slice must be 256.  Entries
	// for unused character codes must be set to 0.
	//
	// This is only used for simple fonts.
	Encoding []glyph.ID

	// ROS specifies the character collection of the font, using Adobe's
	// Registry, Ordering, Supplement system.
	//
	// This is only used for CID-keyed fonts.
	ROS *CIDSystemInfo

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
