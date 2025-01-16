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
)

// Write writes the binary form of a CFF font.
func (cff *Font) Write(w io.Writer) error {
	numGlyphs := uint16(len(cff.Glyphs))

	// TODO(voss): this should be done per private dict.
	charStrings, defWidth, nomWidth, err := cff.encodeCharStrings()
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
	blobs = append(blobs, cffIndex{[]byte(cff.FontInfo.FontName)}.encode())

	// section 2: top dict INDEX
	topDict := makeTopDict(cff.FontInfo)
	// opCharset is updated below
	// opCharStrings is updated below
	if cff.IsCIDKeyed() {
		// see afdko/c/shared/source/cffwrite/cffwrite_dict.c:cfwDictFillTop
		registrySID := strings.lookup(cff.ROS.Registry)
		orderingSID := strings.lookup(cff.ROS.Ordering)
		topDict[opROS] = []interface{}{
			registrySID, orderingSID, cff.ROS.Supplement,
		}
		topDict[opCIDCount] = []interface{}{int32(numGlyphs)}
		// opFDArray is updated below
		// opFDSelect is updated below
	} else {
		// opEncoding is updated below
		// opPrivate is updated below
	}
	topDict.setFontMatrix(opFontMatrix, cff.FontInfo.FontMatrix, cff.IsCIDKeyed())
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
	if cff.ROS == nil {
		glyphNames = make([]int32, numGlyphs)
		for i := uint16(0); i < numGlyphs; i++ {
			glyphNames[i] = strings.lookup(cff.Glyphs[i].Name)
		}

		if len(cff.Encoding) == 0 || isStandardEncoding(cff.Encoding, cff.Glyphs) {
			// topDict[opEncoding] = []interface{}{int32(0)}
		} else if isExpertEncoding(cff.Encoding, cff.Glyphs) {
			topDict[opEncoding] = []interface{}{int32(1)}
		} else {
			encoding, err := encodeEncoding(cff.Encoding, glyphNames)
			if err != nil {
				return err
			}
			secEncodings = len(blobs)
			blobs = append(blobs, encoding)
		}
	}

	// section 6: charsets
	var charsets []byte
	if cff.ROS == nil {
		charsets, err = encodeCharset(glyphNames)
		if err != nil {
			return fmt.Errorf("Glyph names: %w", err)
		}
	} else {
		tmp := make([]int32, len(cff.GIDToCID))
		for i, cid := range cff.GIDToCID {
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
	if cff.ROS != nil {
		secFDSelect = len(blobs)
		blobs = append(blobs, cff.FDSelect.encode(int(numGlyphs)))
	}

	// section 8: charstrings INDEX
	secCharStringsIndex := len(blobs)
	blobs = append(blobs, charStrings.encode())

	// section 9: font DICT INDEX
	numFonts := len(cff.Private)
	fontDicts := make([]cffDict, numFonts)
	if cff.ROS != nil {
		for i := range fontDicts {
			// see afdko/c/shared/source/cffwrite/cffwrite_dict.c:cfwDictFillFont
			fontDict := cffDict{}
			fontDict.setFontMatrix(opFontMatrix, cff.FontMatrices[i], false)
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
		privateDicts[i] = cff.makePrivateDict(i, defWidth, nomWidth)
		// opSubrs is set below
		secPrivateDicts[i] = len(blobs)
		blobs = append(blobs, nil)
	}

	// section 11: subrs INDEX
	// TODO(voss): only write this section, if subroutines are present?
	secSubrsIndex := len(blobs)
	blobs = append(blobs, cffIndex{}.encode())

	numSections := len(blobs)

	cumsum := func() []int32 {
		res := make([]int32, numSections+1)
		for i := 0; i < numSections; i++ {
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
		for i := 0; i < numFonts; i++ {
			secPrivateDict := secPrivateDicts[i]
			// TODO(voss): only write this key, if subroutines are present?
			privateDicts[i][opSubrs] = []interface{}{offs[secSubrsIndex] - offs[secPrivateDict]}
			blobs[secPrivateDict] = privateDicts[i].encode(strings)
			pdSize := len(blobs[secPrivateDict])
			pdDesc := []interface{}{int32(pdSize), offs[secPrivateDict]}
			if cff.ROS != nil {
				fontDicts[i][opPrivate] = pdDesc
				fontDictData := fontDicts[i].encode(strings)
				fontDictIndex = append(fontDictIndex, fontDictData)
			} else {
				topDict[opPrivate] = pdDesc
			}
		}
		if cff.ROS != nil {
			blobs[secFontDictIndex] = fontDictIndex.encode()
		}

		topDict[opCharset] = []interface{}{offs[secCharsets]}
		if secEncodings >= 4 {
			topDict[opEncoding] = []interface{}{offs[secEncodings]}
		}
		topDict[opCharStrings] = []interface{}{offs[secCharStringsIndex]}
		if secFDSelect >= 0 {
			topDict[opFDSelect] = []interface{}{offs[secFDSelect]}
			topDict[opFDArray] = []interface{}{offs[secFontDictIndex]}
		}
		topDictData := topDict.encode(strings)
		blobs[secTopDictIndex] = cffIndex{topDictData}.encode()

		blobs[secStringIndex] = strings.encode()

		newOffs := cumsum()
		done := true
		for i := 0; i < numSections; i++ {
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

	for i := 0; i < numSections; i++ {
		_, err = w.Write(blobs[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func (cff *Font) selectWidths() (float64, float64) {
	numGlyphs := int32(len(cff.Glyphs))
	if numGlyphs == 0 {
		return 0, 0
	} else if numGlyphs == 1 {
		return cff.Glyphs[0].Width, cff.Glyphs[0].Width
	}

	widthHist := make(map[float64]int32)
	var mostFrequentCount int32
	var defaultWidth float64
	for _, glyph := range cff.Glyphs {
		w := glyph.Width
		widthHist[w]++
		if widthHist[w] > mostFrequentCount {
			defaultWidth = w
			mostFrequentCount = widthHist[w]
		}
	}

	// TODO(voss): the choice of nominalWidth can be improved
	var sum float64
	var minWidth float64 = math.Inf(+1)
	var maxWidth float64 = math.Inf(-1)
	for _, glyph := range cff.Glyphs {
		w := glyph.Width
		if w == defaultWidth {
			continue
		}
		sum += w
		if w < minWidth {
			minWidth = w
		}
		if w > maxWidth {
			maxWidth = w
		}
	}
	nominalWidth := math.Round(sum / float64(numGlyphs))
	if nominalWidth < minWidth+107 {
		nominalWidth = minWidth + 107
	} else if nominalWidth > maxWidth-107 {
		nominalWidth = maxWidth - 107
	}
	return defaultWidth, nominalWidth
}

func (cff *Font) encodeCharStrings() (cffIndex, float64, float64, error) {
	numGlyphs := len(cff.Glyphs)
	if numGlyphs < 1 || (cff.ROS == nil && cff.Glyphs[0].Name != ".notdef") {
		return nil, 0, 0, invalidSince("missing .notdef glyph")
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

	cc := make(cffIndex, numGlyphs)
	defaultWidth, nominalWidth := cff.selectWidths()
	for i, glyph := range cff.Glyphs {
		code, err := glyph.encodeCharString(defaultWidth, nominalWidth)
		if err != nil {
			return nil, 0, 0, err
		}
		cc[i] = code
	}

	return cc, defaultWidth, nominalWidth, nil
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
