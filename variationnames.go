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
	"slices"

	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/name"
	"seehuhn.de/go/sfnt/stat"
)

// canonicalizeVariationNames assigns deterministic "name" table IDs (>= 256)
// to the fvar/STAT name strings in fv and st, in the order: fvar axes, fvar
// instances (name, then PostScript name), STAT design axes, STAT axis
// values, then the STAT elided fallback name.  Identical strings share one
// ID.  An entry whose resolved Name is empty keeps its existing numeric ID
// untouched, and the allocator skips over all such preserved IDs so it never
// hands out one of them to a different entry.
//
// fv and st are modified in place; either may be nil.  Read and Write share
// this function so that a font's stored NameIDs always end up identical to
// the ones Write would assign, keeping read-write-read round trips stable.
// Write applies it to clones (see cloneFvar/cloneStat) so the caller's Font
// is never mutated; Read applies it directly to the freshly decoded tables.
//
// The return value carries the allocated strings keyed by ID, for
// registration in the "name" table; it is nil when no strings were
// allocated.
func canonicalizeVariationNames(fv *fvar.Table, st *stat.Table) map[name.ID]string {
	preserved := preservedVariationNameIDs(fv, st)

	extra := make(map[name.ID]string)
	next := name.ID(256)
	seen := make(map[string]name.ID)
	alloc := func(s string) uint16 {
		if id, ok := seen[s]; ok {
			return uint16(id)
		}
		for preserved[next] {
			next++
		}
		id := next
		next++
		seen[s] = id
		extra[id] = s
		return uint16(id)
	}

	if fv != nil {
		for i := range fv.Axes {
			if fv.Axes[i].Name != "" {
				fv.Axes[i].NameID = alloc(fv.Axes[i].Name)
			}
		}
		for i := range fv.Instances {
			inst := &fv.Instances[i]
			if inst.Name != "" {
				inst.NameID = alloc(inst.Name)
			}
			if inst.PostScriptName != "" {
				inst.PostScriptNameID = alloc(inst.PostScriptName)
			}
		}
	}

	if st != nil {
		for i := range st.DesignAxes {
			if st.DesignAxes[i].Name != "" {
				st.DesignAxes[i].NameID = alloc(st.DesignAxes[i].Name)
			}
		}
		for i, av := range st.AxisValues {
			st.AxisValues[i] = assignAxisValueName(av, alloc)
		}
		if st.ElidedFallbackName != "" {
			st.ElidedFallbackNameID = alloc(st.ElidedFallbackName)
		}
	}

	if len(extra) == 0 {
		return nil
	}
	return extra
}

// preservedVariationNameIDs collects the numeric name IDs of entries whose
// resolved Name is empty.  canonicalizeVariationNames leaves those IDs
// untouched, so its allocator must avoid handing any of them out to a
// different entry.
func preservedVariationNameIDs(fv *fvar.Table, st *stat.Table) map[name.ID]bool {
	preserved := make(map[name.ID]bool)

	if fv != nil {
		for i := range fv.Axes {
			if fv.Axes[i].Name == "" {
				preserved[name.ID(fv.Axes[i].NameID)] = true
			}
		}
		for i := range fv.Instances {
			inst := &fv.Instances[i]
			if inst.Name == "" {
				preserved[name.ID(inst.NameID)] = true
			}
			// PostScriptNameID 0xFFFF marks "absent", not a preserved ID.
			if inst.PostScriptName == "" && inst.PostScriptNameID != 0xFFFF {
				preserved[name.ID(inst.PostScriptNameID)] = true
			}
		}
	}

	if st != nil {
		for i := range st.DesignAxes {
			if st.DesignAxes[i].Name == "" {
				preserved[name.ID(st.DesignAxes[i].NameID)] = true
			}
		}
		for _, av := range st.AxisValues {
			if id, ok := emptyAxisValueNameID(av); ok {
				preserved[id] = true
			}
		}
		if st.ElidedFallbackName == "" {
			preserved[name.ID(st.ElidedFallbackNameID)] = true
		}
	}

	return preserved
}

// emptyAxisValueNameID returns av's NameID and true when its resolved Name
// is empty.
func emptyAxisValueNameID(av stat.AxisValue) (name.ID, bool) {
	switch v := av.(type) {
	case *stat.Format1:
		if v.Name == "" {
			return name.ID(v.NameID), true
		}
	case *stat.Format2:
		if v.Name == "" {
			return name.ID(v.NameID), true
		}
	case *stat.Format3:
		if v.Name == "" {
			return name.ID(v.NameID), true
		}
	case *stat.Format4:
		if v.Name == "" {
			return name.ID(v.NameID), true
		}
	}
	return 0, false
}

func cloneFvar(t *fvar.Table) *fvar.Table {
	c := *t
	c.Axes = slices.Clone(t.Axes)
	c.Instances = slices.Clone(t.Instances)
	for i := range c.Instances {
		c.Instances[i].Coordinates = slices.Clone(t.Instances[i].Coordinates)
	}
	return &c
}

func cloneStat(t *stat.Table) *stat.Table {
	c := *t
	c.DesignAxes = slices.Clone(t.DesignAxes)
	c.AxisValues = slices.Clone(t.AxisValues) // elements replaced by the caller
	return &c
}

// assignAxisValueName returns a copy of av with its NameID reassigned when its
// resolved Name is non-empty.
func assignAxisValueName(av stat.AxisValue, alloc func(string) uint16) stat.AxisValue {
	switch v := av.(type) {
	case *stat.Format1:
		c := *v
		if c.Name != "" {
			c.NameID = alloc(c.Name)
		}
		return &c
	case *stat.Format2:
		c := *v
		if c.Name != "" {
			c.NameID = alloc(c.Name)
		}
		return &c
	case *stat.Format3:
		c := *v
		if c.Name != "" {
			c.NameID = alloc(c.Name)
		}
		return &c
	case *stat.Format4:
		c := *v
		c.Values = slices.Clone(v.Values)
		if c.Name != "" {
			c.NameID = alloc(c.Name)
		}
		return &c
	default:
		return av
	}
}
