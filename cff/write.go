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
	"io"
	"math"
	"slices"
	"sort"

	"seehuhn.de/go/postscript/type1"
	"seehuhn.de/go/sfnt/glyph"
)

// maxGlyphs is the largest number of glyphs a CFF font can contain, since
// the CharStrings INDEX stores the glyph count in a 16-bit field.
const maxGlyphs = 65535

// maxCharStringLen is the largest charstring, in bytes, a Type 2 charstring
// interpreter must support.  The limit applies to both CFF and CFF2.
const maxCharStringLen = 65535

// Write writes the binary form of a CFF font.
func (f *Font) Write(w io.Writer) error {
	if err := type1.CheckFontName(f.FontInfo.FontName); err != nil {
		return err
	}
	if len(f.Glyphs) > maxGlyphs {
		return invalidSince("too many glyphs")
	}
	if f.ROS == nil && len(f.Private) != 1 {
		// the Top DICT holds a single Private entry
		return invalidSince("wrong number of private dicts")
	}
	if len(f.Private) > maxFontDICTs {
		return invalidSince("too many Font DICTs")
	}
	numGlyphs := uint16(len(f.Glyphs))

	charStrings, defWidths, nomWidths, err := f.encodeCharStrings()
	if err != nil {
		return err
	}

	var blobs [][]byte
	strings := &cffStrings{}

	// section 0: Header
	secHeader := len(blobs)
	blobs = append(blobs, []byte{
		1, // major
		0, // minor
		4, // hdrSize
		4, // offSize (updated below)
	})

	// section 1: Name INDEX
	blobs = append(blobs, cffIndex{[]byte(f.FontInfo.FontName)}.encode())

	// section 2: top dict INDEX
	topDict := makeTopDict(f.FontInfo)
	// opCharset is updated below
	// opCharStrings is updated below
	if f.IsCIDKeyed() {
		// see afdko/c/shared/source/cffwrite/cffwrite_dict.c:cfwDictFillTop
		registrySID := strings.lookup(f.ROS.Registry)
		orderingSID := strings.lookup(f.ROS.Ordering)
		topDict[opROS] = []any{
			registrySID, orderingSID, f.ROS.Supplement,
		}
		topDict[opCIDCount] = []any{int32(numGlyphs)}
		// opFDArray is updated below
		// opFDSelect is updated below
	} else {
		// opEncoding is updated below
		// opPrivate is updated below
	}
	topDict.setFontMatrix(opFontMatrix, f.FontInfo.FontMatrix, f.IsCIDKeyed())
	secTopDictIndex := len(blobs)
	blobs = append(blobs, nil)

	// section 3: string INDEX
	// The new string index is stored in `strings`.
	// We encode the blob below, once all strings are known.
	secStringIndex := len(blobs)
	blobs = append(blobs, nil)

	// section 4: global subr INDEX
	blobs = append(blobs, cffIndex{}.encode())

	// section 5: encodings
	secEncodings := -1
	var glyphNames []int32
	if f.ROS == nil {
		glyphNames = make([]int32, numGlyphs)
		for i := range numGlyphs {
			glyphNames[i] = strings.lookup(f.Glyphs[i].Name)
		}

		if len(f.Encoding) == 0 || isStandardEncoding(f.Encoding, f.Glyphs) {
			// topDict[opEncoding] = []interface{}{int32(0)}
		} else if isExpertEncoding(f.Encoding, f.Glyphs) {
			topDict[opEncoding] = []any{int32(1)}
		} else {
			encoding, err := encodeEncoding(f.Encoding, glyphNames)
			if err != nil {
				return err
			}
			secEncodings = len(blobs)
			blobs = append(blobs, encoding)
		}
	}

	// section 6: charsets
	var charsets []byte
	if f.ROS == nil {
		charsets, err = encodeCharset(glyphNames)
		if err != nil {
			return fmt.Errorf("Glyph names: %w", err)
		}
	} else {
		tmp := make([]int32, len(f.GIDToCID))
		for i, cid := range f.GIDToCID {
			tmp[i] = int32(cid)
		}
		charsets, err = encodeCharset(tmp)
		if err != nil {
			return fmt.Errorf("Gid2Cid: %w", err)
		}
	}
	secCharsets := len(blobs)
	blobs = append(blobs, charsets)

	// section 7: FDSelect
	secFDSelect := -1
	if f.ROS != nil {
		fdSelect, err := f.FDSelect.encode(int(numGlyphs), len(f.Private))
		if err != nil {
			return err
		}
		secFDSelect = len(blobs)
		blobs = append(blobs, fdSelect)
	}

	// section 8: charstrings INDEX
	secCharStringsIndex := len(blobs)
	blobs = append(blobs, charStrings.encode())

	// section 9: font DICT INDEX
	numFonts := len(f.Private)
	fontDicts := make([]cffDict, numFonts)
	if f.ROS != nil {
		for i := range fontDicts {
			// see afdko/c/shared/source/cffwrite/cffwrite_dict.c:cfwDictFillFont
			fontDict := cffDict{}
			fontDict.setFontMatrix(opFontMatrix, f.FontMatrices[i], false)
			if err := fontDict.checkReals(); err != nil {
				return err
			}
			// opPrivate is set below
			fontDicts[i] = fontDict
		}
	}
	secFontDictIndex := len(blobs)
	blobs = append(blobs, nil)

	// section 10: private DICT
	privateDicts := make([]cffDict, numFonts)
	secPrivateDicts := make([]int, numFonts)
	for i := range privateDicts {
		var err error
		privateDicts[i], err = f.makePrivateDict(i, defWidths[i], nomWidths[i])
		if err != nil {
			return err
		}
		// opSubrs is set below
		secPrivateDicts[i] = len(blobs)
		blobs = append(blobs, nil)
	}

	// The dicts hold all their content by now; the layout loop below only
	// adds offsets.
	if err := topDict.checkReals(); err != nil {
		return err
	}

	// section 11: subrs INDEX
	// TODO(voss): only write this section, if subroutines are present?
	secSubrsIndex := len(blobs)
	blobs = append(blobs, cffIndex{}.encode())

	numSections := len(blobs)

	cumsum := func() []int32 {
		res := make([]int32, numSections+1)
		for i := range numSections {
			res[i+1] = res[i] + int32(len(blobs[i]))
		}
		return res
	}

	offs := cumsum()
	for {
		// This loop terminates because the elements of offs are monotonically
		// increasing.

		blobs[secHeader][3] = offsSize(offs[numSections])

		var fontDictIndex cffIndex
		for i := range numFonts {
			secPrivateDict := secPrivateDicts[i]
			// TODO(voss): only write this key, if subroutines are present?
			privateDicts[i][opSubrs] = []any{offs[secSubrsIndex] - offs[secPrivateDict]}
			blobs[secPrivateDict] = privateDicts[i].encode(strings)
			pdSize := len(blobs[secPrivateDict])
			pdDesc := []any{int32(pdSize), offs[secPrivateDict]}
			if f.ROS != nil {
				fontDicts[i][opPrivate] = pdDesc
				fontDictData := fontDicts[i].encode(strings)
				fontDictIndex = append(fontDictIndex, fontDictData)
			} else {
				topDict[opPrivate] = pdDesc
			}
		}
		if f.ROS != nil {
			blobs[secFontDictIndex] = fontDictIndex.encode()
		}

		topDict[opCharset] = []any{offs[secCharsets]}
		if secEncodings >= 4 {
			topDict[opEncoding] = []any{offs[secEncodings]}
		}
		topDict[opCharStrings] = []any{offs[secCharStringsIndex]}
		if secFDSelect >= 0 {
			topDict[opFDSelect] = []any{offs[secFDSelect]}
			topDict[opFDArray] = []any{offs[secFontDictIndex]}
		}
		topDictData := topDict.encode(strings)
		blobs[secTopDictIndex] = cffIndex{topDictData}.encode()

		blobs[secStringIndex], err = strings.encode()
		if err != nil {
			return err
		}

		newOffs := cumsum()
		done := true
		for i := range numSections {
			if newOffs[i] != offs[i] {
				done = false
				break
			}
		}
		if done {
			break
		}

		offs = newOffs
	}

	for i := range numSections {
		_, err = w.Write(blobs[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// selectWidths chooses the defaultWidthX and nominalWidthX entries of a
// private dictionary for the given glyphs.  The default width is the most
// frequent integer width, so that as many glyphs as possible omit the width
// operand, and the nominal width is the integer which lets the most of the
// remaining integer widths use the shortest operand encodings.  Ties are
// broken towards the smaller value.
//
// An error is returned if a glyph has a width [ValidWidth] rejects, or if
// the non-default widths are spread too far for one nominal width to bring
// them all into the range of a charstring number.
func selectWidths(glyphs []*Glyph) (defaultWidth, nominalWidth float64, err error) {
	hist := make(map[float64]int)
	var bestCount int
	for _, g := range glyphs {
		w := g.Width
		if !ValidWidth(w) {
			return 0, 0, errWidthRange
		}
		if w != math.Trunc(w) {
			continue
		}
		hist[w]++
		if c := hist[w]; c > bestCount || c == bestCount && w < defaultWidth {
			defaultWidth = w
			bestCount = c
		}
	}

	// A fractional width needs the five-byte operand whatever the nominal
	// width is, so only the integer widths take part in the scoring below.
	var widths, scored []float64
	for _, g := range glyphs {
		if g.Width != defaultWidth {
			widths = append(widths, g.Width)
			if g.Width == math.Trunc(g.Width) {
				scored = append(scored, g.Width)
			}
		}
	}
	if len(widths) == 0 {
		return defaultWidth, 0, nil
	}
	slices.Sort(widths)
	slices.Sort(scored)

	// A width w is encodable relative to n if -32768 <= w-n < 32768.
	// Integers n in [lo, hi] satisfy this for every width; the DICT integer
	// range is a further restriction.
	lo := math.Ceil(widths[len(widths)-1] - 32768 + 1.0/65536)
	hi := math.Floor(widths[0] + 32768)
	lo = max(lo, math.MinInt32)
	hi = min(hi, math.MaxInt32)
	if lo > hi {
		return 0, 0, errWidthSpread
	}
	if len(scored) == 0 {
		return defaultWidth, min(max(0, lo), hi), nil
	}

	// The number of scored widths within r of n is the number of them in
	// [n-r, n+r].
	count := func(n, r float64) int {
		i, _ := slices.BinarySearch(scored, n-r)
		j := sort.Search(len(scored), func(k int) bool { return scored[k] > n+r })
		return j - i
	}

	// The counts are piecewise constant in n and change only where a
	// window edge passes a width, so the candidates below reach every
	// value, each at the smallest n where it is taken.
	candidates := []float64{lo}
	for _, w := range scored {
		for _, r := range []float64{107, 1131} {
			candidates = append(candidates, math.Ceil(w-r), math.Floor(w+r)+1)
		}
	}
	slices.Sort(candidates)
	bestOne, bestTwo := -1, -1
	for _, n := range candidates {
		if n < lo || n > hi {
			continue
		}
		one := count(n, 107)
		if one < bestOne {
			continue
		}
		two := count(n, 1131)
		if one > bestOne || two > bestTwo {
			nominalWidth = n
			bestOne, bestTwo = one, two
		}
	}
	return defaultWidth, nominalWidth, nil
}

// encodeCharStrings encodes the glyphs and returns the default and nominal
// widths chosen for each private dictionary.
func (f *Font) encodeCharStrings() (cffIndex, []float64, []float64, error) {
	numGlyphs := len(f.Glyphs)
	if numGlyphs < 1 || (f.ROS == nil && f.Glyphs[0].Name != ".notdef") {
		return nil, nil, nil, invalidSince("missing .notdef glyph")
	}

	// TODO(voss): re-introduce subroutines.
	//
	// Size used for a subroutine:
	//   - an entry in the subrs and gsubrs INDEX takes
	//     up to 4 bytes, plus the size of the subroutine
	//   - the subrouting must finish with t2return
	//     or t2endchar (1 byte)
	//   - calling the subroutine uses k+1 bytes, where
	//     k=1 for the first 215 subroutines of each type, and
	//     k=2 for the next 2048 subroutines of each type.
	// An approximation could be the following:
	//   - if n bytes occur k times, this uses n*k bytes
	//   - if the n bytes are turned into a subroutine, this uses
	//     approximately k*2 + n + 3 or k*3 + n + 4 bytes.
	//   - the savings are n*k - k*2 - n - 3 = (n-2)*(k-1)-5
	//     or n*k - k*3 - n - 4 = (n-3)*(k-1)-7 bytes.
	//
	// http://www.allisons.org/ll/AlgDS/Tree/Suffix/
	// https://stackoverflow.com/questions/9452701/ukkonens-suffix-tree-algorithm-in-plain-english

	numFonts := len(f.Private)
	fdOf := make([]int, numGlyphs)
	byFD := make([][]*Glyph, numFonts)
	for gid, g := range f.Glyphs {
		fd := 0
		if f.ROS != nil {
			fd = f.FDSelect(glyph.ID(gid))
			if fd < 0 || fd >= numFonts {
				return nil, nil, nil, invalidSince("FDSelect index out of range")
			}
		}
		fdOf[gid] = fd
		byFD[fd] = append(byFD[fd], g)
	}
	defaultWidths := make([]float64, numFonts)
	nominalWidths := make([]float64, numFonts)
	for fd, glyphs := range byFD {
		var err error
		defaultWidths[fd], nominalWidths[fd], err = selectWidths(glyphs)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	cc := make(cffIndex, numGlyphs)
	for i, g := range f.Glyphs {
		fd := fdOf[i]
		code, err := g.encodeCharString(defaultWidths[fd], nominalWidths[fd])
		if err != nil {
			return nil, nil, nil, err
		}
		if len(code) > maxCharStringLen {
			return nil, nil, nil, invalidSince("charstring too long")
		}
		cc[i] = code
	}

	return cc, defaultWidths, nominalWidths, nil
}

func offsSize(i int32) byte {
	switch {
	case i < 1<<8:
		return 1
	case i < 1<<16:
		return 2
	case i < 1<<24:
		return 3
	default:
		return 4
	}
}
