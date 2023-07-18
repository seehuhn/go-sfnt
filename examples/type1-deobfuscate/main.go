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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"seehuhn.de/go/sfnt/type1"
)

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: %s font.pf{a,b}\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}
	fname := flag.Arg(0)
	err := deobfuscate(fname)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func deobfuscate(fname string) error {
	fd, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer fd.Close()

	// read the first byte of fd to determine the file type
	var head [1]byte
	_, err = io.ReadFull(fd, head[:])
	if err != nil {
		return err
	}
	fd.Seek(0, 0)

	var raw io.Reader = fd
	if head[0] == 0x80 {
		raw = type1.DecodePFB(raw)
	}
	r := &reader{
		r:   raw,
		buf: make([]byte, 4096),
	}

	// Copy everything until the end of the first match of eexecStartRegexp
	// to stdout.
	for {
		err := r.refill()
		if err == io.EOF {
			if r.used > 0 {
				fmt.Println(string(r.buf[r.start:r.used]))
			}
			return nil
		} else if err != nil {
			return err
		}

		end := r.used
		eexecLoc := eexecStartRegexp.FindIndex(r.buf[:r.used])
		if eexecLoc != nil {
			end = eexecLoc[0]
		}

		// print buf[start:end], fixing up the line endings
		lloc := eolRegexp.FindAllIndex(r.buf[:end], -1)
		for _, loc := range lloc {
			if loc[0] == end-1 && eexecLoc == nil {
				// don't mistake \r\n which spans two buffers for a single \r
				break
			}
			fmt.Println(string(r.buf[r.start:loc[0]]))
			r.start = loc[1]
		}
		if eexecLoc != nil {
			if r.start < end {
				fmt.Println(string(r.buf[r.start:end]))
			}
			r.start = eexecLoc[1]
			break
		}
		if r.start == 0 && r.used == len(r.buf) {
			return errors.New("line too long")
		}
	}
	fmt.Println("% currentfile eexec")
	fmt.Println("%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%")

	// check whether the eexec section is binary or hex-encoded
	for r.used < r.start+eexecN {
		err = r.refill()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return err
		}
	}
	r.isBinary = false
	for _, b := range r.buf[r.start : r.start+eexecN] {
		if !('0' <= b && b <= '9' || 'a' <= b && b <= 'f' || 'A' <= b && b <= 'F') {
			r.isBinary = true
			break
		}
	}
	r.R = eexecR

	for i := 0; i < eexecN; i++ {
		_, err := r.nextDecoded()
		if err != nil {
			return err
		}
	}

	var line []byte
	ignoreLF := false
	for {
		b, err := r.nextDecoded()
		os.Stdout.Write([]byte{b})
		if err == io.EOF {
			if len(line) > 0 {
				// fmt.Println(string(line))
				line = line[:0]
			}
			return nil
		} else if err != nil {
			return err
		}

		if b == '\n' && ignoreLF {
			ignoreLF = false
			continue
		}
		ignoreLF = b == '\r'
		if b == '\r' || b == '\n' {
			// fmt.Println(string(line))
			line = line[:0]
			continue
		}
		line = append(line, b)
	}

	fmt.Println("%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%")
	return errors.New("not implemented")
}

type reader struct {
	r     io.Reader
	buf   []byte
	start int
	used  int

	R        uint16
	isBinary bool
}

func (r *reader) refill() error {
	n := copy(r.buf, r.buf[r.start:r.used])
	r.start = 0
	r.used = n
	n, err := r.r.Read(r.buf[r.used:])
	if err != nil {
		return err
	}
	r.used += n
	return nil
}

func (r *reader) nextDecoded() (byte, error) {
	cipher, err := r.nextObfuscated()
	if err != nil {
		return 0, err
	}
	plain := cipher ^ byte(r.R>>8)
	r.R = (uint16(cipher)+r.R)*eexecC1 + eexecC2
	return plain, nil
}

func (r *reader) nextObfuscated() (byte, error) {
	if r.isBinary {
		return r.nextRaw()
	}

	i := 0
	var out byte
readLoop:
	for i < 2 {
		b, err := r.nextRaw()
		var nibble byte
		switch {
		case err != nil:
			return 0, err
		case b <= 32:
			continue readLoop
		case b >= '0' && b <= '9':
			nibble = b - '0'
		case b >= 'A' && b <= 'F':
			nibble = b - 'A' + 10
		case b >= 'a' && b <= 'f':
			nibble = b - 'a' + 10
		default:
			return 0, fmt.Errorf("invalid hex digit %q", b)
		}
		out = out<<4 | nibble
		i++
	}
	return out, nil
}

func (r *reader) nextRaw() (byte, error) {
	for r.start >= r.used {
		err := r.refill()
		if err != nil {
			return 0, err
		}
	}
	b := r.buf[r.start]
	r.start++
	return b, nil
}

const (
	eexecN  = 4
	eexecR  = 55665
	eexecC1 = 52845
	eexecC2 = 22719
)

var (
	eolRegexp        = regexp.MustCompile(`(\r\n|\r|\n)`)
	eexecStartRegexp = regexp.MustCompile(`currentfile[ \t\r\n]+eexec[ \t\r\n]`)
)
