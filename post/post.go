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

// Package post reads and writes "post" tables.
// https://docs.microsoft.com/en-us/typography/opentype/spec/post
package post

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/parser"
)

// Info contains information from the "post" table.
type Info struct {
	ItalicAngle        float64     // Italic angle in degrees
	UnderlinePosition  funit.Int16 // Underline position (negative)
	UnderlineThickness funit.Int16 // Underline thickness
	IsFixedPitch       bool

	Names []string // can be nil
}

// Read reads the "post" table from r.
// The slice in the .Names field in the returned structure, if non-nil,
// may point to shared internal storage and must not be shared.
// The function may read r beyond the end of the table.
func Read(r parser.ReadSeekSizer) (*Info, error) {
	p := parser.New(r)

	post := &postEnc{}
	if err := binary.Read(p, binary.BigEndian, post); err != nil {
		return nil, err
	}

	info := &Info{
		ItalicAngle:        float64(post.ItalicAngle) / 65536,
		UnderlinePosition:  post.UnderlinePosition,
		UnderlineThickness: post.UnderlineThickness,
		IsFixedPitch:       post.IsFixedPitch != 0,
	}

	switch post.Version {
	case 0x00010000:
		info.Names = macRoman

	case 0x00020000:
		glyphNameIndex, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}
		numGlyphs := len(glyphNameIndex)

		var names []string

		info.Names = make([]string, numGlyphs)
		nMac := len(macRoman)
		for i, idx := range glyphNameIndex {
			idx := int(idx)
			if idx < nMac {
				info.Names[i] = macRoman[idx]
			} else {
				idx -= nMac
				for len(names) <= idx {
					l, err := p.ReadUint8()
					if err != nil {
						return nil, err
					}
					buf, err := p.ReadBytes(int(l))
					if err != nil {
						return nil, err
					}
					names = append(names, string(buf))
				}
				info.Names[i] = names[idx]
			}
		}

	case 0x00030000:
		// pass

	case 0x00040000:
		// https://developer.apple.com/fonts/TrueType-Reference-Manual/RM06/Chap6post.html
		// pass

	default:
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/post",
			Feature:   fmt.Sprintf("table version %08x", post.Version),
		}
	}

	return info, nil
}

// Encode encodes the "post" table.
func (info *Info) Encode() []byte {
	var version uint32
	if info.Names == nil {
		version = 0x00030000
	} else if isMacRoman(info.Names) {
		version = 0x00010000
	} else {
		version = 0x00020000
	}

	header := &postEnc{
		Version:            version,
		ItalicAngle:        int32(math.Round(info.ItalicAngle * 65536)),
		UnderlinePosition:  info.UnderlinePosition,
		UnderlineThickness: info.UnderlineThickness,
	}
	if info.IsFixedPitch {
		header.IsFixedPitch = 1
	}
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, header)

	if version == 0x00020000 {
		numGlyphs := len(info.Names)
		buf.Write([]byte{byte(numGlyphs >> 8), byte(numGlyphs)})

		mac := make(map[string]int, len(macRoman))
		for i, name := range macRoman {
			mac[name] = i
		}
		var stringData []byte
		numStrings := 0

		for _, name := range info.Names {
			idx, ok := mac[name]
			if !ok {
				idx = len(macRoman) + numStrings
				stringData = append(stringData, byte(len(name)))
				stringData = append(stringData, name...)
				numStrings++
			}
			buf.Write([]byte{byte(idx >> 8), byte(idx)})
		}
		buf.Write(stringData)
	}

	return buf.Bytes()
}

type postEnc struct {
	Version            uint32
	ItalicAngle        int32
	UnderlinePosition  funit.Int16
	UnderlineThickness funit.Int16
	IsFixedPitch       uint32
	MinMemType42       uint32
	MaxMemType42       uint32
	MinMemType1        uint32
	MaxMemType1        uint32
}
