// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package header

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"
)

// TestWriteSkipsNilTables checks that tables with nil data, and table names
// which are not four bytes long, are neither written nor counted in the
// table directory.
func TestWriteSkipsNilTables(t *testing.T) {
	tables := map[string][]byte{
		"head": make([]byte, 54),
		"cmap": {1, 2, 3, 4},
		"glyf": {},
		"VORG": nil,
		"vmtx": nil,
		"bad":  {5, 6, 7, 8},
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

// TestWriteNilHead checks that a nil "head" entry is skipped rather than
// patched.
func TestWriteNilHead(t *testing.T) {
	tables := map[string][]byte{
		"head": nil,
		"cmap": {1, 2, 3, 4},
	}
	if _, err := Write(&bytes.Buffer{}, ScalerTypeTrueType, tables); err != nil {
		t.Fatal(err)
	}
}
