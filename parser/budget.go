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
	"unsafe"
)

// Memory-budget tuning constants.  See NewBudget for how they combine.
const (
	// perAllocOverhead is the fixed cost added to every charge, to
	// account for slice-header and heap-block overhead.  Without this
	// surcharge an attacker can amplify input size by issuing many
	// tiny allocations, each costing far more than its payload.
	perAllocOverhead = 32

	// budgetBase is the minimum budget any caller starts with,
	// independent of input size, so tiny tables can still parse.
	budgetBase = 1 << 20 // 1 MiB

	// budgetMultiplier is the maximum number of bytes a parser may
	// allocate per byte of input table data.
	budgetMultiplier = 64

	// budgetHardCap caps the input-proportional part of the budget,
	// so a header claiming a huge table cannot unlock unlimited memory.
	budgetHardCap = 128 << 20 // 128 MiB
)

// ErrBudgetExceeded is returned by Budget.Charge when an allocation
// would push the budget below zero.
var ErrBudgetExceeded = errors.New("sfnt: memory budget exceeded")

// Budget tracks the remaining memory budget for a single table parse.
// Callers that do not opt in to budget tracking can pass a nil *Budget;
// in that case every charge succeeds.
type Budget struct {
	remaining int64
}

// NewBudget returns a Budget sized for a table of tableSize bytes.
// The returned budget grows with tableSize up to a hard cap.
func NewBudget(tableSize int64) *Budget {
	if tableSize < 0 {
		tableSize = 0
	}
	add := int64(budgetMultiplier) * tableSize
	if add < 0 || add > budgetHardCap {
		add = budgetHardCap
	}
	return &Budget{remaining: budgetBase + add}
}

// Remaining returns the number of bytes still available in the budget.
// A nil receiver returns 0.
func (b *Budget) Remaining() int64 {
	if b == nil {
		return 0
	}
	return b.remaining
}

// Charge subtracts (bytes + perAllocOverhead) from the budget.  A nil
// receiver is a no-op (callers without budget tracking pass through).
// On exhaustion the budget is left unchanged and ErrBudgetExceeded is
// returned.
func (b *Budget) Charge(bytes int) error {
	if b == nil {
		return nil
	}
	if bytes < 0 {
		return ErrBudgetExceeded
	}
	cost := int64(bytes) + perAllocOverhead
	if cost < int64(bytes) { // overflow guard
		return ErrBudgetExceeded
	}
	if b.remaining < cost {
		return ErrBudgetExceeded
	}
	b.remaining -= cost
	return nil
}

// Slice charges b for a slice of n elements of T and, if the charge
// succeeds, returns make([]T, n).  A nil budget skips the check and
// always allocates.
//
// For slice, map, or interface element types, only the header size is
// charged here; the referenced elements must be charged separately when
// allocated.
func AllocSlice[T any](b *Budget, n int) ([]T, error) {
	var zero T
	size := int(unsafe.Sizeof(zero))
	if n < 0 {
		return nil, ErrBudgetExceeded
	}
	// overflow-safe multiplication for the byte count
	bytes := n * size
	if size != 0 && bytes/size != n {
		return nil, ErrBudgetExceeded
	}
	if err := b.Charge(bytes); err != nil {
		return nil, err
	}
	return make([]T, n), nil
}
