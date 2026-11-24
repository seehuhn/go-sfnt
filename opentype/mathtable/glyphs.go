// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"errors"

	"seehuhn.de/go/sfnt/glyph"
)

// LimitGlyphs returns a table without any reference to a glyph ID
// numGlyphs or larger.  If the table has no such reference, the table itself
// is returned.  Otherwise the result is a copy, in which the out-of-range
// glyphs are removed as described for [Info.MapGlyphs].
//
// The receiver is never modified.  A nil receiver returns nil.
func (info *Info) LimitGlyphs(numGlyphs int) *Info {
	if info == nil || info.CheckGlyphs(numGlyphs) == nil {
		return info
	}
	return info.MapGlyphs(func(gid glyph.ID) (glyph.ID, bool) {
		return gid, int(gid) < numGlyphs
	})
}

// CheckGlyphs returns an error if the table refers to a glyph ID numGlyphs
// or larger.
func (info *Info) CheckGlyphs(numGlyphs int) error {
	ok := keysBelow(info.ItalicsCorrection, numGlyphs) &&
		keysBelow(info.TopAccent, numGlyphs) &&
		keysBelow(info.Kerns, numGlyphs) &&
		constructionsBelow(info.Vertical, numGlyphs) &&
		constructionsBelow(info.Horizontal, numGlyphs)
	for gid, covered := range info.ExtendedShapes {
		if covered && int(gid) >= numGlyphs {
			ok = false
		}
	}
	if !ok {
		return errors.New("glyph ID out of range")
	}
	return nil
}

// keysBelow reports whether all keys of m are smaller than numGlyphs.
func keysBelow[V any](m map[glyph.ID]V, numGlyphs int) bool {
	for gid := range m {
		if int(gid) >= numGlyphs {
			return false
		}
	}
	return true
}

// constructionsBelow reports whether the keys of m, the variants and the
// assembly parts all use glyph IDs smaller than numGlyphs.
func constructionsBelow(m map[glyph.ID]*Construction, numGlyphs int) bool {
	if !keysBelow(m, numGlyphs) {
		return false
	}
	for _, c := range m {
		if c == nil {
			continue
		}
		for _, v := range c.Variants {
			if int(v.Glyph) >= numGlyphs {
				return false
			}
		}
		if c.Assembly == nil {
			continue
		}
		for _, part := range c.Assembly.Parts {
			if int(part.Glyph) >= numGlyphs {
				return false
			}
		}
	}
	return true
}
