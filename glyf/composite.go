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
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// CompositeGlyph represents a glyph that is built from multiple component glyphs.
// Unlike simple glyphs which contain their own outline data, composite glyphs
// reference other glyphs and specify how to transform and position them.
type CompositeGlyph struct {
	Components   []GlyphComponent // The component glyphs that make up this composite
	Instructions []byte           // TrueType instructions for the composite glyph
}

// GlyphComponent represents a single component of a composite glyph.
// Each component references another glyph by ID and contains transformation
// data in its Data field that specifies how to position and transform the
// referenced glyph when rendering the composite.
//
// https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#composite-glyph-description
type GlyphComponent struct {
	Flags      ComponentFlag // Flags controlling how the component is processed
	GlyphIndex glyph.ID      // ID of the glyph to include as a component
	Data       []byte        // Raw transformation data (arguments and matrix values)
}

// ComponentFlag controls how a component glyph is processed within a composite.
// These flags determine the format of transformation data and how components
// are combined.
type ComponentFlag uint16

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

// The recognized values for the ComponentFlag field.
//
// https://learn.microsoft.com/en-us/typography/opentype/spec/glyf#compositeGlyphFlags
const (
	FlagArg1And2AreWords        ComponentFlag = 0x0001 // Arguments are 16-bit signed values
	FlagArgsAreXYValues         ComponentFlag = 0x0002 // Arguments are x,y offsets rather than point numbers
	FlagRoundXYToGrid           ComponentFlag = 0x0004 // Round offset values to grid
	FlagWeHaveAScale            ComponentFlag = 0x0008 // Component has uniform scaling
	FlagMoreComponents          ComponentFlag = 0x0020 // More components follow this one
	FlagWeHaveAnXAndYScale      ComponentFlag = 0x0040 // Component has separate x and y scaling
	FlagWeHaveATwoByTwo         ComponentFlag = 0x0080 // Component has full 2x2 transformation matrix
	FlagWeHaveInstructions      ComponentFlag = 0x0100 // Composite glyph has instructions
	FlagUseMyMetrics            ComponentFlag = 0x0200 // Use this component's metrics for the composite
	FlagOverlapCompound         ComponentFlag = 0x0400 // Components overlap (used by some rasterizers)
	FlagScaledComponentOffset   ComponentFlag = 0x0800 // Apply scaling to offset values
	FlagUnscaledComponentOffset ComponentFlag = 0x1000 // Do not apply scaling to offset values
)

// f2dot14Factor is the scaling factor for F2.14 fixed-point numbers.
const f2dot14Factor = 1 << 14 // 16384

// floatToF2dot14 converts a float64 to a 16-bit F2.14 fixed-point number.
// Values are clamped to the valid range of int16.
func floatToF2dot14(f float64) int16 {
	val := f * f2dot14Factor
	if val > 32767.0 { // Max int16
		return 32767
	}
	if val < -32768.0 { // Min int16
		return -32768
	}
	return int16(math.Round(val))
}

// f2dot14ToFloat converts a 16-bit F2.14 fixed-point number back to float64.
func f2dot14ToFloat(i int16) float64 {
	return float64(i) / f2dot14Factor
}

// decodeGlyphComposite decodes a composite glyph from binary data.
// It parses the component descriptions and optional instructions
// according to the TrueType glyf table format.
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

// Components returns the component glyph IDs of a composite glyph.
// Returns nil if the glyph is simple.
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
// For simple glyphs, returns the glyph unchanged. For composite glyphs,
// returns a new glyph with component IDs mapped according to newGid.
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

// Unpack extracts the component data into a more accessible format.
// When FlagArgsAreXYValues is not set, arg1 and arg2 represent
// point indices for point matching, not offsets. In this case, the
// actual offset values depend on the referenced glyphs' point data
// and cannot be determined without additional context.
func (gc GlyphComponent) Unpack() (*ComponentUnpacked, error) {
	res := &ComponentUnpacked{
		Child:           gc.GlyphIndex,
		RoundXYToGrid:   gc.Flags&FlagRoundXYToGrid != 0,
		UseMyMetrics:    gc.Flags&FlagUseMyMetrics != 0,
		OverlapCompound: gc.Flags&FlagOverlapCompound != 0,
	}

	// Determine ScaledComponentOffset behavior
	if gc.Flags&FlagScaledComponentOffset != 0 {
		res.ScaledComponentOffset = true
	} else if gc.Flags&FlagUnscaledComponentOffset != 0 {
		res.ScaledComponentOffset = false
	} else {
		// When neither flag is set, the behavior is implementation-dependent.
		// We default to unscaled for compatibility.
		res.ScaledComponentOffset = false
	}

	r := bytes.NewReader(gc.Data)

	var arg1, arg2 int16
	if gc.Flags&FlagArg1And2AreWords != 0 {
		if err := binary.Read(r, binary.BigEndian, &arg1); err != nil {
			return nil, fmt.Errorf("reading arg1 (word): %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &arg2); err != nil {
			return nil, fmt.Errorf("reading arg2 (word): %w", err)
		}
	} else {
		var bArg1, bArg2 int8
		if err := binary.Read(r, binary.BigEndian, &bArg1); err != nil {
			return nil, fmt.Errorf("reading arg1 (byte): %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &bArg2); err != nil {
			return nil, fmt.Errorf("reading arg2 (byte): %w", err)
		}
		arg1 = int16(bArg1)
		arg2 = int16(bArg2)
	}

	// Default to identity transform
	res.Trfm = matrix.Matrix{1, 0, 0, 1, 0, 0}

	if gc.Flags&FlagWeHaveAScale != 0 {
		var scaleRaw int16
		if err := binary.Read(r, binary.BigEndian, &scaleRaw); err != nil {
			return nil, fmt.Errorf("reading scale (F2DOT14): %w", err)
		}
		scale := f2dot14ToFloat(scaleRaw)
		res.Trfm[0] = scale // xx
		res.Trfm[3] = scale // yy
	} else if gc.Flags&FlagWeHaveAnXAndYScale != 0 {
		var xScaleRaw, yScaleRaw int16
		if err := binary.Read(r, binary.BigEndian, &xScaleRaw); err != nil {
			return nil, fmt.Errorf("reading xScale (F2DOT14): %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &yScaleRaw); err != nil {
			return nil, fmt.Errorf("reading yScale (F2DOT14): %w", err)
		}
		xScale := f2dot14ToFloat(xScaleRaw)
		yScale := f2dot14ToFloat(yScaleRaw)
		res.Trfm[0] = xScale // xx
		res.Trfm[3] = yScale // yy
	} else if gc.Flags&FlagWeHaveATwoByTwo != 0 {
		var m0, m1, m2, m3 int16
		if err := binary.Read(r, binary.BigEndian, &m0); err != nil {
			return nil, fmt.Errorf("reading matrix xx (F2DOT14): %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &m1); err != nil {
			return nil, fmt.Errorf("reading matrix xy (F2DOT14): %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &m2); err != nil {
			return nil, fmt.Errorf("reading matrix yx (F2DOT14): %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &m3); err != nil {
			return nil, fmt.Errorf("reading matrix yy (F2DOT14): %w", err)
		}
		res.Trfm[0] = f2dot14ToFloat(m0) // xx
		res.Trfm[1] = f2dot14ToFloat(m1) // xy
		res.Trfm[2] = f2dot14ToFloat(m2) // yx
		res.Trfm[3] = f2dot14ToFloat(m3) // yy
	}

	if gc.Flags&FlagArgsAreXYValues != 0 {
		res.Trfm[4] = float64(arg1) // dx
		res.Trfm[5] = float64(arg2) // dy
		res.AlignPoints = false
	} else {
		// Point matching case - store the point indices
		res.OurPoint = arg1
		res.TheirPoint = arg2
		res.AlignPoints = true
		// dx and dy remain 0 in the transformation matrix
	}

	return res, nil
}

// ComponentUnpacked provides a structured representation of a glyph component,
// making it easier to define composite glyphs programmatically.
type ComponentUnpacked struct {
	// Child is the glyph ID of the component glyph to include.
	Child glyph.ID

	// Trfm is the 2D affine transformation matrix applied to the component.
	// Format: [xx, xy, yx, yy, dx, dy].
	// If ScaledComponentOffset is false, child coordinates are transformed as:
	//   x' = xx*x + yx*y + dx
	//   y' = xy*x + yy*y + dy
	// If ScaledComponentOffset is true, the offset is scaled:
	//   x' = xx*x + yx*y + xx*dx + yx*dy
	//   y' = xy*x + yy*y + xy*dx + yy*dy
	Trfm matrix.Matrix // [6]float64

	// AlignPoints indicates whether Arg1 and Arg2 contain
	// point indices for point matching (true) or whether Trfm[4] and Trfm[5] contain
	// actual offset values (false).
	AlignPoints bool

	// OurPoint and TheirPoint store point indices when ArgsArePointIndices is true.
	// When ArgsArePointIndices is false, these fields are ignored and
	// Trfm[4] and Trfm[5] are used instead.
	OurPoint, TheirPoint int16

	// RoundXYToGrid instructs the rasterizer to round the translation
	// values to the nearest grid points during rendering.
	RoundXYToGrid bool

	// UseMyMetrics indicates that this component's advance width and other
	// metrics should be used for the entire composite glyph.
	UseMyMetrics bool

	// OverlapCompound is a hint to rasterizers that this component may
	// overlap with other components in the composite glyph.
	OverlapCompound bool

	// ScaledComponentOffset controls how the offset in Trfm is applied.
	// When true, the offset is scaled by the transformation matrix.
	// When false, the offset is applied directly.
	ScaledComponentOffset bool
}

// Pack converts the unpacked component data back to its binary representation.
// The method preserves the positioning mode from the unpacked data:
// - If AlignPoints is false, it encodes Trfm[4] and Trfm[5] as x,y offsets
// - If AlignPoints is true, it encodes Arg1 and Arg2 as point indices for point matching
// The transformation matrix is encoded efficiently: uniform scaling uses 2 bytes,
// non-uniform scaling uses 4 bytes, and full 2x2 matrices use 8 bytes.
func (cu *ComponentUnpacked) Pack() GlyphComponent {
	gc := GlyphComponent{
		GlyphIndex: cu.Child,
	}

	// Set flags from boolean fields
	if cu.RoundXYToGrid {
		gc.Flags |= FlagRoundXYToGrid
	}
	if cu.UseMyMetrics {
		gc.Flags |= FlagUseMyMetrics
	}
	if cu.OverlapCompound {
		gc.Flags |= FlagOverlapCompound
	}

	// Only set offset scaling flags when needed to disambiguate behavior
	if cu.ScaledComponentOffset {
		gc.Flags |= FlagScaledComponentOffset
	} else {
		gc.Flags |= FlagUnscaledComponentOffset
	}

	if cu.AlignPoints {
		// Point matching case - do NOT set FlagArgsAreXYValues
		if cu.OurPoint >= -128 && cu.OurPoint <= 127 && cu.TheirPoint >= -128 && cu.TheirPoint <= 127 {
			// Can use 8-bit values
		} else {
			gc.Flags |= FlagArg1And2AreWords
		}

		var buf bytes.Buffer

		if gc.Flags&FlagArg1And2AreWords != 0 {
			binary.Write(&buf, binary.BigEndian, cu.OurPoint)
			binary.Write(&buf, binary.BigEndian, cu.TheirPoint)
		} else {
			binary.Write(&buf, binary.BigEndian, int8(cu.OurPoint))
			binary.Write(&buf, binary.BigEndian, int8(cu.TheirPoint))
		}

		gc.Data = buf.Bytes()
	} else {
		gc.Flags |= FlagArgsAreXYValues

		// Extract offset values from transformation matrix
		dx := cu.Trfm[4]
		dy := cu.Trfm[5]

		// Note: RoundXYToGrid is an instruction to the rasterizer,
		// not to round values during storage
		arg1 := int16(math.Round(dx))
		arg2 := int16(math.Round(dy))

		if arg1 >= -128 && arg1 <= 127 && arg2 >= -128 && arg2 <= 127 {
			// Can use 8-bit values
		} else {
			gc.Flags |= FlagArg1And2AreWords
		}

		var buf bytes.Buffer

		if gc.Flags&FlagArg1And2AreWords != 0 {
			binary.Write(&buf, binary.BigEndian, arg1)
			binary.Write(&buf, binary.BigEndian, arg2)
		} else {
			binary.Write(&buf, binary.BigEndian, int8(arg1))
			binary.Write(&buf, binary.BigEndian, int8(arg2))
		}

		gc.Data = buf.Bytes()
	}

	// Append transformation matrix data
	xx, xy, yx, yy := cu.Trfm[0], cu.Trfm[1], cu.Trfm[2], cu.Trfm[3]

	isIdentityScaleRotation := (xx == 1 && xy == 0 && yx == 0 && yy == 1)
	isUniformScale := (xx == yy && xy == 0 && yx == 0)
	isNonUniformScale := (xy == 0 && yx == 0 && (xx != yy || xx != 1 || yy != 1))

	var buf bytes.Buffer
	buf.Write(gc.Data)

	if isIdentityScaleRotation {
		// No scaling data needed
	} else if isUniformScale {
		gc.Flags |= FlagWeHaveAScale
		binary.Write(&buf, binary.BigEndian, floatToF2dot14(xx))
	} else if isNonUniformScale {
		gc.Flags |= FlagWeHaveAnXAndYScale
		binary.Write(&buf, binary.BigEndian, floatToF2dot14(xx))
		binary.Write(&buf, binary.BigEndian, floatToF2dot14(yy))
	} else {
		gc.Flags |= FlagWeHaveATwoByTwo
		binary.Write(&buf, binary.BigEndian, floatToF2dot14(xx))
		binary.Write(&buf, binary.BigEndian, floatToF2dot14(xy))
		binary.Write(&buf, binary.BigEndian, floatToF2dot14(yx))
		binary.Write(&buf, binary.BigEndian, floatToF2dot14(yy))
	}

	gc.Data = buf.Bytes()
	return gc
}

var errIncompleteGlyph = &parser.InvalidFontError{
	SubSystem: "sfnt/glyf",
	Reason:    "incomplete glyph",
}
