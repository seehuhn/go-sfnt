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

package gvar

import (
	"errors"
	"math"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/variation"
)

// ErrWorkLimit is returned by [Table.Apply] when the cumulative work budget is
// exhausted.
var ErrWorkLimit = errors.New("gvar: work budget exhausted")

// ApplyResult holds the outcome of applying variation deltas to one glyph.
type ApplyResult struct {
	// Glyph is the repacked instance outline.  For a simple glyph the bounding
	// box is recomputed from the varied outline.  For a composite glyph only
	// the component offsets are updated and the header bounding box is left
	// unchanged (see [Table.Apply]).  Glyph is nil when the input glyph is nil
	// (an empty glyph).
	Glyph *glyf.Glyph

	// Advance is the instance advance width, derived from the horizontal
	// phantom points.
	Advance funit.Uint16
}

// Apply computes the instance outline of glyph gid at the normalized axis
// coordinates coords.
//
// glyphs supplies the default (un-varied) outlines and widths supplies the
// default advance widths indexed by glyph ID (the value of horizontal phantom
// point 2).  For each tuple whose interpolation weight is non-zero, the tuple's
// deltas are scaled by that weight and accumulated.  Simple glyphs whose tuples
// reference only a subset of points have the remaining outline points filled in
// by inferred-delta interpolation (IUP), computed per contour against the
// original coordinates; phantom points are never inferred.  The accumulated
// deltas are rounded once and applied to the outline points and to the
// horizontal phantom points.  The two vertical phantom points are decoded and
// their deltas accumulated, but the result is discarded: vertical-metrics
// variation is out of scope.
//
// For composite glyphs the per-component X/Y offsets (components with
// ARGS_ARE_XY_VALUES) are adjusted by the rounded component deltas, widening
// the offset arguments from bytes to words when necessary.  Point-matching
// components take no offset.  The composite's header bounding box is NOT
// recomputed here, because it depends on the final child outlines; the caller
// must instantiate the child glyphs first and recompute composite bounding
// boxes in a second pass.
//
// budget bounds memory use.  workBudget, when non-nil, is decremented by the
// number of active tuples times the point count; Apply returns ErrWorkLimit
// once it reaches zero.  A nil workBudget imposes no work limit.
//
// A gid outside the gvar table's per-glyph range is treated as having no
// variation data rather than as an error.
func (t *Table) Apply(glyphs glyf.Glyphs, widths []funit.Uint16, gid glyph.ID, coords []variation.F2Dot14, budget *membudget.Budget, workBudget *int64) (*ApplyResult, error) {
	var g *glyf.Glyph
	if int(gid) < len(glyphs) {
		g = glyphs[gid]
	}

	var aw funit.Uint16
	if int(gid) < len(widths) {
		aw = widths[gid]
	}

	// point universe: outline or component points, then 4 phantom points
	var simple *glyf.SimpleUnpacked
	var composite *glyf.CompositeGlyph
	var nBase int         // number of non-phantom points
	var contourEnds []int // exclusive end index of each simple contour

	if g != nil {
		switch d := g.Data.(type) {
		case glyf.SimpleGlyph:
			su, err := d.Unpack()
			if err != nil {
				return nil, err
			}
			simple = su
			for _, c := range su.Contours {
				nBase += len(c)
				contourEnds = append(contourEnds, nBase)
			}
		case glyf.CompositeGlyph:
			composite = &d
			nBase = len(d.Components)
		default:
			panic("gvar: unexpected glyph type")
		}
	}
	nPoints := nBase + 4

	// out-of-range gid means the table simply has no data for this glyph
	var tuples []variation.TupleVariation
	if int(gid) < len(t.PerGlyph) {
		var err error
		tuples, err = t.Unpack(gid, nPoints, budget)
		if err != nil {
			return nil, err
		}
	}

	// interpolation weights and the active-tuple work charge
	scalars, err := membudget.AllocSlice[float64](budget, len(tuples))
	if err != nil {
		return nil, err
	}
	active := 0
	for i := range tuples {
		s := tuples[i].Scalar(coords, t.SharedTuples)
		scalars[i] = s
		if s != 0 {
			active++
		}
	}
	if workBudget != nil {
		*workBudget -= int64(active) * int64(nPoints)
		if *workBudget <= 0 {
			return nil, ErrWorkLimit
		}
	}

	// accumulated deltas across all tuples
	dx, err := membudget.AllocSlice[float64](budget, nPoints)
	if err != nil {
		return nil, err
	}
	dy, err := membudget.AllocSlice[float64](budget, nPoints)
	if err != nil {
		return nil, err
	}

	// original coordinates, needed for IUP of simple glyphs
	var origX, origY []float64
	if simple != nil {
		origX, err = membudget.AllocSlice[float64](budget, nPoints)
		if err != nil {
			return nil, err
		}
		origY, err = membudget.AllocSlice[float64](budget, nPoints)
		if err != nil {
			return nil, err
		}
		p := 0
		for _, c := range simple.Contours {
			for _, pt := range c {
				origX[p] = float64(pt.X)
				origY[p] = float64(pt.Y)
				p++
			}
		}
	}

	// per-tuple scratch for subset (private/shared point) tuples
	var tdx, tdy []float64
	var touched []bool
	for i := range tuples {
		if scalars[i] != 0 && tuples[i].Points != nil {
			tdx, err = membudget.AllocSlice[float64](budget, nPoints)
			if err != nil {
				return nil, err
			}
			tdy, err = membudget.AllocSlice[float64](budget, nPoints)
			if err != nil {
				return nil, err
			}
			touched, err = membudget.AllocSlice[bool](budget, nPoints)
			if err != nil {
				return nil, err
			}
			break
		}
	}

	for i := range tuples {
		s := scalars[i]
		if s == 0 {
			continue
		}
		tv := &tuples[i]

		if tv.Points == nil {
			// deltas for every point, no interpolation
			for p := range nPoints {
				dx[p] += s * float64(tv.Deltas[p])
				dy[p] += s * float64(tv.Deltas[nPoints+p])
			}
			continue
		}

		// subset: fill referenced points, IUP the rest (simple glyphs only)
		pc := len(tv.Points)
		for p := range nPoints {
			touched[p] = false
			tdx[p] = 0
			tdy[p] = 0
		}
		for k, pn := range tv.Points {
			p := int(pn) // guaranteed < nPoints by the tuple decoder
			touched[p] = true
			tdx[p] = float64(tv.Deltas[k])
			tdy[p] = float64(tv.Deltas[pc+k])
		}
		if simple != nil {
			start := 0
			for _, end := range contourEnds {
				iupContour(tdx[start:end], origX[start:end], touched[start:end])
				iupContour(tdy[start:end], origY[start:end], touched[start:end])
				start = end
			}
		}
		for p := range nPoints {
			dx[p] += s * tdx[p]
			dy[p] += s * tdy[p]
		}
	}

	res := &ApplyResult{
		Advance: clampUint16(math.Round(float64(aw) + dx[nBase+1] - dx[nBase])),
	}

	switch {
	case simple != nil:
		p := 0
		for ci := range simple.Contours {
			for pi := range simple.Contours[ci] {
				pt := &simple.Contours[ci][pi]
				pt.X = clampInt16(math.Round(origX[p] + dx[p]))
				pt.Y = clampInt16(math.Round(origY[p] + dy[p]))
				p++
			}
		}
		gl := simple.AsGlyph()
		res.Glyph = &gl

	case composite != nil:
		newComps := make([]glyf.GlyphComponent, len(composite.Components))
		for k, comp := range composite.Components {
			nc := comp
			ddx := int(math.Round(dx[k]))
			ddy := int(math.Round(dy[k]))
			if comp.Flags&glyf.FlagArgsAreXYValues != 0 && (ddx != 0 || ddy != 0) {
				spliceOffset(&nc, ddx, ddy)
			}
			newComps[k] = nc
		}
		res.Glyph = &glyf.Glyph{
			Rect16: g.Rect16,
			Data:   glyf.CompositeGlyph{Components: newComps, Instructions: composite.Instructions},
		}
	}

	return res, nil
}

// spliceOffset adds (ddx, ddy) to a component's X/Y offset arguments in place,
// preserving all trailing transform bytes and every flag except that it sets
// ARG_1_AND_2_ARE_WORDS when a byte-encoded value no longer fits in an int8.
// It must be called only for components with ARGS_ARE_XY_VALUES set.
func spliceOffset(comp *glyf.GlyphComponent, ddx, ddy int) {
	words := comp.Flags&glyf.FlagArg1And2AreWords != 0
	data := comp.Data

	var arg1, arg2, argLen int
	if words {
		argLen = 4
		if len(data) < 4 {
			return
		}
		arg1 = int(int16(uint16(data[0])<<8 | uint16(data[1])))
		arg2 = int(int16(uint16(data[2])<<8 | uint16(data[3])))
	} else {
		argLen = 2
		if len(data) < 2 {
			return
		}
		arg1 = int(int8(data[0]))
		arg2 = int(int8(data[1]))
	}
	arg1 += ddx
	arg2 += ddy

	if !words && (arg1 < math.MinInt8 || arg1 > math.MaxInt8 ||
		arg2 < math.MinInt8 || arg2 > math.MaxInt8) {
		words = true
		comp.Flags |= glyf.FlagArg1And2AreWords
	}

	trailing := data[argLen:]
	var buf []byte
	if words {
		a1 := int16(clampInt16(float64(arg1)))
		a2 := int16(clampInt16(float64(arg2)))
		buf = make([]byte, 0, 4+len(trailing))
		buf = append(buf, byte(uint16(a1)>>8), byte(a1), byte(uint16(a2)>>8), byte(a2))
	} else {
		buf = make([]byte, 0, 2+len(trailing))
		buf = append(buf, byte(int8(arg1)), byte(int8(arg2)))
	}
	buf = append(buf, trailing...)
	comp.Data = buf
}

func clampInt16(v float64) funit.Int16 {
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return funit.Int16(v)
}

func clampUint16(v float64) funit.Uint16 {
	if v > math.MaxUint16 {
		return math.MaxUint16
	}
	if v < 0 {
		return 0
	}
	return funit.Uint16(v)
}
