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
func (cff *Font) Widths() []funit.Int16 {
	res := make([]funit.Int16, len(cff.Glyphs))
	for i, glyph := range cff.Glyphs {
		res[i] = glyph.Width
	}
	return res
}

// GlyphWidthPDF returns the advance width of a glyph in PDF text space units.
func (cff *Font) GlyphWidthPDF(gid glyph.ID) float64 {
	return float64(cff.Glyphs[gid].Width) * cff.FontMatrix[0]
}

// WidthsPDF returns the advance widths of the glyphs in the font,
// in PDF text space units.
func (cff *Font) WidthsPDF() []float64 {
	widths := make([]float64, cff.NumGlyphs())
	for gid, g := range cff.Glyphs {
		widths[gid] = float64(g.Width) * cff.FontMatrix[0]
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
