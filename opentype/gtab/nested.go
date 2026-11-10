// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
	"seehuhn.de/go/sfnt/parser"
)

// SeqLookup describes the actions for contextual and chained contextual
// lookups.
type SeqLookup struct {
	SequenceIndex   uint16
	LookupListIndex LookupIndex
}

func readNested(p *parser.Parser, seqLookupCount int) ([]SeqLookup, error) {
	res, err := p.Budget.AllocSlice[SeqLookup](seqLookupCount)
	if err != nil {
		return nil, err
	}
	for i := range res {
		buf, err := p.ReadBytes(4)
		if err != nil {
			return nil, err
		}
		res[i].SequenceIndex = uint16(buf[0])<<8 | uint16(buf[1])
		res[i].LookupListIndex = LookupIndex(buf[2])<<8 | LookupIndex(buf[3])
	}
	return res, nil
}

func appendUint16(buf []byte, x int) []byte {
	return append(buf, byte(x>>8), byte(x))
}

func appendActions(buf []byte, actions []SeqLookup) []byte {
	for _, a := range actions {
		buf = append(buf,
			byte(a.SequenceIndex>>8), byte(a.SequenceIndex),
			byte(a.LookupListIndex>>8), byte(a.LookupListIndex),
		)
	}
	return buf
}

// encodeSeqRule encodes a rule table of a [SeqContext1] subtable.
func encodeSeqRule(r *SeqRule) []byte {
	return encodeSeqRuleGeneric(r.Input, r.Actions)
}

// encodeClassSeqRule encodes a rule table of a [SeqContext2] subtable.
func encodeClassSeqRule(r *ClassSeqRule) []byte {
	return encodeSeqRuleGeneric(r.Input, r.Actions)
}

func encodeSeqRuleGeneric[T ~uint16](input []T, actions []SeqLookup) []byte {
	buf := make([]byte, 0, 4+2*len(input)+4*len(actions))
	buf = appendUint16(buf, len(input)+1)
	buf = appendUint16(buf, len(actions))
	for _, x := range input {
		buf = appendUint16(buf, int(x))
	}
	return appendActions(buf, actions)
}

// encodeChainedSeqRule encodes a rule table of a [ChainedSeqContext1]
// subtable.
func encodeChainedSeqRule(r *ChainedSeqRule) []byte {
	return encodeChainedRuleGeneric(r.Backtrack, r.Input, r.Lookahead, r.Actions)
}

// encodeChainedClassSeqRule encodes a rule table of a [ChainedSeqContext2]
// subtable.
func encodeChainedClassSeqRule(r *ChainedClassSeqRule) []byte {
	return encodeChainedRuleGeneric(r.Backtrack, r.Input, r.Lookahead, r.Actions)
}

func encodeChainedRuleGeneric[T ~uint16](backtrack, input, lookahead []T, actions []SeqLookup) []byte {
	buf := make([]byte, 0, 8+2*(len(backtrack)+len(input)+len(lookahead))+4*len(actions))
	buf = appendUint16(buf, len(backtrack))
	for _, x := range backtrack {
		buf = appendUint16(buf, int(x))
	}
	buf = appendUint16(buf, len(input)+1)
	for _, x := range input {
		buf = appendUint16(buf, int(x))
	}
	buf = appendUint16(buf, len(lookahead))
	for _, x := range lookahead {
		buf = appendUint16(buf, int(x))
	}
	buf = appendUint16(buf, len(actions))
	return appendActions(buf, actions)
}

// encodeRuleSet encodes a rule set table.  Rules with identical encodings,
// and rules shared between several references, are stored once.
func encodeRuleSet[R any](name string, rules []*R, encodeRule func(*R) []byte) ([]byte, error) {
	pool := offsetpool.NewDeferred(name+" rule", 2+2*len(rules))
	idx := make([]int, len(rules))
	for i, r := range rules {
		var err error
		idx[i], err = pool.AddFunc(r, func() []byte { return encodeRule(r) })
		if err != nil {
			return nil, err
		}
	}
	offs, data := pool.Place()
	buf := make([]byte, 0, 2+2*len(rules)+len(data))
	buf = appendUint16(buf, len(rules))
	for _, i := range idx {
		buf = appendUint16(buf, int(offs[i]))
	}
	return append(buf, data...), nil
}

// ruleSetLayout is the encoded layout of a subtable of type SeqContext1,
// SeqContext2, ChainedSeqContext1 or ChainedSeqContext2.  The header
// consists of the format, the offsets of the coverage and class definition
// tables, and the offsets of the rule sets.  The tables follow the header.
// Identical tables are stored once.
type ruleSetLayout struct {
	name    string
	format  uint16
	table   []int // pool indices of the coverage and class definition tables
	ruleSet []int // pool indices of the rule sets, -1 for absent rule sets
	pool    *offsetpool.Deferred
}

func newRuleSetLayout(name string, format uint16, nTables, nSets int) *ruleSetLayout {
	ruleSet := make([]int, nSets)
	for i := range ruleSet {
		ruleSet[i] = -1
	}
	return &ruleSetLayout{
		name:    name,
		format:  format,
		table:   make([]int, nTables),
		ruleSet: ruleSet,
		pool:    offsetpool.NewDeferred(name, 4+2*nTables+2*nSets),
	}
}

// addTable stores a coverage or class definition table for header field i.
func (lay *ruleSetLayout) addTable(i int, enc []byte) error {
	idx, err := lay.pool.Add(enc)
	if err != nil {
		return err
	}
	lay.table[i] = idx
	return nil
}

// add stores the coverage and class definition tables, given in header
// order, and the rule sets which addSets stores.  If tablesFirst is set, the
// tables are placed before the rule sets, otherwise after them.  The pool
// reorders the tables if the subtable does not fit this way.
func (lay *ruleSetLayout) add(tablesFirst bool, tables [][]byte, addSets func() error) error {
	addTables := func() error {
		for i, enc := range tables {
			if err := lay.addTable(i, enc); err != nil {
				return err
			}
		}
		return nil
	}
	first, second := addSets, addTables
	if tablesFirst {
		first, second = addTables, addSets
	}
	if err := first(); err != nil {
		return err
	}
	return second()
}

// addRuleSets stores the rule sets.  Rule sets which are nil are absent.
// Rule sets with the same rules are encoded once.
func addRuleSets[R any](lay *ruleSetLayout, sets [][]*R, encodeRule func(*R) []byte) error {
	var keys offsetpool.SeqKeys[*R]
	for i, rules := range sets {
		if rules == nil {
			continue
		}
		var encErr error
		idx, err := lay.pool.AddFunc(keys.Key(offsetpool.SliceKey(rules), len(rules), slices.Values(rules)), func() []byte {
			var enc []byte
			enc, encErr = encodeRuleSet(lay.name, rules, encodeRule)
			return enc
		})
		if encErr != nil {
			return encErr
		}
		if err != nil {
			return err
		}
		lay.ruleSet[i] = idx
	}
	return nil
}

func (lay *ruleSetLayout) encodeLen() int {
	return 4 + 2*len(lay.table) + 2*len(lay.ruleSet) + lay.pool.Len()
}

func (lay *ruleSetLayout) encode() []byte {
	offs, data := lay.pool.Place()
	buf := make([]byte, 0, lay.encodeLen())
	buf = appendUint16(buf, int(lay.format))
	buf = appendOffsets(buf, offs, lay.table)
	buf = appendUint16(buf, len(lay.ruleSet))
	buf = appendOffsets(buf, offs, lay.ruleSet)
	return append(buf, data...)
}

// addCoverageSets stores the coverage tables for sets and returns their
// pool indices.
func addCoverageSets(pool *offsetpool.Deferred, sets []coverage.Set) ([]int, error) {
	indices := make([]int, len(sets))
	for i, set := range sets {
		idx, err := pool.AddFunc(offsetpool.MapKey(set), func() []byte { return set.ToTable().Encode() })
		if err != nil {
			return nil, err
		}
		indices[i] = idx
	}
	return indices, nil
}

// poolOffset returns the offset of the pool table with index idx.  A
// negative index stands for an absent table and gives offset 0.
func poolOffset(offs []uint16, idx int) uint16 {
	if idx < 0 {
		return 0
	}
	return offs[idx]
}

// appendOffsets appends the offsets of the pool tables with the given
// indices.
func appendOffsets(buf []byte, offs []uint16, indices []int) []byte {
	for _, idx := range indices {
		buf = appendUint16(buf, int(poolOffset(offs, idx)))
	}
	return buf
}

// SeqContext1 is used for GSUB type 5 format 1 subtables and for GPOS type 7 format 1 subtables.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#sequence-context-format-1-simple-glyph-contexts
type SeqContext1 struct {
	Cov   coverage.Table
	Rules [][]*SeqRule // indexed by coverage index
}

// SeqRule describes a rule in a [SeqContext1] subtable.
type SeqRule struct {
	Input   []glyph.ID // excludes the first input glyph, since this is in Cov
	Actions []SeqLookup
}

func readSeqContext1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	coverageOffset, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	seqRuleSetOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}

	cov, err := coverage.Read(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}
	if len(cov) > len(seqRuleSetOffsets) {
		cov.Prune(len(seqRuleSetOffsets))
	} else {
		seqRuleSetOffsets = seqRuleSetOffsets[:len(cov)]
	}

	rulesOuter, err := p.Budget.AllocSlice[[]*SeqRule](len(seqRuleSetOffsets))
	if err != nil {
		return nil, err
	}
	res := &SeqContext1{
		Cov:   cov,
		Rules: rulesOuter,
	}

	ruleCache := readcache.New[*SeqRule]()
	readRule := func(pos int64) (*SeqRule, error) {
		err := p.SeekPos(pos)
		if err != nil {
			return nil, err
		}
		buf, err := p.ReadBytes(4)
		if err != nil {
			return nil, err
		}
		glyphCount := int(buf[0])<<8 | int(buf[1])
		if glyphCount == 0 {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "invalid glyph count in SeqContext1",
			}
		}
		seqLookupCount := int(buf[2])<<8 | int(buf[3])
		inputSequence, err := p.Budget.AllocSlice[glyph.ID](glyphCount - 1)
		if err != nil {
			return nil, err
		}
		for k := range inputSequence {
			xk, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			inputSequence[k] = glyph.ID(xk)
		}
		actions, err := readNested(p, seqLookupCount)
		if err != nil {
			return nil, err
		}
		return &SeqRule{Input: inputSequence, Actions: actions}, nil
	}
	setCache := readcache.New[[]*SeqRule]()
	readSet := func(base int64) ([]*SeqRule, error) {
		err := p.SeekPos(base)
		if err != nil {
			return nil, err
		}
		seqRuleOffsets, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}
		rules, err := p.Budget.AllocSlice[*SeqRule](len(seqRuleOffsets))
		if err != nil {
			return nil, err
		}
		for j, seqRuleOffset := range seqRuleOffsets {
			rules[j], err = ruleCache.Get(base+int64(seqRuleOffset), readRule)
			if err != nil {
				return nil, err
			}
		}
		return rules, nil
	}

	for i, seqRuleSetOffset := range seqRuleSetOffsets {
		if seqRuleSetOffset == 0 {
			continue
		}
		res.Rules[i], err = setCache.Get(subtablePos+int64(seqRuleSetOffset), readSet)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

// apply implements the [Subtable] interface.
func (l *SeqContext1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	gid := seq[a].GID
	rulesIdx, ok := l.Cov[gid]
	if !ok {
		return -1
	}
	rules := l.Rules[rulesIdx]

	matchPos := ctx.scratch
ruleLoop:
	for _, rule := range rules {
		p := a
		matchPos = append(matchPos[:0], p)
		glyphsNeeded := len(rule.Input)
		for _, gid := range rule.Input {
			glyphsNeeded--
			p++
			for p+glyphsNeeded < b && !keep.Keep(seq[p].GID) {
				p++
			}
			if p+glyphsNeeded >= b || seq[p].GID != gid {
				continue ruleLoop
			}
			matchPos = append(matchPos, p)
		}

		p++
		for p < b && !keep.Keep(seq[p].GID) {
			p++
		}

		ctx.scratch = nil // claim the scratch space as our own
		ctx.stack = append(ctx.stack, &nested{
			InputPos: matchPos,
			Actions:  rule.Actions,
			EndPos:   p,
		})
		return p
	}

	ctx.scratch = matchPos // release the scratch space
	return -1
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *SeqContext1) layout() (*ruleSetLayout, error) {
	lay := newRuleSetLayout("SeqContext1", 1, 1, len(l.Rules))
	err := lay.add(false, [][]byte{l.Cov.Encode()}, func() error {
		return addRuleSets(lay, l.Rules, encodeSeqRule)
	})
	return lay, err
}

// encodeLen implements the [Subtable] interface.
func (l *SeqContext1) encodeLen() int {
	return mustFit(l.layout()).encodeLen()
}

// encode implements the [Subtable] interface.
func (l *SeqContext1) encode() []byte {
	return mustFit(l.layout()).encode()
}

// SeqContext2 is used for GSUB type 5 format 2 subtables and for GPOS type 7 format 2 subtables.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#sequence-context-format-2-class-based-glyph-contexts
type SeqContext2 struct {
	Cov   coverage.Table
	Input classdef.Table
	Rules [][]*ClassSeqRule // indexed by class index of the first glyph
}

// ClassSeqRule describes a sequence of glyph classes and the actions to
// be performed
type ClassSeqRule struct {
	Input   []uint16 // excludes the first input glyph
	Actions []SeqLookup
}

func readSeqContext2(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	coverageOffset := uint16(buf[0])<<8 | uint16(buf[1])
	classDefOffset := uint16(buf[2])<<8 | uint16(buf[3])
	classSeqRuleSetOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}

	cov, err := coverage.Read(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}

	classDef, err := classdef.Read(p, subtablePos+int64(classDefOffset))
	if err != nil {
		return nil, err
	}

	numClasses := classDef.NumClasses()
	if len(classSeqRuleSetOffsets) > numClasses {
		classSeqRuleSetOffsets = classSeqRuleSetOffsets[:numClasses]
	}
	seqRuleSetCount := len(classSeqRuleSetOffsets)

	rulesOuter, err := p.Budget.AllocSlice[[]*ClassSeqRule](seqRuleSetCount)
	if err != nil {
		return nil, err
	}
	res := &SeqContext2{
		Cov:   cov,
		Input: classDef,
		Rules: rulesOuter,
	}

	ruleCache := readcache.New[*ClassSeqRule]()
	readRule := func(pos int64) (*ClassSeqRule, error) {
		err := p.SeekPos(pos)
		if err != nil {
			return nil, err
		}
		buf, err := p.ReadBytes(4)
		if err != nil {
			return nil, err
		}
		glyphCount := int(buf[0])<<8 | int(buf[1])
		if glyphCount == 0 {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "invalid glyph count in SeqContext2",
			}
		}
		seqLookupCount := int(buf[2])<<8 | int(buf[3])
		inputSequence, err := p.Budget.AllocSlice[uint16](glyphCount - 1)
		if err != nil {
			return nil, err
		}
		for k := range inputSequence {
			inputSequence[k], err = p.ReadUint16()
			if err != nil {
				return nil, err
			}
		}
		actions, err := readNested(p, seqLookupCount)
		if err != nil {
			return nil, err
		}
		return &ClassSeqRule{Input: inputSequence, Actions: actions}, nil
	}
	setCache := readcache.New[[]*ClassSeqRule]()
	readSet := func(base int64) ([]*ClassSeqRule, error) {
		err := p.SeekPos(base)
		if err != nil {
			return nil, err
		}
		seqRuleOffsets, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}
		rules, err := p.Budget.AllocSlice[*ClassSeqRule](len(seqRuleOffsets))
		if err != nil {
			return nil, err
		}
		for j, seqRuleOffset := range seqRuleOffsets {
			rules[j], err = ruleCache.Get(base+int64(seqRuleOffset), readRule)
			if err != nil {
				return nil, err
			}
		}
		return rules, nil
	}

	for i, classSeqRuleSetOffset := range classSeqRuleSetOffsets {
		if classSeqRuleSetOffset == 0 {
			continue
		}
		res.Rules[i], err = setCache.Get(subtablePos+int64(classSeqRuleSetOffset), readSet)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

// apply implements the [Subtable] interface.
func (l *SeqContext2) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	gid := seq[a].GID
	_, ok := l.Cov[gid]
	if !ok {
		return -1
	}
	ruleIdx := l.Input[gid]
	if int(ruleIdx) >= len(l.Rules) {
		return -1
	}
	rules := l.Rules[ruleIdx]

	matchPos := ctx.scratch
ruleLoop:
	for _, rule := range rules {
		p := a
		matchPos = append(matchPos[:0], p)
		glyphsNeeded := len(rule.Input)
		for _, cls := range rule.Input {
			glyphsNeeded--
			p++
			for p+glyphsNeeded < b && !keep.Keep(seq[p].GID) {
				p++
			}
			if p+glyphsNeeded >= b || l.Input[seq[p].GID] != cls {
				continue ruleLoop
			}
			matchPos = append(matchPos, p)
		}

		p++
		for p < b && !keep.Keep(seq[p].GID) {
			p++
		}

		ctx.scratch = nil // claim the scratch space as our own
		ctx.stack = append(ctx.stack, &nested{
			InputPos: matchPos,
			Actions:  rule.Actions,
			EndPos:   p,
		})
		return p
	}

	ctx.scratch = matchPos // release the scratch space
	return -1
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *SeqContext2) layout() (*ruleSetLayout, error) {
	tables := [][]byte{l.Cov.Encode(), l.Input.Append(nil)}
	lay := newRuleSetLayout("SeqContext2", 2, 2, len(l.Rules))
	err := lay.add(false, tables, func() error {
		return addRuleSets(lay, l.Rules, encodeClassSeqRule)
	})
	return lay, err
}

// encodeLen implements the [Subtable] interface.
func (l *SeqContext2) encodeLen() int {
	return mustFit(l.layout()).encodeLen()
}

// encode implements the [Subtable] interface.
func (l *SeqContext2) encode() []byte {
	return mustFit(l.layout()).encode()
}

// SeqContext3 is used for GSUB type 5 format 3 and for GPOS type 7 format 3 subtables.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#sequence-context-format-3-coverage-based-glyph-contexts
type SeqContext3 struct {
	Input   []coverage.Set
	Actions []SeqLookup
}

// coverageSetReader returns a function which reads a coverage table as a set.
func coverageSetReader(p *parser.Parser) func(pos int64) (coverage.Set, error) {
	return func(pos int64) (coverage.Set, error) {
		return coverage.ReadSet(p, pos)
	}
}

func readSeqContext3(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	glyphCount := int(buf[0])<<8 | int(buf[1])
	if glyphCount < 1 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    "invalid glyph count in SeqContext3",
		}
	}
	seqLookupCount := int(buf[2])<<8 | int(buf[3])
	coverageOffsets, err := p.Budget.AllocSlice[uint16](glyphCount)
	if err != nil {
		return nil, err
	}
	for i := range coverageOffsets {
		coverageOffsets[i], err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
	}

	actions, err := readNested(p, seqLookupCount)
	if err != nil {
		return nil, err
	}

	cov, err := p.Budget.AllocSlice[coverage.Set](glyphCount)
	if err != nil {
		return nil, err
	}
	covCache := readcache.New[coverage.Set]()
	for i, offset := range coverageOffsets {
		cov[i], err = covCache.Get(subtablePos+int64(offset), coverageSetReader(p))
		if err != nil {
			return nil, err
		}
	}

	res := &SeqContext3{
		Input:   cov,
		Actions: actions,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *SeqContext3) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	gid := seq[a].GID
	if !l.Input[0][gid] {
		return -1
	}

	p := a
	matchPos := append(ctx.scratch[:0], p)
	glyphsNeeded := len(l.Input) - 1
	for _, cov := range l.Input[1:] {
		glyphsNeeded--
		p++
		for p+glyphsNeeded < b && !keep.Keep(seq[p].GID) {
			p++
		}
		if p+glyphsNeeded >= b || !cov[seq[p].GID] {
			ctx.scratch = matchPos // release the scratch space
			return -1
		}
		matchPos = append(matchPos, p)
	}

	p++
	for p < b && !keep.Keep(seq[p].GID) {
		p++
	}

	ctx.scratch = nil // claim the scratch space
	ctx.stack = append(ctx.stack, &nested{
		InputPos: matchPos,
		Actions:  l.Actions,
		EndPos:   p,
	})
	return p
}

// seqContext3Layout is the encoded layout of a [SeqContext3] subtable: the
// header with the coverage offsets and the actions, followed by the
// coverage tables.  Identical tables are stored once.
type seqContext3Layout struct {
	cov      []int // pool indices of the coverage tables
	nActions int
	pool     *offsetpool.Deferred
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *SeqContext3) layout() (*seqContext3Layout, error) {
	pool := offsetpool.NewDeferred("SeqContext3", 6+2*len(l.Input)+4*len(l.Actions))
	cov, err := addCoverageSets(pool, l.Input)
	if err != nil {
		return nil, err
	}
	return &seqContext3Layout{cov: cov, nActions: len(l.Actions), pool: pool}, nil
}

// encodeLen implements the [Subtable] interface.
func (l *SeqContext3) encodeLen() int {
	lay := mustFit(l.layout())
	return 6 + 2*len(lay.cov) + 4*lay.nActions + lay.pool.Len()
}

// encode implements the [Subtable] interface.
func (l *SeqContext3) encode() []byte {
	lay := mustFit(l.layout())
	offs, data := lay.pool.Place()
	buf := make([]byte, 0, 6+2*len(lay.cov)+4*lay.nActions+len(data))
	buf = appendUint16(buf, 3) // format
	buf = appendUint16(buf, len(l.Input))
	buf = appendUint16(buf, len(l.Actions))
	buf = appendOffsets(buf, offs, lay.cov)
	buf = appendActions(buf, l.Actions)
	return append(buf, data...)
}

// ChainedSeqContext1 is used for GSUB type 6 format 1 and for GPOS type 8 format 1 subtables.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#chseqctxt1
type ChainedSeqContext1 struct {
	Cov   coverage.Table
	Rules [][]*ChainedSeqRule // indexed by coverage index
}

// ChainedSeqRule describes the rules in a ChainedSeqContext1.
type ChainedSeqRule struct {
	Backtrack []glyph.ID
	Input     []glyph.ID // excludes the first input glyph, since this is in Cov
	Lookahead []glyph.ID
	Actions   []SeqLookup
}

func readChainedSeqContext1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	coverageOffset, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	chainedSeqRuleSetOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}

	cov, err := coverage.Read(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}

	if len(cov) > len(chainedSeqRuleSetOffsets) {
		cov.Prune(len(chainedSeqRuleSetOffsets))
	} else {
		chainedSeqRuleSetOffsets = chainedSeqRuleSetOffsets[:len(cov)]
	}

	rules, err := p.Budget.AllocSlice[[]*ChainedSeqRule](len(chainedSeqRuleSetOffsets))
	if err != nil {
		return nil, err
	}

	ruleCache := readcache.New[*ChainedSeqRule]()
	readRule := func(pos int64) (*ChainedSeqRule, error) {
		err := p.SeekPos(pos)
		if err != nil {
			return nil, err
		}
		backtrackSequence, err := readGIDSlice(p)
		if err != nil {
			return nil, err
		}
		inputGlyphCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		if inputGlyphCount == 0 {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "invalid input glyph count in ChainedSeqContext1",
			}
		}
		inputSequence, err := p.Budget.AllocSlice[glyph.ID](int(inputGlyphCount) - 1)
		if err != nil {
			return nil, err
		}
		for k := range inputSequence {
			val, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			inputSequence[k] = glyph.ID(val)
		}
		lookaheadSequence, err := readGIDSlice(p)
		if err != nil {
			return nil, err
		}
		seqLookupCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		actions, err := readNested(p, int(seqLookupCount))
		if err != nil {
			return nil, err
		}
		return &ChainedSeqRule{
			Backtrack: backtrackSequence,
			Input:     inputSequence,
			Lookahead: lookaheadSequence,
			Actions:   actions,
		}, nil
	}
	setCache := readcache.New[[]*ChainedSeqRule]()
	readSet := func(base int64) ([]*ChainedSeqRule, error) {
		err := p.SeekPos(base)
		if err != nil {
			return nil, err
		}
		chainedSeqRuleOffsets, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}
		set, err := p.Budget.AllocSlice[*ChainedSeqRule](len(chainedSeqRuleOffsets))
		if err != nil {
			return nil, err
		}
		for j, chainedSeqRuleOffset := range chainedSeqRuleOffsets {
			set[j], err = ruleCache.Get(base+int64(chainedSeqRuleOffset), readRule)
			if err != nil {
				return nil, err
			}
		}
		return set, nil
	}

	for i, chainedSeqRuleSetOffset := range chainedSeqRuleSetOffsets {
		if chainedSeqRuleSetOffset == 0 {
			continue
		}
		rules[i], err = setCache.Get(subtablePos+int64(chainedSeqRuleSetOffset), readSet)
		if err != nil {
			return nil, err
		}
	}

	res := &ChainedSeqContext1{
		Cov:   cov,
		Rules: rules,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *ChainedSeqContext1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	gid := seq[a].GID
	rulesIdx, ok := l.Cov[gid]
	if !ok {
		return -1
	}
	rules := l.Rules[rulesIdx]

	matchPos := ctx.scratch
ruleLoop:
	for _, rule := range rules {
		p := a
		glyphsNeeded := len(rule.Backtrack)
		for _, gid := range rule.Backtrack {
			glyphsNeeded--
			p--
			for p-glyphsNeeded >= 0 && !keep.Keep(seq[p].GID) {
				p--
			}
			if p-glyphsNeeded < 0 || seq[p].GID != gid {
				continue ruleLoop
			}
		}

		p = a
		matchPos = append(matchPos[:0], p)
		glyphsNeeded = len(rule.Input)
		for _, gid := range rule.Input {
			glyphsNeeded--
			p++
			for p+glyphsNeeded < b && !keep.Keep(seq[p].GID) {
				p++
			}
			if p+glyphsNeeded >= b || seq[p].GID != gid {
				continue ruleLoop
			}
			matchPos = append(matchPos, p)
		}
		next := p

		glyphsNeeded = len(rule.Lookahead)
		for _, gid := range rule.Lookahead {
			glyphsNeeded--
			p++
			for p+glyphsNeeded < len(seq) && !keep.Keep(seq[p].GID) {
				p++
			}
			if p+glyphsNeeded >= len(seq) || seq[p].GID != gid {
				continue ruleLoop
			}
		}

		next++
		for next < b && !keep.Keep(seq[next].GID) {
			next++
		}

		ctx.scratch = nil // claim the scratch space
		ctx.stack = append(ctx.stack, &nested{
			InputPos: matchPos,
			Actions:  rule.Actions,
			EndPos:   next,
		})
		return next
	}

	ctx.scratch = matchPos // release the scratch space
	return -1
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *ChainedSeqContext1) layout() (*ruleSetLayout, error) {
	lay := newRuleSetLayout("ChainedSeqContext1", 1, 1, len(l.Rules))
	err := lay.add(true, [][]byte{l.Cov.Encode()}, func() error {
		return addRuleSets(lay, l.Rules, encodeChainedSeqRule)
	})
	return lay, err
}

// encodeLen implements the [Subtable] interface.
func (l *ChainedSeqContext1) encodeLen() int {
	return mustFit(l.layout()).encodeLen()
}

// encode implements the [Subtable] interface.
func (l *ChainedSeqContext1) encode() []byte {
	return mustFit(l.layout()).encode()
}

// ChainedSeqContext2 is used for GSUB type 6 format 2 and for GPOS type 8 format 2 subtables.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#chseqctxt2
type ChainedSeqContext2 struct {
	Cov       coverage.Table
	Backtrack classdef.Table
	Input     classdef.Table
	Lookahead classdef.Table
	Rules     [][]*ChainedClassSeqRule // indexed by input glyph class
}

// ChainedClassSeqRule is used to represent the rules in a [ChainedSeqContext2].
// It describes a sequence of nested lookups together with the context where
// they apply.  The Backtrack, Input and Lookahead sequences are given
// as lists of glyph classes, as defined by the corresponding class definition
// tables in the ChainedSeqContext2 structure.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#chseqctxt2
type ChainedClassSeqRule struct {
	Backtrack []uint16
	Input     []uint16 // excludes the first input glyph, since this is in Cov
	Lookahead []uint16
	Actions   []SeqLookup
}

func readChainedSeqContext2(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(8)
	if err != nil {
		return nil, err
	}
	covOffset := uint16(buf[0])<<8 | uint16(buf[1])
	backtrackClassDefOffset := uint16(buf[2])<<8 | uint16(buf[3])
	inputClassDefOffset := uint16(buf[4])<<8 | uint16(buf[5])
	lookaheadClassDefOffset := uint16(buf[6])<<8 | uint16(buf[7])

	chainedClassSeqRuleSetOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}

	cov, err := coverage.Read(p, subtablePos+int64(covOffset))
	if err != nil {
		return nil, err
	}
	classDefCache := readcache.New[classdef.Table]()
	readClassDef := func(pos int64) (classdef.Table, error) {
		return classdef.Read(p, pos)
	}
	backtrackClassDef, err := classDefCache.Get(subtablePos+int64(backtrackClassDefOffset), readClassDef)
	if err != nil {
		return nil, err
	}
	inputClassDef, err := classDefCache.Get(subtablePos+int64(inputClassDefOffset), readClassDef)
	if err != nil {
		return nil, err
	}
	lookaheadClassDef, err := classDefCache.Get(subtablePos+int64(lookaheadClassDefOffset), readClassDef)
	if err != nil {
		return nil, err
	}

	numClasses := inputClassDef.NumClasses()
	if numClasses < len(chainedClassSeqRuleSetOffsets) {
		chainedClassSeqRuleSetOffsets = chainedClassSeqRuleSetOffsets[:numClasses]
	}

	rules, err := p.Budget.AllocSlice[[]*ChainedClassSeqRule](len(chainedClassSeqRuleSetOffsets))
	if err != nil {
		return nil, err
	}

	ruleCache := readcache.New[*ChainedClassSeqRule]()
	readRule := func(pos int64) (*ChainedClassSeqRule, error) {
		err := p.SeekPos(pos)
		if err != nil {
			return nil, err
		}
		backtrackSequence, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}
		inputGlyphCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		if inputGlyphCount == 0 {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "invalid input glyph count in ChainedSeqContext2",
			}
		}
		inputSequence, err := p.Budget.AllocSlice[uint16](int(inputGlyphCount) - 1)
		if err != nil {
			return nil, err
		}
		for k := range inputSequence {
			inputSequence[k], err = p.ReadUint16()
			if err != nil {
				return nil, err
			}
		}
		lookaheadSequence, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}
		seqLookupCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		actions, err := readNested(p, int(seqLookupCount))
		if err != nil {
			return nil, err
		}
		return &ChainedClassSeqRule{
			Backtrack: backtrackSequence,
			Input:     inputSequence,
			Lookahead: lookaheadSequence,
			Actions:   actions,
		}, nil
	}
	setCache := readcache.New[[]*ChainedClassSeqRule]()
	readSet := func(base int64) ([]*ChainedClassSeqRule, error) {
		err := p.SeekPos(base)
		if err != nil {
			return nil, err
		}
		chainedClassSeqRuleOffsets, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}
		set, err := p.Budget.AllocSlice[*ChainedClassSeqRule](len(chainedClassSeqRuleOffsets))
		if err != nil {
			return nil, err
		}
		for j, chainedClassSeqRuleOffset := range chainedClassSeqRuleOffsets {
			set[j], err = ruleCache.Get(base+int64(chainedClassSeqRuleOffset), readRule)
			if err != nil {
				return nil, err
			}
		}
		return set, nil
	}

	for i, chainedClassSeqRuleSetOffset := range chainedClassSeqRuleSetOffsets {
		if chainedClassSeqRuleSetOffset == 0 {
			continue
		}
		rules[i], err = setCache.Get(subtablePos+int64(chainedClassSeqRuleSetOffset), readSet)
		if err != nil {
			return nil, err
		}
	}

	res := &ChainedSeqContext2{
		Cov:       cov,
		Backtrack: backtrackClassDef,
		Input:     inputClassDef,
		Lookahead: lookaheadClassDef,
		Rules:     rules,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *ChainedSeqContext2) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	gid := seq[a].GID
	_, ok := l.Cov[gid]
	if !ok {
		return -1
	}
	rulesIdx := l.Input[gid]
	if int(rulesIdx) >= len(l.Rules) {
		return -1
	}
	rules := l.Rules[rulesIdx]

	matchPos := ctx.scratch
ruleLoop:
	for _, rule := range rules {
		p := a
		glyphsNeeded := len(rule.Backtrack)
		for _, cls := range rule.Backtrack {
			glyphsNeeded--
			p--
			for p-glyphsNeeded >= 0 && !keep.Keep(seq[p].GID) {
				p--
			}
			if p-glyphsNeeded < 0 || l.Backtrack[seq[p].GID] != cls {
				continue ruleLoop
			}
		}

		p = a
		matchPos = append(matchPos[:0], p)
		glyphsNeeded = len(rule.Input)
		for _, cls := range rule.Input {
			glyphsNeeded--
			p++
			for p+glyphsNeeded < b && !keep.Keep(seq[p].GID) {
				p++
			}
			if p+glyphsNeeded >= b || l.Input[seq[p].GID] != cls {
				continue ruleLoop
			}
			matchPos = append(matchPos, p)
		}
		next := p

		glyphsNeeded = len(rule.Lookahead)
		for _, cls := range rule.Lookahead {
			glyphsNeeded--
			p++
			for p+glyphsNeeded < len(seq) && !keep.Keep(seq[p].GID) {
				p++
			}
			if p+glyphsNeeded >= len(seq) || l.Lookahead[seq[p].GID] != cls {
				continue ruleLoop
			}
		}

		next++
		for next < b && !keep.Keep(seq[next].GID) {
			next++
		}

		ctx.scratch = nil // claim the scratch space
		ctx.stack = append(ctx.stack, &nested{
			InputPos: matchPos,
			Actions:  rule.Actions,
			EndPos:   next,
		})
		return next
	}

	ctx.scratch = matchPos // release the scratch space
	return -1
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *ChainedSeqContext2) layout() (*ruleSetLayout, error) {
	tables := [][]byte{
		l.Cov.Encode(),
		l.Backtrack.Append(nil),
		l.Input.Append(nil),
		l.Lookahead.Append(nil),
	}
	lay := newRuleSetLayout("ChainedSeqContext2", 2, 4, len(l.Rules))
	err := lay.add(true, tables, func() error {
		return addRuleSets(lay, l.Rules, encodeChainedClassSeqRule)
	})
	return lay, err
}

// encodeLen implements the [Subtable] interface.
func (l *ChainedSeqContext2) encodeLen() int {
	return mustFit(l.layout()).encodeLen()
}

// encode implements the [Subtable] interface.
func (l *ChainedSeqContext2) encode() []byte {
	return mustFit(l.layout()).encode()
}

// ChainedSeqContext3 is used for GSUB type 6 and for GPOS type 8 format 3 subtables.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#chained-sequence-context-format-3-coverage-based-glyph-contexts
type ChainedSeqContext3 struct {
	Backtrack []coverage.Set
	Input     []coverage.Set
	Lookahead []coverage.Set
	Actions   []SeqLookup
}

func readChainedSeqContext3(p *parser.Parser, subtablePos int64) (Subtable, error) {
	backtrackCoverageOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}
	inputCoverageOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}
	lookaheadCoverageOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}
	if len(inputCoverageOffsets) < 1 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    "invalid glyph count in ChainedSeqContext3",
		}
	}

	seqLookupCount, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	actions, err := readNested(p, int(seqLookupCount))
	if err != nil {
		return nil, err
	}

	covCache := readcache.New[coverage.Set]()
	backtrackCov, err := p.Budget.AllocSlice[coverage.Set](len(backtrackCoverageOffsets))
	if err != nil {
		return nil, err
	}
	for i, offset := range backtrackCoverageOffsets {
		backtrackCov[i], err = covCache.Get(subtablePos+int64(offset), coverageSetReader(p))
		if err != nil {
			return nil, err
		}
	}

	inputCov, err := p.Budget.AllocSlice[coverage.Set](len(inputCoverageOffsets))
	if err != nil {
		return nil, err
	}
	for i, offset := range inputCoverageOffsets {
		inputCov[i], err = covCache.Get(subtablePos+int64(offset), coverageSetReader(p))
		if err != nil {
			return nil, err
		}
	}

	lookaheadCov, err := p.Budget.AllocSlice[coverage.Set](len(lookaheadCoverageOffsets))
	if err != nil {
		return nil, err
	}
	for i, offset := range lookaheadCoverageOffsets {
		lookaheadCov[i], err = covCache.Get(subtablePos+int64(offset), coverageSetReader(p))
		if err != nil {
			return nil, err
		}
	}

	res := &ChainedSeqContext3{
		Backtrack: backtrackCov,
		Input:     inputCov,
		Lookahead: lookaheadCov,
		Actions:   actions,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *ChainedSeqContext3) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	p := a
	glyphsNeeded := len(l.Backtrack)
	for _, cov := range l.Backtrack {
		glyphsNeeded--
		p--
		for p-glyphsNeeded >= 0 && !keep.Keep(seq[p].GID) {
			p--
		}
		if p-glyphsNeeded < 0 || !cov[seq[p].GID] {
			return -1
		}
	}

	if !l.Input[0][seq[a].GID] {
		return -1
	}

	p = a
	matchPos := append(ctx.scratch[:0], p)
	glyphsNeeded = len(l.Input) - 1
	for _, cov := range l.Input[1:] {
		glyphsNeeded--
		p++
		for p+glyphsNeeded < b && !keep.Keep(seq[p].GID) {
			p++
		}
		if p+glyphsNeeded >= b || !cov[seq[p].GID] {
			ctx.scratch = matchPos // release the scratch space
			return -1
		}
		matchPos = append(matchPos, p)
	}

	p++
	for p < b && !keep.Keep(seq[p].GID) {
		p++
	}
	next := p

	glyphsNeeded = len(l.Lookahead)
	for _, cov := range l.Lookahead {
		if p+glyphsNeeded-1 >= len(seq) || !cov[seq[p].GID] {
			ctx.scratch = matchPos // return the scratch space
			return -1
		}
		glyphsNeeded--
		p++
		for p+glyphsNeeded < len(seq) && !keep.Keep(seq[p].GID) {
			p++
		}
	}

	ctx.scratch = nil // claim the scratch space as our own
	ctx.stack = append(ctx.stack, &nested{
		InputPos: matchPos,
		Actions:  l.Actions,
		EndPos:   next,
	})
	return next
}

// chainedSeqContext3Layout is the encoded layout of a [ChainedSeqContext3]
// subtable: the header with the coverage offsets and the actions, followed
// by the coverage tables.  Identical tables are stored once.
type chainedSeqContext3Layout struct {
	backtrack, input, lookahead []int // pool indices of the coverage tables
	nActions                    int
	pool                        *offsetpool.Deferred
}

func (lay *chainedSeqContext3Layout) headerLen() int {
	return 10 + 2*(len(lay.backtrack)+len(lay.input)+len(lay.lookahead)) + 4*lay.nActions
}

// layout computes the encoded layout of the subtable.  An error is returned
// if an offset does not fit into an Offset16.
func (l *ChainedSeqContext3) layout() (*chainedSeqContext3Layout, error) {
	pool := offsetpool.NewDeferred("ChainedSeqContext3",
		10+2*(len(l.Backtrack)+len(l.Input)+len(l.Lookahead))+4*len(l.Actions))
	lay := &chainedSeqContext3Layout{nActions: len(l.Actions), pool: pool}
	var err error
	if lay.backtrack, err = addCoverageSets(pool, l.Backtrack); err != nil {
		return nil, err
	}
	if lay.input, err = addCoverageSets(pool, l.Input); err != nil {
		return nil, err
	}
	if lay.lookahead, err = addCoverageSets(pool, l.Lookahead); err != nil {
		return nil, err
	}
	return lay, nil
}

// encodeLen implements the [Subtable] interface.
func (l *ChainedSeqContext3) encodeLen() int {
	lay := mustFit(l.layout())
	return lay.headerLen() + lay.pool.Len()
}

// encode implements the [Subtable] interface.
func (l *ChainedSeqContext3) encode() []byte {
	lay := mustFit(l.layout())
	offs, data := lay.pool.Place()
	buf := make([]byte, 0, lay.headerLen()+len(data))
	buf = appendUint16(buf, 3) // format
	for _, indices := range [][]int{lay.backtrack, lay.input, lay.lookahead} {
		buf = appendUint16(buf, len(indices))
		buf = appendOffsets(buf, offs, indices)
	}
	buf = appendUint16(buf, len(l.Actions))
	buf = appendActions(buf, l.Actions)
	return append(buf, data...)
}

// fits implements the [Subtable] interface.
func (l *SeqContext1) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *SeqContext2) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *SeqContext3) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *ChainedSeqContext1) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *ChainedSeqContext2) fits() error {
	_, err := l.layout()
	return err
}

// fits implements the [Subtable] interface.
func (l *ChainedSeqContext3) fits() error {
	_, err := l.layout()
	return err
}
