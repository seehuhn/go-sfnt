// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2021  Jochen Voss <voss@seehuhn.de>
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
