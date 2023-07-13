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
	"os"
	"testing"
	"time"
)

func TestXXX(t *testing.T) {
	fname := "/Users/voss/project/pdf/type1/NimbusRoman-Regular.pfa"
	fd, err := os.Open(fname)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()

	font, err := Read(fd)
	if err != nil {
		t.Fatal(err)
	}
	font.CreationDate = time.Now()

	err = font.Write(os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
}
