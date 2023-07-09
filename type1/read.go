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

package type1

import (
	"errors"
	"fmt"
	"io"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"seehuhn.de/go/postscript"
	"seehuhn.de/go/sfnt/funit"
)

func Read(r io.Reader) (*Font, error) {
	intp := postscript.NewInterpreter()
	err := intp.Execute(r)
	if err != nil {
		return nil, err
	}
	if len(intp.Fonts) != 1 {
		return nil, errors.New("expected exactly one font in file")
	}

	var key postscript.Name
	var fd postscript.Dict
	for key, fd = range intp.Fonts {
		break
	}
	fontType, ok := fd["FontType"].(postscript.Integer)
	if !ok || fontType != 1 {
		return nil, errors.New("wrong FontType")
	}

	var fontName postscript.Name
	if fd["FontName"] == nil {
		fontName = key
	} else if n, ok := fd["FontName"].(postscript.Name); ok {
		fontName = n
	}

	fontInfo, ok := fd["FontInfo"].(postscript.Dict)
	if !ok {
		return nil, errors.New("invalid FontInfo")
	}

	Version, _ := fontInfo["version"].(postscript.String)
	Notice, _ := fontInfo["Notice"].(postscript.String)
	Copyright, _ := fontInfo["Copyright"].(postscript.String)
	FullName, _ := fontInfo["FullName"].(postscript.String)
	FamilyName, _ := fontInfo["FamilyName"].(postscript.String)
	Weight, _ := fontInfo["Weight"].(postscript.String)
	ItalicAngle, ok := fontInfo["ItalicAngle"].(postscript.Real)
	if !ok {
		if i, ok := fontInfo["ItalicAngle"].(postscript.Integer); ok {
			ItalicAngle = postscript.Real(i)
		}
	}
	IsFixedPitch, _ := fontInfo["isFixedPitch"].(postscript.Boolean)
	// TODO(voss): change these to reals?
	UnderlinePosition, _ := fontInfo["UnderlinePosition"].(postscript.Integer)
	UnderlineThickness, _ := fontInfo["UnderlineThickness"].(postscript.Integer)

	fontMatrixArray, ok := fd["FontMatrix"].(postscript.Array)
	if !ok || len(fontMatrixArray) != 6 {
		return nil, errors.New("invalid FontMatrix")
	}
	fontMatrix := make([]float64, 6)
	for i, v := range fontMatrixArray {
		vReal, ok := v.(postscript.Real)
		if ok {
			fontMatrix[i] = float64(vReal)
			continue
		}
		vInt, ok := v.(postscript.Integer)
		if ok {
			fontMatrix[i] = float64(vInt)
			continue
		}
		return nil, errors.New("invalid FontMatrix")
	}

	fi := &FontInfo{
		FontName:           string(fontName),
		Version:            string(Version),
		Notice:             string(Notice),
		Copyright:          string(Copyright),
		FullName:           string(FullName),
		FamilyName:         string(FamilyName),
		Weight:             string(Weight),
		ItalicAngle:        float64(ItalicAngle),
		IsFixedPitch:       bool(IsFixedPitch),
		UnderlinePosition:  funit.Int16(UnderlinePosition),
		UnderlineThickness: funit.Int16(UnderlineThickness),
		FontMatrix:         fontMatrix,
	}

	pd, ok := fd["Private"].(postscript.Dict)
	if !ok {
		return nil, errors.New("missing/invalid Private dictionary")
	}
	blueValuesArray, ok := pd["BlueValues"].(postscript.Array)
	if !ok {
		return nil, errors.New("missing/invalid BlueValues array")
	}
	blueValues := make([]funit.Int16, len(blueValuesArray))
	for i, v := range blueValuesArray {
		vInt, ok := v.(postscript.Integer)
		if !ok {
			return nil, errors.New("invalid BlueValues array")
		}
		blueValues[i] = funit.Int16(vInt)
	}
	var otherBlues []funit.Int16 // optional
	otherBluesArray, ok := pd["OtherBlues"].(postscript.Array)
	if ok {
		otherBlues = make([]funit.Int16, len(otherBluesArray))
		for i, v := range otherBluesArray {
			vInt, ok := v.(postscript.Integer)
			if !ok {
				otherBlues = nil
				break
			}
			otherBlues[i] = funit.Int16(vInt)
		}
	}
	var blueScale float64 // optional
	blueScaleReal, ok := pd["BlueScale"].(postscript.Real)
	if ok {
		blueScale = float64(blueScaleReal)
	}
	var blueShift int32 // optional
	blueShiftInt, ok := pd["BlueShift"].(postscript.Integer)
	if ok {
		blueShift = int32(blueShiftInt)
	}
	var blueFuzz int32 // optional
	blueFuzzInt, ok := pd["BlueFuzz"].(postscript.Integer)
	if ok {
		blueFuzz = int32(blueFuzzInt)
	}
	var stdHW float64
	stdHWArray, ok := pd["StdHW"].(postscript.Array)
	if ok && len(stdHWArray) == 1 {
		stdHWReal, ok := stdHWArray[0].(postscript.Real)
		if ok {
			stdHW = float64(stdHWReal)
		}
	}
	var stdVW float64
	stdVWArray, ok := pd["StdVW"].(postscript.Array)
	if ok && len(stdVWArray) == 1 {
		stdVWReal, ok := stdVWArray[0].(postscript.Real)
		if ok {
			stdVW = float64(stdVWReal)
		}
	}
	forceBold := false
	forceBoldBool, ok := pd["ForceBold"].(postscript.Boolean)
	if ok {
		forceBold = bool(forceBoldBool)
	}

	// TODO(voss): StemSnapH, StemSnapV

	var encoding []string
	if enc, _ := fd["Encoding"].(postscript.Array); len(enc) == 256 {
		encoding = make([]string, 256)
		for i, glyphNameObj := range enc {
			glyphName, ok := glyphNameObj.(postscript.Name)
			if !ok {
				return nil, errors.New("invalid Encoding array")
			}
			encoding[i] = string(glyphName)
		}
		fmt.Println("found encoding")
	}

	private := &PrivateDict{
		BlueValues: blueValues,
		OtherBlues: otherBlues,
		BlueScale:  blueScale,
		BlueShift:  blueShift,
		BlueFuzz:   blueFuzz,
		StdHW:      stdHW,
		StdVW:      stdVW,
		ForceBold:  forceBold,
	}

	// =============================================================

	lenIV, ok := pd["lenIV"].(postscript.Integer)
	if !ok {
		lenIV = 4
	}

	ctx := &decodeInfo{}
	if subrs, ok := pd["Subrs"].(postscript.Array); ok {
		for _, cipherObj := range subrs {
			cipher, ok := cipherObj.(postscript.String)
			if !ok {
				ctx.subrs = append(ctx.subrs, nil)
				continue
			}
			plain := deobfuscateCharstring(cipher, int(lenIV))
			ctx.subrs = append(ctx.subrs, plain)
		}
	}

	cs, ok := fd["CharStrings"].(postscript.Dict)
	if !ok {
		return nil, errors.New("missing/invalid CharStrings dictionary")
	}
	names := maps.Keys(cs)
	slices.Sort(names)
	glyphs := make(map[string]*Glyph)
	for _, name := range names {
		obfuscated, ok := cs[name].(postscript.String)
		if !ok || len(obfuscated) < 4 {
			fmt.Println("warning: skipping non-string glyph", name)
			continue
		}
		fmt.Println(name, len(obfuscated))
		plain := deobfuscateCharstring(obfuscated, int(lenIV))
		glyph, err := ctx.decodeCharString(plain, string(name))
		if err != nil {
			return nil, err
		}
		fmt.Println()

		glyphs[string(name)] = glyph
	}

	for _, seac := range ctx.seacs {
		base := glyphs[encoding[byte(seac.base)]]
		accent := glyphs[encoding[byte(seac.accent)]]
		if base == nil || accent == nil {
			continue
		}
		g := glyphs[seac.name]
		g.Cmds = append(g.Cmds[:0], base.Cmds...)
		for _, cmd := range accent.Cmds {
			switch cmd.Op {
			case OpMoveTo:
				g.Cmds = append(g.Cmds, GlyphOp{
					Op:   OpMoveTo,
					Args: []float64{cmd.Args[0] + seac.dx, cmd.Args[1] + seac.dy},
				})
			case OpLineTo:
				g.Cmds = append(g.Cmds, GlyphOp{
					Op:   OpLineTo,
					Args: []float64{cmd.Args[0] + seac.dx, cmd.Args[1] + seac.dy},
				})
			case OpCurveTo:
				g.Cmds = append(g.Cmds, GlyphOp{
					Op: OpCurveTo,
					Args: []float64{
						cmd.Args[0] + seac.dx, cmd.Args[1] + seac.dy,
						cmd.Args[2] + seac.dx, cmd.Args[3] + seac.dy,
						cmd.Args[4] + seac.dx, cmd.Args[5] + seac.dy,
					},
				})
			}
		}
		g.HStem = append(g.HStem[:0], base.HStem...)
		g.VStem = append(g.VStem[:0], base.VStem...)
		glyphs[seac.name] = g
	}

	res := &Font{
		Info:     fi,
		Private:  private,
		Glyphs:   glyphs,
		Encoding: encoding,
	}
	return res, nil
}
