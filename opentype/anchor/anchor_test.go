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
		{"format3-shared", Table{X: 5, Y: 6,
			XDev: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1},
			YDev: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}}},
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

// TestSharedDevice checks that identical Device tables are written once.
func TestSharedDevice(t *testing.T) {
	dev := func() *device.Table {
		return &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}
	}
	tab := Table{XDev: dev(), YDev: dev()}
	buf := tab.Append(nil)
	if want := 10 + dev().EncodeLen(); len(buf) != want {
		t.Errorf("length %d, want %d", len(buf), want)
	}
	if buf[6] != buf[8] || buf[7] != buf[9] {
		t.Errorf("offsets differ: % x", buf[6:10])
	}

	other := Table{XDev: dev(), YDev: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, 1}, DeltaFormat: 1}}
	if want := 10 + 2*dev().EncodeLen(); other.EncodeLen() != want {
		t.Errorf("distinct tables: length %d, want %d", other.EncodeLen(), want)
	}
}

// TestReaderSharing checks that a Reader returns one table for all reads at
// the same position, and shares Device tables between anchors.
func TestReaderSharing(t *testing.T) {
	dev := &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}

	// two format 3 anchors, followed by one Device table which is used for
	// the x and y offsets of both
	buf := markBE16(3, 1, 2, 20, 20)
	buf = append(buf, markBE16(3, 3, 4, 10, 10)...)
	buf = append(buf, dev.Encode()...)

	p := parser.New(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
	r := NewReader(p)
	x, err := r.Read(0)
	if err != nil {
		t.Fatal(err)
	}
	x2, err := r.Read(0)
	if err != nil {
		t.Fatal(err)
	}
	if x != x2 {
		t.Error("same position gave different tables")
	}
	y, err := r.Read(10)
	if err != nil {
		t.Fatal(err)
	}
	if x.XDev == nil || x.XDev != x.YDev || x.XDev != y.XDev || x.XDev != y.YDev {
		t.Error("Device table not shared")
	}
	if diff := cmp.Diff(dev, x.XDev); diff != "" {
		t.Errorf("wrong Device table (-want +got):\n%s", diff)
	}
	if _, err := r.Read(int64(len(buf))); err == nil {
		t.Error("read beyond the data succeeded")
	}
}

func markBE16(vals ...int) []byte {
	var buf []byte
	for _, v := range vals {
		buf = append(buf, byte(v>>8), byte(v))
	}
	return buf
}

// bigDevice returns a format 3 Device table which encodes to 65542 bytes,
// with the given first delta.
func bigDevice(first int8) *device.Table {
	deltas := make([]int8, 65535)
	deltas[0] = first
	return &device.Table{StartSize: 1, EndSize: 65535, Deltas: deltas, DeltaFormat: 3}
}

// TestLargeDeviceFirst checks that a large XDev table is stored after a
// short YDev table, so that both can be reached.
func TestLargeDeviceFirst(t *testing.T) {
	tab := Table{
		X: 1, Y: 2,
		XDev: bigDevice(1),
		YDev: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1},
	}
	data := tab.Append(nil)
	if len(data) != tab.EncodeLen() {
		t.Errorf("encodeLen mismatch: %d != %d", tab.EncodeLen(), len(data))
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	got, err := Read(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(tab, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestDevicesTooLarge(t *testing.T) {
	tab := Table{XDev: bigDevice(1), YDev: bigDevice(2)}
	if NewReader(nil).fits(&tab) {
		t.Error("fits reports success")
	}
	defer func() {
		if recover() == nil {
			t.Error("expected panic, got none")
		}
	}()
	tab.Append(nil)
}

// TestReadDropsUnreachableDevice checks that the YDev table is dropped on
// read if both Device tables are too large to be reached when written.  The
// tables overlap in the input: YDev starts inside the deltas of XDev.
func TestReadDropsUnreachableDevice(t *testing.T) {
	devHeader := []byte{0, 1, 0xFF, 0xFF, 0, 3}

	data := []byte{0, 3, 0, 1, 0, 2, 0, 10, 0, 16}
	data = append(data, devHeader...)
	data = append(data, devHeader...) // the first deltas of XDev
	data = append(data, make([]byte, 65536)...)

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	got, err := Read(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.XDev == nil || got.XDev.EncodeLen() != 65542 {
		t.Fatal("XDev not read")
	}
	if got.YDev != nil {
		t.Error("YDev kept")
	}

	data2 := got.Append(nil)
	p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
	got2, err := Read(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(got, got2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}
