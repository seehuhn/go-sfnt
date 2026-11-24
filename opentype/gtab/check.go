// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"errors"
	"fmt"
	"slices"

	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
)

// check returns an error if info cannot be encoded as a table of type tp.
// It verifies the feature and lookup indices, the lookup types and the
// language tags.  The constraints on the subtables, including their coverage
// tables, are verified when the lookup list is encoded.
func (info *Info) check(tp Type) error {
	if err := info.ScriptList.checkFeatures(len(info.FeatureList)); err != nil {
		return err
	}
	if err := info.ScriptList.checkTags(); err != nil {
		return err
	}

	numLookups := len(info.LookupList)
	checked := map[any]bool{}
	for _, feature := range info.FeatureList {
		key := offsetpool.SliceKey(feature.Lookups)
		if checked[key] {
			continue
		}
		checked[key] = true
		if !lookupsInRange(feature.Lookups, numLookups) {
			return errors.New("feature: lookup index out of range")
		}
	}
	for _, record := range info.Variations {
		for _, sub := range record.Substitutions {
			if int(sub.FeatureIndex) >= len(info.FeatureList) {
				return errors.New("feature variations: feature index out of range")
			}
			if !lookupsInRange(sub.Lookups, numLookups) {
				return errors.New("feature variations: lookup index out of range")
			}
		}
	}

	for i, lookup := range info.LookupList {
		for _, st := range lookup.Subtables {
			if !lookupTypeMatches(st, lookup.Meta.LookupType, tp) {
				return fmt.Errorf("lookup %d: subtable does not match lookup type %d",
					i, lookup.Meta.LookupType)
			}

			valid := true
			forEachActions(st, func(actions *[]SeqLookup) {
				for _, a := range *actions {
					if int(a.LookupListIndex) >= numLookups {
						valid = false
					}
				}
			})
			if !valid {
				return fmt.Errorf("lookup %d: nested lookup index out of range", i)
			}
		}
	}
	return nil
}

// lookupTypeMatches reports whether a lookup of the given type, in a table of
// type tp, can hold the subtable.
func lookupTypeMatches(st Subtable, lookupType uint16, tp Type) bool {
	var gsub, gpos uint16
	switch st.(type) {
	case *Gsub1_1:
		gsub = 1
	case *Gsub1_2:
		gsub = 1
	case *Gsub2_1:
		gsub = 2
	case *Gsub3_1:
		gsub = 3
	case *Gsub4_1:
		gsub = 4
	case *Gsub8_1:
		gsub = 8
	case *Gpos1_1, *Gpos1_2:
		gpos = 1
	case *Gpos2_1, *Gpos2_2:
		gpos = 2
	case *Gpos3_1:
		gpos = 3
	case *Gpos4_1:
		gpos = 4
	case *Gpos5_1:
		gpos = 5
	case *Gpos6_1:
		gpos = 6
	case *SeqContext1, *SeqContext2, *SeqContext3:
		gsub, gpos = 5, 7
	case *ChainedSeqContext1, *ChainedSeqContext2, *ChainedSeqContext3:
		gsub, gpos = 6, 8
	default:
		return true
	}
	want := gsub
	if tp == TypeGpos {
		want = gpos
	}
	return want != 0 && lookupType == want
}

// forEachActions calls fn for the list of nested lookup actions of every rule
// of a contextual subtable.  The function may replace the list.
//
// A subtable read from a file can reach one rule, or one rule set, through
// many offsets.  Such rules and rule sets are visited once, so that the work
// is in proportion to the size of the subtable in memory.
func forEachActions(st Subtable, fn func(*[]SeqLookup)) {
	switch l := st.(type) {
	case *SeqContext1:
		forEachRule(l.Rules, func(r *SeqRule) { fn(&r.Actions) })
	case *SeqContext2:
		forEachRule(l.Rules, func(r *ClassSeqRule) { fn(&r.Actions) })
	case *SeqContext3:
		fn(&l.Actions)
	case *ChainedSeqContext1:
		forEachRule(l.Rules, func(r *ChainedSeqRule) { fn(&r.Actions) })
	case *ChainedSeqContext2:
		forEachRule(l.Rules, func(r *ChainedClassSeqRule) { fn(&r.Actions) })
	case *ChainedSeqContext3:
		fn(&l.Actions)
	}
}

// forEachRule calls fn once for every distinct rule of the rule sets.
func forEachRule[R any](sets [][]*R, fn func(*R)) {
	seenSets := map[any]bool{}
	seenRules := map[*R]bool{}
	for _, rules := range sets {
		key := offsetpool.SliceKey(rules)
		if seenSets[key] {
			continue
		}
		seenSets[key] = true
		for _, r := range rules {
			if seenRules[r] {
				continue
			}
			seenRules[r] = true
			fn(r)
		}
	}
}

// dropInvalidLookups removes the lookup indices which are out of range for
// the lookup list, from the features and from the nested lookup actions.
func (info *Info) dropInvalidLookups() {
	numLookups := len(info.LookupList)
	// features which share a list of lookups keep sharing the repaired list
	repaired := map[any][]LookupIndex{}
	for _, feature := range info.FeatureList {
		key := offsetpool.SliceKey(feature.Lookups)
		lookups, ok := repaired[key]
		if !ok {
			lookups = limitLookups(feature.Lookups, numLookups)
			repaired[key] = lookups
		}
		feature.Lookups = lookups
	}
	for _, lookup := range info.LookupList {
		for _, st := range lookup.Subtables {
			forEachActions(st, func(actions *[]SeqLookup) {
				valid := func(a SeqLookup) bool { return int(a.LookupListIndex) < numLookups }
				if !slices.ContainsFunc(*actions, func(a SeqLookup) bool { return !valid(a) }) {
					return
				}
				*actions = slices.DeleteFunc(slices.Clone(*actions),
					func(a SeqLookup) bool { return !valid(a) })
			})
		}
	}
}

// CheckMarkFilteringSets returns an error if a lookup selects a mark
// filtering set with an index of numSets or larger.  Here numSets is the
// number of mark glyph sets in the GDEF table of the font, or 0 if there is
// none.
func (info *Info) CheckMarkFilteringSets(numSets int) error {
	for i, lookup := range info.LookupList {
		if !markSetValid(lookup, numSets) {
			return fmt.Errorf("lookup %d: mark filtering set out of range", i)
		}
	}
	return nil
}

// LimitMarkFilteringSets returns a table in which no lookup selects a mark
// filtering set with an index of numSets or larger.  Where a lookup does,
// the UseMarkFilteringSet flag and the set index are cleared, so that the
// lookup applies to all marks.  The number of lookups is not changed.
//
// If no lookup needs to change, info is returned.  The receiver is never
// modified.  A nil receiver returns nil.
func (info *Info) LimitMarkFilteringSets(numSets int) *Info {
	if info == nil {
		return nil
	}
	var ll LookupList
	for i, lookup := range info.LookupList {
		if markSetValid(lookup, numSets) {
			continue
		}
		if ll == nil {
			ll = slices.Clone(info.LookupList)
		}
		meta := *lookup.Meta
		meta.LookupFlags &^= UseMarkFilteringSet
		meta.MarkFilteringSet = 0
		ll[i] = &LookupTable{Meta: &meta, Subtables: lookup.Subtables}
	}
	if ll == nil {
		return info
	}
	res := *info
	res.LookupList = ll
	return &res
}

func markSetValid(lookup *LookupTable, numSets int) bool {
	meta := lookup.Meta
	return meta.LookupFlags&UseMarkFilteringSet == 0 || int(meta.MarkFilteringSet) < numSets
}
