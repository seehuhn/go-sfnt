// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2024  Jochen Voss <voss@seehuhn.de>
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

package testcases

type GsubTestCase struct {
	Desc    string
	In, Out string
	Text    string // text content, if different from `in`
}

// Gsub lists test cases for the GSUB table.
//
// My overall aim is to replicate the behaviour of ligatures in MS Word, but so
// far I have failed to reverse engineer the complete set of rules for this.
// For comparison, I have also included the behaviour of harfbuzz and of the
// MacOS layout engine.
var Gsub = []*GsubTestCase{
	{ // test0001.odf
		Desc: "GSUB1: A->X, C->Z",
		In:   "ABC",
		Out:  "XBZ",
	},
	{
		Desc: "GSUB1: A->B, B->A",
		In:   "ABC",
		Out:  "BAC",
	},
	{
		Desc: "GSUB1: -base A->B, B->A",
		In:   "ABC",
		Out:  "BBC",
	},
	{
		Desc: "GSUB1: -marks A->B, M->N",
		In:   "AAMBA",
		Out:  "BBMBB",
	},

	{
		Desc: `GSUB2: A->A A`,
		In:   "AA",
		Out:  "AAAA",
	},
	{
		Desc: `GSUB2: -marks A -> "ABA", M -> A`,
		In:   "ABMA",
		Out:  "ABABMABA",
	},

	{
		Desc: `GSUB3: A -> [B C D]`,
		In:   "AB",
		Out:  "BB",
	},
	{
		Desc: `GSUB3: -marks A -> [B C], M -> [B C]`,
		In:   "AM",
		Out:  "BM",
	},
	{
		Desc: `GSUB3: A -> []`,
		In:   "AB",
		Out:  "AB",
	},

	{
		Desc: `GSUB4: "BA" -> B`,
		In:   "ABAABA",
		Out:  "ABAB",
	},
	{
		Desc: `GSUB4: "AAA" -> "B", "AA" -> "C", "A" -> "D"`,
		In:   "AAAAAXA",
		Out:  "BCXD",
	},
	{
		Desc: `GSUB4: -marks "AAA" -> "X"`,
		In:   "AAABMAAACAMAADAAMAEAAAM",
		Out:  "XBMXCXMDXMEXM",
		Text: "AAABMAAACAAAMDAAAMEAAAM",
	},
	{
		Desc: `GSUB4: -marks "AAA" -> "C", "AA" -> "B"`,
		In:   "AAAMAMAMAAA",
		Out:  "CMCMMB",
		Text: "AAAMAAAMMAA",
	},

	{
		Desc: `GSUB5: "AAA" -> 3@2 1@0 2@1
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAA",
		Out: "XYZ",
	},
	{ // test0011.odf
		Desc: `GSUB5: "XXX" -> 1@0
				GSUB1: "X" -> "A"`,
		In:  "XXXXXXXX",
		Out: "AXXAXXXX",
	},
	{
		Desc: `GSUB5: "ABC" -> 1@0 1@1 1@2
				GSUB1: "B" -> "X"`,
		In:  "ABC",
		Out: "AXC",
	},
	{ // harfbuzz, Mac, Windows: XYZ
		Desc: `GSUB5: "AAAA" -> 1@0 4@2 3@1 2@0
				GSUB4: "AA" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAAA",
		Out: "XYZ",
	},
	{ // harfbuzz, Mac, Windows: XYZA
		Desc: `GSUB5: "AAA" -> 1@0 4@2 3@1 2@0
				GSUB2: "A" -> "AA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAA",
		Out: "XYZA",
	},
	{ // harfbuzz, Mac, Windows: XYZA
		Desc: `GSUB5: "AAA" -> 1@0 5@2 4@1 3@0
				GSUB5: "AA" -> 2@1
				GSUB2: "A" -> "AA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAA",
		Out: "XYZA",
	},

	//
	// ------------------------------------------------------------------
	// Testing glyph positions in recursive lookups, in particular when the
	// sequence length changes:
	//
	{ // harfbuzz, Mac, Windows: XA
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "X"`,
		In:  "AA",
		Out: "XA",
	},
	{ // harfbuzz, Mac, Windows: XA
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB2: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "X"`,
		In:  "AA",
		Out: "XA",
	},
	{ // harfbuzz, Mac, Windows: XA
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB4: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "X"`,
		In:  "AA",
		Out: "XA",
	},
	//
	// The same, but with one more level of nesting.
	{
		Desc: `GSUB5: "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "X"`,
		In:  "AA",
		Out: "XA",
	},
	{
		Desc: `GSUB5: "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB2: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "X"`,
		In:  "AA",
		Out: "XA",
	},
	{
		Desc: `GSUB5: "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB4: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "X"`,
		In:  "AA",
		Out: "XA",
	},
	//
	// ... and with ligatures ignored
	{
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "X"`,
		In:  "ALA",
		Out: "XLA",
	},
	{
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@0
				GSUB5: "AL" -> 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "X"`,
		In:  "ALA",
		Out: "XLA",
	},
	{ // harfbuzz, Mac, Windows:
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@1
				GSUB5: "AL" -> 2@0
				GSUB2: "A" -> "BB"
				GSUB1: "A" -> "Y", "B" -> "Y"`,
		In:  "ALA",
		Out: "BYLA",
	},

	// Check under which circumstances new glyphs are added to the
	// input sequence.

	// ------------------------------------------------------------------
	// single glyphs:
	//   A: yes
	//   M: no

	// We have seen above that if a normal glyph is replaced, the
	// replacement IS added to the input sequence.

	// If a single ignored glyph is replaced, the replacement is NOT added
	// to the input sequence:
	{ // ALA -> ABA -> ABX
		// harfbuzz, Mac, Windows: ABX
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@1
				GSUB5: "ALA" -> 2@1
				GSUB4: "L" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "ABX",
	},
	{ // ALA -> ABBA -> ...
		// harfbuzz: AYBA, Mac: ABYA, Windows: ABBX
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@1
				GSUB5: "AL" -> 2@1
				GSUB2: "L" -> "BB"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "ABBX",
	},

	// ------------------------------------------------------------------
	// pairs:
	//   AA: yes
	//   MA: yes
	//   AM: mixed????
	//   MM: no

	// When a pair of normal glyphs is replaced, the replacement IS added.
	{ // AAA -> AB -> ...
		// harfbuzz, Mac: AY; Windows:
		Desc: `GSUB5: -ligs "AAA" -> 1@0 3@1
				GSUB5: "AAA" -> 2@1
				GSUB4: "AA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAA",
		Out: "AY",
	},

	{ // ALA -> AB -> ...
		// harfbuzz, Mac: AB, Windows: AY -> yes
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@1
				GSUB5: "ALA" -> 2@1
				GSUB4: "LA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "AY",
	},

	// normal+ignored
	{ // harfbuzz, Mac, Windows: YAA -> yes
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AM" -> 2@0
				GSUB4: "AM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "YAA",
	},
	// { // harfbuzz: AXA, Mac: AXA, Windows: AAX -> no ????????????????????
	// 	desc: `GSUB5: -marks "AAA" -> 1@1 2@1
	// 			GSUB4: "AM" -> "A"
	// 			GSUB1: "A" -> "X"`,
	// 	in:  "AAMA",
	// 	out: "AAX",
	// },
	// { // harfbuzz: AXA, Mac: AXA, Windows: AAX -> no ????????????????????
	// 	desc: `GSUB5: -marks "ALA" -> 1@1 2@1
	// 			GSUB4: "LM" -> "A"
	// 			GSUB1: "A" -> "X"`,
	// 	in:  "ALMA",
	// 	out: "AAX",
	// },

	// When a pair of ignored glyphs is replaced, the replacement is NOT
	// added.
	{ // ALLA -> ABA -> ...
		// harfbuzz: ABA, Mac: ABX, Windows: ABX -> no
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@1
				GSUB5: "ALL" -> 2@1
				GSUB4: "LL" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALLA",
		Out: "ABX",
	},

	// ------------------------------------------------------------------
	// triples:
	//   AAA: (I assume yes)
	//   AAM: yes
	//   AMA: yes
	//   AMM: yes
	//   MAA: yes
	//   MAM: no
	//   MMA: yes
	//   MMM: no

	{ // harfbuzz: YA, Mac: YA, Windows: YA -> included
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AAM" -> 2@0
				GSUB4: "AAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMA",
		Out: "YA",
	},

	{ // harfbuzz: YA, Mac: YA, Windows: YA -> included
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMA" -> 2@0
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "YA",
	},
	{ // harfbuzz: ABA, Mac: AYA, Windows: AYA -> included
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AAMA" -> 2@1
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMAA",
		Out: "AYA",
	},
	{ // harfbuzz: ABX, Mac: AYA, Windows: AYA -> included
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@1
				GSUB5: "AAMA" -> 2@1
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMAA",
		Out: "AYA",
	},

	{ // harfbuzz: YAA, Mac: YAA, Windows: YAA -> included
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMM" -> 2@0
				GSUB4: "AMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAA",
		Out: "YAA",
	},

	{ // harfbuzz: AB, Mac: AB, Windows: AY -> included
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAA" -> 2@1
				GSUB4: "MAA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "AY",
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: ABX -> not included
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAM" -> 2@1
				GSUB4: "MAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "ABX",
	},
	{ // ALALA -> ABA -> ...
		// harfbuzz: ABA, Mac: ABX, Windows: ABX -> not included
		Desc: `GSUB5: -ligs "AAA" -> 1@0 3@1
				GSUB5: "ALALA" -> 2@1
				GSUB4: "LAL" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALALA",
		Out: "ABX",
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: AYA -> included
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMMA" -> 2@1
				GSUB4: "MMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAA",
		Out: "AYA",
	},

	{ // harfbuzz: ABAA; Mac, Windows: ABXA -> not included
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMMM" -> 2@1
				GSUB4: "MMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMMAA",
		Out: "ABXA",
	},

	// XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX

	// sequences of length 4
	//   AAAA -> (yes, I guess)
	//   AAAM
	//   AAMA
	//   AAMM
	//   AMAA
	//   AMAM yes
	//   AMMA
	//   AMMM yes
	//   MAAA
	//   MAAM no
	//   MAMA yes
	//   MAMM no
	//   MMAA
	//   MMAM no
	//   MMMA
	//   MMMM -> (no, I guess)

	{ // harfbuzz: , Mac: YA, Windows: YA -> yes
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMAM" -> 2@0
				GSUB4: "AMAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "YA",
	},

	{ // harfbuzz: , Mac: YAA, Windows: YAA -> yes
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMMM" -> 2@0
				GSUB4: "AMMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMMAA",
		Out: "YAA",
	},

	{ // harfbuzz: , Mac: ABX, Windows: ABX -> no
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@1
				GSUB5: "AMAAM" -> 2@1
				GSUB4: "MAAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAAMA",
		Out: "ABX",
	},

	{ // harfbuzz: , Mac: AB, Windows: AY -> yes
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAMA" -> 2@1
				GSUB4: "MAMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "AY",
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: ABX -> no
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAMM" -> 2@1
				GSUB4: "MAMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMMA",
		Out: "ABX",
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: ABX -> no
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMMAM" -> 2@1
				GSUB4: "MMAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAMA",
		Out: "ABX",
	},

	// XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX

	// The difference between the following two cases is mysterious to me.

	{ // harfbuzz: YAA, Mac: YAA, Windows: YAA -> yes
		Desc: `GSUB5: -marks "AAA" -> 1@0 2@0
				GSUB4: "AM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "YAA",
	},
	// { // harfbuzz: AYA, Mac: AYA, Windows: ABX -> no ????????????????????
	// 	desc: `GSUB5: -marks "AAA" -> 1@1 2@1
	// 			GSUB4: "AM" -> "B"
	// 			GSUB1: "A" -> "X", "B" -> "Y"`,
	// 	in:  "AAMA",
	// 	out: "ABX",
	// },

	// XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX

	// longer:
	//   MMMAAA -> yes
	//   MMAMAA -> yes

	{ // harfbuzz: ABA, Mac: ABX, Windows: AYA -> included
		Desc: `GSUB5: -marks "AAAAA" -> 1@0 3@1
				GSUB5: "AMMMAAA" -> 2@1
				GSUB4: "MMMAAA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMMAAAA",
		Out: "AYA",
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: AYA -> included
		Desc: `GSUB5: -marks "AAAAA" -> 1@0 3@1
				GSUB5: "AMMAMAA" -> 2@1
				GSUB4: "MMAMAA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAMAAA",
		Out: "AYA",
	},

	// XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX

	// {
	// 	// ALMA -> AAA -> ...
	// 	// harfbuzz: AXA, Mac: AXY, Windows: AAY ????????????????????????
	// 	desc: `GSUB5: -ligs -marks "AA" -> 1@0 4@1
	// 			GSUB5: -marks "ALA" -> 2@1 3@1
	// 			GSUB4: "LM" -> "A"
	// 			GSUB1: "A" -> "X"
	// 			GSUB1: "A" -> "Y", "X" -> "Y"`,
	// 	in:  "ALMA",
	// 	out: "AAY",
	// },

	{ // harfbuzz: DEI, Mac: DEI, Windows: DEI
		Desc: `GSUB5: "ABC" -> 1@0 1@2 1@3 2@2
				GSUB2: "A" -> "DE", "B" -> "FG", "G" -> "H"
				GSUB4: "FHC" -> "I"`,
		In:  "ABC",
		Out: "DEI", // ABC -> DEBC -> DEFGC -> DEFHC -> DEI
	},
	{ // harfbuzz: AXAAKA, Mac: AAXAKA, Windows: AAAXKA
		Desc: `GSUB5: -ligs "AAA" -> 1@0 2@1 3@1
				GSUB5: "AK" -> 2@1
				GSUB2: "K" -> "AA"
				GSUB1: "A" -> "X", "K" -> "X", "L" -> "X"`,
		In:  "AKAKA",
		Out: "AAAXKA",
	},
	{ //  harfbuzz: AXLAKA, Mac: ALXAKA, Windows: ALLXKA
		Desc: `GSUB5: -ligs "AAA" -> 1@0 2@1 3@1
				GSUB5: "AK" -> 2@1
				GSUB2: "K" -> "LL"
				GSUB1: "A" -> "X", "K" -> "X", "L" -> "X"`,
		In:  "AKAKA",
		Out: "ALLXKA",
	},
	{ // harfbuzz, Mac, Windows:
		Desc: `GSUB5: -ligs "AAA" -> 1@0 5@2 4@1 3@0
				GSUB5: "AL" -> 2@1
				GSUB1: "L" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "ALAA",
		Out: "XAYZ",
	},
	{ // harfbuzz, Mac, Windows:
		Desc: `GSUB5: "AB" -> 1@0 0@0, "AAB" -> 1@0 0@0, "AAAB" -> 1@0 0@0
				GSUB2: "A" -> "AA"`,
		In:  "AB",
		Out: "AAAAB",
	},
	{ // harfbuzz: XYAZA, Mac: XAYZA, Windows: XAAYZ
		Desc: `GSUB5: -ligs "AAA" -> 1@0 5@2 4@1 3@0
				GSUB5: "AL" -> 2@1
				GSUB2: "L" -> "AA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "ALAA",
		Out: "XAAYZ",
	},
	{ // harfbuzz: XLYZ, Mac: XLYZ, Windows: XLYZA
		Desc: `GSUB5: -ligs "AAAA" -> 1@0 5@2 4@1 3@0
				GSUB5: "AL" -> 2@1
				GSUB4: "LA" -> "L"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "ALAAA",
		Out: "XLYZA",
	},
	{ // harfbuzz: AKA, Mac: AKA, Windows: ABAA
		Desc: `GSUB5: -ligs "AAA" -> 1@0
				GSUB5: "AL" -> 2@1 3@1
				GSUB4: "LA" -> "K"
				GSUB1: "L" -> "B"`,
		In:  "ALAA",
		Out: "ABAA",
	},
	{ // harfbuzz: LXLYLZL, Mac: LXLYLZL, Windows: LXLYLZL
		Desc: `GSUB5: -ligs "AAA" -> 3@2 1@0 2@1
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "LALALAL",
		Out: "LXLYLZL",
	},

	// { // harfbuzz: LXALX, Mac: LXXX, Windows: LXXAL ???????????????????????
	// 	Desc: `GSUB5: -ligs "AAA" -> 1@0 1@1 1@2
	// 			GSUB4: "AL" -> "X"`,
	// 	In:  "LALALAL",
	// 	Out: "LXXAL",
	// },
	// { // Mac: LALALX, Windows: LALALAL ????????????????????????????????????
	// 	desc: `GSUB5: -ligs "AAA" -> 1@2
	// 			GSUB4: "AL" -> "X"`,
	// 	in:  "LALALAL",
	// 	out: "LALALAL",
	// },
	{ // Mac, Windows: LALALXB -> "AL" WAS added to input here
		Desc: `GSUB5: -ligs "AAA" -> 1@2
				GSUB4: "AL" -> "X"`,
		In:  "LALALALB",
		Out: "LALALXB",
	},
	{ // Mac, Windows: ABXD -> "CL" was added to input here
		Desc: `GSUB5: -ligs "ABC" -> 1@2
			GSUB4: C L -> X`,
		In:  "ABCLD",
		Out: "ABXD",
	},

	{ // Mac, Windows: XCEAAAAXFBCAACX
		Desc: `GSUB5:
				"ACE" -> 1@0 ||
				class :AB: = [A B]
				class :CD: = [C D]
				class :EF: = [E F]
				/A B/ :AB: :CD: :EF: -> 1@1 ||
				class :AB: = [A B]
				/A/ :AB: :: :AB: -> 1@2
			GSUB1: A -> X, B -> X, C -> X, D -> X, E -> X, F -> X`,
		In:  "ACEAAAACFBCAACB",
		Out: "XCEAAAAXFBCAACX",
	},

	{ // Mac, Windows: XBCFBCXA
		Desc: `GSUB5:
				[A-E] [A B] [C-X] -> 1@0 ||
				[B-E] [B-E] [A-C] -> 1@1
			GSUB1: A -> X, B -> X, C -> X, D -> X, E -> X, F -> X`,
		In:  "ABCFBCEA",
		Out: "XBCFBCXA",
	},
	{ // harfbuzz, Mac, Windows: X
		Desc: `GSUB5: -ligs
				class :A: = [A]
				/A/ :A: -> 1@0
			GSUB4: A L -> X`,
		In:  "AL",
		Out: "X",
	},

	// lookup rules with context

	{
		Desc: `GSUB6: A B | C D | E F -> 1@0
			GSUB1: C -> X`,
		In:  "ABCDEF",
		Out: "ABXDEF",
	},
	{
		Desc: `GSUB6: A B | C D | E F -> 1@0
			GSUB1: C -> X`,
		In:  "ABCDE",
		Out: "ABCDE",
	},
	{
		Desc: `GSUB6: A B | C D | E F -> 1@0
			GSUB1: C -> X`,
		In:  "ABC",
		Out: "ABC",
	},
	{
		Desc: `GSUB6: A | A | A -> 1@0, X | A | A -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAAA",
		Out: "AXXXXA",
	},
	{
		Desc: `GSUB6: | A | A -> 1@0, | A | A -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAAA",
		Out: "XXXXXA",
	},
	{
		Desc: `GSUB6: A | A | -> 1@0, X | A | -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAAA",
		Out: "AXXXXX",
	},
	{ // harfbuzz: AYALYLLA
		Desc: `GSUB6: -ligs A | B B | A -> 1@0 2@0
			GSUB4: -ligs B B -> X
			GSUB1: X -> Y`,
		In:   "ABBALBLBLA",
		Out:  "AYALYLLA",
		Text: "ABBALBBLLA",
	},
	{ // harfbuzz, Mac: AX; Windows: ABB
		Desc: `GSUB6: A | B | B -> 1@0
			GSUB4: B B -> X`,
		In:  "ABB",
		Out: "ABB",
	},
	{ // harfbuzz: ABXDE; Mac, Windows:
		Desc: `GSUB6: B | C | D -> 1@0
			GSUB6: A B | C | D E -> 2@0
			GSUB4: C -> X`,
		In:  "ABCDE",
		Out: "ABXDE",
	},
	{
		Desc: `GSUB6: -ligs A | A A | A -> 1@0
			GSUB4: A L A -> X`,
		In:  "AALAA",
		Out: "AXA",
	},
	{
		Desc: `GSUB6: -ligs A | A | A -> 1@0
			GSUB4: A L A -> X`,
		In:  "AALA",
		Out: "AALA",
	},
	{
		Desc: `GSUB6: -ligs A | A | A -> 1@0
			GSUB4: A L -> X`,
		In:  "AALA",
		Out: "AXA",
	},

	{
		Desc: `GSUB6:
				backtrackclass :all: = [A - Z]
				inputclass :A: = [A B]
				inputclass :C: = [C D]
				inputclass :E: = [E F]
				lookaheadclass :all: = [A - Z]
				/E F/ :all: | :E: :C: :A: | :all: -> 1@0
			GSUB1: F -> X`,
		In:  "GFDBK",
		Out: "GXDBK",
	},
	{
		Desc: `GSUB6:
				backtrackclass :A: = [A]
				backtrackclass :B: = [B]
				backtrackclass :C: = [C]
				inputclass :D: = [D]
				inputclass :E: = [E]
				inputclass :F: = [F]
				lookaheadclass :G: = [G]
				lookaheadclass :H: = [H]
				lookaheadclass :I: = [I]
				/D/ :A: :B: :C: | :D: :E: :F: | :G: :H: :I: -> 1@0
			GSUB1: D -> X`,
		In:  "ABCDEFGHI",
		Out: "ABCXEFGHI",
	},
	{
		Desc: `GSUB6:
				backtrackclass :A: = [A X]
				inputclass :A: = [A]
				lookaheadclass :A: = [A]
				/A/ :A: | :A: | :A: -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "AXXXA",
	},
	{
		Desc: `GSUB6:
				backtrackclass :A: = [A X]
				inputclass :A: = [A]
				/A/ :A: | :A: | -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "AXXXX",
	},
	{
		Desc: `GSUB6:
				inputclass :A: = [A]
				lookaheadclass :A: = [A]
				/A/ | :A: | :A: -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "XXXXA",
	},
	{
		Desc: `GSUB6: -ligs
				backtrackclass :A: = [A]
				inputclass :LM: = [L M]
				lookaheadclass :A: = [A]
				/L M/ :A: | :LM: | :A: -> 1@0
			GSUB1: L -> X, M -> X`,
		In:  "ALAMA",
		Out: "ALAXA",
	},

	{
		Desc: `GSUB6:
				[A X] | [A] | [A] -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "AXXXA",
	},
	{
		Desc: `GSUB6:
				| [A] | [A] -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "XXXXA",
	},
	{
		Desc: `GSUB6:
				[A X] | [A] | -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "AXXXX",
	},
	{
		Desc: `GSUB6: -ligs
				[A] | [L M] | [A] -> 1@0
			GSUB1: L -> X, M -> X`,
		In:  "ALAMALMLA",
		Out: "ALAXALXLA",
	},
}
