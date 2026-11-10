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

	"github.com/google/go-cmp/cmp"
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

func TestFeatureListOffsetRange(t *testing.T) {
	lookups := func(n, first int) []LookupIndex {
		res := make([]LookupIndex, n)
		for i := range res {
			res[i] = LookupIndex(first + i)
		}
		return res
	}
	chars := make([]rune, 30000)
	for i := range chars {
		chars[i] = rune('A' + i%26)
	}

	// In each case, the second feature only fits if the large one is
	// stored last.
	fitting := map[string]FeatureListInfo{
		"lookups": {
			{Tag: "liga", Lookups: lookups(40000, 0)},
			{Tag: "kern"},
		},
		"params": {
			{Tag: "cv01", Params: &FeatureParamsCharacterVariants{Characters: chars}},
			{Tag: "kern"},
		},
		"within limit": {
			{Tag: "liga", Lookups: lookups(30000, 0)},
			{Tag: "kern", Lookups: []LookupIndex{1}},
		},
	}
	for name, info := range fitting {
		t.Run(name, func(t *testing.T) {
			data := info.encode()
			p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			got, err := readFeatureList(p, 0)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(info, got); diff != "" {
				t.Errorf("round trip failed (-want +got):\n%s", diff)
			}
		})
	}

	tooLarge := map[string]FeatureListInfo{
		"three large features": {
			{Tag: "liga", Lookups: lookups(30000, 0)},
			{Tag: "dlig", Lookups: lookups(30000, 1)},
			{Tag: "hlig", Lookups: lookups(30000, 2)},
		},
		"params offset": {
			{Tag: "cv01", Lookups: lookups(40000, 0), Params: &FeatureParamsCharacterVariants{}},
		},
	}
	for name, info := range tooLarge {
		t.Run(name, func(t *testing.T) {
			assertPanics(t, func() { info.encode() })
		})
	}
}

// TestFeatureListSharedTables checks that a feature list in which many
// records reach the same Feature table, including its parameters, can be
// written back.  Without sharing, the output would need more than 64 KiB.
func TestFeatureListSharedTables(t *testing.T) {
	const nFeatures = 200
	const nChars = 200

	featurePos := 2 + 6*nFeatures
	data := []byte{0, nFeatures}
	for range nFeatures {
		data = append(data, "cv01"...)
		data = append(data, byte(featurePos>>8), byte(featurePos))
	}
	// one lookup, parameters directly behind the lookup indices
	data = append(data, 0, 6, 0, 1, 0, 7,
		0, 0, 1, 2, 0, 3, 0, 4, 0, 1, 1, 5, byte(nChars>>8), byte(nChars))
	for i := range nChars {
		data = append(data, 0, 0, byte('A'+i%26))
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != nFeatures || info[nFeatures-1].Params == nil {
		t.Fatalf("unexpected feature list %v", info)
	}

	l, err := info.layout()
	if err != nil {
		t.Fatal(err)
	}
	enc := info.encode()
	if len(enc) != l.encodeLen() {
		t.Errorf("encode/encodeLen mismatch: %d vs %d", len(enc), l.encodeLen())
	}
	if len(enc) > len(data) {
		t.Errorf("re-encoded list has %d bytes, input has %d", len(enc), len(data))
	}
	p = parser.New(bytes.NewReader(enc), parser.NewBudget(int64(len(enc))))
	info2, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestFeatureListSharingIdenticalOnly checks that only Feature tables with
// identical encodings, parameters included, share storage.
func TestFeatureListSharingIdenticalOnly(t *testing.T) {
	size := func(info FeatureListInfo) int {
		l, err := info.layout()
		if err != nil {
			t.Fatal(err)
		}
		return l.encodeLen()
	}
	ss := func(id uint16) FeatureParams { return &FeatureParamsStylisticSet{UINameID: id} }

	cases := []struct {
		name string
		info FeatureListInfo
		want int
	}{
		{"identical", FeatureListInfo{
			{Tag: "ss01", Params: ss(1)}, {Tag: "ss01", Params: ss(1)}}, 2 + 12 + 8},
		{"different params", FeatureListInfo{
			{Tag: "ss01", Params: ss(1)}, {Tag: "ss01", Params: ss(2)}}, 2 + 12 + 16},
		{"different lookups", FeatureListInfo{
			{Tag: "liga", Lookups: []LookupIndex{1}}, {Tag: "liga", Lookups: []LookupIndex{2}}}, 2 + 12 + 12},
		{"different tags", FeatureListInfo{
			{Tag: "liga", Lookups: []LookupIndex{1}}, {Tag: "clig", Lookups: []LookupIndex{1}}}, 2 + 12 + 6},
	}
	for _, c := range cases {
		if got := size(c.info); got != c.want {
			t.Errorf("%s: got %d bytes, want %d", c.name, got, c.want)
		}
	}
}

// TestFeatureListRepair checks that a feature list whose Feature tables
// cannot all be written back is repaired on read: the features keep their
// tags, and as many as possible keep their lookups.  The Feature tables
// overlap in the input, and each holds about 2000 lookups.
func TestFeatureListRepair(t *testing.T) {
	const nFeatures = 100
	const base = 2000

	data := []byte{0, nFeatures}
	for i := range nFeatures {
		off := 2 + 6*nFeatures + 2*i
		data = append(data, 'l', 'i', 'g', 'a', byte(off>>8), byte(off))
	}
	for j := range 2*nFeatures + base + 10 {
		w := base + j
		data = append(data, byte(w>>8), byte(w))
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != nFeatures {
		t.Fatalf("got %d features, want %d", len(info), nFeatures)
	}
	kept := 0
	for _, f := range info {
		if f.Tag != "liga" {
			t.Errorf("unexpected tag %q", f.Tag)
		}
		if len(f.Lookups) > 0 {
			kept++
		}
	}
	// each Feature table takes about 4 kB, so about 15 fit
	if kept < 10 || kept >= nFeatures {
		t.Errorf("%d features kept their lookups", kept)
	}

	data2 := info.encode()
	p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
	info2, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestFeatureListRepairParams checks that feature parameters which cannot be
// reached from their Feature table are dropped on read, while the lookups
// are kept.
func TestFeatureListRepairParams(t *testing.T) {
	n := 40000
	data := []byte{0, 1, 'c', 'v', '0', '1', 0, 8}
	// the params offset points into the lookup list
	data = append(data, 0, 4, byte(n>>8), byte(n))
	data = append(data, make([]byte, 2*n)...)

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != 1 || len(info[0].Lookups) != n || info[0].Params != nil {
		t.Fatalf("unexpected result %v", info)
	}
	info.encode()
}

// TestFeatureListSharedFeatures checks that many records reaching one Feature
// table, including one with parameters, are read within the default budget
// and can be written back.
func TestFeatureListSharedFeatures(t *testing.T) {
	nRecords := 3000
	nLookups := 1000

	hdr := 2 + 6*nRecords
	data := []byte{byte(nRecords >> 8), byte(nRecords)}
	for i := range nRecords {
		tag := "liga"
		if i%2 == 1 {
			tag = "size"
		}
		data = append(data, tag...)
		data = append(data, byte(hdr>>8), byte(hdr))
	}
	// one Feature table with parameters directly after the lookup indices
	paramsOffs := 4 + 2*nLookups
	data = append(data, byte(paramsOffs>>8), byte(paramsOffs), byte(nLookups>>8), byte(nLookups))
	for i := range nLookups {
		data = append(data, byte(i>>8), byte(i))
	}
	data = append(data, 0, 100, 0, 1, 1, 0, 0, 80, 0, 120)

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != nRecords {
		t.Fatalf("got %d features, want %d", len(info), nRecords)
	}
	if info[1].Params == nil || info[0].Params != nil {
		t.Error("wrong feature parameters")
	}
	if &info[0].Lookups[0] != &info[2].Lookups[0] {
		t.Error("lookups not shared")
	}

	enc := info.encode()
	p = parser.New(bytes.NewReader(enc), parser.NewBudget(int64(len(enc))))
	info2, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}
