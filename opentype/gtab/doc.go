// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package gtab reads and writes OpenType "GSUB" and "GPOS" tables.
//
// There are two main entry points:
//
//   - The [Info] type represents a parsed OpenType "GSUB" or "GPOS" table.
//   - The [Context] type allows to apply a "GSUB" or "GPOS" table to a sequence
//     of glyphs.
package gtab
