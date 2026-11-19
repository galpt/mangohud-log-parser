// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package mangohud

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var errBadNumber = errors.New("bad number")

// parseNumber reads a decimal float the way the C++ default ostream prints
// one, which includes forms such as "1e+06", "inf" and "nan". Go's ParseFloat
// is wider than that: it takes hex floats and underscores, which would let a
// corrupted cell through, and it refuses a signed NaN that glibc prints as
// "-nan".
func parseNumber(s string) (float64, error) {
	if strings.ContainsAny(s, "xXpP_") {
		return 0, errBadNumber
	}
	unsigned := s
	if s != "" && (s[0] == '+' || s[0] == '-') {
		unsigned = s[1:]
	}
	if strings.EqualFold(unsigned, "nan") {
		return math.NaN(), nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, errBadNumber
	}
	return v, nil
}
