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

package sfnt

import (
	"golang.org/x/text/language"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/gtab"
)

// A Layouter can turn a string into a sequence of glyphs.
type Layouter struct {
	font *Font
	cmap cmap.Subtable
	gsub *gtab.Context
	gpos *gtab.Context
	buf  []glyph.Info
}

// NewLayouter creates a new layouter for the given cmap and lookups.
func (f *Font) NewLayouter(lang language.Tag, gsubFeatures, gposFeatures map[string]bool) (*Layouter, error) {
	cmap, err := f.CMapTable.GetBest()
	if err != nil {
		return nil, err
	}

	var gsub, gpos *gtab.Context

	if f.Gsub != nil {
		if gsubFeatures == nil {
			gsubFeatures = gtab.GsubDefaultFeatures
		}
		gsubLookups := f.Gsub.FindLookups(lang, gsubFeatures)
		gsub = gtab.NewContext(f.Gsub.LookupList, f.Gdef, gsubLookups)
	}

	if f.Gpos != nil {
		if gposFeatures == nil {
			gposFeatures = gtab.GposDefaultFeatures
		}
		gposLookups := f.Gpos.FindLookups(lang, gposFeatures)
		gpos = gtab.NewContext(f.Gpos.LookupList, f.Gdef, gposLookups)
	}

	return &Layouter{
		font: f,
		cmap: cmap,
		gsub: gsub,
		gpos: gpos,
	}, nil
}

// Layout returns the glyph sequence for the given text.
//
// The returned slice is owned by the Layouter and is only valid until the next
// call to Layout.
func (l *Layouter) Layout(s string) []glyph.Info {
	seq := l.buf[:0]
	for _, r := range s {
		gid := l.cmap.Lookup(r)
		seq = append(seq, glyph.Info{
			GID:  gid,
			Text: []rune{r},
		})
	}

	if l.gsub != nil {
		seq = l.gsub.Apply(seq)
	}

	font := l.font
	for i := range seq {
		gid := seq[i].GID
		if !font.Gdef.IsMark(gid) {
			seq[i].Advance = funit.Int16(font.GlyphWidth(gid)) // TODO(voss)
		}
	}

	if l.gpos != nil {
		seq = l.gpos.Apply(seq)
	}

	l.buf = seq
	return seq
}
