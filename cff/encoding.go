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

package cff

import (
	"fmt"
	"io"

	"seehuhn.de/go/postscript/psenc"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

func readEncoding(p *parser.Parser, charset []int32) ([]glyph.ID, error) {
	format, err := p.ReadUint8()
	if err != nil {
		return nil, err
	}

	res := make([]glyph.ID, 256)
	currentGid := glyph.ID(1)
	switch format & 127 {
	case 0:
		nCodes, err := p.ReadUint8()
		if err != nil {
			return nil, err
		}
		if int(nCodes) >= len(charset) {
			return nil, invalidSince("format 0 encoding too long")
		}
		codes := make([]byte, nCodes)
		_, err = io.ReadFull(p, codes)
		if err != nil {
			return nil, err
		}
		for _, c := range codes {
			if res[c] != 0 {
				return nil, invalidSince("invalid format 0 encoding")
			}
			res[c] = currentGid
			currentGid++
		}
	case 1:
		nRanges, err := p.ReadUint8()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(nRanges); i++ {
			first, err := p.ReadUint8()
			if err != nil {
				return nil, err
			}
			nLeft, err := p.ReadUint8()
			if err != nil {
				return nil, err
			}
			if int(first)+int(nLeft) > 255 {
				return nil, invalidSince("invalid format 1 encoding")
			}
			for j := int(first); j <= int(first+nLeft); j++ {
				if int(currentGid) >= len(charset) {
					return nil, invalidSince("format 1 encoding too long")
				} else if res[j] != 0 {
					return nil, invalidSince("invalid format 1 encoding")
				}
				res[j] = currentGid
				currentGid++
			}
		}
	default:
		return nil, unsupported(fmt.Sprintf("encoding format %d", format&127))
	}

	if (format & 128) != 0 {
		lookup := make(map[uint16]glyph.ID)
		for gid, sid := range charset {
			lookup[uint16(sid)] = glyph.ID(gid)
		}
		nSups, err := p.ReadUint8()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(nSups); i++ {
			code, err := p.ReadUint8()
			if err != nil {
				return nil, err
			} else if res[code] != 0 {
				return nil, invalidSince("invalid encoding supplement")
			}
			sid, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			gid := lookup[sid]
			if gid >= currentGid {
				return nil, invalidSince("invalid encoding supplement")
			}
			if gid != 0 {
				res[code] = gid
			}
		}
	}

	return res, nil
}

// encodeEncoding creates the CFF binary representation of an encoding vector.
// The encoding vector is given as a slice of glyph IDs (length 256),
// where the glyph ID 0 is used to indicate a missing glyph.
//
// The encoded glyphs must form a contiguous range starting at glyph ID 1.
//
// The glyphNames argument is only used for supplemented encodings, where
// one or more glyphs have multiple codes.
func encodeEncoding(encoding []glyph.ID, glyphNames []int32) ([]byte, error) {
	var maxGid glyph.ID
	codes := map[glyph.ID]uint8{}
	type suppl struct {
		code uint8
		gid  glyph.ID
	}
	var extra []suppl
	for code, gid := range encoding {
		if gid == 0 {
			continue
		}
		c8 := uint8(code)
		if _, ok := codes[gid]; ok {
			extra = append(extra, suppl{c8, gid})
			continue
		}
		codes[gid] = c8
		if gid > maxGid {
			maxGid = gid
		}
	}

	type seg struct {
		firstCode uint8
		nLeft     uint8
	}
	var ss []seg

	startGid := glyph.ID(1)
	startCode := codes[startGid]
	for gid := glyph.ID(1); gid <= maxGid; gid++ {
		code, ok := codes[gid]
		if !ok {
			msg := fmt.Sprintf("encoded glyphs not contiguous (glyph %d not encoded)", gid)
			return nil, invalidSince(msg)
		}
		if int(gid-startGid) != int(code)-int(startCode) {
			ss = append(ss, seg{startCode, uint8(gid - startGid - 1)})
			startGid = gid
			startCode = code
		}
	}
	ss = append(ss, seg{startCode, uint8(maxGid - startGid)})
	if len(ss) > 255 {
		return nil, invalidSince("too many segments")
	}

	format0Len := 2 + int(maxGid)
	format1Len := 2 + len(ss)*2
	extraLen := 0
	if len(extra) > 0 {
		extraLen = 1 + 3*len(extra)
	}
	var buf []byte
	var extraBase int
	if format0Len <= format1Len && maxGid <= 255 {
		extraBase = format0Len
		buf = make([]byte, format0Len+extraLen)
		// buf[0] = 0
		buf[1] = byte(maxGid)
		for i := glyph.ID(1); i <= maxGid; i++ {
			buf[i+1] = codes[i]
		}
	} else {
		extraBase = format1Len
		buf = make([]byte, format1Len+extraLen)
		buf[0] = 1
		buf[1] = byte(len(ss))
		for i, s := range ss {
			buf[2+i*2] = s.firstCode
			buf[2+i*2+1] = s.nLeft
		}
	}

	if len(extra) > 0 {
		buf[0] |= 128
		buf[extraBase] = byte(len(extra))
		for i, s := range extra {
			buf[extraBase+i*3+1] = s.code
			sid := uint16(glyphNames[s.gid])
			buf[extraBase+i*3+2] = byte(sid >> 8)
			buf[extraBase+i*3+3] = byte(sid)
		}
	}

	return buf, nil
}

// StandardEncoding returns the encoding vector for the standard encoding.
// The result can be used for the `Outlines.Encoding` field.
func StandardEncoding(glyphs []*Glyph) []glyph.ID {
	encoding := make([]glyph.ID, 256)
	for gid, g := range glyphs {
		code, ok := psenc.StandardEncodingRev[g.Name]
		if ok {
			encoding[code] = glyph.ID(gid)
		}
	}
	return encoding
}

func isStandardEncoding(encoding []glyph.ID, glyphs []*Glyph) bool {
	tmp := StandardEncoding(glyphs)
	for i, gid := range tmp {
		if encoding[i] != gid {
			return false
		}
	}
	return true
}

func expertEncoding(glyphs []*Glyph) []glyph.ID {
	res := make([]glyph.ID, 256)
	for gid, g := range glyphs {
		code, ok := expertEnc[g.Name]
		if ok {
			res[code] = glyph.ID(gid)
		}
	}
	return res
}

func isExpertEncoding(encoding []glyph.ID, glyphs []*Glyph) bool {
	tmp := expertEncoding(glyphs)
	for i, gid := range tmp {
		if encoding[i] != gid {
			return false
		}
	}
	return true
}

// expertEnc is the expert encoding for Type 1 fonts.
var expertEnc = map[string]byte{
	"space":             32,
	"exclamsmall":       33,
	"Hungarumlautsmall": 34,

	"dollaroldstyle":      36,
	"dollarsuperior":      37,
	"ampersandsmall":      38,
	"Acutesmall":          39,
	"parenleftsuperior":   40,
	"parenrightsuperior":  41,
	"twodotenleader":      42,
	"onedotenleader":      43,
	"comma":               44,
	"hyphen":              45,
	"period":              46,
	"fraction":            47,
	"zerooldstyle":        48,
	"oneoldstyle":         49,
	"twooldstyle":         50,
	"threeoldstyle":       51,
	"fouroldstyle":        52,
	"fiveoldstyle":        53,
	"sixoldstyle":         54,
	"sevenoldstyle":       55,
	"eightoldstyle":       56,
	"nineoldstyle":        57,
	"colon":               58,
	"semicolon":           59,
	"commasuperior":       60,
	"threequartersemdash": 61,
	"periodsuperior":      62,
	"questionsmall":       63,

	"asuperior":    65,
	"bsuperior":    66,
	"centsuperior": 67,
	"dsuperior":    68,
	"esuperior":    69,

	"isuperior": 73,

	"lsuperior": 76,
	"msuperior": 77,
	"nsuperior": 78,
	"osuperior": 79,

	"rsuperior": 82,
	"ssuperior": 83,
	"tsuperior": 84,

	"ff":                86,
	"fi":                87,
	"fl":                88,
	"ffi":               89,
	"ffl":               90,
	"parenleftinferior": 91,

	"parenrightinferior": 93,
	"Circumflexsmall":    94,
	"hyphensuperior":     95,
	"Gravesmall":         96,
	"Asmall":             97,
	"Bsmall":             98,
	"Csmall":             99,
	"Dsmall":             100,
	"Esmall":             101,
	"Fsmall":             102,
	"Gsmall":             103,
	"Hsmall":             104,
	"Ismall":             105,
	"Jsmall":             106,
	"Ksmall":             107,
	"Lsmall":             108,
	"Msmall":             109,
	"Nsmall":             110,
	"Osmall":             111,
	"Psmall":             112,
	"Qsmall":             113,
	"Rsmall":             114,
	"Ssmall":             115,
	"Tsmall":             116,
	"Usmall":             117,
	"Vsmall":             118,
	"Wsmall":             119,
	"Xsmall":             120,
	"Ysmall":             121,
	"Zsmall":             122,
	"colonmonetary":      123,
	"onefitted":          124,
	"rupiah":             125,
	"Tildesmall":         126,

	"exclamdownsmall": 161,
	"centoldstyle":    162,
	"Lslashsmall":     163,

	"Scaronsmall":   166,
	"Zcaronsmall":   167,
	"Dieresissmall": 168,
	"Brevesmall":    169,
	"Caronsmall":    170,

	"Dotaccentsmall": 172,

	"Macronsmall": 175,

	"figuredash":     178,
	"hypheninferior": 179,

	"Ogoneksmall":  182,
	"Ringsmall":    183,
	"Cedillasmall": 184,

	"onequarter":        188,
	"onehalf":           189,
	"threequarters":     190,
	"questiondownsmall": 191,
	"oneeighth":         192,
	"threeeighths":      193,
	"fiveeighths":       194,
	"seveneighths":      195,
	"onethird":          196,
	"twothirds":         197,

	"zerosuperior":     200,
	"onesuperior":      201,
	"twosuperior":      202,
	"threesuperior":    203,
	"foursuperior":     204,
	"fivesuperior":     205,
	"sixsuperior":      206,
	"sevensuperior":    207,
	"eightsuperior":    208,
	"ninesuperior":     209,
	"zeroinferior":     210,
	"oneinferior":      211,
	"twoinferior":      212,
	"threeinferior":    213,
	"fourinferior":     214,
	"fiveinferior":     215,
	"sixinferior":      216,
	"seveninferior":    217,
	"eightinferior":    218,
	"nineinferior":     219,
	"centinferior":     220,
	"dollarinferior":   221,
	"periodinferior":   222,
	"commainferior":    223,
	"Agravesmall":      224,
	"Aacutesmall":      225,
	"Acircumflexsmall": 226,
	"Atildesmall":      227,
	"Adieresissmall":   228,
	"Aringsmall":       229,
	"AEsmall":          230,
	"Ccedillasmall":    231,
	"Egravesmall":      232,
	"Eacutesmall":      233,
	"Ecircumflexsmall": 234,
	"Edieresissmall":   235,
	"Igravesmall":      236,
	"Iacutesmall":      237,
	"Icircumflexsmall": 238,
	"Idieresissmall":   239,
	"Ethsmall":         240,
	"Ntildesmall":      241,
	"Ogravesmall":      242,
	"Oacutesmall":      243,
	"Ocircumflexsmall": 244,
	"Otildesmall":      245,
	"Odieresissmall":   246,
	"OEsmall":          247,
	"Oslashsmall":      248,
	"Ugravesmall":      249,
	"Uacutesmall":      250,
	"Ucircumflexsmall": 251,
	"Udieresissmall":   252,
	"Yacutesmall":      253,
	"Thornsmall":       254,
	"Ydieresissmall":   255,
}
