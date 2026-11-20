// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package glyf

import (
	"errors"
	"math"

	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
)

var errBBoxRange = errors.New("glyf: composite glyph bounding box exceeds the int16 range")

// RecomputeCompositeBBoxes replaces the bounding box of every composite glyph
// by the union of its components' bounding boxes, each mapped through its
// component transform and rounded outwards.  Simple glyphs keep their boxes,
// so this is called once those are final, for example after the outlines
// have been varied.  The glyphs are modified in place.
//
// A component whose transform rotates or shears contributes the axis-aligned
// box of its child's transformed box, which can be larger than the
// transformed outline.  A component contributes nothing if it refers to a
// blank or missing glyph, or if its data is too short for its flags, and a
// composite glyph none of whose components contribute becomes blank.
//
// The boxes describe what [Outlines.Path] draws, and share its limitations:
// a component which positions itself by point matching is placed without an
// offset, and [FlagScaledComponentOffset] is ignored.
//
// Each composite glyph is computed once, after the composite glyphs it
// references, so the cost is linear in the number of components even where a
// glyph is referenced many times over.  A component which refers back to a
// glyph still being computed (a reference cycle, which valid fonts do not
// contain) contributes that glyph's stored box.
//
// If the box of a composite glyph does not fit into the int16 coordinates of the
// glyph header, RecomputeCompositeBBoxes returns an error.  The glyphs are
// then left partly updated.
func (gg Glyphs) RecomputeCompositeBBoxes() error {
	state := make([]bboxState, len(gg))
	for gid, g := range gg {
		if g == nil || state[gid] != bboxPending {
			continue
		}
		if _, ok := g.Data.(CompositeGlyph); ok {
			if err := gg.recomputeCompositeBBox(glyph.ID(gid), state); err != nil {
				return err
			}
		}
	}
	return nil
}

// bboxState tracks the progress of RecomputeCompositeBBoxes for one glyph.
type bboxState uint8

const (
	bboxPending bboxState = iota
	bboxComputing
	bboxDone
)

// recomputeCompositeBBox computes the box of the composite glyph gid, first
// computing any composite components which are still pending.
func (gg Glyphs) recomputeCompositeBBox(gid glyph.ID, state []bboxState) error {
	state[gid] = bboxComputing

	var box funit.Rect16
	for _, comp := range gg[gid].Data.(CompositeGlyph).Components {
		child := comp.GlyphIndex
		if int(child) >= len(gg) || gg[child] == nil {
			continue
		}
		if _, ok := gg[child].Data.(CompositeGlyph); ok && state[child] == bboxPending {
			if err := gg.recomputeCompositeBBox(child, state); err != nil {
				return err
			}
		}

		// a child still being computed lies on a reference cycle and
		// contributes its stored box
		childBox := gg[child].Rect16
		if childBox.IsZero() { // blank glyph
			continue
		}
		cu, err := comp.Unpack()
		if err != nil {
			continue
		}
		b, err := rect16(rectOf(childBox).Transform(cu.Trfm))
		if err != nil {
			return err
		}
		box.Extend(b)
	}

	gg[gid].Rect16 = box
	state[gid] = bboxDone
	return nil
}

// rect16 rounds r outwards and converts it to the integer type used in the
// glyph header, so that the stored box still contains r.
func rect16(r rect.Rect) (funit.Rect16, error) {
	r = r.Rounded()
	for _, x := range []float64{r.LLx, r.LLy, r.URx, r.URy} {
		if !(x >= math.MinInt16 && x <= math.MaxInt16) {
			return funit.Rect16{}, errBBoxRange
		}
	}
	return funit.Rect16{
		LLx: funit.Int16(r.LLx),
		LLy: funit.Int16(r.LLy),
		URx: funit.Int16(r.URx),
		URy: funit.Int16(r.URy),
	}, nil
}

// rectOf converts a glyph header box to a rect.Rect.
func rectOf(r funit.Rect16) rect.Rect {
	return rect.Rect{
		LLx: float64(r.LLx),
		LLy: float64(r.LLy),
		URx: float64(r.URx),
		URy: float64(r.URy),
	}
}
