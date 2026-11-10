// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cmap

import (
	"fmt"
	"reflect"
	"testing"
)

func FuzzCmapHeader(f *testing.F) {
	f.Add([]byte{
		0, 0,
		0, 2,
		0, 0, 0, 4, 0, 0, 0, 20,
		0, 3, 0, 10, 0, 0, 0, 20,
		0, 6, 0, 10, 0, 0, 0, 0,
	})
	ss := Table{
		{PlatformID: 3, EncodingID: 10}: []byte{0, 1, 0, 8, 1, 2, 3, 4, 101, 102, 103, 104},
		{PlatformID: 0, EncodingID: 4}:  []byte{0, 1, 0, 8, 5, 6, 7, 8, 101, 102, 103, 104},
	}
	f.Add(ss.Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		ss, err := Decode(data)
		if err != nil {
			return
		}
		data2 := ss.Encode()
		if len(data2) > len(data) {
			fmt.Printf("A % x\n", data)
			fmt.Printf("B % x\n", data2)
			t.Errorf("too long")
		}
		ss2, err := Decode(data2)
		if err != nil {
			for key, data := range ss {
				fmt.Printf("%d %d % x\n", key.PlatformID, key.EncodingID, data)
			}
			fmt.Printf("% x\n", data2)
			t.Fatal(err)
		}
		if !reflect.DeepEqual(ss, ss2) {
			fmt.Printf("A % x\n", data)
			fmt.Printf("B % x\n", data2)
			t.Errorf("ss != ss2")
		}
	})
}
