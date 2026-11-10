// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package head

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestHeadLength(t *testing.T) {
	info := &Info{}
	data := info.Encode()
	if len(data) != headLength {
		t.Errorf("expected %d, got %d", headLength, len(data))
	}
}

func FuzzHead(f *testing.F) {
	info := &Info{}
	data := info.Encode()
	f.Add(data)

	f.Fuzz(func(t *testing.T, d1 []byte) {
		i1, err := Read(bytes.NewReader(d1))
		if err != nil {
			return
		}

		d2 := i1.Encode()

		i2, err := Read(bytes.NewReader(d2))
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(i1, i2) {
			fmt.Println(i1)
			fmt.Println(i2)
			t.Fatal("not equal")
		}
	})
}

func FuzzVersion(f *testing.F) {
	f.Add(uint32(0x00010000))
	f.Fuzz(func(t *testing.T, x uint32) {
		v1 := Version(x)
		s := v1.String()
		v2, err := VersionFromString(s)
		if err != nil {
			t.Fatal(err)
		}
		if v1.Round() != v2 {
			t.Errorf("%s != %s", v1, v2)
		}
	})
}
