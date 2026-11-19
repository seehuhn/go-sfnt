// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package header

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"sort"
)

// Write writes an sfnt file containing the given tables.
// Table names must be valid sfnt tags: one to four printable ASCII
// characters (0x21 to 0x7E), padded with trailing spaces to four bytes.
// Tables where the data is nil are not written; use a zero-length slice
// to write a table with no data.  At least one table must be written.
// Write returns an error if these conditions are not met.
// This changes the checksum in the "head" table in place.
func Write(w io.Writer, scalerType uint32, tables map[string][]byte) (int64, error) {
	tableNames := make([]string, 0, len(tables))
	for name, data := range tables {
		if !validTag(name) {
			return 0, fmt.Errorf("sfnt: invalid table name %q", name)
		}
		if data != nil {
			tableNames = append(tableNames, name)
		}
	}
	numTables := len(tableNames)
	if numTables == 0 {
		return 0, errors.New("sfnt: no tables to write")
	}

	// sort the table names in the recommended order
	sort.Slice(tableNames, func(i, j int) bool {
		iPrio := ttTableOrder[tableNames[i]]
		jPrio := ttTableOrder[tableNames[j]]
		if iPrio != jPrio {
			return iPrio > jPrio
		}
		return tableNames[i] < tableNames[j]
	})

	// prepare the header
	entrySelector := bits.Len(uint(numTables)) - 1
	header := &offsets{
		ScalerType:    scalerType,
		NumTables:     uint16(numTables),
		SearchRange:   1 << (entrySelector + 4),
		EntrySelector: uint16(entrySelector),
		RangeShift:    uint16(16 * (numTables - 1<<entrySelector)),
	}

	// temporarily clear the checksum in the "head" table
	if headData := tables["head"]; headData != nil {
		clearChecksum(headData)
	}

	var totalSum uint32
	offset := uint32(12 + 16*numTables)
	records := make([]rawRecord, numTables)
	for i, name := range tableNames {
		body := tables[name]
		length := uint32(len(body))
		sum := checksum(body)

		records[i].Tag = tag{name[0], name[1], name[2], name[3]}
		records[i].CheckSum = sum
		records[i].Offset = offset
		records[i].Length = length

		totalSum += sum
		offset += 4 * ((length + 3) / 4)
	}
	sort.Slice(records, func(i, j int) bool {
		return bytes.Compare(records[i].Tag[:], records[j].Tag[:]) < 0
	})

	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.BigEndian, header)
	_ = binary.Write(buf, binary.BigEndian, records)
	headerBytes := buf.Bytes()
	totalSum += checksum(headerBytes)

	// set the final checksum in the "head" table
	if headData := tables["head"]; headData != nil {
		patchChecksum(headData, totalSum)
	}

	// write the tables
	var totalSize int64
	n, err := w.Write(headerBytes)
	totalSize += int64(n)
	if err != nil {
		return totalSize, err
	}
	var pad [3]byte
	for _, name := range tableNames {
		body := tables[name]
		n, err := w.Write(body)
		totalSize += int64(n)
		if err != nil {
			return totalSize, err
		}
		if k := n % 4; k != 0 {
			l, err := w.Write(pad[:4-k])
			totalSize += int64(l)
			if err != nil {
				return totalSize, err
			}
		}
	}
	return totalSize, nil
}

// clearChecksum zeros the checksum field of the head table.
func clearChecksum(head []byte) {
	binary.BigEndian.PutUint32(head[8:12], 0)
}

// patchChecksum updates the checksum of the head table.
// The argument is the checksum of the entire font before patching.
func patchChecksum(head []byte, sum uint32) {
	binary.BigEndian.PutUint32(head[8:12], 0xB1B0AFBA-sum)
}

// The offsets sub-table forms the first part of Header.
type offsets struct {
	ScalerType    uint32
	NumTables     uint16
	SearchRange   uint16
	EntrySelector uint16
	RangeShift    uint16
}

// A rawRecord is part of the file Header.  It contains data about a single
// sfnt table.
type rawRecord struct {
	Tag      tag
	CheckSum uint32
	Offset   uint32
	Length   uint32
}

// tag represents a tag string composed of 4 ASCII bytes
type tag [4]byte

// validTag reports whether name is a valid sfnt tag: one to four printable
// ASCII characters, padded with trailing spaces to four bytes.
func validTag(name string) bool {
	if len(name) != 4 || name[0] == ' ' {
		return false
	}
	padding := false
	for i := range 4 {
		switch c := name[i]; {
		case c == ' ':
			padding = true
		case c < 0x20 || c > 0x7e || padding:
			return false
		}
	}
	return true
}

// https://docs.microsoft.com/en-us/typography/opentype/spec/recom#optimized-table-ordering
var ttTableOrder = map[string]int{
	"head": 95,
	"hhea": 90,
	"vhea": 88,
	"maxp": 85,
	"OS/2": 80,
	"hmtx": 75,
	"vmtx": 74,
	"VORG": 73,
	"LTSH": 70,
	"VDMX": 65,
	"hdmx": 60,
	"cmap": 55,
	"fpgm": 50,
	"prep": 45,
	"cvt ": 40,
	"loca": 35,
	"glyf": 30,
	"kern": 25,
	"name": 20,
	"post": 15,
	"gasp": 10,
	"DSIG": 5,
}
