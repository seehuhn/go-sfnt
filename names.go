// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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
	"fmt"
	"strings"

	"seehuhn.de/go/postscript/type1/names"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/opentype/gtab"
)

// EnsureGlyphNames makes sure that all glyphs in the font have a name.
// If all names are present, the function does nothing.
// Otherwise, the function tries to infer the missing glyph names from
// the cmap table and gsub tables.
func (f *Font) EnsureGlyphNames() {
	glyphNames := f.MakeGlyphNames()

	switch f := f.Outlines.(type) {
	case *cff.Outlines:
		for gid, g := range f.Glyphs {
			if g == nil {
				continue
			}
			g.Name = glyphNames[gid]
		}
	case *glyf.Outlines:
		f.Names = glyphNames
	default:
		panic("unexpected font type")
	}
}

func (f *Font) MakeGlyphNames() []string {
	glyphNames := make([]string, f.NumGlyphs())
	switch f := f.Outlines.(type) {
	case *cff.Outlines:
		for gid, g := range f.Glyphs {
			if g == nil {
				continue
			}
			glyphNames[gid] = g.Name
		}
	case *glyf.Outlines:
		if len(f.Names) == len(f.Glyphs) {
			copy(glyphNames, f.Names)
		}
	default:
		panic("unexpected font type")
	}
	glyphNames[0] = ".notdef"

	used := make(map[string]bool)
	for i, name := range glyphNames {
		if used[name] {
			glyphNames[i] = ""
		} else {
			used[name] = true
		}
	}

	complete := true
	for _, name := range glyphNames {
		if name == "" {
			complete = false
			break
		}
	}
	if complete {
		return glyphNames
	}

	if cmap, _ := f.CMapTable.GetBest(); cmap != nil {
		a, b := cmap.CodeRange()
		for r := a; r <= b; r++ {
			gid := cmap.Lookup(r)
			if glyphNames[gid] != "" {
				// This includes the case of unmapped runes (gid == 0).
				continue
			}
			name := names.FromUnicode(r)
			if !used[name] {
				glyphNames[gid] = name
				used[name] = true
			}
		}
	}

	if f.Gsub != nil {
		for _, lookup := range f.Gsub.LookupList {
			for _, subtable := range lookup.Subtables {
				switch subtable := subtable.(type) {
				case *gtab.Gsub1_1:
					for origGid := range subtable.Cov {
						newGid := origGid + subtable.Delta
						if glyphNames[origGid] == "" || glyphNames[newGid] != "" {
							continue
						}
						glyphNames[newGid] = makeVariant(used, glyphNames[origGid])
					}
				case *gtab.Gsub1_2:
					for origGid, idx := range subtable.Cov {
						newGid := subtable.SubstituteGlyphIDs[idx]
						if glyphNames[origGid] == "" || glyphNames[newGid] != "" {
							continue
						}
						glyphNames[newGid] = makeVariant(used, glyphNames[origGid])
					}
				case *gtab.Gsub3_1:
					for origGid, idx := range subtable.Cov {
						if glyphNames[origGid] == "" {
							continue
						}
						for _, newGid := range subtable.Alternates[idx] {
							if glyphNames[newGid] == "" {
								glyphNames[newGid] = makeVariant(used, glyphNames[origGid])
							}
						}
					}
				case *gtab.Gsub4_1:
					var nn []string
					for origGid, idx := range subtable.Cov {
						name := glyphNames[origGid]
						if name == "" {
							continue
						}
						nn = append(nn[:0], name)
					replLoop:
						for _, lig := range subtable.Repl[idx] {
							nn = nn[:1]
							for _, gid := range lig.In {
								if name := glyphNames[gid]; name != "" {
									nn = append(nn, name)
								} else {
									continue replLoop
								}
							}
							newName := strings.Join(nn, "_")
							glyphNames[lig.Out] = makeVariant(used, newName)
						}
					}
				}
			}
		}
	}

	k := 1
	for i, name := range glyphNames {
		if name != "" {
			continue
		}
		for {
			name := fmt.Sprintf("orn%03d", k)
			k++
			if !used[name] {
				used[name] = true
				glyphNames[i] = name
				break
			}
		}
	}
	return glyphNames
}

func makeVariant(used map[string]bool, basename string) string {
	try := 0
	name := basename
	for used[name] {
		try++
		name = fmt.Sprintf("%s.%d", basename, try)
	}
	used[name] = true
	return name
}
