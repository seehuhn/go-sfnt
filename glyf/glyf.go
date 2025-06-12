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

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/maxp"
	"seehuhn.de/go/sfnt/parser"
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

// decodeGlyph decodes a glyph from binary data.
// Returns nil for empty glyphs. The function retains sub-slices of the input data.
func decodeGlyph(data []byte) (*Glyph, error) {
	if len(data) == 0 {
		return nil, nil
	} else if len(data) < 10 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/glyf",
			Reason:    "incomplete glyph header",
		}
	}

	var glyphData any
	numCont := int16(data[0])<<8 | int16(data[1])
	if numCont >= 0 {
		simple := SimpleGlyph{
			NumContours: numCont,
			Encoded:     data[10:],
		}
		err := simple.removePadding()
		if err != nil {
			return nil, err
		}
		glyphData = simple
	} else {
		comp, err := decodeGlyphComposite(data[10:])
		if err != nil {
			return nil, err
		}
		glyphData = *comp
	}

	g := &Glyph{
		Rect16: funit.Rect16{
			LLx: funit.Int16(data[2])<<8 | funit.Int16(data[3]),
			LLy: funit.Int16(data[4])<<8 | funit.Int16(data[5]),
			URx: funit.Int16(data[6])<<8 | funit.Int16(data[7]),
			URy: funit.Int16(data[8])<<8 | funit.Int16(data[9]),
		},
		Data: glyphData,
	}
	return g, nil
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

func (g *Glyph) encodeLen() int {
	if g == nil {
		return 0
	}

	total := 10
	switch d := g.Data.(type) {
	case SimpleGlyph:
		total += len(d.Encoded)
	case CompositeGlyph:
		for _, comp := range d.Components {
			total += 4 + len(comp.Data)
		}
		if d.Instructions != nil {
			total += 2 + len(d.Instructions)
		}
	default:
		panic("unexpected glyph type")
	}
	for total%glyfAlign != 0 {
		total++
	}
	return total
}

func (g *Glyph) append(buf []byte) []byte {
	if g == nil {
		return buf
	}

	var numContours int16
	switch g0 := g.Data.(type) {
	case SimpleGlyph:
		numContours = g0.NumContours
	case CompositeGlyph:
		numContours = -1
	default:
		panic("unexpected glyph type")
	}

	buf = append(buf,
		byte(numContours>>8),
		byte(numContours),
		byte(g.LLx>>8),
		byte(g.LLx),
		byte(g.LLy>>8),
		byte(g.LLy),
		byte(g.URx>>8),
		byte(g.URx),
		byte(g.URy>>8),
		byte(g.URy))

	switch d := g.Data.(type) {
	case SimpleGlyph:
		buf = append(buf, d.Encoded...)
	case CompositeGlyph:
		for i, comp := range d.Components {
			flags := comp.Flags
			// Set FlagMoreComponents for all but the last component
			if i < len(d.Components)-1 {
				flags |= FlagMoreComponents
			}
			buf = append(buf,
				byte(flags>>8), byte(flags),
				byte(comp.GlyphIndex>>8), byte(comp.GlyphIndex))
			buf = append(buf, comp.Data...)
		}
		if d.Instructions != nil {
			L := len(d.Instructions)
			buf = append(buf, byte(L>>8), byte(L))
			buf = append(buf, d.Instructions...)
		}
	default:
		panic("unexpected glyph type")
	}

	for len(buf)%glyfAlign != 0 {
		buf = append(buf, 0)
	}

	return buf
}

// Path returns a path representing the glyph outline.
// For composite glyphs, this recursively includes all component glyphs
// with their transformations applied.
func (gg Glyphs) Path(gid glyph.ID) path.Path {
	if int(gid) >= len(gg) || gg[gid] == nil {
		return func(yield func(path.Command, []path.Point) bool) {}
	}

	if g, ok := gg[gid].Data.(SimpleGlyph); ok {
		return g.Path()
	}

	return func(yield func(path.Command, []path.Point) bool) {
		// allocate a separate map for each call of the iterator
		seen := make(map[glyph.ID]bool)
		for cmd, pts := range gg.pathRecursive(seen, gid) {
			if !yield(cmd, pts) {
				return
			}
		}
	}
}

func (gg Glyphs) pathRecursive(seen map[glyph.ID]bool, gid glyph.ID) path.Path {
	if int(gid) >= len(gg) || seen[gid] {
		return func(yield func(path.Command, []path.Point) bool) {}
	}
	seen[gid] = true

	g := gg[gid]
	if g == nil { // blank glyph
		return func(yield func(path.Command, []path.Point) bool) {}
	}

	switch g := g.Data.(type) {
	case SimpleGlyph:
		return g.Path()

	case CompositeGlyph:
		return gg.compositePath(seen, g)

	default:
		panic("invalid glyph data type")
	}
}

func (gg Glyphs) compositePath(seen map[glyph.ID]bool, g CompositeGlyph) path.Path {
	return func(yield func(path.Command, []path.Point) bool) {
	componentLoop:
		for _, comp := range g.Components {
			transform := [6]float64{1, 0, 0, 1, 0, 0} // identity matrix

			data := comp.Data
			offset := 0

			// Helper to safely read int16 from data
			readInt16 := func() (float64, bool) {
				if offset+1 >= len(data) {
					return 0, false
				}
				val := uint16(data[offset])<<8 | uint16(data[offset+1])
				offset += 2
				return float64(int16(val)), true
			}

			// Helper to safely read int8 from data
			readInt8 := func() (float64, bool) {
				if offset >= len(data) {
					return 0, false
				}
				val := int8(data[offset])
				offset++
				return float64(val), true
			}

			// Read translation/offset
			if comp.Flags&FlagArgsAreXYValues != 0 {
				var dx, dy float64
				var ok bool
				if comp.Flags&FlagArg1And2AreWords != 0 {
					// 16-bit offsets
					if dx, ok = readInt16(); !ok {
						continue // skip malformed component
					}
					if dy, ok = readInt16(); !ok {
						continue // skip malformed component
					}
				} else {
					// 8-bit offsets
					if dx, ok = readInt8(); !ok {
						continue // skip malformed component
					}
					if dy, ok = readInt8(); !ok {
						continue // skip malformed component
					}
				}
				transform[4], transform[5] = dx, dy
			}

			// Read scaling/transformation
			switch {
			case comp.Flags&FlagWeHaveAScale != 0:
				// Uniform scaling
				scale, ok := readInt16()
				if !ok {
					continue // skip malformed component
				}
				scale /= 16384.0
				transform[0], transform[3] = scale, scale

			case comp.Flags&FlagWeHaveAnXAndYScale != 0:
				// Separate X and Y scaling
				xScale, ok1 := readInt16()
				yScale, ok2 := readInt16()
				if !ok1 || !ok2 {
					continue componentLoop // skip malformed component
				}
				transform[0] = xScale / 16384.0
				transform[3] = yScale / 16384.0

			case comp.Flags&FlagWeHaveATwoByTwo != 0:
				// Full 2x2 transformation matrix
				for i := 0; i < 4; i++ {
					val, ok := readInt16()
					if !ok {
						continue componentLoop // skip malformed component
					}
					transform[i] = val / 16384.0
				}
			}

			// Apply transformation to component paths
			componentPath := gg.pathRecursive(seen, comp.GlyphIndex)
			transformedPath := transformPath(componentPath, transform)
			for cmd, pts := range transformedPath {
				if !yield(cmd, pts) {
					return
				}
			}
		}
	}
}

// transformPath applies a 2D affine transformation to a path.
// The transformation matrix t is [a, b, c, d, e, f] representing:
// [a c e]   [x]
// [b d f] * [y]
// [0 0 1]   [1]
func transformPath(p path.Path, t [6]float64) path.Path {
	return func(yield func(path.Command, []path.Point) bool) {
		var buf [3]path.Point
		for cmd, pts := range p {
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

const glyfAlign = 2
