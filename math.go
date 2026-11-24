// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/mathtable"
)

// closeMath adds the size variants and assembly parts of the glyphs in the
// subset.  Since s.glyphs grows during the loop, variants of variants are
// included, too.
func (s *subsetter) closeMath(info *mathtable.Info) {
	if info == nil {
		return
	}
	for i := 0; i < len(s.glyphs); i++ {
		gid := s.glyphs[i]
		for _, c := range [...]*mathtable.Construction{info.Vertical[gid], info.Horizontal[gid]} {
			if c == nil {
				continue
			}
			for _, v := range c.Variants {
				s.addGlyph(v.Glyph)
			}
			if c.Assembly != nil {
				for _, p := range c.Assembly.Parts {
					s.addGlyph(p.Glyph)
				}
			}
		}
	}
}

// SubsetMath returns the "MATH" table restricted to the glyphs of the
// subset, with glyph IDs renumbered.
func (s *subsetter) SubsetMath(info *mathtable.Info) *mathtable.Info {
	if info == nil {
		return nil
	}
	return info.MapGlyphs(func(gid glyph.ID) (glyph.ID, bool) {
		newGid, ok := s.newGid[gid]
		return newGid, ok
	})
}
