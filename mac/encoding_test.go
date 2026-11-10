// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mac

import "testing"

func TestEncoding(t *testing.T) {
	for i := range 256 {
		s := Decode([]byte{byte(i)})
		cc := Encode(s)
		if len(cc) != 1 || cc[0] != byte(i) {
			t.Errorf("%d: %q -> %q", i, s, cc)
		}
	}
}
