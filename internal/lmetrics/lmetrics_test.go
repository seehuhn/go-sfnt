// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package lmetrics

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/postscript/funit"
)

func TestHeader(t *testing.T) {
	h1 := &Header{Version: 0x00010000, Ascent: 5, Descent: -3, NumLong: 7}
	data := h1.Bytes()
	if len(data) != HeaderLength {
		t.Fatalf("wrong length %d", len(data))
	}
	h2, err := ParseHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(h1, h2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	if _, err := ParseHeader(data[:HeaderLength-1]); err == nil {
		t.Error("short header: expected error")
	}
}

func TestMetrics(t *testing.T) {
	adv := []funit.Uint16{10, 20, 20, 20, 65535}
	bear := []funit.Int16{-1, 2, -3, 4, 5}
	data, numLong := Encode(adv, bear)
	if numLong != 5 || len(data) != 4*5 {
		t.Fatalf("numLong=%d, len=%d", numLong, len(data))
	}

	adv = []funit.Uint16{10, 20, 20, 20}
	bear = bear[:4]
	data, numLong = Encode(adv, bear)
	if numLong != 2 || len(data) != 4*2+2*2 {
		t.Fatalf("numLong=%d, len=%d", numLong, len(data))
	}
	a2, b2, err := Decode(data, numLong)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(adv, a2); diff != "" {
		t.Errorf("advances differ (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(bear, b2); diff != "" {
		t.Errorf("bearings differ (-want +got):\n%s", diff)
	}

	if _, _, err := Decode(data[:len(data)-1], numLong); err == nil {
		t.Error("odd length: expected error")
	}
	if _, _, err := Decode(data[:6], numLong); err == nil {
		t.Error("too few long metrics: expected error")
	}
}
