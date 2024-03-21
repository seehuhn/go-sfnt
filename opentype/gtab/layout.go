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

		ctx := &Context{
			Seq:    seq,
			Engine: e,
			Lookup: e.ll[lookupIndex],
			Keep:   newKeepFunc(e.ll[lookupIndex].Meta, e.gdef),
		}

		pos := 0
		// TODO(voss): GSUB 8.1 subtables are applied in reverse order.
		for pos < len(ctx.Seq) {
			oldTodo := len(ctx.Seq) - pos
			pos = ctx.At(pos)

			// Make sure that every step makes some progress.
			// TODO(voss): Is this needed?
			newTodo := len(ctx.Seq) - pos
			if newTodo >= oldTodo {
				pos = len(ctx.Seq) - oldTodo + 1
			}
			oldTodo = newTodo
		}
		seq = ctx.Seq
	}
	return seq
}

type Context struct {
	Seq    []glyph.Info
	Engine *LayoutEngine
	Lookup *LookupTable
	Keep   *KeepFunc
}

// At applies a single lookup to the given glyphs at position pos.
// It returns the new glyph sequence and position for the next lookup.
func (ctx *Context) At(pos int) int {
	// Check if the lookup applies to the input sequence.
	if !ctx.Keep.Keep(ctx.Seq[pos].GID) {
		return pos + 1
	}
	match := ctx.Lookup.Subtables.Apply(ctx.Keep, ctx.Seq, pos, len(ctx.Seq))
	if match == nil {
		return pos + 1
	}
	next := match.Next

	if len(match.Replace) > 0 {
		if len(match.Actions) > 0 {
			panic("invalid match object")
		}
		ctx.Seq = applyMatch(ctx.Seq, match, pos)
		return next + len(match.Replace) - len(match.InputPos)
	}

	stack := []*nested{
		{
			InputPos: match.InputPos,
			Actions:  match.Actions,
			EndPos:   match.Next,
		},
	}

	numActions := 1
	for len(stack) > 0 && numActions < 64 {
		k := len(stack) - 1
		if len(stack[k].Actions) == 0 {
			stack = stack[:k]
			continue
		}

		numActions++

		lookupIndex := stack[k].Actions[0].LookupListIndex
		seqIdx := stack[k].Actions[0].SequenceIndex
		if int(seqIdx) >= len(stack[k].InputPos) {
			continue
		}
		pos := stack[k].InputPos[seqIdx]
		end := stack[k].EndPos
		stack[k].Actions = stack[k].Actions[1:]

		if int(lookupIndex) >= len(ctx.Engine.ll) {
			continue
		}
		lookup := ctx.Engine.ll[lookupIndex]

		keep := newKeepFunc(lookup.Meta, ctx.Engine.gdef)
		var match *Match
		if keep.Keep(ctx.Seq[pos].GID) {
			match = lookup.Subtables.Apply(keep, ctx.Seq, pos, end)
		}
		if match == nil {
			continue
		}

		if len(match.Replace) > 0 {
			if len(match.Actions) > 0 {
				panic("invalid match object")
			}
			ctx.Seq = applyMatch(ctx.Seq, match, pos)
			fixActionStack(stack, match.InputPos, len(match.Replace))
			next += len(match.Replace) - len(match.InputPos)
		} else {
			stack = append(stack, &nested{
				InputPos: match.InputPos,
				Actions:  match.Actions,
				EndPos:   match.Next,
			})
		}
	}

	return next
}
