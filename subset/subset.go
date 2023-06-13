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

package subset

import (
	"errors"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/type1"
)

type Glyph struct {
	OrigGID glyph.ID
	CID     type1.CID
}

func Simple(info *sfnt.Info, subset []Glyph) (*sfnt.Info, error) {
	if len(subset) == 0 || subset[0].OrigGID != 0 {
		return nil, errors.New("subset does not start with .notdef")
	}

	res := &sfnt.Info{}
	*res = *info

	switch outlines := info.Outlines.(type) {
	case *cff.Outlines:
		o2 := &cff.Outlines{}
		o2.Encoding = make([]glyph.ID, 256)
		pIdx := -1
		for i, g := range subset {
			o2.Glyphs = append(o2.Glyphs, outlines.Glyphs[g.OrigGID])
			oldPIdx := outlines.FdSelect(g.OrigGID)
			if pIdx == -1 {
				pIdx = oldPIdx
			} else if pIdx != oldPIdx {
				return nil, errors.New("subset contains glyphs from multiple private dicts")
			}

			if g.CID >= 256 {
				return nil, errors.New("subset contains CID > 255")
			}
			o2.Encoding[g.CID] = glyph.ID(i)
		}
		// TODO(voss): do we need to error out if there are no glyph names?
		o2.Private = []*type1.PrivateDict{outlines.Private[pIdx]}
		o2.FdSelect = func(gid glyph.ID) int { return 0 }
		res.Outlines = o2

	case *glyf.Outlines:
		newGid := make(map[glyph.ID]glyph.ID)
		todo := make(map[glyph.ID]bool)
		nextGid := glyph.ID(0)
		for _, g := range subset {
			gid := g.OrigGID
			newGid[gid] = nextGid
			nextGid++

			for _, gid2 := range outlines.Glyphs[gid].Components() {
				if _, ok := newGid[gid2]; !ok {
					todo[gid2] = true
				}
			}
		}
		for len(todo) > 0 {
			gid := pop(todo)
			subset = append(subset, Glyph{OrigGID: gid})
			newGid[gid] = nextGid
			nextGid++

			for _, gid2 := range outlines.Glyphs[gid].Components() {
				if _, ok := newGid[gid2]; !ok {
					todo[gid2] = true
				}
			}
		}

		o2 := &glyf.Outlines{
			Tables: outlines.Tables,
			Maxp:   outlines.Maxp,
		}
		for _, g := range subset {
			gid := g.OrigGID
			newGlyph := outlines.Glyphs[gid]
			o2.Glyphs = append(o2.Glyphs, newGlyph.FixComponents(newGid))
			o2.Widths = append(o2.Widths, outlines.Widths[gid])
			// o2.Names = append(o2.Names, outlines.Names[gid])
		}
		res.Outlines = o2

		// Use a format 4 TrueType cmap to specify the mapping from
		// character codes to glyph indices.
		//
		// TODO(voss): what to do if there is a CID > 65535?
		// Use a different cmap format?  Return an error?
		encoding := cmap.Format4{}
		for subsetGid, g := range subset {
			encoding[uint16(g.CID)] = glyph.ID(subsetGid)
		}
		res.CMap = encoding

	default:
		panic("unexpected font type")
	}

	return res, nil
}

func pop(todo map[glyph.ID]bool) glyph.ID {
	for key := range todo {
		delete(todo, key)
		return key
	}
	panic("empty map")
}
