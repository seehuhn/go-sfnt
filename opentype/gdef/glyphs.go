// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gdef

import (
	"errors"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
)

// LimitGlyphs returns a table without any reference to a glyph ID
// numGlyphs or larger.  If the table has no such reference, the table itself
// is returned.  Otherwise the result is a copy, in which the out-of-range
// glyphs are removed from the class definitions, the attachment point list,
// the ligature caret list and the mark glyph sets.  The lists are set to nil
// if no entry is left.  The number of mark glyph sets is not changed.
//
// The receiver is never modified.  A nil receiver returns nil.
func (table *Table) LimitGlyphs(numGlyphs int) *Table {
	res, _ := table.limitGlyphs(numGlyphs)
	return res
}

// CheckGlyphs returns an error if the table refers to a glyph ID numGlyphs
// or larger.
func (table *Table) CheckGlyphs(numGlyphs int) error {
	if table.AttachList != nil {
		if err := table.AttachList.checkShape(); err != nil {
			return err
		}
	}
	if table.LigCaretList != nil {
		if err := table.LigCaretList.checkShape(); err != nil {
			return err
		}
	}
	if _, changed := table.limitGlyphs(numGlyphs); changed {
		return errors.New("glyph ID out of range")
	}
	return nil
}

// limitGlyphs implements LimitGlyphs.  The second result reports whether
// the returned table differs from the receiver.
func (table *Table) limitGlyphs(numGlyphs int) (*Table, bool) {
	if table == nil {
		return nil, false
	}

	res := *table
	changed := false

	var c bool
	if res.GlyphClass, c = table.GlyphClass.Limit(numGlyphs); c {
		changed = true
	}
	if res.MarkAttachClass, c = table.MarkAttachClass.Limit(numGlyphs); c {
		changed = true
	}
	if res.AttachList, c = table.AttachList.limit(numGlyphs); c {
		changed = true
	}
	if res.LigCaretList, c = table.LigCaretList.limit(numGlyphs); c {
		changed = true
	}

	cloned := false
	for i, set := range table.MarkGlyphSets {
		limited, c := set.Limit(numGlyphs)
		if !c {
			continue
		}
		if !cloned {
			res.MarkGlyphSets = slices.Clone(table.MarkGlyphSets)
			cloned = true
		}
		changed = true
		res.MarkGlyphSets[i] = limited
	}

	if !changed {
		return table, false
	}
	return &res, true
}

// limitCoverage removes the glyphs numGlyphs and above from a coverage
// table and from the data indexed by its coverage indices.  The last result
// reports whether any glyph was removed.
func limitCoverage[T any](cov coverage.Table, data []T, numGlyphs int) (coverage.Table, []T, bool) {
	return coverage.Filter(cov, data, func(gid glyph.ID, _ T) bool {
		return int(gid) < numGlyphs
	})
}

func (al *AttachList) limit(numGlyphs int) (*AttachList, bool) {
	if al == nil {
		return nil, false
	}
	cov, points, changed := limitCoverage(al.Cov, al.Points, numGlyphs)
	if !changed {
		return al, false
	}
	if len(cov) == 0 {
		return nil, true
	}
	return &AttachList{Cov: cov, Points: points}, true
}

func (lcl *LigCaretList) limit(numGlyphs int) (*LigCaretList, bool) {
	if lcl == nil {
		return nil, false
	}
	cov, carets, changed := limitCoverage(lcl.Cov, lcl.Carets, numGlyphs)
	if !changed {
		return lcl, false
	}
	if len(cov) == 0 {
		return nil, true
	}
	return &LigCaretList{Cov: cov, Carets: carets}, true
}
