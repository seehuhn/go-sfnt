// Copyright 2023 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Extract-cff extracts the CFF data from OpenType font files.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"seehuhn.de/go/sfnt/header"
)

func main() {
	args := os.Args[1:]
	for _, fname := range args {
		outName := filepath.Base(strings.TrimSuffix(fname, ".otf") + ".cff")
		fmt.Println(fname, "->", outName)

		r, err := os.Open(fname)
		if err != nil {
			log.Fatalf("%s: %v", fname, err)
		}

		header, err := header.Read(r)
		if err != nil {
			log.Fatalf("%s: %v", fname, err)
		}

		cffData, err := header.ReadTableBytes(r, "CFF ")
		if err != nil {
			log.Fatalf("%s: %v", fname, err)
		}

		err = r.Close()
		if err != nil {
			log.Fatalf("%s: %v", fname, err)
		}

		err = os.WriteFile(outName, cffData, 0644)
		if err != nil {
			log.Fatalf("%s: %v", outName, err)
		}
	}
}
