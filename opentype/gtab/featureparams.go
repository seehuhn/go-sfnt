// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
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

package gtab

import (
	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
)

// FeatureParams holds the feature-specific parameters of a [Feature].  The
// concrete type is selected by the feature tag: 'size' uses
// [FeatureParamsSize], 'ss01'–'ss20' use [FeatureParamsStylisticSet], and
// 'cv01'–'cv99' use [FeatureParamsCharacterVariants].
type FeatureParams interface {
	encodeLen() int
	encode() []byte
	isFeatureParams()
}

// FeatureParamsSize holds the parameters of the 'size' feature.  Sizes are
// given in tenths of a point.
type FeatureParamsSize struct {
	DesignSize      uint16 // design size for which the font was designed
	SubfamilyID     uint16 // identifier shared by fonts in the same size group
	SubfamilyNameID uint16 // 'name' table entry for the size group, or 0
	RangeStart      uint16 // smallest size in the recommended range
	RangeEnd        uint16 // largest size in the recommended range
}

func (*FeatureParamsSize) isFeatureParams() {}

func (*FeatureParamsSize) encodeLen() int { return 10 }

func (p *FeatureParamsSize) encode() []byte {
	return []byte{
		byte(p.DesignSize >> 8), byte(p.DesignSize),
		byte(p.SubfamilyID >> 8), byte(p.SubfamilyID),
		byte(p.SubfamilyNameID >> 8), byte(p.SubfamilyNameID),
		byte(p.RangeStart >> 8), byte(p.RangeStart),
		byte(p.RangeEnd >> 8), byte(p.RangeEnd),
	}
}

// FeatureParamsStylisticSet holds the parameters of a stylistic set feature
// ('ss01'–'ss20').
type FeatureParamsStylisticSet struct {
	Version  uint16 // table version (0)
	UINameID uint16 // 'name' table entry for the UI label
}

func (*FeatureParamsStylisticSet) isFeatureParams() {}

func (*FeatureParamsStylisticSet) encodeLen() int { return 4 }

func (p *FeatureParamsStylisticSet) encode() []byte {
	return []byte{
		byte(p.Version >> 8), byte(p.Version),
		byte(p.UINameID >> 8), byte(p.UINameID),
	}
}

// FeatureParamsCharacterVariants holds the parameters of a character variant
// feature ('cv01'–'cv99').
type FeatureParamsCharacterVariants struct {
	Format                  uint16 // table format (0)
	FeatUILabelNameID       uint16 // 'name' entry for the UI label, or 0
	FeatUITooltipTextNameID uint16 // 'name' entry for the tooltip, or 0
	SampleTextNameID        uint16 // 'name' entry for sample text, or 0
	NumNamedParameters      uint16 // number of named parameters
	FirstParamUILabelNameID uint16 // 'name' entry for the first named parameter
	Characters              []rune // Unicode values affected by this feature
}

func (*FeatureParamsCharacterVariants) isFeatureParams() {}

func (p *FeatureParamsCharacterVariants) encodeLen() int { return 14 + 3*len(p.Characters) }

func (p *FeatureParamsCharacterVariants) encode() []byte {
	charCount := len(p.Characters)
	buf := make([]byte, 0, 14+3*charCount)
	buf = append(buf,
		byte(p.Format>>8), byte(p.Format),
		byte(p.FeatUILabelNameID>>8), byte(p.FeatUILabelNameID),
		byte(p.FeatUITooltipTextNameID>>8), byte(p.FeatUITooltipTextNameID),
		byte(p.SampleTextNameID>>8), byte(p.SampleTextNameID),
		byte(p.NumNamedParameters>>8), byte(p.NumNamedParameters),
		byte(p.FirstParamUILabelNameID>>8), byte(p.FirstParamUILabelNameID),
		byte(charCount>>8), byte(charCount),
	)
	for _, c := range p.Characters {
		buf = append(buf, byte(c>>16), byte(c>>8), byte(c))
	}
	return buf
}

// readFeatureParams reads the feature parameters table for a feature with the
// given tag.  Tags without a defined parameters table, and offsets pointing
// outside the table, yield a nil result.
func readFeatureParams(p *parser.Parser, pos int64, tag string) (FeatureParams, error) {
	// feature parameters are optional metadata; an offset or table that runs
	// past the end of the data must not fail the whole feature list, so treat
	// it as absent
	if pos < 0 {
		return nil, nil
	}
	switch {
	case tag == "size":
		if pos+10 > p.Size() {
			return nil, nil
		}
		if err := p.SeekPos(pos); err != nil {
			return nil, err
		}
		buf, err := p.ReadBytes(10)
		if err != nil {
			return nil, err
		}
		return &FeatureParamsSize{
			DesignSize:      uint16(buf[0])<<8 | uint16(buf[1]),
			SubfamilyID:     uint16(buf[2])<<8 | uint16(buf[3]),
			SubfamilyNameID: uint16(buf[4])<<8 | uint16(buf[5]),
			RangeStart:      uint16(buf[6])<<8 | uint16(buf[7]),
			RangeEnd:        uint16(buf[8])<<8 | uint16(buf[9]),
		}, nil
	case isStylisticSetTag(tag):
		if pos+4 > p.Size() {
			return nil, nil
		}
		if err := p.SeekPos(pos); err != nil {
			return nil, err
		}
		buf, err := p.ReadBytes(4)
		if err != nil {
			return nil, err
		}
		return &FeatureParamsStylisticSet{
			Version:  uint16(buf[0])<<8 | uint16(buf[1]),
			UINameID: uint16(buf[2])<<8 | uint16(buf[3]),
		}, nil
	case isCharVariantTag(tag):
		if pos+14 > p.Size() {
			return nil, nil
		}
		if err := p.SeekPos(pos); err != nil {
			return nil, err
		}
		buf, err := p.ReadBytes(14)
		if err != nil {
			return nil, err
		}
		charCount := int(buf[12])<<8 | int(buf[13])
		if pos+14+3*int64(charCount) > p.Size() {
			return nil, nil
		}
		chars, err := membudget.AllocSlice[rune](p.Budget, charCount)
		if err != nil {
			return nil, err
		}
		for i := range chars {
			b, err := p.ReadBytes(3)
			if err != nil {
				return nil, err
			}
			chars[i] = rune(b[0])<<16 | rune(b[1])<<8 | rune(b[2])
		}
		return &FeatureParamsCharacterVariants{
			Format:                  uint16(buf[0])<<8 | uint16(buf[1]),
			FeatUILabelNameID:       uint16(buf[2])<<8 | uint16(buf[3]),
			FeatUITooltipTextNameID: uint16(buf[4])<<8 | uint16(buf[5]),
			SampleTextNameID:        uint16(buf[6])<<8 | uint16(buf[7]),
			NumNamedParameters:      uint16(buf[8])<<8 | uint16(buf[9]),
			FirstParamUILabelNameID: uint16(buf[10])<<8 | uint16(buf[11]),
			Characters:              chars,
		}, nil
	default:
		return nil, nil
	}
}

func isStylisticSetTag(tag string) bool {
	return len(tag) == 4 && tag[0] == 's' && tag[1] == 's' && isDigit(tag[2]) && isDigit(tag[3])
}

func isCharVariantTag(tag string) bool {
	return len(tag) == 4 && tag[0] == 'c' && tag[1] == 'v' && isDigit(tag[2]) && isDigit(tag[3])
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
