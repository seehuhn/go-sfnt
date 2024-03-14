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

// GsubTestCase is a test case for the GSUB lookups.
type GsubTestCase struct {
	// Name is a human-readable label for the test.
	Name string

	// Desc is a textual description of a GSUB lookup.
	// This uses the syntax of seehuhn.de/go/sfnt/opentype/gtab/builder .
	Desc string

	// In is the input string for the GSUB lookup.
	In string

	// Out is the expected output string for the GSUB lookup.
	Out string

	// Text is the expected output text content for the GSUB lookup.  This is
	// only used in cases where ignored glyphs have to be moved after the
	// glyphs involved in a lookup, thus changing the order of text.
	// If this is empty, the expected text content is the same as In.
	Text string
}

//go:generate go run ./generate/

// TODO(voss): make the generator script list the harfbuzz and coretext/MacOS
// versions here.

// Gsub lists test cases for the GSUB table.
//
// My overall aim is to replicate the behaviour of ligatures in MS Word, but so
// far I have failed to reverse engineer the complete set of rules for this.
// For comparison, I have also included the behaviour of harfbuzz and of the
// MacOS layout engine.
var Gsub = []*GsubTestCase{ // START OF TEST CASES

	// ------------------------------------------------------------------
	// SECTION 1: simple lookups
	// Here we test lookups which are not chained and have no context.
	// All implementations I have tested agree on the outcome in these cases.

	// GSUB1 replaces single glyphs with single glyphs.

	{ // harfbuzz: XBZ, Mac: XBZ
		Name: "1_01",
		Desc: "GSUB1: A->X, C->Z",
		In:   "ABC",
		Out:  "XBZ",
	},
	{ // harfbuzz: BAC, Mac: BAC
		Name: "1_02",
		Desc: "GSUB1: A->B, B->A",
		In:   "ABC",
		Out:  "BAC",
	},

	// Ignored glyphs are not replaced:
	{ // harfbuzz: BBC, Mac: BBC
		Name: "1_03",
		Desc: "GSUB1: -base A->B, B->A",
		In:   "ABC",
		Out:  "BBC",
	},
	{ // harfbuzz: BBMBB, Mac: BBMBB
		Name: "1_04",
		Desc: "GSUB1: -marks A->B, M->N",
		In:   "AAMBA",
		Out:  "BBMBB",
	},

	// GSUB2 replaces a single glyph with sequences of glyphs.

	{ // harfbuzz: AAAA, Mac: AAAA
		Name: "1_05",
		Desc: `GSUB2: A->A A`,
		In:   "AA",
		Out:  "AAAA",
	},
	{ // harfbuzz: ABABMABA, Mac: ABABMABA
		Name: "1_06",
		Desc: `GSUB2: -marks A -> "ABA", M -> A`,
		In:   "ABMA",
		Out:  "ABABMABA",
	},

	// GSUB3 replaces sequences of glyphs with list of glyphs to choose
	// from.  Our implementation always chooses the first glyph from the list.

	{ // harfbuzz: BB, Mac: BB
		Name: "1_07",
		Desc: `GSUB3: A -> [B C D]`,
		In:   "AB",
		Out:  "BB",
	},
	{ // harfbuzz: BM, Mac: BM
		Name: "1_08",
		Desc: `GSUB3: -marks A -> [B C], M -> [B C]`,
		In:   "AM",
		Out:  "BM",
	},
	// If the list of replacements is empty, the lookup is ignored:
	{ // harfbuzz: AAA, Mac: AAA
		Name: "1_09",
		Desc: `GSUB3: A -> []`,
		In:   "AAA",
		Out:  "AAA",
	},

	// GSUB4 replaces sequences of glyphs with single glyphs.
	// This is normally used for ligatures.

	{ // harfbuzz: ABAB, Mac: ABAB
		Name: "1_10",
		Desc: `GSUB4: "BA" -> B`,
		In:   "ABAABA",
		Out:  "ABAB",
	},
	{ // harfbuzz: BCXD, Mac: BCXD
		Name: "1_11",
		Desc: `GSUB4: "AAA" -> "B", "AA" -> "C", "A" -> "D"`,
		In:   "AAAAAXA",
		Out:  "BCXD",
	},
	{ // harfbuzz: XBXMA, Mac: XBXMA
		Name: "1_12",
		Desc: `GSUB4: -marks "AAA" -> "X"`,
		In:   "AAABAMAAA",
		Out:  "XBXMA",
		Text: "AAABAAAMA",
	},

	// ------------------------------------------------------------------
	// SECTION 2: recursive lookups

	{ // harfbuzz: AXA, Mac: AXA
		Name: "2_01",
		Desc: `GSUB5: "AAA" -> 1@1
				GSUB1: "A" -> "X"`,
		In:  "AAA",
		Out: "AXA",
	},
	{ // harfbuzz: XYZ, Mac: XYZ
		Name: "2_02",
		Desc: `GSUB5: "AAA" -> 1@0 2@1 3@2
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAA",
		Out: "XYZ",
	},
	{ // harfbuzz: XYZ, Mac: XYZ
		Name: "2_03",
		Desc: `GSUB5: "AAA" -> 3@2 1@0 2@1
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAA",
		Out: "XYZ",
	},
	{ // harfbuzz: AXA, Mac: AXA
		Name: "2_04",
		Desc: `GSUB5: "AAA" -> 1@1 2@1 3@1
				GSUB1: "A" -> "B"
				GSUB1: "B" -> "M"
				GSUB1: "M" -> "X"`,
		In:  "AAA",
		Out: "AXA",
	},

	// Child looksups which don't apply are ignored:
	{ // harfbuzz: AXC, Mac: AXC
		Name: "2_05",
		Desc: `GSUB5: "ABC" -> 1@0 1@1 1@2
				GSUB1: "B" -> "X"`,
		In:  "ABC",
		Out: "AXC",
	},

	// Matches cannot overlap:
	{ // harfbuzz: AXXAXXXX, Mac: AXXAXXXX
		Name: "2_06",
		Desc: `GSUB5: "XXX" -> 1@0
				GSUB1: "X" -> "A"`,
		In:  "XXXXXXXX",
		Out: "AXXAXXXX",
	},

	// Try different types of child lookups:
	{ // harfbuzz: YA, Mac: YA
		Name: "2_07",
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	{ // harfbuzz: YA, Mac: YA
		Name: "2_08",
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB2: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	{ // harfbuzz: YA, Mac: YA
		Name: "2_09",
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB4: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	//
	// The same, but with one more level of nesting.
	{ // harfbuzz: YA, Mac: YA
		Name: "2_10",
		Desc: `GSUB5: "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	{ // harfbuzz: YA, Mac: YA
		Name: "2_11",
		Desc: `GSUB5: "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB2: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	{ // harfbuzz: YA, Mac: YA
		Name: "2_12",
		Desc: `GSUB5: "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB4: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	//
	// ... and with ligatures ignored
	{ // harfbuzz: YLA, Mac: YLA
		Name: "2_13",
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "YLA",
	},
	{ // harfbuzz: YLA, Mac: YLA
		Name: "2_14",
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB2: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "YLA",
	},
	{ // harfbuzz: YLA, Mac: YLA
		Name: "2_15",
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB4: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "YLA",
	},

	// ------------------------------------------------------------------
	// SECTION 3: Testing glyph positions in recursive lookups where the
	// sequence length changes.

	// The positions for chained actions are interpreted at the time the child
	// action is run, not when the parent lookup is matched:
	{ // harfbuzz: AAX, Mac: AAX, Windows: AAX
		Name: "3_01",
		Desc: `GSUB5: "AAAA" -> 1@0 2@2
				GSUB4: "AA" -> "A"
				GSUB1: "A" -> "X"`,
		In:  "AAAA",
		Out: "AAX",
	},
	{ // harfbuzz: AAXAA, Mac: AAXAA, Windows: AAXAA
		Name: "3_02",
		Desc: `GSUB5: "AAA" -> 1@1 2@2
				GSUB2: "A" -> "AAA"
				GSUB1: "A" -> "X"`,
		In:  "AAA",
		Out: "AAXAA",
	},

	{ // harfbuzz: XYZ, Mac: XYZ, Windows: XYZ
		Name: "3_03",
		Desc: `GSUB5: "AAAA" -> 1@0 4@2 3@1 2@0
				GSUB4: "AA" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAAA",
		Out: "XYZ",
	},
	{ // harfbuzz: XYZA, Mac: XYZA, Windows: XYZA
		Name: "3_04",
		Desc: `GSUB5: "AAA" -> 1@0 4@2 3@1 2@0
				GSUB2: "A" -> "AA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAA",
		Out: "XYZA",
	},
	{ // harfbuzz: XYZA, Mac: XYZA, Windows: XYZA
		Name: "3_05",
		Desc: `GSUB5: "AAA" -> 1@0 5@2 4@1 3@0
				GSUB5: "AA" -> 2@1
				GSUB2: "A" -> "AA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAA",
		Out: "XYZA",
	},

	// ------------------------------------------------------------------
	// SECTION 4: Check under which circumstances new glyphs are added to the
	// input sequence.  The behaviour in some of these cases is not precisely
	// defined in the OpenType specification, and implementations differ.

	// ------------------------------------------------------------------
	// single glyphs:
	//   A: yes
	//   M: no

	// We have seen above that if a normal glyph is replaced, the
	// replacement IS added to the input sequence.

	// If a single ignored glyph is replaced, the replacement is NOT added
	// to the input sequence:
	{ // harfbuzz: ABX, Mac: ABX, Windows: ABX
		Name: "4_01",
		// ALA -> ABA -> ABX
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@1
				GSUB5: "ALA" -> 2@1
				GSUB4: "L" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "ABX",
	},
	{ // harfbuzz: AYBA, Mac: ABYA, Windows: ABBX
		Name: "4_02",
		// ALA -> ABBA -> ...
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@1
				GSUB5: "AL" -> 2@1
				GSUB2: "L" -> "BB"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "ABBX", // (!!!)
	},
	// If a glyph is replaced with a glyph from an ignored class,
	// the replacement glyph stays in the input sequence.
	{ // harfbuzz: AYA, Mac: AYA
		Name: "4_03",
		Desc: `GSUB5: -marks "AAA" -> 1@1 2@1
				GSUB1: "A" -> "M"
				GSUB1: "A" -> "X", "M" -> "Y"`,
		In:  "AAA",
		Out: "AYA",
	},

	// ------------------------------------------------------------------
	// pairs:
	//   AA: yes
	//   MA: yes
	//   AM: mixed????
	//   MM: no

	// When a pair of normal glyphs is replaced, the replacement IS added.
	{ // harfbuzz: AY, Mac: AY; Windows:
		Name: "4_04",
		// AAA -> AB -> ...
		Desc: `GSUB5: -ligs "AAA" -> 1@0 3@1
				GSUB5: "AAA" -> 2@1
				GSUB4: "AA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAA",
		Out: "AY",
	},

	{ // harfbuzz: AB, Mac: AB, Windows: AY -> yes
		Name: "4_05",
		// ALA -> AB -> ...
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@1
				GSUB5: "ALA" -> 2@1
				GSUB4: "LA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "AY", // (!!!)
	},

	// normal+ignored
	{ // harfbuzz: YAA, Mac: YAA, Windows: YAA -> yes
		Name: "4_06",
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
	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> no
		Name: "4_07",
		// ALLA -> ABA -> ...
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
		Name: "4_08",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AAM" -> 2@0
				GSUB4: "AAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMA",
		Out: "YA",
	},

	{ // harfbuzz: YA, Mac: YA, Windows: YA -> included
		Name: "4_09",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMA" -> 2@0
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "YA",
	},
	{ // harfbuzz: ABA (!!!), Mac: AYA, Windows: AYA -> included
		Name: "4_10",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AAMA" -> 2@1
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMAA",
		Out: "AYA",
	},
	{ // harfbuzz: ABX (!!!), Mac: AYA, Windows: AYA -> included
		Name: "4_11",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@1
				GSUB5: "AAMA" -> 2@1
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMAA",
		Out: "AYA",
	},

	{ // harfbuzz: YAA, Mac: YAA, Windows: YAA -> included
		Name: "4_12",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMM" -> 2@0
				GSUB4: "AMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAA",
		Out: "YAA",
	},

	{ // harfbuzz: AB, Mac: AB, Windows: AY -> included
		Name: "4_13",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAA" -> 2@1
				GSUB4: "MAA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "AY", // (!!!)
	},

	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> not included
		Name: "4_14",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAM" -> 2@1
				GSUB4: "MAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "ABX",
	},
	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> not included
		Name: "4_15",
		// ALALA -> ABA -> ...
		Desc: `GSUB5: -ligs "AAA" -> 1@0 3@1
				GSUB5: "ALALA" -> 2@1
				GSUB4: "LAL" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALALA",
		Out: "ABX",
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: AYA -> included
		Name: "4_16",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMMA" -> 2@1
				GSUB4: "MMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAA",
		Out: "AYA", // (!!!)
	},

	{ // harfbuzz: ABAA (!!!), Mac: ABXA, Windows: ABXA -> not included
		Name: "4_17",
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

	{ // harfbuzz: YA, Mac: YA, Windows: YA -> yes
		Name: "4_18",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMAM" -> 2@0
				GSUB4: "AMAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "YA",
	},

	{ // harfbuzz: YAA, Mac: YAA, Windows: YAA -> yes
		Name: "4_19",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMMM" -> 2@0
				GSUB4: "AMMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMMAA",
		Out: "YAA",
	},

	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> no
		Name: "4_20",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@1
				GSUB5: "AMAAM" -> 2@1
				GSUB4: "MAAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAAMA",
		Out: "ABX",
	},

	{ // harfbuzz: AB, Mac: AB, Windows: AY -> yes
		Name: "4_21",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAMA" -> 2@1
				GSUB4: "MAMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "AY", // (!!!)
	},

	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> no
		Name: "4_22",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAMM" -> 2@1
				GSUB4: "MAMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMMA",
		Out: "ABX",
	},

	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> no
		Name: "4_23",
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
		Name: "4_24",
		Desc: `GSUB5: -marks "AAA" -> 1@0 2@0
				GSUB4: "AM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "YAA",
	},
	// { // harfbuzz: AYA, Mac: AYA, Windows: ABX -> no ????????????????????
	// 	Desc: `GSUB5: -marks "AAA" -> 1@1 2@1
	// 			GSUB4: "AM" -> "B"
	// 			GSUB1: "A" -> "X", "B" -> "Y"`,
	// 	In:  "AAMA",
	// 	Out: "ABX",
	// },

	// XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX

	// longer:
	//   MMMAAA -> yes
	//   MMAMAA -> yes

	{ // harfbuzz: ABA, Mac: ABX, Windows: AYA -> included
		Name: "4_25",
		Desc: `GSUB5: -marks "AAAAA" -> 1@0 3@1
				GSUB5: "AMMMAAA" -> 2@1
				GSUB4: "MMMAAA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMMAAAA",
		Out: "AYA", // (!!!)
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: AYA -> included
		Name: "4_26",
		Desc: `GSUB5: -marks "AAAAA" -> 1@0 3@1
				GSUB5: "AMMAMAA" -> 2@1
				GSUB4: "MMAMAA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAMAAA",
		Out: "AYA", // (!!!)
	},

	// XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX

	// { // harfbuzz: AXA, Mac: AXY, Windows: AAY ????????????????????????
	// 	// ALMA -> AAA -> ...
	// 	desc: `GSUB5: -ligs -marks "AA" -> 1@0 4@1
	// 			GSUB5: -marks "ALA" -> 2@1 3@1
	// 			GSUB4: "LM" -> "A"
	// 			GSUB1: "A" -> "X"
	// 			GSUB1: "A" -> "Y", "X" -> "Y"`,
	// 	in:  "ALMA",
	// 	out: "AAY",
	// },

	{ // harfbuzz: DEI, Mac: DEI, Windows: DEI
		Name: "4_27",
		Desc: `GSUB5: "ABC" -> 1@0 1@2 1@3 2@2
				GSUB2: "A" -> "DE", "B" -> "FG", "G" -> "H"
				GSUB4: "FHC" -> "I"`,
		In:  "ABC",
		Out: "DEI",
	},
	{ // harfbuzz: AXAAKA, Mac: AAXAKA, Windows: AAAXKA
		Name: "4_28",
		Desc: `GSUB5: -ligs "AAA" -> 1@0 2@1 3@1
				GSUB5: "AK" -> 2@1
				GSUB2: "K" -> "AA"
				GSUB1: "A" -> "X", "K" -> "X", "L" -> "X"`,
		In:  "AKAKA",
		Out: "AAAXKA", // (!!!)
	},
	{ // harfbuzz: AXLAKA, Mac: ALXAKA, Windows: ALLXKA
		Name: "4_29",
		Desc: `GSUB5: -ligs "AAA" -> 1@0 2@1 3@1
				GSUB5: "AK" -> 2@1
				GSUB2: "K" -> "LL"
				GSUB1: "A" -> "X", "K" -> "X", "L" -> "X"`,
		In:  "AKAKA",
		Out: "ALLXKA", // (!!!)
	},
	{ // harfbuzz: XAYZ, Mac: XAYZ
		Name: "4_30",
		Desc: `GSUB5: -ligs "AAA" -> 1@0 5@2 4@1 3@0
				GSUB5: "AL" -> 2@1
				GSUB1: "L" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "ALAA",
		Out: "XAYZ",
	},
	{ // harfbuzz: AAAAB, Mac: AAAAB
		Name: "4_31",
		Desc: `GSUB5: "AB" -> 1@0 0@0, "AAB" -> 1@0 0@0, "AAAB" -> 1@0 0@0
				GSUB2: "A" -> "AA"`,
		In:  "AB",
		Out: "AAAAB",
	},
	{ // harfbuzz: XYAZA, Mac: XAYZA, Windows: XAAYZ
		Name: "4_32",
		Desc: `GSUB5: -ligs "AAA" -> 1@0 5@2 4@1 3@0
				GSUB5: "AL" -> 2@1
				GSUB2: "L" -> "AA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "ALAA",
		Out: "XAAYZ", // (!!!)
	},
	{ // harfbuzz: XLYZ, Mac: XLYZ, Windows: XLYZA
		Name: "4_33",
		Desc: `GSUB5: -ligs "AAAA" -> 1@0 5@2 4@1 3@0
				GSUB5: "AL" -> 2@1
				GSUB4: "LA" -> "L"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "ALAAA",
		Out: "XLYZA", // (!!!)
	},
	{ // harfbuzz: AKA, Mac: AKA, Windows: ABAA
		Name: "4_34",
		Desc: `GSUB5: -ligs "AAA" -> 1@0
				GSUB5: "AL" -> 2@1 3@1
				GSUB4: "LA" -> "K"
				GSUB1: "L" -> "B"`,
		In:  "ALAA",
		Out: "ABAA", // (!!!)
	},
	{ // harfbuzz: LXLYLZL, Mac: LXLYLZL, Windows: LXLYLZL
		Name: "4_35",
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
	{ // harfbuzz: LALALXB, Mac: LALALXB, Windows: LALALXB -> "AL" WAS added to input here
		Name: "4_36",
		Desc: `GSUB5: -ligs "AAA" -> 1@2
				GSUB4: "AL" -> "X"`,
		In:  "LALALALB",
		Out: "LALALXB",
	},
	{ // harfbuzz: ABXD, Mac: ABXD, Windows: ABXD -> "CL" was added to input here
		Name: "4_37",
		Desc: `GSUB5: -ligs "ABC" -> 1@2
			GSUB4: C L -> X`,
		In:  "ABCLD",
		Out: "ABXD",
	},

	{ // harfbuzz: XCEAAAAXFBCAACX, Mac: XCEAAAAXFBCAACX, Windows: XCEAAAAXFBCAACX
		Name: "4_38",
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

	{ // harfbuzz: XBCFBCXA, Mac: XBCFBCXA, Windows: XBCFBCXA
		Name: "4_39",
		Desc: `GSUB5:
				[A-E] [A B] [C-X] -> 1@0 ||
				[B-E] [B-E] [A-C] -> 1@1
			GSUB1: A -> X, B -> X, C -> X, D -> X, E -> X, F -> X`,
		In:  "ABCFBCEA",
		Out: "XBCFBCXA",
	},
	{ // harfbuzz: X, Mac: X, Windows: X
		Name: "4_40",
		Desc: `GSUB5: -ligs
				class :A: = [A]
				/A/ :A: -> 1@0
			GSUB4: A L -> X`,
		In:  "AL",
		Out: "X",
	},

	// ------------------------------------------------------------------
	// SECTION 5: lookup rules with context

	{ // harfbuzz: ABXDEF, Mac: ABXDEF
		Name: "5_01",
		Desc: `GSUB6: A B | C D | E F -> 1@0
			GSUB1: C -> X`,
		In:  "ABCDEF",
		Out: "ABXDEF",
	},
	{ // harfbuzz: ABCDE, Mac: ABCDE
		Name: "5_02",
		Desc: `GSUB6: A B | C D | E F -> 1@0
			GSUB1: C -> X`,
		In:  "ABCDE",
		Out: "ABCDE",
	},
	{ // harfbuzz: ABC, Mac: ABC
		Name: "5_03",
		Desc: `GSUB6: A B | C D | E F -> 1@0
			GSUB1: C -> X`,
		In:  "ABC",
		Out: "ABC",
	},
	{ // harfbuzz: AXXXXA, Mac: AXXXXA
		Name: "5_04",
		Desc: `GSUB6: A | A | A -> 1@0, X | A | A -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAAA",
		Out: "AXXXXA",
	},
	{ // harfbuzz: XXXXXA, Mac: XXXXXA
		Name: "5_05",
		Desc: `GSUB6: | A | A -> 1@0, | A | A -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAAA",
		Out: "XXXXXA",
	},
	{ // harfbuzz: AXXXXX, Mac: AXXXXX
		Name: "5_06",
		Desc: `GSUB6: A | A | -> 1@0, X | A | -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAAA",
		Out: "AXXXXX",
	},
	{ // harfbuzz: AYALYLLA, Mac: AYALYLLA
		Name: "5_07",
		Desc: `GSUB6: -ligs A | B B | A -> 1@0 2@0
			GSUB4: -ligs B B -> X
			GSUB1: X -> Y`,
		In:   "ABBALBLBLA",
		Out:  "AYALYLLA",
		Text: "ABBALBBLLA",
	},
	{ // harfbuzz: AX, Mac: AX; Windows: ABB
		Name: "5_08",
		Desc: `GSUB6: A | B | B -> 1@0
			GSUB4: B B -> X`,
		In:  "ABB",
		Out: "ABB", // (!!!)
	},
	{ // harfbuzz: ABXDE, Mac: ABXDE
		Name: "5_09",
		Desc: `GSUB6: B | C | D -> 1@0
			GSUB6: A B | C | D E -> 2@0
			GSUB4: C -> X`,
		In:  "ABCDE",
		Out: "ABXDE",
	},
	{ // harfbuzz: AXA, Mac: AXA
		Name: "5_10",
		Desc: `GSUB6: -ligs A | A A | A -> 1@0
			GSUB4: A L A -> X`,
		In:  "AALAA",
		Out: "AXA",
	},
	{ // harfbuzz: AX, Mac: AX, Windows: AALA
		Name: "5_11",
		Desc: `GSUB6: -ligs A | A | A -> 1@0
			GSUB4: A L A -> X`,
		In:  "AALA",
		Out: "AALA", // (!!!)
	},
	{ // harfbuzz: AXA, Mac: AXA
		Name: "5_12",
		Desc: `GSUB6: -ligs A | A | A -> 1@0
			GSUB4: A L -> X`,
		In:  "AALA",
		Out: "AXA",
	},

	{ // harfbuzz: GXDBK, Mac: GXDBK
		Name: "5_13",
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
	{ // harfbuzz: ABCXEFGHI, Mac: ABCXEFGHI
		Name: "5_14",
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
	{ // harfbuzz: AXXXA, Mac: AXXXA
		Name: "5_15",
		Desc: `GSUB6:
				backtrackclass :A: = [A X]
				inputclass :A: = [A]
				lookaheadclass :A: = [A]
				/A/ :A: | :A: | :A: -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "AXXXA",
	},
	{ // harfbuzz: AXXXX, Mac: AXXXX
		Name: "5_16",
		Desc: `GSUB6:
				backtrackclass :A: = [A X]
				inputclass :A: = [A]
				/A/ :A: | :A: | -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "AXXXX",
	},
	{ // harfbuzz: XXXXA, Mac: XXXXA
		Name: "5_17",
		Desc: `GSUB6:
				inputclass :A: = [A]
				lookaheadclass :A: = [A]
				/A/ | :A: | :A: -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "XXXXA",
	},
	{ // harfbuzz: ALAXA, Mac: ALAXA
		Name: "5_18",
		Desc: `GSUB6: -ligs
				backtrackclass :A: = [A]
				inputclass :LM: = [L M]
				lookaheadclass :A: = [A]
				/L M/ :A: | :LM: | :A: -> 1@0
			GSUB1: L -> X, M -> X`,
		In:  "ALAMA",
		Out: "ALAXA",
	},

	{ // harfbuzz: AXXXA, Mac: AXXXA
		Name: "5_19",
		Desc: `GSUB6:
				[A X] | [A] | [A] -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "AXXXA",
	},
	{ // harfbuzz: XXXXA, Mac: XXXXA
		Name: "5_20",
		Desc: `GSUB6:
				| [A] | [A] -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "XXXXA",
	},
	{ // harfbuzz: AXXXX, Mac: AXXXX
		Name: "5_21",
		Desc: `GSUB6:
				[A X] | [A] | -> 1@0
			GSUB1: A -> X`,
		In:  "AAAAA",
		Out: "AXXXX",
	},
	{ // harfbuzz: ALAXALXLA, Mac: ALAXALXLA
		Name: "5_22",
		Desc: `GSUB6: -ligs
				[A] | [L M] | [A] -> 1@0
			GSUB1: L -> X, M -> X`,
		In:  "ALAMALMLA",
		Out: "ALAXALXLA",
	},
} // END OF TEST CASES
