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

package debug

import (
	"seehuhn.de/go/geom/matrix"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/hvar"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/variation"
)

// The glyph IDs of the variable CFF2 font built by [MakeVarCFF2Font].
const (
	VarCFF2GidNotdef glyph.ID = 0 // an empty glyph, no variation
	VarCFF2GidBox    glyph.ID = 1 // straight-line box, Font DICT 0, inherited vsindex
	VarCFF2GidCurve  glyph.ID = 2 // curves and stem hints, Font DICT 0, vsindex 1
	VarCFF2GidTwo    glyph.ID = 3 // two subpaths, Font DICT 0, vsindex 1
	VarCFF2GidFD1    glyph.ID = 4 // Font DICT 1, overriding its vsindex back to 0
)

// The item variation data subtables of the CFF2 variation store.  A charstring
// blend carries one delta per region of the subtable its vsindex selects, so
// the two counts differ and a glyph which picks the wrong one decodes garbage.
const (
	varCFF2VSIndexTwo = 0 // regions: wght = +1, wdth = -1
	varCFF2VSIndexAll = 1 // the same two plus an intermediate wght region
)

// MakeVarCFF2Font builds a variable CFF2 font covering the parts of the format
// where the blend machinery can go wrong: two axes, two item variation data
// subtables of different width, an intermediate region which stays at zero over
// the lower half of the wght axis, two Font DICTs with their own private dicts,
// glyphs which select either subtable, blended stem hints behind a hintmask,
// cubic curves, and a glyph of two subpaths.
//
// A CFF2 font carries no phantom points, so the advance varies through HVAR
// alone and the two cannot disagree the way they can in a glyf font.
func MakeVarCFF2Font() *sfnt.Font {
	// wght rises from the default to +1, wdth falls from the default to -1, so
	// the two named axis extremes reach opposite ends of the design space.
	regionWght := variation.Region{
		{Start: 0, Peak: f2(1), End: f2(1)},
		{Start: 0, Peak: 0, End: 0},
	}
	regionWdth := variation.Region{
		{Start: 0, Peak: 0, End: 0},
		{Start: f2(-1), Peak: f2(-1), End: 0},
	}
	// an intermediate region: zero until wght passes the midpoint, so a delta
	// attached to it separates linear interpolation from the real thing
	regionWghtTop := variation.Region{
		{Start: f2(0.5), Peak: f2(1), End: f2(1)},
		{Start: 0, Peak: 0, End: 0},
	}

	b := func(v float64) cff.Blend { return cff.Blend{Default: v} }
	// b2 and b3 carry one delta per region of the subtable named by their
	// vsindex; the counts must match or the decoder mis-reads the operands
	b2 := func(v, wght, wdth float64) cff.Blend {
		return cff.Blend{Default: v, Deltas: []float64{wght, wdth}}
	}
	b3 := func(v, wght, wdth, wghtTop float64) cff.Blend {
		return cff.Blend{Default: v, Deltas: []float64{wght, wdth, wghtTop}}
	}

	notdef := &cff.GlyphCFF2{Cmds: []cff.GlyphOpCFF2{
		{Op: cff.OpMoveTo, Args: []cff.Blend{b(0), b(0)}},
	}}

	// right edge 500 -> 600 at wght = +1 and -> 440 at wdth = -1
	box := &cff.GlyphCFF2{
		VSIndex: varCFF2VSIndexTwo,
		Cmds: []cff.GlyphOpCFF2{
			{Op: cff.OpMoveTo, Args: []cff.Blend{b(0), b(0)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b2(500, 100, -60), b(0)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b2(500, 100, -60), b(700)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b(0), b(700)}},
		},
	}

	// Curves under the wider subtable.  The second control point of the first
	// curve moves only through the intermediate region, so it stays put over
	// the lower half of the wght axis and then catches up.
	curve := &cff.GlyphCFF2{
		VSIndex: varCFF2VSIndexAll,
		HStem:   []cff.Blend{b(0), b3(60, 10, 0, 15)},
		VStem:   []cff.Blend{b(40), b(100)},
		Cmds: []cff.GlyphOpCFF2{
			{Op: cff.OpHintMask, Args: []cff.Blend{b(0x80)}},
			{Op: cff.OpMoveTo, Args: []cff.Blend{b(50), b(0)}},
			{Op: cff.OpCurveTo, Args: []cff.Blend{
				b(150), b3(200, 40, 0, 0),
				b(350), b3(400, 0, 0, 80),
				b3(450, 50, -20, 0), b(600),
			}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b3(450, 50, -20, 0), b(700)}},
			{Op: cff.OpCurveTo, Args: []cff.Blend{
				b(300), b(760),
				b(100), b(760),
				b(50), b3(700, 0, -30, 0),
			}},
		},
	}

	// two subpaths, so that the implicit closepath between them is exercised
	two := &cff.GlyphCFF2{
		VSIndex: varCFF2VSIndexAll,
		Cmds: []cff.GlyphOpCFF2{
			{Op: cff.OpMoveTo, Args: []cff.Blend{b(0), b(0)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b3(200, 30, -10, 25), b(0)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b3(200, 30, -10, 25), b(300)}},
			{Op: cff.OpMoveTo, Args: []cff.Blend{b(300), b(400)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b3(500, 40, -20, 0), b(400)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b(300), b3(650, 25, 0, 0)}},
		},
	}

	// the only glyph of the second Font DICT, overriding that dict's non-zero
	// vsindex back to the narrower subtable
	fd1 := &cff.GlyphCFF2{
		VSIndex: varCFF2VSIndexTwo,
		Cmds: []cff.GlyphOpCFF2{
			{Op: cff.OpMoveTo, Args: []cff.Blend{b(100), b(100)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b2(400, 60, -40), b(100)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b2(400, 60, -40), b2(500, 0, 80)}},
			{Op: cff.OpLineTo, Args: []cff.Blend{b(100), b2(500, 0, 80)}},
		},
	}

	// Font DICT 0 defaults to the narrower subtable and Font DICT 1 to the
	// wider one, so the private dicts differ in vsindex as well as in content.
	//
	// No glyph inherits a non-zero Font DICT default, even though the spec
	// allows it: fontTools reads charstrings assuming vsindex 0 until an
	// explicit operator says otherwise (still so in 4.65), which would leave
	// the oracle in internal/varexpect unable to read this font at all.  That
	// path is covered in the cff package instead, by
	// TestReadCFF2PrivateVSIndex and TestWriteCFF2InheritedVSIndex.
	o := &cff.OutlinesCFF2{
		Glyphs: []*cff.GlyphCFF2{notdef, box, curve, two, fd1},
		Widths: []float64{600, 550, 620, 700, 480},
		Private: []*cff.PrivateCFF2{
			{StdHW: b(50), StdVW: b(60)},
			{VSIndex: varCFF2VSIndexAll, StdHW: b3(70, 12, -6, 4), StdVW: b(80)},
		},
		FDSelect: func(gid glyph.ID) int {
			if gid >= VarCFF2GidFD1 {
				return 1
			}
			return 0
		},
		VarStore: &variation.ItemVariationStore{
			Regions: []variation.Region{regionWght, regionWdth, regionWghtTop},
			Data: []*variation.ItemVariationData{
				varCFF2VSIndexTwo: {RegionIndexes: []uint16{0, 1}, Deltas: [][]int32{}},
				varCFF2VSIndexAll: {RegionIndexes: []uint16{0, 1, 2}, Deltas: [][]int32{}},
			},
		},
	}

	font := &sfnt.Font{
		FamilyName:         "CFF2Var",
		FullName:           "CFF2Var Regular",
		CreationTime:       fixtureDate,
		ModificationTime:   fixtureDate,
		Width:              os2.WidthNormal,
		Weight:             os2.WeightNormal,
		UnitsPerEm:         1000,
		Ascent:             700,
		Descent:            -300,
		LineGap:            100,
		CapHeight:          700,
		XHeight:            500,
		UnderlinePosition:  -100,
		UnderlineThickness: 50,
		FontMatrix:         matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		Outlines:           o,
	}
	font.Fvar = &fvar.Table{
		Axes: []fvar.Axis{
			{Tag: "wght", Min: 100, Default: 400, Max: 900, Name: "Weight"},
			{Tag: "wdth", Min: 75, Default: 100, Max: 100, Name: "Width"},
		},
	}
	// HVAR moves the advances independently of the outlines, so a reader which
	// took the advance from the charstring would show up here.
	font.Hvar = &hvar.Table{
		Store: &variation.ItemVariationStore{
			Regions: []variation.Region{regionWght, regionWdth},
			Data: []*variation.ItemVariationData{
				{
					RegionIndexes: []uint16{0, 1},
					Deltas: [][]int32{
						{0, 0},    // notdef: no variation
						{50, -40}, // box
						{30, -25}, // curve
						{20, -10}, // two
						{45, -15}, // fd1
					},
				},
			},
		},
		AdvanceMap: &variation.DeltaSetIndexMap{Map: []uint32{0, 1, 2, 3, 4}},
	}
	return font
}
