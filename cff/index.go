// Copyright 2021 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cff

import (
	"bytes"
	"errors"
	"fmt"

	"seehuhn.de/go/sfnt/parser"
)

// cffIndex is a CFF INDEX, i.e. an ordered sequence of binary blobs.
type cffIndex [][]byte

func readIndexAt(p *parser.Parser, pos int64, name string) (cffIndex, error) {
	if pos < 4 || pos >= p.Size() {
		return nil, errors.New("cff: missing " + name + " INDEX")
	}
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	return readIndex(p)
}

func readIndex(p *parser.Parser) (cffIndex, error) {
	count, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	return readIndexBody(p, int(count))
}

// readIndex32 reads a CFF2 INDEX, which uses a uint32 count instead of the
// uint16 count of a CFF1 INDEX.  An empty CFF2 INDEX is four zero bytes.
func readIndex32(p *parser.Parser) (cffIndex, error) {
	count, err := p.ReadUint32()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	// bound count before allocating: each of the count+1 offsets needs at
	// least one byte, so count cannot exceed the remaining input
	if int64(count) > p.Size()-p.Pos() {
		return nil, invalidSince("INDEX count exceeds input size")
	}
	return readIndexBody(p, int(count))
}

// readIndexBody reads the offSize, offset array and object data of an INDEX,
// given the already-read object count.  It is shared by the CFF1 (readIndex)
// and CFF2 (readIndex32) readers.
func readIndexBody(p *parser.Parser, count int) (cffIndex, error) {
	offSize, err := p.ReadUint8()
	if err != nil {
		return nil, err
	}

	offsets := make([]uint32, 0, count+1)
	prevOffset := uint32(1)
	size := p.Size()
	for i := 0; i <= count; i++ {
		blob, err := p.ReadBytes(int(offSize))
		if err != nil {
			return nil, err
		}

		var offs uint32
		for _, x := range blob {
			offs = offs<<8 | uint32(x)
		}
		if offs < prevOffset || int64(offs) >= size {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/cff",
				Reason:    "invalid INDEX offset",
			}
		}
		offsets = append(offsets, offs-1)
		prevOffset = offs
	}

	buf := make([]byte, offsets[count])
	_, err = p.Read(buf)
	if err != nil {
		return nil, err
	}

	res := make([][]byte, count)
	for i := range count {
		res[i] = buf[offsets[i]:offsets[i+1]]
	}

	return res, nil
}

// emptyIndex returns the binary representation of an empty CFF INDEX.
func emptyIndex() []byte {
	return []byte{0, 0}
}

// emptyIndex32 returns the binary representation of an empty CFF2 INDEX.
func emptyIndex32() []byte {
	return []byte{0, 0, 0, 0}
}

// encode converts a CFF INDEX to its binary representation.  An error is
// returned if the INDEX has 65536 or more items, or holds too much data for
// 32-bit offsets.
func (data cffIndex) encode() ([]byte, error) {
	count := len(data)
	if count >= 1<<16 {
		return nil, fmt.Errorf("cff: too many items for INDEX (%d)", count)
	}
	if count == 0 {
		return emptyIndex(), nil
	}

	out := &bytes.Buffer{}
	out.Write([]byte{byte(count >> 8), byte(count)}) // count
	if err := data.encodeBody(out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// encode32 converts a CFF2 INDEX to its binary representation, using a
// uint32 count.  An empty CFF2 INDEX encodes as four zero bytes.  An error
// is returned if the INDEX holds too much data for 32-bit offsets.
func (data cffIndex) encode32() ([]byte, error) {
	count := len(data)
	if count == 0 {
		return emptyIndex32(), nil
	}

	out := &bytes.Buffer{}
	out.Write([]byte{
		byte(count >> 24), byte(count >> 16), byte(count >> 8), byte(count), // count
	})
	if err := data.encodeBody(out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// encodeBody writes the offSize, offset array and object data of an INDEX,
// i.e. everything after the count.  Shared by encode and encode32.  An error
// is returned if the data is too large for 32-bit offsets.
func (data cffIndex) encodeBody(out *bytes.Buffer) error {
	count := len(data)

	bodyLength := 0
	for _, blob := range data {
		bodyLength += len(blob)
	}

	offSize := 1
	for bodyLength+1 >= 1<<(8*offSize) {
		offSize++
	}
	if offSize > 4 {
		return errors.New("cff: too much data for INDEX")
	}

	out.WriteByte(byte(offSize))

	// offset
	var offsetBuf [4]byte
	pos := uint32(1)
	for i := 0; i <= count; i++ {
		for j := 0; j < offSize; j++ {
			offsetBuf[j] = byte(pos >> (8 * (offSize - j - 1)))
		}
		out.Write(offsetBuf[:offSize])
		if i < count {
			pos += uint32(len(data[i]))
		}
	}

	// data
	for i := range count {
		out.Write(data[i])
	}
	return nil
}
