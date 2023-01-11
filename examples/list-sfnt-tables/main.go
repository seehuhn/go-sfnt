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
