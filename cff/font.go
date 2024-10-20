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

	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
)

// TODO(voss): implement support for font matrices

// Font stores a CFF font.
//
// TODO(voss): make this more similar to type1.Font.  Maybe merge Outlines into
// Font?
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

// NumGlyphs returns the number of glyphs in the font.
func (cff *Font) NumGlyphs() int {
	return len(cff.Glyphs)
}

// Widths returns the widths of all glyphs.
func (cff *Font) Widths() []float64 {
	res := make([]float64, len(cff.Glyphs))
	for i, glyph := range cff.Glyphs {
		res[i] = glyph.Width
	}
	return res
}

// WidthsPDF returns the advance widths of the glyphs in the font,
// in PDF text space units.
func (cff *Font) WidthsPDF() []float64 {
	widths := make([]float64, cff.NumGlyphs())
	for gid, glyph := range cff.Glyphs {
		widths[gid] = float64(glyph.Width) * cff.FontMatrix[0]
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
type Outlines struct {
	Glyphs []*Glyph

	Private []*type1.PrivateDict

	// FDSelect determines which private dictionary is used for each glyph.
	FDSelect FDSelectFn

	// Encoding lists the glyphs corresponding to the 256 one-byte character
	// codes in a simple font. The length of this slice must be 256, entries
	// for unused character codes must be set to 0.
	// For CIDFonts (where ROS != nil), Encoding must be nil.
	Encoding []glyph.ID

	// ROS specifies the character collection of the font, using Adobe's
	// Registry, Ordering, Supplement system.  This must be non-nil
	// if and only if the font is a CIDFont.
	ROS *CIDSystemInfo

	// GIDToCID lists the character identifiers corresponding to the glyphs.
	// This is only present for CIDFonts, and encodes the information from the
	// charset table in the CFF font.  When present, the first entry
	// (corresponding to the .notdef glyph) must be 0.
	//
	// Since CID values are used to select glyphs in the font, the CID values
	// in the slice should be distinct.
	GIDToCID []cid.CID
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

// BBox returns the font bounding box.
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

func (o *Outlines) GetEncoding() []string {
	if len(o.Encoding) != 256 {
		return nil
	}
	res := make([]string, 256)
	for i, gid := range o.Encoding {
		if gid == 0 || int(gid) >= len(o.Glyphs) {
			res[i] = ".notdef"
		} else {
			res[i] = o.Glyphs[gid].Name
		}
	}
	return res
}

// GlyphWidthPDF returns the advance width of a glyph in PDF text space units.
func (cff *Font) GlyphWidthPDF(gid glyph.ID) float64 {
	return float64(cff.Glyphs[gid].Width) * cff.FontMatrix[0]
}
