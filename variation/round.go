// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package variation

import "math"

// OTRound rounds x to the nearest integer, following the OpenType Font
// Variations rounding convention: ties round towards positive infinity
// (OTRound(-3.5) == -3, OTRound(3.5) == 4), unlike [math.Round] which rounds
// ties away from zero.  Instantiation must use this convention when
// converting accumulated variation deltas to integer font units, or the
// result diverges from other implementations (including fontTools) at exact
// half-integer deltas.
func OTRound(x float64) float64 {
	return math.Floor(x + 0.5)
}
