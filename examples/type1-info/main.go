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

package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"seehuhn.de/go/postscript"
	"seehuhn.de/go/sfnt/type1"
)

func main() {
	for _, fname := range os.Args[1:] {
		err := showInfo(fname)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func showInfo(fname string) error {
	fd, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer fd.Close()

	// read the first byte of fd to determine the file type
	var buf [1]byte
	_, err = io.ReadFull(fd, buf[:])
	if err != nil {
		return err
	}
	fd.Seek(0, 0)

	var r io.Reader = fd
	if buf[0] == 0x80 {
		r = type1.DecodePFB(r)
	}

	intp := postscript.NewInterpreter()
	err = intp.Execute(r)
	if err != nil {
		return err
	}

	for key, font := range intp.Fonts {
		fmt.Printf("# %s\n\n", key)
		for key, val := range font {
			if key == "Private" || key == "FontInfo" {
				fmt.Println(string(key) + ":")
				for k2, v2 := range val.(postscript.Dict) {
					valString := fmt.Sprint(v2)
					if len(valString) > 70 {
						valString = fmt.Sprintf("<%T>", val)
					}
					fmt.Println("  "+string(k2)+":", valString)
				}
				continue
			}
			valString := fmt.Sprint(val)
			if len(valString) > 70 {
				valString = fmt.Sprintf("<%T>", val)
			}
			fmt.Println(string(key)+":", valString)
		}
	}
	return nil
}
