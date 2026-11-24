// Copyright 2021 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cff

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/geom/matrix"
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
			blob, err := tc.dict.encodeCFF2()
			if err != nil {
				t.Fatal(err)
			}
			out := decodeDictCFF2(blob, constK(tc.k))
			if !reflect.DeepEqual(out, tc.dict) {
				t.Errorf("round trip failed:\n got %#v\nwant %#v", out, tc.dict)
			}
		})
	}
}

// dictNumbers writes integer DICT operands to buf.
func dictNumbers(buf *bytes.Buffer, xs ...int32) {
	for _, x := range xs {
		encodeDictNumber(buf, x)
	}
}

// TestDictCFF2Malformed checks that an operator whose operands cannot be
// decoded is dropped, while the rest of the DICT is kept.
func TestDictCFF2Malformed(t *testing.T) {
	cases := []struct {
		name        string
		k           int
		regionCount func(int) (int, error)
		operands    func(buf *bytes.Buffer)
	}{
		{
			name:     "underflow-empty",
			k:        2,
			operands: func(buf *bytes.Buffer) { buf.WriteByte(byte(opBlend)) },
		},
		{
			// n=2, k=2 needs 6 operands but only 3 are present
			name: "underflow-operands",
			k:    2,
			operands: func(buf *bytes.Buffer) {
				dictNumbers(buf, 1, 2, 3, 2)
				buf.WriteByte(byte(opBlend))
			},
		},
		{
			name: "negative-region-count",
			k:    -1,
			operands: func(buf *bytes.Buffer) {
				dictNumbers(buf, 1, 1)
				buf.WriteByte(byte(opBlend))
			},
		},
		{
			name:        "region-count-error",
			regionCount: func(int) (int, error) { return 0, errCorruptDict },
			operands: func(buf *bytes.Buffer) {
				dictNumbers(buf, 1, 1)
				buf.WriteByte(byte(opBlend))
			},
		},
		{
			// a blended value used as a delta of another blend
			name: "blended-delta",
			k:    2,
			operands: func(buf *bytes.Buffer) {
				dictNumbers(buf, 10, 1, 2, 3, 1)
				buf.WriteByte(byte(opBlend))
				dictNumbers(buf, 5, 1)
				buf.WriteByte(byte(opBlend))
			},
		},
		{
			// the deltas of two blends sum to a value beyond the range
			// of a DICT real
			name: "delta-sum-overflow",
			k:    1,
			operands: func(buf *bytes.Buffer) {
				dictNumbers(buf, 10)
				encodeDictNumber(buf, 9e299)
				dictNumbers(buf, 1)
				buf.WriteByte(byte(opBlend))
				encodeDictNumber(buf, 9e299)
				dictNumbers(buf, 1)
				buf.WriteByte(byte(opBlend))
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := c.regionCount
			if rc == nil {
				rc = constK(c.k)
			}
			buf := &bytes.Buffer{}
			dictNumbers(buf, 50)
			buf.WriteByte(byte(opStdHW))
			c.operands(buf)
			buf.WriteByte(byte(opStdVW))
			dictNumbers(buf, 7)
			buf.WriteByte(byte(opSubrs))

			got := decodeDictCFF2(buf.Bytes(), rc)
			want := cffDict{
				opStdHW: []any{int32(50)},
				opSubrs: []any{int32(7)},
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %#v, want %#v", got, want)
			}
		})
	}
}

// TestDictCFF2Reblend checks that a blend whose default value was blended
// before adds its deltas to the earlier ones.
func TestDictCFF2Reblend(t *testing.T) {
	buf := &bytes.Buffer{}
	dictNumbers(buf, 1, 2, 3, 1)
	buf.WriteByte(byte(opBlend))
	dictNumbers(buf, 4, 5, 1)
	buf.WriteByte(byte(opBlend))
	buf.WriteByte(byte(opStdHW))

	got := decodeDictCFF2(buf.Bytes(), constK(2))
	want := cffDict{
		opStdHW: []any{dictBlendValue{Default: int32(1), Deltas: []any{int32(6), int32(8)}}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong value (-want +got):\n%s", diff)
	}

	// an array whose second element is blended twice
	buf.Reset()
	dictNumbers(buf, -10, 10, 1, 2, 3, 4, 2)
	buf.WriteByte(byte(opBlend))
	dictNumbers(buf, 5, 6, 1)
	buf.WriteByte(byte(opBlend))
	buf.WriteByte(byte(opBlueValues))

	d := decodeDictCFF2(buf.Bytes(), constK(2))
	gotArr := toBlends(d.getBlendArray(opBlueValues))
	wantArr := []Blend{
		{Default: -10, Deltas: []float64{1, 2}},
		{Default: 0, Deltas: []float64{9, 12}},
	}
	if diff := cmp.Diff(wantArr, gotArr); diff != "" {
		t.Errorf("wrong array (-want +got):\n%s", diff)
	}
}

// TestDictCFF2Truncated checks that decoding stops at bytes which are not a
// number or an operator, and keeps the operators decoded before them.
func TestDictCFF2Truncated(t *testing.T) {
	stdVW := byte(opStdVW)
	cases := []struct {
		name string
		tail []byte // bytes after the operand of StdVW
	}{
		{"short 28", []byte{28, 0}},
		{"short 29", []byte{29, 0, 0, 0}},
		{"bad real", []byte{30, 0x1d, stdVW}},
		{"short 247", []byte{247}},
		{"short 251", []byte{251}},
		{"lone 12", []byte{12}},
		{"byte 255", []byte{255, stdVW}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			encodeDictNumber(buf, int32(50))
			buf.WriteByte(byte(opStdHW))
			encodeDictNumber(buf, int32(70))
			buf.Write(c.tail)

			got := decodeDictCFF2(buf.Bytes(), constK(0))
			want := cffDict{opStdHW: []any{int32(50)}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %#v, want %#v", got, want)
			}
		})
	}
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

	if _, ok := decodeDictCFF2(build(512), constK(0))[opBlueValues]; !ok {
		t.Error("n=512 should be accepted")
	}
	if _, ok := decodeDictCFF2(build(513), constK(0))[opBlueValues]; ok {
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

// TestBlendArrayPlainAfterBlended checks that a plain operand of an array
// is a difference without deltas, so that its element keeps the deltas of
// the element before it.
func TestBlendArrayPlainAfterBlended(t *testing.T) {
	buf := &bytes.Buffer{}
	for _, v := range []int32{-10, 1, 1} {
		encodeDictNumber(buf, v)
	}
	buf.WriteByte(byte(opBlend))
	for _, v := range []int32{10, 440, 10, -20, -1, 2} {
		encodeDictNumber(buf, v)
	}
	buf.WriteByte(byte(opBlend))
	buf.WriteByte(byte(opBlueValues))

	d := decodeDictCFF2(buf.Bytes(), constK(1))
	got := toBlends(d.getBlendArray(opBlueValues))
	want := []Blend{
		{Default: -10, Deltas: []float64{1}},
		{Default: 0, Deltas: []float64{1}},
		{Default: 440, Deltas: []float64{-19}},
		{Default: 450, Deltas: []float64{-20}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong array (-want +got):\n%s", diff)
	}
}

// TestBlendArrayZeroDeltas checks that an element whose deltas sum to zero
// reads as a constant.
func TestBlendArrayZeroDeltas(t *testing.T) {
	d := cffDict{
		opBlueValues: []any{
			dictBlendValue{Default: int32(-10), Deltas: []any{int32(1)}},
			dictBlendValue{Default: int32(10), Deltas: []any{int32(-1)}},
		},
	}
	got := toBlends(d.getBlendArray(opBlueValues))
	want := []Blend{
		{Default: -10, Deltas: []float64{1}},
		{Default: 0},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong array (-want +got):\n%s", diff)
	}
}

// TestDictCFF2ReservedOperator checks that a reserved operator is ignored
// together with its operands, and that decoding continues after it.
func TestDictCFF2ReservedOperator(t *testing.T) {
	for _, op := range []byte{25, 26, 27, 31} {
		buf := &bytes.Buffer{}
		encodeDictNumber(buf, int32(50))
		buf.WriteByte(byte(opStdHW))
		encodeDictNumber(buf, int32(70))
		buf.WriteByte(op)
		encodeDictNumber(buf, int32(80))
		buf.WriteByte(byte(opStdVW))

		got := decodeDictCFF2(buf.Bytes(), constK(0))
		want := cffDict{
			opStdHW: []any{int32(50)},
			opStdVW: []any{int32(80)},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("operator %d: got %#v, want %#v", op, got, want)
		}
	}
}

// TestDictCFF2VSIndexAfterBlend checks that a vsindex operator after a blend
// is ignored, so that all blends use the same vsindex.
func TestDictCFF2VSIndexAfterBlend(t *testing.T) {
	regionCount := func(vsindex int) (int, error) { return vsindex + 1, nil }

	buf := &bytes.Buffer{}
	for _, v := range []int32{10, 1, 1} {
		encodeDictNumber(buf, v)
	}
	buf.WriteByte(byte(opBlend))
	buf.WriteByte(byte(opStdHW))
	encodeDictNumber(buf, int32(1))
	buf.WriteByte(byte(opVSIndex))
	for _, v := range []int32{20, 3, 1} {
		encodeDictNumber(buf, v)
	}
	buf.WriteByte(byte(opBlend))
	buf.WriteByte(byte(opStdVW))

	got := decodeDictCFF2(buf.Bytes(), regionCount)
	want := cffDict{
		opStdHW: []any{dictBlendValue{Default: int32(10), Deltas: []any{int32(1)}}},
		opStdVW: []any{dictBlendValue{Default: int32(20), Deltas: []any{int32(3)}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// TestEncodeCFF2StackLimit checks that the operands of a key are split over
// several blend operators such that the operand stack never holds more than
// 513 values, and that an error is returned where this is impossible.
func TestEncodeCFF2StackLimit(t *testing.T) {
	blended := func(n, k int) []any {
		res := make([]any, n)
		for i := range res {
			deltas := make([]any, k)
			for j := range deltas {
				deltas[j] = int32(i + j)
			}
			res[i] = dictBlendValue{Default: int32(i), Deltas: deltas}
		}
		return res
	}

	for _, k := range []int{1, 40, 72, 255} {
		d := cffDict{opBlueValues: blended(14, k)}
		blob, err := d.encodeCFF2()
		if err != nil {
			t.Errorf("k=%d: %v", k, err)
			continue
		}
		if depth, _ := scanDictCFF2(t, blob, k); depth > maxStackCFF2 {
			t.Errorf("k=%d: stack depth %d exceeds %d", k, depth, maxStackCFF2)
		}
		if got := decodeDictCFF2(blob, constK(k)); !reflect.DeepEqual(got, d) {
			t.Errorf("k=%d: round trip failed", k)
		}
	}

	for _, c := range []struct{ n, k int }{{1, 512}, {14, 510}} {
		d := cffDict{opBlueValues: blended(c.n, c.k)}
		if _, err := d.encodeCFF2(); err == nil {
			t.Errorf("n=%d, k=%d: expected error", c.n, c.k)
		}
	}

	// plain operands only
	plain := func(n int) []any {
		res := make([]any, n)
		for i := range res {
			res[i] = int32(i)
		}
		return res
	}
	if _, err := (cffDict{opBlueValues: plain(maxStackCFF2)}).encodeCFF2(); err != nil {
		t.Errorf("%d plain operands: %v", maxStackCFF2, err)
	}
	if _, err := (cffDict{opBlueValues: plain(maxStackCFF2 + 1)}).encodeCFF2(); err == nil {
		t.Errorf("%d plain operands: expected error", maxStackCFF2+1)
	}
}

// scanDictCFF2 simulates the operand stack while decoding the CFF2 DICT data
// buf, whose blends use k regions.  It returns the largest number of values
// on the stack, and the number of blended values n of each blend operator in
// order.  The data must contain only integer operands.
func scanDictCFF2(t *testing.T, buf []byte, k int) (peak int, blends []int) {
	t.Helper()
	depth := 0
	var last int32
	push := func(v int32, n int) {
		last = v
		depth++
		peak = max(peak, depth)
		buf = buf[n:]
	}
	for len(buf) > 0 {
		b0 := buf[0]
		switch {
		case b0 == byte(opBlend):
			n := int(last)
			blends = append(blends, n)
			depth += n - (n*(k+1) + 1)
			buf = buf[1:]
		case b0 == 12:
			depth = 0
			buf = buf[2:]
		case b0 <= 24:
			depth = 0
			buf = buf[1:]
		case b0 == 28:
			push(int32(int16(uint16(buf[1])<<8|uint16(buf[2]))), 3)
		case b0 == 29:
			push(int32(uint32(buf[1])<<24|uint32(buf[2])<<16|uint32(buf[3])<<8|uint32(buf[4])), 5)
		case b0 >= 32 && b0 <= 246:
			push(int32(b0)-139, 1)
		case b0 >= 247 && b0 <= 250:
			push((int32(b0)-247)*256+int32(buf[1])+108, 2)
		case b0 >= 251 && b0 <= 254:
			push(-(int32(b0)-251)*256-int32(buf[1])-108, 2)
		default:
			t.Fatalf("unexpected byte %d", b0)
		}
	}
	return peak, blends
}

func FuzzDictCFF2(f *testing.F) {
	seeds := []cffDict{
		{opCharStrings: []any{int32(100)}},
		{opVSIndex: []any{int32(0)}, opStdHW: []any{dictBlendValue{Default: int32(10), Deltas: []any{int32(1), int32(2)}}}},
	}
	for _, d := range seeds {
		blob, err := d.encodeCFF2()
		if err != nil {
			f.Fatal(err)
		}
		f.Add(blob)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		rc := constK(2)
		d1 := decodeDictCFF2(data, rc)
		data2, err := d1.encodeCFF2()
		if err != nil {
			t.Fatal(err)
		}
		d2 := decodeDictCFF2(data2, rc)

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
		if x != y {
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

// TestEncodeFloatExact checks that a real operand survives a write and read
// unchanged.  The encoder used to keep nine significant digits, which lost the
// tail of any value needing more.
func TestEncodeFloatExact(t *testing.T) {
	for _, x := range []float64{
		0, 1, -1, 0.5, 0.039625, 1.0 / 3.0, math.Pi, -math.Pi,
		123456789012345.0, 1e-299, 1e300, -1e300, 2e-5, 0.00302,
	} {
		data := encodeFloat(x)
		tail, got, err := decodeFloat(data)
		if err != nil {
			t.Errorf("%v: %v", x, err)
			continue
		}
		if len(tail) != 0 {
			t.Errorf("%v: %d bytes left over", x, len(tail))
		}
		if got != x {
			t.Errorf("%v encoded as % x, read back as %v", x, data, got)
		}
	}
}

// TestFontMatrixNearDefault checks that a font matrix close to the one a
// reader assumes in the absence of the FontMatrix entry is written out, rather
// than omitted and read back as the default.  A font with 1001 units per em,
// for example, has a matrix which differs from the default by only 10^-6.
func TestFontMatrixNearDefault(t *testing.T) {
	cases := []struct {
		name string
		top  matrix.Matrix
		fds  []matrix.Matrix
	}{
		{"name-keyed", matrix.Scale(1.0/1001, 1.0/1001), nil},
		{"CID-keyed top", matrix.Scale(1.000001, 1.000001), []matrix.Matrix{matrix.Scale(0.001, 0.001), matrix.Scale(0.001, 0.001)}},
		{"CID-keyed FD", matrix.Identity, []matrix.Matrix{matrix.Scale(0.001, 0.001), matrix.Scale(1.0/1001, 1.0/1001)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := writeRead(t, fontWithMatrices(c.top, c.fds))
			if got.FontInfo.FontMatrix != c.top {
				t.Errorf("top matrix: got %v, want %v", got.FontInfo.FontMatrix, c.top)
			}
			if diff := cmp.Diff(c.fds, got.FontMatrices); diff != "" {
				t.Errorf("FD matrices (-want +got):\n%s", diff)
			}
		})
	}
}

// TestGetFontMatrixIntegerOperands checks that FontMatrix entries encoded as
// integers are read, not discarded in favour of the default matrix.
func TestGetFontMatrixIntegerOperands(t *testing.T) {
	d := cffDict{
		opFontMatrix: []any{1.0 / 2048, int32(0), int32(0), 1.0 / 2048, int32(0), int32(0)},
	}
	want := matrix.Matrix{1.0 / 2048, 0, 0, 1.0 / 2048, 0, 0}
	for _, isCIDKeyed := range []bool{false, true} {
		if got := d.getFontMatrix(opFontMatrix, isCIDKeyed); got != want {
			t.Errorf("isCIDKeyed=%t: got %v, want %v", isCIDKeyed, got, want)
		}
	}
}
