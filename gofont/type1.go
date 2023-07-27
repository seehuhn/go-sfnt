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

package gofont

import (
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/psenc"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
)

func Type1(font FontID) (*type1.Font, error) {
	info, err := TrueType(font)
	if err != nil {
		return nil, err
	}
	info.EnsureGlyphNames()

	// convert glypf outlines to cff outlines
	// origOutlines := info.Outlines.(*glyf.Outlines)

	newGlyphs := make(map[string]*type1.Glyph)
	newInfo := make(map[string]*type1.GlyphInfo)
	origOutlines := info.Outlines.(*glyf.Outlines)

	for i, origGlyph := range origOutlines.Glyphs {
		gid := glyph.ID(i)
		name := info.GlyphName(gid)
		newGlyph := &type1.Glyph{}
		newGlyphI := &type1.GlyphInfo{ // TODO(voss)
			WidthX: info.GlyphWidth(gid),
		}

		if origGlyph == nil {
			goto done
		}

		switch g := origGlyph.Data.(type) {
		case glyf.SimpleGlyph:
			glyphInfo, err := g.Decode()
			if err != nil {
				panic(err)
			}

			for _, cc := range glyphInfo.Contours {
				var extended glyf.Contour
				var prev glyf.Point
				onCurve := true
				for _, cur := range cc {
					if !onCurve && !cur.OnCurve {
						extended = append(extended, glyf.Point{
							X:       (cur.X + prev.X) / 2,
							Y:       (cur.Y + prev.Y) / 2,
							OnCurve: true,
						})
					}
					extended = append(extended, cur)
					prev = cur
					onCurve = cur.OnCurve
				}
				n := len(extended)

				var offs int
				for i := 0; i < len(extended); i++ {
					if extended[i].OnCurve {
						offs = i
						break
					}
				}

				newGlyph.MoveTo(float64(extended[offs].X), float64(extended[offs].Y))

				i := 0
				for i < n {
					i0 := (i + offs) % n
					if !extended[i0].OnCurve {
						panic("not on curve")
					}
					i1 := (i0 + 1) % n
					if extended[i1].OnCurve {
						if i == n-1 {
							break
						}
						newGlyph.LineTo(float64(extended[i1].X), float64(extended[i1].Y))
						i++
					} else {
						// See the following link for converting truetype outlines
						// to CFF outlines:
						// https://pomax.github.io/bezierinfo/#reordering
						i2 := (i1 + 1) % n
						newGlyph.CurveTo(
							float64(extended[i0].X)/3+float64(extended[i1].X)*2/3,
							float64(extended[i0].Y)/3+float64(extended[i1].Y)*2/3,
							float64(extended[i1].X)*2/3+float64(extended[i2].X)/3,
							float64(extended[i1].Y)*2/3+float64(extended[i2].Y)/3,
							float64(extended[i2].X),
							float64(extended[i2].Y))
						i += 2
					}
				}

				newGlyph.ClosePath()
			}
		case glyf.CompositeGlyph:
			panic("not implemented")
		}

	done:
		newGlyphs[name] = newGlyph
		newInfo[name] = newGlyphI
	}

	encoding := make([]string, 256)
	for i := 0; i < 256; i++ {
		name := psenc.StandardEncoding[i]
		if _, ok := newGlyphs[name]; ok {
			encoding[i] = name
		} else {
			encoding[i] = ".notdef"
		}
	}

	var topMin, topMax funit.Int16
	var bottomMin, bottomMax funit.Int16
	for c := 'A'; c <= 'Z'; c++ {
		gid := info.CMap.Lookup(c)

		ext := info.GlyphExtent(gid)
		top := ext.URy
		if c == 'A' || top < topMin {
			topMin = top
		}
		if c == 'A' || top > topMax {
			topMax = top
		}

		if c == 'Q' {
			continue
		}
		bottom := ext.LLy
		if c == 'A' || bottom < bottomMin {
			bottomMin = bottom
		}
		if c == 'A' || bottom > bottomMax {
			bottomMax = bottom
		}
	}

	Private := &type1.PrivateDict{
		BlueValues: []funit.Int16{
			bottomMin, bottomMax, topMin, topMax,
		},
	}

	res := &type1.Font{
		CreationDate: info.CreationTime,
		UnitsPerEm:   info.UnitsPerEm,
		Info:         info.GetFontInfo(),
		Private:      Private,
		Outlines:     newGlyphs,
		GlyphInfo:    newInfo,
		Encoding:     encoding,
	}

	return res, nil
}