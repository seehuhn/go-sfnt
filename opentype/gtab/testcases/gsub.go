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

	// GSUB3 replaces sequences of glyphs with one of list of possible choices.
	// Our implementation always chooses the first glyph from the list.

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

	// GSUB4 replaces a sequence of glyphs with a single glyph.
	// This is normally used for ligatures.

	{ // harfbuzz: AXAX, Mac: AXAX
		Name: "1_10",
		Desc: `GSUB4: "BC" -> X`,
		In:   "ABCABC",
		Out:  "AXAX",
	},
	{ // harfbuzz: BCXD, Mac: BCXD
		Name: "1_11",
		Desc: `GSUB4: "AAA" -> "B", "AA" -> "C", "A" -> "D"`,
		In:   "AAAAAXA",
		Out:  "BCXD",
	},
	// Any ignored glyphs embedded inside the match are moved after the
	// replacement:
	{ // harfbuzz: XM, Mac: XM
		Name: "1_12",
		Desc: `GSUB4: -marks "AA" -> "X"`,
		In:   "AMA",
		Out:  "XM",
		Text: "AAM",
	},
	{ // harfbuzz: XLMN, Mac: XLMN
		Name: "1_13",
		Desc: `GSUB4: -marks -ligs "AB" -> "X"`,
		In:   "ALMNB",
		Out:  "XLMN",
		Text: "ABLMN",
	},
	// Sequences explicitly including ignored glyphs are not replaced:
	{ // harfbuzz: AMA, Mac: AMA
		Name: "1_14",
		Desc: `GSUB4: -marks "AMA" -> "X"`,
		In:   "AMA",
		Out:  "AMA",
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

	// Child lookups which don't apply are ignored:
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

	// Child matches cannot extend beyond the parent match:
	{ // harfbuzz: X, Mac: X, Windows: ABC
		Name: "2_07",
		Desc: `GSUB5: "AB" -> 1@0
				GSUB4: "ABC" -> "X"`,
		In:  "ABC",
		Out: "ABC", // (!!!)
	},
	// But trailing ignored glyphs are included in the parent match,
	// so can be matched in the child lookup:
	{ // harfbuzz: BAM (!!!), Mac: BB, Windows: BB
		Name: "2_08",
		Desc: `GSUB5: -marks "AA" -> 1@0 1@1
				GSUB4: "AM" -> "B"`,
		In:  "AMAM",
		Out: "BB",
	},

	// Child lookups can be of any type:
	{ // harfbuzz: YA, Mac: YA
		Name: "2_09",
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	{ // harfbuzz: YA, Mac: YA
		Name: "2_10",
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB2: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	{ // harfbuzz: YA, Mac: YA
		Name: "2_11",
		Desc: `GSUB5: "AA" -> 1@0 2@0
				GSUB4: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	//
	// The same, but with one more level of nesting.
	{ // harfbuzz: YA, Mac: YA
		Name: "2_12",
		Desc: `GSUB5: "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	{ // harfbuzz: YA, Mac: YA
		Name: "2_13",
		Desc: `GSUB5: "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB2: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AA",
		Out: "YA",
	},
	{ // harfbuzz: YA, Mac: YA
		Name: "2_14",
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
		Name: "2_15",
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB1: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "YLA",
	},
	{ // harfbuzz: YLA, Mac: YLA
		Name: "2_16",
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB2: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "YLA",
	},
	{ // harfbuzz: YLA, Mac: YLA
		Name: "2_17",
		Desc: `GSUB5: -ligs "AA" -> 1@0 3@0
				GSUB5: "A" -> 2@0
				GSUB4: "A" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALA",
		Out: "YLA",
	},

	// Glyphs which are ignored in the parent lookup may be matched in the
	// child lookup:
	{ // harfbuzz: AXA, Mac: AXA
		Name: "2_18",
		Desc: `GSUB5: -marks "AA" -> 1@0
				GSUB5: "AMA" -> 2@1
				GSUB1: "M" -> "X"`,
		In:  "AMA",
		Out: "AXA",
	},

	// ------------------------------------------------------------------
	// SECTION 3: Testing lookup positions in recursive lookups where the
	// sequence length changes.

	// Adding/removing matched glyphs in the parent sequence modifies the input
	// sequence.  This is true, even if the added glyphs would have been
	// ignored during the original matching.  The positions for chained actions
	// are interpreted at the time the child action is run, not when the parent
	// lookup is matched.

	// If glyphs are removed, the positions of the following actions are
	// shifted to make up for the removed glyphs; in the following example,
	// "Z" indicates position 2 after one of the "A"s has been removed.
	{ // harfbuzz: AAZA, Mac: AAZA, Windows: AAZA
		Name: "3_01",
		Desc: `GSUB5: "AAAAA" -> 1@0 2@2
				GSUB4: "AA" -> "A"
				GSUB1: "A" -> "Z"`,
		In:  "AAAAA",
		Out: "AAZA",
	},
	// If one glyph is replaced with several, the new glyphs count towards
	// the positions of the following actions.  Here, "X" indicates position 1
	// after the first "A" has been replaced with "AAA".
	{ // harfbuzz: AXAA, Mac: AXAA, Windows: AXAA
		Name: "3_02",
		Desc: `GSUB5: "AA" -> 1@0 2@1
				GSUB2: "A" -> "AAA"
				GSUB1: "A" -> "X"`,
		In:  "AA",
		Out: "AXAA",
	},
	// In a longer chain of actions, the positions are updated after every
	// action:
	{ // harfbuzz: B, Mac: B, Windows: B
		Name: "3_03",
		Desc: `GSUB5: "AAAAB" -> 1@3 1@2 1@1 1@0
				GSUB4: "AB" -> "B"`,
		In:  "AAAAB",
		Out: "B",
	},
	// The rules apply recursively to child lookups.  Here we first substitute
	// the middle "A" with "AA", position 1 for this sub-lookup is indicated by
	// "X" to get "(A)AXA".  Then we mark position 1 in the parent lookup with
	// "Y" to get "AYXA".
	{ // harfbuzz: AYXA, Mac: AYXA, Windows: AYXA
		Name: "3_04",
		Desc: `GSUB5: "AAA" -> 1@1 4@1
				GSUB5: "AA" -> 2@0 3@1
				GSUB2: "A" -> "AA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"`,
		In:  "AAA",
		Out: "AYXA",
	},
	// The previous example still works, when ignored glyphs are interspersed
	// with the sequence:
	{ // harfbuzz: AMYXMA, Mac: AMYXMA
		Name: "3_05",
		Desc: `GSUB5: -marks "AAA" -> 1@1 4@1
				GSUB5: -marks "AA" -> 2@0 3@1
				GSUB2: "A" -> "AA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"`,
		In:  "AMAMA",
		Out: "AMYXMA",
	},
	// If a normally ignored glyph is introduced, it becomes part of the input
	// sequence:
	{ // harfbuzz: AYA, Mac: AYA, Windows: AYA
		Name: "3_06",
		Desc: `GSUB5: -marks "AA" -> 1@0 2@1
				GSUB2: "A" -> "AM"
				GSUB1: "A" -> "X", "M" -> "Y"`,
		In:  "AA",
		Out: "AYA",
	},

	// Check that matches don't overlap when the length of the sequence changes.
	{ // harfbuzz: XAAAXAAAXAAA, Mac: XAAAXAAAXAAA
		Name: "3_07",
		Desc: `GSUB5: "AA" -> 1@0 2@1
				GSUB1: "A" -> "X"
				GSUB2: "A" -> "AAA"`,
		In:  "AAAAAA",
		Out: "XAAAXAAAXAAA",
	},
	{ // harfbuzz: XAXAXA, Mac: XAXAXA
		Name: "3_08",
		Desc: `GSUB5: "AAAA" -> 1@0 2@1
				GSUB1: "A" -> "X"
				GSUB4: "AAA" -> "A"`,
		In:  "AAAAAAAAAAAA",
		Out: "XAXAXA",
	},

	// ------------------------------------------------------------------
	// SECTION 4: Check situations where a child lookup replaces an ignored
	// glyph.  The behaviour in some of these cases is not precisely defined in
	// the OpenType specification, and the behaviour of different
	// implementations differs.

	// If embedded ignored glyphs are removed, this does not affect the
	// input sequence:
	{ // harfbuzz: AAX (!!!), Mac: AXA, Windows: AXA
		Name: "4_01",
		Desc: `GSUB5: -marks "AAA" -> 1@0 2@1
				GSUB4: "AM" -> "A"
				GSUB1: "A" -> "X"`,
		In:  "AMAA",
		Out: "AXA",
	},
	{ // harfbuzz: AAA (!!!), Mac: AXA
		Name: "4_02",
		Desc: `GSUB5: -marks "AAA" -> 1@0 2@1
				GSUB4: "AMM" -> "A"
				GSUB1: "A" -> "X"`,
		In:  "AMMAA",
		Out: "AXA",
	},

	// If embedded ignored glyphs are replaced, should the replacement be added
	// to the input sequence?
	{ // harfbuzz: AAX, Mac: AAX, Windows: AAX -> not added
		Name: "4_03",
		Desc: `GSUB5: -marks "AA" -> 1@0 3@1
				GSUB5: "AM" -> 2@1
				GSUB1: "M" -> "A"
				GSUB1: "A" -> "X"`,
		In:  "AMA",
		Out: "AAX",
	},
	{ // harfbuzz: AXAA, Mac: AAXA, Windows: AAAX -> not added
		Name: "4_04",
		Desc: `GSUB5: -marks "AA" -> 1@0 3@1
				GSUB5: "AM" -> 2@1
				GSUB2: "M" -> "AA"
				GSUB1: "A" -> "X"`,
		In:  "AMA",
		Out: "AAAX", // (!!!)
	},
	{ // harfbuzz: AXAAA, Mac: AAXAX, Windows: AAXAA -> ??????????????????
		Name: "4_05",
		Desc: `GSUB5: -marks "AA" -> 1@0 3@1
				GSUB5: "AM" -> 2@1
				GSUB2: "M" -> "AAA"
				GSUB1: "A" -> "X"`,
		In:  "AMA",
		Out: "AAAAX", // (!!!)
	},

	{ // harfbuzz: XAYZA, Mac: XAYZA, Windows: XAYZA
		Name: "4_06",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@0 4@1 5@2
				GSUB5: "AMA" -> 2@1
				GSUB1: "M" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AMAAA",
		Out: "XAYZA",
	},
	{ // harfbuzz: XYZAAAA, Mac: XAYZAAA, Windows: XAYAZAA -> ?????
		Name: "4_07",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@0 4@1 5@2
				GSUB5: "AMA" -> 2@1
				GSUB2: "M" -> "AAA"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AMAAA",
		Out: "XAAAYZA", // (!!!)
	},
	{ // harfbuzz: XAAYZ (!!!), Mac: XAYZA, Windows: XAYZA
		Name: "4_08",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@0 4@1 5@2
				GSUB5: "AMMA" -> 2@1
				GSUB4: "MM" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AMMAAA",
		Out: "XAYZA",
	},
	{ // harfbuzz: XNY, Mac: XNY, Windows: XNY -> not added
		Name: "4_09",
		Desc: `GSUB5: -marks "AA" -> 1@0 3@0 4@1 5@2
				GSUB5: "AMA" -> 2@1
				GSUB1: "M" -> "N"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AMA",
		Out: "XNY",
	},
	{ // harfbuzz: XNA (!!!), Mac: XNY, Windows: XNY -> not added
		Name: "4_10",
		Desc: `GSUB5: -marks "AA" -> 1@0 3@0 4@1 5@2
				GSUB5: "AMMA" -> 2@1
				GSUB4: "MM" -> "N"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AMMA",
		Out: "XNY",
	},

	// ------------------------------------------------------------------
	// pairs:
	//   AA: yes
	//   MA: yes
	//   AM: mixed????
	//   MM: no

	// When a pair of normal glyphs is replaced, the replacement IS added.
	{ // harfbuzz: XYZ, Mac: XYZ, Windows: XYZ
		Name: "4_11",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 2@0 3@1 4@2
				GSUB4: "AA" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAAA",
		Out: "XYZ",
	},
	{ // harfbuzz: XYZ, Mac: XYZ, Windows: XYZ
		Name: "4_12",
		Desc: `GSUB5: -marks "AAAA" -> 1@1 2@0 3@1 4@2
				GSUB4: "AA" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAAA",
		Out: "XYZ",
	},
	// This works, even if the replacement would normally be ignored:
	{ // harfbuzz: XYZ, Mac: XYZ
		Name: "4_13",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 2@0 3@1 4@2
				GSUB4: "AA" -> "M"
				GSUB1: "M" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAAA",
		Out: "XYZ",
	},
	{ // harfbuzz: XYZ, Mac: XYZ
		Name: "4_14",
		Desc: `GSUB5: -marks "AAAA" -> 1@1 2@0 3@1 4@2
				GSUB4: "AA" -> "M"
				GSUB1: "A" -> "X"
				GSUB1: "M" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAAA",
		Out: "XYZ",
	},

	// If a pair of the form matched+ignored is replaced, the replacement is
	// sometimes added and sometimes not.
	{ // harfbuzz: XAMY (!!!), Mac: XYMZ, Windows: XYMZ -> added
		Name: "4_15",
		Desc: `GSUB5: -marks "AAA" -> 1@0 2@0 3@1 4@2
				GSUB4: "AM" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AMAMA",
		Out: "XYMZ",
	},
	{ // harfbuzz: XMYA (!!!), Mac: XMYZ, Windows: XMYZ -> added
		Name: "4_16",
		Desc: `GSUB5: -marks "AAA" -> 1@1 2@0 3@1 4@2
				GSUB4: "AM" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AMAMA",
		Out: "XMYZ",
	},
	{ // harfbuzz: XYA (!!!), Mac: XYZ, Windows: XYZ -> added
		Name: "4_17",
		Desc: `GSUB5: -marks "AAA" -> 1@1 2@0 3@1 4@2
				GSUB4: "AM" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"
				GSUB1: "A" -> "Z"`,
		In:  "AAMA",
		Out: "XYZ",
	},
	// Maybe MS Word tries to second-guess us, and concludes from the presence
	// of the @2 in the parent lookup, that we want to add the replacement?
	// Otherwise, the difference between the previous test and this one is hard
	// to explain.
	{ // harfbuzz: XYA, Mac: XYA, Windows: XAY -> not added ??????????????????
		Name: "4_18",
		Desc: `GSUB5: -marks "AAA" -> 1@1 2@0 3@1
				GSUB4: "AM" -> "A"
				GSUB1: "A" -> "X"
				GSUB1: "A" -> "Y"`,
		In:  "AAMA",
		Out: "XYA",
	},

	// ...

	// normal+ignored
	{ // harfbuzz: YAA, Mac: YAA, Windows: YAA -> yes
		Name: "4_19",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AM" -> 2@0
				GSUB4: "AM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "YAA",
	},
	// { // harfbuzz: AXA, Mac: AXA, Windows: AAX -> no ????????????????????
	// 	desc: `GSUB5: -marks "ALA" -> 1@1 2@1
	// 			GSUB4: "LM" -> "A"
	// 			GSUB1: "A" -> "X"`,
	// 	in:  "ALMA",
	// 	out: "AAX",
	// },

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
		Name: "4_20",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AAM" -> 2@0
				GSUB4: "AAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMA",
		Out: "YA",
	},

	{ // harfbuzz: YA, Mac: YA, Windows: YA -> included
		Name: "4_21",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMA" -> 2@0
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "YA",
	},
	{ // harfbuzz: ABA (!!!), Mac: AYA, Windows: AYA -> included
		Name: "4_22",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AAMA" -> 2@1
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMAA",
		Out: "AYA",
	},
	{ // harfbuzz: ABX (!!!), Mac: AYA, Windows: AYA -> included
		Name: "4_23",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@1
				GSUB5: "AAMA" -> 2@1
				GSUB4: "AMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AAMAA",
		Out: "AYA",
	},

	{ // harfbuzz: YAA, Mac: YAA, Windows: YAA -> included
		Name: "4_24",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMM" -> 2@0
				GSUB4: "AMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAA",
		Out: "YAA",
	},

	{ // harfbuzz: AB, Mac: AB, Windows: AY -> included
		Name: "4_25",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAA" -> 2@1
				GSUB4: "MAA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAA",
		Out: "AY", // (!!!)
	},

	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> not included
		Name: "4_26",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAM" -> 2@1
				GSUB4: "MAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "ABX",
	},
	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> not included
		Name: "4_27",
		// ALALA -> ABA -> ...
		Desc: `GSUB5: -ligs "AAA" -> 1@0 3@1
				GSUB5: "ALALA" -> 2@1
				GSUB4: "LAL" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "ALALA",
		Out: "ABX",
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: AYA -> included
		Name: "4_28",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMMA" -> 2@1
				GSUB4: "MMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMAA",
		Out: "AYA", // (!!!)
	},

	{ // harfbuzz: ABAA (!!!), Mac: ABXA, Windows: ABXA -> not included
		Name: "4_29",
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
		Name: "4_30",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMAM" -> 2@0
				GSUB4: "AMAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "YA",
	},

	{ // harfbuzz: YAA, Mac: YAA, Windows: YAA -> yes
		Name: "4_31",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@0
				GSUB5: "AMMM" -> 2@0
				GSUB4: "AMMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMMAA",
		Out: "YAA",
	},

	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> no
		Name: "4_32",
		Desc: `GSUB5: -marks "AAAA" -> 1@0 3@1
				GSUB5: "AMAAM" -> 2@1
				GSUB4: "MAAM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAAMA",
		Out: "ABX",
	},

	{ // harfbuzz: AB, Mac: AB, Windows: AY -> yes
		Name: "4_33",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAMA" -> 2@1
				GSUB4: "MAMA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMA",
		Out: "AY", // (!!!)
	},

	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> no
		Name: "4_34",
		Desc: `GSUB5: -marks "AAA" -> 1@0 3@1
				GSUB5: "AMAMM" -> 2@1
				GSUB4: "MAMM" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMAMMA",
		Out: "ABX",
	},

	{ // harfbuzz: ABA (!!!), Mac: ABX, Windows: ABX -> no
		Name: "4_35",
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
		Name: "4_36",
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
		Name: "4_37",
		Desc: `GSUB5: -marks "AAAAA" -> 1@0 3@1
				GSUB5: "AMMMAAA" -> 2@1
				GSUB4: "MMMAAA" -> "B"
				GSUB1: "A" -> "X", "B" -> "Y"`,
		In:  "AMMMAAAA",
		Out: "AYA", // (!!!)
	},

	{ // harfbuzz: ABA, Mac: ABX, Windows: AYA -> included
		Name: "4_38",
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
		Name: "4_39",
		Desc: `GSUB5: "ABC" -> 1@0 1@2 1@3 2@2
				GSUB2: "A" -> "DE", "B" -> "FG", "G" -> "H"
				GSUB4: "FHC" -> "I"`,
		In:  "ABC",
		Out: "DEI",
	},
	{ // harfbuzz: AXAAKA, Mac: AAXAKA, Windows: AAAXKA
		Name: "4_40",
		Desc: `GSUB5: -ligs "AAA" -> 1@0 2@1 3@1
				GSUB5: "AK" -> 2@1
				GSUB2: "K" -> "AA"
				GSUB1: "A" -> "X", "K" -> "X", "L" -> "X"`,
		In:  "AKAKA",
		Out: "AAAXKA", // (!!!)
	},
	{ // harfbuzz: AXLAKA, Mac: ALXAKA, Windows: ALLXKA
		Name: "4_41",
		Desc: `GSUB5: -ligs "AAA" -> 1@0 2@1 3@1
				GSUB5: "AK" -> 2@1
				GSUB2: "K" -> "LL"
				GSUB1: "A" -> "X", "K" -> "X", "L" -> "X"`,
		In:  "AKAKA",
		Out: "ALLXKA", // (!!!)
	},
	{ // harfbuzz: XAYZ, Mac: XAYZ
		Name: "4_42",
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
		Name: "4_43",
		Desc: `GSUB5: "AB" -> 1@0 0@0, "AAB" -> 1@0 0@0, "AAAB" -> 1@0 0@0
				GSUB2: "A" -> "AA"`,
		In:  "AB",
		Out: "AAAAB",
	},
	{ // harfbuzz: XYAZA, Mac: XAYZA, Windows: XAAYZ
		Name: "4_44",
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
		Name: "4_45",
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
		Name: "4_46",
		Desc: `GSUB5: -ligs "AAA" -> 1@0
				GSUB5: "AL" -> 2@1 3@1
				GSUB4: "LA" -> "K"
				GSUB1: "L" -> "B"`,
		In:  "ALAA",
		Out: "ABAA", // (!!!)
	},
	{ // harfbuzz: LXLYLZL, Mac: LXLYLZL, Windows: LXLYLZL
		Name: "4_47",
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
		Name: "4_48",
		Desc: `GSUB5: -ligs "AAA" -> 1@2
				GSUB4: "AL" -> "X"`,
		In:  "LALALALB",
		Out: "LALALXB",
	},
	{ // harfbuzz: ABXD, Mac: ABXD, Windows: ABXD -> "CL" was added to input here
		Name: "4_49",
		Desc: `GSUB5: -ligs "ABC" -> 1@2
			GSUB4: C L -> X`,
		In:  "ABCLD",
		Out: "ABXD",
	},

	{ // harfbuzz: XCEAAAAXFBCAACX, Mac: XCEAAAAXFBCAACX, Windows: XCEAAAAXFBCAACX
		Name: "4_50",
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
		Name: "4_51",
		Desc: `GSUB5:
				[A-E] [A B] [C-X] -> 1@0 ||
				[B-E] [B-E] [A-C] -> 1@1
			GSUB1: A -> X, B -> X, C -> X, D -> X, E -> X, F -> X`,
		In:  "ABCFBCEA",
		Out: "XBCFBCXA",
	},
	{ // harfbuzz: X, Mac: X, Windows: X
		Name: "4_52",
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
