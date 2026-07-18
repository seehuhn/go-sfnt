// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
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
	"errors"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/membudget"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// maxFontDICTs bounds the number of Font DICTs in a CFF2 FDArray.  The
// FDSelect fd index is at most 16 bits, but the FDArray INDEX in practice
// never exceeds a handful of entries.
const maxFontDICTs = 256

// ReadCFF2 reads a CFF2 font from r.  Allocations are charged against budget.
func ReadCFF2(r parser.ReadSeekSizer, budget *membudget.Budget) (*FontCFF2, error) {
	p := parser.New(r, budget)

	// section 0: header
	major, err := p.ReadUint8()
	if err != nil {
		return nil, err
	}
	if major != 2 {
		return nil, invalidSince("cff2: not a CFF2 font")
	}
	if _, err := p.ReadUint8(); err != nil { // minor version, ignored
		return nil, err
	}
	headerSize, err := p.ReadUint8()
	if err != nil {
		return nil, err
	}
	if headerSize < 5 {
		return nil, invalidSince("cff2: invalid header size")
	}
	topDictLength, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}

	// section 1: top DICT, located at headerSize
	if int64(headerSize)+int64(topDictLength) > p.Size() {
		return nil, invalidSince("cff2: top DICT exceeds input size")
	}
	if err := p.SeekPos(int64(headerSize)); err != nil {
		return nil, err
	}
	topDictBlob, err := p.ReadBlob(int(topDictLength))
	if err != nil {
		return nil, err
	}
	// blend is illegal at the top level: any blend operator fails the read.
	topDict, err := decodeDictCFF2(topDictBlob, rejectBlend)
	if err != nil {
		return nil, err
	}

	// section 2: global subr INDEX, immediately after the top DICT data
	gsubrs, err := readIndex32(p)
	if err != nil {
		return nil, err
	}

	// section 3: variation store (optional)
	var varStore *variation.ItemVariationStore
	if vstoreOffs := topDict.getInt(opVStore, 0); vstoreOffs != 0 {
		if vstoreOffs < 4 || int64(vstoreOffs)+2 > p.Size() {
			return nil, invalidSince("cff2: invalid vstore offset")
		}
		if err := p.SeekPos(int64(vstoreOffs)); err != nil {
			return nil, err
		}
		if _, err := p.ReadUint16(); err != nil { // store length, informational
			return nil, err
		}
		varStore, err = variation.ReadItemVariationStore(p, int64(vstoreOffs)+2)
		if err != nil {
			return nil, err
		}
	}

	// regionCount reports the number of variation regions of the item
	// variation data subtable selected by a vsindex.  Without a variation
	// store, any blend is malformed, so every lookup fails.
	regionCount := func(vsindex int) (int, error) {
		if varStore == nil {
			return 0, errNoVarStore
		}
		if vsindex < 0 || vsindex >= len(varStore.Data) {
			return 0, errBadVSIndex
		}
		return len(varStore.Data[vsindex].RegionIndexes), nil
	}

	// section 4: FDArray of Font DICTs (required per spec; synthesised when
	// absent, for permissive reading)
	var privates []*PrivateCFF2
	var fontMatrices []matrix.Matrix
	var localSubrs []cffIndex

	fdArrayOffs := topDict.getInt(opFDArray, 0)
	if fdArrayOffs >= 4 && int64(fdArrayOffs) < p.Size() {
		fdArray, err := readIndex32At(p, int64(fdArrayOffs), "FDArray")
		if err != nil {
			return nil, err
		}
		if len(fdArray) > maxFontDICTs {
			return nil, invalidSince("cff2: too many Font DICTs")
		}
		for _, fdBlob := range fdArray {
			fontDict, err := decodeDictCFF2(fdBlob, regionCount)
			if err != nil {
				return nil, err
			}
			priv, subrs, err := readPrivateCFF2(p, fontDict, regionCount)
			if err != nil {
				return nil, err
			}
			privates = append(privates, priv)
			localSubrs = append(localSubrs, subrs)
			fontMatrices = append(fontMatrices, getFontMatrixCFF2(fontDict, matrix.Identity))
		}
	}
	if len(privates) == 0 {
		// synthesise a single default Font DICT
		privates = append(privates, newPrivateCFF2())
		localSubrs = append(localSubrs, nil)
		fontMatrices = append(fontMatrices, matrix.Identity)
	}

	// section 5: CharStrings INDEX
	charStringsOffs := topDict.getInt(opCharStrings, 0)
	if charStringsOffs == 0 {
		return nil, invalidSince("cff2: missing CharStrings")
	}
	charStrings, err := readIndex32At(p, int64(charStringsOffs), "CharStrings")
	if err != nil {
		return nil, err
	}
	nGlyphs := len(charStrings)
	if nGlyphs == 0 {
		return nil, invalidSince("cff2: no charstrings")
	}

	// section 6: FDSelect (optional; absent means all glyphs use FD 0)
	fdSelect := fdSelectSimple
	if fdSelectOffs := topDict.getInt(opFDSelect, 0); fdSelectOffs != 0 {
		if fdSelectOffs < 4 || int64(fdSelectOffs) >= p.Size() {
			return nil, invalidSince("cff2: invalid FDSelect offset")
		}
		if err := p.SeekPos(int64(fdSelectOffs)); err != nil {
			return nil, err
		}
		fdSelect, err = readFDSelect(p, nGlyphs, len(privates), true)
		if err != nil {
			return nil, err
		}
	}

	// section 7: decode the charstrings
	decoders := make([]*decodeInfoCFF2, len(privates))
	for i := range privates {
		decoders[i] = &decodeInfoCFF2{
			subr:           localSubrs[i],
			gsubr:          gsubrs,
			defaultVSIndex: privates[i].VSIndex,
			regionCount:    regionCount,
			budget:         p.Budget,
		}
	}

	glyphs, err := membudget.AllocSlice[*GlyphCFF2](p.Budget, nGlyphs)
	if err != nil {
		return nil, err
	}
	for gid, code := range charStrings {
		fdIdx := fdSelect(glyph.ID(gid))
		g, err := decoders[fdIdx].decodeCharStringCFF2(code)
		if err != nil {
			return nil, err
		}
		glyphs[gid] = g
	}

	return &FontCFF2{
		FontMatrix: getFontMatrixCFF2(topDict, defaultFontMatrix),
		OutlinesCFF2: &OutlinesCFF2{
			Glyphs:       glyphs,
			Private:      privates,
			FDSelect:     fdSelect,
			FontMatrices: fontMatrices,
			VarStore:     varStore,
		},
	}, nil
}

// readIndex32At seeks to pos and reads a CFF2 INDEX (uint32 count) from there.
func readIndex32At(p *parser.Parser, pos int64, name string) (cffIndex, error) {
	if pos < 4 || pos >= p.Size() {
		return nil, errors.New("cff2: missing " + name + " INDEX")
	}
	if err := p.SeekPos(pos); err != nil {
		return nil, err
	}
	return readIndex32(p)
}

// readPrivateCFF2 reads a CFF2 Private DICT referenced by the (size, offset)
// pair of fontDict, together with its local Subrs INDEX.
func readPrivateCFF2(p *parser.Parser, fontDict cffDict, regionCount func(int) (int, error)) (*PrivateCFF2, cffIndex, error) {
	pdSize, pdOffs, ok := fontDict.getPair(opPrivate)
	if !ok || pdOffs < 4 || pdSize < 0 || int64(pdSize) > p.Size()-int64(pdOffs) {
		return nil, nil, invalidSince("cff2: invalid Private DICT")
	}

	if err := p.SeekPos(int64(pdOffs)); err != nil {
		return nil, nil, err
	}
	blob, err := p.ReadBlob(int(pdSize))
	if err != nil {
		return nil, nil, err
	}

	pd, err := decodeDictCFF2(blob, regionCount)
	if err != nil {
		return nil, nil, err
	}

	priv := newPrivateCFF2()
	priv.BlueValues = toBlends(pd.getBlendArray(opBlueValues))
	priv.OtherBlues = toBlends(pd.getBlendArray(opOtherBlues))
	priv.FamilyBlues = toBlends(pd.getBlendArray(opFamilyBlues))
	priv.FamilyOtherBlues = toBlends(pd.getBlendArray(opFamilyOtherBlues))
	priv.StemSnapH = toBlends(pd.getBlendArray(opStemSnapH))
	priv.StemSnapV = toBlends(pd.getBlendArray(opStemSnapV))
	if rb, ok := pd.getBlend(opBlueScale); ok {
		priv.BlueScale = toBlend(rb)
	}
	if rb, ok := pd.getBlend(opBlueShift); ok {
		priv.BlueShift = toBlend(rb)
	}
	if rb, ok := pd.getBlend(opBlueFuzz); ok {
		priv.BlueFuzz = toBlend(rb)
	}
	if rb, ok := pd.getBlend(opStdHW); ok {
		priv.StdHW = toBlend(rb)
	}
	if rb, ok := pd.getBlend(opStdVW); ok {
		priv.StdVW = toBlend(rb)
	}
	if rb, ok := pd.getBlend(opExpansionFactor); ok {
		priv.ExpansionFactor = toBlend(rb)
	}
	priv.LanguageGroup = pd.getInt(opLanguageGroup, 0)
	priv.VSIndex = int(pd.getInt(opVSIndex, 0))

	var subrs cffIndex
	if subrsOffs := pd.getInt(opSubrs, 0); subrsOffs > 0 {
		subrs, err = readIndex32At(p, int64(pdOffs)+int64(subrsOffs), "Subrs")
		if err != nil {
			return nil, nil, err
		}
	}

	return priv, subrs, nil
}

// getFontMatrixCFF2 reads the six FontMatrix operands from a CFF2 DICT,
// resolving blended operands to their default values.  It returns def when the
// operator is absent or malformed.
func getFontMatrixCFF2(d cffDict, def matrix.Matrix) matrix.Matrix {
	xx := d[opFontMatrix]
	if len(xx) != 6 {
		return def
	}
	var res matrix.Matrix
	for i, v := range xx {
		res[i] = toFloat(plainOperand(v))
	}
	return res
}

// toBlend converts a resolved DICT blend value into a Blend.
func toBlend(rb resolvedBlend) Blend {
	b := Blend{Default: rb.Default}
	if len(rb.Deltas) > 0 {
		b.Deltas = append([]float64(nil), rb.Deltas...)
	}
	return b
}

// toBlends converts a slice of resolved DICT blend values into Blends.
func toBlends(rbs []resolvedBlend) []Blend {
	if len(rbs) == 0 {
		return nil
	}
	res := make([]Blend, len(rbs))
	for i, rb := range rbs {
		res[i] = toBlend(rb)
	}
	return res
}

// rejectBlend is a regionCount callback that always fails.  It is used for the
// CFF2 top DICT, where the blend operator is not permitted: the callback is
// consulted only when a blend operator is encountered.
func rejectBlend(int) (int, error) {
	return 0, errTopBlend
}

var (
	errNoVarStore = invalidSince("cff2: blend without variation store")
	errTopBlend   = invalidSince("cff2: blend in top DICT")
)
