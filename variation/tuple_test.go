// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
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

package variation

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/membudget"
)

func testBudget() *membudget.Budget { return membudget.New(1 << 20) }

// handTupleAllPoints is a single-tuple block: embedded peak (1.0), all
// points, three byte deltas, cvar-like (dims=1, axisCount=1, nPoints=3).
var handTupleAllPoints = []byte{
	0x00, 0x01, // tupleVariationCount = 1
	0x00, 0x0A, // dataOffset = 10

	0x00, 0x04, // variationDataSize = 4
	0x80, 0x00, // tupleIndex: EMBEDDED_PEAK_TUPLE
	0x40, 0x00, // peak = 1.0

	0x02, 0x0A, 0xFB, 0x03, // byte run of 3: 10, -5, 3
}

func TestDecodeTupleAllPoints(t *testing.T) {
	got, err := DecodeTupleData(handTupleAllPoints, 1, 1, 3, false, testBudget())
	if err != nil {
		t.Fatal(err)
	}
	want := []TupleVariation{
		{Peak: []F2Dot14{f2(1.0)}, Deltas: []int32{10, -5, 3}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("decode mismatch (-want +got):\n%s", diff)
	}
}

// handTupleShared exercises SHARED_POINT_NUMBERS, a shared-peak reference,
// a private-point tuple with word deltas and an intermediate region.
// axisCount=1, dims=1, nPoints=10.
var handTupleShared = []byte{
	0x80, 0x02, // tupleVariationCount = 2, SHARED_POINT_NUMBERS
	0x00, 0x12, // dataOffset = 18

	// tuple 0 header: shared peak index 1, no flags
	0x00, 0x03, // variationDataSize = 3
	0x00, 0x01, // tupleIndex = 1 (shared peak)

	// tuple 1 header: embedded peak + intermediate + private points
	0x00, 0x06, // variationDataSize = 6
	0xE0, 0x00, // EMBEDDED_PEAK | INTERMEDIATE_REGION | PRIVATE_POINT_NUMBERS
	0x40, 0x00, // peak = 1.0
	0x00, 0x00, // intermediateStart = 0.0
	0x40, 0x00, // intermediateEnd = 1.0

	// serialized data at offset 18:
	// shared point numbers: count 2, byte run [2, 3] -> points 2, 5
	0x02, 0x01, 0x02, 0x03,

	// tuple 0 data (size 3): byte deltas for 2 shared points: 7, -8
	0x01, 0x07, 0xF8,

	// tuple 1 data (size 8):
	// private points: count 1, byte run [4] -> point 4
	0x01, 0x00, 0x04,
	// word deltas for 1 point: 300
	0x40, 0x01, 0x2C,
}

func TestDecodeTupleShared(t *testing.T) {
	got, err := DecodeTupleData(handTupleShared, 1, 1, 10, true, testBudget())
	if err != nil {
		t.Fatal(err)
	}
	want := []TupleVariation{
		{
			SharedPeak: 1,
			Points:     []uint16{2, 5},
			Deltas:     []int32{7, -8},
		},
		{
			Peak:              []F2Dot14{f2(1.0)},
			IntermediateStart: []F2Dot14{f2(0.0)},
			IntermediateEnd:   []F2Dot14{f2(1.0)},
			Points:            []uint16{4},
			Deltas:            []int32{300},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("decode mismatch (-want +got):\n%s", diff)
	}
}

func TestDecodeTupleSharedDisallowed(t *testing.T) {
	_, err := DecodeTupleData(handTupleShared, 1, 1, 10, false, testBudget())
	if err == nil {
		t.Error("expected error when shared points disallowed")
	}
}

func TestTupleRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		axisCount int
		dims      int
		nPoints   int
		tuples    []TupleVariation
	}{
		{
			name: "empty", axisCount: 1, dims: 2, nPoints: 4,
			tuples: nil,
		},
		{
			name: "all points, byte deltas", axisCount: 2, dims: 2, nPoints: 3,
			tuples: []TupleVariation{
				{Peak: []F2Dot14{f2(1), f2(0)}, Deltas: []int32{1, 2, 3, -1, -2, -3}},
			},
		},
		{
			name: "private points, word deltas", axisCount: 1, dims: 1, nPoints: 20,
			tuples: []TupleVariation{
				{
					Peak:   []F2Dot14{f2(-0.5)},
					Points: []uint16{0, 5, 19},
					Deltas: []int32{300, -300, 30000},
				},
			},
		},
		{
			name: "shared-peak refs and intermediate", axisCount: 2, dims: 2, nPoints: 5,
			tuples: []TupleVariation{
				{SharedPeak: 3, Deltas: []int32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
				{
					Peak:              []F2Dot14{f2(1), f2(1)},
					IntermediateStart: []F2Dot14{f2(0), f2(0.5)},
					IntermediateEnd:   []F2Dot14{f2(1), f2(1)},
					Points:            []uint16{1, 2},
					Deltas:            []int32{5, 6, 7, 8},
				},
			},
		},
		{
			name: "16-bit point count", axisCount: 1, dims: 1, nPoints: 400,
			tuples: []TupleVariation{
				{Peak: []F2Dot14{f2(1)}, Points: point130(), Deltas: deltas130()},
			},
		},
	}
	for _, c := range cases {
		enc, err := EncodeTupleData(c.tuples, c.axisCount, c.dims, c.nPoints, nil)
		if err != nil {
			t.Errorf("%s: encode: %v", c.name, err)
			continue
		}
		enc2, err := EncodeTupleData(c.tuples, c.axisCount, c.dims, c.nPoints, nil)
		if err != nil || string(enc) != string(enc2) {
			t.Errorf("%s: encode not deterministic", c.name)
		}
		got, err := DecodeTupleData(enc, c.axisCount, c.dims, c.nPoints, true, testBudget())
		if err != nil {
			t.Errorf("%s: decode: %v", c.name, err)
			continue
		}
		if diff := cmp.Diff(c.tuples, got); diff != "" {
			t.Errorf("%s: round trip mismatch (-want +got):\n%s", c.name, diff)
		}
	}
}

func point130() []uint16 {
	p := make([]uint16, 130)
	for i := range p {
		p[i] = uint16(i * 2)
	}
	return p
}

func deltas130() []int32 {
	d := make([]int32, 130)
	for i := range d {
		d[i] = int32(i - 65)
	}
	return d
}

func TestEncodeNeverEmitsSharedPoints(t *testing.T) {
	// two tuples referencing the same point set must not use
	// SHARED_POINT_NUMBERS on encode.
	tuples := []TupleVariation{
		{Peak: []F2Dot14{f2(1)}, Points: []uint16{1, 2}, Deltas: []int32{1, 2}},
		{Peak: []F2Dot14{f2(1)}, Points: []uint16{1, 2}, Deltas: []int32{3, 4}},
	}
	enc, err := EncodeTupleData(tuples, 1, 1, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if enc[0]&0x80 != 0 {
		t.Error("encode set SHARED_POINT_NUMBERS flag")
	}
}

func TestTupleScalar(t *testing.T) {
	shared := [][]F2Dot14{
		{f2(0.5)},
		{f2(1.0)},
	}
	cases := []struct {
		name   string
		tv     TupleVariation
		coords []F2Dot14
		want   float64
	}{
		{
			"embedded peak positive implicit",
			TupleVariation{Peak: []F2Dot14{f2(1.0)}},
			[]F2Dot14{f2(0.5)}, 0.5,
		},
		{
			"embedded peak negative implicit",
			TupleVariation{Peak: []F2Dot14{f2(-0.5)}},
			[]F2Dot14{f2(-0.25)}, 0.5,
		},
		{
			"explicit intermediate",
			TupleVariation{
				Peak:              []F2Dot14{f2(0.5)},
				IntermediateStart: []F2Dot14{f2(0)},
				IntermediateEnd:   []F2Dot14{f2(1)},
			},
			[]F2Dot14{f2(0.25)}, 0.5,
		},
		{
			"shared peak reference",
			TupleVariation{SharedPeak: 0},
			[]F2Dot14{f2(0.25)}, 0.5,
		},
		{
			"shared peak out of range",
			TupleVariation{SharedPeak: 9},
			[]F2Dot14{f2(0.25)}, 0.0,
		},
		{
			"nil shared with nil peak",
			TupleVariation{SharedPeak: 0},
			[]F2Dot14{f2(0.25)}, 0.0,
		},
	}
	for _, c := range cases {
		var sh [][]F2Dot14
		if c.name != "nil shared with nil peak" {
			sh = shared
		}
		got := c.tv.Scalar(c.coords, sh)
		if got != c.want {
			t.Errorf("%s: Scalar = %v, want %v", c.name, got, c.want)
		}
	}
}

func FuzzTupleData(f *testing.F) {
	f.Add(append([]byte{1, 1, 3}, handTupleAllPoints...))
	f.Add(append([]byte{1, 1, 10}, handTupleShared...))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}
		axisCount := int(data[0] % 4)
		dims := int(data[1]%2) + 1
		nPoints := int(data[2] % 32)
		body := data[3:]

		budget := membudget.New(int64(len(data))*512 + 4096)
		tuples, err := DecodeTupleData(body, axisCount, dims, nPoints, true, budget)
		if err != nil {
			return
		}
		enc, err := EncodeTupleData(tuples, axisCount, dims, nPoints, nil)
		if err != nil {
			t.Fatalf("encode after successful decode: %v", err)
		}
		tuples2, err := DecodeTupleData(enc, axisCount, dims, nPoints, true, membudget.New(1<<24))
		if err != nil {
			t.Fatalf("re-decode: %v", err)
		}
		if diff := cmp.Diff(tuples, tuples2); diff != "" {
			t.Fatalf("round trip mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestEncodeTupleAllPointsIsExplicit checks that a tuple whose deltas apply to
// every point still stores a point-number array, namely the count zero which
// stands for "all points".  Leaving the array out makes the tuple refer to
// shared point numbers the block does not carry, and a reader which does not
// assume "all points" in that case drops every delta.
func TestEncodeTupleAllPointsIsExplicit(t *testing.T) {
	enc, err := EncodeTupleData([]TupleVariation{
		{Peak: []F2Dot14{f2(1.0)}, Deltas: []int32{10, -5, 3}},
	}, 1, 1, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01, // tupleVariationCount = 1, no SHARED_POINT_NUMBERS
		0x00, 0x0A, // dataOffset = 10

		0x00, 0x05, // variationDataSize = 5
		0xA0, 0x00, // EMBEDDED_PEAK_TUPLE | PRIVATE_POINT_NUMBERS
		0x40, 0x00, // peak = 1.0

		0x00,                   // point count 0: all points
		0x02, 0x0A, 0xFB, 0x03, // byte run of 3: 10, -5, 3
	}
	if diff := cmp.Diff(want, enc); diff != "" {
		t.Errorf("encoding mismatch (-want +got):\n%s", diff)
	}
}

// TestEncodeTupleDeltaRunsPerDimension checks that delta runs restart at each
// dimension boundary.  The deltas of a gvar tuple are the x-deltas of every
// point followed by the y-deltas, and readers decode the two arrays
// separately, so a run spanning the boundary leaves them short.
func TestEncodeTupleDeltaRunsPerDimension(t *testing.T) {
	// two points, x-deltas 0 0, y-deltas 0 7: a greedy packer would merge
	// the three leading zeros into one run.
	enc, err := EncodeTupleData([]TupleVariation{
		{Peak: []F2Dot14{f2(1.0)}, Deltas: []int32{0, 0, 0, 7}},
	}, 1, 2, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01, // tupleVariationCount = 1
		0x00, 0x0A, // dataOffset = 10

		0x00, 0x05, // variationDataSize = 5
		0xA0, 0x00, // EMBEDDED_PEAK_TUPLE | PRIVATE_POINT_NUMBERS
		0x40, 0x00, // peak = 1.0

		0x00,       // point count 0: all points
		0x81,       // x: run of 2 zeros
		0x80,       // y: run of 1 zero
		0x00, 0x07, // y: byte run of 1: 7
	}
	if diff := cmp.Diff(want, enc); diff != "" {
		t.Errorf("encoding mismatch (-want +got):\n%s", diff)
	}
}

// TestEncodeTupleDeltaCount checks that a tuple is rejected unless it carries
// one delta per point and dimension.  The count is not stored in the file: a
// reader derives it from the point numbers and stops once it has that many, so
// a surplus delta is dropped on the next read and a missing one sends the
// reader past the end of the tuple's data.
func TestEncodeTupleDeltaCount(t *testing.T) {
	cases := []struct {
		name   string
		points []uint16
		deltas []int32
		dims   int
		ok     bool
	}{
		{"all points", nil, []int32{1, 2, 3, 4}, 2, true},
		{"all points, short", nil, []int32{1, 2, 3}, 2, false},
		{"all points, long", nil, []int32{1, 2, 3, 4, 5}, 2, false},
		{"listed points", []uint16{0, 1}, []int32{1, 2, 3, 4}, 2, true},
		{"listed points, short", []uint16{0, 1}, []int32{1, 2}, 2, false},
		{"listed points, long", []uint16{0, 1}, []int32{1, 2, 3, 4, 5, 6}, 2, false},
		// a repeated point number gets its own delta
		{"repeated point", []uint16{1, 1}, []int32{1, 2, 3, 4}, 2, true},
		{"one dimension", nil, []int32{1, 2}, 1, true},
		{"one dimension, long", nil, []int32{1, 2, 3}, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tuples := []TupleVariation{
				{Peak: []F2Dot14{f2(1.0)}, Points: c.points, Deltas: c.deltas},
			}
			_, err := EncodeTupleData(tuples, 1, c.dims, 2, nil)
			if (err == nil) != c.ok {
				t.Errorf("got error %v, want ok=%v", err, c.ok)
			}
		})
	}
}
