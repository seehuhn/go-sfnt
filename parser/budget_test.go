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

package parser

import (
	"errors"
	"testing"
)

func TestNewBudget(t *testing.T) {
	cases := []struct {
		name      string
		tableSize int64
		want      int64
	}{
		{"zero", 0, budgetBase},
		{"small", 1024, budgetBase + budgetMultiplier*1024},
		{"capped", 1 << 30, budgetBase + budgetHardCap},
		{"negative", -1, budgetBase},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := NewBudget(c.tableSize)
			if b.Remaining() != c.want {
				t.Errorf("remaining = %d, want %d", b.Remaining(), c.want)
			}
		})
	}
}

func TestBudgetCharge(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		b := &Budget{remaining: 1000}
		if err := b.Charge(100); err != nil {
			t.Fatal(err)
		}
		if got, want := b.Remaining(), int64(1000-100-perAllocOverhead); got != want {
			t.Errorf("remaining = %d, want %d", got, want)
		}
	})

	t.Run("exhaustion leaves budget unchanged", func(t *testing.T) {
		b := &Budget{remaining: 10}
		if err := b.Charge(100); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("err = %v, want ErrBudgetExceeded", err)
		}
		if b.Remaining() != 10 {
			t.Errorf("remaining = %d, want 10", b.Remaining())
		}
	})

	t.Run("negative size errors", func(t *testing.T) {
		b := &Budget{remaining: 1000}
		if err := b.Charge(-1); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("err = %v, want ErrBudgetExceeded", err)
		}
	})

	t.Run("nil receiver is no-op", func(t *testing.T) {
		var b *Budget
		if err := b.Charge(1 << 30); err != nil {
			t.Fatal(err)
		}
		if b.Remaining() != 0 {
			t.Errorf("nil.Remaining = %d, want 0", b.Remaining())
		}
	})
}

func TestBudgetSurcharge(t *testing.T) {
	// many tiny charges should drain the budget faster than the sum of
	// the payloads, by perAllocOverhead per call.
	b := &Budget{remaining: 10 * perAllocOverhead}
	for i := range 10 {
		if err := b.Charge(0); err != nil {
			t.Fatalf("charge %d: %v", i, err)
		}
	}
	if err := b.Charge(0); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("11th charge: err = %v, want ErrBudgetExceeded", err)
	}
}

func TestSlice(t *testing.T) {
	t.Run("uint16", func(t *testing.T) {
		b := NewBudget(0)
		before := b.Remaining()
		s, err := AllocSlice[uint16](b, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 100 {
			t.Errorf("len = %d, want 100", len(s))
		}
		if got, want := before-b.Remaining(), int64(100*2+perAllocOverhead); got != want {
			t.Errorf("consumed = %d, want %d", got, want)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		b := NewBudget(0)
		before := b.Remaining()
		s, err := AllocSlice[*int](b, 4)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 4 {
			t.Errorf("len = %d, want 4", len(s))
		}
		// 8 bytes per pointer on 64-bit
		if got, want := before-b.Remaining(), int64(4*8+perAllocOverhead); got != want {
			t.Errorf("consumed = %d, want %d", got, want)
		}
	})

	t.Run("exhaustion", func(t *testing.T) {
		b := &Budget{remaining: 10}
		s, err := AllocSlice[uint16](b, 1000)
		if !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("err = %v, want ErrBudgetExceeded", err)
		}
		if s != nil {
			t.Errorf("got slice %v, want nil", s)
		}
		if b.Remaining() != 10 {
			t.Errorf("remaining = %d, want 10", b.Remaining())
		}
	})

	t.Run("nil receiver", func(t *testing.T) {
		s, err := AllocSlice[uint16](nil, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 5 {
			t.Errorf("len = %d, want 5", len(s))
		}
	})

	t.Run("overflow", func(t *testing.T) {
		// pick an n where n * sizeof(uint16) overflows
		s, err := AllocSlice[uint16](&Budget{remaining: 1 << 40}, int(^uint(0)>>1))
		if !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("err = %v, want ErrBudgetExceeded", err)
		}
		if s != nil {
			t.Errorf("got slice %v, want nil", s)
		}
	})
}
