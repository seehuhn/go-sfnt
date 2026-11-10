// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

// Memory-budget regression tests.  Each test crafts a GSUB/GPOS subtable
// which a naive reader would expand to orders of magnitude more memory than
// the table itself.  The reader must either return an error before
// exhausting heap memory, or, where the expansion comes from shared tables,
// read each shared table only once.

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
// seqRuleOffset inside that RuleSet points to the same Rule.
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

// TestSeqContext2Aliased checks that a SeqContext2 subtable in which all
// offsets reach the same RuleSet and Rule reads within the budget, since
// the shared tables are read only once, and that it can be written back.
func TestSeqContext2Aliased(t *testing.T) {
	checkSharedRoundTripDeep(t, readSeqContext2, makeAliasedSeqContext2(1024, 1024))
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

// TestGsub4_1Aliased is the counterpart of [TestSeqContext2Aliased] for
// ligature substitutions.
func TestGsub4_1Aliased(t *testing.T) {
	checkSharedRoundTripDeep(t, readGsub4_1, makeAliasedGsub4_1(1024, 1024))
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
