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
	"cmp"
	"errors"
	"math"
	"slices"

	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/vec"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
)

// maxStemHints is the largest total number of stem hints (horizontal and
// vertical combined) a Type 2 charstring can use in one glyph.
const maxStemHints = 96

// minFontMatrixDet is the smallest determinant a font matrix can have without
// being treated as singular.  A matrix with this determinant maps a unit
// square of glyph space to an area of 10^-12 square text space units.
const minFontMatrixDet = 1e-12

// FromType1 converts a Type 1 font into a name-keyed CFF font.
//
// The glyphs are ordered as in [type1.Outlines.GlyphList]: the ".notdef" glyph
// comes first, followed by the glyphs of the built-in encoding in code order,
// followed by the remaining glyphs in alphabetical order.  If the font has no
// ".notdef" glyph, a blank glyph half an em wide is inserted.  The resulting
// font has a single private dictionary and is not CID-keyed; use
// [Outlines.MakeCIDKeyed] to add a CID mapping.
//
// The result shares no data with f, so the two fonts can be modified
// independently.
//
// Information which CFF cannot represent is lost: sub-paths are always
// closed, the vertical advance width of a glyph is discarded, and stem hints
// are reduced to the form Type 2 charstrings allow (sorted, non-overlapping,
// at most 96 stems per glyph).
func FromType1(f *type1.Font) (*Font, error) {
	if f == nil || f.FontInfo == nil || f.Outlines == nil {
		return nil, errors.New("incomplete Type 1 font")
	}
	if f.MM != nil {
		return nil, errors.New("multiple master font not instantiated")
	}

	glyphNames := f.GlyphList()

	// A name-keyed CFF font needs a string identifier for every glyph name and
	// top dictionary string which is not one of the standard strings.  Since
	// glyph names are unique, this limits the number of glyphs more tightly
	// than the 16-bit glyph count of the CharStrings INDEX does.
	strings := &cffStrings{}
	makeTopDict(f.FontInfo).registerStrings(strings)
	for _, name := range glyphNames {
		strings.lookup(name)
	}
	if err := strings.check(); err != nil {
		return nil, err
	}

	m := f.FontInfo.FontMatrix
	det := math.Abs(m[0]*m[3] - m[1]*m[2])
	if !(det > minFontMatrixDet) { // the negated test also rejects NaN
		return nil, errors.New("singular font matrix")
	}

	// Width used for a synthesised ".notdef" glyph.  Advance widths are
	// measured along the glyph space x-axis, which the font matrix maps to a
	// vector of length hypot(m[0], m[1]), so this is half an em wide.  The
	// matrix is not singular, so the length is non-zero.
	notdefWidth := 0.5 / math.Hypot(m[0], m[1])

	glyphs := make([]*Glyph, len(glyphNames))
	gidByName := make(map[string]glyph.ID, len(glyphNames))
	for i, name := range glyphNames {
		g := &Glyph{Name: name}
		if orig := f.Glyphs[name]; orig != nil {
			convertGlyph(g, orig)
		} else {
			g.Width = notdefWidth
		}
		glyphs[i] = g
		gidByName[name] = glyph.ID(i)
	}

	encoding := make([]glyph.ID, 256)
	for code, name := range f.Encoding {
		if code >= 256 {
			break
		}
		encoding[code] = gidByName[name]
	}

	// CFF omits private dictionary entries which equal the default value,
	// so a missing Type 1 private dictionary maps to the CFF defaults.
	var private *type1.PrivateDict
	if f.Private != nil {
		private = clone(f.Private)
		private.BlueValues = slices.Clone(f.Private.BlueValues)
		private.OtherBlues = slices.Clone(f.Private.OtherBlues)

		// A Type 1 private dictionary always holds effective values: the
		// reader fills in omitted entries with their defaults.  Zero is a
		// legal BlueShift and BlueFuzz, so these are passed through, but a
		// BlueScale of zero is invalid and is replaced by the default.
		if private.BlueScale <= 0 {
			private.BlueScale = defaultBlueScale
		}
	} else {
		private = &type1.PrivateDict{
			BlueScale: defaultBlueScale,
			BlueShift: defaultBlueShift,
			BlueFuzz:  defaultBlueFuzz,
		}
	}

	res := &Font{
		FontInfo: clone(f.FontInfo),
		Outlines: &Outlines{
			Glyphs:   glyphs,
			Private:  []*type1.PrivateDict{private},
			FDSelect: fdSelectSimple,
			Encoding: encoding,
		},
	}
	return res, nil
}

// convertGlyph fills in the width, stem hints and outline of dst from the
// Type 1 glyph src.
func convertGlyph(dst *Glyph, src *type1.Glyph) {
	dst.Width = src.WidthX
	dst.HStem = convertStems(src.HStem)
	dst.VStem = convertStems(src.VStem)

	// cap the combined stem count at the Type 2 limit
	if nH, nV := len(dst.HStem)/2, len(dst.VStem)/2; nH+nV > maxStemHints {
		nH = min(nH, maxStemHints)
		dst.HStem = dst.HStem[:2*nH]
		dst.VStem = dst.VStem[:2*(maxStemHints-nH)]
		if len(dst.VStem) == 0 {
			dst.VStem = nil
		}
	}

	// CFF charstrings close sub-paths implicitly, so no command is written for
	// path.CmdClose.  Closing a sub-path returns the current point to the start
	// of the sub-path, so drawing commands which follow without an intervening
	// move need an explicit move to that point.
	var start vec.Vec2
	needMoveTo := false
	for cmd, pts := range src.Path().ToCubic() {
		if needMoveTo && cmd != path.CmdMoveTo && cmd != path.CmdClose {
			dst.MoveTo(start.X, start.Y)
			needMoveTo = false
		}

		switch cmd {
		case path.CmdMoveTo:
			start = pts[0]
			needMoveTo = false
			dst.MoveTo(pts[0].X, pts[0].Y)
		case path.CmdLineTo:
			dst.LineTo(pts[0].X, pts[0].Y)
		case path.CmdCubeTo:
			dst.CurveTo(pts[0].X, pts[0].Y, pts[1].X, pts[1].Y, pts[2].X, pts[2].Y)
		case path.CmdClose:
			needMoveTo = true
		}
	}
}

// convertStems converts Type 1 stem hints to the form Type 2 charstrings
// require: the edge pairs are sorted by increasing bottom edge, and pairs
// which overlap an earlier pair are dropped.  Type 1 hint replacement leaves
// the hints of all replacement groups accumulated in the glyph, so such
// pairs are common in real fonts.
func convertStems(stems []funit.Int16) []float64 {
	n := len(stems) / 2
	if n == 0 {
		return nil
	}

	pairs := make([][2]float64, n)
	for i := range pairs {
		pairs[i] = [2]float64{float64(stems[2*i]), float64(stems[2*i+1])}
	}
	// Ghost hints list their edges in descending order, so the bottom edge
	// of a pair is the smaller of the two values.
	bottom := func(p [2]float64) float64 { return min(p[0], p[1]) }
	top := func(p [2]float64) float64 { return max(p[0], p[1]) }
	slices.SortStableFunc(pairs, func(p, q [2]float64) int {
		return cmp.Compare(bottom(p), bottom(q))
	})

	res := make([]float64, 0, 2*n)
	var last [2]float64
	for i, p := range pairs {
		if i > 0 && (bottom(p) < top(last) || p == last) {
			continue
		}
		res = append(res, p[0], p[1])
		last = p
	}
	return res
}
