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

package glyf

import (
	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/clamp"
)

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
func (gg Glyphs) RecomputeCompositeBBoxes() {
	state := make([]bboxState, len(gg))
	for gid, g := range gg {
		if g == nil || state[gid] != bboxPending {
			continue
		}
		if _, ok := g.Data.(CompositeGlyph); ok {
			gg.recomputeCompositeBBox(glyph.ID(gid), state)
		}
	}
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
func (gg Glyphs) recomputeCompositeBBox(gid glyph.ID, state []bboxState) {
	state[gid] = bboxComputing

	var box funit.Rect16
	for _, comp := range gg[gid].Data.(CompositeGlyph).Components {
		child := comp.GlyphIndex
		if int(child) >= len(gg) || gg[child] == nil {
			continue
		}
		if _, ok := gg[child].Data.(CompositeGlyph); ok && state[child] == bboxPending {
			gg.recomputeCompositeBBox(child, state)
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
		box.Extend(clamp.Rect16(rectOf(childBox).Transform(cu.Trfm)))
	}

	gg[gid].Rect16 = box
	state[gid] = bboxDone
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
