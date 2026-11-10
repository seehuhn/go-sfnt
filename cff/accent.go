// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cff

import (
	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/psenc"
)

// resolveAccents replaces the glyphs listed in refs by the composition of
// their base and accent glyphs.  The map is indexed by glyph ID.
//
// The components are looked up by StandardEncoding code, and are taken from
// the glyphs as decoded, before any composition (nesting is not allowed).
// Glyphs where a component is missing, or where the composed outline cannot
// be written back, are left unchanged.  The composed glyph keeps its own
// stems if it has any, and otherwise takes the stems and masks of the base;
// the accent's stems and masks, and any outline the composite draws itself,
// are dropped.  The size of the composed outlines is charged to the budget.
func resolveAccents(glyphs []*Glyph, refs map[int]*accentRef, budget *membudget.Budget) error {
	if len(refs) == 0 {
		return nil
	}

	byName := make(map[string]int, len(glyphs))
	for gid, g := range glyphs {
		byName[g.Name] = gid // the last glyph with a name wins
	}

	composed := make(map[int]*Glyph, len(refs))
	for gid, ref := range refs {
		baseName := psenc.StandardEncoding[ref.base]
		accentName := psenc.StandardEncoding[ref.accent]
		if baseName == ".notdef" || accentName == ".notdef" {
			continue
		}
		baseID, ok := byName[baseName]
		if !ok {
			continue
		}
		accentID, ok := byName[accentName]
		if !ok {
			continue
		}
		g, base, accent := glyphs[gid], glyphs[baseID], glyphs[accentID]

		useBaseStems := len(g.HStem) == 0 && len(g.VStem) == 0

		// the mask length would only match the stems they were written for
		keepBase := func(op GlyphOpType) bool { return useBaseStems || !isMaskOp(op) }
		keepAccent := func(op GlyphOpType) bool { return !isMaskOp(op) }

		nCmds, nArgs := 0, 0
		for _, cmd := range base.Cmds {
			if keepBase(cmd.Op) {
				nCmds++
				nArgs += len(cmd.Args)
			}
		}
		for _, cmd := range accent.Cmds {
			if keepAccent(cmd.Op) {
				nCmds++
				nArgs += len(cmd.Args)
			}
		}
		if useBaseStems {
			nArgs += len(base.HStem) + len(base.VStem)
		}

		cmds, err := budget.AllocSlice[GlyphOp](nCmds)
		if err != nil {
			return err
		}
		// all numbers of the composed glyph share one backing array
		pool, err := budget.AllocSlice[float64](nArgs)
		if err != nil {
			return err
		}
		take := func(src []float64) []float64 {
			if src == nil {
				return nil
			}
			dst := pool[:len(src):len(src)]
			pool = pool[len(src):]
			copy(dst, src)
			return dst
		}

		i := 0
		for _, cmd := range base.Cmds {
			if keepBase(cmd.Op) {
				cmds[i] = GlyphOp{Op: cmd.Op, Args: take(cmd.Args)}
				i++
			}
		}
		for _, cmd := range accent.Cmds {
			if keepAccent(cmd.Op) {
				args := take(cmd.Args)
				for j := 0; j+1 < len(args); j += 2 {
					args[j] += ref.dx
					args[j+1] += ref.dy
				}
				cmds[i] = GlyphOp{Op: cmd.Op, Args: args}
				i++
			}
		}

		if _, err := encodeArgs(cmds); err != nil {
			continue
		}

		res := *g
		res.Cmds = cmds
		if useBaseStems {
			res.HStem = take(base.HStem)
			res.VStem = take(base.VStem)
		}
		composed[gid] = &res
	}

	for gid, g := range composed {
		glyphs[gid] = g
	}
	return nil
}

func isMaskOp(op GlyphOpType) bool {
	return op == OpHintMask || op == OpCntrMask
}
