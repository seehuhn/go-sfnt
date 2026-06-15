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

package gtab

// Memory-budget regression tests.  Each test crafts a hostile GSUB/GPOS
// subtable that, without the per-table memory budget, would cause the
// reader to allocate orders of magnitude more memory than the table
// itself.  The expectation is that gtab.Read returns an error long
// before exhausting heap memory.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/parser"
)

func writeU16(buf *bytes.Buffer, v uint16) {
	binary.Write(buf, binary.BigEndian, v)
}

// makeAliasedSeqContext2 builds a SeqContext2 subtable in which every
// classSeqRuleSetOffset points to the same RuleSet, and every
// seqRuleOffset inside that RuleSet points to the same Rule.  Used by
// the parser via direct readSeqContext2 entry — see
// TestSeqContext2BudgetedAliasing for the budget-aware caller.
func makeAliasedSeqContext2(N, M uint16) []byte {
	sub := &bytes.Buffer{}
	writeU16(sub, 2) // format 2
	writeU16(sub, 0) // covOffset placeholder
	writeU16(sub, 0) // classDefOffset placeholder
	writeU16(sub, N) // classSeqRuleSetCount

	ruleSetOffsetsStart := uint16(sub.Len())
	for range N {
		writeU16(sub, 0)
	}

	covOff := uint16(sub.Len())
	writeU16(sub, 1) // format 1
	writeU16(sub, 0) // 0 glyphs

	clsOff := uint16(sub.Len())
	writeU16(sub, 2)     // format 2
	writeU16(sub, 1)     // 1 range
	writeU16(sub, 0)     // start
	writeU16(sub, 0)     // end
	writeU16(sub, 65535) // class -> NumClasses=65536

	ruleSetOff := uint16(sub.Len())
	writeU16(sub, M) // rule count
	ruleOff := uint16(2 + 2*M)
	for range M {
		writeU16(sub, ruleOff)
	}
	writeU16(sub, 1) // glyphCount = 1 (inputSequence empty)
	writeU16(sub, 0) // seqLookupCount = 0

	subBytes := sub.Bytes()
	binary.BigEndian.PutUint16(subBytes[2:4], covOff)
	binary.BigEndian.PutUint16(subBytes[4:6], clsOff)
	for i := range N {
		binary.BigEndian.PutUint16(subBytes[ruleSetOffsetsStart+2*i:], ruleSetOff)
	}
	return subBytes
}

// budgetedReadSeqContext2 returns the result of reading sub via
// readSeqContext2 with a budget sized for a hypothetical GSUB table of
// the same size — the same envelope the production gtab.Read uses.
func budgetedReadSeqContext2(t *testing.T, sub []byte) (Subtable, error) {
	t.Helper()
	p := parser.New(bytes.NewReader(sub), parser.NewBudget(int64(len(sub))))
	format, err := p.ReadUint16()
	if err != nil {
		t.Fatalf("read format: %v", err)
	}
	if format != 2 {
		t.Fatalf("unexpected format: %d", format)
	}
	return readSeqContext2(p, 0)
}

// TestSeqContext2BombBudgeted hands an aliasing bomb to readSeqContext2
// and asserts the budget rejects it before memory is exhausted.
func TestSeqContext2BombBudgeted(t *testing.T) {
	// 1024×1024 = ~1M aliased rules expanded from a few KiB of input.
	sub := makeAliasedSeqContext2(1024, 1024)
	_, err := budgetedReadSeqContext2(t, sub)
	if !errors.Is(err, membudget.ErrExceeded) {
		t.Fatalf("err = %v, want ErrExceeded", err)
	}
}

// makeAliasedGsub4_1 builds a Gsub 4.1 subtable in which every
// ligatureSetOffset points to the same LigatureSet and every
// ligatureOffset inside it points to the same Ligature.
func makeAliasedGsub4_1(N, M uint16) []byte {
	sub := &bytes.Buffer{}
	writeU16(sub, 1) // format 1
	writeU16(sub, 0) // covOffset placeholder
	writeU16(sub, N) // ligatureSetCount

	ligSetOffsetsStart := uint16(sub.Len())
	for range N {
		writeU16(sub, 0)
	}

	covOff := uint16(sub.Len())
	writeU16(sub, 1)
	writeU16(sub, N)
	for i := range N {
		writeU16(sub, i)
	}

	ligSetOff := uint16(sub.Len())
	writeU16(sub, M)
	ligOff := uint16(2 + 2*M)
	for range M {
		writeU16(sub, ligOff)
	}
	writeU16(sub, 0) // ligatureGlyph
	writeU16(sub, 1) // componentCount = 1 (no extra components)

	subBytes := sub.Bytes()
	binary.BigEndian.PutUint16(subBytes[2:4], covOff)
	for i := range N {
		binary.BigEndian.PutUint16(subBytes[ligSetOffsetsStart+2*i:], ligSetOff)
	}
	return subBytes
}

func TestGsub4_1BombBudgeted(t *testing.T) {
	sub := makeAliasedGsub4_1(1024, 1024)
	p := parser.New(bytes.NewReader(sub), parser.NewBudget(int64(len(sub))))
	format, err := p.ReadUint16()
	if err != nil || format != 1 {
		t.Fatalf("format: %d, err: %v", format, err)
	}
	_, err = readGsub4_1(p, 0)
	if !errors.Is(err, membudget.ErrExceeded) {
		t.Fatalf("err = %v, want ErrExceeded", err)
	}
}

// TestCoverageFormat2BombBudgeted exercises a coverage Format 2 table
// whose single range spans 65535 glyph IDs.  Without budget tracking
// the underlying map would grow to >1MiB from a 10-byte input.
func TestCoverageFormat2BombBudgeted(t *testing.T) {
	cov := &bytes.Buffer{}
	writeU16(cov, 2)     // format 2
	writeU16(cov, 1)     // rangeCount
	writeU16(cov, 0)     // startGlyphID
	writeU16(cov, 65535) // endGlyphID
	writeU16(cov, 0)     // startCoverageIndex

	data := cov.Bytes()
	// Budget sized for a small input table.  65535*24 ~= 1.5MiB charge,
	// which exceeds the 1MiB base + small input-proportional add.
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	_, err := coverage.Read(p, 0)
	if !errors.Is(err, membudget.ErrExceeded) {
		t.Fatalf("err = %v, want ErrExceeded", err)
	}
}

// TestClassdefFormat2BombBudgeted exercises a classdef Format 2 table
// whose ranges expand to millions of entries.
func TestClassdefFormat2BombBudgeted(t *testing.T) {
	buf := &bytes.Buffer{}
	writeU16(buf, 2)     // version
	writeU16(buf, 1)     // classRangeCount
	writeU16(buf, 0)     // startGlyphID
	writeU16(buf, 65535) // endGlyphID
	writeU16(buf, 1)     // classValue (non-zero)

	data := buf.Bytes()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	_, err := classdef.Read(p, 0)
	if !errors.Is(err, membudget.ErrExceeded) {
		t.Fatalf("err = %v, want ErrExceeded", err)
	}
}
