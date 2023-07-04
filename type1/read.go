package type1

import (
	"errors"
	"io"

	"seehuhn.de/go/postscript"
	"seehuhn.de/go/sfnt/funit"
)

func Read(r io.Reader) (*FontInfo, error) {
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

	res := &FontInfo{
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
	return res, nil
}
