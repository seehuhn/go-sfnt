// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cff

import "seehuhn.de/go/sfnt/parser"

func unsupported(feature string) error {
	return &parser.NotSupportedError{
		SubSystem: "cff",
		Feature:   feature,
	}
}

func invalidSince(reason string) error {
	return &parser.InvalidFontError{
		SubSystem: "cff",
		Reason:    reason,
	}
}
