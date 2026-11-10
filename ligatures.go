// Copyright 2024 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"maps"
	"slices"

	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
)

func standardLigatures(cmap cmap.Subtable) *gtab.Info {
	// `All` lists the ligatures characters, followed by the sequence of glyphs
	// that should be replaced by the ligature.  Longer sequences are listed
	// first, so that the longest match is found first.
	all := []string{"ﬃffi", "ﬄffl", "ﬀff", "ﬁfi", "ﬂfl"}

	// ligatures grouped by the first letter of the split glyph sequence
	ll := map[glyph.ID][]gtab.Ligature{}

	var gg []glyph.ID
ligLoop:
	for _, lig := range all {
		gg = gg[:0]
		for _, r := range lig {
			gid := cmap.Lookup(r)
			if gid == 0 {
				continue ligLoop
			}
			gg = append(gg, gid)
		}

		ll[gg[1]] = append(ll[gg[1]], gtab.Ligature{
			In:  slices.Clone(gg[2:]),
			Out: gg[0],
		})
	}

	if len(ll) == 0 {
		return nil
	}

	// TODO(voss): merge this with the code in go-sfnt/examples/type1-to-otf/main.go

	keys := slices.Sorted(maps.Keys(ll))

	cov := coverage.Table{}
	var repl [][]gtab.Ligature
	for i, gid := range keys {
		cov[gid] = i
		repl = append(repl, ll[gid])
	}
	subst := &gtab.Gsub4_1{
		Cov:  cov,
		Repl: repl,
	}
	gsub := &gtab.Info{
		ScriptList: map[language.Tag]*gtab.Features{
			language.MustParse("und-Latn-x-latn"): {Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: []*gtab.Feature{
			{Tag: "liga", Lookups: []gtab.LookupIndex{0}},
		},
		LookupList: []*gtab.LookupTable{
			{
				Meta:      &gtab.LookupMetaInfo{LookupType: 4},
				Subtables: []gtab.Subtable{subst},
			},
		},
	}
	return gsub
}
