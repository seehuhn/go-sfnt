// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2024  Jochen Voss <voss@seehuhn.de>
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
	"fmt"
	"math"
	"strings"

	"seehuhn.de/go/postscript/funit"
)

// Glyph represents a glyph in a CFF font.
type Glyph struct {
	Name  string
	Cmds  []GlyphOp
	HStem []float64
	VStem []float64
	Width float64
}

// NewGlyph allocates a new glyph.
func NewGlyph(name string, width float64) *Glyph {
	return &Glyph{
		Name:  name,
		Width: width,
	}
}

func (g *Glyph) String() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "Glyph %q (width %v):\n", g.Name, g.Width)
	fmt.Fprintf(b, "  HStem: %v\n", g.HStem)
	fmt.Fprintf(b, "  HStem: %v\n", g.VStem)
	for i, cmd := range g.Cmds {
		fmt.Fprintf(b, "  Cmds[%d]: %s\n", i, cmd)
	}
	return b.String()
}

// MoveTo starts a new sub-path and moves the current point to (x, y).
// The previous sub-path, if any, is closed.
func (g *Glyph) MoveTo(x, y float64) {
	g.Cmds = append(g.Cmds, GlyphOp{
		Op:   OpMoveTo,
		Args: []float64{x, y},
	})
}

// LineTo adds a straight line to the current sub-path.
func (g *Glyph) LineTo(x, y float64) {
	g.Cmds = append(g.Cmds, GlyphOp{
		Op:   OpLineTo,
		Args: []float64{x, y},
	})
}

// CurveTo adds a cubic Bezier curve to the current sub-path.
func (g *Glyph) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	g.Cmds = append(g.Cmds, GlyphOp{
		Op:   OpCurveTo,
		Args: []float64{x1, y1, x2, y2, x3, y3},
	})
}

// Extent computes the Glyph extent in font design units, as the bounding box
// of the outline.  Bézier control points are included, so the box always
// contains the glyph.
func (g *Glyph) Extent() funit.Rect16 {
	bbox := g.Path().BBox()
	return funit.Rect16{
		LLx: funit.Int16(math.Floor(bbox.LLx)),
		LLy: funit.Int16(math.Floor(bbox.LLy)),
		URx: funit.Int16(math.Ceil(bbox.URx)),
		URy: funit.Int16(math.Ceil(bbox.URy)),
	}
}

// GlyphOp is a CFF glyph drawing command.
type GlyphOp struct {
	Op   GlyphOpType
	Args []float64
}

// GlyphOpType is the type of a CFF glyph drawing command.
//
// TODO(voss): merge this with the Type 1 command type?
type GlyphOpType byte

func (op GlyphOpType) String() string {
	switch op {
	case OpMoveTo:
		return "moveto"
	case OpLineTo:
		return "lineto"
	case OpCurveTo:
		return "curveto"
	case OpHintMask:
		return "hintmask"
	case OpCntrMask:
		return "cntrmask"
	default:
		return fmt.Sprintf("CommandType(%d)", op)
	}
}

const (
	// OpMoveTo closes the previous subpath and starts a new one at the given point.
	OpMoveTo GlyphOpType = iota + 1

	// OpLineTo appends a straight line segment from the previous point to the given point.
	OpLineTo

	// OpCurveTo appends a Bezier curve segment from the previous point to the given point.
	OpCurveTo

	// OpHintMask adds a CFF hintmask command.
	OpHintMask

	// OpCntrMask adds a CFF cntrmask command.
	OpCntrMask
)

func (c GlyphOp) String() string {
	return fmt.Sprint("cmd", c.Args, c.Op)
}
