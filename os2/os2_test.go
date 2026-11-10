// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package os2

import (
	"bytes"
	"fmt"
	"reflect"
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

		if !reflect.DeepEqual(i1, i2) {
			d := cmp.Diff(i1, i2)
			fmt.Println(d)
			t.Fatal("not equal")
		}
	})
}
