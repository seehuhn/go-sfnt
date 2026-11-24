// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package mapfilter restricts maps to a subset of their keys.
package mapfilter

// Keys returns the entries of m whose keys satisfy keep.  The second result
// reports whether any entry was removed.  If not, m itself is returned,
// otherwise the result is a new map.
func Keys[M ~map[K]V, K comparable, V any](m M, keep func(K) bool) (M, bool) {
	numKept := 0
	for key := range m {
		if keep(key) {
			numKept++
		}
	}
	if numKept == len(m) {
		return m, false
	}
	res := make(M, numKept)
	for key, val := range m {
		if keep(key) {
			res[key] = val
		}
	}
	return res, true
}
