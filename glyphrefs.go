// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"fmt"
)

// checkGlyphRefs returns an error if the layout tables of f refer to glyphs
// which the font does not have, or if a GSUB or GPOS lookup selects a mark
// filtering set which the GDEF table does not define.  The layout tables
// must be encodable.
func (f *Font) checkGlyphRefs() error {
	n := f.NumGlyphs()
	numSets := 0
	if f.Gdef != nil {
		if err := f.Gdef.CheckGlyphs(n); err != nil {
			return fmt.Errorf("sfnt: GDEF: %w", err)
		}
		numSets = len(f.Gdef.MarkGlyphSets)
	}
	if f.Math != nil {
		if err := f.Math.CheckGlyphs(n); err != nil {
			return fmt.Errorf("sfnt: MATH: %w", err)
		}
	}
	if f.Gsub != nil {
		if err := f.Gsub.CheckGlyphs(n); err != nil {
			return fmt.Errorf("sfnt: GSUB: %w", err)
		}
		if err := f.Gsub.CheckMarkFilteringSets(numSets); err != nil {
			return fmt.Errorf("sfnt: GSUB: %w", err)
		}
	}
	if f.Gpos != nil {
		if err := f.Gpos.CheckGlyphs(n); err != nil {
			return fmt.Errorf("sfnt: GPOS: %w", err)
		}
		if err := f.Gpos.CheckMarkFilteringSets(numSets); err != nil {
			return fmt.Errorf("sfnt: GPOS: %w", err)
		}
	}
	return nil
}
