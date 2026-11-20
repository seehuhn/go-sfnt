// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func FuzzMath(f *testing.F) {
	if enc, err := sampleInfo().Encode(); err == nil {
		f.Add(enc)
	}
	if enc, err := (&Info{}).Encode(); err == nil {
		f.Add(enc)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		info1, err := readBytes(data)
		if err != nil {
			return
		}

		encoded, err := info1.Encode()
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}
		info2, err := readBytes(encoded)
		if err != nil {
			t.Fatalf("re-read failed: %v", err)
		}
		if diff := cmp.Diff(info1, info2); diff != "" {
			t.Errorf("round trip failed (-first +second):\n%s", diff)
		}
	})
}
