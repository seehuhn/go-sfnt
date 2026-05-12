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

package sfnt

import (
	"fmt"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
)

// aliveClasses returns the set of class indices in cd that have at least
// one surviving glyph after subsetting.  Class 0 (the implicit class for
// glyphs not listed in cd) is included if any surviving glyph is unclassed.
func (s *subsetter) aliveClasses(cd classdef.Table) map[uint16]bool {
	alive := map[uint16]bool{}
	for _, oldGid := range s.glyphs {
		if cls, ok := cd[oldGid]; ok {
			alive[cls] = true
		} else {
			alive[0] = true
		}
	}
	return alive
}

// classesAllAlive reports whether every class index in classes is alive.
func classesAllAlive(classes []uint16, alive map[uint16]bool) bool {
	for _, c := range classes {
		if !alive[c] {
			return false
		}
	}
	return true
}

// subsetSeqContext1 returns a glyph-filtered copy of sOld, or nil if the
// resulting subtable would have no rules.  Rule structs are deep-copied so
// the caller may later mutate their Actions slices.
func (s *subsetter) subsetSeqContext1(sOld *gtab.SeqContext1) *gtab.SeqContext1 {
	// Walk s.glyphs in newGid order so the new coverage table has
	// monotonically increasing gids per coverage index, as required by
	// the encoder.
	sNew := &gtab.SeqContext1{Cov: coverage.Table{}}
	for newGid, oldGid := range s.glyphs {
		oldIdx, ok := sOld.Cov[oldGid]
		if !ok {
			continue
		}
		var newRules []*gtab.SeqRule
	ruleLoop:
		for _, r := range sOld.Rules[oldIdx] {
			newInput := make([]glyph.ID, len(r.Input))
			for i, g := range r.Input {
				ng, ok := s.newGid[g]
				if !ok {
					continue ruleLoop
				}
				newInput[i] = ng
			}
			newRules = append(newRules, &gtab.SeqRule{
				Input:   newInput,
				Actions: r.Actions,
			})
		}
		if len(newRules) > 0 {
			sNew.Cov[glyph.ID(newGid)] = len(sNew.Rules)
			sNew.Rules = append(sNew.Rules, newRules)
		}
	}
	if len(sNew.Cov) == 0 {
		return nil
	}
	return sNew
}

// subsetSeqContext2 returns a glyph-filtered copy of sOld, or nil if the
// resulting subtable would have no covered glyphs.  Class-based rules are
// shallow-cloned so the caller may later mutate their Actions slices.
func (s *subsetter) subsetSeqContext2(sOld *gtab.SeqContext2) *gtab.SeqContext2 {
	cov := coverage.Table{}
	for newGid, oldGid := range s.glyphs {
		if _, ok := sOld.Cov[oldGid]; ok {
			cov[glyph.ID(newGid)] = len(cov)
		}
	}
	if len(cov) == 0 {
		return nil
	}
	input := classdef.Table{}
	for oldGid, cls := range sOld.Input {
		if newGid, ok := s.newGid[oldGid]; ok {
			input[newGid] = cls
		}
	}
	alive := s.aliveClasses(sOld.Input)
	newRules := make([][]*gtab.ClassSeqRule, len(sOld.Rules))
	anyKept := false
	for firstClass, ruleList := range sOld.Rules {
		if !alive[uint16(firstClass)] {
			continue
		}
		for _, r := range ruleList {
			if !classesAllAlive(r.Input, alive) {
				continue
			}
			cp := *r
			newRules[firstClass] = append(newRules[firstClass], &cp)
			anyKept = true
		}
	}
	if !anyKept {
		return nil
	}
	// Rules is indexed by input class; len(Rules) must match
	// NumClasses(Input) so the encode → read round trip is bijective.
	// (The reader truncates trailing entries to NumClasses; without
	// this trim, those trailing nils would be lost on re-read.)
	newRules = trimTrailingNilRules(newRules, input.NumClasses())
	return &gtab.SeqContext2{
		Cov:   cov,
		Input: input,
		Rules: newRules,
	}
}

// trimTrailingNilRules shrinks rules to length n, panicking if any
// non-nil entry would be discarded.  Subset paths use this to keep
// Rules' length in sync with their input class-def's NumClasses, since
// the gtab reader truncates surplus entries.
func trimTrailingNilRules[T any](rules [][]T, n int) [][]T {
	if len(rules) <= n {
		return rules
	}
	for i := n; i < len(rules); i++ {
		if rules[i] != nil {
			panic(fmt.Sprintf("trimTrailingNilRules: non-nil rule at index %d beyond NumClasses=%d", i, n))
		}
	}
	return rules[:n]
}

// subsetSeqContext3 returns a glyph-filtered copy of sOld.  If any input
// coverage set becomes empty the rule can never fire and nil is returned.
func (s *subsetter) subsetSeqContext3(sOld *gtab.SeqContext3) *gtab.SeqContext3 {
	newInput := make([]coverage.Set, len(sOld.Input))
	for i, set := range sOld.Input {
		ns := coverage.Set{}
		for g := range set {
			if ng, ok := s.newGid[g]; ok {
				ns[ng] = true
			}
		}
		if len(ns) == 0 {
			return nil
		}
		newInput[i] = ns
	}
	return &gtab.SeqContext3{
		Input:   newInput,
		Actions: append([]gtab.SeqLookup(nil), sOld.Actions...),
	}
}

func (s *subsetter) subsetChainedSeqContext1(sOld *gtab.ChainedSeqContext1) *gtab.ChainedSeqContext1 {
	sNew := &gtab.ChainedSeqContext1{Cov: coverage.Table{}}
	for newGid, oldGid := range s.glyphs {
		oldIdx, ok := sOld.Cov[oldGid]
		if !ok {
			continue
		}
		var newRules []*gtab.ChainedSeqRule
	rLoop:
		for _, r := range sOld.Rules[oldIdx] {
			newB := make([]glyph.ID, len(r.Backtrack))
			for i, g := range r.Backtrack {
				ng, ok := s.newGid[g]
				if !ok {
					continue rLoop
				}
				newB[i] = ng
			}
			newI := make([]glyph.ID, len(r.Input))
			for i, g := range r.Input {
				ng, ok := s.newGid[g]
				if !ok {
					continue rLoop
				}
				newI[i] = ng
			}
			newL := make([]glyph.ID, len(r.Lookahead))
			for i, g := range r.Lookahead {
				ng, ok := s.newGid[g]
				if !ok {
					continue rLoop
				}
				newL[i] = ng
			}
			newRules = append(newRules, &gtab.ChainedSeqRule{
				Backtrack: newB,
				Input:     newI,
				Lookahead: newL,
				Actions:   r.Actions,
			})
		}
		if len(newRules) > 0 {
			sNew.Cov[glyph.ID(newGid)] = len(sNew.Rules)
			sNew.Rules = append(sNew.Rules, newRules)
		}
	}
	if len(sNew.Cov) == 0 {
		return nil
	}
	return sNew
}

func (s *subsetter) subsetChainedSeqContext2(sOld *gtab.ChainedSeqContext2) *gtab.ChainedSeqContext2 {
	cov := coverage.Table{}
	for newGid, oldGid := range s.glyphs {
		if _, ok := sOld.Cov[oldGid]; ok {
			cov[glyph.ID(newGid)] = len(cov)
		}
	}
	if len(cov) == 0 {
		return nil
	}
	remap := func(cd classdef.Table) classdef.Table {
		out := classdef.Table{}
		for g, cls := range cd {
			if ng, ok := s.newGid[g]; ok {
				out[ng] = cls
			}
		}
		return out
	}
	backAlive := s.aliveClasses(sOld.Backtrack)
	inputAlive := s.aliveClasses(sOld.Input)
	lookAlive := s.aliveClasses(sOld.Lookahead)
	newRules := make([][]*gtab.ChainedClassSeqRule, len(sOld.Rules))
	anyKept := false
	for firstClass, ruleList := range sOld.Rules {
		if !inputAlive[uint16(firstClass)] {
			continue
		}
		for _, r := range ruleList {
			if !classesAllAlive(r.Backtrack, backAlive) ||
				!classesAllAlive(r.Input, inputAlive) ||
				!classesAllAlive(r.Lookahead, lookAlive) {
				continue
			}
			cp := *r
			newRules[firstClass] = append(newRules[firstClass], &cp)
			anyKept = true
		}
	}
	if !anyKept {
		return nil
	}
	newInput := remap(sOld.Input)
	// see trimTrailingNilRules — the reader truncates Rules to
	// NumClasses(Input), so we must shrink Rules to match.
	newRules = trimTrailingNilRules(newRules, newInput.NumClasses())
	return &gtab.ChainedSeqContext2{
		Cov:       cov,
		Backtrack: remap(sOld.Backtrack),
		Input:     newInput,
		Lookahead: remap(sOld.Lookahead),
		Rules:     newRules,
	}
}

func (s *subsetter) subsetChainedSeqContext3(sOld *gtab.ChainedSeqContext3) *gtab.ChainedSeqContext3 {
	filterSets := func(sets []coverage.Set) ([]coverage.Set, bool) {
		out := make([]coverage.Set, len(sets))
		for i, set := range sets {
			ns := coverage.Set{}
			for g := range set {
				if ng, ok := s.newGid[g]; ok {
					ns[ng] = true
				}
			}
			if len(ns) == 0 {
				return nil, false
			}
			out[i] = ns
		}
		return out, true
	}
	newBack, ok := filterSets(sOld.Backtrack)
	if !ok {
		return nil
	}
	newInput, ok := filterSets(sOld.Input)
	if !ok {
		return nil
	}
	newLook, ok := filterSets(sOld.Lookahead)
	if !ok {
		return nil
	}
	return &gtab.ChainedSeqContext3{
		Backtrack: newBack,
		Input:     newInput,
		Lookahead: newLook,
		Actions:   append([]gtab.SeqLookup(nil), sOld.Actions...),
	}
}

// subsetGsub8_1 returns a glyph-filtered copy of sOld, or nil if no input
// glyph survives.  Backtrack and Lookahead coverage tables are filtered;
// if any becomes empty the whole subtable is dropped (the rule can never
// fire).
func (s *subsetter) subsetGsub8_1(sOld *gtab.Gsub8_1) *gtab.Gsub8_1 {
	// Walk s.glyphs in newGid order so coverage indices come out in
	// ascending-gid order, required by the encoder.
	filterCov := func(t coverage.Table) coverage.Table {
		out := coverage.Table{}
		for newGid, oldGid := range s.glyphs {
			if _, ok := t[oldGid]; ok {
				out[glyph.ID(newGid)] = len(out)
			}
		}
		return out
	}
	newBack := make([]coverage.Table, len(sOld.Backtrack))
	for i, c := range sOld.Backtrack {
		newBack[i] = filterCov(c)
		if len(newBack[i]) == 0 {
			return nil
		}
	}
	newLook := make([]coverage.Table, len(sOld.Lookahead))
	for i, c := range sOld.Lookahead {
		newLook[i] = filterCov(c)
		if len(newLook[i]) == 0 {
			return nil
		}
	}
	newInput := coverage.Table{}
	var newSubs []glyph.ID
	for newGid, oldGid := range s.glyphs {
		oldIdx, ok := sOld.Input[oldGid]
		if !ok {
			continue
		}
		newInput[glyph.ID(newGid)] = len(newSubs)
		newSubs = append(newSubs, s.getNewGid(sOld.SubstituteGlyphIDs[oldIdx]))
	}
	if len(newInput) == 0 {
		return nil
	}
	return &gtab.Gsub8_1{
		Input:              newInput,
		Backtrack:          newBack,
		Lookahead:          newLook,
		SubstituteGlyphIDs: newSubs,
	}
}

// remapSeqLookupActions returns a new Actions slice with every
// LookupListIndex remapped through oldToNew.  Actions that point at a
// dropped lookup (oldToNew[idx] < 0) are filtered out.
func remapSeqLookupActions(actions []gtab.SeqLookup, oldToNew []int) []gtab.SeqLookup {
	var out []gtab.SeqLookup
	for _, a := range actions {
		if int(a.LookupListIndex) >= len(oldToNew) {
			continue
		}
		newIdx := oldToNew[a.LookupListIndex]
		if newIdx < 0 {
			continue
		}
		out = append(out, gtab.SeqLookup{
			SequenceIndex:   a.SequenceIndex,
			LookupListIndex: gtab.LookupIndex(newIdx),
		})
	}
	return out
}

// remapContextualLookupIndices walks every contextual subtable in
// lookupList, replacing each rule's Actions with a remapped copy.  Subtables
// that are not contextual are left untouched.  The function mutates rule
// structs in place; callers must pass subtables that are not aliased with
// the original lookups (i.e. produced by the subsetSeqContext*/subset…
// helpers above).
func remapContextualLookupIndices(lookupList gtab.LookupList, oldToNew []int) {
	for _, lk := range lookupList {
		for _, sub := range lk.Subtables {
			switch s := sub.(type) {
			case *gtab.SeqContext1:
				for _, rs := range s.Rules {
					for _, r := range rs {
						r.Actions = remapSeqLookupActions(r.Actions, oldToNew)
					}
				}
			case *gtab.SeqContext2:
				for _, rs := range s.Rules {
					for _, r := range rs {
						r.Actions = remapSeqLookupActions(r.Actions, oldToNew)
					}
				}
			case *gtab.SeqContext3:
				s.Actions = remapSeqLookupActions(s.Actions, oldToNew)
			case *gtab.ChainedSeqContext1:
				for _, rs := range s.Rules {
					for _, r := range rs {
						r.Actions = remapSeqLookupActions(r.Actions, oldToNew)
					}
				}
			case *gtab.ChainedSeqContext2:
				for _, rs := range s.Rules {
					for _, r := range rs {
						r.Actions = remapSeqLookupActions(r.Actions, oldToNew)
					}
				}
			case *gtab.ChainedSeqContext3:
				s.Actions = remapSeqLookupActions(s.Actions, oldToNew)
			}
		}
	}
}
