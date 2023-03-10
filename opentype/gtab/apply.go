// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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
	"math"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/gdef"
)

// ApplyLookup applies a single lookup to the given glyphs.
func (ll LookupList) ApplyLookup(seq []glyph.Info, lookupIndex LookupIndex, gdef *gdef.Table) []glyph.Info {
	pos := 0
	numLeft := len(seq)
	for pos < len(seq) {
		// TODO(voss): GSUB 8.1 subtables are applied in reverse order.
		seq, pos = ll.applyLookupAt(seq, lookupIndex, gdef, pos, len(seq))
		newNumLeft := len(seq) - pos
		if newNumLeft >= numLeft {
			pos = len(seq) - numLeft + 1
		}
		numLeft = newNumLeft
	}

	return seq
}

// Match describes the effect of applying a Lookup to a glyph sequence.
type Match struct {
	InputPos []int // in increasing order
	Replace  []glyph.Info
	Actions  SeqLookups
	Next     int
}

type nested struct {
	InputPos []int
	Actions  SeqLookups
	EndPos   int
}

// applyLookupAt applies a single lookup to the given glyphs at position pos.
func (ll LookupList) applyLookupAt(seq []glyph.Info, lookupIndex LookupIndex, gdef *gdef.Table, pos, b int) ([]glyph.Info, int) {
	numLookups := len(ll)
	stack := []*nested{
		{
			InputPos: []int{pos},
			Actions: SeqLookups{
				{SequenceIndex: 0, LookupListIndex: lookupIndex},
			},
			EndPos: b,
		},
	}

	next := pos + 1
	nextUpdated := false

	numActions := 0
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

		if int(lookupIndex) >= numLookups {
			continue
		}
		lookup := ll[lookupIndex]

		keep := makeFilter(lookup.Meta, gdef)
		var match *Match
		if keep(seq[pos].Gid) {
			// We have to pass keep into Apply, so that lookups operating on
			// sequences of glyphs can check the glyps different from seq[pos].
			match = lookup.Subtables.Apply(keep, seq, pos, end)
		}
		if match == nil {
			continue
		}

		if !nextUpdated {
			next = match.Next
			nextUpdated = true
		}

		if len(match.Replace) > 0 {
			if len(match.Actions) > 0 {
				panic("invalid match object")
			}
			seq = applyMatch(seq, match, pos)
			fixMatchPos(stack, match.InputPos, len(match.Replace))
			next += len(match.Replace) - len(match.InputPos)
		} else {
			stack = append(stack, &nested{
				InputPos: match.InputPos,
				Actions:  match.Actions,
				EndPos:   match.Next,
			})
		}
	}

	return seq, next
}

func fixMatchPos(actions []*nested, remove []int, numInsert int) {
	if len(actions) == 0 {
		return
	}

	minPos := math.MaxInt
	maxPos := math.MinInt
	for _, action := range actions {
		for _, pos := range action.InputPos {
			if pos < minPos {
				minPos = pos
			}
			if pos > maxPos {
				maxPos = pos
			}
		}
		if action.EndPos > maxPos {
			maxPos = action.EndPos
		}
	}

	insertPos := remove[0]
	lastRemoved := remove[len(remove)-1]

	newPos := make([]int, maxPos-minPos+1)
	for i := range newPos {
		newPos[i] = minPos + i
	}
	for l := len(remove) - 1; l >= 0; l-- {
		i := remove[l]
		if i < insertPos {
			panic("inconsistent insert position")
		}
		start := i + 1
		if i >= minPos {
			newPos[i-minPos] = -1
		} else {
			start = minPos
		}
		for j := start; j <= maxPos; j++ {
			newPos[j-minPos]--
		}
	}

	for _, action := range actions {
		numRemoved := 0
		for _, pos := range remove {
			if pos < action.EndPos {
				numRemoved++
			} else {
				break
			}
		}

		var out []int
		in := action.InputPos
		for len(in) > 0 && in[0] < insertPos {
			out = append(out, in[0])
			in = in[1:]
		}

		// Decide whether or not to add the new glyphs to the input glyph
		// sequence of this action. We try to imitate the behavior of the
		// Windows layout engine, but I failed to reverse engineer the rules
		// completely.  The rule we are using here is that we include the
		// new glyphs, if and only if one of the endpoints of the match
		// was included in the original action input sequence.
		addToInput := false
		if len(in) > 0 && in[0] == insertPos {
			// first matched glyph was present
			addToInput = true
		} else {
			// final matched glyph was present
			for i := 0; i < len(in); i++ {
				if in[i] == lastRemoved {
					addToInput = true
				}
				if in[i] >= lastRemoved {
					break
				}
			}
		}

		if addToInput {
			for j := 0; j < numInsert; j++ {
				out = append(out, insertPos+j)
			}
		}
		for _, pos := range in {
			pos = newPos[pos-minPos]
			if pos >= 0 {
				out = append(out, pos+numInsert)
			}
		}
		action.InputPos = out
		action.EndPos += numInsert - numRemoved
	}
}

func applyMatch(seq []glyph.Info, m *Match, pos int) []glyph.Info {
	matchPos := m.InputPos

	oldLen := len(seq)
	oldTailPos := matchPos[len(matchPos)-1] + 1
	tailLen := oldLen - oldTailPos
	newLen := oldLen - len(matchPos) + len(m.Replace)
	newTailPos := newLen - tailLen

	var newText []rune
	for _, offs := range matchPos {
		newText = append(newText, seq[offs].Text...)
	}

	out := seq

	if newLen > oldLen {
		// In case the sequence got longer, move the tail out of the way first.
		out = append(out, make([]glyph.Info, newLen-oldLen)...)
		copy(out[newTailPos:], out[oldTailPos:])
	}

	// copy the ignored glyphs into position, just before the new tail
	removeListIdx := len(matchPos) - 1
	insertPos := newTailPos - 1
	for i := oldTailPos - 1; i >= pos; i-- {
		if removeListIdx >= 0 && matchPos[removeListIdx] == i {
			removeListIdx--
		} else {
			out[insertPos] = seq[i]
			insertPos--
		}
	}

	// copy the new glyphs into position
	if len(m.Replace) > 0 {
		copy(out[pos:], m.Replace)
		out[pos].Text = newText
	}

	if newLen < oldLen {
		// In case the sequence got shorter, move the tail to the new position now.
		copy(out[newTailPos:], out[oldTailPos:])
		out = out[:newLen]
	}
	return out
}
