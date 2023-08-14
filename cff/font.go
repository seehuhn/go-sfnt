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
	"fmt"
	"math"
	"strings"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
)

// TODO(voss): implement support for font matrices

// Font stores a CFF font.
//
// TODO(voss): make this more similar to type1.Font
type Font struct {
	*type1.FontInfo
	*Outlines
}

func (f *Font) BBox() (bbox funit.Rect16) {
	first := true
	for _, glyph := range f.Glyphs {
		glyphBox := glyph.Extent()
		if glyphBox.IsZero() {
			continue
		}
		if first {
			bbox = glyphBox
		} else {
			bbox.Extend(glyphBox)
		}
	}
	return bbox
}

// Widths returns the widths of all glyphs.
func (cff *Font) Widths() []funit.Int16 {
	res := make([]funit.Int16, len(cff.Glyphs))
	for i, glyph := range cff.Glyphs {
		res[i] = glyph.Width
	}
	return res
}

func (cff *Font) Subset(keep func(glyph.ID) bool) *Font {
	origOutline := cff.Outlines
	subsetOutlines := &Outlines{}

	subsetOutlines.Private = origOutline.Private
	if origOutline.IsCIDKeyed() {
		var origGid []glyph.ID
		origGid = append(origGid, 0) // .notdef
		for gid := 1; gid < len(origOutline.Glyphs); gid++ {
			if keep(glyph.ID(gid)) {
				origGid = append(origGid, glyph.ID(gid))
			}
		}

		subsetOutlines.FDSelect = func(gid glyph.ID) int {
			return origOutline.FDSelect(origGid[gid])
		}
		subsetOutlines.ROS = origOutline.ROS
		subsetOutlines.Gid2Cid = make([]type1.CID, len(origGid))
		for gid, origGid := range origGid {
			subsetOutlines.Glyphs = append(subsetOutlines.Glyphs, origOutline.Glyphs[origGid])
			subsetOutlines.Gid2Cid[gid] = origOutline.Gid2Cid[origGid]
		}
	} else {
		subsetGid := make([]glyph.ID, len(origOutline.Glyphs))
		for gid := 0; gid < len(origOutline.Glyphs); gid++ {
			if gid == 0 || keep(glyph.ID(gid)) {
				subsetGid[gid] = glyph.ID(len(subsetOutlines.Glyphs))
				subsetOutlines.Glyphs = append(subsetOutlines.Glyphs, origOutline.Glyphs[gid])
			}
		}
		subsetOutlines.FDSelect = func(glyph.ID) int {
			return 0
		}
		subsetOutlines.Encoding = make([]glyph.ID, 256)
		for i, gid := range origOutline.Encoding {
			subsetOutlines.Encoding[i] = subsetGid[gid]
		}
	}

	subset := &Font{
		FontInfo: cff.FontInfo,
		Outlines: subsetOutlines,
	}
	return subset
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
	ROS *type1.CIDSystemInfo

	// Gid2Cid lists the character identifiers corresponding to the glyphs.
	// This is only present for CIDFonts, and encodes the information from the
	// charset table in the CFF font.  When present, the first entry
	// (corresponding to the .notdef glyph) must be 0.
	Gid2Cid []type1.CID
}

// IsCIDKeyed returns true if the font is a CID-keyed font.
func (o *Outlines) IsCIDKeyed() bool {
	return o.ROS != nil
}

// Glyph represents a glyph in a CFF font.
type Glyph struct {
	Name  string
	Cmds  []GlyphOp
	HStem []funit.Int16
	VStem []funit.Int16
	Width funit.Int16
}

// NewGlyph allocates a new glyph.
func NewGlyph(name string, width funit.Int16) *Glyph {
	return &Glyph{
		Name:  name,
		Width: width,
	}
}

func (g *Glyph) String() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "Glyph %q (width %d):\n", g.Name, g.Width)
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

// GlyphOp is a CFF glyph drawing command.
type GlyphOp struct {
	Op   GlyphOpType
	Args []float64
}

// GlyphOpType is the type of a CFF glyph drawing command.
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
