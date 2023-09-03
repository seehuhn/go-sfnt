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

package cff

import (
	"fmt"
	"math"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// Read reads a CFF font from r.
func Read(r parser.ReadSeekSizer) (*Font, error) {
	cff := &Font{
		Outlines: &Outlines{},
	}

	p := parser.New(r)

	// section 0: header
	x, err := p.ReadUint32()
	if err != nil {
		return nil, err
	}
	major := x >> 24
	minor := (x >> 16) & 0xFF
	nameIndexOffs := int64((x >> 8) & 0xFF)
	offSize := x & 0xFF // only used to exclude non-CFF files
	if major == 2 {
		return nil, unsupported(fmt.Sprintf("version %d.%d", major, minor))
	} else if major != 1 || nameIndexOffs < 4 || offSize > 4 {
		return nil, invalidSince("invalid header")
	}

	// section 1: Name INDEX
	err = p.SeekPos(nameIndexOffs)
	if err != nil {
		return nil, err
	}
	fontNames, err := readIndex(p)
	if err != nil {
		return nil, err
	}
	if len(fontNames) == 0 {
		return nil, invalidSince("no font data")
	} else if len(fontNames) > 1 {
		return nil, unsupported("fontsets with more than one font")
	}
	cff.FontInfo = &type1.FontInfo{
		FontName: string(fontNames[0]),
	}

	// section 2: top DICT INDEX
	topDictIndex, err := readIndex(p)
	if err != nil {
		return nil, err
	}
	if len(topDictIndex) != len(fontNames) {
		return nil, invalidSince("wrong number of top dicts")
	}

	// section 3: String INDEX
	stringIndex, err := readIndex(p)
	if err != nil {
		return nil, err
	}
	strings := &cffStrings{
		data: make([]string, len(stringIndex)),
	}
	for i, s := range stringIndex {
		strings.data[i] = string(s)
	}

	// interlude: decode the top DICT
	topDict, err := decodeDict(topDictIndex[0], strings)
	if err != nil {
		return nil, err
	}
	if topDict.getInt(opCharstringType, 2) != 2 {
		return nil, unsupported("charstring type != 2")
	}
	cff.FontInfo.Version = topDict.getString(opVersion)
	cff.FontInfo.Notice = topDict.getString(opNotice)
	cff.FontInfo.Copyright = topDict.getString(opCopyright)
	cff.FontInfo.FullName = topDict.getString(opFullName)
	cff.FontInfo.FamilyName = topDict.getString(opFamilyName)
	cff.FontInfo.Weight = topDict.getString(opWeight)
	isFixedPitch := topDict.getInt(opIsFixedPitch, 0)
	cff.FontInfo.IsFixedPitch = isFixedPitch != 0
	italicAngle := topDict.getFloat(opItalicAngle, 0)
	cff.FontInfo.ItalicAngle = normaliseAngle(italicAngle)
	// TODO(voss): change underline parameters to reals.
	cff.FontInfo.UnderlinePosition = funit.Float64(topDict.getInt(opUnderlinePosition,
		defaultUnderlinePosition))
	cff.FontInfo.UnderlineThickness = funit.Float64(topDict.getInt(opUnderlineThickness,
		defaultUnderlineThickness))

	// TODO(voss): different default for CIDFonts?
	cff.FontInfo.FontMatrix = topDict.getFontMatrix(opFontMatrix)

	// section 4: global subr INDEX
	gsubrs, err := readIndex(p)
	if err != nil {
		return nil, err
	}

	// section 5: encodings
	// read below, once we know the charset

	// read the CharStrings INDEX
	charStringsOffs := topDict.getInt(opCharStrings, 0)
	charStrings, err := readIndexAt(p, charStringsOffs, "CharStrings")
	nGlyphs := len(charStrings)
	if err != nil {
		return nil, err
	} else if nGlyphs == 0 {
		return nil, invalidSince("no charstrings")
	}

	ROS, isCIDFont := topDict[opROS]
	var decoders []*decodeInfo
	if isCIDFont {
		if len(ROS) != 3 {
			return nil, invalidSince("wrong number of ROS values")
		}
		ros := &type1.CIDSystemInfo{}
		if reg, ok := ROS[0].(string); ok {
			ros.Registry = reg
		} else {
			return nil, invalidSince("wrong type for Registry")
		}
		if ord, ok := ROS[1].(string); ok {
			ros.Ordering = ord
		} else {
			return nil, invalidSince("wrong type for Ordering")
		}
		if sup, ok := ROS[2].(int32); ok {
			ros.Supplement = sup
		} else {
			return nil, invalidSince("wrong type for Supplement")
		}
		cff.ROS = ros

		fdArrayOffs := topDict.getInt(opFDArray, 0)
		fdArrayIndex, err := readIndexAt(p, fdArrayOffs, "Font DICT")
		if err != nil {
			return nil, err
		} else if len(fdArrayIndex) > 256 {
			return nil, invalidSince("too many Font DICTs")
		} else if len(fdArrayIndex) == 0 {
			return nil, invalidSince("no Font DICTs")
		}
		for _, fdBlob := range fdArrayIndex {
			fontDict, err := decodeDict(fdBlob, strings)
			if err != nil {
				return nil, err
			}
			pInfo, err := fontDict.readPrivate(p, strings)
			if err != nil {
				return nil, err
			}
			cff.Private = append(cff.Private, pInfo.private)
			decoders = append(decoders, &decodeInfo{
				subr:         pInfo.subrs,
				gsubr:        gsubrs,
				defaultWidth: pInfo.defaultWidth,
				nominalWidth: pInfo.nominalWidth,
			})
		}

		fdSelectOffs := topDict.getInt(opFDSelect, 0)
		if fdSelectOffs < 4 {
			return nil, invalidSince("missing FDSelect")
		}
		err = p.SeekPos(int64(fdSelectOffs))
		if err != nil {
			return nil, err
		}
		cff.FDSelect, err = readFDSelect(p, nGlyphs, len(cff.Private))
		if err != nil {
			return nil, err
		}
	} else {
		cff.FDSelect = func(gid glyph.ID) int { return 0 }
	}

	// read the list of glyph names
	charsetOffs := topDict.getInt(opCharset, 0)
	var charset []int32
	if isCIDFont {
		err = p.SeekPos(int64(charsetOffs))
		if err != nil {
			return nil, err
		}
		charset, err = readCharset(p, nGlyphs)
		if err != nil {
			return nil, err
		}
		cff.GIDToCID = make([]type1.CID, nGlyphs) // filled in below
	} else {
		switch charsetOffs {
		case 0: // ISOAdobe charset
			if nGlyphs > len(isoAdobeCharset) {
				return nil, invalidSince("invalid charset")
			}
			charset = make([]int32, nGlyphs)
			for i := range charset {
				charset[i] = strings.lookup(isoAdobeCharset[i])
			}
		case 1: // Expert charset
			if nGlyphs > len(expertCharset) {
				return nil, invalidSince("invalid charset")
			}
			charset = make([]int32, nGlyphs)
			for i := range charset {
				charset[i] = strings.lookup(expertCharset[i])
			}
		case 2: // ExpertSubset charset
			if nGlyphs > len(expertSubsetCharset) {
				return nil, invalidSince("invalid charset")
			}
			charset = make([]int32, nGlyphs)
			for i := range charset {
				charset[i] = strings.lookup(expertSubsetCharset[i])
			}
		default:
			err = p.SeekPos(int64(charsetOffs))
			if err != nil {
				return nil, err
			}
			charset, err = readCharset(p, nGlyphs)
			if err != nil {
				return nil, err
			}
		}
	}

	// read the Private DICT
	if !isCIDFont {
		pInfo, err := topDict.readPrivate(p, strings)
		if err != nil {
			return nil, err
		}
		cff.Private = []*type1.PrivateDict{pInfo.private}
		decoders = append(decoders, &decodeInfo{
			subr:         pInfo.subrs,
			gsubr:        gsubrs,
			defaultWidth: pInfo.defaultWidth,
			nominalWidth: pInfo.nominalWidth,
		})
	}

	cff.Glyphs = make([]*Glyph, nGlyphs)
	fdSelect := cff.FDSelect
	for gid, code := range charStrings {
		fdIdx := fdSelect(glyph.ID(gid))
		info := decoders[fdIdx]

		glyph, err := info.decodeCharString(code)
		if err != nil {
			return nil, err
		}
		if isCIDFont {
			if charset != nil {
				cff.GIDToCID[gid] = type1.CID(charset[gid])
			}
		} else {
			name, err := strings.get(charset[gid])
			if err != nil {
				return nil, err
			}
			glyph.Name = name
		}
		cff.Glyphs[gid] = glyph
	}

	// read the encoding
	if !isCIDFont {
		encodingOffs := topDict.getInt(opEncoding, 0)
		var enc []glyph.ID
		switch {
		case encodingOffs == 0:
			enc = StandardEncoding(cff.Glyphs)
		case encodingOffs == 1:
			enc = expertEncoding(cff.Glyphs)
		default:
			err = p.SeekPos(int64(encodingOffs))
			if err != nil {
				return nil, err
			}
			enc, err = readEncoding(p, charset)
			if err != nil {
				return nil, err
			}
		}
		cff.Encoding = enc
	}

	return cff, nil
}

func normaliseAngle(x float64) float64 {
	y := math.Mod(x+180, 360)
	if y < 0 {
		y += 360
	}
	return y - 180
}
