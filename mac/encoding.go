// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2021  Jochen Voss <voss@seehuhn.de>
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

// Package mac implements the Mac Roman encoding.
// This is the standard Roman encoding that is used for PlatformID==1,
// EncodingID == 0.  This is similar to the MacRomanEncoding in PDF, but adds
// 15 entries and replaces the currency glyph with the Euro glyph.
// https://en.wikipedia.org/wiki/Mac_OS_Roman
package mac

// TODO(voss): this seems to be duplicated at least two times:
//    mac/encoding.go
//    post/names.go
// There is also similar code in cff/charset.go.

// Decode decodes a string of MacRoman encoded bytes.
func Decode(cc []byte) string {
	rr := make([]rune, len(cc))
	for i, c := range cc {
		if c < 128 {
			rr[i] = rune(c)
		} else {
			rr[i] = dec[c-128]
		}
	}
	return string(rr)
}

// DecodeOne decodes a single MacRoman encoded byte.
func DecodeOne(c byte) rune {
	if c < 128 {
		return rune(c)
	}
	return dec[c-128]
}

// Encode encodes a string of Unicode runes.  Runes which cannot be represented
// in the MacRoman encoding are replaced by question marks.
func Encode(s string) []byte {
	rr := []rune(s)
	res := make([]byte, len(rr))
	for i, r := range rr {
		if r < 128 {
			res[i] = byte(r)
		} else {
			c, ok := enc[r]
			if !ok {
				c = '?'
			}
			res[i] = c
		}
	}
	return res
}

var dec = []rune{
	0x00c4, 0x00c5, 0x00c7, 0x00c9, 0x00d1, 0x00d6, 0x00dc, 0x00e1,
	0x00e0, 0x00e2, 0x00e4, 0x00e3, 0x00e5, 0x00e7, 0x00e9, 0x00e8,
	0x00ea, 0x00eb, 0x00ed, 0x00ec, 0x00ee, 0x00ef, 0x00f1, 0x00f3,
	0x00f2, 0x00f4, 0x00f6, 0x00f5, 0x00fa, 0x00f9, 0x00fb, 0x00fc,
	0x2020, 0x00b0, 0x00a2, 0x00a3, 0x00a7, 0x2022, 0x00b6, 0x00df,
	0x00ae, 0x00a9, 0x2122, 0x00b4, 0x00a8, 0x2260, 0x00c6, 0x00d8,
	0x221e, 0x00b1, 0x2264, 0x2265, 0x00a5, 0x00b5, 0x2202, 0x2211,
	0x220f, 0x03c0, 0x222b, 0x00aa, 0x00ba, 0x03a9, 0x00e6, 0x00f8,
	0x00bf, 0x00a1, 0x00ac, 0x221a, 0x0192, 0x2248, 0x2206, 0x00ab,
	0x00bb, 0x2026, 0x00a0, 0x00c0, 0x00c3, 0x00d5, 0x0152, 0x0153,
	0x2013, 0x2014, 0x201c, 0x201d, 0x2018, 0x2019, 0x00f7, 0x25ca,
	0x00ff, 0x0178, 0x2044, 0x20ac, 0x2039, 0x203a, 0xfb01, 0xfb02,
	0x2021, 0x00b7, 0x201a, 0x201e, 0x2030, 0x00c2, 0x00ca, 0x00c1,
	0x00cb, 0x00c8, 0x00cd, 0x00ce, 0x00cf, 0x00cc, 0x00d3, 0x00d4,
	0xf8ff, 0x00d2, 0x00da, 0x00db, 0x00d9, 0x0131, 0x02c6, 0x02dc,
	0x00af, 0x02d8, 0x02d9, 0x02da, 0x00b8, 0x02dd, 0x02db, 0x02c7,
}

var enc = map[rune]byte{
	0x00a0: 202,
	0x00a1: 193,
	0x00a2: 162,
	0x00a3: 163,
	0x00a5: 180,
	0x00a7: 164,
	0x00a8: 172,
	0x00a9: 169,
	0x00aa: 187,
	0x00ab: 199,
	0x00ac: 194,
	0x00ae: 168,
	0x00af: 248,
	0x00b0: 161,
	0x00b1: 177,
	0x00b4: 171,
	0x00b5: 181,
	0x00b6: 166,
	0x00b7: 225,
	0x00b8: 252,
	0x00ba: 188,
	0x00bb: 200,
	0x00bf: 192,
	0x00c0: 203,
	0x00c1: 231,
	0x00c2: 229,
	0x00c3: 204,
	0x00c4: 128,
	0x00c5: 129,
	0x00c6: 174,
	0x00c7: 130,
	0x00c8: 233,
	0x00c9: 131,
	0x00ca: 230,
	0x00cb: 232,
	0x00cc: 237,
	0x00cd: 234,
	0x00ce: 235,
	0x00cf: 236,
	0x00d1: 132,
	0x00d2: 241,
	0x00d3: 238,
	0x00d4: 239,
	0x00d5: 205,
	0x00d6: 133,
	0x00d8: 175,
	0x00d9: 244,
	0x00da: 242,
	0x00db: 243,
	0x00dc: 134,
	0x00df: 167,
	0x00e0: 136,
	0x00e1: 135,
	0x00e2: 137,
	0x00e3: 139,
	0x00e4: 138,
	0x00e5: 140,
	0x00e6: 190,
	0x00e7: 141,
	0x00e8: 143,
	0x00e9: 142,
	0x00ea: 144,
	0x00eb: 145,
	0x00ec: 147,
	0x00ed: 146,
	0x00ee: 148,
	0x00ef: 149,
	0x00f1: 150,
	0x00f2: 152,
	0x00f3: 151,
	0x00f4: 153,
	0x00f5: 155,
	0x00f6: 154,
	0x00f7: 214,
	0x00f8: 191,
	0x00f9: 157,
	0x00fa: 156,
	0x00fb: 158,
	0x00fc: 159,
	0x00ff: 216,
	0x0131: 245,
	0x0152: 206,
	0x0153: 207,
	0x0178: 217,
	0x0192: 196,
	0x02c6: 246,
	0x02c7: 255,
	0x02d8: 249,
	0x02d9: 250,
	0x02da: 251,
	0x02db: 254,
	0x02dc: 247,
	0x02dd: 253,
	0x03a9: 189,
	0x03c0: 185,
	0x2013: 208,
	0x2014: 209,
	0x2018: 212,
	0x2019: 213,
	0x201a: 226,
	0x201c: 210,
	0x201d: 211,
	0x201e: 227,
	0x2020: 160,
	0x2021: 224,
	0x2022: 165,
	0x2026: 201,
	0x2030: 228,
	0x2039: 220,
	0x203a: 221,
	0x2044: 218,
	0x20ac: 219,
	0x2122: 170,
	0x2202: 182,
	0x2206: 198,
	0x220f: 184,
	0x2211: 183,
	0x221a: 195,
	0x221e: 176,
	0x222b: 186,
	0x2248: 197,
	0x2260: 173,
	0x2264: 178,
	0x2265: 179,
	0x25ca: 215,
	0xf8ff: 240,
	0xfb01: 222,
	0xfb02: 223,
}
