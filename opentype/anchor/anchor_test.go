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

package anchor

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
)

func TestRoundTrip(t *testing.T) {
	cp := uint16(7)
	cases := []struct {
		name string
		tab  Table
	}{
		{"format1", Table{X: 100, Y: -200}},
		{"format1-origin", Table{X: 0, Y: 0}},
		{"format2", Table{X: 1, Y: 2, ContourPoint: &cp}},
		{"format3-x", Table{X: 3, Y: 4,
			XDev: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}}},
		{"format3-y", Table{X: 7, Y: 8,
			YDev: &device.Table{StartSize: 1, EndSize: 1, Deltas: []int8{1}, DeltaFormat: 1}}},
		{"format3-xy", Table{X: 5, Y: 6,
			XDev: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1},
			YDev: &device.Table{OuterIndex: 2, InnerIndex: 3, DeltaFormat: device.VariationIndexFormat}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := c.tab.Append(nil)
			if len(buf) != c.tab.EncodeLen() {
				t.Errorf("EncodeLen = %d, Append wrote %d", c.tab.EncodeLen(), len(buf))
			}
			p := parser.New(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
			got, err := Read(p, 0)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(c.tab, got); diff != "" {
				t.Errorf("round trip (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRejectInvalidFormat(t *testing.T) {
	data := []byte{0, 4, 0, 0, 0, 0} // anchorFormat = 4
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if _, err := Read(p, 0); err == nil {
		t.Error("expected error for invalid anchor format")
	}
}
