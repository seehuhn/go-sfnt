// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package builder

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/gtab"
)

func TestGsubParser(t *testing.T) {
	fontInfo := debug.MakeSimpleFont()
	lookups, err := Parse(fontInfo, `
	GSUB1: A->B, M->N
	GSUB1: A-C -> B-D, M->N, N->O
	GSUB1: A->X, B->X, C->X, M->X, N->X
	GSUB2: A -> "AA", B -> "AA", C -> "ABAAC"
	GSUB3: A -> [ "BCD" ]
	GSUB4: -marks A A A -> B, A -> D, A A -> C
	GSUB5:
		"AAA" -> 1@0 2@1 1@0, "AAB" -> 1@0 1@1 2@0 ||
		class :alpha: = [A-K]
		class :digits: = [L-Z]
		/A B C/ :alpha: :digits: -> 2@1, :alpha: :: :digits: -> 2@2 ||
		[A B C] [A C] [A D] -> 3@0
	GSUB6:
		A B | C D | E F -> 1@0 2@1, B | C D E | F -> 1@2 ||
		inputclass :ABC: = ["ABC"]
		backtrackclass :DEF: = ["DEF"]
		lookaheadclass :DEF: = ["DEF"]
		/A B C/ :DEF: :: | :ABC: | :: :DEF: -> 1@0 ||
		[A] [A B C] | [A B] [A C] [B C] | [A B C] [A B C] -> 1@0 1@1 1@2
	`)
	if err != nil {
		t.Fatal(err)
	}
	fontInfo.Gsub = &gtab.Info{LookupList: lookups}
	ExplainGsub(fontInfo)
}

func FuzzGsub1(f *testing.F) {
	f.Add("A->B, M->N")
	f.Add("A-C -> B-D, M->N, N->O")
	f.Add("A->X, B->X, C->X, M->X, N->X")
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GSUB1: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gsub = &gtab.Info{LookupList: lookups}
		desc2 := ExplainGsub(fontInfo)
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}

func FuzzGsub2(f *testing.F) {
	f.Add("A->B, M->N")
	f.Add(`A -> "AA", B -> "AA", C -> "ABAAC"`)
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GSUB2: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gsub = &gtab.Info{LookupList: lookups}
		desc2 := ExplainGsub(fontInfo)
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}

func FuzzGsub3(f *testing.F) {
	f.Add("A->[B], M->[N]")
	f.Add(`A -> ["AA"], B -> ["AA"], C -> ["ABAAC"]`)
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GSUB3: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gsub = &gtab.Info{LookupList: lookups}
		desc2 := ExplainGsub(fontInfo)
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}

func FuzzGsub5(f *testing.F) {
	f.Add(`"AAA" -> 1@0 2@1 1@0, "AAB" -> 1@0 1@1 2@0 `)
	f.Add(`
		class :alpha: = [A-K]
		class :digits: = [L-Z]
    	/A B C/ :alpha: :digits: -> 2@1, :alpha: :: :digits: -> 2@2`)
	f.Add(`[A B C] [A C] [A D] -> 3@0`)
	f.Add(`"AAA" -> 1@0 2@1 1@0, "AAB" -> 1@0 1@1 2@0 ||
		class :alpha: = [A-K]
		class :digits: = [L-Z]
		/A B C/ :alpha: :digits: -> 2@1, :alpha: :: :digits: -> 2@2 ||
		[A B C] [A C] [A D] -> 3@0`)
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GSUB5: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gsub = &gtab.Info{LookupList: lookups}
		desc2 := ExplainGsub(fontInfo)
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}

func FuzzGsub6(f *testing.F) {
	f.Add(`A B | C D | E F -> 1@0 2@1, B | C D E | F -> 1@2`)
	f.Add(`
		backtrackclass :DEF: = ["DEF"]
		inputclass :ABC: = ["ABC"]
		lookaheadclass :GHI: = ["GHI"]
		/A B C/ :DEF: :: | :ABC: | :: :GHI: -> 1@0`)
	f.Add(`[A] [A B C] | [A B] [A C] [B C] | [A B C] [A B C] -> 1@0 1@1 1@2`)
	f.Add(`
		A B | C D | E F -> 1@0 2@1, B | C D E | F -> 1@2 ||
		inputclass :ABC: = ["ABC"]
		backtrackclass :DEF: = ["DEF"]
		lookaheadclass :DEF: = ["DEF"]
		/A B C/ :DEF: :: | :ABC: | :: :DEF: -> 1@0 ||
		[A] [A B C] | [A B] [A C] [B C] | [A B C] [A B C] -> 1@0 1@1 1@2`)
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GSUB6: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gsub = &gtab.Info{LookupList: lookups}
		desc2 := ExplainGsub(fontInfo)
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}

func TestGposParser(t *testing.T) {
	fontInfo := debug.MakeSimpleFont()
	lookups, err := Parse(fontInfo, `
	GPOS1: [A-C] -> y+10 ||
		D -> dx-1, E -> dx+1, F -> dx-1, G -> dx+1, H -> x+1, I -> y+1
	GPOS2: A V -> dx-100, O O -> dx+100, "AW" -> dx-100
	GPOS2: T E -> y+100 dx-50 & y-100
	GPOS2:
	    /A L V W/
	    first V W, A L;
		second E O, V W;
		_ & _, _ & _, _ & _,
		_ & _, dx-50 & y-10, dx+10 & _,
		_ & _, dx-10 & y+10, dx-30 & _
	GPOS3:
	  A: 1,1 to 2,2; B: 1,0 to 0,1; C: -1,-1 to 100,100 ||
	  M: 1,1 to 2,2; N: 1,1 to 2,2
	GPOS4:
	  mark M: 0@100,100;
	  mark N: 1@200,100;
	  base A: @400,1000 @500,1000;
	  base B: @500,1000 @600,900;
	  base C: @500,1000 @500,-1000;
	`)
	if err != nil {
		t.Fatal(err)
	}
	fontInfo.Gpos = &gtab.Info{LookupList: lookups}

	explain := ExplainGpos(fontInfo)
	fmt.Println(explain)
}

func FuzzGpos1(f *testing.F) {
	f.Add("A-> dx+0")
	f.Add("[A-C] -> dy+10")
	f.Add("D -> dx-1, E -> dx+1, F -> dy-1, G -> dy+1, H -> x+1, I -> y+1")
	f.Add(`[A-C] -> dy+10 ||
	D -> dx-1, E -> dx+1, F -> dy-1, G -> dy+1, H -> x+1, I -> y+1`)
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GPOS1: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gpos = &gtab.Info{LookupList: lookups}
		explained := ExplainGpos(fontInfo)
		desc2 := strings.Join(explained, "\n")
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}

func FuzzGpos2(f *testing.F) {
	f.Add(`A V -> dx-100`)
	f.Add(`A V -> dx-100, O O -> dx+100, "AW" -> dx-100`)
	f.Add(`T E -> y+100 dx-50 & y-100`)
	f.Add(`A B -> x-1, A C -> x+1 || A D -> y-1`)
	f.Add(`/A L V W/
		first V W, A L;
		second E O, V W;
		_ & _, _ & _, _ & _,
		_ & _, dx-50 & y-10, dx+10 & _,
		_ & _, dx-10 & y+10, dx-30 & _`)
	f.Add(`A A -> y+0 & y+0`)
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GPOS2: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gpos = &gtab.Info{LookupList: lookups}
		explained := ExplainGpos(fontInfo)
		desc2 := strings.Join(explained, "\n")
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			fmt.Println(desc)
			fmt.Println(desc2)
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}

func TestGpos2PositionSecond(t *testing.T) {
	cases := []struct {
		desc    string
		want    bool
		wantErr bool
	}{
		{desc: `A V -> dx-100, O O -> dx+100`, want: false},
		{desc: `A V -> dx-100 & _, O O -> _ & x+1`, want: true},
		{desc: `A A -> y+0 & y+0`, want: true},
		{desc: `A V -> dx-100, O O -> dx+100 & x+1`, wantErr: true},
		{desc: `A V -> dx-100 & _ || O O -> dx+100`, want: true},
		{desc: "/A/ first A; second V; _, _, _, dx-1", want: false},
		{desc: "/A/ first A; second V; _ & _, _ & _, _ & _, dx-1 & _", want: true},
		{desc: "/A/ first A; second V; _, _, _, dx-1 & _", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			fontInfo := debug.MakeSimpleFont()
			lookups, err := Parse(fontInfo, "GPOS2: "+c.desc)
			if c.wantErr {
				if err == nil {
					t.Fatal("mixed pairs accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			var got bool
			switch l := lookups[0].Subtables[0].(type) {
			case *gtab.Gpos2_1:
				got = l.PositionSecond
			case *gtab.Gpos2_2:
				got = l.PositionSecond
			default:
				t.Fatalf("unexpected subtable type %T", l)
			}
			if got != c.want {
				t.Errorf("got PositionSecond=%t, want %t", got, c.want)
			}

			fontInfo.Gpos = &gtab.Info{LookupList: lookups}
			explained := strings.Join(ExplainGpos(fontInfo), "\n")
			lookups2, err := Parse(fontInfo, explained)
			if err != nil {
				t.Fatal(err)
			}
			if d := cmp.Diff(lookups, lookups2); d != "" {
				t.Errorf("explain round trip failed (-want +got):\n%s", d)
			}
		})
	}
}

func FuzzGpos3(f *testing.F) {
	f.Add(`A: 0,0 to 100,100`)
	f.Add(`A: 0,0 to 100,100; B: -10,10 to 0,0`)
	f.Add(`A: 0,0 to 100,100; B: -10,10 to 0,0 || C: 0,0 to 2,2`)
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GPOS3: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gpos = &gtab.Info{LookupList: lookups}
		explained := ExplainGpos(fontInfo)
		desc2 := strings.Join(explained, "\n")
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			fmt.Println(desc)
			fmt.Println(desc2)
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}

func FuzzGpos4(f *testing.F) {
	f.Add(`mark M: 0@100,100;
		mark N: 1@200,100;
		base A: @400,1000 @500,1000;
		base B: @500,1000 @600,900;
		base C: @500,1000 @500,-1000;`)
	f.Add(`mark M: 0@100,100;
		base A: @400,1000`)
	f.Fuzz(func(t *testing.T, desc string) {
		fontInfo := debug.MakeSimpleFont()
		lookups, err := Parse(fontInfo, "GPOS4: "+desc)
		if err != nil || len(lookups) != 1 {
			return
		}
		fontInfo.Gpos = &gtab.Info{LookupList: lookups}
		explained := ExplainGpos(fontInfo)
		desc2 := strings.Join(explained, "\n")
		lookups2, err := Parse(fontInfo, desc2)
		if err != nil {
			fmt.Println(desc)
			fmt.Println(desc2)
			t.Fatal(err)
		}
		if d := cmp.Diff(lookups, lookups2); d != "" {
			t.Error(d)
		}
	})
}
