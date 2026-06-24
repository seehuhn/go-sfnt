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

package gtab

import (
	"fmt"

	"seehuhn.de/go/sfnt/parser"
)

// FeatureIndex enumerates features.
// It is used as an index into the FeatureListInfo.
// Valid values are in the range from 0 to 0xFFFE.
// The special value 0xFFFF is used to indicate the absence of required
// features in the `Features` struct.
type FeatureIndex uint16

// FeatureListInfo contains the contents of an OpenType "Feature List" table.
type FeatureListInfo []*Feature

// Feature describes an OpenType font feature.
type Feature struct {
	// Tag describes the function of this feature.
	// https://docs.microsoft.com/en-us/typography/opentype/spec/featuretags
	Tag string

	// Lookups is a list of lookup indices that implement this feature.
	Lookups []LookupIndex

	// Params holds feature-specific parameters, or nil if the feature has
	// none.
	Params FeatureParams
}

func (f Feature) String() string {
	return fmt.Sprintf("%s:%v", f.Tag, f.Lookups)
}

// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#feature-list-table
func readFeatureList(p *parser.Parser, pos int64) (FeatureListInfo, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	featureCount, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	type featureRecord struct {
		tag  string
		offs uint16
	}
	var featureList []*featureRecord
	for i := 0; i < int(featureCount); i++ {
		buf, err := p.ReadBytes(6)
		if err != nil {
			return nil, err
		}

		featureList = append(featureList, &featureRecord{
			tag:  string(buf[:4]),
			offs: uint16(buf[4])<<8 | uint16(buf[5]),
		})
	}

	info := FeatureListInfo{}
	totalSize := 2 + 6*len(featureList)
	for _, rec := range featureList {
		err = p.SeekPos(pos + int64(rec.offs))
		if err != nil {
			return nil, err
		}
		buf, err := p.ReadBytes(4)
		if err != nil {
			return nil, err
		}
		featureParamsOffset := uint16(buf[0])<<8 | uint16(buf[1])
		featureLookupCount := uint16(buf[2])<<8 | uint16(buf[3])

		if totalSize > 0xFFFF {
			// this condition also ensures featureCount < 0xFFFF
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/gtab",
				Reason:    "feature list overflow",
			}
		}
		totalSize += 4 + 2*int(featureLookupCount)

		var lookupListIndices []LookupIndex
		for i := 0; i < int(featureLookupCount); i++ {
			idx, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			lookupListIndices = append(lookupListIndices, LookupIndex(idx))
		}

		feature := &Feature{
			Tag:     rec.tag,
			Lookups: lookupListIndices,
		}
		if featureParamsOffset != 0 {
			// the offset is measured from the start of the Feature table
			feature.Params, err = readFeatureParams(p, pos+int64(rec.offs)+int64(featureParamsOffset), rec.tag)
			if err != nil {
				return nil, err
			}
		}
		info = append(info, feature)
	}

	return info, nil
}

func (info FeatureListInfo) encode() []byte {
	if info == nil {
		return nil
	}

	offs := make([]uint16, len(info))
	totalSize := 2 + 6*len(info)
	var largestOffset int
	for i, f := range info {
		largestOffset = totalSize
		offs[i] = uint16(totalSize)
		totalSize += 4 + 2*len(f.Lookups)
		if f.Params != nil {
			totalSize += f.Params.encodeLen()
		}
	}
	if largestOffset > 0xFFFF {
		panic("featureListInfo too large")
	}

	buf := make([]byte, 2+6*len(info), totalSize)
	buf[0] = byte(len(info) >> 8)
	buf[1] = byte(len(info))
	for i, f := range info {
		copy(buf[2+6*i:6+6*i], f.Tag)
		buf[6+6*i] = byte(offs[i] >> 8)
		buf[7+6*i] = byte(offs[i])
	}
	for _, f := range info {
		// the feature params table, if any, follows the lookup indices
		var featureParamsOffset int
		if f.Params != nil {
			featureParamsOffset = 4 + 2*len(f.Lookups)
		}
		buf = append(buf,
			byte(featureParamsOffset>>8), byte(featureParamsOffset),
			byte(len(f.Lookups)>>8), byte(len(f.Lookups)))
		for _, l := range f.Lookups {
			buf = append(buf, byte(l>>8), byte(l))
		}
		if f.Params != nil {
			buf = append(buf, f.Params.encode()...)
		}
	}
	return buf
}

// Default features for use with the [Info.FindLookups] method.
var (
	// GsubDefaultFeatures lists a set of default features for "GSUB" tables.
	GsubDefaultFeatures = map[string]bool{
		"calt": true, // Contextual Alternates
		"ccmp": true, // Glyph Composition / Decomposition
		"clig": true, // Contextual Ligatures
		"liga": true, // Standard Ligatures
		"locl": true, // Localized Forms
	}

	// GposDefaultFeatures lists a set of default features for "GPOS" tables.
	GposDefaultFeatures = map[string]bool{
		"kern": true, // Kerning
		"mark": true, // Mark Positioning
		"mkmk": true, // Mark to Mark Positioning
	}
)
