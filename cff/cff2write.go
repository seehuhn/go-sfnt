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
	"io"
	"math"

	"seehuhn.de/go/geom/matrix"

	"seehuhn.de/go/sfnt/glyph"
)

// Write writes the binary form of a CFF2 font.
func (f *FontCFF2) Write(w io.Writer) error {
	o := f.OutlinesCFF2
	numGlyphs := len(o.Glyphs)
	if numGlyphs == 0 {
		return invalidSince("cff2: no glyphs")
	}
	if len(o.Private) == 0 {
		return invalidSince("cff2: no private dicts")
	}
	if len(o.Private) > maxFontDICTs {
		return invalidSince("cff2: too many Font DICTs")
	}

	// encode the charstrings, each with its Font DICT's private dict for the
	// default vsindex
	charStrings := make(cffIndex, numGlyphs)
	for gid, g := range o.Glyphs {
		fd := 0
		if o.FDSelect != nil {
			fd = o.FDSelect(glyph.ID(gid))
		}
		if fd < 0 || fd >= len(o.Private) {
			// the same indices are written to the FDSelect table below,
			// so they cannot be silently replaced here
			return invalidSince("cff2: FDSelect out of range")
		}
		code, err := encodeCharStringCFF2(g, o.Private[fd])
		if err != nil {
			return err
		}
		if len(code) > maxCharStringLen {
			return invalidSince("cff2: charstring too long")
		}
		charStrings[gid] = code
	}

	// private dicts have fixed content (no local subrs, no self offsets)
	privateBlobs := make([][]byte, len(o.Private))
	for i, p := range o.Private {
		privateBlobs[i] = makePrivateDictCFF2(p).encodeCFF2()
	}

	// FDSelect is omitted when a single Font DICT covers every glyph
	var fdSelectBlob []byte
	if fdSelectNeededCFF2(o.FDSelect, numGlyphs) {
		fdSelectBlob = encodeFDSelectCFF2(o.FDSelect, numGlyphs)
	}

	// variation store, prefixed by its uint16 byte length
	var vstoreBlob []byte
	if o.VarStore != nil {
		enc := o.VarStore.Encode()
		vstoreBlob = make([]byte, 2+len(enc))
		vstoreBlob[0] = byte(len(enc) >> 8)
		vstoreBlob[1] = byte(len(enc))
		copy(vstoreBlob[2:], enc)
	}

	// section layout: header, top DICT, global subr INDEX, vstore, FDSelect,
	// FDArray, private DICTs, charstrings INDEX
	var blobs [][]byte
	secHeader := len(blobs)
	blobs = append(blobs, make([]byte, 5))
	secTopDict := len(blobs)
	blobs = append(blobs, nil)
	blobs = append(blobs, cffIndex(nil).encode32()) // global subr INDEX (empty)

	secVStore := -1
	if vstoreBlob != nil {
		secVStore = len(blobs)
		blobs = append(blobs, vstoreBlob)
	}
	secFDSelect := -1
	if fdSelectBlob != nil {
		secFDSelect = len(blobs)
		blobs = append(blobs, fdSelectBlob)
	}
	secFDArray := len(blobs)
	blobs = append(blobs, nil)
	secPrivate := make([]int, len(privateBlobs))
	for i := range privateBlobs {
		secPrivate[i] = len(blobs)
		blobs = append(blobs, privateBlobs[i])
	}
	secCharStrings := len(blobs)
	blobs = append(blobs, charStrings.encode32())

	numSections := len(blobs)
	cumsum := func() []int32 {
		res := make([]int32, numSections+1)
		for i := range numSections {
			res[i+1] = res[i] + int32(len(blobs[i]))
		}
		return res
	}

	// two-pass offset fixup: the offsets stored in the top DICT and the Font
	// DICTs depend on the section sizes, which in turn depend on the encoded
	// offset widths; iterate until the layout stabilises
	offs := cumsum()
	for {
		fdArray := make(cffIndex, len(o.Private))
		for i := range o.Private {
			fontDict := cffDict{}
			if i < len(o.FontMatrices) && o.FontMatrices[i] != matrix.Identity {
				setFontMatrixCFF2(fontDict, o.FontMatrices[i])
			}
			fontDict[opPrivate] = []any{
				int32(len(privateBlobs[i])),
				offs[secPrivate[i]],
			}
			fdArray[i] = fontDict.encodeCFF2()
		}
		blobs[secFDArray] = fdArray.encode32()

		topDict := cffDict{}
		if f.FontMatrix != defaultFontMatrix {
			setFontMatrixCFF2(topDict, f.FontMatrix)
		}
		topDict[opCharStrings] = []any{offs[secCharStrings]}
		if secVStore >= 0 {
			topDict[opVStore] = []any{offs[secVStore]}
		}
		topDict[opFDArray] = []any{offs[secFDArray]}
		if secFDSelect >= 0 {
			topDict[opFDSelect] = []any{offs[secFDSelect]}
		}
		topBlob := topDict.encodeCFF2()
		blobs[secTopDict] = topBlob
		blobs[secHeader] = []byte{2, 0, 5, byte(len(topBlob) >> 8), byte(len(topBlob))}

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
		if _, err := w.Write(blobs[i]); err != nil {
			return err
		}
	}
	return nil
}

// setFontMatrixCFF2 stores the six FontMatrix operands as plain float DICT
// operands.
func setFontMatrixCFF2(d cffDict, fm matrix.Matrix) {
	val := make([]any, 6)
	for i, v := range fm {
		val[i] = v
	}
	d[opFontMatrix] = val
}

// makePrivateDictCFF2 builds the CFF2 Private DICT for p, emitting only the
// entries that differ from their defaults.  Array operands are re-differenced
// (the inverse of getBlendArray's prefix sum); scalar operands are stored
// directly.
func makePrivateDictCFF2(p *PrivateCFF2) cffDict {
	d := cffDict{}
	if p.VSIndex != 0 {
		d[opVSIndex] = []any{int32(p.VSIndex)}
	}
	setBlendArrayCFF2(d, opBlueValues, p.BlueValues)
	setBlendArrayCFF2(d, opOtherBlues, p.OtherBlues)
	setBlendArrayCFF2(d, opFamilyBlues, p.FamilyBlues)
	setBlendArrayCFF2(d, opFamilyOtherBlues, p.FamilyOtherBlues)
	setBlendArrayCFF2(d, opStemSnapH, p.StemSnapH)
	setBlendArrayCFF2(d, opStemSnapV, p.StemSnapV)
	setBlendScalarCFF2(d, opBlueScale, p.BlueScale, Blend{Default: 0.039625})
	setBlendScalarCFF2(d, opBlueShift, p.BlueShift, Blend{Default: 7})
	setBlendScalarCFF2(d, opBlueFuzz, p.BlueFuzz, Blend{Default: 1})
	setBlendScalarCFF2(d, opStdHW, p.StdHW, Blend{})
	setBlendScalarCFF2(d, opStdVW, p.StdVW, Blend{})
	setBlendScalarCFF2(d, opExpansionFactor, p.ExpansionFactor, Blend{Default: 0.06})
	if p.LanguageGroup != 0 {
		d[opLanguageGroup] = []any{p.LanguageGroup}
	}
	return d
}

// setBlendScalarCFF2 stores a single blendable operand, unless it equals def.
func setBlendScalarCFF2(d cffDict, op dictOp, v, def Blend) {
	if blendEqualCFF2(v, def) {
		return
	}
	d[op] = []any{blendToOperandCFF2(v)}
}

// setBlendArrayCFF2 stores a delta-encoded array of blendable operands.  Both
// the defaults and each region's deltas are re-differenced across the array so
// that getBlendArray's running sum reproduces the absolute values.
func setBlendArrayCFF2(d cffDict, op dictOp, blends []Blend) {
	if len(blends) == 0 {
		return
	}
	res := make([]any, len(blends))
	var prevDefault float64
	var prevDeltas []float64
	for i, t := range blends {
		relDefault := t.Default - prevDefault
		prevDefault = t.Default
		if len(t.Deltas) == 0 {
			res[i] = dictNumberOperandCFF2(relDefault)
			continue
		}
		m := len(t.Deltas)
		if len(prevDeltas) < m {
			grown := make([]float64, m)
			copy(grown, prevDeltas)
			prevDeltas = grown
		}
		relDeltas := make([]any, m)
		for j := range m {
			relDeltas[j] = dictNumberOperandCFF2(t.Deltas[j] - prevDeltas[j])
			prevDeltas[j] = t.Deltas[j]
		}
		res[i] = dictBlendValue{Default: dictNumberOperandCFF2(relDefault), Deltas: relDeltas}
	}
	d[op] = res
}

// blendToOperandCFF2 converts a scalar Blend into a plain or blended DICT
// operand.  Scalar operands are stored directly (getBlend does not prefix-sum).
func blendToOperandCFF2(v Blend) any {
	if len(v.Deltas) == 0 {
		return dictNumberOperandCFF2(v.Default)
	}
	deltas := make([]any, len(v.Deltas))
	for i, d := range v.Deltas {
		deltas[i] = dictNumberOperandCFF2(d)
	}
	return dictBlendValue{Default: dictNumberOperandCFF2(v.Default), Deltas: deltas}
}

// dictNumberOperandCFF2 chooses the DICT representation of a value: an integer
// operand when it is integral and fits int32, otherwise a real operand.
func dictNumberOperandCFF2(v float64) any {
	if v == math.Trunc(v) && v >= math.MinInt32 && v <= math.MaxInt32 {
		return int32(v)
	}
	return v
}

// blendEqualCFF2 reports whether two Blends are identical.
func blendEqualCFF2(a, b Blend) bool {
	if a.Default != b.Default || len(a.Deltas) != len(b.Deltas) {
		return false
	}
	for i := range a.Deltas {
		if a.Deltas[i] != b.Deltas[i] {
			return false
		}
	}
	return true
}

// fdSelectNeededCFF2 reports whether an FDSelect table must be written: it is
// needed only when some glyph maps to a Font DICT other than 0.
func fdSelectNeededCFF2(fn FDSelectFn, nGlyphs int) bool {
	if fn == nil {
		return false
	}
	for gid := range nGlyphs {
		if fn(glyph.ID(gid)) != 0 {
			return true
		}
	}
	return false
}

// encodeFDSelectCFF2 encodes the glyph-to-Font-DICT map, choosing the smallest
// of formats 0, 3 and 4.  Ties favour the lower format number.  Formats 0 and
// 3 use 16-bit glyph indices and are skipped when the font has more than 65535
// glyphs.
func encodeFDSelectCFF2(fn FDSelectFn, nGlyphs int) []byte {
	var best []byte
	consider := func(cand []byte) {
		if best == nil || len(cand) < len(best) {
			best = cand
		}
	}
	if nGlyphs <= 0xFFFF {
		consider(fdSelectFormat0(fn, nGlyphs))
		consider(fdSelectFormat3(fn, nGlyphs))
	}
	consider(fn.encodeFormat4(nGlyphs))
	return best
}

// fdSelectFormat0 encodes the map as FDSelect format 0.
func fdSelectFormat0(fn FDSelectFn, nGlyphs int) []byte {
	buf := make([]byte, nGlyphs+1)
	buf[0] = 0
	for i := range nGlyphs {
		buf[i+1] = byte(fn(glyph.ID(i)))
	}
	return buf
}

// fdSelectFormat3 encodes the map as FDSelect format 3, one range per maximal
// run of equal Font DICT indices.
func fdSelectFormat3(fn FDSelectFn, nGlyphs int) []byte {
	buf := []byte{3, 0, 0}
	currentFD := 0
	nSeg := 0
	for i := range nGlyphs {
		fd := fn(glyph.ID(i))
		if i > 0 && fd == currentFD {
			continue
		}
		buf = append(buf, byte(i>>8), byte(i), byte(fd))
		nSeg++
		currentFD = fd
	}
	buf = append(buf, byte(nGlyphs>>8), byte(nGlyphs))
	buf[1], buf[2] = byte(nSeg>>8), byte(nSeg)
	return buf
}
