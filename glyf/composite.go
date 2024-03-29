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

package glyf

import (
	"fmt"
	"strings"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// CompositeGlyph is a composite glyph.
type CompositeGlyph struct {
	Components   []GlyphComponent
	Instructions []byte
}

// GlyphComponent is a single component of a composite glyph.
//
// https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#composite-glyph-description
type GlyphComponent struct {
	Flags      ComponentFlag
	GlyphIndex glyph.ID
	Data       []byte
}

type ComponentFlag uint16

// The recognised values for the ComponentFlag field.
//
// https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#compositeGlyphFlags
const (
	FlagArg1And2AreWords        ComponentFlag = 0x0001
	FlagArgsAreXYValues         ComponentFlag = 0x0002
	FlagRoundXYToGrid           ComponentFlag = 0x0004
	FlagWeHaveAScale            ComponentFlag = 0x0008
	FlagMoreComponents          ComponentFlag = 0x0020
	FlagWeHaveAnXAndYScale      ComponentFlag = 0x0040
	FlagWeHaveATwoByTwo         ComponentFlag = 0x0080
	FlagWeHaveInstructions      ComponentFlag = 0x0100
	FlagUseMyMetrics            ComponentFlag = 0x0200
	FlagOverlapCompound         ComponentFlag = 0x0400
	FlagScaledComponentOffset   ComponentFlag = 0x0800
	FlagUnscaledComponentOffset ComponentFlag = 0x1000
)

func (f ComponentFlag) String() string {
	var res []string
	if f&FlagArg1And2AreWords != 0 {
		res = append(res, "ARG_1_AND_2_ARE_WORDS")
	}
	if f&FlagArgsAreXYValues != 0 {
		res = append(res, "ARGS_ARE_XY_VALUES")
	}
	if f&FlagRoundXYToGrid != 0 {
		res = append(res, "ROUND_XY_TO_GRID")
	}
	if f&FlagWeHaveAScale != 0 {
		res = append(res, "WE_HAVE_A_SCALE")
	}
	if f&FlagMoreComponents != 0 {
		res = append(res, "MORE_COMPONENTS")
	}
	if f&FlagWeHaveAnXAndYScale != 0 {
		res = append(res, "WE_HAVE_AN_X_AND_Y_SCALE")
	}
	if f&FlagWeHaveATwoByTwo != 0 {
		res = append(res, "WE_HAVE_A_TWO_BY_TWO")
	}
	if f&FlagWeHaveInstructions != 0 {
		res = append(res, "WE_HAVE_INSTRUCTIONS")
	}
	if f&FlagUseMyMetrics != 0 {
		res = append(res, "USE_MY_METRICS")
	}
	if f&FlagOverlapCompound != 0 {
		res = append(res, "OVERLAP_COMPOUND")
	}
	if f&FlagScaledComponentOffset != 0 {
		res = append(res, "SCALED_COMPONENT_OFFSET")
	}
	if f&FlagUnscaledComponentOffset != 0 {
		res = append(res, "UNSCALED_COMPONENT_OFFSET")
	}
	if f&0xE010 != 0 {
		res = append(res, fmt.Sprintf("0x%04x", f&0xE010))
	}
	return strings.Join(res, "|")
}

// Note that decodeGlyph retains sub-slices of data.
func decodeGlyph(data []byte) (*Glyph, error) {
	if len(data) == 0 {
		return nil, nil
	} else if len(data) < 10 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/glyf",
			Reason:    "incomplete glyph header",
		}
	}

	var glyphData interface{}
	numCont := int16(data[0])<<8 | int16(data[1])
	if numCont >= 0 {
		simple := SimpleGlyph{
			NumContours: numCont,
			Encoded:     data[10:],
		}
		err := simple.removePadding()
		if err != nil {
			return nil, err
		}
		glyphData = simple
	} else {
		comp, err := decodeGlyphComposite(data[10:])
		if err != nil {
			return nil, err
		}
		glyphData = *comp
	}

	g := &Glyph{
		Rect16: funit.Rect16{
			LLx: funit.Int16(data[2])<<8 | funit.Int16(data[3]),
			LLy: funit.Int16(data[4])<<8 | funit.Int16(data[5]),
			URx: funit.Int16(data[6])<<8 | funit.Int16(data[7]),
			URy: funit.Int16(data[8])<<8 | funit.Int16(data[9]),
		},
		Data: glyphData,
	}
	return g, nil
}

func decodeGlyphComposite(data []byte) (*CompositeGlyph, error) {
	var components []GlyphComponent
	done := false
	weHaveInstructions := false
	for !done {
		if len(data) < 4 {
			return nil, errIncompleteGlyph
		}

		flags := ComponentFlag(data[0])<<8 | ComponentFlag(data[1])
		glyphIndex := uint16(data[2])<<8 | uint16(data[3])
		data = data[4:]

		if flags&FlagWeHaveInstructions != 0 {
			weHaveInstructions = true
		}

		skip := 0
		if flags&FlagArg1And2AreWords != 0 {
			skip += 4
		} else {
			skip += 2
		}
		if flags&FlagWeHaveAScale != 0 {
			skip += 2
		} else if flags&FlagWeHaveAnXAndYScale != 0 {
			skip += 4
		} else if flags&FlagWeHaveATwoByTwo != 0 {
			skip += 8
		}
		if len(data) < skip {
			return nil, errIncompleteGlyph
		}
		args := data[:skip]
		data = data[skip:]

		components = append(components, GlyphComponent{
			Flags:      flags,
			GlyphIndex: glyph.ID(glyphIndex),
			Data:       args,
		})

		done = flags&FlagMoreComponents == 0
	}

	if weHaveInstructions && len(data) >= 2 {
		L := int(data[0])<<8 | int(data[1])
		data = data[2:]
		if len(data) > L {
			data = data[:L]
		}
	} else {
		data = nil
	}

	res := &CompositeGlyph{
		Components:   components,
		Instructions: data,
	}
	return res, nil
}

func (g *Glyph) encodeLen() int {
	if g == nil {
		return 0
	}

	total := 10
	switch d := g.Data.(type) {
	case SimpleGlyph:
		total += len(d.Encoded)
	case CompositeGlyph:
		for _, comp := range d.Components {
			total += 4 + len(comp.Data)
		}
		if d.Instructions != nil {
			total += 2 + len(d.Instructions)
		}
	default:
		panic("unexpected glyph type")
	}
	for total%glyfAlign != 0 {
		total++
	}
	return total
}

func (g *Glyph) append(buf []byte) []byte {
	if g == nil {
		return buf
	}

	var numContours int16
	switch g0 := g.Data.(type) {
	case SimpleGlyph:
		numContours = g0.NumContours
	case CompositeGlyph:
		numContours = -1
	default:
		panic("unexpected glyph type")
	}

	buf = append(buf,
		byte(numContours>>8),
		byte(numContours),
		byte(g.LLx>>8),
		byte(g.LLx),
		byte(g.LLy>>8),
		byte(g.LLy),
		byte(g.URx>>8),
		byte(g.URx),
		byte(g.URy>>8),
		byte(g.URy))

	switch d := g.Data.(type) {
	case SimpleGlyph:
		buf = append(buf, d.Encoded...)
	case CompositeGlyph:
		for _, comp := range d.Components {
			buf = append(buf,
				byte(comp.Flags>>8), byte(comp.Flags),
				byte(comp.GlyphIndex>>8), byte(comp.GlyphIndex))
			buf = append(buf, comp.Data...)
		}
		if d.Instructions != nil {
			L := len(d.Instructions)
			buf = append(buf, byte(L>>8), byte(L))
			buf = append(buf, d.Instructions...)
		}
	default:
		panic("unexpected glyph type")
	}

	for len(buf)%glyfAlign != 0 {
		buf = append(buf, 0)
	}

	return buf
}

// Components returns the components of a composite glyph, or nil if the glyph
// is simple.
func (g *Glyph) Components() []glyph.ID {
	if g == nil {
		return nil
	}
	switch d := g.Data.(type) {
	case SimpleGlyph:
		return nil
	case CompositeGlyph:
		res := make([]glyph.ID, len(d.Components))
		for i, comp := range d.Components {
			res[i] = comp.GlyphIndex
		}
		return res
	default:
		panic("unexpected glyph type")
	}
}

// FixComponents changes the glyph component IDs of a composite glyph.
func (g *Glyph) FixComponents(newGid map[glyph.ID]glyph.ID) *Glyph {
	if g == nil {
		return nil
	}
	switch d := g.Data.(type) {
	case SimpleGlyph:
		return g
	case CompositeGlyph:
		d2 := CompositeGlyph{
			Components:   make([]GlyphComponent, len(d.Components)),
			Instructions: d.Instructions,
		}
		for i, c := range d.Components {
			d2.Components[i] = GlyphComponent{
				Flags:      c.Flags,
				GlyphIndex: newGid[c.GlyphIndex],
				Data:       c.Data,
			}
		}
		g2 := &Glyph{
			Rect16: g.Rect16,
			Data:   d2,
		}
		return g2
	default:
		panic("unexpected glyph type")
	}
}

const glyfAlign = 2

var errIncompleteGlyph = &parser.InvalidFontError{
	SubSystem: "sfnt/glyf",
	Reason:    "incomplete glyph",
}
