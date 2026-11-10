// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package parser

// InvalidFontError indicates a problem with font data.
type InvalidFontError struct {
	SubSystem string
	Reason    string
}

func (err *InvalidFontError) Error() string {
	return err.SubSystem + ": " + err.Reason
}

// NotSupportedError indicates that a font file seems valid but uses a
// CFF feature which is not supported by this library.
type NotSupportedError struct {
	SubSystem string
	Feature   string
}

func (err *NotSupportedError) Error() string {
	return err.SubSystem + ": " + err.Feature + " not supported"
}

// IsUnsupported returns true if the error is a NotSupportedError.
func IsUnsupported(err error) bool {
	_, ok := err.(*NotSupportedError)
	return ok
}
