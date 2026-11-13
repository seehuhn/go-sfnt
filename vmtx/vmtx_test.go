// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package vmtx

import (
	"encoding/binary"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/internal/lmetrics"
)

func testInfo() *Info {
	return &Info{
		Heights: []funit.Uint16{1000, 900, 800, 800},
		GlyphExtents: []funit.Rect16{
			{LLx: 10, LLy: -100, URx: 90, URy: 800},
			{LLx: 20, LLy: 0, URx: 200, URy: 700},
			{},
			{LLx: 40, LLy: -50, URx: 300, URy: 750},
		},
		Ascent:         500,
		Descent:        -500,
		LineGap:        20,
		CaretSlopeRise: 0,
		CaretSlopeRun:  1,
		CaretOffset:    2,
		TSB:            []funit.Int16{100, 150, 0, 50},
	}
}

func TestRoundTrip(t *testing.T) {
	i1 := testInfo()
	vhea, vmtx := i1.Encode()

	i2, err := Decode(vhea, vmtx)
	if err != nil {
		t.Fatal(err)
	}
	want := *i1
	want.GlyphExtents = nil
	if diff := cmp.Diff(&want, i2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}

	vhea2, vmtx2 := i2.Encode()
	i3, err := Decode(vhea2, vmtx2)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(i2, i3); diff != "" {
		t.Errorf("second round trip failed (-want +got):\n%s", diff)
	}
}

func TestDerivedFields(t *testing.T) {
	info := testInfo()
	vhea, _ := info.Encode()

	h, err := lmetrics.ParseHeader(vhea)
	if err != nil {
		t.Fatal(err)
	}
	// glyph 2 has an empty bounding box and is skipped
	if h.Version != 0x00011000 {
		t.Errorf("wrong version %08x", h.Version)
	}
	if h.AdvanceMax != 1000 {
		t.Errorf("wrong advanceHeightMax %d", h.AdvanceMax)
	}
	if h.MinBearing1 != 50 {
		t.Errorf("wrong minTopSideBearing %d", h.MinBearing1)
	}
	// bottom side bearings: 1000-100-900=0, 900-150-700=50, 800-50-800=-50
	if h.MinBearing2 != -50 {
		t.Errorf("wrong minBottomSideBearing %d", h.MinBearing2)
	}
	// extents: 100+900, 150+700, 50+800
	if h.MaxExtent != 1000 {
		t.Errorf("wrong yMaxExtent %d", h.MaxExtent)
	}
}

func TestSaturation(t *testing.T) {
	info := &Info{
		Heights:      []funit.Uint16{100},
		TSB:          []funit.Int16{20000},
		GlyphExtents: []funit.Rect16{{LLy: -10000, URy: 10000}},
	}
	vhea, _ := info.Encode()
	h, err := lmetrics.ParseHeader(vhea)
	if err != nil {
		t.Fatal(err)
	}
	if h.MaxExtent != 32767 {
		t.Errorf("yMaxExtent: got %d, want 32767", h.MaxExtent)
	}
	if h.MinBearing2 != -32768 {
		t.Errorf("minBottomSideBearing: got %d, want -32768", h.MinBearing2)
	}
}

func TestEncodePanics(t *testing.T) {
	cases := []*Info{
		{Heights: []funit.Uint16{1, 2}, TSB: []funit.Int16{1}},
		{Heights: []funit.Uint16{1, 2}},
		{Heights: []funit.Uint16{1, 2}, TSB: []funit.Int16{1, 2},
			GlyphExtents: []funit.Rect16{{}}},
	}
	for i, info := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("case %d: expected panic", i)
				}
			}()
			info.Encode()
		}()
	}
}

func TestVersions(t *testing.T) {
	info := testInfo()
	vhea, vmtx := info.Encode()

	var decoded []*Info
	for _, version := range []uint32{0x00010000, 0x00011000} {
		data := append([]byte(nil), vhea...)
		binary.BigEndian.PutUint32(data, version)
		i, err := Decode(data, vmtx)
		if err != nil {
			t.Fatalf("version %08x: %v", version, err)
		}
		decoded = append(decoded, i)
	}
	if diff := cmp.Diff(decoded[0], decoded[1]); diff != "" {
		t.Errorf("versions differ (-v1.0 +v1.1):\n%s", diff)
	}
	if decoded[0].Ascent != 500 || decoded[0].Descent != -500 || decoded[0].LineGap != 20 {
		t.Errorf("wrong line metrics %+v", decoded[0])
	}

	for _, version := range []uint32{0, 0x00020000, 0x00010001} {
		data := append([]byte(nil), vhea...)
		binary.BigEndian.PutUint32(data, version)
		if _, err := Decode(data, vmtx); err == nil {
			t.Errorf("version %08x: expected error", version)
		}
	}
}

func TestCaretRepair(t *testing.T) {
	info := &Info{} // rise = run = 0
	vhea, _ := info.Encode()
	if h, _ := lmetrics.ParseHeader(vhea); h.CaretSlopeRise != 0 || h.CaretSlopeRun != 1 {
		t.Errorf("wrong encoded caret slope %d/%d", h.CaretSlopeRise, h.CaretSlopeRun)
	}
	i, err := Decode(vhea, nil)
	if err != nil {
		t.Fatal(err)
	}
	if i.CaretSlopeRise != 0 || i.CaretSlopeRun != 1 {
		t.Errorf("wrong caret slope %d/%d", i.CaretSlopeRise, i.CaretSlopeRun)
	}

	info = &Info{CaretSlopeRise: 1, CaretSlopeRun: 0}
	vhea, _ = info.Encode()
	i, err = Decode(vhea, nil)
	if err != nil {
		t.Fatal(err)
	}
	if i.CaretSlopeRise != 1 || i.CaretSlopeRun != 0 {
		t.Errorf("wrong caret slope %d/%d", i.CaretSlopeRise, i.CaretSlopeRun)
	}
}

func TestLengths(t *testing.T) {
	info := &Info{
		Heights: []funit.Uint16{100, 200, 300, 300, 300},
		TSB:     []funit.Int16{0, 10, 20, 30, 40},
	}
	vhea, vmtx := info.Encode()

	if len(vhea) != lmetrics.HeaderLength {
		t.Errorf("expected %d, got %d", lmetrics.HeaderLength, len(vhea))
	}
	numLong := 3
	want := 4*numLong + 2*(len(info.Heights)-numLong)
	if len(vmtx) != want {
		t.Errorf("expected %d, got %d", want, len(vmtx))
	}
}

func TestMissingData(t *testing.T) {
	info := testInfo()
	vhea, vmtx := info.Encode()

	// no vmtx table
	i, err := Decode(vhea, nil)
	if err != nil {
		t.Fatal(err)
	}
	if i.Heights != nil || i.TSB != nil {
		t.Error("unexpected metrics")
	}

	// short vhea table
	if _, err := Decode(vhea[:lmetrics.HeaderLength-1], vmtx); err == nil {
		t.Error("short vhea: expected error")
	}
	if _, err := Decode(nil, vmtx); err == nil {
		t.Error("missing vhea: expected error")
	}

	// truncated vmtx tables
	for n := range len(vmtx) {
		_, err := Decode(vhea, vmtx[:n])
		short := n%2 != 0 || n < 3*4 // an odd length or fewer than numLong records
		if short && err == nil {
			t.Errorf("vmtx[:%d]: expected error", n)
		}
	}

	// vmtx without any glyphs but with numLong > 0
	if _, err := Decode(vhea, []byte{}); err == nil {
		t.Error("empty vmtx: expected error")
	}

	// no metrics: header only
	vhea, vmtx = (&Info{}).Encode()
	if len(vhea) != lmetrics.HeaderLength || vmtx != nil {
		t.Errorf("wrong lengths %d, %d", len(vhea), len(vmtx))
	}
}

func FuzzVmtx(f *testing.F) {
	i1 := testInfo()
	vhea, vmtx := i1.Encode()
	f.Add(vhea, vmtx)

	f.Fuzz(func(t *testing.T, vhea, vmtx []byte) {
		i1, err := Decode(vhea, vmtx)
		if err != nil {
			return
		}

		vhea, vmtx = i1.Encode()
		i2, err := Decode(vhea, vmtx)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(i1, i2); diff != "" {
			t.Errorf("round trip failed (-first +second):\n%s", diff)
		}
	})
}
