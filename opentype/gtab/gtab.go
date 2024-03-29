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
	"encoding/binary"
	"fmt"

	"seehuhn.de/go/sfnt/parser"
)

// Info contains the information from an OpenType "GSUB" or "GPOS" table.
type Info struct {
	// The ScriptList lists the font features available for each natural
	// language.  Features are given as indices into the FeatureList.
	ScriptList ScriptListInfo

	// The FeatureList enumerates all font features available in the font.
	// Features are implemented by lookups from the LookupList.
	FeatureList FeatureListInfo

	// The LookupList enumerates all the OpenType lookups used to implement
	// the font features.
	LookupList LookupList
}

// Type chooses between "GSUB" and "GPOS" tables.
// The possible values are [TypeGsub] and [TypeGpos].
type Type byte

func (tp Type) String() string {
	switch tp {
	case TypeGsub:
		return "GSUB"
	case TypeGpos:
		return "GPOS"
	default:
		return fmt.Sprintf("Type(%d)", tp)
	}

}

// These are the allowed types for use in the [Read] function.
const (
	// TypeGsub is an OpenType "GSUB" table.
	TypeGsub = iota + 1

	// TypeGpos is an OpenType "GPOS" table.
	TypeGpos
)

// Read reads and decodes an OpenType "GSUB" or "GPOS" table from r.
// The tp argument must be one of [TypeGsub] or [TypeGpos].
//
// The format of the data read is defined here:
//   - https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#gsub-header
//   - https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#gpos-header
func Read(r parser.ReadSeekSizer, tp Type) (*Info, error) {
	var sr subtableReader
	switch tp {
	case TypeGsub:
		sr = readGsubSubtable
	case TypeGpos:
		sr = readGposSubtable
	default:
		return nil, fmt.Errorf("unsupported Gtab table type %d", tp)
	}
	return readGtab(r, tp, sr)
}

func readGtab(r parser.ReadSeekSizer, tp Type, sr subtableReader) (*Info, error) {
	p := parser.New(r)

	var header struct {
		MajorVersion      uint16
		MinorVersion      uint16
		ScriptListOffset  uint16
		FeatureListOffset uint16
		LookupListOffset  uint16
	}
	var FeatureVariationsOffset uint32

	err := binary.Read(p, binary.BigEndian, &header)
	if err != nil {
		return nil, err
	}
	if header.MajorVersion != 1 || header.MinorVersion > 1 {
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/opentype/gtab",
			Feature: fmt.Sprintf("%s table version %d.%d",
				tp, header.MajorVersion, header.MinorVersion),
		}
	}
	endOfHeader := uint32(10)
	if header.MinorVersion == 1 {
		FeatureVariationsOffset, err = p.ReadUint32()
		if err != nil {
			return nil, err
		}
		endOfHeader += 4
	}

	if header.ScriptListOffset == 0 || header.LookupListOffset == 0 {
		return &Info{
			ScriptList: make(ScriptListInfo),
		}, nil
	}

	fileSize := p.Size()
	for _, offset := range []uint32{
		uint32(header.ScriptListOffset),
		uint32(header.FeatureListOffset),
		uint32(header.LookupListOffset),
	} {
		if offset < endOfHeader || int64(offset) >= fileSize {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason: fmt.Sprintf("%s header has invalid offset %d",
					tp, offset),
			}
		}
	}
	if FeatureVariationsOffset != 0 && FeatureVariationsOffset < endOfHeader ||
		int64(FeatureVariationsOffset) >= fileSize {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    fmt.Sprintf("%s header has invalid FeatureVariationsOffset", tp),
		}
	}

	info := &Info{}
	info.ScriptList, err = readScriptList(p, int64(header.ScriptListOffset))
	if err != nil {
		return nil, err
	}
	info.FeatureList, err = readFeatureList(p, int64(header.FeatureListOffset))
	if err != nil {
		return nil, err
	}
	info.LookupList, err = readLookupList(p, int64(header.LookupListOffset), sr)
	if err != nil {
		return nil, err
	}

	_ = FeatureVariationsOffset // TODO(voss): implement this

	return info, nil
}

// Encode returns the binary representation of a "GSUB" or "GPOS" table.
func (info *Info) Encode() []byte {
	scriptList := info.ScriptList.encode()
	featureList := info.FeatureList.encode()
	lookupList := info.LookupList.encode()

	total := 10
	var scriptListOffset int
	if scriptList != nil {
		scriptListOffset = total
		total += len(scriptList)
	}
	var featureListOffset int
	if featureList != nil {
		featureListOffset = total
		total += len(featureList)
	}
	var lookupListOffset int
	if lookupList != nil {
		lookupListOffset = total
		total += len(lookupList)
	}

	buf := make([]byte, total)
	copy(buf, []byte{
		0, 1, // major version
		0, 0, // minor version
		byte(scriptListOffset >> 8), byte(scriptListOffset),
		byte(featureListOffset >> 8), byte(featureListOffset),
		byte(lookupListOffset >> 8), byte(lookupListOffset),
	})
	copy(buf[scriptListOffset:], scriptList)
	copy(buf[featureListOffset:], featureList)
	copy(buf[lookupListOffset:], lookupList)

	return buf
}
