// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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

package type1

import (
	"fmt"
	"math"
	"time"

	"seehuhn.de/go/sfnt/funit"
)

type Font struct {
	CreationDate time.Time
	Info         *FontInfo
	Private      *PrivateDict
	Glyphs       map[string]*Glyph
	Encoding     []string
}

// Glyph represents a glyph in a Type 1 font.
type Glyph struct {
	Cmds   []GlyphOp
	HStem  []funit.Int16
	VStem  []funit.Int16
	WidthX funit.Int16
	WidthY funit.Int16
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

func (g *Glyph) ClosePath() {
	g.Cmds = append(g.Cmds, GlyphOp{Op: OpClosePath})
}

// Extent computes the Glyph extent in font design units
func (g *Glyph) Extent() funit.Rect16 {
	var left, right, top, bottom float64
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
		if first || x < left {
			left = x
		}
		if first || x > right {
			right = x
		}
		if first || y < bottom {
			bottom = y
		}
		if first || y > top {
			top = y
		}
		first = false
	}
	return funit.Rect16{
		LLx: funit.Int16(math.Floor(left)),
		LLy: funit.Int16(math.Floor(bottom)),
		URx: funit.Int16(math.Ceil(right)),
		URy: funit.Int16(math.Ceil(top)),
	}
}

// GlyphOp is a Type 1 glyph drawing command.
type GlyphOp struct {
	Op   GlyphOpType
	Args []float64
}

// GlyphOpType is the type of a Type 1 glyph drawing command.
type GlyphOpType byte

func (op GlyphOpType) String() string {
	switch op {
	case OpMoveTo:
		return "moveto"
	case OpLineTo:
		return "lineto"
	case OpCurveTo:
		return "curveto"
	case OpClosePath:
		return "closepath"
	default:
		return fmt.Sprintf("CommandType(%d)", op)
	}
}

const (
	// OpMoveTo tarts a new subpath at the given point.
	OpMoveTo GlyphOpType = iota + 1

	// OpLineTo appends a straight line segment from the previous point to the
	// given point.
	OpLineTo

	// OpCurveTo appends a Bezier curve segment from the previous point to the
	// given point.
	OpCurveTo

	// OpClosePath closes the current subpath by appending a straight line from
	// the current point to the starting point of the current subpath.  This
	// does not change the current point.
	OpClosePath
)

func (c GlyphOp) String() string {
	return fmt.Sprint("cmd", c.Args, c.Op)
}
