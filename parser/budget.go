// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package parser

import (
	"seehuhn.de/go/membudget"
)

// sfnt-specific budget sizing.  See [NewBudget] for how the constants
// combine.
const (
	// budgetBase is the minimum budget any table parse starts with,
	// independent of input size, so tiny tables can still parse.
	budgetBase = 1 << 20 // 1 MiB

	// budgetMultiplier is the maximum number of bytes a parser may
	// allocate per byte of input table data.
	budgetMultiplier = 64

	// budgetHardCap caps the input-proportional part of the budget,
	// so a header claiming a huge table cannot unlock unlimited memory.
	budgetHardCap = 128 << 20 // 128 MiB
)

// NewBudget returns a [*membudget.Budget] sized for parsing a table of
// tableSize bytes.  The returned budget grows with tableSize up to a
// hard cap.
func NewBudget(tableSize int64) *membudget.Budget {
	if tableSize < 0 {
		tableSize = 0
	}
	add := int64(budgetMultiplier) * tableSize
	if add < 0 || add > budgetHardCap {
		add = budgetHardCap
	}
	return membudget.New(budgetBase + add)
}
