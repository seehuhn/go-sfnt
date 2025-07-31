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
func (f *Font) Clone() *Font {
	fontInfo := *f.FontInfo
	outlines := *f.Outlines
	return &Font{
		FontInfo: &fontInfo,
		Outlines: &outlines,
	}
}

// FontBBoxPDF computes the bounding box of the font in PDF glyph space units
// (1/1000th of a text space units).
func (f *Font) FontBBoxPDF() rect.Rect {
	var bbox rect.Rect
	for gid := range f.Glyphs {
		glyphBox := f.Outlines.GlyphBBoxPDF(f.FontInfo.FontMatrix, glyph.ID(gid))
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
func (f *Font) Widths() []float64 {
	res := make([]float64, len(f.Glyphs))
	for i, glyph := range f.Glyphs {
		res[i] = glyph.Width
	}
	return res
}

// WidthsPDF returns the advance widths of the glyphs in the font,
// in PDF glyph space units (1/1000th of a text space unit).
func (f *Font) WidthsPDF() []float64 {
	widths := make([]float64, f.NumGlyphs())
	q := f.FontMatrix[0] * 1000
	for gid, glyph := range f.Glyphs {
		widths[gid] = glyph.Width * q
	}
	return widths
}

// WidthsMapPDF returns a map from glyph names to advance widths in PDF text
// space units.
//
// If the font uses CIDFont operators, nil is returned (because there are no
// glyph names).
func (f *Font) WidthsMapPDF() map[string]float64 {
	if f.IsCIDKeyed() {
		return nil
	}

	q := f.FontMatrix[0]
	if math.Abs(f.FontMatrix[3]) > 1e-6 {
		q -= f.FontMatrix[1] * f.FontMatrix[2] / f.FontMatrix[3]
	}
	q *= 1000

	widths := make(map[string]float64)
	for _, glyph := range f.Glyphs {
		widths[glyph.Name] = glyph.Width * q
	}
	return widths
}

// GlyphWidthPDF returns the advance width of a glyph in PDF glyph space units.
func (f *Font) GlyphWidthPDF(gid glyph.ID) float64 {
	var fm matrix.Matrix
	if f.IsCIDKeyed() {
		fm = f.FontMatrices[f.FDSelect(gid)].Mul(f.FontInfo.FontMatrix)
	} else {
		fm = f.FontInfo.FontMatrix
	}

	q := fm[0]
	if math.Abs(fm[3]) > 1e-6 {
		q -= fm[1] * fm[2] / fm[3]
	}

	return f.Glyphs[gid].Width * (q * 1000)
}

func (f *Font) makePrivateDict(idx int, defaultWidth, nominalWidth float64) cffDict {
	private := f.Private[idx]

	privateDict := cffDict{}

	privateDict.setDeltaF16(opBlueValues, private.BlueValues)
	privateDict.setDeltaF16(opOtherBlues, private.OtherBlues)
	if math.Abs(private.BlueScale-defaultBlueScale) > 1e-6 {
		privateDict[opBlueScale] = []interface{}{private.BlueScale}
	}
	if private.BlueShift != defaultBlueShift {
		privateDict[opBlueShift] = []interface{}{private.BlueShift}
	}
	if private.BlueFuzz != defaultBlueFuzz {
		privateDict[opBlueFuzz] = []interface{}{private.BlueFuzz}
	}
	if private.StdHW != 0 {
		privateDict[opStdHW] = []interface{}{private.StdHW}
	}
	if private.StdVW != 0 {
		privateDict[opStdVW] = []interface{}{private.StdVW}
	}
	if private.ForceBold {
		privateDict[opForceBold] = []interface{}{int32(1)}
	}

	if defaultWidth != 0 {
		privateDict[opDefaultWidthX] = []interface{}{defaultWidth}
	}
	if nominalWidth != 0 {
		privateDict[opNominalWidthX] = []interface{}{nominalWidth}
	}

	return privateDict
}
