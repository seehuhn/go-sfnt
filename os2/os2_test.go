// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package os2

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func FuzzOS2(f *testing.F) {
	f.Fuzz(func(t *testing.T, in []byte) {
		i1, err := Read(bytes.NewReader(in))
		if err != nil {
			return
		}

		buf := i1.Encode()
		i2, err := Read(bytes.NewReader(buf))
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(i1, i2); diff != "" {
			t.Errorf("round trip failed (-want +got):\n%s", diff)
		}
	})
}

// TestWinMetricsUnsigned checks that usWinAscent and usWinDescent values
// beyond the int16 range are read and written correctly.
func TestWinMetricsUnsigned(t *testing.T) {
	info := &Info{WinAscent: 40000, WinDescent: 65535}
	got, err := Read(bytes.NewReader(info.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	if got.WinAscent != 40000 || got.WinDescent != 65535 {
		t.Errorf("got WinAscent %d, WinDescent %d; want 40000, 65535",
			got.WinAscent, got.WinDescent)
	}
}
