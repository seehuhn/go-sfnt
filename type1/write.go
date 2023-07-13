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
	"io"
	"text/template"
	"time"

	"seehuhn.de/go/postscript"
)

func (f *Font) Write(w io.Writer) error {
	info := fontInfo{
		FontName:           f.Info.FontName,
		Version:            f.Info.Version,
		Copyright:          f.Info.Copyright,
		FullName:           f.Info.FullName,
		FamilyName:         f.Info.FamilyName,
		Weight:             f.Info.Weight,
		ItalicAngle:        f.Info.ItalicAngle,
		IsFixedPitch:       f.Info.IsFixedPitch,
		UnderlinePosition:  float64(f.Info.UnderlinePosition),
		UnderlineThickness: float64(f.Info.UnderlineThickness),
		CreationDate:       f.CreationDate,
	}

	return tmpl.Execute(w, info)
}

type fontInfo struct {
	FontName           string
	Version            string
	Copyright          string
	FullName           string
	FamilyName         string
	Weight             string
	ItalicAngle        float64
	IsFixedPitch       bool
	UnderlinePosition  float64
	UnderlineThickness float64
	CreationDate       time.Time
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
}).Parse(
	`%!FontType1-1.1: {{.FontName}} {{.Version}}
{{if not .CreationDate.IsZero}}%%CreationDate: {{.CreationDate.Format "Mon Jan 2 2006"}}
{{end -}}
% {{.Copyright}}
11 dict begin
%
/FontInfo 8 dict dup begin
/version {{.Version|PS}} readonly def
/FullName {{.FullName|PS}} readonly def
/FamilyName {{.FamilyName|PS}} readonly def
/Weight {{.Weight|PS}} readonly def
/ItalicAngle {{.ItalicAngle}} def
/isFixedPitch {{.IsFixedPitch}} def
/UnderlinePosition {{.UnderlinePosition}} def
/UnderlineThickness {{.UnderlineThickness}} def
end readonly def
%
/FontName {{.FontName|PN}} def
/PaintType 0 def
/FontType 1 def
/FontMatrix [0.001 0 0 0.001 0 0] readonly def
/Encoding 256 array
0 1 255 {1 index exch /.notdef put } for
dup 32 /space put
% ...
% . . . repetitive assignments to Encoding array omitted % ...
dup 254 /bracerightbt put
readonly def
/FontBBox {-180 -293 1090 1010} readonly def
currentdict end
%
dup /Private 8 dict dup begin
/RD {string currentfile exch readstring pop} executeonly def
/ND {def} executeonly def
/NP {put} executeonly def
/MinFeature {16 16} def
/password 5839 def
/BlueValues [-17 0 487 500 673 685] def
/Subrs 43 array
dup 0 15 RD ~15~binary~bytes~ NP
% ...
% . . . 41 subroutine definitions omitted
% ...
dup 42 23 RD ~23~binary~bytes~ NP
ND
2 index /CharStrings 190 dict dup begin
/Alpha 186 RD ~186~binary~bytes~ ND
% ...
% . . . 188 character definitions omitted
% ...
/.notdef 9 RD ~9~binary~bytes~ ND
end
end
readonly put
put
dup /FontName get exch definefont pop
`))
