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

// List-sfnt-tables prints a list of the tables in an sfnt font file.
package main

import (
	"fmt"
	"log"
	"os"
	"sort"

	"golang.org/x/exp/maps"
	"seehuhn.de/go/sfnt/header"
)

func main() {
	fileNames := os.Args[1:]
	if len(fileNames) == 0 {
		fmt.Fprintf(os.Stderr, "usage: %s font.ttf font.otf ...\n", os.Args[0])
	}

	for _, fileName := range fileNames {
		font, err := os.Open(fileName)
		if err != nil {
			log.Fatal(err)
		}

		info, err := header.Read(font)
		if err != nil {
			log.Fatal(fileName+":", err)
		}

		var fontType string
		switch info.ScalerType {
		case header.ScalerTypeTrueType:
			fontType = "TrueType"
		case header.ScalerTypeCFF:
			fontType = "CFF"
		case header.ScalerTypeApple:
			fontType = "TrueType (Apple)"
		}

		names := maps.Keys(info.Toc)
		sort.Slice(names, func(i, j int) bool {
			return info.Toc[names[i]].Offset < info.Toc[names[j]].Offset
		})

		fmt.Println(fileName+":", fontType, "font")
		fmt.Println()
		fmt.Println("  name | offset | length")
		fmt.Println("  -----+--------+-------")
		for _, name := range names {
			fmt.Printf("  %4s | %6d | %6d\n", name, info.Toc[name].Offset, info.Toc[name].Length)
		}
		fmt.Println()

		err = font.Close()
		if err != nil {
			log.Fatal(fileName+":", err)
		}
	}
}
