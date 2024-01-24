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
	"errors"
	"fmt"

	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
)

// Subset returns a subset of the font containing containing the given
// glyphs at the first positions.  More glyphs may be included in the
// subset, if they occur as ligatures between the given glyphs.
func (f *Font) Subset(glyphs []glyph.ID) (*Font, error) {
	if glyphs[0] != 0 {
		return nil, errors.New("sfnt: subset must start with .notdef glyph")
	}

	res := f.Clone()

	s := subsetter{
		glyphs: glyphs,
		newGid: map[glyph.ID]glyph.ID{},
	}
	for newgid, oldGid := range glyphs {
		s.newGid[oldGid] = glyph.ID(newgid)
	}

	if f.CMapTable != nil {
		res.CMapTable = make(cmap.Table, len(f.CMapTable))
		for key := range f.CMapTable {
			c, err := res.CMapTable.Get(key)
			if err != nil {
				continue
			}
			c = s.SubsetCMap(c)
			res.CMapTable[key] = c.Encode(key.Language)
		}
	}
	res.Gsub = s.SubsetGsub(f.Gsub)
	// At this point we have the final list of glyphs.
	res.Gpos = s.SubsetGpos(f.Gpos)
	res.Gdef = s.SubsetGdef(f.Gdef)

	switch outlines := f.Outlines.(type) {
	case *cff.Outlines:
		res.Outlines = s.SubsetCFF(outlines)
	case *glyf.Outlines:
		res.Outlines = s.SubsetGlyf(outlines)
	}

	return res, nil
}

type subsetter struct {
	glyphs []glyph.ID
	newGid map[glyph.ID]glyph.ID
}

func (s *subsetter) getNewGid(oldGid glyph.ID) glyph.ID {
	newGid, ok := s.newGid[oldGid]
	if !ok {
		newGid = glyph.ID(len(s.glyphs))
		s.glyphs = append(s.glyphs, oldGid)
		s.newGid[oldGid] = newGid
	}
	return newGid
}

func (s *subsetter) SubsetCMap(c cmap.Subtable) cmap.Subtable {
	if c == nil {
		return nil
	}

	switch c := c.(type) {
	case cmap.Format4:
		res := cmap.Format4{}
		for key, oldGid := range c {
			newGid, ok := s.newGid[oldGid]
			if !ok {
				continue
			}
			res[key] = newGid
		}
		return res
	case cmap.Format12:
		res := cmap.Format12{}
		for key, oldGid := range c {
			newGid, ok := s.newGid[oldGid]
			if !ok {
				continue
			}
			res[key] = newGid
		}
		return res
	default:
		panic(fmt.Sprintf("sfnt: unsupported cmap format %T", c))
	}
}

func (s *subsetter) SubsetGsub(old *gtab.Info) *gtab.Info {
	if old == nil {
		return nil
	}

	res := *old
	res.LookupList = make(gtab.LookupList, len(old.LookupList))
	// TODO(voss): if glyphs were added, we need to re-run this loop
	for i, tOld := range old.LookupList {
		tNew := &gtab.LookupTable{
			Meta:      tOld.Meta,
			Subtables: make(gtab.Subtables, len(tOld.Subtables)),
		}
		for j, sOld := range tOld.Subtables {
			switch sOld := sOld.(type) {
			case *gtab.Gsub1_1:
				// It is difficult to produce a format 1 subtable for the subset
				// (constant offset between original and subtitute), so we
				// convert this to format 2 instead.
				sNew := &gtab.Gsub1_2{
					Cov: make(map[glyph.ID]int),
				}
				for oldOrig := range sOld.Cov {
					newOrig, ok := s.newGid[oldOrig]
					if !ok {
						continue
					}
					newSubst := oldOrig + sOld.Delta
					// TODO(voss): keep the coverage table sorted
					sNew.Cov[newOrig] = len(sNew.SubstituteGlyphIDs)
					sNew.SubstituteGlyphIDs = append(sNew.SubstituteGlyphIDs, s.getNewGid(newSubst))
				}
				tNew.Subtables[j] = sNew
			case *gtab.Gsub1_2:
				panic("not implemented")
			case *gtab.Gsub2_1:
				panic("not implemented")
			case *gtab.Gsub3_1:
				panic("not implemented")
			case *gtab.Gsub4_1:
				panic("not implemented")
			case *gtab.Gsub8_1:
				panic("not implemented")
			case *gtab.SeqContext1:
				panic("not implemented")
			case *gtab.SeqContext2:
				panic("not implemented")
			case *gtab.SeqContext3:
				panic("not implemented")
			case *gtab.ChainedSeqContext1:
				panic("not implemented")
			case *gtab.ChainedSeqContext2:
				panic("not implemented")
			case *gtab.ChainedSeqContext3:
				panic("not implemented")
			default:
				panic(fmt.Sprintf("sfnt: unsupported GSUB format %T", sOld))
			}
		}
		res.LookupList[i] = tNew
	}

	return &res
}

func (s *subsetter) SubsetGpos(old *gtab.Info) *gtab.Info {
	if old == nil {
		return nil
	}

	res := *old
	res.LookupList = make(gtab.LookupList, len(old.LookupList))
	for i, tOld := range old.LookupList {
		tNew := &gtab.LookupTable{
			Meta:      tOld.Meta,
			Subtables: make(gtab.Subtables, len(tOld.Subtables)),
		}
		for j, sOld := range tOld.Subtables {
			switch sOld := sOld.(type) {
			case *gtab.Gpos1_1:
			case *gtab.Gpos1_2:
			case gtab.Gpos2_1:
				sNew := gtab.Gpos2_1{}
				for pair, adj := range sOld {
					if _, ok := s.newGid[pair.Left]; !ok {
						continue
					}
					if _, ok := s.newGid[pair.Right]; !ok {
						continue
					}
					sNew[pair] = adj
				}
				tNew.Subtables[j] = sNew
			case *gtab.Gpos2_2:
				panic("not implemented")
			case *gtab.Gpos3_1:
				panic("not implemented")
			case *gtab.Gpos4_1:
				panic("not implemented")
			case *gtab.Gpos6_1:
				panic("not implemented")
			case *gtab.SeqContext1:
				panic("not implemented")
			case *gtab.SeqContext2:
				panic("not implemented")
			case *gtab.SeqContext3:
				panic("not implemented")
			case *gtab.ChainedSeqContext1:
				panic("not implemented")
			case *gtab.ChainedSeqContext2:
				panic("not implemented")
			case *gtab.ChainedSeqContext3:
				panic("not implemented")
			default:
				panic(fmt.Sprintf("sfnt: unsupported GPOS format %T", tOld))
			}
		}
		res.LookupList[i] = tNew
	}

	return &res
}

func (s *subsetter) SubsetGdef(old *gdef.Table) *gdef.Table {
	if old == nil {
		return nil
	}
	panic("not implemented")
}

func (s *subsetter) SubsetCFF(oldOutlines *cff.Outlines) *cff.Outlines {
	newOutlines := &cff.Outlines{}

	newOutlines.Glyphs = make([]*cff.Glyph, len(s.glyphs))
	for i, oldGid := range s.glyphs {
		newOutlines.Glyphs[i] = oldOutlines.Glyphs[oldGid]
	}

	pIdxMap := make(map[int]int)
	for _, oldGid := range s.glyphs {
		oldPIdx := oldOutlines.FDSelect(oldGid)
		if _, ok := pIdxMap[oldPIdx]; !ok {
			newPIdx := len(newOutlines.Private)
			newOutlines.Private = append(newOutlines.Private, oldOutlines.Private[oldPIdx])
			pIdxMap[oldPIdx] = newPIdx
		}
	}
	if len(newOutlines.Private) == 1 {
		newOutlines.FDSelect = func(glyph.ID) int { return 0 }
	} else {
		fdSel := make([]int, len(s.glyphs))
		for newgid, oldGid := range s.glyphs {
			fdSel[newgid] = pIdxMap[oldOutlines.FDSelect(oldGid)]
		}
		newOutlines.FDSelect = func(gid glyph.ID) int { return fdSel[gid] }
	}

	if oldOutlines.Encoding != nil {
		newOutlines.Encoding = make([]glyph.ID, len(oldOutlines.Encoding))
		for i, oldGid := range oldOutlines.Encoding {
			if newGid, ok := s.newGid[oldGid]; ok {
				newOutlines.Encoding[i] = newGid
			}
		}
	}

	newOutlines.ROS = oldOutlines.ROS

	if oldOutlines.GIDToCID != nil {
		newOutlines.GIDToCID = make([]cid.CID, len(s.glyphs))
		for newGid, oldGid := range s.glyphs {
			newOutlines.GIDToCID[newGid] = oldOutlines.GIDToCID[oldGid]
		}
	}

	return newOutlines
}

func (s *subsetter) SubsetGlyf(oldOutlines *glyf.Outlines) *glyf.Outlines {
	newOutlines := &glyf.Outlines{
		Tables: oldOutlines.Tables,
		Maxp:   oldOutlines.Maxp,
	}

	todo := make(map[glyph.ID]bool, len(s.glyphs))
	for _, oldGid := range s.glyphs {
		todo[oldGid] = true
	}
	for len(todo) > 0 {
		oldGid := pop(todo)
		cc := oldOutlines.Glyphs[oldGid].Components()
		for _, componentGidOld := range cc {
			if _, ok := s.newGid[componentGidOld]; ok {
				continue
			}
			componendGidNew := glyph.ID(len(s.glyphs))
			s.glyphs = append(s.glyphs, componentGidOld)
			s.newGid[oldGid] = componendGidNew
			todo[componentGidOld] = true
		}
	}

	newOutlines.Glyphs = make([]*glyf.Glyph, len(s.glyphs))
	for newGid, oldGid := range s.glyphs {
		newOutlines.Glyphs[newGid] = oldOutlines.Glyphs[oldGid].FixComponents(s.newGid)
	}

	newOutlines.Widths = make([]funit.Int16, len(s.glyphs))
	for newGid, oldGid := range s.glyphs {
		newOutlines.Widths[newGid] = oldOutlines.Widths[oldGid]
	}

	if oldOutlines.Names != nil {
		newOutlines.Names = make([]string, len(s.glyphs))
		for newGid, oldGid := range s.glyphs {
			newOutlines.Names[newGid] = oldOutlines.Names[oldGid]
		}
	}

	// TODO(voss): can anything be done to make the "fpgm" table smaller?

	return newOutlines
}

func pop(todo map[glyph.ID]bool) glyph.ID {
	for key := range todo {
		delete(todo, key)
		return key
	}
	panic("empty map")
}
