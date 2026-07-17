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
	"math"
	"reflect"
	"testing"
)

// constK returns a regionCount function that reports k regions for every
// variation-store index.
func constK(k int) func(int) (int, error) {
	return func(int) (int, error) { return k, nil }
}

func TestDictCFF2RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		dict cffDict
		k    int
	}{
		{
			name: "plain",
			dict: cffDict{
				opFontMatrix:  []any{0.001, 0.0, 0.0, 0.001, 0.0, 0.0},
				opCharStrings: []any{int32(1234)},
			},
			k: 2,
		},
		{
			name: "blend-scalar",
			dict: cffDict{
				opVSIndex: []any{int32(3)},
				opStdHW:   []any{dictBlendValue{Default: int32(10), Deltas: []any{int32(1), int32(-2)}}},
			},
			k: 2,
		},
		{
			name: "blend-array",
			dict: cffDict{
				opVSIndex: []any{int32(1)},
				opBlueValues: []any{
					dictBlendValue{Default: int32(-20), Deltas: []any{int32(1), int32(2)}},
					dictBlendValue{Default: int32(30), Deltas: []any{int32(-3), int32(4)}},
				},
			},
			k: 2,
		},
		{
			name: "blend-no-vsindex",
			dict: cffDict{
				opStdVW: []any{dictBlendValue{Default: int32(80), Deltas: []any{int32(5), int32(6)}}},
			},
			k: 2,
		},
		{
			name: "mixed-plain-and-blend",
			dict: cffDict{
				opStemSnapH: []any{
					int32(10),
					dictBlendValue{Default: int32(20), Deltas: []any{int32(1), int32(2)}},
					int32(30),
				},
			},
			k: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blob := tc.dict.encodeCFF2()
			out, err := decodeDictCFF2(blob, constK(tc.k))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(out, tc.dict) {
				t.Errorf("round trip failed:\n got %#v\nwant %#v", out, tc.dict)
			}
		})
	}
}

func TestDictCFF2Malformed(t *testing.T) {
	t.Run("underflow-empty", func(t *testing.T) {
		buf := []byte{byte(opBlend)}
		if _, err := decodeDictCFF2(buf, constK(2)); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("underflow-operands", func(t *testing.T) {
		// n=2, k=2 needs 6 operands but only 3 present
		buf := &bytes.Buffer{}
		encodeDictNumber(buf, int32(1))
		encodeDictNumber(buf, int32(2))
		encodeDictNumber(buf, int32(3))
		encodeDictNumber(buf, int32(2))
		buf.WriteByte(byte(opBlend))
		if _, err := decodeDictCFF2(buf.Bytes(), constK(2)); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("negative-region-count", func(t *testing.T) {
		buf := &bytes.Buffer{}
		encodeDictNumber(buf, int32(1))
		encodeDictNumber(buf, int32(1))
		buf.WriteByte(byte(opBlend))
		if _, err := decodeDictCFF2(buf.Bytes(), constK(-1)); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("region-count-error", func(t *testing.T) {
		buf := &bytes.Buffer{}
		encodeDictNumber(buf, int32(1))
		encodeDictNumber(buf, int32(1))
		buf.WriteByte(byte(opBlend))
		bad := func(int) (int, error) { return 0, errCorruptDict }
		if _, err := decodeDictCFF2(buf.Bytes(), bad); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("nested-blend", func(t *testing.T) {
		// a blended value must not be re-blended as another blend's operand
		buf := &bytes.Buffer{}
		for _, x := range []int32{1, 2, 3, 1} { // first blend n=1,k=2
			encodeDictNumber(buf, x)
		}
		buf.WriteByte(byte(opBlend))
		for _, x := range []int32{4, 5, 1} { // second blend reuses the result as base
			encodeDictNumber(buf, x)
		}
		buf.WriteByte(byte(opBlend))
		if _, err := decodeDictCFF2(buf.Bytes(), constK(2)); err == nil {
			t.Error("expected error")
		}
	})
}

func TestDictCFF2BlendCap(t *testing.T) {
	// with k=0 the cap n*(k+1)+1 <= 513 reduces to n <= 512
	build := func(n int) []byte {
		buf := &bytes.Buffer{}
		for range n {
			encodeDictNumber(buf, int32(0))
		}
		encodeDictNumber(buf, int32(n))
		buf.WriteByte(byte(opBlend))
		buf.WriteByte(byte(opBlueValues))
		return buf.Bytes()
	}

	if _, err := decodeDictCFF2(build(512), constK(0)); err != nil {
		t.Errorf("n=512 should be accepted: %v", err)
	}
	if _, err := decodeDictCFF2(build(513), constK(0)); err == nil {
		t.Error("n=513 should exceed the cap")
	}
}

func TestDictCFF2Accessors(t *testing.T) {
	d := cffDict{
		opVSIndex: []any{int32(0)},
		opStdHW:   []any{dictBlendValue{Default: int32(50), Deltas: []any{int32(3), int32(-1)}}},
		opStdVW:   []any{int32(70)},
	}

	bv, ok := d.getBlend(opStdHW)
	if !ok || bv.Default != 50 || !reflect.DeepEqual(bv.Deltas, []float64{3, -1}) {
		t.Errorf("getBlend blend: %+v ok=%v", bv, ok)
	}
	bv, ok = d.getBlend(opStdVW)
	if !ok || bv.Default != 70 || bv.Deltas != nil {
		t.Errorf("getBlend plain: %+v ok=%v", bv, ok)
	}

	// delta-decoded array: defaults and per-region deltas accumulate
	arr := cffDict{
		opBlueValues: []any{
			dictBlendValue{Default: int32(-20), Deltas: []any{int32(1), int32(2)}},
			dictBlendValue{Default: int32(50), Deltas: []any{int32(3), int32(4)}},
		},
	}.getBlendArray(opBlueValues)
	if len(arr) != 2 {
		t.Fatalf("len = %d", len(arr))
	}
	if arr[0].Default != -20 || arr[1].Default != 30 {
		t.Errorf("defaults not accumulated: %v %v", arr[0].Default, arr[1].Default)
	}
	if !reflect.DeepEqual(arr[1].Deltas, []float64{4, 6}) {
		t.Errorf("deltas not accumulated: %v", arr[1].Deltas)
	}
}

func FuzzDictCFF2(f *testing.F) {
	seeds := []cffDict{
		{opCharStrings: []any{int32(100)}},
		{opVSIndex: []any{int32(0)}, opStdHW: []any{dictBlendValue{Default: int32(10), Deltas: []any{int32(1), int32(2)}}}},
	}
	for _, d := range seeds {
		f.Add(d.encodeCFF2())
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		rc := constK(2)
		d1, err := decodeDictCFF2(data, rc)
		if err != nil {
			return
		}

		data2 := d1.encodeCFF2()

		d2, err := decodeDictCFF2(data2, rc)
		if err != nil {
			t.Fatal(err)
		}

		if len(d1) != len(d2) {
			t.Fatalf("wrong length: %d != %d", len(d1), len(d2))
		}
		for key, v1 := range d1 {
			v2, ok := d2[key]
			if !ok {
				t.Fatalf("missing key %04x", key)
			}
			if len(v1) != len(v2) {
				t.Fatalf("wrong operand count for %04x: %d != %d", key, len(v1), len(v2))
			}
			for i := range v1 {
				if !operandsClose(v1[i], v2[i]) {
					t.Fatalf("value mismatch for %04x: %v != %v", key, v1[i], v2[i])
				}
			}
		}
	})
}

// numClose reports whether two numeric DICT operands agree within the
// tolerance of the 9-significant-digit float encoding.
func numClose(a, b any) bool {
	x, y := toFloat(a), toFloat(b)
	return math.Abs(x-y) <= 1e-8*(math.Abs(x)+math.Abs(y))
}

func operandsClose(a, b any) bool {
	ba, aok := a.(dictBlendValue)
	bb, bok := b.(dictBlendValue)
	if aok != bok {
		return false
	}
	if !aok {
		return numClose(a, b)
	}
	if !numClose(ba.Default, bb.Default) || len(ba.Deltas) != len(bb.Deltas) {
		return false
	}
	for i := range ba.Deltas {
		if !numClose(ba.Deltas[i], bb.Deltas[i]) {
			return false
		}
	}
	return true
}

func TestDictDecodeFloat(t *testing.T) {
	cases := []struct {
		in  []byte
		out float64
	}{
		{[]byte{0xe2, 0xa2, 0x5f}, -2.25},
		{[]byte{0x0a, 0x14, 0x05, 0x41, 0xc3, 0xff}, 0.140541e-3},
	}
	for _, test := range cases {
		buf, x, err := decodeFloat(test.in)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(buf) != 0 {
			t.Error("not all input used")
		}
		if math.Abs(x-test.out) > 1e-6 {
			t.Errorf("wrong result: %g - %g = %g", x, test.out, x-test.out)
		}
	}
}

func FuzzFloatEncoding(f *testing.F) {
	f.Add([]byte{0x0f})
	f.Add([]byte{0xe2, 0xa2, 0x5f})
	f.Add([]byte{0x0a, 0x14, 0x05, 0x41, 0xc3, 0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, x, err := decodeFloat(data)
		if err != nil {
			return
		}
		data2 := encodeFloat(x)
		if len(data2) > len(data) {
			t.Errorf("inefficient encoding: % x -> % x", data, data2)
		}
		tail, y, err := decodeFloat(data2)
		if err != nil {
			t.Fatalf("% x -> % x -> ... %s", data, data2, err)
		}
		if len(tail) != 0 {
			t.Errorf("not all input used: % x -> % x", data, data2)
		}
		if math.Abs(x-y) > 1e-8*(math.Abs(x)+math.Abs(y)) {
			t.Errorf("%g != %g", x, y)
		}
	})
}

func TestDictDecodeInt(t *testing.T) {
	cases := []struct {
		x   int32
		enc []byte
	}{
		{0, []byte{0x8b}},
		{100, []byte{0xef}},
		{-100, []byte{0x27}},
		{1000, []byte{0xfa, 0x7c}},
		{-1000, []byte{0xfe, 0x7c}},
		{10000, []byte{0x1c, 0x27, 0x10}},
		{-10000, []byte{0x1c, 0xd8, 0xf0}},
		{100000, []byte{0x1d, 0x00, 0x01, 0x86, 0xa0}},
		{-100000, []byte{0x1d, 0xff, 0xfe, 0x79, 0x60}},
	}
	var buf []byte
	for _, test := range cases {
		buf = append(buf[:0], test.enc...)
		buf = append(buf, byte(opDebug>>8), byte(opDebug&0xFF))

		d, err := decodeDict(buf, nil)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(d) != 1 {
			t.Error("wrong DICT length")
			continue
		}

		args, ok := d[opDebug]
		if !ok {
			t.Error("wrong DICT op")
			continue
		}
		if len(args) != 1 {
			t.Error("wrong DICT args length")
			continue
		}

		x := args[0].(int32)
		if x != test.x {
			t.Errorf("wrong value: %d != %d", x, test.x)
		}
	}
}

func TestDictEncodeInt(t *testing.T) {
	var op dictOp = 7
	for i := int32(-32769); i <= 32769; i += 3 {
		d := cffDict{
			op: []any{i, i + 1, i + 2},
		}
		blob := d.encode(nil)
		d2, err := decodeDict(blob, nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(d2) != 1 {
			t.Fatal("wrong length")
		}
		args, ok := d2[op]
		if !ok {
			t.Fatal("wrong op")
		}
		if len(d[op]) != len(args) {
			t.Errorf("wrong args count: %d != %d",
				len(d[op]), len(args))
		}
		for i, x := range args {
			if x.(int32) != d[op][i].(int32) {
				t.Fatalf("wrong value: %d != %d",
					x.(int32), d[op][i].(int32))
			}
		}
	}
}

func TestDictFloat(t *testing.T) {
	cases := []float64{
		0,
		1,
		-1,
		2,
		-2,
		999999,
		-999999,
		3.1415926535,
		1.234e56,
		1.234e-56,
		-1.234e56,
		-1.234e-56,
	}
	for _, x := range cases {
		d := cffDict{
			opDebug: []any{x},
		}
		blob := d.encode(nil)
		d2, err := decodeDict(blob, nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(d2) != 1 {
			t.Fatalf("wrong length %d", len(d2))
		}
		args, ok := d2[opDebug]
		if !ok {
			t.Fatal("wrong op")
		}
		if len(args) != 1 {
			t.Errorf("wrong args count: %d != %d",
				len(args), len(d[0]))
		}
		out := args[0].(float64)
		if math.Abs(out-x) > 1e-6 || math.Abs(out-x) > 1e-8*(math.Abs(out)+math.Abs(x)) {
			t.Errorf("%g != %g", out, x)
		}
	}
}

func FuzzDict(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		ss := &cffStrings{}

		d1, err := decodeDict(data, ss)
		if err != nil {
			return
		}

		data2 := d1.encode(ss)
		if len(ss.data) != 0 {
			t.Errorf("%d strings appeared out of thin air", len(ss.data))
		}
		if len(data2) > len(data) {
			t.Errorf("inefficient encoding")
		}

		d2, err := decodeDict(data2, ss)
		if err != nil {
			t.Fatal(err)
		}

		if len(d2) != len(d1) {
			t.Fatalf("wrong length: %d != %d", len(d2), len(d1))
		}
		for key, val1 := range d1 {
			val2, ok := d2[key]
			if !ok {
				t.Fatalf("missing key %04x", key)
			}
			if len(val1) != len(val2) {
				t.Fatalf("wrong length: %d != %d", len(val1), len(val2))
			}
			for i, x1 := range val1 {
				x2 := val2[i]
				if x1num, ok := x1.(float64); ok {
					x2num, ok := x2.(float64)
					if !ok || math.Abs(x1num-x2num) > 1e-8*(math.Abs(x1num)+math.Abs(x2num)) {
						t.Fatalf("wrong value: %v != %v", x1, x2)
					}
				} else if x1 != x2 {
					t.Fatalf("wrong value: %v != %v", x1, x2)
				}
			}
		}
	})
}
