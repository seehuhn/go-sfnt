// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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

package gtab

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"seehuhn.de/go/sfnt/parser"
)

func TestFeatureParamsRoundTrip(t *testing.T) {
	info := FeatureListInfo{
		{Tag: "size", Params: &FeatureParamsSize{
			DesignSize: 100, SubfamilyID: 1, SubfamilyNameID: 256, RangeStart: 80, RangeEnd: 120}},
		{Tag: "ss01", Lookups: []LookupIndex{5, 6}, Params: &FeatureParamsStylisticSet{UINameID: 257}},
		{Tag: "cv01", Params: &FeatureParamsCharacterVariants{
			FeatUILabelNameID: 258, NumNamedParameters: 1, FirstParamUILabelNameID: 259,
			Characters: []rune{0x41, 0x42, 0x1F600}}},
		{Tag: "kern", Lookups: []LookupIndex{0}},
	}
	data := info.encode()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	got, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(info, got) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, info)
	}
}

func TestFeatureParamsOutOfRange(t *testing.T) {
	// a 'size' feature whose featureParamsOffset points past the end of the
	// table must not fail the whole feature list; the params are dropped
	data := []byte{
		0, 1, // featureCount
		's', 'i', 'z', 'e', 0, 8, // FeatureRecord: tag + featureOffset
		0xFF, 0xFF, // featureParamsOffset (out of range)
		0, 0, // lookupIndexCount
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	got, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tag != "size" {
		t.Fatalf("expected a single 'size' feature, got %+v", got)
	}
	if got[0].Params != nil {
		t.Errorf("expected nil params for out-of-range offset, got %+v", got[0].Params)
	}
}

func TestFeatureParamsTruncated(t *testing.T) {
	// a params table whose offset is in range but whose body runs past the
	// end of the data must drop only the params, not the whole feature list
	info := FeatureListInfo{
		{Tag: "size", Params: &FeatureParamsSize{
			DesignSize: 100, SubfamilyID: 1, SubfamilyNameID: 256, RangeStart: 80, RangeEnd: 120}},
	}
	data := info.encode()
	data = data[:len(data)-1] // truncate the trailing params table by a byte

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	got, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Tag != "size" {
		t.Fatalf("expected a single 'size' feature, got %+v", got)
	}
	if got[0].Params != nil {
		t.Errorf("expected nil params for truncated table, got %+v", got[0].Params)
	}
}

func FuzzFeatureList(f *testing.F) {
	info := FeatureListInfo{}
	info = append(info, &Feature{Tag: "test"})
	f.Add(info.encode())
	info = append(info, &Feature{Tag: "kern", Lookups: []LookupIndex{0, 1, 2, 3}})
	f.Add(info.encode())
	info = append(info,
		&Feature{Tag: "size", Params: &FeatureParamsSize{DesignSize: 100, RangeStart: 80, RangeEnd: 120}},
		&Feature{Tag: "ss01", Lookups: []LookupIndex{5}, Params: &FeatureParamsStylisticSet{UINameID: 257}},
		&Feature{Tag: "cv01", Params: &FeatureParamsCharacterVariants{
			FeatUILabelNameID: 258, Characters: []rune{0x41, 0x1F600}}},
	)
	f.Add(info.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		info, err := readFeatureList(p, 0)
		if err != nil {
			return
		}

		data2 := info.encode()

		// if len(data2) > len(data) {
		// 	fmt.Printf("A % x\n", data)
		// 	fmt.Printf("B % x\n", data2)
		// 	t.Errorf("encode: %d > %d", len(data2), len(data))
		// }

		p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
		info2, err := readFeatureList(p, 0)
		if err != nil {
			fmt.Printf("A % x\n", data)
			fmt.Printf("B % x\n", data2)
			fmt.Println(info)
			t.Fatal(err)
		}

		if !reflect.DeepEqual(info, info2) {
			// fmt.Printf("A % x\n", data)
			// fmt.Printf("B % x\n", data2)

			if len(info) != len(info2) {
				t.Fatal("different lengths")
			}
			fmt.Println("length:", len(info))
			for i, f1 := range info {
				f2 := info2[i]

				if f1.Tag != f2.Tag {
					t.Fatalf("info[%d].tag: %q != %q", i, f1.Tag, f2.Tag)
				}
			}
			t.Error("different")
		}
	})
}
