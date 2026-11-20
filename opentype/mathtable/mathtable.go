// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package mathtable reads and writes the OpenType "MATH" table, which
// holds the font data needed for typesetting mathematical formulas.
// https://learn.microsoft.com/en-us/typography/opentype/spec/math
//
// The per-glyph data is stored in maps keyed by glyph ID, instead of
// coverage tables with parallel arrays.  The coverage tables are generated
// when the table is written.
package mathtable

import (
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
)

// Info represents the contents of a "MATH" table.
//
// Read returns nil for empty maps, and Encode treats nil and empty maps
// alike.  Pointers in the structure may be shared, for example between the
// kern tables of different glyphs; such sharing is preserved by Read and
// used by Encode to save space.  Shared values must not be modified.
type Info struct {
	// Constants holds the font-wide layout parameters.
	Constants Constants

	// ItalicsCorrection gives the italics correction of slanted glyphs.
	ItalicsCorrection map[glyph.ID]Value

	// TopAccent gives the horizontal attachment point for top accents.
	TopAccent map[glyph.ID]Value

	// ExtendedShapes is the set of glyphs which are extended shapes.
	ExtendedShapes coverage.Set

	// Kerns holds the mathematical kerning information of glyphs.
	Kerns map[glyph.ID]*KernInfo

	// MinConnectorOverlap is the minimum overlap of connecting glyphs
	// during glyph construction.
	MinConnectorOverlap funit.Uint16

	// Vertical and Horizontal describe how glyphs can grow in the vertical
	// and horizontal directions.
	Vertical, Horizontal map[glyph.ID]*Construction
}

// Value is a MathValueRecord: a value in font design units, with an
// optional correction.
type Value struct {
	V funit.Int16

	// Device is the optional correction for V.  It is either a Device table
	// or a VariationIndex table, or nil.
	Device *device.Table
}

// Constants holds the fields of the MathConstants table, in the order and
// with the names used in the specification.
type Constants struct {
	ScriptPercentScaleDown       int16
	ScriptScriptPercentScaleDown int16
	DelimitedSubFormulaMinHeight funit.Uint16
	DisplayOperatorMinHeight     funit.Uint16

	MathLeading                              Value
	AxisHeight                               Value
	AccentBaseHeight                         Value
	FlattenedAccentBaseHeight                Value
	SubscriptShiftDown                       Value
	SubscriptTopMax                          Value
	SubscriptBaselineDropMin                 Value
	SuperscriptShiftUp                       Value
	SuperscriptShiftUpCramped                Value
	SuperscriptBottomMin                     Value
	SuperscriptBaselineDropMax               Value
	SubSuperscriptGapMin                     Value
	SuperscriptBottomMaxWithSubscript        Value
	SpaceAfterScript                         Value
	UpperLimitGapMin                         Value
	UpperLimitBaselineRiseMin                Value
	LowerLimitGapMin                         Value
	LowerLimitBaselineDropMin                Value
	StackTopShiftUp                          Value
	StackTopDisplayStyleShiftUp              Value
	StackBottomShiftDown                     Value
	StackBottomDisplayStyleShiftDown         Value
	StackGapMin                              Value
	StackDisplayStyleGapMin                  Value
	StretchStackTopShiftUp                   Value
	StretchStackBottomShiftDown              Value
	StretchStackGapAboveMin                  Value
	StretchStackGapBelowMin                  Value
	FractionNumeratorShiftUp                 Value
	FractionNumeratorDisplayStyleShiftUp     Value
	FractionDenominatorShiftDown             Value
	FractionDenominatorDisplayStyleShiftDown Value
	FractionNumeratorGapMin                  Value
	FractionNumDisplayStyleGapMin            Value
	FractionRuleThickness                    Value
	FractionDenominatorGapMin                Value
	FractionDenomDisplayStyleGapMin          Value
	SkewedFractionHorizontalGap              Value
	SkewedFractionVerticalGap                Value
	OverbarVerticalGap                       Value
	OverbarRuleThickness                     Value
	OverbarExtraAscender                     Value
	UnderbarVerticalGap                      Value
	UnderbarRuleThickness                    Value
	UnderbarExtraDescender                   Value
	RadicalVerticalGap                       Value
	RadicalDisplayStyleVerticalGap           Value
	RadicalRuleThickness                     Value
	RadicalExtraAscender                     Value
	RadicalKernBeforeDegree                  Value
	RadicalKernAfterDegree                   Value

	RadicalDegreeBottomRaisePercent int16
}

// numConstantValues is the number of MathValueRecords in MathConstants.
const numConstantValues = 51

// values returns pointers to the MathValueRecord fields, in the order in
// which they are stored in the table.
func (c *Constants) values() [numConstantValues]*Value {
	return [numConstantValues]*Value{
		&c.MathLeading,
		&c.AxisHeight,
		&c.AccentBaseHeight,
		&c.FlattenedAccentBaseHeight,
		&c.SubscriptShiftDown,
		&c.SubscriptTopMax,
		&c.SubscriptBaselineDropMin,
		&c.SuperscriptShiftUp,
		&c.SuperscriptShiftUpCramped,
		&c.SuperscriptBottomMin,
		&c.SuperscriptBaselineDropMax,
		&c.SubSuperscriptGapMin,
		&c.SuperscriptBottomMaxWithSubscript,
		&c.SpaceAfterScript,
		&c.UpperLimitGapMin,
		&c.UpperLimitBaselineRiseMin,
		&c.LowerLimitGapMin,
		&c.LowerLimitBaselineDropMin,
		&c.StackTopShiftUp,
		&c.StackTopDisplayStyleShiftUp,
		&c.StackBottomShiftDown,
		&c.StackBottomDisplayStyleShiftDown,
		&c.StackGapMin,
		&c.StackDisplayStyleGapMin,
		&c.StretchStackTopShiftUp,
		&c.StretchStackBottomShiftDown,
		&c.StretchStackGapAboveMin,
		&c.StretchStackGapBelowMin,
		&c.FractionNumeratorShiftUp,
		&c.FractionNumeratorDisplayStyleShiftUp,
		&c.FractionDenominatorShiftDown,
		&c.FractionDenominatorDisplayStyleShiftDown,
		&c.FractionNumeratorGapMin,
		&c.FractionNumDisplayStyleGapMin,
		&c.FractionRuleThickness,
		&c.FractionDenominatorGapMin,
		&c.FractionDenomDisplayStyleGapMin,
		&c.SkewedFractionHorizontalGap,
		&c.SkewedFractionVerticalGap,
		&c.OverbarVerticalGap,
		&c.OverbarRuleThickness,
		&c.OverbarExtraAscender,
		&c.UnderbarVerticalGap,
		&c.UnderbarRuleThickness,
		&c.UnderbarExtraDescender,
		&c.RadicalVerticalGap,
		&c.RadicalDisplayStyleVerticalGap,
		&c.RadicalRuleThickness,
		&c.RadicalExtraAscender,
		&c.RadicalKernBeforeDegree,
		&c.RadicalKernAfterDegree,
	}
}

// KernInfo holds the math kerning tables for the four corners of a glyph.
// A nil corner means that no kerning is applied there.
type KernInfo struct {
	TopRight, TopLeft, BottomRight, BottomLeft *Kern
}

// corners returns the kern tables in the order used in the file.
func (k *KernInfo) corners() [4]*Kern {
	return [4]*Kern{k.TopRight, k.TopLeft, k.BottomRight, k.BottomLeft}
}

// Kern is a MathKern table.  Kerns[i] applies to heights between
// Heights[i-1] and Heights[i].
//
// The correction heights must be in non-decreasing order, and Kerns must have
// one more element than Heights.
type Kern struct {
	Heights, Kerns []Value
}

// Construction describes how a glyph can grow in one direction.  At least
// one of Variants and Assembly must be present.
type Construction struct {
	// Variants lists pre-made variants of the glyph, normally
	// listed in order of increasing size.
	Variants []Variant

	// Assembly describes how the glyph can be assembled from parts, or is
	// nil.
	Assembly *Assembly
}

// Variant is a pre-made larger variant of a glyph.
type Variant struct {
	Glyph glyph.ID

	// Advance is the size of the variant in the direction of growth.
	Advance funit.Uint16
}

// Assembly describes how a glyph can be built from several parts.  The
// parts are listed from left to right, or from bottom to top, and there
// must be at least one.
type Assembly struct {
	ItalicsCorrection Value
	Parts             []Part
}

// Part is one part of a glyph assembly.
type Part struct {
	Glyph glyph.ID

	// StartConnector and EndConnector give the lengths of the connecting
	// material at the two ends of the part.
	StartConnector, EndConnector funit.Uint16

	// FullAdvance is the full size of the part in the direction of growth.
	FullAdvance funit.Uint16

	// Extender is true if the part can be skipped or repeated.
	Extender bool
}
