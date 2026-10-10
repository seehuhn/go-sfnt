// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2025  Jochen Voss <voss@seehuhn.de>
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
	"slices"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/geom/vec"

	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
)

// Outlines stores the glyph data of a CFF font.
//
// There are two cases:
//   - For a simple font, Encoding is used, and ROS, GIDToCID, and FontMatrices
//     must be nil.  In this case FDSelect always returns 0.
//   - For CID-keyed fonts, ROS, GIDToCID, and FontMatrices are in use,
//     and Encoding must be nil.
type Outlines struct {
	// Glyphs contains the glyph outlines of the font, indexed by glyph ID.
	// All entries must be non-nil.
	Glyphs []*Glyph

	// Private stores the private dictionaries of the font.
	// The length of this slice must be at least one.
	// For a simple font the length is exactly one.
	Private []*type1.PrivateDict

	// FDSelect determines which private dictionary is used for each glyph.
	// For a simple font, this function always returns 0.
	FDSelect FDSelectFn

	// Encoding lists the glyphs corresponding to the 256 one-byte character
	// codes.  If present, the length of this slice must be 256.  Entries
	// for unused character codes must be set to 0.
	//
	// This is only used for simple fonts.
	Encoding []glyph.ID

	// ROS specifies the character collection of the font, using Adobe's
	// Registry, Ordering, Supplement system.
	//
	// This is only used for CID-keyed fonts.
	ROS *cid.SystemInfo

	// GIDToCID lists the character identifiers corresponding to the glyphs.
	// When present, the first entry (corresponding to the .notdef glyph) must
	// be 0.
	//
	// This is only used for CID-keyed fonts.
	GIDToCID []cid.CID

	// FontMatrices lists the font matrices corresponding to each private
	// dictionary.  The matrices are applied before the font matrix from
	// the Font.FontInfo structure.
	//
	// This is only used for CID-keyed fonts.
	FontMatrices []matrix.Matrix
}

// Clone returns a copy of the outlines which can be converted, subsetted or
// re-encoded without changing the original.
//
// The slices are copied, but the glyphs and private dictionaries they point to
// are shared: a caller which needs a glyph to differ replaces it, the way
// [Outlines.SetGlyphName] does.
func (o *Outlines) Clone() *Outlines {
	other := *o
	other.Glyphs = slices.Clone(o.Glyphs)
	other.Private = slices.Clone(o.Private)
	other.Encoding = slices.Clone(o.Encoding)
	other.GIDToCID = slices.Clone(o.GIDToCID)
	other.FontMatrices = slices.Clone(o.FontMatrices)
	return &other
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

// NumGlyphs returns the number of glyphs in the font.
func (o *Outlines) NumGlyphs() int {
	return len(o.Glyphs)
}

// SetGlyphName gives the glyph the given name.
//
// The glyph is replaced rather than written to: glyphs are commonly shared
// between fonts, for example between a font and a subset of it, and a name
// written in place would reach every font which has the glyph.
func (o *Outlines) SetGlyphName(gid glyph.ID, name string) {
	if o.Glyphs[gid].Name == name {
		return
	}
	g := clone(o.Glyphs[gid])
	g.Name = name
	o.Glyphs[gid] = g
}

// Path returns the glyph outline as a path.Path iterator.
// This converts CFF glyph commands to path commands.
func (o *Outlines) Path(gid glyph.ID) path.Path {
	return o.Glyphs[gid].Path()
}

// Path returns the glyph outline as a path.Path iterator, converting the CFF
// glyph commands (including Bézier control points) to path commands.
func (g *Glyph) Path() path.Path {
	return func(yield func(path.Command, []vec.Vec2) bool) {
		var buf [3]vec.Vec2
		hasSubpath := false // track whether we have an open subpath

		for _, cmd := range g.Cmds {
			switch cmd.Op {
			case OpMoveTo:
				if len(cmd.Args) >= 2 {
					// in CFF, MoveTo implicitly closes the previous subpath
					if hasSubpath {
						if !yield(path.CmdClose, nil) {
							return
						}
					}
					buf[0] = vec.Vec2{X: cmd.Args[0], Y: cmd.Args[1]}
					if !yield(path.CmdMoveTo, buf[:1]) {
						return
					}
					hasSubpath = true
				}
			case OpLineTo:
				if len(cmd.Args) >= 2 {
					buf[0] = vec.Vec2{X: cmd.Args[0], Y: cmd.Args[1]}
					if !yield(path.CmdLineTo, buf[:1]) {
						return
					}
				}
			case OpCurveTo:
				if len(cmd.Args) >= 6 {
					buf[0] = vec.Vec2{X: cmd.Args[0], Y: cmd.Args[1]} // control point 1
					buf[1] = vec.Vec2{X: cmd.Args[2], Y: cmd.Args[3]} // control point 2
					buf[2] = vec.Vec2{X: cmd.Args[4], Y: cmd.Args[5]} // end point
					if !yield(path.CmdCubeTo, buf[:3]) {
						return
					}
				}
			}
		}

		// close the final subpath
		if hasSubpath {
			if !yield(path.CmdClose, nil) {
				return
			}
		}
	}
}

// GlyphBBox computes the bounding box of a glyph, after the matrix M has been
// applied to the glyph outline.  For a CID-keyed font, M must already include
// the per-FD font matrix; use [Outlines.GlyphMatrix] to compose it.
//
// For a glyph which draws nothing, the zero rectangle is returned.
func (o *Outlines) GlyphBBox(M matrix.Matrix, gid glyph.ID) rect.Rect {
	return o.Path(gid).Transform([6]float64(M)).BBox()
}

// GlyphBBoxPDF computes the bounding box of a glyph in PDF glyph space units
// (1/1000th of a text space unit).
// The font matrix M is applied to the glyph outline.
//
// For a glyph which draws nothing, the zero rectangle is returned.
func (o *Outlines) GlyphBBoxPDF(M matrix.Matrix, gid glyph.ID) (bbox rect.Rect) {
	M = o.GlyphMatrix(M, gid).Mul(matrix.Scale(1000, 1000))
	return o.GlyphBBox(M, gid)
}

// GlyphMatrix returns the effective font matrix for the given glyph,
// composing any per-FD matrix with the supplied top-level font matrix.
// For non-CID-keyed fonts, or when the selected FD has no per-FD matrix
// (a malformed font), the top matrix is returned unchanged.
func (o *Outlines) GlyphMatrix(top matrix.Matrix, gid glyph.ID) matrix.Matrix {
	if !o.IsCIDKeyed() {
		return top
	}
	return o.FDMatrix(o.FDSelect(gid), top)
}

// FDMatrix returns the effective font matrix for FD index fd, composing the
// per-FD matrix with the supplied top-level font matrix.  If the font has no
// per-FD matrices, the top matrix is returned unchanged.
func (o *Outlines) FDMatrix(fd int, top matrix.Matrix) matrix.Matrix {
	if fd >= 0 && fd < len(o.FontMatrices) {
		return o.FontMatrices[fd].Mul(top)
	}
	return top
}

// GlyphAdvanceScale returns the scalar factor that converts a glyph's
// CFF design-unit advance width to text space units.  It accounts for any
// per-FD font matrix as well as shear in the supplied top-level matrix.
//
// Multiply by 1000 to obtain PDF glyph space units.
func (o *Outlines) GlyphAdvanceScale(top matrix.Matrix, gid glyph.ID) float64 {
	return advanceScale(o.GlyphMatrix(top, gid))
}

// advanceScale returns the factor which converts an advance width in glyph
// design units to text space units, for a glyph whose effective font matrix
// is fm.
func advanceScale(fm matrix.Matrix) float64 {
	q := fm[0]
	if math.Abs(fm[3]) > 1e-6 {
		q -= fm[1] * fm[2] / fm[3]
	}
	return q
}

// usableFontMatrix reports whether fm, the effective font matrix of a glyph,
// has finite entries and a finite advance scale.  Each entry a font file gives
// is finite, but composing a per-FD matrix with the top-level matrix can
// overflow, and so can the shear correction in [advanceScale].  A glyph whose
// matrix fails the test has neither a usable outline nor a usable advance
// width.
func usableFontMatrix(fm matrix.Matrix) bool {
	for _, x := range fm {
		if math.IsInf(x, 0) || math.IsNaN(x) {
			return false
		}
	}
	q := advanceScale(fm)
	return !math.IsInf(q, 0) && !math.IsNaN(q)
}

// repairFontMatrices replaces font matrices read from a file which leave a
// glyph without a usable effective font matrix (see [usableFontMatrix]).  The
// top-level matrix top is composed with each of the per-FD matrices fds, or
// used on its own if there are none; a matrix which is replaced gets the value
// a reader assumes when the matrix is absent, topDefault or fdDefault.
//
// The top-level matrix is at fault if it fails even with the default per-FD
// matrix.  Otherwise, a failing composition is the fault of the per-FD matrix.
func repairFontMatrices(top *matrix.Matrix, fds []matrix.Matrix, topDefault, fdDefault matrix.Matrix) {
	probe := *top
	if len(fds) > 0 {
		probe = fdDefault.Mul(*top)
	}
	if !usableFontMatrix(probe) {
		*top = topDefault
	}
	for i, fd := range fds {
		if !usableFontMatrix(fd.Mul(*top)) {
			fds[i] = fdDefault
		}
	}
}
