// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package post

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"seehuhn.de/go/sfnt/parser"
)

func FuzzPost(f *testing.F) {
	f.Add((&Info{
		ItalicAngle:        -9,
		UnderlinePosition:  -50,
		UnderlineThickness: 10,
	}).Encode())
	f.Add((&Info{
		ItalicAngle:        -9,
		UnderlinePosition:  -50,
		UnderlineThickness: 10,
		Names:              []string{".notdef", "sonderbar", "merkwuerdig", "dcroat"},
	}).Encode())

	f.Fuzz(func(t *testing.T, in []byte) {
		i1, err := Read(bytes.NewReader(in), parser.NewBudget(int64(len(in))))
		if err != nil {
			return
		}

		buf := i1.Encode()

		i2, err := Read(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(i1, i2) {
			fmt.Printf("%#v\n", i1)
			fmt.Printf("%#v\n", i2)
			t.Fatal("not equal")
		}
	})
}
