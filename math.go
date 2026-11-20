// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"fmt"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/mathtable"
)

// repairMath removes the parts of a "MATH" table read from a file which
// refer to glyphs beyond numGlyphs.  VariationIndex tables which refer to
// rows missing from the GDEF item variation store are dropped.
func repairMath(info *mathtable.Info, numGlyphs int, gdefTable *gdef.Table) *mathtable.Info {
	if info == nil {
		return nil
	}
	info = info.MapGlyphs(func(gid glyph.ID) (glyph.ID, bool) {
		return gid, int(gid) < numGlyphs
	})
	return info.MapValues(func(v mathtable.Value) mathtable.Value {
		if v.Device != nil && v.Device.IsVariationIndex() && !hasVariationRow(gdefTable, v.Device) {
			v.Device = nil
		}
		return v
	})
}

// checkMathVariations returns an error if a VariationIndex table in info
// refers to a row missing from the GDEF item variation store.
func checkMathVariations(info *mathtable.Info, gdefTable *gdef.Table) error {
	for v := range info.Values() {
		d := v.Device
		if d != nil && d.IsVariationIndex() && !hasVariationRow(gdefTable, d) {
			return fmt.Errorf("sfnt: MATH refers to missing variation data (%d, %d)",
				d.OuterIndex, d.InnerIndex)
		}
	}
	return nil
}

// hasVariationRow reports whether the GDEF item variation store contains
// the row selected by the VariationIndex table d.
func hasVariationRow(gdefTable *gdef.Table, d *device.Table) bool {
	if gdefTable == nil || gdefTable.ItemVarStore == nil {
		return false
	}
	data := gdefTable.ItemVarStore.Data
	outer, inner := int(d.OuterIndex), int(d.InnerIndex)
	return outer < len(data) && inner < len(data[outer].Deltas)
}

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
