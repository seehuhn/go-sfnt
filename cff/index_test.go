// Copyright 2021 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

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

		buf := mustEncode(data)

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

		buf := mustEncode32(data)

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
		buf := mustEncode32(data)
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
	f.Add(mustEncode32(iSeed))
	for _, d := range [][]byte{{}, {0}, {0, 1, 2, 3}} {
		iSeed = append(iSeed, d)
		f.Add(mustEncode32(iSeed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		i1, err := readIndex32(p)
		if err != nil {
			return
		}

		buf := mustEncode32(i1)
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
	buf := mustEncode(iSeed)
	f.Add(buf)
	for _, d := range [][]byte{{}, {0}, {0, 1, 2, 3}} {
		iSeed = append(iSeed, d)
		buf := mustEncode(iSeed)
		f.Add(buf)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		i1, err := readIndex(p)
		if err != nil {
			return
		}

		buf := mustEncode(i1)
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

// mustEncode returns the encoded INDEX.  The test data used with it is
// known to be encodable.
func mustEncode(data cffIndex) []byte {
	buf, err := data.encode()
	if err != nil {
		panic(err)
	}
	return buf
}

// mustEncode32 returns the encoded CFF2 INDEX.  The test data used with it
// is known to be encodable.
func mustEncode32(data cffIndex) []byte {
	buf, err := data.encode32()
	if err != nil {
		panic(err)
	}
	return buf
}

func TestIndexTooManyItems(t *testing.T) {
	data := make(cffIndex, 1<<16)
	if _, err := data.encode(); err == nil {
		t.Error("INDEX with 65536 items encoded")
	}
}
