// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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

package sfnt

import (
	"fmt"

	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
)

// Subset returns a subset of the font containing containing the given
// glyphs at the first positions.  More glyphs may be included in the
// subset, if they occur as ligatures between the given glyphs.
//
// The slice glyphs must start with glyph ID 0 to represent the notdef glyph.
func (f *Font) Subset(glyphs []glyph.ID) *Font {
	res := f.Clone()

	s := subsetter{
		glyphs: glyphs,
		newGid: map[glyph.ID]glyph.ID{},
	}
	for newgid, oldGid := range glyphs {
		s.newGid[oldGid] = glyph.ID(newgid)
	}

	if f.CMapTable != nil {
		res.CMapTable = make(cmap.Table, len(f.CMapTable))
		for key := range f.CMapTable {
			c, err := res.CMapTable.Get(key)
			if err != nil {
				continue
			}
			c = s.SubsetCMap(c)
			res.CMapTable[key] = c.Encode(key.Language)
		}
	}
	res.Gsub = s.SubsetGsub(f.Gsub)
	// At this point we have the final list of glyphs.
	res.Gpos = s.SubsetGpos(f.Gpos)
	res.Gdef = s.SubsetGdef(f.Gdef)

	switch outlines := f.Outlines.(type) {
	case *cff.Outlines:
		res.Outlines = s.SubsetCFF(outlines)
	case *glyf.Outlines:
		res.Outlines = s.SubsetGlyf(outlines)
	}

	return res
}

type subsetter struct {
	glyphs []glyph.ID
	newGid map[glyph.ID]glyph.ID
}

func (s *subsetter) hasOldGid(oldGid glyph.ID) bool {
	_, ok := s.newGid[oldGid]
	return ok
}

func (s *subsetter) getNewGid(oldGid glyph.ID) glyph.ID {
	newGid, ok := s.newGid[oldGid]
	if !ok {
		newGid = glyph.ID(len(s.glyphs))
		s.glyphs = append(s.glyphs, oldGid)
		s.newGid[oldGid] = newGid
	}
	return newGid
}

func (s *subsetter) SubsetCMap(c cmap.Subtable) cmap.Subtable {
	if c == nil {
		return nil
	}

	switch c := c.(type) {
	case cmap.Format4:
		res := cmap.Format4{}
		for key, oldGid := range c {
			newGid, ok := s.newGid[oldGid]
			if !ok {
				continue
			}
			res[key] = newGid
		}
		return res
	case cmap.Format12:
		res := cmap.Format12{}
		for key, oldGid := range c {
			newGid, ok := s.newGid[oldGid]
			if !ok {
				continue
			}
			res[key] = newGid
		}
		return res
	default:
		panic(fmt.Sprintf("sfnt: unsupported cmap format %T", c))
	}
}

// TODO(voss): This is incomplete.  Finish this!
func (s *subsetter) SubsetGsub(old *gtab.Info) *gtab.Info {
	if old == nil {
		return nil
	}

	// step 1: make a list of all GSUB rules
	type rule struct {
		nMissing int
		in       []glyph.ID
		out      []glyph.ID
	}
	// gsub8Rule captures the any-of-per-position context of a Gsub8_1
	// rule: it fires iff input is in the subset AND each backtrack and
	// lookahead position has at least one glyph from its coverage in
	// the subset.  Encoding this in the flat `rule` type would either
	// be too strict (treating every context glyph as required) or
	// require an exponential cartesian-product expansion, so the
	// fixpoint loop below handles them on a side channel.
	type gsub8Rule struct {
		input     glyph.ID
		target    glyph.ID
		backtrack []coverage.Table
		lookahead []coverage.Table
	}
	var rules []rule
	var gsub8Rules []gsub8Rule
	for _, tOld := range old.LookupList {
		for _, sOld := range tOld.Subtables {
			switch sOld := sOld.(type) {
			case *gtab.Gsub1_1:
				for gid := range sOld.Cov {
					rules = append(rules, rule{
						in:  []glyph.ID{gid},
						out: []glyph.ID{gid + sOld.Delta},
					})
				}
			case *gtab.Gsub1_2:
				for gid, idx := range sOld.Cov {
					rules = append(rules, rule{
						in:  []glyph.ID{gid},
						out: []glyph.ID{sOld.SubstituteGlyphIDs[idx]},
					})
				}
			case *gtab.Gsub2_1:
				for gid, idx := range sOld.Cov {
					rules = append(rules, rule{
						in:  []glyph.ID{gid},
						out: sOld.Repl[idx],
					})
				}
			case *gtab.Gsub3_1:
				for gid, idx := range sOld.Cov {
					rules = append(rules, rule{
						in:  []glyph.ID{gid},
						out: sOld.Alternates[idx],
					})
				}
			case *gtab.Gsub4_1:
				for gid, idx := range sOld.Cov {
					for _, lig := range sOld.Repl[idx] {
						in := make([]glyph.ID, 0, len(lig.In)+1)
						in = append(in, gid)
						in = append(in, lig.In...)
						rules = append(rules, rule{
							in:  in,
							out: []glyph.ID{lig.Out},
						})
					}
				}
			case *gtab.Gsub8_1:
				for gid, idx := range sOld.Input {
					gsub8Rules = append(gsub8Rules, gsub8Rule{
						input:     gid,
						target:    sOld.SubstituteGlyphIDs[idx],
						backtrack: sOld.Backtrack,
						lookahead: sOld.Lookahead,
					})
				}
			case *gtab.SeqContext1, *gtab.SeqContext2, *gtab.SeqContext3,
				*gtab.ChainedSeqContext1, *gtab.ChainedSeqContext2, *gtab.ChainedSeqContext3:
				// contextual subtables only constrain when nested lookups
				// fire; the nested lookups contribute their own rules to
				// this list, so no rule is added here.
			default:
				panic(fmt.Sprintf("sfnt: unsupported GSUB format %T", sOld))
			}
		}
	}

	// step 2: finalise the list of glyphs needed
	for i, r := range rules {
		nMissing := 0
		for _, in := range r.in {
			if _, ok := s.newGid[in]; !ok {
				nMissing++
			}
		}
		rules[i].nMissing = nMissing
	}
	// gsub8CanFire reports whether r's input is in the subset and every
	// backtrack/lookahead position has at least one glyph from its
	// coverage in the subset.
	gsub8CanFire := func(r *gsub8Rule) bool {
		if !s.hasOldGid(r.input) {
			return false
		}
		for _, list := range [...][]coverage.Table{r.backtrack, r.lookahead} {
			for _, cov := range list {
				covered := false
				for g := range cov {
					if s.hasOldGid(g) {
						covered = true
						break
					}
				}
				if !covered {
					return false
				}
			}
		}
		return true
	}

	added := make(map[glyph.ID]struct{})
	needsRun := true
	for needsRun {
		clear(added)

		pos := 0
		for pos < len(rules) {
			r := rules[pos]
			if r.nMissing == 0 {
				for _, gid := range r.out {
					if !s.hasOldGid(gid) {
						s.getNewGid(gid)
						added[gid] = struct{}{}
					}
				}
				rules = append(rules[:pos], rules[pos+1:]...)
			} else {
				pos++
			}
		}

		// gsub8 rules use any-of-per-position semantics; re-evaluate
		// each iteration so newly-added glyphs can satisfy their context.
		pos = 0
		for pos < len(gsub8Rules) {
			r := &gsub8Rules[pos]
			if gsub8CanFire(r) {
				if !s.hasOldGid(r.target) {
					s.getNewGid(r.target)
					added[r.target] = struct{}{}
				}
				gsub8Rules = append(gsub8Rules[:pos], gsub8Rules[pos+1:]...)
			} else {
				pos++
			}
		}

		needsRun = false
		for i, r := range rules {
			for _, in := range r.in {
				if _, ok := added[in]; ok {
					rules[i].nMissing--
				}
			}
			if rules[i].nMissing == 0 {
				needsRun = true
			}
		}
		// gsub8 rules don't use nMissing; if anything new was added
		// and any gsub8 rule is left, retry — a freshly-added glyph
		// may satisfy a previously-missing context position.
		if len(added) > 0 && len(gsub8Rules) > 0 {
			needsRun = true
		}
	}

	// step 3: create the new GSUB table
	res := *old
	res.LookupList = nil
	oldToNew := make([]int, 0, len(old.LookupList))
	for _, tOld := range old.LookupList {
		tNew := &gtab.LookupTable{
			Meta: tOld.Meta,
		}
		for _, sOld := range tOld.Subtables {
			switch sOld := sOld.(type) {
			case *gtab.Gsub1_1:
				// It is difficult to produce a format 1 subtable for the subset
				// (constant offset between original and subtitute), so we
				// convert this to format 2 instead.  Walking s.glyphs in
				// newGid order ensures the coverage indices come out in
				// ascending-gid order, required by the encoder.
				sNew := &gtab.Gsub1_2{
					Cov: make(coverage.Table),
				}
				for newFrom, oldOrig := range s.glyphs {
					if _, ok := sOld.Cov[oldOrig]; !ok {
						continue
					}
					newTo := oldOrig + sOld.Delta
					sNew.Cov[glyph.ID(newFrom)] = len(sNew.SubstituteGlyphIDs)
					sNew.SubstituteGlyphIDs = append(sNew.SubstituteGlyphIDs, s.getNewGid(newTo))
				}
				if len(sNew.Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gsub1_2:
				sNew := &gtab.Gsub1_2{
					Cov: make(coverage.Table),
				}
				for newGid, oldGid := range s.glyphs {
					oldIdx, ok := sOld.Cov[oldGid]
					if !ok {
						continue
					}
					sNew.Cov[glyph.ID(newGid)] = len(sNew.SubstituteGlyphIDs)
					sNew.SubstituteGlyphIDs = append(sNew.SubstituteGlyphIDs,
						s.getNewGid(sOld.SubstituteGlyphIDs[oldIdx]))
				}
				if len(sNew.Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gsub2_1:
				sNew := &gtab.Gsub2_1{
					Cov: make(coverage.Table),
				}
				for newGid, oldGid := range s.glyphs {
					oldIdx, ok := sOld.Cov[oldGid]
					if !ok {
						continue
					}
					repl := make([]glyph.ID, len(sOld.Repl[oldIdx]))
					for i, gid := range sOld.Repl[oldIdx] {
						repl[i] = s.getNewGid(gid)
					}
					sNew.Cov[glyph.ID(newGid)] = len(sNew.Repl)
					sNew.Repl = append(sNew.Repl, repl)
				}
				if len(sNew.Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gsub3_1:
				sNew := &gtab.Gsub3_1{
					Cov: make(coverage.Table),
				}
				for newGid, oldGid := range s.glyphs {
					oldIdx, ok := sOld.Cov[oldGid]
					if !ok {
						continue
					}
					alts := make([]glyph.ID, len(sOld.Alternates[oldIdx]))
					for i, gid := range sOld.Alternates[oldIdx] {
						alts[i] = s.getNewGid(gid)
					}
					sNew.Cov[glyph.ID(newGid)] = len(sNew.Alternates)
					sNew.Alternates = append(sNew.Alternates, alts)
				}
				if len(sNew.Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gsub4_1:
				sNew := &gtab.Gsub4_1{
					Cov: make(coverage.Table),
				}
				for newFirst, oldFirst := range s.glyphs {
					idx, ok := sOld.Cov[oldFirst]
					if !ok {
						continue
					}
					var ligs []gtab.Ligature
				ligLoop:
					for _, lig := range sOld.Repl[idx] {
						for _, oldGID := range lig.In {
							if _, ok := s.newGid[oldGID]; !ok {
								continue ligLoop
							}
						}
						newLig := gtab.Ligature{
							In:  make([]glyph.ID, len(lig.In)),
							Out: s.getNewGid(lig.Out),
						}
						for i, oldGID := range lig.In {
							newLig.In[i] = s.getNewGid(oldGID)
						}
						ligs = append(ligs, newLig)
					}
					if len(ligs) > 0 {
						sNew.Cov[glyph.ID(newFirst)] = len(sNew.Repl)
						sNew.Repl = append(sNew.Repl, ligs)
					}
				}
				if len(sNew.Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gsub8_1:
				if sNew := s.subsetGsub8_1(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.SeqContext1:
				if sNew := s.subsetSeqContext1(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.SeqContext2:
				if sNew := s.subsetSeqContext2(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.SeqContext3:
				if sNew := s.subsetSeqContext3(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.ChainedSeqContext1:
				if sNew := s.subsetChainedSeqContext1(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.ChainedSeqContext2:
				if sNew := s.subsetChainedSeqContext2(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.ChainedSeqContext3:
				if sNew := s.subsetChainedSeqContext3(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			default:
				panic(fmt.Sprintf("sfnt: unsupported GSUB format %T", sOld))
			}
		}

		if len(tNew.Subtables) > 0 {
			oldToNew = append(oldToNew, len(res.LookupList))
			res.LookupList = append(res.LookupList, tNew)
		} else {
			oldToNew = append(oldToNew, -1)
		}
	}

	remapContextualLookupIndices(res.LookupList, oldToNew)

	return &res
}

func (s *subsetter) SubsetGpos(old *gtab.Info) *gtab.Info {
	if old == nil {
		return nil
	}

	res := *old
	res.LookupList = nil
	oldToNew := make([]int, 0, len(old.LookupList))
	for _, tOld := range old.LookupList {
		tNew := &gtab.LookupTable{
			Meta: tOld.Meta,
		}
		for _, sOld := range tOld.Subtables {
			switch sOld := sOld.(type) {
			case *gtab.Gpos1_1:
				// Walk s.glyphs in newGid order so the coverage indices come
				// out in ascending-gid order, required by the encoder.
				sNew := &gtab.Gpos1_1{
					Cov:    coverage.Table{},
					Adjust: sOld.Adjust,
				}
				for newGid, oldGid := range s.glyphs {
					if _, ok := sOld.Cov[oldGid]; ok {
						sNew.Cov[glyph.ID(newGid)] = len(sNew.Cov)
					}
				}
				if len(sNew.Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gpos1_2:
				sNew := &gtab.Gpos1_2{
					Cov: coverage.Table{},
				}
				for newGid, oldGid := range s.glyphs {
					oldIdx, ok := sOld.Cov[oldGid]
					if !ok {
						continue
					}
					sNew.Cov[glyph.ID(newGid)] = len(sNew.Adjust)
					sNew.Adjust = append(sNew.Adjust, sOld.Adjust[oldIdx])
				}
				if len(sNew.Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case gtab.Gpos2_1:
				sNew := gtab.Gpos2_1{}
				for pair, adj := range sOld {
					left, ok := s.newGid[pair.Left]
					if !ok {
						continue
					}
					right, ok := s.newGid[pair.Right]
					if !ok {
						continue
					}
					sNew[glyph.Pair{Left: left, Right: right}] = adj
				}
				if len(sNew) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gpos2_2:
				newCov := coverage.Set{}
				for gid := range sOld.Cov {
					if newGid, ok := s.newGid[gid]; ok {
						newCov[newGid] = true
					}
				}
				if len(newCov) == 0 {
					continue
				}
				newClass1 := classdef.Table{}
				for gid, cls := range sOld.Class1 {
					if newGid, ok := s.newGid[gid]; ok {
						newClass1[newGid] = cls
					}
				}
				newClass2 := classdef.Table{}
				for gid, cls := range sOld.Class2 {
					if newGid, ok := s.newGid[gid]; ok {
						newClass2[newGid] = cls
					}
				}
				tNew.Subtables = append(tNew.Subtables, &gtab.Gpos2_2{
					Cov:    newCov,
					Class1: newClass1,
					Class2: newClass2,
					Adjust: sOld.Adjust,
				})
			case *gtab.Gpos3_1:
				sNew := &gtab.Gpos3_1{
					Cov: coverage.Table{},
				}
				for newGid, oldGid := range s.glyphs {
					oldIdx, ok := sOld.Cov[oldGid]
					if !ok {
						continue
					}
					sNew.Cov[glyph.ID(newGid)] = len(sNew.Records)
					sNew.Records = append(sNew.Records, sOld.Records[oldIdx])
				}
				if len(sNew.Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gpos4_1:
				sNew := &gtab.Gpos4_1{
					MarkCov: coverage.Table{},
					BaseCov: coverage.Table{},
				}
				for newGid, oldGid := range s.glyphs {
					if oldIdx, ok := sOld.MarkCov[oldGid]; ok {
						sNew.MarkCov[glyph.ID(newGid)] = len(sNew.MarkArray)
						sNew.MarkArray = append(sNew.MarkArray, sOld.MarkArray[oldIdx])
					}
					if oldIdx, ok := sOld.BaseCov[oldGid]; ok {
						sNew.BaseCov[glyph.ID(newGid)] = len(sNew.BaseArray)
						sNew.BaseArray = append(sNew.BaseArray, sOld.BaseArray[oldIdx])
					}
				}
				// Gpos4_1 only fires when both a mark and a base glyph match;
				// drop the subtable if either side is empty.
				if len(sNew.MarkCov) > 0 && len(sNew.BaseCov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gpos5_1:
				// The per-component × per-mark-class shape of each
				// ligature row is preserved, so markClassCount stays
				// consistent with the encoder's invariants.
				sNew := &gtab.Gpos5_1{
					MarkCov: coverage.Table{},
					LigCov:  coverage.Table{},
				}
				for newGid, oldGid := range s.glyphs {
					if oldIdx, ok := sOld.MarkCov[oldGid]; ok {
						sNew.MarkCov[glyph.ID(newGid)] = len(sNew.MarkArray)
						sNew.MarkArray = append(sNew.MarkArray, sOld.MarkArray[oldIdx])
					}
					if oldIdx, ok := sOld.LigCov[oldGid]; ok {
						sNew.LigCov[glyph.ID(newGid)] = len(sNew.LigArray)
						sNew.LigArray = append(sNew.LigArray, sOld.LigArray[oldIdx])
					}
				}
				if len(sNew.MarkCov) > 0 && len(sNew.LigCov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.Gpos6_1:
				sNew := &gtab.Gpos6_1{
					Mark1Cov: coverage.Table{},
					Mark2Cov: coverage.Table{},
				}
				for newGid, oldGid := range s.glyphs {
					if oldIdx, ok := sOld.Mark1Cov[oldGid]; ok {
						sNew.Mark1Cov[glyph.ID(newGid)] = len(sNew.Mark1Array)
						sNew.Mark1Array = append(sNew.Mark1Array, sOld.Mark1Array[oldIdx])
					}
					if oldIdx, ok := sOld.Mark2Cov[oldGid]; ok {
						sNew.Mark2Cov[glyph.ID(newGid)] = len(sNew.Mark2Array)
						sNew.Mark2Array = append(sNew.Mark2Array, sOld.Mark2Array[oldIdx])
					}
				}
				if len(sNew.Mark1Cov) > 0 && len(sNew.Mark2Cov) > 0 {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.SeqContext1:
				if sNew := s.subsetSeqContext1(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.SeqContext2:
				if sNew := s.subsetSeqContext2(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.SeqContext3:
				if sNew := s.subsetSeqContext3(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.ChainedSeqContext1:
				if sNew := s.subsetChainedSeqContext1(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.ChainedSeqContext2:
				if sNew := s.subsetChainedSeqContext2(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			case *gtab.ChainedSeqContext3:
				if sNew := s.subsetChainedSeqContext3(sOld); sNew != nil {
					tNew.Subtables = append(tNew.Subtables, sNew)
				}
			default:
				panic(fmt.Sprintf("sfnt: unsupported GPOS format %T", sOld))
			}
		}

		if len(tNew.Subtables) > 0 {
			oldToNew = append(oldToNew, len(res.LookupList))
			res.LookupList = append(res.LookupList, tNew)
		} else {
			oldToNew = append(oldToNew, -1)
		}
	}

	remapContextualLookupIndices(res.LookupList, oldToNew)

	return &res
}

func (s *subsetter) SubsetGdef(old *gdef.Table) *gdef.Table {
	if old == nil {
		return nil
	}
	res := &gdef.Table{}
	if old.GlyphClass != nil {
		res.GlyphClass = classdef.Table{}
		for gid, cls := range old.GlyphClass {
			if newGid, ok := s.newGid[gid]; ok {
				res.GlyphClass[newGid] = cls
			}
		}
	}
	if old.MarkAttachClass != nil {
		res.MarkAttachClass = classdef.Table{}
		for gid, cls := range old.MarkAttachClass {
			if newGid, ok := s.newGid[gid]; ok {
				res.MarkAttachClass[newGid] = cls
			}
		}
	}
	if old.MarkGlyphSets != nil {
		res.MarkGlyphSets = make([]coverage.Set, len(old.MarkGlyphSets))
		for i, set := range old.MarkGlyphSets {
			newSet := coverage.Set{}
			for gid := range set {
				if newGid, ok := s.newGid[gid]; ok {
					newSet[newGid] = true
				}
			}
			res.MarkGlyphSets[i] = newSet
		}
	}
	return res
}

func (s *subsetter) SubsetCFF(oldOutlines *cff.Outlines) *cff.Outlines {
	newOutlines := &cff.Outlines{}

	newOutlines.Glyphs = make([]*cff.Glyph, len(s.glyphs))
	for i, oldGid := range s.glyphs {
		newOutlines.Glyphs[i] = oldOutlines.Glyphs[oldGid]
	}

	pIdxMap := make(map[int]int)
	for _, oldGid := range s.glyphs {
		oldPIdx := oldOutlines.FDSelect(oldGid)
		if _, ok := pIdxMap[oldPIdx]; !ok {
			newPIdx := len(newOutlines.Private)
			newOutlines.Private = append(newOutlines.Private, oldOutlines.Private[oldPIdx])
			if oldOutlines.IsCIDKeyed() {
				newOutlines.FontMatrices = append(newOutlines.FontMatrices, oldOutlines.FontMatrices[oldPIdx])
			}
			pIdxMap[oldPIdx] = newPIdx
		}
	}
	if len(newOutlines.Private) == 1 {
		newOutlines.FDSelect = func(glyph.ID) int { return 0 }
	} else {
		fdSel := make([]int, len(s.glyphs))
		for newgid, oldGid := range s.glyphs {
			fdSel[newgid] = pIdxMap[oldOutlines.FDSelect(oldGid)]
		}
		newOutlines.FDSelect = func(gid glyph.ID) int { return fdSel[gid] }
	}

	if oldOutlines.Encoding != nil {
		newOutlines.Encoding = make([]glyph.ID, len(oldOutlines.Encoding))
		for i, oldGid := range oldOutlines.Encoding {
			if newGid, ok := s.newGid[oldGid]; ok {
				newOutlines.Encoding[i] = newGid
			}
		}
	}

	newOutlines.ROS = oldOutlines.ROS

	if oldOutlines.GIDToCID != nil {
		newOutlines.GIDToCID = make([]cid.CID, len(s.glyphs))
		for newGid, oldGid := range s.glyphs {
			newOutlines.GIDToCID[newGid] = oldOutlines.GIDToCID[oldGid]
		}
	}

	return newOutlines
}

func (s *subsetter) SubsetGlyf(oldOutlines *glyf.Outlines) *glyf.Outlines {
	newOutlines := &glyf.Outlines{
		Tables: oldOutlines.Tables,
		Maxp:   oldOutlines.Maxp,
	}

	todo := make(map[glyph.ID]bool, len(s.glyphs))
	for _, oldGid := range s.glyphs {
		todo[oldGid] = true
	}
	for len(todo) > 0 {
		oldGid := pop(todo)
		cc := oldOutlines.Glyphs[oldGid].Components()
		for _, componentGidOld := range cc {
			if _, ok := s.newGid[componentGidOld]; ok {
				continue
			}
			componendGidNew := glyph.ID(len(s.glyphs))
			s.glyphs = append(s.glyphs, componentGidOld)
			s.newGid[componentGidOld] = componendGidNew
			todo[componentGidOld] = true
		}
	}

	newOutlines.Glyphs = make([]*glyf.Glyph, len(s.glyphs))
	for newGid, oldGid := range s.glyphs {
		newOutlines.Glyphs[newGid] = oldOutlines.Glyphs[oldGid].FixComponents(s.newGid)
	}

	newOutlines.Widths = make([]funit.Int16, len(s.glyphs))
	for newGid, oldGid := range s.glyphs {
		newOutlines.Widths[newGid] = oldOutlines.Widths[oldGid]
	}

	if oldOutlines.Names != nil {
		newOutlines.Names = make([]string, len(s.glyphs))
		for newGid, oldGid := range s.glyphs {
			newOutlines.Names[newGid] = oldOutlines.Names[oldGid]
		}
	}

	// TODO(voss): can anything be done to make the "fpgm" table smaller?

	return newOutlines
}

func pop(todo map[glyph.ID]bool) glyph.ID {
	for key := range todo {
		delete(todo, key)
		return key
	}
	panic("empty map")
}
