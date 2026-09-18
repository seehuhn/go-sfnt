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
	"seehuhn.de/go/geom/vec"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/maxp"
	"seehuhn.de/go/sfnt/parser"
)

const glyfAlign = 2

// Outlines stores the glyph data of a TrueType font.
type Outlines struct {
	// Glyphs is a slice of glyph outlines in the font.
	// Nil entries denote blank glyphs.
	Glyphs Glyphs

	// Widths contains the glyph widths, indexed by glyph ID.
	Widths []funit.Uint16

	// Names, if non-nil, contains the glyph names.  The slice then has one
	// entry per glyph; an empty string denotes a glyph with no name.
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

// GlyphMatrix returns the effective font matrix for the given glyph.  Glyf
// outlines are always drawn on the design grid, so the top-level font matrix
// is returned unchanged.
func (o *Outlines) GlyphMatrix(top matrix.Matrix, gid glyph.ID) matrix.Matrix {
	return top
}

// GlyphBBoxPDF computes the bounding box of a glyph in PDF glyph space units
// (1/1000th of a font unit).
// The font matrix fm is applied to the glyph bounding box from the font data.
//
// For a glyph without data, the zero rectangle is returned.
func (o *Outlines) GlyphBBoxPDF(fm matrix.Matrix, gid glyph.ID) (bbox rect.Rect) {
	M := fm.Mul(matrix.Scale(1000, 1000))
	return o.GlyphBBox(M, gid)
}

// GlyphBBox computes the bounding box of a glyph, after the matrix M has been
// applied to the glyph outline.  The box is the one recorded in the glyph
// header, which need not be tight and may be non-empty even for a glyph with
// no outline.
//
// For a glyph without data, the zero rectangle is returned.
func (o *Outlines) GlyphBBox(M matrix.Matrix, gid glyph.ID) (bbox rect.Rect) {
	g := o.Glyphs[gid]
	if g == nil { // blank glyph
		return
	}

	stored := rect.Rect{
		LLx: float64(g.LLx),
		LLy: float64(g.LLy),
		URx: float64(g.URx),
		URy: float64(g.URy),
	}
	return stored.Transform(M)
}

// Glyphs contains a slice of TrueType glyph outlines.
// This represents the information stored in the "glyf" and "loca" tables
// of a TrueType font.  A nil entry denotes a blank glyph.
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

	gg.dropMissingComponents()

	return gg, nil
}

// dropMissingComponents removes component references to glyphs which are not
// part of the font.  A composite glyph left without any components becomes a
// blank glyph.
func (gg Glyphs) dropMissingComponents() {
	for i, g := range gg {
		if g == nil {
			continue
		}
		d, ok := g.Data.(CompositeGlyph)
		if !ok {
			continue
		}

		valid := true
		for _, comp := range d.Components {
			if int(comp.GlyphIndex) >= len(gg) {
				valid = false
				break
			}
		}
		if valid {
			continue
		}

		var kept []GlyphComponent
		for _, comp := range d.Components {
			if int(comp.GlyphIndex) < len(gg) {
				kept = append(kept, comp)
			}
		}
		if kept == nil {
			gg[i] = nil
			continue
		}
		d.Components = kept
		g.Data = d
	}
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
			// the layout flags follow from the position in the list and from
			// the presence of instructions, so any stored value is ignored
			flags := comp.Flags &^ (FlagMoreComponents | FlagWeHaveInstructions)
			if i < len(d.Components)-1 {
				flags |= FlagMoreComponents
			} else if d.Instructions != nil {
				flags |= FlagWeHaveInstructions
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

// Path returns the glyph outline.
// For composite glyphs, this recursively includes all component glyphs
// with their transformations applied.
func (o *Outlines) Path(gid glyph.ID) path.Path {
	if o.Glyphs[gid] == nil { // blank glyph
		return path.Empty
	}

	if g, ok := o.Glyphs[gid].Data.(SimpleGlyph); ok {
		return g.Path()
	}

	return func(yield func(path.Command, []vec.Vec2) bool) {
		// allocate separate state for each call of the iterator
		st := &pathState{
			onPath: make(map[glyph.ID]bool),
			budget: pathBudgetPerGlyph*len(o.Glyphs) + pathBudgetBase,
		}
		for cmd, pts := range o.Glyphs.pathRecursive(st, gid) {
			if !yield(cmd, pts) {
				return
			}
		}
	}
}

// pathState holds the state used while expanding one composite glyph outline.
type pathState struct {
	// onPath contains the composite glyphs currently being expanded, so that
	// recursive references can be broken.  A glyph is removed again once its
	// components have been emitted: a glyph referenced twice by the same
	// parent contributes its outline twice.
	onPath map[glyph.ID]bool

	// budget limits the total number of glyphs expanded.
	budget int
}

// Expanding one glyph outline is limited to
// pathBudgetPerGlyph*len(Glyphs)+pathBudgetBase glyph expansions.  Recursion
// is already excluded by pathState.onPath, but a chain of composite glyphs
// which each reference the next one more than once still grows exponentially
// with the nesting depth.
//
// A survey of 4401 TrueType fonts found no glyph needing more than 65
// expansions or more than five levels of nesting, so the limit is far out of
// reach for real fonts.
const (
	pathBudgetPerGlyph = 2
	pathBudgetBase     = 1000
)

func (gg Glyphs) pathRecursive(st *pathState, gid glyph.ID) path.Path {
	if int(gid) >= len(gg) || st.budget <= 0 {
		return path.Empty
	}
	st.budget--

	g := gg[gid]
	if g == nil { // blank glyph
		return path.Empty
	}

	switch g := g.Data.(type) {
	case SimpleGlyph:
		return g.Path()

	case CompositeGlyph:
		// The iterator is evaluated lazily, so gid has to be marked while the
		// components are emitted rather than while the path is constructed.
		return func(yield func(path.Command, []vec.Vec2) bool) {
			if st.onPath[gid] {
				return
			}
			st.onPath[gid] = true
			defer delete(st.onPath, gid)

			for cmd, pts := range gg.compositePath(st, gid, g) {
				if !yield(cmd, pts) {
					return
				}
			}
		}

	default:
		panic("invalid glyph data type")
	}
}

func (gg Glyphs) compositePath(st *pathState, gid glyph.ID, g CompositeGlyph) path.Path {
	return func(yield func(path.Command, []vec.Vec2) bool) {
	componentLoop:
		for _, comp := range g.Components {
			M := [6]float64{1, 0, 0, 1, 0, 0} // identity matrix

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
				M[4], M[5] = dx, dy
			} else {
				// Point matching case - arguments are point indices
				var ourPointIdx, theirPointIdx uint16
				var ok bool
				if comp.Flags&FlagArg1And2AreWords != 0 {
					// 16-bit point indices
					val, ok1 := readInt16()
					if !ok1 {
						continue // skip malformed component
					}
					ourPointIdx = uint16(val)
					val, ok = readInt16()
					if !ok {
						continue // skip malformed component
					}
					theirPointIdx = uint16(val)
				} else {
					// 8-bit point indices
					val, ok1 := readInt8()
					if !ok1 {
						continue // skip malformed component
					}
					ourPointIdx = uint16(val)
					val, ok = readInt8()
					if !ok {
						continue // skip malformed component
					}
					theirPointIdx = uint16(val)
				}

				// Calculate offset from point matching
				ourPoint, ourOk := gg.getGlyphPoint(gid, ourPointIdx)
				theirPoint, theirOk := gg.getGlyphPoint(comp.GlyphIndex, theirPointIdx)
				if ourOk && theirOk {
					M[4] = ourPoint.X - theirPoint.X
					M[5] = ourPoint.Y - theirPoint.Y
				}
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
				M[0], M[3] = scale, scale

			case comp.Flags&FlagWeHaveAnXAndYScale != 0:
				// Separate X and Y scaling
				xScale, ok1 := readInt16()
				yScale, ok2 := readInt16()
				if !ok1 || !ok2 {
					continue componentLoop // skip malformed component
				}
				M[0] = xScale / 16384.0
				M[3] = yScale / 16384.0

			case comp.Flags&FlagWeHaveATwoByTwo != 0:
				// Full 2x2 transformation matrix
				for i := range 4 {
					val, ok := readInt16()
					if !ok {
						continue componentLoop // skip malformed component
					}
					M[i] = val / 16384.0
				}
			}

			// Apply transformation to component paths
			componentPath := gg.pathRecursive(st, comp.GlyphIndex)
			transformedPath := componentPath.Transform(M)
			for cmd, pts := range transformedPath {
				if !yield(cmd, pts) {
					return
				}
			}
		}
	}
}

// getGlyphPoint extracts the coordinates of a specific point from a glyph.
// Returns the point coordinates and true if successful, or false if the
// point index is invalid or the glyph cannot be processed.
func (gg Glyphs) getGlyphPoint(gid glyph.ID, pointIdx uint16) (point struct{ X, Y float64 }, ok bool) {
	if int(gid) >= len(gg) || gg[gid] == nil {
		return point, false
	}

	g := gg[gid]
	switch gData := g.Data.(type) {
	case SimpleGlyph:
		unpacked, err := gData.Unpack()
		if err != nil {
			return point, false
		}

		// Find the point in the contours
		pointCount := uint16(0)
		for _, contour := range unpacked.Contours {
			if pointIdx < pointCount+uint16(len(contour)) {
				pt := contour[pointIdx-pointCount]
				return struct{ X, Y float64 }{float64(pt.X), float64(pt.Y)}, true
			}
			pointCount += uint16(len(contour))
		}
		return point, false

	case CompositeGlyph:
		// For composite glyphs, we need to collect all points from components
		// This is more complex as it requires applying transformations.

		// TODO(voss): implement this.
		return point, false

	default:
		panic("unexpected glyph type")
	}
}
