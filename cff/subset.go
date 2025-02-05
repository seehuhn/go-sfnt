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
	"seehuhn.de/go/postscript/cid"

	"seehuhn.de/go/sfnt/glyph"
)

// Subset returns a subset of the font.
//
// The new subset contains only the given glyphs, in the order they are
// specified in the glyphs slice.  The glyphs slice must start with GID 0.
//
// The returned font shares the private dictionaries and glyph data with the
// original font.
func (o *Outlines) Subset(glyphs []glyph.ID) *Outlines {
	subset := &Outlines{}

	// transfer the glyphs
	subset.Glyphs = make([]*Glyph, len(glyphs))
	for newGID, oldGID := range glyphs {
		subset.Glyphs[newGID] = o.Glyphs[oldGID]
	}

	// transfer the private dictionaries and create a new FDSelect function
	pIdxMap := make(map[int]int)
	for _, oldGID := range glyphs {
		oldPIdx := o.FDSelect(oldGID)
		if _, ok := pIdxMap[oldPIdx]; !ok {
			newPIdx := len(subset.Private)
			subset.Private = append(subset.Private, o.Private[oldPIdx])
			if o.IsCIDKeyed() {
				subset.FontMatrices = append(subset.FontMatrices, o.FontMatrices[oldPIdx])
			}
			pIdxMap[oldPIdx] = newPIdx
		}
	}
	if len(subset.Private) == 1 {
		subset.FDSelect = fdSelectSimple
	} else {
		fdSel := make([]int, len(glyphs))
		for newGID, oldGID := range glyphs {
			fdSel[newGID] = pIdxMap[o.FDSelect(oldGID)]
		}
		subset.FDSelect = func(gid glyph.ID) int { return fdSel[gid] }
	}

	// transfer the encoding, where applicable
	if o.Encoding != nil {
		gidMap := make(map[glyph.ID]glyph.ID)
		for newGID, oldGID := range glyphs {
			gidMap[oldGID] = glyph.ID(newGID)
		}
		subset.Encoding = make([]glyph.ID, len(o.Encoding))
		for i, oldGID := range o.Encoding {
			if newGID, ok := gidMap[oldGID]; ok {
				subset.Encoding[i] = newGID
			}
		}
	}

	subset.ROS = o.ROS

	// transfer the GID to CID mapping, where applicable
	if o.GIDToCID != nil {
		subset.GIDToCID = make([]cid.CID, len(glyphs))
		for newGid, oldGid := range glyphs {
			subset.GIDToCID[newGid] = o.GIDToCID[oldGid]
		}
	}

	return subset
}
