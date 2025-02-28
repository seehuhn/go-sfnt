// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2025  Jochen Voss <voss@seehuhn.de>
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

package cff

import (
	"fmt"
	"strings"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/type1/names"
	"seehuhn.de/go/sfnt/glyph"
)

// MakeSimple converts the font to a simple font.
//
// Changes include the following:
//   - Any private font matrices are discarded.
//   - Glyphnames are added, where required.
//   - The encoding is set to StandardEncoding.
func (o *Outlines) MakeSimple(glyphText map[glyph.ID]string) {
	o.ROS = nil
	o.GIDToCID = nil
	o.FontMatrices = nil
	o.makeNames(glyphText)
	o.Encoding = StandardEncoding(o.Glyphs)
}

// makeNames fills in missing CFF glyph names.
// If glyphText is not nil, it is used to generate names for glyphs that do not
// have a name yet.
func (o *Outlines) makeNames(glyphText map[glyph.ID]string) {
	o.Glyphs[0].Name = ".notdef"

	// keep existing names, unless they are invalid or duplicate
	glyphNameUsed := make(map[string]bool)
	for _, g := range o.Glyphs {
		glyphName := g.Name

		if !names.IsValid(glyphName) || glyphNameUsed[glyphName] {
			g.Name = ""
			continue
		}
		glyphNameUsed[glyphName] = true
	}

	// try to fill missing names based on text content
	if glyphText != nil {
		for gid, g := range o.Glyphs {
			if g.Name != "" {
				continue
			}

			glyphText := glyphText[glyph.ID(gid)]
			if glyphText == "" {
				continue
			}

			var parts []string
			for _, r := range glyphText {
				parts = append(parts, names.FromUnicode(r))
			}
			baseName := strings.Join(parts, "_")

			try := 0
			for {
				var suffix string
				if try > 0 {
					suffix = fmt.Sprintf(".alt%d", try)
				}
				try++

				glyphName := baseName + suffix
				if !names.IsValid(glyphName) {
					// If the name is too long now, it won't get any better.
					// Give up for now and rely on the generic names, below.
					break
				}
				if !glyphNameUsed[glyphName] {
					g.Name = glyphName
					glyphNameUsed[glyphName] = true
					break
				}
			}
		}
	}

	// allocate generic names for the remaining glyphs
	ornIdx := 1
	for _, g := range o.Glyphs {
		if g.Name != "" {
			continue
		}

		for {
			glyphName := fmt.Sprintf("orn%03d", ornIdx)
			ornIdx++

			if !glyphNameUsed[glyphName] {
				g.Name = glyphName
				glyphNameUsed[glyphName] = true
				break
			}
		}
	}
}

// MakeCIDKeyed converts the font to a CID-keyed font.
//
// Changes include the following:
//   - The encoding is discarded.
//   - The CID SystemInfo is replaced with the given ros.
func (o *Outlines) MakeCIDKeyed(ros *cid.SystemInfo, gidToCID []cid.CID) {
	// remove information onlly relevant for simple fonts
	for i, g := range o.Glyphs {
		if g.Name == "" {
			continue
		}

		g = clone(g)
		g.Name = ""
		o.Glyphs[i] = g
	}
	o.Encoding = nil

	o.ROS = ros
	o.GIDToCID = gidToCID

	// TODO(voss): I think it would be more normal to have the identity
	// matrix in the top dict, and the real font matrix here.
	for len(o.FontMatrices) < len(o.Private) {
		o.FontMatrices = append(o.FontMatrices, matrix.Identity)
	}
}

func clone[T any](x *T) *T {
	y := *x
	return &y
}
