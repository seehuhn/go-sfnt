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

package parser

import (
	"bytes"
	"testing"

	"seehuhn.de/go/membudget"
)

func TestPos(t *testing.T) {
	data := []byte{'0', '1', '2', '3', '4', '5', '6', '7'}
	buf := bytes.NewReader(data)
	p := New(buf, NewBudget(int64(len(data))))

	pos := p.Pos()
	if pos != 0 {
		t.Errorf("wrong position, expected 0 but got %d", pos)
	}

	_, err := p.ReadUint16()
	if err != nil {
		t.Fatal(err)
	}

	pos = p.Pos()
	if pos != 2 {
		t.Errorf("wrong position, expected 2 but got %d", pos)
	}

	err = p.SeekPos(5)
	if err != nil {
		t.Fatal(err)
	}
	if p.Pos() != 5 {
		t.Errorf("wrong position, expected 5 but got %d", p.Pos())
	}
}

func TestReadInt32(t *testing.T) {
	data := []byte{0xFF, 0xFF, 0xFF, 0xFB} // -5
	buf := bytes.NewReader(data)
	p := New(buf, NewBudget(int64(len(data))))

	val, err := p.ReadInt32()
	if err != nil {
		t.Fatal(err)
	}
	if val != -5 {
		t.Errorf("wrong value, expected -5 but got %d", val)
	}
}

func TestReadInt16Slice(t *testing.T) {
	data := []byte{0xFF, 0xFF, 0x00, 0x02, 0x80, 0x00}
	buf := bytes.NewReader(data)
	p := New(buf, NewBudget(int64(len(data))))

	got, err := p.ReadInt16Slice(3)
	if err != nil {
		t.Fatal(err)
	}
	want := []int16{-1, 2, -32768}
	if len(got) != len(want) {
		t.Fatalf("wrong length, expected %d but got %d", len(want), len(got))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("wrong value at %d, expected %d but got %d", i, v, got[i])
		}
	}
}

func TestReadInt16SliceBudget(t *testing.T) {
	data := make([]byte, 6)
	buf := bytes.NewReader(data)
	p := New(buf, membudget.New(1))

	_, err := p.ReadInt16Slice(3)
	if err == nil {
		t.Error("expected budget error, got nil")
	}
}
