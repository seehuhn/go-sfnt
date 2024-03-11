// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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

// A KeepFunc decised which glyphs to consider in a lookup.
// Glyphs where the KeepFunc returns false are ignored.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#lookupFlags
type KeepFunc struct {
	Gdef *gdef.Table
	Meta *LookupMetaInfo
}

func newKeepFunc(meta *LookupMetaInfo, gdef *gdef.Table) *KeepFunc {
	if gdef == nil || gdef.GlyphClass == nil || meta.LookupFlags == 0 {
		return nil
	}

	return &KeepFunc{
		Gdef: gdef,
		Meta: meta,
	}
}

// Keep returns true, if the glyph with the given ID should be considered in
// the lookup.
func (k *KeepFunc) Keep(gid glyph.ID) bool {
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
