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

// Font-info shows information about a font file.
package main

import (
	"fmt"
	"log"
	"os"

	"seehuhn.de/go/sfnt"
)

func main() {
	fileNames := os.Args[1:]
	if len(fileNames) == 0 {
		fmt.Fprintf(os.Stderr, "usage: %s font.ttf font.otf ...\n", os.Args[0])
	}

	for _, fileName := range fileNames {
		fmt.Println(fileName)
		info, err := sfnt.ReadFile(fileName)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("  FamilyName:", info.FamilyName)
		fmt.Println("  Width:", info.Width)
		fmt.Println("  Weight:", info.Weight)
		fmt.Println("  IsItalic:", info.IsItalic)
		fmt.Println("  IsBold:", info.IsBold)
		fmt.Println("  IsRegular:", info.IsRegular)
		fmt.Println("  IsOblique:", info.IsOblique)
		fmt.Println("  IsSerif:", info.IsSerif)
		fmt.Println("  IsScript:", info.IsScript)
		fmt.Println("  Version:", info.Version)
		fmt.Println("  CreationTime:", info.CreationTime)
		fmt.Println("  ModificationTime:", info.ModificationTime)
		if info.Description != "" {
			fmt.Println("  Description:", info.Description)
		}
		if info.SampleText != "" {
			fmt.Println("  SampleText:", info.SampleText)
		}
		if info.Copyright != "" {
			fmt.Println("  Copyright:", info.Copyright)
		}
		if info.Trademark != "" {
			fmt.Println("  Trademark:", info.Trademark)
		}
		if info.License != "" {
			fmt.Println("  License:", info.License)
		}
		if info.LicenseURL != "" {
			fmt.Println("  LicenseURL:", info.LicenseURL)
		}
		fmt.Println("  PermUse:", info.PermUse)
		fmt.Println("  UnitsPerEm:", info.UnitsPerEm)
		fmt.Println("  Ascent:", info.Ascent)
		fmt.Println("  Descent:", info.Descent)
		fmt.Println("  LineGap:", info.LineGap)
		fmt.Println("  CapHeight:", info.CapHeight)
		fmt.Println("  XHeight:", info.XHeight)
		fmt.Println("  ItalicAngle:", info.ItalicAngle)
		fmt.Println("  UnderlinePosition:", info.UnderlinePosition)
		fmt.Println("  UnderlineThickness:", info.UnderlineThickness)

		//
		//		CMap     cmap.Subtable
		//		Outlines interface{} // either *cff.Outlines or *glyf.Outlines

		// Gdef * gdef.Table
		// Gsub * gtab.Info
		// Gpos * gtab.Info

		fmt.Println()
	}
}
