// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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

package type1

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMoveTo(t *testing.T) {
	baseX := 2.0
	baseY := 3.0
	type testCase struct {
		dx, dy float64
	}
	cases := []testCase{
		{0, 0},
		{0, 11},
		{13, 0},
		{17, 19},
	}
	for _, c := range cases {
		g1 := &Glyph{}
		g1.MoveTo(baseX, baseY)
		g1.MoveTo(baseX+c.dx, baseY+c.dy)
		buf := g1.encodeCharString()

		ctx := &decodeInfo{}
		g2, err := ctx.decodeCharString(buf, "test")
		if err != nil {
			t.Fatal(err)
		}

		if d := cmp.Diff(g1, g2); d != "" {
			t.Errorf("unexpected diff: %s", d)
		}
	}
}

func TestLineTo(t *testing.T) {
	baseX := 2.0
	baseY := 3.0
	type testCase struct {
		dx, dy float64
	}
	cases := []testCase{
		{0, 0},
		{0, 11},
		{13, 0},
		{17, -19},
	}
	for _, c := range cases {
		g1 := &Glyph{}
		g1.MoveTo(baseX, baseY)
		g1.LineTo(baseX+c.dx, baseY+c.dy)
		buf := g1.encodeCharString()

		ctx := &decodeInfo{}
		g2, err := ctx.decodeCharString(buf, "test")
		if err != nil {
			t.Fatal(err)
		}

		if d := cmp.Diff(g1, g2); d != "" {
			t.Errorf("unexpected diff: %s", d)
		}
	}
}

func TestCurveTo(t *testing.T) {
	baseX := 2.0
	baseY := 3.0
	type testCase struct {
		a, b, c, d, e, f float64
	}
	cases := []testCase{
		{0, 0, 0, 0, 0, 0},
		{11, 0, 13, 17, 13, 19},
		{0, 20, 30, 60, 50, 60},
		{10, 20, 30, 40, 50, 60},
	}
	for _, c := range cases {
		g1 := &Glyph{}
		g1.MoveTo(baseX, baseY)
		g1.CurveTo(baseX+c.a, baseY+c.b, baseX+c.c, baseY+c.d, baseX+c.e, baseY+c.f)
		buf := g1.encodeCharString()

		ctx := &decodeInfo{}
		g2, err := ctx.decodeCharString(buf, "test")
		if err != nil {
			t.Fatal(err)
		}

		if d := cmp.Diff(g1, g2); d != "" {
			t.Errorf("unexpected diff: %s", d)
		}
	}
}

func TestAppendInt(t *testing.T) {
	for _, x := range []int32{0, 1, -1, 2, -2, 107, -107, 108, -108, 1131, -1131, 1132, -1132, math.MaxInt32, math.MinInt32} {
		var buf []byte
		buf = appendInt(buf, 0)
		buf = appendInt(buf, 0)
		buf = appendOp(buf, t1hsbw)
		buf = appendInt(buf, x)
		buf = appendOp(buf, t1hmoveto)
		buf = appendOp(buf, t1endchar)
		ctx := &decodeInfo{}
		g, err := ctx.decodeCharString(buf, "test")
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Cmds) != 1 || g.Cmds[0].Op != OpMoveTo || len(g.Cmds[0].Args) != 2 {
			t.Fatalf("test is broken")
		}
		if int32(g.Cmds[0].Args[0]) != x {
			t.Errorf("x=%g, want %d", g.Cmds[0].Args[0], x)
		}
	}
}

func TestAppendNumber(t *testing.T) {
	for _, x := range []float64{0, 1, -1, 2, -2, 0.1, 0.5, 2.5, 1.0 / 3, math.Pi} {
		var buf []byte
		buf = appendInt(buf, 0)
		buf = appendInt(buf, 0)
		buf = appendOp(buf, t1hsbw)
		buf, xEnc := appendNumber(buf, x)
		buf = appendOp(buf, t1hmoveto)
		buf = appendOp(buf, t1endchar)

		delta := math.Abs(x - xEnc)
		if delta > 1e-4 {
			t.Errorf("x=%g, want %g (delta=%g)", xEnc, x, delta)
		}

		ctx := &decodeInfo{}
		g, err := ctx.decodeCharString(buf, "test")
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Cmds) != 1 || g.Cmds[0].Op != OpMoveTo || len(g.Cmds[0].Args) != 2 {
			t.Fatalf("test is broken")
		}
		if g.Cmds[0].Args[0] != xEnc {
			t.Errorf("x=%g, want %g", g.Cmds[0].Args[0], x)
		}
	}
}
