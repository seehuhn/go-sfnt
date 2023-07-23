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

package type1

import (
	"strconv"

	"seehuhn.de/go/sfnt/funit"
)

type FontInfo struct {
	// PostScript language name (FontName or CIDFontName) of the font.
	FontName string

	// Version is the version number of the font program.
	Version string

	// Notice is used to save any trademark notice/information for the font.
	Notice string

	// The copyright notice of the font.
	Copyright string

	// FullName is a unique, human-readable name for an individual font.
	FullName string

	// FamilyName is a human-readable name for a group of fonts that are
	// stylistic variants of a single design.  All fonts that are members of
	// such a group should have exactly the same FamilyName value.
	FamilyName string

	// A human-readable name for the weight, or "boldness," of a font.
	Weight string

	// ItalicAngle is the angle, in degrees counterclockwise from the vertical,
	// of the dominant vertical strokes of the font.
	ItalicAngle float64

	// IsFixedPitch is a flag indicating whether the font is a fixed-pitch
	// (monospaced) font.
	IsFixedPitch bool

	// UnderlinePosition is the recommended distance from the baseline for
	// positioning underlining strokes. This number is the y coordinate (in the
	// glyph coordinate system) of the center of the stroke.
	//
	// TODO(voss): change this to funit.Float64?
	UnderlinePosition funit.Int16

	// UnderlineThickness is the recommended stroke width for underlining, in
	// units of the glyph coordinate system.
	//
	// TODO(voss): change this to funit.Float64?
	UnderlineThickness funit.Int16

	FontMatrix []float64
}

// PrivateDict contains information about a font's private dictionary.
type PrivateDict struct {
	// BlueValues is an array containing an even number of integers.
	// The first integer in each pair is less than or equal to the second integer.
	// The first pair is the baseline overshoot position and the baseline.
	// All subsequent pairs describe alignment zones for the tops of character features.
	BlueValues []funit.Int16

	OtherBlues []funit.Int16

	BlueScale float64

	BlueShift int32

	BlueFuzz int32

	// StdHW is the dominant width of horizontal stems for glyphs in the font.
	StdHW float64

	// StdVW the dominant width of vertical stems.
	// Typically, this will be the width of straight stems in lower case letters.
	StdVW float64

	// TODO(voss): StemSnapH, StemSnapV

	ForceBold bool
}

// CIDSystemInfo describes a character collection covered by a PostScript font.
// A character collection implies an encoding which maps Character IDs to glyphs.
// See sections 9.7.3 of PDF 32000-1:2008.
type CIDSystemInfo struct {
	Registry   string
	Ordering   string
	Supplement int32
}

func (ROS *CIDSystemInfo) String() string {
	return ROS.Registry + "-" + ROS.Ordering + "-" + strconv.Itoa(int(ROS.Supplement))
}

// CID represents a character identifier.  This is the index of character in a
// character collection.
//
// TODO(voss): should this be uint16 instead?
type CID uint32
