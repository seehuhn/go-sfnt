// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
)

// TestValueRecordApply checks that Apply updates all five glyph-info
// fields it can touch, and never panics on a fully populated record.
func TestValueRecordApply(t *testing.T) {
	vr := &GposValueRecord{
		XPlacement:    1,
		YPlacement:    2,
		XAdvance:      3,
		YAdvance:      4,
		XPlacementDev: &device.Table{OuterIndex: 1, DeltaFormat: device.VariationIndexFormat},
		YPlacementDev: &device.Table{OuterIndex: 2, DeltaFormat: device.VariationIndexFormat},
		XAdvanceDev:   &device.Table{OuterIndex: 3, DeltaFormat: device.VariationIndexFormat},
		YAdvanceDev:   &device.Table{OuterIndex: 4, DeltaFormat: device.VariationIndexFormat},
	}
	g := &glyph.Info{XOffset: 100, YOffset: 200, Advance: 300, YAdvance: 400}
	vr.Apply(g)
	want := &glyph.Info{XOffset: 101, YOffset: 202, Advance: 303, YAdvance: 404}
	if diff := cmp.Diff(want, g); diff != "" {
		t.Errorf("apply mismatch (-want +got):\n%s", diff)
	}
}

// TestValueRecordRoundTripAllBits round-trips a Gpos1_1 carrying every
// supported valueFormat bit, including all four Device pointers.
func TestValueRecordRoundTripAllBits(t *testing.T) {
	l1 := &Gpos1_1{
		Cov: coverage.Table{5: 0},
		Adjust: &GposValueRecord{
			XPlacement: 10,
			YPlacement: 20,
			XAdvance:   30,
			YAdvance:   40,
			XPlacementDev: &device.Table{
				StartSize: 8, EndSize: 10,
				Deltas: []int8{1, -1, 2}, DeltaFormat: 2,
			},
			YPlacementDev: &device.Table{
				StartSize: 6, EndSize: 7,
				Deltas: []int8{0, -1}, DeltaFormat: 1,
			},
			XAdvanceDev: &device.Table{
				OuterIndex:  3,
				InnerIndex:  7,
				DeltaFormat: device.VariationIndexFormat,
			},
			YAdvanceDev: &device.Table{
				StartSize: 12, EndSize: 14,
				Deltas: []int8{5, -5, 0}, DeltaFormat: 3,
			},
		},
	}

	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Fatalf("encode length mismatch: encode=%d encodeLen=%d", len(data), l1.encodeLen())
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	got, err := readGpos1_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(l1, got); diff != "" {
		t.Errorf("round-trip mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatBuilder(t *testing.T) {
	cases := []struct {
		name    string
		records []*GposValueRecord
		want    uint16
	}{
		{"empty", nil, 0},
		{"nil", []*GposValueRecord{nil, nil}, 0},
		{"zero", []*GposValueRecord{{}, nil}, 0x0004},
		{"zero and YAdvance", []*GposValueRecord{{}, {YAdvance: 5}}, 0x0008},
		{"nil and YAdvance", []*GposValueRecord{nil, {YAdvance: 5}}, 0x0008},
		{"zero and device", []*GposValueRecord{{}, {YAdvanceDev: &device.Table{}}}, 0x0080},
		{"union", []*GposValueRecord{{XPlacement: 1}, {YAdvance: -1}}, 0x0009},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b formatBuilder
			for _, vr := range c.records {
				b.add(vr)
			}
			if got := b.format(); got != c.want {
				t.Errorf("got format 0x%04x, want 0x%04x", got, c.want)
			}
		})
	}
}

// TestSharedFormatZeroRecords checks that zero records do not add fields to
// the format shared with records which use other fields, that the subtables
// still round-trip, and that nil records sharing a non-zero format read back
// as zero records.  With PositionSecond set, the second records are present
// even if all of them are nil.
func TestSharedFormatZeroRecords(t *testing.T) {
	yAdv := &GposValueRecord{YAdvance: 5}
	zero := &GposValueRecord{}

	cases := []struct {
		name    string
		sub     Subtable
		want    Subtable // expected read result, if different from sub
		read    func(*parser.Parser, int64) (Subtable, error)
		formats []uint16
	}{
		{
			name: "Gpos1_2",
			sub: &Gpos1_2{
				Cov:    coverage.Table{5: 0, 6: 1},
				Adjust: []*GposValueRecord{zero, yAdv},
			},
			read:    readGpos1_2,
			formats: []uint16{0x0008},
		},
		{
			name: "Gpos1_2 nil",
			sub: &Gpos1_2{
				Cov:    coverage.Table{5: 0, 6: 1},
				Adjust: []*GposValueRecord{nil, yAdv},
			},
			want: &Gpos1_2{
				Cov:    coverage.Table{5: 0, 6: 1},
				Adjust: []*GposValueRecord{zero, yAdv},
			},
			read:    readGpos1_2,
			formats: []uint16{0x0008},
		},
		{
			name: "Gpos2_1 nil",
			sub: &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{
				{Left: 1, Right: 2}: {First: nil, Second: nil},
				{Left: 1, Right: 3}: {First: yAdv, Second: nil},
			}},
			want: &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{
				{Left: 1, Right: 2}: {First: zero, Second: nil},
				{Left: 1, Right: 3}: {First: yAdv, Second: nil},
			}},
			read:    readGpos2_1,
			formats: []uint16{0x0008, 0x0000},
		},
		{
			name: "Gpos2_1",
			sub: &Gpos2_1{
				Adjust: map[glyph.Pair]*PairAdjust{
					{Left: 1, Right: 2}: {First: zero, Second: zero},
					{Left: 1, Right: 3}: {First: yAdv, Second: zero},
				},
				PositionSecond: true,
			},
			read:    readGpos2_1,
			formats: []uint16{0x0008, 0x0004},
		},
		{
			name: "Gpos2_1 nil second",
			sub: &Gpos2_1{
				Adjust: map[glyph.Pair]*PairAdjust{
					{Left: 1, Right: 2}: {First: yAdv, Second: nil},
				},
				PositionSecond: true,
			},
			want: &Gpos2_1{
				Adjust: map[glyph.Pair]*PairAdjust{
					{Left: 1, Right: 2}: {First: yAdv, Second: zero},
				},
				PositionSecond: true,
			},
			read:    readGpos2_1,
			formats: []uint16{0x0008, 0x0004},
		},
		{
			name: "Gpos2_2",
			sub: &Gpos2_2{
				Cov:    coverage.Set{1: true},
				Class1: classdef.Table{},
				Class2: classdef.Table{3: 1, 4: 2},
				Adjust: [][]*PairAdjust{{
					{First: zero, Second: zero},
					{First: yAdv, Second: zero},
					{First: zero, Second: zero},
				}},
				PositionSecond: true,
			},
			read:    readGpos2_2,
			formats: []uint16{0x0008, 0x0004},
		},
		{
			name: "Gpos2_2 nil second",
			sub: &Gpos2_2{
				Cov:    coverage.Set{1: true},
				Class1: classdef.Table{},
				Class2: classdef.Table{3: 1},
				Adjust: [][]*PairAdjust{{
					{First: nil, Second: nil},
					{First: yAdv, Second: nil},
				}},
				PositionSecond: true,
			},
			want: &Gpos2_2{
				Cov:    coverage.Set{1: true},
				Class1: classdef.Table{},
				Class2: classdef.Table{3: 1},
				Adjust: [][]*PairAdjust{{
					{First: zero, Second: zero},
					{First: yAdv, Second: zero},
				}},
				PositionSecond: true,
			},
			read:    readGpos2_2,
			formats: []uint16{0x0008, 0x0004},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := c.sub.encode()
			if len(data) != c.sub.encodeLen() {
				t.Fatalf("encode/encodeLen mismatch: %d vs %d", len(data), c.sub.encodeLen())
			}
			for i, want := range c.formats {
				got := uint16(data[4+2*i])<<8 | uint16(data[5+2*i])
				if got != want {
					t.Errorf("valueFormat%d: got 0x%04x, want 0x%04x", i+1, got, want)
				}
			}

			p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			if err := p.Discard(2); err != nil {
				t.Fatal(err)
			}
			got, err := c.read(p, 0)
			if err != nil {
				t.Fatal(err)
			}
			want := c.want
			if want == nil {
				want = c.sub
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("round trip failed (-want +got):\n%s", diff)
			}
		})
	}
}

// TestReservedValueFormatBits checks that a valueFormat consisting only of
// reserved bits selects no fields, so that the records read as absent.
func TestReservedValueFormatBits(t *testing.T) {
	if got := decodeValueFormat([]byte{0x30, 0x04}); got != 0x0004 {
		t.Errorf("got format 0x%04x, want 0x0004", got)
	}

	// Gpos1_2 with valueFormat 0x3000 and two value records
	data := []byte{
		0, 2, // posFormat
		0, 10, // coverageOffset
		0x30, 0x00, // valueFormat (reserved bits only)
		0, 2, // valueCount
		0, 0, // padding
		0, 1, 0, 2, 0, 5, 0, 6, // coverage format 1: glyphs 5, 6
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	got, err := readGpos1_2(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := &Gpos1_2{
		Cov:    coverage.Table{5: 0, 6: 1},
		Adjust: []*GposValueRecord{nil, nil},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("read failed (-want +got):\n%s", diff)
	}
}
