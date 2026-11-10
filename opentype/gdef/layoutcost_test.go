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

package gdef

import (
	"runtime"
	"testing"

	"seehuhn.de/go/sfnt/opentype/device"
)

// allocated returns the number of bytes allocated while running f.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestAttachListLayoutCost checks that rows which share one Go slice are
// encoded once.
func TestAttachListLayoutCost(t *testing.T) {
	const n = 3000
	const m = 20000
	shared := make([]uint16, m)
	al := &AttachList{Cov: sequentialCoverage(n), Points: make([][]uint16, n)}
	for i := range al.Points {
		al.Points[i] = shared
	}

	var err error
	used := allocated(func() { _, err = al.layout() })
	if err != nil {
		t.Fatal(err)
	}
	if limit := uint64(16 * (4 + 2*n + 2 + 2*m)); used > limit {
		t.Errorf("layout allocated %d bytes, limit %d", used, limit)
	}
}

// TestLigCaretListLayoutCost checks that rows which share one Go slice are
// encoded once.
func TestLigCaretListLayoutCost(t *testing.T) {
	const n = 3000
	const m = 15000
	shared := make([]CaretValue, m)
	lcl := &LigCaretList{Cov: sequentialCoverage(n), Carets: make([][]CaretValue, n)}
	for i := range lcl.Carets {
		lcl.Carets[i] = shared
	}

	var err error
	used := allocated(func() { _, err = lcl.layout() })
	if err != nil {
		t.Fatal(err)
	}
	if limit := uint64(16 * (4 + 2*n + 2 + 2*m + 4)); used > limit {
		t.Errorf("layout allocated %d bytes, limit %d", used, limit)
	}
}

// TestLigCaretListSharedCaretCost checks that many distinct LigGlyph rows,
// as read from different positions in a font, which refer to one caret value
// with a large Device table, are not encoded once per row.
func TestLigCaretListSharedCaretCost(t *testing.T) {
	const n = 5000
	const sizes = 28000
	dev := &device.Table{StartSize: 1, EndSize: sizes, Deltas: make([]int8, sizes), DeltaFormat: 3}
	lcl := &LigCaretList{Cov: sequentialCoverage(n), Carets: make([][]CaretValue, n)}
	for i := range lcl.Carets {
		lcl.Carets[i] = []CaretValue{{Device: dev}}
	}

	var err error
	used := allocated(func() { _, err = lcl.layout() })
	if err != nil {
		t.Fatal(err)
	}
	// LigGlyph tables of 4 bytes, the caret value and the Device table; the
	// rows need a few map entries each, while encoding the Device table once
	// per row would allocate about 2500 times the input size
	inputSize := 4 + 2*n + 6 + 4*n + 6 + dev.EncodeLen()
	if limit := uint64(64 * inputSize); used > limit {
		t.Errorf("layout allocated %d bytes, limit %d", used, limit)
	}
}
