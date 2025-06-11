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
	"fmt"
	"iter"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
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
	if int(gid) >= len(o.Glyphs) {
		return
	}
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
	Data any // either SimpleGlyph or CompositeGlyph
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
	if numGlyphs < 0 {
		return Glyphs{}, nil
	}

	gg := make(Glyphs, numGlyphs)
	for i := range gg {
		if offs[i+1] > len(enc.GlyfData) || offs[i] > offs[i+1] {
			return nil, fmt.Errorf("invalid glyph offset at index %d", i)
		}
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

// Path returns a compound path representing the glyph outline.
// For composite glyphs, this recursively includes all component glyphs
// with their transformations applied.
func (gg Glyphs) Path(gid glyph.ID) path.Compound {
	return &glyphPath{gg: gg, gid: gid}
}

type glyphPath struct {
	gg  Glyphs
	gid glyph.ID
}

func (q *glyphPath) Contours() iter.Seq[path.Contour] {
	seen := make(map[glyph.ID]bool)
	return q.do(seen, q.gid)
}

func (q *glyphPath) do(seen map[glyph.ID]bool, gid glyph.ID) iter.Seq[path.Contour] {
	if int(gid) >= len(q.gg) || seen[gid] {
		return func(yield func(path.Contour) bool) {}
	}
	seen[gid] = true

	g := q.gg[gid]
	if g == nil { // blank glyph
		return func(yield func(path.Contour) bool) {}
	}

	switch g := g.Data.(type) {
	case SimpleGlyph:
		return g.Contours()

	case CompositeGlyph:
		return q.compositeContours(seen, g)

	default:
		panic("invalid glyph data type")
	}
}

func (q *glyphPath) compositeContours(seen map[glyph.ID]bool, g CompositeGlyph) iter.Seq[path.Contour] {
	return func(yield func(path.Contour) bool) {
		for _, comp := range g.Components {
			transform := [6]float64{1, 0, 0, 1, 0, 0} // identity matrix

			data := comp.Data
			idx := 0

			readInt16 := func(i int) float64 {
				if i+1 >= len(data) {
					return 0
				}
				u := uint16(data[i])<<8 | uint16(data[i+1])
				return float64(int16(u))
			}

			// Read offset/alignment
			if comp.Flags&FlagArgsAreXYValues != 0 {
				if comp.Flags&FlagArg1And2AreWords != 0 {
					if idx+3 >= len(data) {
						return
					}
					transform[4] = readInt16(idx)
					transform[5] = readInt16(idx + 2)
					idx += 4
				} else {
					if idx+1 >= len(data) {
						return
					}
					transform[4] = float64(int8(data[idx]))
					transform[5] = float64(int8(data[idx+1]))
					idx += 2
				}
			}

			// Read transformation
			if comp.Flags&FlagWeHaveAScale != 0 {
				if idx+1 >= len(data) {
					return
				}
				scale := readInt16(idx) / 16384.0
				transform[0], transform[3] = scale, scale
				idx += 2
			} else if comp.Flags&FlagWeHaveAnXAndYScale != 0 {
				if idx+3 >= len(data) {
					return
				}
				transform[0] = readInt16(idx) / 16384.0
				transform[3] = readInt16(idx+2) / 16384.0
				idx += 4
			} else if comp.Flags&FlagWeHaveATwoByTwo != 0 {
				if idx+7 >= len(data) {
					return
				}
				for i := range 4 {
					transform[i] = readInt16(idx+i*2) / 16384.0
				}
				idx += 8
			}

			// Apply transformation to component contours
			for contour := range q.do(seen, comp.GlyphIndex) {
				if !yield(transformContour(contour, transform)) {
					return
				}
			}
		}
	}
}

func transformContour(contour path.Contour, t [6]float64) path.Contour {
	return func(yield func(path.Command, []path.Point) bool) {
		var buf [3]path.Point
		for cmd, pts := range contour {
			for i, p := range pts {
				buf[i] = path.Point{
					X: p.X*t[0] + p.Y*t[2] + t[4],
					Y: p.X*t[1] + p.Y*t[3] + t[5],
				}
			}
			if !yield(cmd, buf[:len(pts)]) {
				return
			}
		}
	}
}
