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
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/gdef"
)

// Context holds all information required to apply a lookup to a glyph sequence.
type Context struct {
	lookups []LookupIndex
	ll      LookupList
	gdef    *gdef.Table

	seq    []glyph.Info
	lookup *LookupTable

	// keep represents the lookup flags.  Glyphs for which keep returns false
	// must be skipped when constructing the input sequence.
	keep *KeepFunc

	stack []*nested

	scratch []int
}

type nested struct {
	// InputPos gives the glyph positions for the matched input sequence,
	// in ascending order.
	InputPos []int

	// Actions gives the remaining sub-lookups to be applied.
	Actions []SeqLookup

	// EndPos is the position after the last glyph which can be matched
	// by sub-lookups.
	EndPos int
}

// NewContext creates a new layout engine.
// The engine applies the given lookups in the given order.
// The gdef parameter, if non-nil, is used to resolve glyph classes.
func (ll LookupList) NewContext(lookups []LookupIndex, gdef *gdef.Table) *Context {
	return &Context{lookups: lookups, ll: ll, gdef: gdef}
}

// ApplyAll applies the lookups to the given sequence of glyphs.
//
// This is the main entry-point for external users of GSUB and GPOS tables.
func (ctx *Context) ApplyAll(seq []glyph.Info) []glyph.Info {
	for _, lookupIndex := range ctx.lookups {
		if int(lookupIndex) >= len(ctx.ll) {
			continue
		}

		ctx.seq = seq
		ctx.lookup = ctx.ll[lookupIndex]
		ctx.keep = newKeepFunc(ctx.ll[lookupIndex].Meta, ctx.gdef)

		pos := 0
		// TODO(voss): GSUB 8.1 subtables are applied in reverse order.
		for pos < len(ctx.seq) {
			oldTodo := len(ctx.seq) - pos
			pos = ctx.ApplyAtRecursively(pos)

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

// ApplyAtRecursively applies a single lookup to the given glyphs at position
// pos.  It returns the new glyph sequence and position for the next lookup.
func (ctx *Context) ApplyAtRecursively(pos int) int {
	// Check if the lookup applies to the input sequence.
	if !ctx.keep.Keep(ctx.seq[pos].GID) {
		return pos + 1
	}
	next := ctx.ApplyAt(ctx.lookup.Subtables, pos, len(ctx.seq))
	if next < 0 {
		return pos + 1
	}

	numActions := 1
	for len(ctx.stack) > 0 && numActions < 64 {
		k := len(ctx.stack) - 1
		if len(ctx.stack[k].Actions) == 0 {
			if k == 0 {
				next = ctx.stack[0].EndPos
			}
			ctx.scratch = ctx.stack[k].InputPos
			ctx.stack = ctx.stack[:k]
			continue
		}

		numActions++

		lookupIndex := ctx.stack[k].Actions[0].LookupListIndex
		seqIdx := ctx.stack[k].Actions[0].SequenceIndex
		if int(seqIdx) >= len(ctx.stack[k].InputPos) {
			continue
		}
		pos := ctx.stack[k].InputPos[seqIdx]
		end := ctx.stack[k].EndPos
		ctx.stack[k].Actions = ctx.stack[k].Actions[1:]

		if int(lookupIndex) >= len(ctx.ll) {
			continue
		}

		lookup := ctx.ll[lookupIndex]
		keep := newKeepFunc(lookup.Meta, ctx.gdef)

		if keep.Keep(ctx.seq[pos].GID) {
			oldLookup := ctx.lookup
			ctx.lookup = lookup
			oldKeep := ctx.keep
			ctx.keep = keep
			ctx.ApplyAt(lookup.Subtables, pos, end)
			ctx.lookup = oldLookup
			ctx.keep = oldKeep
		}
	}

	return next
}

// ApplyAt tries the subtables one by one and applies the first one that
// matches.  If no subtable matches, a -1 is returned.
func (ctx *Context) ApplyAt(ss []Subtable, pos, b int) int {
	for _, subtable := range ss {
		next := subtable.Apply(ctx, pos, b)
		if next >= 0 {
			return next
		}
	}
	return -1
}

// FixStackInsert adjusts the stack of nested actions after replacing the glyph
// at `pos` with a sequence of `num` glyphs.  If the glyph at `pos` is part of
// an input sequence, the new glyphs are inserted into the input sequence.
func (ctx *Context) FixStackInsert(pos, num int) {
	for _, action := range ctx.stack {
		if action.EndPos <= pos {
			continue
		}

		hasPosAt := -1
		for i, p := range action.InputPos {
			if p == pos {
				hasPosAt = i
			} else if p > pos {
				action.InputPos[i] += num - 1
			}
		}
		action.EndPos += num - 1

		if hasPosAt >= 0 {
			i := hasPosAt

			// move the tail out by num-1 positions
			action.InputPos = slices.Grow(action.InputPos, num-1)
			action.InputPos = action.InputPos[:len(action.InputPos)+num-1]
			copy(action.InputPos[i+num:], action.InputPos[i+1:])

			// insert the new positions
			for j := 1; j < num; j++ {
				action.InputPos[i+j] = pos + j
			}
		}
	}
}

// FixStackMerge adjusts the stack of nested actions after merging the glyphs
// in `pos` into a single glyph at `pos[0]`.  If any of the glyphs in `pos` is part of an
// input sequence, the new glyph is inserted into the input sequence.
func (ctx *Context) FixStackMerge(pos []int) {
	for _, action := range ctx.stack {
		if action.EndPos <= pos[0] {
			continue
		}

		// We remove all the glyphs in `pos[1:]` from the input sequence, and
		// determine whether one of the glyphs in the input sequence was in
		// pos.
		i := 0
		j := 0
		in := action.InputPos
		delta := 0
		needsMergePos := false
		for i < len(pos) && j < len(in) {
			if pos[i] < in[j] { // a merged glyph, not in the input sequence
				if i > 0 {
					delta++
				}
				i++
			} else if pos[i] > in[j] { // a glyph in the input sequence, not merged
				in[j] -= delta
				j++
			} else { // a glyph in the input sequence, merged
				if i == 0 || i == len(pos)-1 {
					needsMergePos = true
				}

				if i > 0 {
					delta++
					in = slices.Delete(in, j, j+1)
				} else {
					j++
				}
				i++
			}
		}
		for j < len(in) {
			in[j] -= delta
			j++
		}

		// We need to decide whether or not to add the new glyphs to the input
		// glyph sequence of this action.  The behaviour is not specified in
		// the OpenType spec, so we try to imitate the behavior of the Windows
		// layout engine.  The rules seem very complicated, and I failed to
		// reverse engineer the rules completely.  The rule we are using here
		// is that we include the new glyphs, if and only if one of the
		// endpoints of the match was included in the original action input
		// sequence.
		//
		// See the test cases for TestGsub for details.
		idx, hasMergePos := slices.BinarySearch(in, pos[0])
		if needsMergePos && !hasMergePos {
			in = slices.Insert(in, idx, pos[0])
		} else if hasMergePos && !needsMergePos {
			in = slices.Delete(in, idx, idx+1)
		}

		action.InputPos = in
		action.EndPos -= delta
	}
}
