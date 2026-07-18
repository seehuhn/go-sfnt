// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
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
	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/geom/vec"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/variation"
)

// Blend is a blendable number: a default value plus one delta per region of
// the item variation data subtable selected by the effective vsindex.
//
// Values are stored in absolute coordinates with absolute deltas.  The
// charstring decoder converts relative charstring operands into absolute
// positions by prefix-summing Default and each Deltas[i] independently;
// blending is linear, so this is exact.  The Deltas slice is aligned to the
// region list of the ItemVariationData subtable selected by the glyph's or
// dict's VSIndex, not the global region list.
type Blend struct {
	Default float64
	Deltas  []float64 // nil == does not vary
}

// At returns the blended value for the given per-region scalars.
//
// A length mismatch between scalars and Deltas is tolerated: only the common
// prefix contributes.
func (b Blend) At(scalars []float64) float64 {
	v := b.Default
	n := min(len(scalars), len(b.Deltas))
	for i := range n {
		v += scalars[i] * b.Deltas[i]
	}
	return v
}

// GlyphOpCFF2 mirrors GlyphOp with blendable arguments.
type GlyphOpCFF2 struct {
	Op   GlyphOpType // reuse OpMoveTo/OpLineTo/OpCurveTo/OpHintMask/OpCntrMask
	Args []Blend
}

// GlyphCFF2 is one glyph of a CFF2 font.  Glyphs have no names and no
// charstring width; widths live in OutlinesCFF2.Widths (from hmtx).
type GlyphCFF2 struct {
	Cmds    []GlyphOpCFF2
	HStem   []Blend // blended stem edges, absolute positions
	VStem   []Blend
	VSIndex int // effective item variation data subtable index
}

// PrivateCFF2 holds a CFF2 private dict with blendable values.
type PrivateCFF2 struct {
	BlueValues, OtherBlues, FamilyBlues, FamilyOtherBlues []Blend
	BlueScale, BlueShift, BlueFuzz                        Blend
	StdHW, StdVW                                          Blend
	StemSnapH, StemSnapV                                  []Blend
	LanguageGroup                                         int32
	ExpansionFactor                                       Blend
	VSIndex                                               int // dict-level vsindex (op 22), default 0
}

// newPrivateCFF2 returns a CFF2 private dict initialised with the spec default
// values for the scalar blend entries.
func newPrivateCFF2() *PrivateCFF2 {
	return &PrivateCFF2{
		BlueScale:       Blend{Default: 0.039625},
		BlueShift:       Blend{Default: 7},
		BlueFuzz:        Blend{Default: 1},
		ExpansionFactor: Blend{Default: 0.06},
	}
}

// OutlinesCFF2 stores the glyph data of a CFF2 font.
type OutlinesCFF2 struct {
	Glyphs       []*GlyphCFF2
	Widths       []float64      // advance widths in design units, from hmtx; may be nil
	Private      []*PrivateCFF2 // one per Font DICT; len >= 1
	FDSelect     FDSelectFn
	FontMatrices []matrix.Matrix               // per-FD matrices (identity where absent)
	VarStore     *variation.ItemVariationStore // nil for a non-variable CFF2 font
}

// FontCFF2 pairs CFF2 outlines with the top-DICT font matrix.
type FontCFF2 struct {
	FontMatrix matrix.Matrix
	*OutlinesCFF2
}

// NumGlyphs returns the number of glyphs in the font.
func (o *OutlinesCFF2) NumGlyphs() int {
	return len(o.Glyphs)
}

// IsBlank returns true if the glyph with the given ID does not add marks to
// the page.  An out-of-range ID is treated as the .notdef glyph.
func (o *OutlinesCFF2) IsBlank(gid glyph.ID) bool {
	if int(gid) >= len(o.Glyphs) {
		gid = 0 // .notdef
	}
	return len(o.Glyphs[gid].Cmds) == 0
}

// Path returns the glyph outline as a path.Path iterator, rendering the
// default instance (using each argument's Default value only).  An
// out-of-range ID is treated as the .notdef glyph.
func (o *OutlinesCFF2) Path(gid glyph.ID) path.Path {
	if int(gid) >= len(o.Glyphs) {
		gid = 0 // .notdef
	}
	g := o.Glyphs[gid]
	if g == nil {
		return func(yield func(path.Command, []vec.Vec2) bool) {}
	}
	return g.Path()
}

// Path returns the default-instance glyph outline as a path.Path iterator,
// converting the CFF2 glyph commands (including Bézier control points) to path
// commands using each argument's Default value only.
func (g *GlyphCFF2) Path() path.Path {
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
					buf[0] = vec.Vec2{X: cmd.Args[0].Default, Y: cmd.Args[1].Default}
					if !yield(path.CmdMoveTo, buf[:1]) {
						return
					}
					hasSubpath = true
				}
			case OpLineTo:
				if len(cmd.Args) >= 2 {
					buf[0] = vec.Vec2{X: cmd.Args[0].Default, Y: cmd.Args[1].Default}
					if !yield(path.CmdLineTo, buf[:1]) {
						return
					}
				}
			case OpCurveTo:
				if len(cmd.Args) >= 6 {
					buf[0] = vec.Vec2{X: cmd.Args[0].Default, Y: cmd.Args[1].Default} // control point 1
					buf[1] = vec.Vec2{X: cmd.Args[2].Default, Y: cmd.Args[3].Default} // control point 2
					buf[2] = vec.Vec2{X: cmd.Args[4].Default, Y: cmd.Args[5].Default} // end point
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

// GlyphBBox computes the bounding box of the default-instance glyph, after the
// matrix M has been applied to the glyph outline.
//
// If the glyph is blank, the zero rectangle is returned.
func (o *OutlinesCFF2) GlyphBBox(M matrix.Matrix, gid glyph.ID) rect.Rect {
	return o.Path(gid).Transform([6]float64(M)).BBox()
}

// GlyphBBoxPDF computes the bounding box of the default-instance glyph in PDF
// glyph space units (1/1000th of a text space unit).  The font matrix M is
// applied to the glyph outline.
//
// If the glyph is blank, the zero rectangle is returned.
func (o *OutlinesCFF2) GlyphBBoxPDF(M matrix.Matrix, gid glyph.ID) (bbox rect.Rect) {
	M = o.GlyphMatrix(M, gid).Mul(matrix.Scale(1000, 1000))
	return o.GlyphBBox(M, gid)
}

// GlyphMatrix returns the effective font matrix for the given glyph, composing
// any per-FD matrix with the supplied top-level font matrix.  When the
// selected FD has no per-FD matrix, the top matrix is returned unchanged.
func (o *OutlinesCFF2) GlyphMatrix(top matrix.Matrix, gid glyph.ID) matrix.Matrix {
	if o.FDSelect == nil {
		return top
	}
	return o.FDMatrix(o.FDSelect(gid), top)
}

// FDMatrix returns the effective font matrix for FD index fd, composing the
// per-FD matrix with the supplied top-level font matrix.  If the font has no
// per-FD matrix for fd, the top matrix is returned unchanged.
func (o *OutlinesCFF2) FDMatrix(fd int, top matrix.Matrix) matrix.Matrix {
	if fd >= 0 && fd < len(o.FontMatrices) {
		return o.FontMatrices[fd].Mul(top)
	}
	return top
}

// Clone creates a new font, consisting of a shallow copy of the outlines with
// freshly copied top-level slices.
func (f *FontCFF2) Clone() *FontCFF2 {
	outlines := f.OutlinesCFF2.clone()
	return &FontCFF2{
		FontMatrix:   f.FontMatrix,
		OutlinesCFF2: outlines,
	}
}

// clone returns a shallow copy with freshly copied top-level slices.  The
// glyph, private-dict and variation-store elements are shared with the
// original.
func (o *OutlinesCFF2) clone() *OutlinesCFF2 {
	res := *o
	res.Glyphs = append([]*GlyphCFF2(nil), o.Glyphs...)
	res.Widths = append([]float64(nil), o.Widths...)
	res.Private = append([]*PrivateCFF2(nil), o.Private...)
	res.FontMatrices = append([]matrix.Matrix(nil), o.FontMatrices...)
	return &res
}
