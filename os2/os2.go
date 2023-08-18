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

// Package os2 reads and writes "OS/2" tables.
// https://docs.microsoft.com/en-us/typography/opentype/spec/os2
package os2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/parser"
)

// Info contains information from the "OS/2" table.
type Info struct {
	WeightClass Weight
	WidthClass  Width

	IsBold    bool // glyphs are emboldened
	IsItalic  bool // font contains italic or oblique glyphs
	IsRegular bool // glyphs are in the standard weight/style for the font
	IsOblique bool // font contains oblique glyphs

	FirstCharIndex uint16
	LastCharIndex  uint16

	Ascent     funit.Int16
	Descent    funit.Int16 // negative
	WinAscent  funit.Int16
	WinDescent funit.Int16 // positive
	LineGap    funit.Int16
	CapHeight  funit.Int16
	XHeight    funit.Int16

	AvgGlyphWidth funit.Int16 // arithmetic average of the width of all non-zero width glyphs

	SubscriptXSize     funit.Int16
	SubscriptYSize     funit.Int16
	SubscriptXOffset   funit.Int16
	SubscriptYOffset   funit.Int16
	SuperscriptXSize   funit.Int16
	SuperscriptYSize   funit.Int16
	SuperscriptXOffset funit.Int16
	SuperscriptYOffset funit.Int16
	StrikeoutSize      funit.Int16
	StrikeoutPosition  funit.Int16

	FamilyClass int16    // https://docs.microsoft.com/en-us/typography/opentype/spec/ibmfc
	Panose      [10]byte // https://monotype.github.io/panose/
	Vendor      string   // https://docs.microsoft.com/en-us/typography/opentype/spec/os2#achvendid

	UnicodeRange  UnicodeRange
	CodePageRange CodePageRange

	PermUse          Permissions
	PermNoSubsetting bool // the font may not be subsetted prior to embedding
	PermOnlyBitmap   bool // only bitmaps contained in the font may be embedded
}

// Read reads the "OS/2" table from r.
func Read(r io.Reader) (*Info, error) {
	v0 := &v0Data{}
	err := binary.Read(r, binary.BigEndian, v0)
	if err != nil {
		return nil, err
	} else if v0.Version > 5 {
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/os2",
			Feature:   fmt.Sprintf("OS/2 table version %d", v0.Version),
		}
	}

	var permUse Permissions
	permBits := v0.Type
	if v0.Version < 3 {
		permBits &= 0xF
	}
	if permBits&8 != 0 {
		permUse = PermEdit
	} else if permBits&4 != 0 {
		permUse = PermView
	} else if permBits&2 != 0 {
		permUse = PermRestricted
	} else {
		permUse = PermInstall
	}

	sel := v0.Selection
	if v0.Version <= 3 {
		// Applications should ignore bits 7 to 15 in a font that has a
		// version 0 to version 3 OS/2 table.
		sel &= 0x007F
	}

	v0.UnicodeRange.Bool(57, v0.LastCharIndex == 0xFFFF) // "Non-Plane 0" bit

	info := &Info{
		WeightClass: Weight(v0.WeightClass),
		WidthClass:  Width(v0.WidthClass),

		IsBold:   sel&0x0060 == 0x0020,
		IsItalic: sel&0x0041 == 0x0001,
		// HasUnderline: sel&0x0042 == 0x0002,
		// IsOutlined:   sel&0x0048 == 0x0008,
		IsRegular: sel&0x0040 != 0,
		IsOblique: sel&0x0200 != 0,

		FirstCharIndex: v0.FirstCharIndex,
		LastCharIndex:  v0.LastCharIndex,

		AvgGlyphWidth: v0.AvgCharWidth,

		SubscriptXSize:     v0.SubscriptXSize,
		SubscriptYSize:     v0.SubscriptYSize,
		SubscriptXOffset:   v0.SubscriptXOffset,
		SubscriptYOffset:   v0.SubscriptYOffset,
		SuperscriptXSize:   v0.SuperscriptXSize,
		SuperscriptYSize:   v0.SuperscriptYSize,
		SuperscriptXOffset: v0.SuperscriptXOffset,
		SuperscriptYOffset: v0.SuperscriptYOffset,
		StrikeoutSize:      v0.StrikeoutSize,
		StrikeoutPosition:  v0.StrikeoutPosition,

		FamilyClass: v0.FamilyClass,
		Panose:      v0.Panose,
		Vendor:      string(v0.VendID[:]),

		UnicodeRange: v0.UnicodeRange,

		PermUse:          permUse,
		PermNoSubsetting: permBits&0x0100 != 0,
		PermOnlyBitmap:   permBits&0x0200 != 0,
	}

	v0ms := &v0MsData{}
	err = binary.Read(r, binary.BigEndian, v0ms)
	if err == io.EOF {
		return info, nil
	} else if err != nil {
		return nil, err
	}
	info.Ascent = v0ms.TypoAscender
	info.Descent = v0ms.TypoDescender
	info.LineGap = v0ms.TypoLineGap
	info.WinAscent = v0ms.WinAscent
	info.WinDescent = v0ms.WinDescent

	if v0.Version < 2 {
		return info, nil
	}

	var codePageRange [8]byte
	err = binary.Read(r, binary.BigEndian, codePageRange[:])
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	info.CodePageRange = CodePageRange(codePageRange[0])<<24 |
		CodePageRange(codePageRange[1])<<16 |
		CodePageRange(codePageRange[2])<<8 |
		CodePageRange(codePageRange[3]) |
		CodePageRange(codePageRange[4])<<56 |
		CodePageRange(codePageRange[5])<<48 |
		CodePageRange(codePageRange[6])<<40 |
		CodePageRange(codePageRange[7])<<32

	v2 := &v2Data{}
	err = binary.Read(r, binary.BigEndian, v2)
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	if v2.XHeight > 0 {
		info.XHeight = v2.XHeight
	}
	if v2.CapHeight > 0 {
		info.CapHeight = v2.CapHeight
	}

	return info, nil
}

// Encode converts the info to a "OS/2" table.
func (info *Info) Encode() []byte {
	var permBits uint16
	switch info.PermUse {
	case PermRestricted:
		permBits |= 2
	case PermView:
		permBits |= 4
	case PermEdit:
		permBits |= 8
	}
	if info.PermNoSubsetting {
		permBits |= 0x0100
	}
	if info.PermOnlyBitmap {
		permBits |= 0x0200
	}

	var sel uint16
	if info.IsRegular {
		sel |= 0x0040
	} else {
		if info.IsItalic {
			sel |= 0x0001
		}
		if info.IsBold {
			sel |= 0x0020
		}
	}
	// if info.HasUnderline {
	// 	sel |= 0x0002
	// }
	// if info.IsOutlined {
	// 	sel |= 0x0008
	// }
	if info.IsOblique {
		sel |= 0x0200
	}
	sel |= 0x0080 // Use_Typo_Metrics: always use Typo{A,De}scender

	vendor := [4]byte{' ', ' ', ' ', ' '}
	if len(info.Vendor) == 4 {
		copy(vendor[:], info.Vendor)
	}

	buf := &bytes.Buffer{}
	v0 := &v0Data{
		Version:            4,
		AvgCharWidth:       info.AvgGlyphWidth,
		WeightClass:        uint16(info.WeightClass),
		WidthClass:         uint16(info.WidthClass),
		Type:               permBits,
		SubscriptXSize:     info.SubscriptXSize,
		SubscriptYSize:     info.SubscriptYSize,
		SubscriptXOffset:   info.SubscriptXOffset,
		SubscriptYOffset:   info.SubscriptYOffset,
		SuperscriptXSize:   info.SuperscriptXSize,
		SuperscriptYSize:   info.SuperscriptYSize,
		SuperscriptXOffset: info.SuperscriptXOffset,
		SuperscriptYOffset: info.SuperscriptYOffset,
		StrikeoutSize:      info.StrikeoutSize,
		StrikeoutPosition:  info.StrikeoutPosition,
		FamilyClass:        info.FamilyClass,
		Panose:             info.Panose,
		UnicodeRange:       info.UnicodeRange,
		VendID:             vendor,
		Selection:          sel,
		FirstCharIndex:     info.FirstCharIndex,
		LastCharIndex:      info.LastCharIndex,
	}
	v0.UnicodeRange.Bool(57, info.LastCharIndex == 0xFFFF) // "Non-Plane 0" bit
	_ = binary.Write(buf, binary.BigEndian, v0)

	v0ms := &v0MsData{
		TypoAscender:  info.Ascent,
		TypoDescender: info.Descent,
		TypoLineGap:   info.LineGap,
		WinAscent:     info.WinAscent,
		WinDescent:    info.WinDescent,
	}
	_ = binary.Write(buf, binary.BigEndian, v0ms)

	codePageRange := info.CodePageRange
	buf.Write([]byte{
		byte(codePageRange >> 24),
		byte(codePageRange >> 16),
		byte(codePageRange >> 8),
		byte(codePageRange),
		byte(codePageRange >> 56),
		byte(codePageRange >> 48),
		byte(codePageRange >> 40),
		byte(codePageRange >> 32),
	})

	v2 := &v2Data{
		XHeight:   info.XHeight,
		CapHeight: info.CapHeight,
		// MaxContext:  0, // TODO(voss)
	}
	_ = binary.Write(buf, binary.BigEndian, v2)

	return buf.Bytes()
}

// UnicodeRange is a bitfield which describes which unicode
// blocks or ranges are "functional" in a font.
// https://learn.microsoft.com/en-us/typography/opentype/spec/os2#ur
type UnicodeRange [4]uint32

// Set sets the given bit in the unicode range.
func (ur *UnicodeRange) Set(bit UnicodeRangeBit) {
	w := bit / 32
	bit = bit % 32
	ur[w] |= 1 << bit
}

// Bool sets or clears the given bit in the unicode range.
func (ur *UnicodeRange) Bool(bit UnicodeRangeBit, set bool) {
	w := bit / 32
	bit = bit % 32
	if set {
		ur[w] |= 1 << bit
	} else {
		ur[w] &^= 1 << bit
	}
}

type UnicodeRangeBit int

const (
	URBasicLatin                UnicodeRangeBit = 0
	URLatin1Sup                 UnicodeRangeBit = 1
	URLatinExtA                 UnicodeRangeBit = 2
	URLatinExtB                 UnicodeRangeBit = 3
	URIPAExtensions             UnicodeRangeBit = 4
	URSpacingModifierLetters    UnicodeRangeBit = 5
	URCombiningDiacriticalMarks UnicodeRangeBit = 6
	URGreek                     UnicodeRangeBit = 7
	URCoptic                    UnicodeRangeBit = 8
	URCyrillic                  UnicodeRangeBit = 9
	URArmenian                  UnicodeRangeBit = 10
	URHebrew                    UnicodeRangeBit = 11
	URVai                       UnicodeRangeBit = 12
	URArabic                    UnicodeRangeBit = 13
	URNko                       UnicodeRangeBit = 14
	URDevanagari                UnicodeRangeBit = 15
	URBengali                   UnicodeRangeBit = 16
	URGurmukhi                  UnicodeRangeBit = 17
	URGujarati                  UnicodeRangeBit = 18
	UROriya                     UnicodeRangeBit = 19
	URTamil                     UnicodeRangeBit = 20
	URTelugu                    UnicodeRangeBit = 21
	URKannada                   UnicodeRangeBit = 22
	URMalayalam                 UnicodeRangeBit = 23
	URThai                      UnicodeRangeBit = 24
	URLao                       UnicodeRangeBit = 25
	URGeorgian                  UnicodeRangeBit = 26
	URBalinese                  UnicodeRangeBit = 27
	URHangulJamo                UnicodeRangeBit = 28
	URLatinExtAdditional        UnicodeRangeBit = 29
	URGreekExt                  UnicodeRangeBit = 30
	URGeneralPunctuation        UnicodeRangeBit = 31
	URSuperscriptsSubscripts    UnicodeRangeBit = 32
	URCurrencySymbols           UnicodeRangeBit = 33
	// TODO(voss): finish this
)

// CodePageRange is a bitmask of code pages supported by a font.
type CodePageRange uint64

// Set sets the given bit in the code page range.
func (cpr *CodePageRange) Set(bit CodePage) {
	*cpr |= 1 << bit
}

// CodePage represents the positions of individual bits which may be set in a
// [CodeSpaceRange].
type CodePage int

// List of code pages supported by the "OS/2" table.
const (
	CP1252      CodePage = 0  // CP1252, Latin 1
	CP1250      CodePage = 1  // CP1250, Latin 2: Eastern Europe
	CP1251      CodePage = 2  // CP1251, Cyrillic
	CP1253      CodePage = 3  // CP1253, Greek
	CP1254      CodePage = 4  // CP1254, Turkish
	CP1255      CodePage = 5  // CP1255, Hebrew
	CP1256      CodePage = 6  // CP1256, Arabic
	CP1257      CodePage = 7  // CP1257, Windows Baltic
	CP1258      CodePage = 8  // CP1258, Vietnamese
	CP874       CodePage = 16 // CP874, Thai
	CP932       CodePage = 17 // CP932, JIS/Japan
	CP936       CodePage = 18 // CP936, Chinese: Simplified chars—PRC and Singapore
	CP949       CodePage = 19 // CP949, Korean Wansung
	CP950       CodePage = 20 // CP950, Chinese: Traditional chars—Taiwan and Hong Kong
	CP1361      CodePage = 21 // CP1361, Korean Johab
	CPMacintosh CodePage = 29 // Macintosh Character Set (US Roman)
	CPOEM       CodePage = 30 // OEM Character Set
	CPSymbol    CodePage = 31 // Symbol Character Set
	CP869       CodePage = 48 // CP869, IBM Greek
	CP866       CodePage = 49 // CP866, MS-DOS Russian
	CP865       CodePage = 50 // CP865, MS-DOS Nordic
	CP864       CodePage = 51 // CP864, Arabic
	CP863       CodePage = 52 // CP863, MS-DOS Canadian French
	CP862       CodePage = 53 // CP862, Hebrew
	CP861       CodePage = 54 // CP861, MS-DOS Icelandic
	CP860       CodePage = 55 // CP860, MS-DOS Portuguese
	CP857       CodePage = 56 // CP857, IBM Turkish
	CP855       CodePage = 57 // CP855, IBM Cyrillic; primarily Russian
	CP852       CodePage = 58 // CP852, Latin 2
	CP775       CodePage = 59 // CP775, MS-DOS Baltic
	CP737       CodePage = 60 // CP737, Greek; former 437 G
	CP708       CodePage = 61 // CP708, Arabic; ASMO 708
	CP850       CodePage = 62 // CP850, WE/Latin 1
	CP437       CodePage = 63 // CP437, US
)

// Permissions describes rights to embed and use a font.
type Permissions int

func (perm Permissions) String() string {
	switch perm {
	case PermInstall:
		return "can install"
	case PermEdit:
		return "can edit"
	case PermView:
		return "can view"
	case PermRestricted:
		return "restricted"
	default:
		return fmt.Sprintf("Permissions(%d)", perm)
	}
}

// The possible permission values.
// https://learn.microsoft.com/en-us/typography/opentype/spec/os2#fstype
const (
	PermInstall    Permissions = iota // bits 0-3 unset
	PermEdit                          // only bit 3 set
	PermView                          // only bit 2 set
	PermRestricted                    // only bit 1 set
)

type v0Data struct {
	Version            uint16
	AvgCharWidth       funit.Int16
	WeightClass        uint16
	WidthClass         uint16
	Type               uint16
	SubscriptXSize     funit.Int16
	SubscriptYSize     funit.Int16
	SubscriptXOffset   funit.Int16
	SubscriptYOffset   funit.Int16
	SuperscriptXSize   funit.Int16
	SuperscriptYSize   funit.Int16
	SuperscriptXOffset funit.Int16
	SuperscriptYOffset funit.Int16
	StrikeoutSize      funit.Int16
	StrikeoutPosition  funit.Int16
	FamilyClass        int16
	Panose             [10]byte
	UnicodeRange       UnicodeRange
	VendID             [4]byte
	Selection          uint16
	FirstCharIndex     uint16
	LastCharIndex      uint16
}

type v0MsData struct {
	TypoAscender  funit.Int16
	TypoDescender funit.Int16
	TypoLineGap   funit.Int16
	WinAscent     funit.Int16
	WinDescent    funit.Int16 // positive
}

type v2Data struct {
	XHeight     funit.Int16
	CapHeight   funit.Int16
	DefaultChar uint16
	BreakChar   uint16
	MaxContext  uint16
}
