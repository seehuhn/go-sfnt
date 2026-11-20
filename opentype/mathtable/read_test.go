// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

func TestReadVersion(t *testing.T) {
	data, err := (&Info{}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	data[1] = 2
	_, err = readBytes(data)
	if !parser.IsUnsupported(err) {
		t.Errorf("unexpected error %v", err)
	}
}

func TestReadNoConstants(t *testing.T) {
	data, err := (&Info{}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	data[4], data[5] = 0, 0
	if _, err := readBytes(data); err == nil {
		t.Error("read succeeded")
	}
	if _, err := readBytes(data[:100]); err == nil {
		t.Error("read of truncated table succeeded")
	}
}

func TestReadDamaged(t *testing.T) {
	info := sampleInfo()
	data, err := info.Encode()
	if err != nil {
		t.Fatal(err)
	}

	// every truncation either fails or can be repaired stably
	for n := range len(data) {
		got, err := readBytes(data[:n])
		if err != nil {
			continue
		}
		enc, err := got.Encode()
		if err != nil {
			t.Fatalf("length %d: encode failed: %v", n, err)
		}
		got2, err := readBytes(enc)
		if err != nil {
			t.Fatalf("length %d: re-read failed: %v", n, err)
		}
		if diff := cmp.Diff(got, got2); diff != "" {
			t.Fatalf("length %d: round trip failed (-first +second):\n%s", n, diff)
		}
	}
}

func TestReadRepairs(t *testing.T) {
	be := func(vals ...uint16) []byte {
		var res []byte
		for _, v := range vals {
			res = append(res, byte(v>>8), byte(v))
		}
		return res
	}
	// header, MathConstants, then the tables to be tested
	const tail = headerSize + constantsSize
	build := func(glyphInfo, variants []byte) []byte {
		data := be(1, 0, headerSize, 0, 0)
		data = append(data, make([]byte, tail-len(data))...)
		if glyphInfo != nil {
			data[6], data[7] = byte(len(data)>>8), byte(len(data))
			data = append(data, glyphInfo...)
		}
		if variants != nil {
			data[8], data[9] = byte(len(data)>>8), byte(len(data))
			data = append(data, variants...)
		}
		return data
	}

	// the read succeeds and a read-write-read cycle is stable
	checkStable := func(t *testing.T, data []byte) {
		t.Helper()
		first, err := readBytes(data)
		if err != nil {
			t.Fatal(err)
		}
		second, _ := roundTrip(t, first)
		if diff := cmp.Diff(first, second); diff != "" {
			t.Errorf("round trip failed (-first +second):\n%s", diff)
		}
	}

	t.Run("italics count larger than coverage", func(t *testing.T) {
		gi := be(8, 0, 0, 0)
		gi = append(gi, be(16, 3)...) // three records, two covered glyphs
		gi = append(gi, be(1, 0, 2, 0, 3, 0)...)
		gi = append(gi, be(1, 2, 4, 5)...)
		checkStable(t, build(gi, nil))
	})

	t.Run("unsorted kern heights", func(t *testing.T) {
		ki := be(12, 1, 18, 0, 0, 0)
		ki = append(ki, be(1, 1, 5)...)
		ki = append(ki, be(2, 20, 0, 10, 0, 1, 0, 2, 0, 3, 0)...)
		gi := be(0, 0, 0, 8)
		gi = append(gi, ki...)
		checkStable(t, build(gi, nil))
	})

	t.Run("empty construction", func(t *testing.T) {
		v := be(0, 12, 0, 1, 0, 18)
		v = append(v, be(1, 1, 3)...)
		v = append(v, be(0, 0)...)
		checkStable(t, build(nil, v))
	})

	t.Run("assembly without parts", func(t *testing.T) {
		v := be(0, 12, 0, 1, 0, 18)
		v = append(v, be(1, 1, 3)...)
		v = append(v, be(6, 1, 9, 100)...) // one variant, assembly at +6
		v = append(v, be(0, 0, 0)...)      // no parts
		checkStable(t, build(nil, v))
	})

	t.Run("part flags", func(t *testing.T) {
		v := be(0, 12, 0, 1, 0, 18)
		v = append(v, be(1, 1, 3)...)
		v = append(v, be(4, 0)...)                        // assembly at +4
		v = append(v, be(0, 0, 1, 7, 1, 2, 3, 0xFFFE)...) // reserved flags only
		checkStable(t, build(nil, v))
	})
}
