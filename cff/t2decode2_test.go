// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
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

package cff

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
)

// encInt encodes v as a type 2 integer operand.
func encInt(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		v -= 108
		return []byte{byte(v/256 + 247), byte(v % 256)}
	case v >= -1131 && v <= -108:
		v = -v - 108
		return []byte{byte(v/256 + 251), byte(v % 256)}
	default:
		return []byte{0x1c, byte(v >> 8), byte(v)} // int16 via op 28
	}
}

// cs assembles a charstring from integer operands (int) and raw opcode bytes.
func cs(parts ...any) []byte {
	var out []byte
	for _, p := range parts {
		switch v := p.(type) {
		case int:
			out = append(out, encInt(v)...)
		case t2op:
			out = append(out, v.Bytes()...)
		case byte:
			out = append(out, v)
		case []byte:
			out = append(out, v...)
		default:
			panic("unsupported charstring part")
		}
	}
	return out
}

// fixedRegionCount returns a regionCount callback that reports k regions for
// vsindex values in [0, maxIdx] and errors otherwise.
func fixedRegionCount(k, maxIdx int) func(int) (int, error) {
	return func(vsindex int) (int, error) {
		if vsindex < 0 || vsindex > maxIdx {
			return 0, errors.New("bad vsindex")
		}
		return k, nil
	}
}

func newInfoCFF2(k int) *decodeInfoCFF2 {
	return &decodeInfoCFF2{
		regionCount: fixedRegionCount(k, 8),
		budget:      parser.NewBudget(1 << 20),
	}
}

func TestDecodeCFF2Basic(t *testing.T) {
	type testCase struct {
		name string
		k    int
		code []byte
		want *GlyphCFF2
	}

	b := func(d float64, deltas ...float64) Blend {
		if len(deltas) == 0 {
			return Blend{Default: d}
		}
		return Blend{Default: d, Deltas: deltas}
	}

	cases := []testCase{
		{
			name: "rmoveto+rlineto",
			code: cs(100, 200, t2rmoveto, 50, 0, t2rlineto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(100), b(200)}},
					{Op: OpLineTo, Args: []Blend{b(150), b(200)}},
				},
			},
		},
		{
			name: "hmoveto+vmoveto",
			code: cs(10, t2hmoveto, 20, t2vmoveto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(10), b(0)}},
					{Op: OpMoveTo, Args: []Blend{b(10), b(20)}},
				},
			},
		},
		{
			name: "hlineto alternating",
			code: cs(0, 0, t2rmoveto, 10, 20, 30, t2hlineto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpLineTo, Args: []Blend{b(10), b(0)}},
					{Op: OpLineTo, Args: []Blend{b(10), b(20)}},
					{Op: OpLineTo, Args: []Blend{b(40), b(20)}},
				},
			},
		},
		{
			name: "rrcurveto",
			code: cs(0, 0, t2rmoveto, 10, 10, 10, 10, 10, 10, t2rrcurveto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpCurveTo, Args: []Blend{b(10), b(10), b(20), b(20), b(30), b(30)}},
				},
			},
		},
		{
			name: "vvcurveto with leading dx1",
			code: cs(0, 0, t2rmoveto, 5, 10, 10, 10, 10, t2vvcurveto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpCurveTo, Args: []Blend{b(5), b(10), b(15), b(20), b(15), b(30)}},
				},
			},
		},
		{
			name: "hvcurveto",
			code: cs(0, 0, t2rmoveto, 10, 20, 30, 40, t2hvcurveto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpCurveTo, Args: []Blend{b(10), b(0), b(30), b(30), b(30), b(70)}},
				},
			},
		},
		{
			name: "flex",
			code: cs(0, 0, t2rmoveto,
				1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 50, t2flex),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpCurveTo, Args: []Blend{b(1), b(2), b(4), b(6), b(9), b(12)}},
					{Op: OpCurveTo, Args: []Blend{b(16), b(20), b(25), b(30), b(36), b(42)}},
				},
			},
		},
		{
			name: "rmoveto blended n=2 k=2",
			k:    2,
			// bases 100,200; deltas (operand-major) 10,20,30,40; n=2; blend
			code: cs(100, 200, 10, 20, 30, 40, 2, t2blend, t2rmoveto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(100, 10, 20), b(200, 30, 40)}},
				},
			},
		},
		{
			name: "vsindex before blend",
			k:    2,
			code: cs(1, t2vsindex, 0, 0, t2rmoveto, 5, 7, 8, 1, t2blend, 0, t2rlineto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpLineTo, Args: []Blend{b(5, 7, 8), b(0)}},
				},
				VSIndex: 1,
			},
		},
		{
			name: "vsindex after blend ignored",
			k:    2,
			code: cs(0, 0, t2rmoveto, 5, 7, 8, 1, t2blend, 0, t2rlineto, 5, t2vsindex),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpLineTo, Args: []Blend{b(5, 7, 8), b(0)}},
				},
				VSIndex: 0,
			},
		},
		{
			name: "second vsindex ignored",
			k:    2,
			code: cs(1, t2vsindex, 2, t2vsindex, 0, 0, t2rmoveto),
			want: &GlyphCFF2{
				Cmds:    []GlyphOpCFF2{{Op: OpMoveTo, Args: []Blend{b(0), b(0)}}},
				VSIndex: 1,
			},
		},
		{
			name: "blend mixed with unblended operands",
			k:    2,
			// first line pair 10,10 unblended; second pair blended
			code: cs(0, 0, t2rmoveto, 10, 10, 20, 30, 1, 2, 3, 4, 2, t2blend, t2rlineto),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpLineTo, Args: []Blend{b(10), b(10)}},
					{Op: OpLineTo, Args: []Blend{b(30, 1, 2), b(40, 3, 4)}},
				},
			},
		},
		{
			name: "flex1 blended horizontal",
			k:    2,
			// s0 blended [1,2], rest plain; large dx picks horizontal close
			code: cs(0, 0, t2rmoveto,
				100, 1, 2, 1, t2blend,
				0, 100, 0, 100, 0, 100, 0, 100, 0, 7, t2flex1),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{
					{Op: OpMoveTo, Args: []Blend{b(0), b(0)}},
					{Op: OpCurveTo, Args: []Blend{b(100, 1, 2), b(0), b(200, 1, 2), b(0), b(300, 1, 2), b(0)}},
					{Op: OpCurveTo, Args: []Blend{b(400, 1, 2), b(0), b(500, 1, 2), b(0), b(507, 1, 2), b(0)}},
				},
			},
		},
		{
			name: "stems + hintmask blended",
			k:    2,
			// first hstem pair 0,100 unblended; second pair blended; then
			// hintmask (1 mask byte)
			code: cs(0, 100, 200, 40, 10, 20, 30, 40, 2, t2blend, t2hstemhm, t2hintmask, byte(0x80)),
			want: &GlyphCFF2{
				HStem: []Blend{
					b(0), b(100),
					b(300, 10, 20), b(340, 40, 60),
				},
				Cmds: []GlyphOpCFF2{
					{Op: OpHintMask, Args: []Blend{b(0x80)}},
				},
			},
		},
		{
			name: "implicit vstem before hintmask",
			// hstem declared, then a bare vstem pair directly before hintmask
			code: cs(0, 10, t2hstem, 20, 30, t2hintmask, byte(0xc0)),
			want: &GlyphCFF2{
				HStem: []Blend{b(0), b(10)},
				VStem: []Blend{b(20), b(50)},
				Cmds:  []GlyphOpCFF2{{Op: OpHintMask, Args: []Blend{b(0xc0)}}},
			},
		},
		{
			name: "hintmask with zero stems dropped",
			code: cs(0, 0, t2rmoveto, t2hintmask),
			want: &GlyphCFF2{
				Cmds: []GlyphOpCFF2{{Op: OpMoveTo, Args: []Blend{b(0), b(0)}}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := newInfoCFF2(tc.k)
			got, err := info.decodeCharStringCFF2(tc.code)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("decode mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDecodeCFF2Subrs exercises nested subroutine calls with implicit return
// (no return opcode) and implicit endchar (no endchar opcode).
func TestDecodeCFF2Subrs(t *testing.T) {
	// gsubr 0: draw a line, no return byte at the end
	gsubr0 := cs(50, 0, t2rlineto)
	// gsubr 1: call gsubr 0, then draw another line, no return
	gsubr1 := cs(pushGsubrIdx(0), t2callgsubr, 0, 60, t2rlineto)

	info := &decodeInfoCFF2{
		gsubr:       cffIndex{gsubr0, gsubr1},
		regionCount: fixedRegionCount(0, 8),
		budget:      parser.NewBudget(1 << 20),
	}

	code := cs(0, 0, t2rmoveto, pushGsubrIdx(1), t2callgsubr, 5, 0, t2rlineto)
	got, err := info.decodeCharStringCFF2(code)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	b := func(x, y float64) []Blend { return []Blend{{Default: x}, {Default: y}} }
	want := &GlyphCFF2{
		Cmds: []GlyphOpCFF2{
			{Op: OpMoveTo, Args: b(0, 0)},
			{Op: OpLineTo, Args: b(50, 0)},
			{Op: OpLineTo, Args: b(50, 60)},
			{Op: OpLineTo, Args: b(55, 60)},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("decode mismatch (-want +got):\n%s", diff)
	}
}

func TestDecodeCFF2Errors(t *testing.T) {
	cases := []struct {
		name string
		k    int
		code []byte
	}{
		{
			name: "bad blend count zero",
			k:    2,
			code: cs(1, 2, 0, t2blend),
		},
		{
			name: "blend stack underflow",
			k:    2,
			code: cs(1, 2, 2, t2blend), // needs 2*(2+1)=6 below n
		},
		{
			name: "arithmetic opcode removed",
			code: cs(1, 2, t2add),
		},
		{
			name: "endchar removed",
			code: cs(0, 0, t2rmoveto, t2endchar),
		},
		{
			name: "return removed",
			code: cs(0, 0, t2rmoveto, t2return),
		},
		{
			name: "blend of blend base",
			k:    2,
			// first blend leaves a Blend on the stack, used as a base below
			code: cs(100, 10, 20, 1, t2blend, 200, 11, 12, 13, 14, 2, t2blend),
		},
		{
			name: "bad vsindex operand negative",
			code: cs(-1, t2vsindex),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := newInfoCFF2(tc.k)
			if _, err := info.decodeCharStringCFF2(tc.code); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// TestDecodeCFF2DepthOverflow checks that unbounded subroutine recursion is
// rejected by the call-depth cap rather than looping forever.
func TestDecodeCFF2DepthOverflow(t *testing.T) {
	// gsubr 0 calls itself forever
	gsubr0 := cs(pushGsubrIdx(0), t2callgsubr)
	info := &decodeInfoCFF2{
		gsubr:       cffIndex{gsubr0},
		regionCount: fixedRegionCount(0, 8),
		budget:      parser.NewBudget(1 << 30),
	}
	code := cs(pushGsubrIdx(0), t2callgsubr)
	if _, err := info.decodeCharStringCFF2(code); err == nil {
		t.Error("expected depth-overflow error, got nil")
	}
}

// TestDecodeCFF2BlendBudget checks that a region-heavy blend is charged against
// the budget so it cannot amplify memory use.
func TestDecodeCFF2BlendBudget(t *testing.T) {
	info := &decodeInfoCFF2{
		regionCount: fixedRegionCount(1000, 8), // 1000 regions per subtable
		budget:      membudget.New(100),        // tiny
	}
	// blend charges n*k*8 = 2*1000*8 bytes before touching the stack
	code := cs(2, t2blend)

	if _, err := info.decodeCharStringCFF2(code); !errors.Is(err, membudget.ErrExceeded) {
		t.Errorf("err = %v, want ErrExceeded", err)
	}
}

// TestDecodeCFF2StickyBlendBudget checks that once a coordinate has been
// blended, the k-sized Deltas slice re-emitted by every subsequent path
// operator is charged against the budget.  A single cheap blend (n=1)
// followed by a long run of plain hlineto operands must not be able to
// allocate far more than the budget allows: the position stays "sticky"
// blended (addBlend widens a nil Deltas to k once one side already has
// Deltas), so each of the n plain line operators emits a fresh k-length
// slice even though its own operand carries no deltas.
func TestDecodeCFF2StickyBlendBudget(t *testing.T) {
	const k = 100 // regions per blended value
	const n = 400 // number of plain hlineto operands after the blend

	parts := []any{0} // blended dx base
	for range k {
		parts = append(parts, 1) // blend deltas
	}
	parts = append(parts, 1, t2blend, 0, t2rmoveto) // n=1, blend, dy=0, rmoveto
	for range n {
		parts = append(parts, 1) // cheap plain hlineto operand
	}
	parts = append(parts, t2hlineto)
	code := cs(parts...)

	// tight budget: the charstring bytes and the single blend are cheap, but
	// n emitted k-sized deltas would need n*k*8 bytes -- far more than this
	// allows.
	tight := &decodeInfoCFF2{
		regionCount: fixedRegionCount(k, 8),
		budget:      membudget.New(int64(len(code)) + 4096),
	}
	if _, err := tight.decodeCharStringCFF2(code); !errors.Is(err, membudget.ErrExceeded) {
		t.Errorf("tight budget: err = %v, want ErrExceeded", err)
	}

	generous := &decodeInfoCFF2{
		regionCount: fixedRegionCount(k, 8),
		budget:      membudget.New(1 << 20),
	}
	if _, err := generous.decodeCharStringCFF2(code); err != nil {
		t.Errorf("generous budget: unexpected error: %v", err)
	}
}

func FuzzT2DecodeCFF2(f *testing.F) {
	f.Add(cs(100, 200, t2rmoveto, 50, 0, t2rlineto))
	f.Add(cs(0, 0, t2rmoveto, 10, 10, 10, 10, 10, 10, t2rrcurveto))
	f.Add(cs(100, 200, 10, 20, 30, 40, 2, t2blend, t2rmoveto))
	f.Add(cs(1, t2vsindex, 0, 0, t2rmoveto, 5, 7, 8, 2, t2blend, 0, t2rlineto))
	f.Add(cs(0, 100, 200, 10, 20, 30, 40, 2, t2blend, t2hstemhm, t2hintmask, byte(0x80)))

	f.Fuzz(func(t *testing.T, data []byte) {
		info := &decodeInfoCFF2{
			subr:        cffIndex{},
			gsubr:       cffIndex{},
			regionCount: fixedRegionCount(2, 8),
			budget:      parser.NewBudget(int64(len(data)) + 16),
		}
		// must not panic or hang; an error is fine
		info.decodeCharStringCFF2(data)
	})
}
