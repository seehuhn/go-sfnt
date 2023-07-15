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
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"seehuhn.de/go/postscript"
	"seehuhn.de/go/sfnt/funit"
)

func (f *Font) Write(w io.Writer) error {
	fontMatrix := f.Info.FontMatrix
	if len(fontMatrix) != 6 {
		fontMatrix = []float64{0.001, 0, 0, 0.001, 0, 0}
	}

	info := fontInfo{
		BlueFuzz:           f.Private.BlueFuzz,
		BlueScale:          f.Private.BlueScale,
		BlueShift:          f.Private.BlueShift,
		BlueValues:         f.Private.BlueValues,
		CharStrings:        f.encodeCharstrings(),
		Copyright:          f.Info.Copyright,
		CreationDate:       f.CreationDate,
		Encoding:           f.Encoding,
		FamilyName:         f.Info.FamilyName,
		FontMatrix:         fontMatrix,
		FontName:           f.Info.FontName,
		ForceBold:          f.Private.ForceBold,
		FullName:           f.Info.FullName,
		IsFixedPitch:       f.Info.IsFixedPitch,
		ItalicAngle:        f.Info.ItalicAngle,
		Notice:             f.Info.Notice,
		OtherBlues:         f.Private.OtherBlues,
		UnderlinePosition:  float64(f.Info.UnderlinePosition),
		UnderlineThickness: float64(f.Info.UnderlineThickness),
		Version:            f.Info.Version,
		Weight:             f.Info.Weight,
		// EExec: 1,
	}
	if f.Private.StdHW != 0 {
		info.StdHW = []float64{f.Private.StdHW}
	}
	if f.Private.StdVW != 0 {
		info.StdVW = []float64{f.Private.StdVW}
	}

	return tmpl.Execute(w, info)
}

func (f *Font) encodeCharstrings() map[string]string {
	charStrings := make(map[string]string)
	for name, g := range f.Glyphs {
		cs := g.encodeCharString()

		var obf []byte
		iv := []byte{0, 0, 0, 0}
		for {
			obf = obfuscateCharstring(cs, iv)
			if obf[0] > 32 {
				couldBeHex := true
				for _, b := range obf[:4] {
					if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F') {
						couldBeHex = false
						break
					}
				}
				if !couldBeHex {
					break
				}
			}

			pos := 0
			for pos < 4 {
				iv[pos]++
				if iv[pos] != 0 {
					break
				}
				pos++
			}
		}
		charStrings[name] = string(obf)
	}
	return charStrings
}

func writeEncoding(encoding []string) string {
	if len(encoding) != 256 {
		return ""
	}
	if isStandardEncoding(encoding) {
		return "/Encoding StandardEncoding def\n"
	}

	b := &strings.Builder{}
	b.WriteString("/Encoding 256 array\n")
	b.WriteString("0 1 255 {1 index exch /.notdef put} for\n")
	for i, name := range encoding {
		if name == ".notdef" {
			continue
		}
		fmt.Fprintf(b, "dup %d %s put\n", i, postscript.Name(name).PS())
	}
	b.WriteString("readonly def\n")
	return b.String()
}

func isStandardEncoding(encoding []string) bool {
	if len(encoding) != 256 {
		return false
	}
	for i, s := range encoding {
		ss := postscript.Name(s)
		if ss != postscript.StandardEncoding[i] && ss != ".notdef" {
			return false
		}
	}
	return true
}

var tmpl = template.Must(template.New("type1").Funcs(template.FuncMap{
	"PS": func(s string) string {
		x := postscript.String(s)
		return x.PS()
	},
	"PN": func(s string) string {
		x := postscript.Name(s)
		return x.PS()
	},
	"E": writeEncoding,
}).Parse(`{{define "SectionA" -}}
%!FontType1-1.1: {{.FontName}} {{.Version}}
{{if not .CreationDate.IsZero}}%%CreationDate: {{.CreationDate.Format "2006-01-02 15:04:05 -0700 MST"}}
{{end -}}
{{if .Copyright}}% {{.Copyright}}
{{end -}}
10 dict begin
/FontInfo 11 dict dup begin
/version {{.Version|PS}} def
{{if .Notice}}/Notice {{.Notice|PS}} def
{{end -}}
{{if .Copyright}}/Copyright {{.Copyright|PS}} def
{{end -}}
/FullName {{.FullName|PS}} def
/FamilyName {{.FamilyName|PS}} def
/Weight {{.Weight|PS}} def
/ItalicAngle {{.ItalicAngle}} def
/isFixedPitch {{.IsFixedPitch}} def
/UnderlinePosition {{.UnderlinePosition}} def
/UnderlineThickness {{.UnderlineThickness}} def
end def
/FontName {{.FontName|PN}} def
{{ .Encoding|E -}}
/PaintType 0 def
/FontType 1 def
/FontMatrix {{ .FontMatrix }} def
/FontBBox [0 0 0 0] def
currentdict end
{{if gt .EExec 0}}currentfile eexec
{{end -}}
{{end -}}

{{define "SectionB" -}}
dup /Private 15 dict dup begin
/RD {string currentfile exch readstring pop} executeonly def
/ND {def} executeonly def
/NP {put} executeonly def
/Subrs {{ len .Subrs }} array
{{ range $index, $subr := .Subrs -}}
dup {{ $index }} {{ len $subr }} RD {{ $subr }} NP
{{ end -}}
{{ if .BlueValues}}/BlueValues {{ .BlueValues }} def
{{end -}}
{{ if .OtherBlues}}/OtherBlues {{ .OtherBlues }} def
{{end -}}
{{ if and (ne .BlueScale 0.0) (or (lt .BlueScale .039624) (gt .BlueScale .039626)) -}}
/BlueScale {{.BlueScale}} def
{{end -}}
{{ if ne .BlueShift 7 }}/BlueShift {{.BlueShift}} def
{{end -}}
{{ if ne .BlueFuzz 1 }}/BlueFuzz {{.BlueFuzz}} def
{{end -}}
{{ if .StdHW }}/StdHW {{ .StdHW }} def
{{end -}}
{{ if .StdVW }}/StdVW {{ .StdVW }} def
{{end -}}
/ForceBold {{ .ForceBold }} def
/password 5839 def
/MinFeature {16 16} def
ND
2 index /CharStrings {{ len .CharStrings }} dict dup begin
{{ range $name, $cs := .CharStrings -}}
{{ $name|PN }} {{ len $cs }} RD {{ $cs }} ND
{{ end -}}
end
end
readonly put
put
dup /FontName get exch definefont pop
{{if gt .EExec 0}}mark currentfile closefile
{{end -}}
{{end -}}

{{define "SectionC" -}}
{{if gt .EExec 0 -}}
0000000000000000000000000000000000000000000000000000000000000000
0000000000000000000000000000000000000000000000000000000000000000
0000000000000000000000000000000000000000000000000000000000000000
0000000000000000000000000000000000000000000000000000000000000000
0000000000000000000000000000000000000000000000000000000000000000
0000000000000000000000000000000000000000000000000000000000000000
0000000000000000000000000000000000000000000000000000000000000000
0000000000000000000000000000000000000000000000000000000000000000
cleartomark
{{end -}}
{{end -}}

{{template "SectionA" . -}}
{{template "SectionB" . -}}
{{template "SectionC" . -}}
`))

type fontInfo struct {
	BlueFuzz           int32
	BlueScale          float64
	BlueShift          int32
	BlueValues         []funit.Int16
	CharStrings        map[string]string
	Copyright          string
	CreationDate       time.Time
	Encoding           []string
	FamilyName         string
	FontMatrix         []float64
	FontName           string
	ForceBold          bool
	FullName           string
	IsFixedPitch       bool
	ItalicAngle        float64
	Notice             string
	OtherBlues         []funit.Int16
	StdHW              []float64
	StdVW              []float64
	Subrs              []string
	UnderlinePosition  float64
	UnderlineThickness float64
	Version            string
	Weight             string

	EExec int // 0 = no eexec, 1 = hex eexec, 2 = binary eexec, 3 = binary eexec, no zeros
}
