// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package maxp

import (
	"bytes"
	"testing"
)

func TestMaxp(t *testing.T) {
	for _, numGlyphs := range []int{1, 2, 3, 255, 256, 1000, 65535} {
		info := &Info{NumGlyphs: numGlyphs}
		maxp := info.Encode()
		maxpInfo, err := Read(bytes.NewReader(maxp))
		if err != nil {
			t.Errorf("ReadMaxp(%d): %v", numGlyphs, err)
			continue
		}
		gotNumGlyphs := maxpInfo.NumGlyphs
		if gotNumGlyphs != numGlyphs {
			t.Errorf("ReadMaxp(%d): got %d glyphs, want %d", numGlyphs, gotNumGlyphs, numGlyphs)
		}
	}
}

func FuzzMaxp(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0x50, 0x00, 0x12, 0x34})
	f.Fuzz(func(t *testing.T, data []byte) {
		maxpInfo, err := Read(bytes.NewReader(data))
		if err != nil {
			return
		}
		data2 := maxpInfo.Encode()
		maxpInfo2, err := Read(bytes.NewReader(data2))
		if err != nil {
			t.Fatal(err)
		}
		if maxpInfo2.NumGlyphs != maxpInfo.NumGlyphs {
			t.Errorf("numGlyphs: %d != %d", maxpInfo2.NumGlyphs, maxpInfo.NumGlyphs)
		}
	})
}
