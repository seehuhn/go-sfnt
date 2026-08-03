// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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

package cff

import (
	"bytes"
	"testing"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

func TestFDSelect4RoundTrip(t *testing.T) {
	const nGlyphs = 60000
	const nPrivate = 1100
	fds := []FDSelectFn{
		func(gid glyph.ID) int { return 0 },
		func(gid glyph.ID) int { return int(gid) / 60 },
		func(gid glyph.ID) int { return int(gid/5) % 5 },
	}
	for _, fd := range fds {
		buf := fd.encodeFormat4(nGlyphs)
		if buf[0] != 4 {
			t.Fatalf("wrong format %d", buf[0])
		}
		p := parser.New(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
		out, err := readFDSelect(p, nGlyphs, nPrivate, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := range glyph.ID(nGlyphs) {
			if fd(i) != out(i) {
				t.Fatalf("%d: %d != %d", i, fd(i), out(i))
			}
		}
	}
}

// TestFDSelect4Wide checks that the format-4 reader handles the uint32
// range fields, using a hand-built table with large boundaries.
func TestFDSelect4Wide(t *testing.T) {
	const numGlyphs = 100000
	// two ranges: [0,70000) -> fd 1, [70000, numGlyphs) -> fd 2
	blob := []byte{
		4,
		0, 0, 0, 2, // nRanges
		0, 0, 0, 0, 0, 1, // first=0, fd=1
		0, 1, 0x11, 0x70, 0, 2, // first=70000, fd=2
		0, 1, 0x86, 0xa0, // sentinel=100000
	}
	p := parser.New(bytes.NewReader(blob), parser.NewBudget(int64(len(blob))))
	out, err := readFDSelect(p, numGlyphs, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if out(0) != 1 || out(65535) != 1 {
		t.Errorf("low range wrong: %d %d", out(0), out(65535))
	}
}

// TestFDSelect4Gated verifies format 4 is rejected outside CFF2 context.
func TestFDSelect4Gated(t *testing.T) {
	blob := []byte{4, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10}
	p := parser.New(bytes.NewReader(blob), parser.NewBudget(int64(len(blob))))
	if _, err := readFDSelect(p, 10, 3, false); err == nil {
		t.Error("format 4 should be rejected when not in CFF2 context")
	}
}

func FuzzFDSelect4(f *testing.F) {
	const nGlyphs = 100
	fds := []FDSelectFn{
		func(gid glyph.ID) int { return 0 },
		func(gid glyph.ID) int { return int(gid) / 4 },
		func(gid glyph.ID) int { return int(gid/5) % 5 },
	}
	for _, fd := range fds {
		f.Add(fd.encodeFormat4(nGlyphs))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		p := parser.New(bytes.NewReader(in), parser.NewBudget(int64(len(in))))
		fdSelect, err := readFDSelect(p, nGlyphs, 10, true)
		if err != nil {
			return
		}

		in2 := fdSelect.encodeFormat4(nGlyphs)

		p = parser.New(bytes.NewReader(in2), parser.NewBudget(int64(len(in2))))
		fdSelect2, err := readFDSelect(p, nGlyphs, 25, true)
		if err != nil {
			t.Fatal(err)
		}

		for i := range glyph.ID(nGlyphs) {
			if fdSelect(i) != fdSelect2(i) {
				t.Errorf("%d: %d != %d", i, fdSelect(i), fdSelect2(i))
			}
		}
	})
}

func FuzzFDSelect(f *testing.F) {
	const nGlyphs = 100
	fds := []FDSelectFn{
		func(gid glyph.ID) int { return 0 },
		func(gid glyph.ID) int { return int(gid) / 60 },
		func(gid glyph.ID) int { return int(gid) / 4 },
		func(gid glyph.ID) int { return int(gid) },
		func(gid glyph.ID) int { return int(gid/5) % 5 },
	}
	for _, fd := range fds {
		seed, err := fd.encode(nGlyphs, nGlyphs)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		p := parser.New(bytes.NewReader(in), parser.NewBudget(int64(len(in))))
		fdSelect, err := readFDSelect(p, nGlyphs, 10, false)
		if err != nil {
			return
		}

		// readFDSelect rejects indices >= 10, so encoding cannot fail
		in2, err := fdSelect.encode(nGlyphs, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(in2) > len(in) {
			t.Error("inefficient encoding")
		}

		p = parser.New(bytes.NewReader(in2), parser.NewBudget(int64(len(in2))))
		fdSelect2, err := readFDSelect(p, nGlyphs, 25, false)
		if err != nil {
			t.Fatal(err)
		}

		for i := range glyph.ID(nGlyphs) {
			if fdSelect(i) != fdSelect2(i) {
				t.Errorf("%d: %d != %d", i, fdSelect(i), fdSelect2(i))
			}
		}
	})
}
