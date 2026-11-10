// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package builder

import (
	"testing"
)

func TestLexInteger(t *testing.T) {
	for _, in := range []string{"0", "123", "-123", "+123"} {
		_, c := lex(in)

		if x := <-c; x.typ != itemInteger || x.val != in {
			t.Errorf("%q -> itemInteger expected, got %s(%d)", in, x, x.typ)
		}
		if x := <-c; x.typ != itemEOF {
			t.Error("itemEOF expected")
		}
		if _, ok := <-c; ok {
			t.Error("channel should be closed")
		}
	}
}

func TestLexBackup(t *testing.T) {
	l := &lexer{
		input: "abc",
	}

	var out []rune
	for {
		r := l.next()
		l.backup()
		r2 := l.next()
		if r != r2 {
			t.Errorf("%q != %q", r, r2)
		}
		if r == 0 {
			break
		}
		out = append(out, r)
	}
	if string(out) != l.input {
		t.Errorf("%q != %q", out, l.input)
	}
}
