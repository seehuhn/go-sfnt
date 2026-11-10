// Copyright 2023 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Rewrite-font rewrites the font files given on the command line.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"seehuhn.de/go/sfnt"
)

func main() {
	for i, fname := range os.Args[1:] {
		info, err := sfnt.ReadFile(fname)
		if err != nil {
			log.Fatal(err)
		}

		ext := filepath.Ext(fname)
		outName := fmt.Sprintf("out%05d%s", i, ext)

		fd, err := os.Create(outName)
		if err != nil {
			log.Fatal(err)
		}
		_, err = info.Write(fd)
		if err != nil {
			log.Fatal(err)
		}
		err = fd.Close()
		if err != nil {
			log.Fatal(err)
		}
	}
}
