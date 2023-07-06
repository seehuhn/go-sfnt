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

type Font struct {
	Info    *FontInfo
	Private *PrivateDict
	Glyphs  map[string]*Glyph
}

type Glyph struct{}

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

	cs, ok := fd["CharStrings"].(postscript.Dict)
	if !ok {
		return nil, errors.New("missing/invalid CharStrings dictionary")
	}
	names := maps.Keys(cs)
	slices.Sort(names)
	for _, name := range names[:10] {
		obfuscated, ok := cs[name].(postscript.String)
		if !ok || len(obfuscated) < 4 {
			fmt.Println("warning: skipping non-string glyph", name)
			continue
		}
		fmt.Println(name, len(obfuscated))
		plain := deobfuscateCharstring(obfuscated, int(lenIV))
		for _, b := range plain {
			fmt.Printf("%02x %d\n", b, b)
		}
		fmt.Println()
	}

	res := &Font{
		Info:    fi,
		Private: private,
	}
	return res, nil
}

func deobfuscateCharstring(charstring []byte, n int) []byte {
	var R uint16 = 4330
	var c1 uint16 = 52845
	var c2 uint16 = 22719
	plain := charstring[:0]
	for i, cipher := range charstring {
		if i >= n {
			plain = append(plain, cipher^byte(R>>8))
		}
		R = (uint16(cipher)+R)*c1 + c2
	}
	return plain
}
