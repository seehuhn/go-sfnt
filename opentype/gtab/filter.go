// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/gdef"
)

// The following table shows the frequency of different lookupflag bits
// in the lookup tables in the fonts on my laptop.
//
// count   |  base ligature marks filtset attchtype
// --------+----------------------------------------
// 104671  |    .      .      .      .      .
//   7317  |    .      .      X      .      .
//   4602  |    .      .      .      .      X
//    876  |    .      .      .      X      .
//    194  |    X      X      .      .      .
//     80  |    X      X      .      .      X
//     58  |    .      X      .      .      .
//     36  |    X      .      .      .      .
//     32  |    .      X      X      .      .
//     20  |    X      .      .      .      X
//      6  |    X      .      .      X      .
//      6  |    .      .      X      .      X
//      4  |    X      X      .      X      .

// A keepFunc decised which glyphs to consider in a lookup.
// Glyphs where the keepFunc returns false are ignored.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#lookupFlags
type keepFunc struct {
	Gdef *gdef.Table
	Meta *LookupMetaInfo
}

func newKeepFunc(meta *LookupMetaInfo, gdef *gdef.Table) *keepFunc {
	if gdef == nil || gdef.GlyphClass == nil || meta.LookupFlags == 0 {
		return nil
	}

	return &keepFunc{
		Gdef: gdef,
		Meta: meta,
	}
}

// Keep returns true, if the glyph with the given ID should be considered in
// the lookup.
func (k *keepFunc) Keep(gid glyph.ID) bool {
	if k == nil {
		return true
	}

	flags := k.Meta.LookupFlags
	switch k.Gdef.GlyphClass[gid] {
	case gdef.GlyphClassBase:
		if flags&IgnoreBaseGlyphs != 0 {
			return false
		}
	case gdef.GlyphClassLigature:
		if flags&IgnoreLigatures != 0 {
			return false
		}
	case gdef.GlyphClassMark:
		if flags&IgnoreMarks != 0 {
			// If the IGNORE_MARKS bit is set, this supersedes any mark filtering
			// set or mark attachment type indications.
			return false
		} else if flags&UseMarkFilteringSet != 0 {
			// If a mark filtering set is specified, this supersedes any mark
			// attachment type indication in the lookup flag.
			set := k.Meta.MarkFilteringSet
			if k.Gdef.MarkGlyphSets == nil || !k.Gdef.MarkGlyphSets[set][gid] {
				return false
			}
		} else if m := flags & MarkAttachTypeMask; m != 0 {
			if k.Gdef.MarkAttachClass == nil || k.Gdef.MarkAttachClass[gid] != uint16(m>>8) {
				return false
			}
		}
	}
	return true
}
