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
)

func TestParsePackedPoints(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		nPoints int
		want    []uint16
	}{
		{"all points", []byte{0x00}, 100, nil},
		{"byte run", []byte{0x03, 0x02, 0x01, 0x02, 0x03}, 100, []uint16{1, 3, 6}},
		{"word run", []byte{0x02, 0x81, 0x01, 0x00, 0x01, 0x00}, 1000, []uint16{256, 512}},
	}
	for _, c := range cases {
		r := &byteReader{data: c.data, pos: 0}
		got, err := parsePackedPoints(r, c.nPoints, testBudget())
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s: mismatch (-want +got):\n%s", c.name, diff)
		}
	}
}

func TestParsePackedPointsOutOfRange(t *testing.T) {
	// accumulated point number 6 is not < nPoints (5).
	r := &byteReader{data: []byte{0x01, 0x00, 0x06}, pos: 0}
	if _, err := parsePackedPoints(r, 5, testBudget()); err == nil {
		t.Error("expected out-of-range error")
	}
}

func TestParsePackedPointsExactCount(t *testing.T) {
	// run supplies 3 values but count is 2: over-supply is an error for
	// point runs.
	r := &byteReader{data: []byte{0x02, 0x02, 0x01, 0x02, 0x03}, pos: 0}
	if _, err := parsePackedPoints(r, 100, testBudget()); err == nil {
		t.Error("expected exact-count error")
	}
}

func TestPackedPointsRoundTrip(t *testing.T) {
	cases := [][]uint16{
		{1, 3, 6},
		{0, 5, 19},
		{256, 512, 513},
		point130(),
	}
	for i, points := range cases {
		enc, err := encodePackedPoints(points)
		if err != nil {
			t.Errorf("case %d: %v", i, err)
			continue
		}
		r := &byteReader{data: enc, pos: 0}
		got, err := parsePackedPoints(r, 1000, testBudget())
		if err != nil {
			t.Errorf("case %d: %v", i, err)
			continue
		}
		if diff := cmp.Diff(points, got); diff != "" {
			t.Errorf("case %d: mismatch (-want +got):\n%s", i, diff)
		}
	}
}
