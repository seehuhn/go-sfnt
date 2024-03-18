// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2024  Jochen Voss <voss@seehuhn.de>
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

// A LayoutEngine applies a sequence of lookups to a sequence of glyphs.
type LayoutEngine struct {
	lookups []LookupIndex
	ll      LookupList
	gdef    *gdef.Table
}

// NewEngine creates a new layout engine.
// The engine applies the given lookups in the given order.
// The gdef parameter, if non-nil, is used to resolve glyph classes.
func (ll LookupList) NewEngine(lookups []LookupIndex, gdef *gdef.Table) *LayoutEngine {
	return &LayoutEngine{lookups: lookups, ll: ll, gdef: gdef}
}

// Apply applies the lookups to the given sequence of glyphs.
//
// This is the main entry-point for external users of GSUB and GPOS tables.
func (e *LayoutEngine) Apply(seq []glyph.Info) []glyph.Info {
	for _, lookupIndex := range e.lookups {
		if int(lookupIndex) >= len(e.ll) {
			continue
		}

		ctx := &applyLookup{
			ll:     e.ll,
			lookup: e.ll[lookupIndex],
			seq:    seq,
			gdef:   e.gdef,
			keep:   newKeepFunc(e.ll[lookupIndex].Meta, e.gdef),
		}

		pos := 0
		// TODO(voss): GSUB 8.1 subtables are applied in reverse order.
		for pos < len(ctx.seq) {
			oldTodo := len(ctx.seq) - pos
			pos = ctx.At(pos)

			// Make sure that every step makes some progress.
			// TODO(voss): Is this needed?
			newTodo := len(ctx.seq) - pos
			if newTodo >= oldTodo {
				pos = len(ctx.seq) - oldTodo + 1
			}
			oldTodo = newTodo
		}
		seq = ctx.seq
	}
	return seq
}
