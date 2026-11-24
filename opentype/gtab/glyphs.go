// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"fmt"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/internal/mapfilter"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
)

// CheckGlyphs returns an error if the table refers to a glyph ID numGlyphs
// or larger.  The table must have the structure which [Info.Encode] requires.
func (info *Info) CheckGlyphs(numGlyphs int) error {
	f := newGlyphFilter(numGlyphs)
	for i, lookup := range info.LookupList {
		for _, st := range lookup.Subtables {
			if _, changed := f.subtable(st); changed {
				return fmt.Errorf("lookup %d: glyph ID out of range", i)
			}
		}
	}
	return nil
}

// LimitGlyphs returns a table which does not refer to glyph IDs numGlyphs or
// larger.  A rule which names such a glyph is removed.  An alternate set
// keeps its valid alternates, and a ligature set its valid ligatures.  Where
// a rule gives a set of glyphs or a glyph class, the glyph is removed from
// the set or class, and a contextual rule which uses a class left without
// glyphs is removed.  Subtables in which no rule is left, or which match no
// glyph sequence because a set of glyphs is left empty, are removed from
// their lookup.  The lookup indices are unchanged.
//
// If the receiver can be encoded, so can the result.  Removing glyphs can
// make the encoding of a subtable longer, since a coverage table with gaps
// takes more space.  A subtable which no longer fits its offsets for this
// reason is removed from its lookup.  If the lookup list becomes too long to
// be reached from the table header, every subtable which refers to a missing
// glyph is removed as a whole.
//
// Values which the receiver uses in several places, for example one glyph
// set at many positions of a context, are shared in the same way in the
// result.
//
// If no glyph needs to be removed, info is returned.  The receiver is never
// modified.  A nil receiver returns nil.
func (info *Info) LimitGlyphs(numGlyphs int) *Info {
	if info == nil {
		return nil
	}
	res := info.limitGlyphs(newGlyphFilter(numGlyphs))
	if res != info && !res.headerFits() && info.headerFits() {
		// Without the subtables which changed, no list is longer than
		// before.
		f := newGlyphFilter(numGlyphs)
		f.dropChanged = true
		res = info.limitGlyphs(f)
	}
	return res
}

func (info *Info) limitGlyphs(f *glyphFilter) *Info {
	ll, changed := f.lookupList(info.LookupList)
	if !changed {
		return info
	}
	res := *info
	res.LookupList = ll
	return &res
}

// glyphFilter removes the references to glyphs beyond the end of a font.  The
// methods return their argument, and false, if there is nothing to remove.
//
// A table can reach one value through many references: the readers return
// one Go value for all offsets which lead to the same bytes.  The filter
// processes every such value once and gives all references the same result.
// This keeps the work in proportion to the size of the table in memory, and
// makes the result share what the argument shares.
type glyphFilter struct {
	numGlyphs int

	// dropChanged makes the filter remove the subtables which refer to a
	// missing glyph from their lookup, instead of keeping the rest of them.
	dropChanged bool

	seen memo
}

func newGlyphFilter(numGlyphs int) *glyphFilter {
	return &glyphFilter{numGlyphs: numGlyphs, seen: memo{}}
}

// memo holds the results which a filter has computed so far.
type memo map[any]any

// memoKey identifies the result of type T for one value.  The result type
// is part of the key, since a value can be used in more than one way: a list
// of glyphs may be a replacement sequence in one subtable and an alternate
// set in another.
type memoKey[T any] struct {
	id any
}

// memoize returns the result of compute for the value which id identifies.
// The id is a pointer, or comes from [offsetpool.SliceKey] or
// [offsetpool.MapKey].  The function compute is called once for each value,
// and must give results of different types T for different uses of a value.
func memoize[T any](m memo, id any, compute func() T) T {
	key := memoKey[T]{id}
	if res, ok := m[key]; ok {
		return res.(T)
	}
	res := compute()
	m[key] = res
	return res
}

// limited is a value without its references to missing glyphs, together
// with whether the value had any.
type limited[T any] struct {
	val     T
	changed bool
}

// memoizeLimit is [memoize] for functions which return a new value and
// whether it differs from the old one.
func memoizeLimit[T any](m memo, id any, limit func() (T, bool)) (T, bool) {
	res := memoize(m, id, func() limited[T] {
		val, changed := limit()
		return limited[T]{val, changed}
	})
	return res.val, res.changed
}

func (f *glyphFilter) ok(gid glyph.ID) bool {
	return int(gid) < f.numGlyphs
}

// valid reports whether all glyphs of a sequence exist.
func (f *glyphFilter) valid(seq []glyph.ID) bool {
	if len(seq) == 0 {
		return true
	}
	return memoize(f.seen, offsetpool.SliceKey(seq), func() bool {
		return !slices.ContainsFunc(seq, func(gid glyph.ID) bool { return !f.ok(gid) })
	})
}

func (f *glyphFilter) lookupList(ll LookupList) (LookupList, bool) {
	return filterSlice(ll, replaceWith(f.lookup))
}

func (f *glyphFilter) lookup(lookup *LookupTable) (*LookupTable, bool) {
	return memoizeLimit(f.seen, lookup, func() (*LookupTable, bool) {
		subtables, changed := filterSlice(lookup.Subtables, func(st Subtable) (Subtable, bool, bool) {
			ns, changed := f.subtable(st)
			return ns, !changed || f.usable(ns), changed
		})
		if !changed {
			return lookup, false
		}
		return &LookupTable{Meta: lookup.Meta, Subtables: subtables}, true
	})
}

// usable reports whether a subtable which has lost glyphs stays in its
// lookup.  It must have rules left and must fit its offsets, so that the
// lookup can be encoded.
func (f *glyphFilter) usable(st Subtable) bool {
	if f.dropChanged {
		return false
	}
	return memoize(f.seen, st, func() bool {
		if isEmptySubtable(st) {
			return false
		}
		_, err := st.layout()
		return err == nil
	})
}

func (f *glyphFilter) set(s coverage.Set) (coverage.Set, bool) {
	return memoizeLimit(f.seen, offsetpool.MapKey(s), func() (coverage.Set, bool) {
		return s.Limit(f.numGlyphs)
	})
}

func (f *glyphFilter) sets(ss []coverage.Set) ([]coverage.Set, bool) {
	return filterSlice(ss, replaceWith(f.set))
}

func (f *glyphFilter) classes(cd classdef.Table) (classdef.Table, bool) {
	return memoizeLimit(f.seen, offsetpool.MapKey(cd), func() (classdef.Table, bool) {
		return cd.Limit(f.numGlyphs)
	})
}

// table removes the invalid glyphs from a coverage table which has no data
// attached to its coverage indices.
func (f *glyphFilter) table(cov coverage.Table) (coverage.Table, bool) {
	return memoizeLimit(f.seen, offsetpool.MapKey(cov), func() (coverage.Table, bool) {
		return cov.Limit(f.numGlyphs)
	})
}

func (f *glyphFilter) tables(tt []coverage.Table) ([]coverage.Table, bool) {
	return filterSlice(tt, replaceWith(f.table))
}

// filterCov restricts a coverage table, and the data indexed by its coverage
// indices, to the glyphs which exist.  The data filter fn is called for the
// data items.  It returns the new item, whether to keep the item, and
// whether the item changed, and gives the same result whenever it is called
// for the same item.  If entries are removed, the remaining ones are
// renumbered in order of glyph ID.
func filterCov[T any](f *glyphFilter, cov coverage.Table, data []T, fn func(T) (T, bool, bool)) (coverage.Table, []T, bool) {
	cov, data, removed := coverage.Filter(cov, data, func(gid glyph.ID, x T) bool {
		if !f.ok(gid) {
			return false
		}
		_, keep, _ := fn(x)
		return keep
	})
	data, replaced := filterSlice(data, func(x T) (T, bool, bool) {
		y, _, changed := fn(x)
		return y, true, changed
	})
	return cov, data, removed || replaced
}

// filterSlice applies the data filter fn to the elements of s.  If fn
// neither changes nor removes an element, s itself is returned.  Otherwise
// the result is a new slice, which is empty but not nil if no element is
// left: a list which the readers find empty in a file is not nil either.
func filterSlice[S ~[]T, T any](s S, fn func(T) (T, bool, bool)) (S, bool) {
	var res S
	modified := false
	for i, x := range s {
		y, keep, changed := fn(x)
		if !modified && (changed || !keep) {
			modified = true
			res = append(S{}, s[:i]...)
		}
		if modified && keep {
			res = append(res, y)
		}
	}
	if !modified {
		return s, false
	}
	return res, true
}

// keepAll is the data filter for tables where the items are never removed.
func keepAll[T any](x T) (T, bool, bool) {
	return x, true, false
}

// keepIf returns the data filter which removes the items which are not
// valid, and leaves the others as they are.
func keepIf[T any](valid func(T) bool) func(T) (T, bool, bool) {
	return func(x T) (T, bool, bool) {
		return x, valid(x), false
	}
}

// replaceWith returns the data filter which replaces each item with the
// result of limit.
func replaceWith[T any](limit func(T) (T, bool)) func(T) (T, bool, bool) {
	return func(x T) (T, bool, bool) {
		y, changed := limit(x)
		return y, true, changed
	}
}

// limitLists returns the data filter for tables where each item is a list.
// The filter removes the list elements which are not valid, and removes a
// list which loses all its elements.  A list which is used several times is
// identified by its first element and its length, and is filtered once.  The
// results are stored in m.  Filters which share m and have the same element
// type must agree on which elements are valid.
func limitLists[T any](m memo, valid func(T) bool) func([]T) ([]T, bool, bool) {
	return func(s []T) ([]T, bool, bool) {
		if len(s) == 0 {
			// empty slices share one key, whether they are nil or not
			return s, true, false
		}
		res, changed := memoizeLimit(m, offsetpool.SliceKey(s), func() ([]T, bool) {
			return filterSlice(s, keepIf(valid))
		})
		return res, len(res) > 0, changed
	}
}

func (f *glyphFilter) subtable(st Subtable) (Subtable, bool) {
	return memoizeLimit(f.seen, st, func() (Subtable, bool) {
		return f.limitSubtable(st)
	})
}

func (f *glyphFilter) limitSubtable(st Subtable) (Subtable, bool) {
	switch l := st.(type) {
	case *Gsub1_1:
		// the glyph ID of the substitute wraps around
		cov, changed := mapfilter.Keys(l.Cov, func(gid glyph.ID) bool {
			return f.ok(gid) && f.ok(gid+l.Delta)
		})
		if !changed {
			return l, false
		}
		return &Gsub1_1{Cov: cov, Delta: l.Delta}, true
	case *Gsub1_2:
		cov, subst, changed := filterCov(f, l.Cov, l.SubstituteGlyphIDs, keepIf(f.ok))
		if !changed {
			return l, false
		}
		return &Gsub1_2{Cov: cov, SubstituteGlyphIDs: subst}, true
	case *Gsub2_1:
		cov, repl, changed := filterCov(f, l.Cov, l.Repl, keepIf(f.valid))
		if !changed {
			return l, false
		}
		return &Gsub2_1{Cov: cov, Repl: repl}, true
	case *Gsub3_1:
		cov, alts, changed := filterCov(f, l.Cov, l.Alternates, limitLists(f.seen, f.ok))
		if !changed {
			return l, false
		}
		return &Gsub3_1{Cov: cov, Alternates: alts}, true
	case *Gsub4_1:
		valid := func(lig Ligature) bool { return f.ok(lig.Out) && f.valid(lig.In) }
		cov, repl, changed := filterCov(f, l.Cov, l.Repl, limitLists(f.seen, valid))
		if !changed {
			return l, false
		}
		return &Gsub4_1{Cov: cov, Repl: repl}, true
	case *Gsub8_1:
		input, subst, c1 := filterCov(f, l.Input, l.SubstituteGlyphIDs, keepIf(f.ok))
		backtrack, c2 := f.tables(l.Backtrack)
		lookahead, c3 := f.tables(l.Lookahead)
		if !c1 && !c2 && !c3 {
			return l, false
		}
		return &Gsub8_1{
			Input:              input,
			Backtrack:          backtrack,
			Lookahead:          lookahead,
			SubstituteGlyphIDs: subst,
		}, true

	case *Gpos1_1:
		cov, changed := f.table(l.Cov)
		if !changed {
			return l, false
		}
		return &Gpos1_1{Cov: cov, Adjust: l.Adjust}, true
	case *Gpos1_2:
		cov, adjust, changed := filterCov(f, l.Cov, l.Adjust, keepAll)
		if !changed {
			return l, false
		}
		return &Gpos1_2{Cov: cov, Adjust: adjust}, true
	case *Gpos2_1:
		adjust, changed := mapfilter.Keys(l.Adjust, func(pair glyph.Pair) bool {
			return f.ok(pair.Left) && f.ok(pair.Right)
		})
		if !changed {
			return l, false
		}
		return &Gpos2_1{Adjust: adjust, PositionSecond: l.PositionSecond}, true
	case *Gpos2_2:
		cov, c1 := f.set(l.Cov)
		class1, c2 := f.classes(l.Class1)
		class2, c3 := f.classes(l.Class2)
		if !c1 && !c2 && !c3 {
			return l, false
		}
		return &Gpos2_2{
			Cov:            cov,
			Class1:         class1,
			Class2:         class2,
			Adjust:         l.Adjust,
			PositionSecond: l.PositionSecond,
		}, true
	case *Gpos3_1:
		cov, records, changed := filterCov(f, l.Cov, l.Records, keepAll)
		if !changed {
			return l, false
		}
		return &Gpos3_1{Cov: cov, Records: records}, true
	case *Gpos4_1:
		markCov, markArray, c1 := filterCov(f, l.MarkCov, l.MarkArray, keepAll)
		baseCov, baseArray, c2 := filterCov(f, l.BaseCov, l.BaseArray, keepAll)
		if !c1 && !c2 {
			return l, false
		}
		return &Gpos4_1{MarkCov: markCov, BaseCov: baseCov, MarkArray: markArray, BaseArray: baseArray}, true
	case *Gpos5_1:
		markCov, markArray, c1 := filterCov(f, l.MarkCov, l.MarkArray, keepAll)
		ligCov, ligArray, c2 := filterCov(f, l.LigCov, l.LigArray, keepAll)
		if !c1 && !c2 {
			return l, false
		}
		return &Gpos5_1{MarkCov: markCov, LigCov: ligCov, MarkArray: markArray, LigArray: ligArray}, true
	case *Gpos6_1:
		cov1, array1, c1 := filterCov(f, l.Mark1Cov, l.Mark1Array, keepAll)
		cov2, array2, c2 := filterCov(f, l.Mark2Cov, l.Mark2Array, keepAll)
		if !c1 && !c2 {
			return l, false
		}
		return &Gpos6_1{Mark1Cov: cov1, Mark2Cov: cov2, Mark1Array: array1, Mark2Array: array2}, true

	case *SeqContext1:
		valid := func(r *SeqRule) bool { return f.valid(r.Input) }
		cov, rules, changed := filterCov(f, l.Cov, l.Rules, limitLists(f.seen, valid))
		if !changed {
			return l, false
		}
		return &SeqContext1{Cov: cov, Rules: rules}, true
	case *SeqContext2:
		cov, c1 := f.table(l.Cov)
		input, c2 := f.classes(l.Input)
		if !c1 && !c2 {
			return l, false
		}
		rules := l.Rules
		if c2 {
			used := usedClasses(input)
			rules = limitRuleSets(rules, input, func(r *ClassSeqRule) bool {
				return used.all(r.Input)
			})
		}
		return &SeqContext2{Cov: cov, Input: input, Rules: rules}, true
	case *SeqContext3:
		input, changed := f.sets(l.Input)
		if !changed {
			return l, false
		}
		return &SeqContext3{Input: input, Actions: l.Actions}, true
	case *ChainedSeqContext1:
		valid := func(r *ChainedSeqRule) bool {
			return f.valid(r.Backtrack) && f.valid(r.Input) && f.valid(r.Lookahead)
		}
		cov, rules, changed := filterCov(f, l.Cov, l.Rules, limitLists(f.seen, valid))
		if !changed {
			return l, false
		}
		return &ChainedSeqContext1{Cov: cov, Rules: rules}, true
	case *ChainedSeqContext2:
		cov, c1 := f.table(l.Cov)
		backtrack, c2 := f.classes(l.Backtrack)
		input, c3 := f.classes(l.Input)
		lookahead, c4 := f.classes(l.Lookahead)
		if !c1 && !c2 && !c3 && !c4 {
			return l, false
		}
		rules := l.Rules
		if c2 || c3 || c4 {
			usedBacktrack := usedClasses(backtrack)
			usedInput := usedClasses(input)
			usedLookahead := usedClasses(lookahead)
			rules = limitRuleSets(rules, input, func(r *ChainedClassSeqRule) bool {
				return usedBacktrack.all(r.Backtrack) &&
					usedInput.all(r.Input) &&
					usedLookahead.all(r.Lookahead)
			})
		}
		return &ChainedSeqContext2{
			Cov:       cov,
			Backtrack: backtrack,
			Input:     input,
			Lookahead: lookahead,
			Rules:     rules,
		}, true
	case *ChainedSeqContext3:
		backtrack, c1 := f.sets(l.Backtrack)
		input, c2 := f.sets(l.Input)
		lookahead, c3 := f.sets(l.Lookahead)
		if !c1 && !c2 && !c3 {
			return l, false
		}
		return &ChainedSeqContext3{
			Backtrack: backtrack,
			Input:     input,
			Lookahead: lookahead,
			Actions:   l.Actions,
		}, true
	}
	return st, false
}

// classSet is the set of glyph classes which have glyphs.
type classSet map[uint16]bool

// usedClasses returns the classes of a class definition table which have
// glyphs.  Class 0 holds the glyphs which the table does not list.
func usedClasses(cd classdef.Table) classSet {
	used := classSet{0: true}
	for _, class := range cd {
		used[class] = true
	}
	return used
}

// all reports whether every one of the given classes has glyphs.
func (used classSet) all(classes []uint16) bool {
	for _, class := range classes {
		if !used[class] {
			return false
		}
	}
	return true
}

// limitRuleSets removes the rules which can no longer match from the rule
// sets of a class-based context subtable.  The rule sets are indexed by the
// class of the first input glyph, as given by the class definition table
// input.  Rule sets for classes without glyphs are removed, and so are the
// rules for which keep returns false.  A rule set left without rules is nil.
func limitRuleSets[R any](sets [][]*R, input classdef.Table, keep func(*R) bool) [][]*R {
	used := usedClasses(input)
	// Whether a rule is kept depends on the classes of its subtable, so the
	// results for shared rules and rule sets are valid for this call only.
	seen := memo{}
	limit := limitLists(seen, func(r *R) bool {
		return memoize(seen, r, func() bool { return keep(r) })
	})
	res := make([][]*R, min(len(sets), input.NumClasses()))
	for class := range res {
		if !used[uint16(class)] {
			continue
		}
		if rules, keep, _ := limit(sets[class]); keep {
			res[class] = rules
		}
	}
	return res
}

// hasRules reports whether one of the rule sets has a rule.
func hasRules[R any](sets [][]*R) bool {
	return slices.ContainsFunc(sets, func(rules []*R) bool { return len(rules) > 0 })
}

// isEmptySubtable reports whether a subtable has no rules which could
// match.
func isEmptySubtable(st Subtable) bool {
	switch l := st.(type) {
	case *Gsub1_1:
		return len(l.Cov) == 0
	case *Gsub1_2:
		return len(l.Cov) == 0
	case *Gsub2_1:
		return len(l.Cov) == 0
	case *Gsub3_1:
		return len(l.Cov) == 0
	case *Gsub4_1:
		return len(l.Cov) == 0
	case *Gsub8_1:
		return len(l.Input) == 0 || hasEmpty(l.Backtrack) || hasEmpty(l.Lookahead)
	case *Gpos1_1:
		return len(l.Cov) == 0
	case *Gpos1_2:
		return len(l.Cov) == 0
	case *Gpos2_1:
		return len(l.Adjust) == 0
	case *Gpos2_2:
		return len(l.Cov) == 0
	case *Gpos3_1:
		return len(l.Cov) == 0
	case *Gpos4_1:
		return len(l.MarkCov) == 0 || len(l.BaseCov) == 0
	case *Gpos5_1:
		return len(l.MarkCov) == 0 || len(l.LigCov) == 0
	case *Gpos6_1:
		return len(l.Mark1Cov) == 0 || len(l.Mark2Cov) == 0
	case *SeqContext1:
		return len(l.Cov) == 0
	case *SeqContext2:
		return len(l.Cov) == 0 || !hasRules(l.Rules)
	case *ChainedSeqContext1:
		return len(l.Cov) == 0
	case *ChainedSeqContext2:
		return len(l.Cov) == 0 || !hasRules(l.Rules)
	case *SeqContext3:
		return hasEmpty(l.Input)
	case *ChainedSeqContext3:
		return hasEmpty(l.Backtrack) || hasEmpty(l.Input) || hasEmpty(l.Lookahead)
	default:
		return false
	}
}

// hasEmpty reports whether one of the glyph sets, each of which stands for a
// position in a glyph sequence, has no glyphs.
func hasEmpty[S ~map[glyph.ID]V, V any](sets []S) bool {
	return slices.ContainsFunc(sets, func(s S) bool { return len(s) == 0 })
}
