// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2024  Jochen Voss <voss@seehuhn.de>
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

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/opentype/gtab/testcases"
)

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	fontGen, err := testcases.NewFontGen()
	if err != nil {
		return err
	}

	for i := range testcases.Gsub {
		info, err := fontGen.GsubTestFont(i)
		if err != nil {
			return err
		}

		c := testcases.Gsub[i]

		err = runOne(info, c)
		if err != nil {
			return err
		}
	}
	return nil
}

func runOne(info *sfnt.Font, test *testcases.GsubTestCase) error {
	dir, err := os.MkdirTemp("", "example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir) // clean up

	const fontName = "test.otf"

	fontPath := filepath.Join(dir, fontName)
	f, err := os.Create(fontPath)
	if err != nil {
		return err
	}
	_, err = info.Write(f)
	if err != nil {
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}

	cmd := exec.Command("hb-shape", "--no-clusters", "--no-positions", fontName, test.In)
	cmd.Dir = dir
	outBytes, err := cmd.Output()
	if err != nil {
		return err
	}
	hbOut := repl.Replace(string(outBytes))

	cmd = exec.Command("./macos/test-gsub", fontPath, test.In)
	outBytes, err = cmd.Output()
	if err != nil {
		return err
	}
	macOut := strings.TrimSpace(string(outBytes))

	warn := ""
	if test.Out != hbOut && test.Out != macOut {
		warn = " // (!!!)"
	} else {
		if hbOut != test.Out {
			hbOut += " (!!!)"
		}
		if macOut != test.Out {
			macOut += " (!!!)"
		}
	}

	fmt.Printf("\t{ // harfbuzz: %s, Mac: %s\n", hbOut, macOut)
	fmt.Printf("\t\tDesc: `%s`,\n", test.Desc)
	fmt.Printf("\t\tIn:   %q,\n", test.In)
	fmt.Printf("\t\tOut:  %q,%s\n", test.Out, warn)
	fmt.Println("\t},")
	return nil
}

var repl = strings.NewReplacer("[", "", "|", "", "]", "", "\n", "")
