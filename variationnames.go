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
// untouched.
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
	extra := make(map[name.ID]string)
	next := name.ID(256)
	seen := make(map[string]name.ID)
	alloc := func(s string) uint16 {
		if id, ok := seen[s]; ok {
			return uint16(id)
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
