// Copyright 2021 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package header

// ErrMissing indicates that a required table is missing from a TrueType or
// OpenType font file.
type ErrMissing struct {
	TableName string
}

func (err *ErrMissing) Error() string {
	return "sfnt: missing " + err.TableName + " table"
}

// IsMissing returns true, if err indicates a missing sfnt table.
func IsMissing(err error) bool {
	_, missing := err.(*ErrMissing)
	return missing
}
