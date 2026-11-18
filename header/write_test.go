// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package header

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"
)

// TestWriteSkipsNilTables checks that tables with nil data are neither
// written nor counted in the table directory.
func TestWriteSkipsNilTables(t *testing.T) {
	tables := map[string][]byte{
		"head": make([]byte, 54),
		"cmap": {1, 2, 3, 4},
		"glyf": {},
		"VORG": nil,
		"vmtx": nil,
	}
	buf := &bytes.Buffer{}
	if _, err := Write(buf, ScalerTypeTrueType, tables); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()

	if got := binary.BigEndian.Uint16(data[4:]); got != 3 {
		t.Errorf("numTables = %d, want 3", got)
	}

	info, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range info.Toc {
		names = append(names, name)
	}
	slices.Sort(names)
	if want := []string{"cmap", "glyf", "head"}; !slices.Equal(names, want) {
		t.Errorf("tables = %q, want %q", names, want)
	}

	// each table is found at the offset and length the directory gives;
	// Write has patched the checksum into the caller's "head" slice
	for _, name := range names {
		rec := info.Toc[name]
		end := int(rec.Offset) + int(rec.Length)
		if end > len(data) {
			t.Errorf("table %q extends beyond the end of the file", name)
			continue
		}
		if got := data[rec.Offset:end]; !bytes.Equal(got, tables[name]) {
			t.Errorf("table %q = %v, want %v", name, got, tables[name])
		}
	}
}

// TestWriteInvalidName checks that a table name which is not a valid tag is
// rejected, whether or not the table has data.
func TestWriteInvalidName(t *testing.T) {
	invalid := []string{
		"bad", "toolong", "äö", "ab\x00c", "ab\x7fc", "a b ", " abc", "    ",
	}
	for _, name := range invalid {
		for _, data := range [][]byte{{1, 2, 3, 4}, nil} {
			tables := map[string][]byte{
				"cmap": {5, 6, 7, 8},
				name:   data,
			}
			if _, err := Write(&bytes.Buffer{}, ScalerTypeTrueType, tables); err == nil {
				t.Errorf("%q, data %v: expected an error", name, data)
			}
		}
	}
}

// TestWriteNoTables checks that writing a font without tables fails.
func TestWriteNoTables(t *testing.T) {
	for _, tables := range []map[string][]byte{{}, {"cmap": nil}} {
		if _, err := Write(&bytes.Buffer{}, ScalerTypeTrueType, tables); err == nil {
			t.Errorf("%v: expected an error", tables)
		}
	}
}

// TestWriteSpacePadding checks that tags padded with trailing spaces are
// written.
func TestWriteSpacePadding(t *testing.T) {
	tables := map[string][]byte{
		"cvt ": {1, 2, 3, 4},
		"ab  ": {5, 6, 7, 8},
		"a   ": {9, 10, 11, 12},
	}
	buf := &bytes.Buffer{}
	if _, err := Write(buf, ScalerTypeTrueType, tables); err != nil {
		t.Fatal(err)
	}
	info, err := Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for name := range tables {
		if _, ok := info.Toc[name]; !ok {
			t.Errorf("table %q missing", name)
		}
	}
}
