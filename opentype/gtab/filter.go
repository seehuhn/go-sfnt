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

// keepGlyphFn is used to drop ignored characters in lookups with non-zero
// lookup flags.  Functions of this type return true if the glyph should be
// used, and false if the glyph should be ignored.
type keepGlyphFn func(glyph.ID) bool

// makeFilter returns a function which filters glyphs according to the
// lookup flags.
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#lookupFlags
func makeFilter(meta *LookupMetaInfo, gdefTable *gdef.Table) keepGlyphFn {
	keep := newKeepFunc(meta, gdefTable)
	return keep.Keep
}

type keepFunc struct {
	gdef  *gdef.Table
	flags LookupFlags
	set   uint16
}

func newKeepFunc(meta *LookupMetaInfo, gdef *gdef.Table) *keepFunc {
	if gdef == nil || gdef.GlyphClass == nil {
		return nil
	}

	flags := meta.LookupFlags
	if flags&IgnoreMarks != 0 {
		// If the IGNORE_MARKS bit is set, this supersedes any mark filtering
		// set or mark attachment type indications.
		flags &^= UseMarkFilteringSet | MarkAttachTypeMask
	} else if flags&UseMarkFilteringSet != 0 {
		// If a mark filtering set is specified, this supersedes any mark
		// attachment type indication in the lookup flag.
		flags &^= MarkAttachTypeMask

		if n := len(gdef.MarkGlyphSets); int(meta.MarkFilteringSet) >= n {
			flags &^= UseMarkFilteringSet
		}
	} else if flags&MarkAttachTypeMask != 0 {
		if gdef.MarkAttachClass == nil {
			flags &^= MarkAttachTypeMask
		}
	}
	if flags == 0 {
		return nil
	}

	return &keepFunc{
		gdef:  gdef,
		flags: flags,
		set:   meta.MarkFilteringSet,
	}
}

func (k *keepFunc) Keep(gid glyph.ID) bool {
	if k == nil {
		return true
	}

	switch k.gdef.GlyphClass[gid] {
	case gdef.GlyphClassBase:
		if k.flags&IgnoreBaseGlyphs != 0 {
			return false
		}
	case gdef.GlyphClassLigature:
		if k.flags&IgnoreLigatures != 0 {
			return false
		}
	case gdef.GlyphClassMark:
		if k.flags&IgnoreMarks != 0 {
			return false
		} else if k.flags&UseMarkFilteringSet != 0 {
			if !k.gdef.MarkGlyphSets[k.set][gid] {
				return false
			}
		} else if m := k.flags & MarkAttachTypeMask; m != 0 {
			if k.gdef.MarkAttachClass[gid] != uint16(m>>8) {
				return false
			}
		}
	}
	return true
}
