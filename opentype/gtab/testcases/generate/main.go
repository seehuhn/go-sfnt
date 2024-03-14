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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

	cp, err := newCopier()
	if err != nil {
		return err
	}

	for i := range testcases.Gsub {
		info, err := fontGen.GsubTestFont(i)
		if err != nil {
			return err
		}

		c := testcases.Gsub[i]

		err = cp.run()
		if err != nil {
			return err
		}

		err = runOne(info, c, cp)
		if err != nil {
			return err
		}
	}

	err = cp.run()
	if err != io.EOF {
		if err != nil {
			return err
		}
		panic("copier out of sync")
	}

	err = os.WriteFile("gsub.go", cp.out.Bytes(), 0644)
	if err != nil {
		return err
	}

	return nil
}

func runOne(info *sfnt.Font, test *testcases.GsubTestCase, cp *copier) error {
	if test.Text == test.In {
		return errors.New("test.Text == test.In")
	}

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

	cmd = exec.Command("./generate/macos/test-gsub", fontPath, test.In)
	outBytes, err = cmd.Output()
	if err != nil {
		return err
	}
	macOut := strings.TrimSpace(string(outBytes))

	warn := ""
	if test.Out != hbOut && test.Out != macOut {
		warn = "(!!!)"
	} else {
		if hbOut != test.Out {
			hbOut += " (!!!)"
		}
		if macOut != test.Out {
			macOut += " (!!!)"
		}
	}

	m := lineRe.FindStringSubmatch(cp.line)
	if m != nil {
		if m[1] == ", Windows:" {
			m[1] = ""
		}
		cp.line = fmt.Sprintf("\t{ // harfbuzz: %s, Mac: %s%s",
			hbOut, macOut, m[1])
	} else {
		cp.line = fmt.Sprintf("\t{ // harfbuzz: %s, Mac: %s",
			hbOut, macOut)
	}
	cp.warn = warn

	return nil
}

var repl = strings.NewReplacer("[", "", "|", "", "]", "", "\n", "")

var (
	lineRe    = regexp.MustCompile(`//.*([,;] [wW]indows.*)$`)
	commentRe = regexp.MustCompile(`\s*(//.*)?$`)
)

type copier struct {
	in  io.ReadCloser
	s   *bufio.Scanner
	out *bytes.Buffer

	inBody bool
	line   string
	warn   string

	sec int
	idx int
}

func newCopier() (*copier, error) {
	file, err := os.Open("gsub.go")
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)

	return &copier{
		in:  file,
		s:   scanner,
		out: &bytes.Buffer{},
	}, nil
}

func (c *copier) run() error {
	if c.inBody {
		c.out.WriteString(c.line)
		c.out.WriteByte('\n')
		fmt.Fprintf(c.out, "\t\tName: \"%d_%02d\",\n", c.sec, c.idx)
	}
	for c.s.Scan() {
		c.line = c.s.Text()
		trimmed := strings.TrimSpace(c.line)
		if c.inBody {
			if strings.HasPrefix(trimmed, "{") {
				c.idx++
				c.warn = ""
				return nil
			} else if strings.HasPrefix(trimmed, "Name: ") {
				continue
			} else if strings.HasPrefix(trimmed, "Out: ") {
				warn := c.warn
				if warn != "" {
					warn = " // " + warn
				}
				c.line = commentRe.ReplaceAllString(c.line, warn)
			} else if strings.HasPrefix(trimmed, "// SECTION") {
				c.sec++
				c.idx = 0
			}
		}
		c.out.WriteString(c.line)
		c.out.WriteByte('\n')

		if strings.Contains(c.line, "// START OF TEST CASES") {
			c.inBody = true
		} else if strings.Contains(c.line, "// END OF TEST CASES") {
			c.inBody = false
		}
	}
	if err := c.s.Err(); err != nil {
		return err
	}
	return io.EOF
}
