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

// Package glyf reads and writes "glyf" and "loca" tables.
// https://docs.microsoft.com/en-us/typography/opentype/spec/glyf
// https://docs.microsoft.com/en-us/typography/opentype/spec/loca
package glyf

import (
	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/maxp"
)

// Outlines stores the glyph data of a TrueType font.
type Outlines struct {
	// Glyphs is a slice of glyph outlines in the font.
	Glyphs Glyphs

	// Widths contains the glyph widths, indexed by glyph ID.
	Widths []funit.Int16

	// Names, if non-nil, contains the glyph names.
	Names []string

	// Tables contains the raw contents of the "cvt ", "fpgm", "prep", "gasp"
	// tables.
	Tables map[string][]byte

	// Maxp contains information from the "maxp" table.
	Maxp *maxp.TTFInfo
}

func (o *Outlines) NumGlyphs() int {
	return len(o.Glyphs)
}

// GlyphBBoxPDF computes the bounding box of a glyph in PDF glyph space units
// (1/1000th of a font unit).
// The font matrix fm is applied to the glyph bounding box from the font data.
//
// If the glyph is blank, the zero rectangle is returned.
func (o *Outlines) GlyphBBoxPDF(fm matrix.Matrix, gid glyph.ID) (bbox rect.Rect) {
	g := o.Glyphs[gid]
	if g == nil {
		return
	}

	M := fm.Mul(matrix.Scale(1000, 1000))
	type p16 struct {
		x, y funit.Int16
	}
	first := true
	for _, p := range []p16{{g.LLx, g.LLy}, {g.URx, g.LLy}, {g.URx, g.URy}, {g.LLx, g.URy}} {
		x, y := M.Apply(float64(p.x), float64(p.y))
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

// Glyphs contains a slice of TrueType glyph outlines.
// This represents the information stored in the "glyf" and "loca" tables
// of a TrueType font.
type Glyphs []*Glyph

// Glyph represents a single glyph in a TrueType font.
type Glyph struct {
	funit.Rect16
	Data interface{} // either SimpleGlyph or CompositeGlyph
}

// Encoded represents the data of a "glyf" and "loca" table.
type Encoded struct {
	GlyfData   []byte
	LocaData   []byte
	LocaFormat int16
}

// Decode converts the data from the "glyf" and "loca" tables into a slice of
// Glyphs.  The value for locaFormat is specified in the indexToLocFormat entry
// in the "head" table.
func Decode(enc *Encoded) (Glyphs, error) {
	offs, err := decodeLoca(enc)
	if err != nil {
		return nil, err
	}

	numGlyphs := len(offs) - 1

	gg := make(Glyphs, numGlyphs)
	for i := range gg {
		data := enc.GlyfData[offs[i]:offs[i+1]]
		g, err := decodeGlyph(data)
		if err != nil {
			return nil, err
		}
		gg[i] = g
	}

	return gg, nil
}

// Encode encodes the Glyphs into a "glyf" and "loca" table.
func (gg Glyphs) Encode() *Encoded {
	n := len(gg)

	offs := make([]int, n+1)
	offs[0] = 0
	for i, g := range gg {
		l := g.encodeLen()
		offs[i+1] = offs[i] + l
	}
	locaData, locaFormat := encodeLoca(offs)

	glyfData := make([]byte, 0, offs[n])
	for _, g := range gg {
		glyfData = g.append(glyfData)
	}

	enc := &Encoded{
		GlyfData:   glyfData,
		LocaData:   locaData,
		LocaFormat: locaFormat,
	}

	return enc
}
