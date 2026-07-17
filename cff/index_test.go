// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2021  Jochen Voss <voss@seehuhn.de>
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
	"reflect"
	"testing"

	"seehuhn.de/go/sfnt/parser"
)

func TestIndex(t *testing.T) {
	blob := make([]byte, 1+127)
	for i := range blob {
		blob[i] = byte(i + 1)
	}

	for _, count := range []int{0, 2, 3, 517} {
		data := make(cffIndex, count)
		for i := range count {
			d := i % 2
			data[i] = blob[d : d+127]
		}

		buf := data.encode()

		if count == 0 && len(buf) != 2 {
			t.Error("wrong length for empty INDEX")
		}

		p := parser.New(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))

		out, err := readIndex(p)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(out) != len(data) {
			t.Errorf("wrong length")
			continue
		}
		for i, blob := range out {
			if !bytes.Equal(blob, data[i]) {
				t.Errorf("wrong data")
				continue
			}
		}
	}
}

func TestIndex32(t *testing.T) {
	blob := make([]byte, 1+127)
	for i := range blob {
		blob[i] = byte(i + 1)
	}

	for _, count := range []int{0, 1, 2, 3, 517} {
		data := make(cffIndex, count)
		for i := range count {
			d := i % 2
			data[i] = blob[d : d+127]
		}

		buf := data.encode32()

		if count == 0 && !bytes.Equal(buf, []byte{0, 0, 0, 0}) {
			t.Errorf("empty INDEX32 = % x, want 4 zero bytes", buf)
		}

		p := parser.New(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
		out, err := readIndex32(p)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(out) != len(data) {
			t.Errorf("wrong length %d != %d", len(out), len(data))
			continue
		}
		for i, b := range out {
			if !bytes.Equal(b, data[i]) {
				t.Errorf("wrong data at %d", i)
			}
		}
	}
}

// TestIndex32OffSize exercises offSize 1..4 by growing the object size.
func TestIndex32OffSize(t *testing.T) {
	for _, size := range []int{1, 1000, 70000, 20000000} {
		data := cffIndex{make([]byte, size)}
		buf := data.encode32()
		p := parser.New(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
		out, err := readIndex32(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || len(out[0]) != size {
			t.Errorf("size %d: round trip failed", size)
		}
	}
}

func FuzzIndex32(f *testing.F) {
	iSeed := cffIndex{}
	f.Add(iSeed.encode32())
	for _, d := range [][]byte{{}, {0}, {0, 1, 2, 3}} {
		iSeed = append(iSeed, d)
		f.Add(iSeed.encode32())
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		i1, err := readIndex32(p)
		if err != nil {
			return
		}

		buf := i1.encode32()
		if len(buf) > len(data) {
			t.Error("inefficient encoding")
		}

		p = parser.New(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
		i2, err := readIndex32(p)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(i1, i2) {
			t.Error("unequal")
		}
	})
}

func FuzzIndex(f *testing.F) {
	iSeed := cffIndex{}
	buf := iSeed.encode()
	f.Add(buf)
	for _, d := range [][]byte{{}, {0}, {0, 1, 2, 3}} {
		iSeed = append(iSeed, d)
		buf := iSeed.encode()
		f.Add(buf)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		i1, err := readIndex(p)
		if err != nil {
			return
		}

		buf := i1.encode()
		if len(buf) > len(data) {
			t.Error("inefficient encoding")
		}

		p = parser.New(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
		i2, err := readIndex(p)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(i1, i2) {
			t.Error("unequal")
		}
	})
}
