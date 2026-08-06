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
	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/avar"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/cvar"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/gvar"
	"seehuhn.de/go/sfnt/hvar"
	"seehuhn.de/go/sfnt/maxp"
	"seehuhn.de/go/sfnt/mvar"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/stat"
	"seehuhn.de/go/sfnt/variation"

	"golang.org/x/text/language"
)

// The glyph IDs of the synthetic variable font built by [MakeVarFont].
const (
	VarGidNotdef glyph.ID = 0 // a plain box, no variation
	VarGidRect   glyph.ID = 1 // rectangle with an all-points wght tuple
	VarGidIUP    glyph.ID = 2 // rectangle with a subset tuple requiring IUP
	VarGidComp   glyph.ID = 3 // composite of Rect + IUP, varying offset
	VarGidTwo    glyph.ID = 4 // two tuples, one an intermediate region
	VarGidSwap   glyph.ID = 5 // target of the FeatureVariations substitution
)

// f2 converts a normalized coordinate in [-1, 1] to F2Dot14.
func f2(x float64) variation.F2Dot14 { return variation.F2Dot14FromFloat(x) }

// simpleGlyph builds an all-on-curve single-contour glyph from the given
// points and computes its bounding box.
func simpleGlyph(pts ...[2]funit.Int16) *glyf.Glyph {
	contour := make(glyf.Contour, len(pts))
	for i, p := range pts {
		contour[i] = glyf.Point{X: p[0], Y: p[1], OnCurve: true}
	}
	su := &glyf.SimpleUnpacked{Contours: []glyf.Contour{contour}}
	g := su.AsGlyph()
	return &g
}

// MakeVarFont builds a deterministic, self-contained TrueType variable font
// exercising every variation table wired into [sfnt.Font].  The design is
// hand-computable: see [MakeVarFontExpect] for the expected instanced values.
//
// The font has two axes (wght 100–900 default 400, wdth 75–125 default 100)
// and six glyphs.  It attaches fvar, avar, STAT, gvar, cvar, HVAR, MVAR, a GDEF
// item variation store, a GSUB with a FeatureVariations substitution, and a
// GPOS kern pair carrying a VariationIndex device entry.
func MakeVarFont() *sfnt.Font {
	// six hand-designed glyphs (single contour, all on-curve)
	notdef := simpleGlyph([2]funit.Int16{0, 0}, [2]funit.Int16{500, 0},
		[2]funit.Int16{500, 700}, [2]funit.Int16{0, 700})
	rect := simpleGlyph([2]funit.Int16{100, 0}, [2]funit.Int16{400, 0},
		[2]funit.Int16{400, 700}, [2]funit.Int16{100, 700})
	iup := simpleGlyph([2]funit.Int16{200, 0}, [2]funit.Int16{600, 0},
		[2]funit.Int16{600, 800}, [2]funit.Int16{200, 800})
	two := simpleGlyph([2]funit.Int16{100, 0}, [2]funit.Int16{500, 0},
		[2]funit.Int16{500, 600}, [2]funit.Int16{100, 600})
	swap := simpleGlyph([2]funit.Int16{50, 0}, [2]funit.Int16{450, 0},
		[2]funit.Int16{450, 500}, [2]funit.Int16{50, 500})

	// composite: Rect at (0,0) plus IUP at (300,50)
	comp0 := (&glyf.ComponentUnpacked{
		Child: VarGidRect,
		Trfm:  matrix.Matrix{1, 0, 0, 1, 0, 0},
	}).Pack()
	comp1 := (&glyf.ComponentUnpacked{
		Child: VarGidIUP,
		Trfm:  matrix.Matrix{1, 0, 0, 1, 300, 50},
	}).Pack()
	comp := &glyf.Glyph{
		// union of Rect (100,0)-(400,700) and IUP+(300,50) (500,50)-(900,850)
		Rect16: funit.Rect16{LLx: 100, LLy: 0, URx: 900, URy: 850},
		Data:   glyf.CompositeGlyph{Components: []glyf.GlyphComponent{comp0, comp1}},
	}

	glyphs := glyf.Glyphs{notdef, rect, iup, comp, two, swap}
	widths := []funit.Uint16{600, 500, 700, 800, 550, 480}
	names := []string{".notdef", "rect", "iup", "comp", "two", "swap"}

	// cvt: two entries, one varied by cvar
	cvt := []byte{0x00, 0x64, 0x00, 0xC8} // 100, 200

	outlines := &glyf.Outlines{
		Glyphs: glyphs,
		Widths: widths,
		Names:  names,
		Tables: map[string][]byte{"cvt ": cvt},
		Maxp: &maxp.TTFInfo{
			MaxPoints:            4,
			MaxContours:          1,
			MaxCompositePoints:   8,
			MaxCompositeContours: 2,
			MaxComponentElements: 2,
			MaxComponentDepth:    1,
		},
	}

	f := &sfnt.Font{
		FamilyName:         "QuireVar",
		Subfamily:          "Regular",
		FullName:           "QuireVar Regular",
		FontName:           "QuireVar-Regular",
		Width:              os2.WidthNormal,
		Weight:             os2.WeightNormal,
		IsRegular:          true,
		UnitsPerEm:         1000,
		FontMatrix:         matrix.Scale(0.001, 0.001),
		Ascent:             800,
		Descent:            -200,
		LineGap:            200,
		CapHeight:          700,
		XHeight:            500,
		UnderlinePosition:  -100,
		UnderlineThickness: 50,
		Outlines:           outlines,
	}

	m := cmap.Format4{}
	m['A'] = VarGidRect
	m['B'] = VarGidIUP
	m['C'] = VarGidComp
	m['D'] = VarGidTwo
	m['E'] = VarGidSwap
	f.InstallCMap(m)

	f.Fvar = &fvar.Table{
		Axes: []fvar.Axis{
			{Tag: "wght", Min: 100, Default: 400, Max: 900, Name: "Weight"},
			{Tag: "wdth", Min: 75, Default: 100, Max: 125, Name: "Width"},
		},
		Instances: []fvar.Instance{
			{
				Coordinates:      []float64{700, 75},
				PostScriptNameID: 0xFFFF,
				Name:             "Bold Narrow",
				PostScriptName:   "QuireVar-BoldNarrow",
			},
		},
	}

	// avar: non-identity on wght, identity on wdth.  The wght map bends the
	// upper half so that avar visibly changes results for wght > 400.
	f.Avar = &avar.Table{
		SegmentMaps: []avar.SegmentMap{
			{
				{From: f2(-1), To: f2(-1)},
				{From: f2(0), To: f2(0)},
				{From: f2(0.5), To: f2(0.75)},
				{From: f2(1), To: f2(1)},
			},
			{
				{From: f2(-1), To: f2(-1)},
				{From: f2(0), To: f2(0)},
				{From: f2(1), To: f2(1)},
			},
		},
	}

	f.Stat = &stat.Table{
		DesignAxes: []stat.DesignAxis{
			{Tag: "wght", Name: "Weight"},
			{Tag: "wdth", Ordering: 1, Name: "Width"},
		},
		AxisValues: []stat.AxisValue{
			&stat.Format1{AxisIndex: 0, Value: 400, Name: "Regular"},
			&stat.Format2{AxisIndex: 1, Nominal: 100, Min: 75, Max: 125, Name: "Normal"},
		},
		ElidedFallbackName: "Regular",
	}

	f.Gvar = makeGvar()

	// cvar: vary cvt entry 1 only, by +50 at wght = +1
	f.Cvar = &cvar.Table{
		AxisCount: 2,
		Tuples: []variation.TupleVariation{
			{
				Peak:   []variation.F2Dot14{f2(1), 0},
				Points: []uint16{1},
				Deltas: []int32{50},
			},
		},
	}

	// HVAR: two regions (wght extreme, wdth extreme); advances vary for Rect
	// and Comp.
	f.Hvar = &hvar.Table{
		Store: &variation.ItemVariationStore{
			Regions: []variation.Region{regionWght, regionWdth},
			Data: []*variation.ItemVariationData{
				{
					RegionIndexes: []uint16{0, 1},
					Deltas: [][]int32{
						{0, 0},    // inner 0: no variation
						{80, -30}, // inner 1: Rect  -> +50 at the extreme
						{40, 20},  // inner 2: Comp  -> +60 at the extreme
					},
				},
			},
		},
		AdvanceMap: &variation.DeltaSetIndexMap{
			// per gid: outer<<16 | inner
			Map: []uint32{0, 1, 0, 2, 0, 0},
		},
	}

	// MVAR: hasc (+50) and cpht (-15) at the extreme
	f.Mvar = &mvar.Table{
		Store: &variation.ItemVariationStore{
			Regions: []variation.Region{regionWght, regionWdth},
			Data: []*variation.ItemVariationData{
				{
					RegionIndexes: []uint16{0, 1},
					Deltas: [][]int32{
						{50, 0},  // inner 0: hasc
						{0, -15}, // inner 1: cpht
					},
				},
			},
		},
		// Encode sorts Records by Tag; list them pre-sorted so the built font
		// matches the round-tripped one.
		Records: []mvar.Record{
			{Tag: "cpht", OuterIndex: 0, InnerIndex: 1},
			{Tag: "hasc", OuterIndex: 0, InnerIndex: 0},
		},
	}

	f.VariationsPostScriptName = "QuireVar"

	// GDEF item variation store, referenced by the GPOS VariationIndex.
	f.Gdef = &gdef.Table{
		ItemVarStore: &variation.ItemVariationStore{
			Regions: []variation.Region{regionWght, regionWdth},
			Data: []*variation.ItemVariationData{
				{
					RegionIndexes: []uint16{0, 1},
					Deltas:        [][]int32{{25, -10}}, // +15 at the extreme
				},
			},
		},
	}

	// GPOS: a kern pair (Rect, IUP) whose x-advance carries a VariationIndex
	// device entry into the GDEF store.
	f.Gpos = &gtab.Info{
		ScriptList: gtab.ScriptListInfo{
			language.MustParse("und-Latn"): {Required: 0xFFFF, Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: gtab.FeatureListInfo{
			{Tag: "kern", Lookups: []gtab.LookupIndex{0}},
		},
		LookupList: gtab.LookupList{
			{
				Meta: &gtab.LookupMetaInfo{LookupType: 2},
				Subtables: []gtab.Subtable{
					gtab.Gpos2_1{
						glyph.Pair{Left: VarGidRect, Right: VarGidIUP}: &gtab.PairAdjust{
							First: &gtab.GposValueRecord{
								XAdvance: -40,
								XAdvanceDev: &device.Table{
									OuterIndex:  0,
									InnerIndex:  0,
									DeltaFormat: device.VariationIndexFormat,
								},
							},
						},
					},
				},
			},
		},
	}

	// GSUB: a FeatureVariations record that swaps Rect -> Swap when wght is in
	// [0.5, 1.0] (normalized, post-avar).  The default feature has no lookups,
	// so the substitution is observable only inside the condition region.
	f.Gsub = &gtab.Info{
		ScriptList: gtab.ScriptListInfo{
			language.MustParse("und-Latn"): {Required: 0xFFFF, Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: gtab.FeatureListInfo{
			{Tag: "rvrn", Lookups: []gtab.LookupIndex{}},
		},
		LookupList: gtab.LookupList{
			{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gsub1_1{Cov: coverage.Set{VarGidRect: true}, Delta: VarGidSwap - VarGidRect},
				},
			},
		},
		Variations: []gtab.FeatureVariationRecord{
			{
				Conditions: []gtab.Condition{
					{Format: 1, AxisIndex: 0, Min: f2(0.5), Max: f2(1)},
				},
				Substitutions: []gtab.FeatureSubstitution{
					{FeatureIndex: 0, Lookups: []gtab.LookupIndex{0}},
				},
			},
		},
	}

	return f
}

// the two shared HVAR/MVAR/GDEF regions, both peaking at 1 at the
// wght=900/wdth=75 extreme.
var (
	// wght {0, +1, +1}; wdth inactive
	regionWght = variation.Region{
		{Start: 0, Peak: f2(1), End: f2(1)},
		{Start: 0, Peak: 0, End: 0},
	}
	// wdth {-1, -1, 0}; wght inactive
	regionWdth = variation.Region{
		{Start: 0, Peak: 0, End: 0},
		{Start: f2(-1), Peak: f2(-1), End: 0},
	}
)

// makeGvar builds the per-glyph gvar blocks.  Outline points only carry deltas;
// the four phantom points are left at zero so advance variation comes solely
// from HVAR.
func makeGvar() *gvar.Table {
	// Rect (gid 1): all-points tuple, top edge rises by +200 at wght = +1.
	// 8 points (4 outline + 4 phantom); deltas are [x0..x7, y0..y7].
	rectBlock := mustEncodeTuples([]variation.TupleVariation{
		{
			Peak: []variation.F2Dot14{f2(1), 0},
			Deltas: []int32{
				0, 0, 0, 0, 0, 0, 0, 0, // x
				0, 0, 200, 200, 0, 0, 0, 0, // y (top corners +200)
			},
		},
	})

	// IUP (gid 2): subset tuple touching points 0 and 2 only; points 1 and 3
	// are filled by IUP.  Deltas are [dx0, dx2, dy0, dy2].
	iupBlock := mustEncodeTuples([]variation.TupleVariation{
		{
			Peak:   []variation.F2Dot14{f2(1), 0},
			Points: []uint16{0, 2},
			Deltas: []int32{-30, 40, 0, 150},
		},
	})

	// Comp (gid 3): all-points tuple over 2 components + 4 phantom = 6 points;
	// component 1's offset shifts by (+200, -100) at wght = +1.
	compBlock := mustEncodeTuples([]variation.TupleVariation{
		{
			Peak: []variation.F2Dot14{f2(1), 0},
			Deltas: []int32{
				0, 200, 0, 0, 0, 0, // x
				0, -100, 0, 0, 0, 0, // y
			},
		},
	})

	// Two (gid 4): tuple A (wght, top +100) and tuple B (an intermediate
	// region on both axes, right edge -80 at wght=+1 & wdth=-1).
	twoBlock := mustEncodeTuples([]variation.TupleVariation{
		{
			Peak: []variation.F2Dot14{f2(1), 0},
			Deltas: []int32{
				0, 0, 0, 0, 0, 0, 0, 0, // x
				0, 0, 100, 100, 0, 0, 0, 0, // y
			},
		},
		{
			Peak:              []variation.F2Dot14{f2(1), f2(-1)},
			IntermediateStart: []variation.F2Dot14{f2(0.5), f2(-1)},
			IntermediateEnd:   []variation.F2Dot14{f2(1), 0},
			Deltas: []int32{
				0, -80, -80, 0, 0, 0, 0, 0, // x (right corners -80)
				0, 0, 0, 0, 0, 0, 0, 0, // y
			},
		},
	})

	perGlyph := make([]gvar.GlyphData, 6)
	perGlyph[VarGidRect] = gvar.GlyphData{Data: rectBlock}
	perGlyph[VarGidIUP] = gvar.GlyphData{Data: iupBlock}
	perGlyph[VarGidComp] = gvar.GlyphData{Data: compBlock}
	perGlyph[VarGidTwo] = gvar.GlyphData{Data: twoBlock}

	return &gvar.Table{AxisCount: 2, PerGlyph: perGlyph}
}

func mustEncodeTuples(tuples []variation.TupleVariation) []byte {
	data, err := variation.EncodeTupleData(tuples, 2, 2, 0, nil)
	if err != nil {
		panic(err)
	}
	return data
}

// VarFontExpect holds the hand-computed expected values for the font returned
// by [MakeVarFont], evaluated at the wght=900/wdth=75 instance.  The comments
// show the scalar arithmetic behind each value.
type VarFontExpect struct {
	// InstanceCoords are the normalized axis coordinates at wght=900/wdth=75,
	// after fvar normalization and the avar segment map.
	//
	// wght: (900-400)/(900-400) = +1.0; avar maps +1 -> +1.
	// wdth: (75-100)/(100-75)   = -1.0; avar identity.
	InstanceCoords []variation.F2Dot14

	// IUPPoints is the instanced outline of the IUP glyph (gid 2) at the
	// instance coordinates.  The wght tuple (scalar 1) touches points 0 and 2;
	// IUP fills 1 and 3.
	//
	//   p0 = (200,0)   + (-30,   0) = (170,   0)
	//   p1 = (600,0)   + (+40,   0) = (640,   0)   [x from p2, y from p0]
	//   p2 = (600,800) + (+40,+150) = (640, 950)
	//   p3 = (200,800) + (-30,+150) = (170, 950)   [x from p0, y from p2]
	IUPPoints []glyf.Point

	// TwoPoints is the instanced outline of gid 4 at the instance coordinates,
	// with both tuples active (scalar 1 each).
	//
	//   p0 = (100,0)   + (   0,   0) = (100,   0)
	//   p1 = (500,0)   + ( -80,   0) = (420,   0)
	//   p2 = (500,600) + ( -80,+100) = (420, 700)
	//   p3 = (100,600) + (   0,+100) = (100, 700)
	TwoPoints []glyf.Point

	// advance widths with HVAR applied at the instance coordinates.  Both
	// regions have scalar 1, so the advance delta is the sum of the two
	// per-region deltas.
	RectAdvanceDefault  funit.Uint16 // 500
	RectAdvanceInstance funit.Uint16 // 500 + (80 - 30) = 550
	CompAdvanceDefault  funit.Uint16 // 800
	CompAdvanceInstance funit.Uint16 // 800 + (40 + 20) = 860

	// MVAR deltas at the instance coordinates.
	HascDelta float64 // 50*1 + 0*1  = 50
	CphtDelta float64 // 0*1 + -15*1 = -15

	// cvt values before and after cvar, at the instance coordinates.
	// entry 1 rises by +50 (scalar 1); entry 0 is untouched.
	CVTDefault  []int16 // {100, 200}
	CVTInstance []int16 // {100, 250}

	// FeatureVariations substitution: Rect -> Swap when wght >= 0.5 (post-avar).
	SubstInGID     glyph.ID          // Rect (1)
	SubstOutGID    glyph.ID          // Swap (5)
	SubstThreshold variation.F2Dot14 // +0.5 normalized

	// GPOS kern pair (Rect, IUP): static x-advance and its VariationIndex delta.
	KernLeft          glyph.ID    // Rect (1)
	KernRight         glyph.ID    // IUP (2)
	KernXAdvance      funit.Int16 // -40
	KernDeltaInstance float64     // 25*1 + -10*1 = 15
}

// MakeVarFontExpect returns the hand-computed expected values for the font
// built by [MakeVarFont] at its wght=900/wdth=75 instance.
func MakeVarFontExpect() VarFontExpect {
	return VarFontExpect{
		InstanceCoords: []variation.F2Dot14{f2(1), f2(-1)},

		IUPPoints: []glyf.Point{
			{X: 170, Y: 0, OnCurve: true},
			{X: 640, Y: 0, OnCurve: true},
			{X: 640, Y: 950, OnCurve: true},
			{X: 170, Y: 950, OnCurve: true},
		},
		TwoPoints: []glyf.Point{
			{X: 100, Y: 0, OnCurve: true},
			{X: 420, Y: 0, OnCurve: true},
			{X: 420, Y: 700, OnCurve: true},
			{X: 100, Y: 700, OnCurve: true},
		},

		RectAdvanceDefault:  500,
		RectAdvanceInstance: 550,
		CompAdvanceDefault:  800,
		CompAdvanceInstance: 860,

		HascDelta: 50,
		CphtDelta: -15,

		CVTDefault:  []int16{100, 200},
		CVTInstance: []int16{100, 250},

		SubstInGID:     VarGidRect,
		SubstOutGID:    VarGidSwap,
		SubstThreshold: f2(0.5),

		KernLeft:          VarGidRect,
		KernRight:         VarGidIUP,
		KernXAdvance:      -40,
		KernDeltaInstance: 15,
	}
}
