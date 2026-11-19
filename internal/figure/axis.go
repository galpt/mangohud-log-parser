// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"math"
	"strconv"
	"strings"
)

// logRatio is the spread of values above which a plot switches to a log
// axis. Below it a linear axis still shows the smallest value at a readable
// height.
const logRatio = 20

// axisRange is an axis whose ends are chosen here rather than by ECharts. The
// figure has to know the ends in advance, both to keep room for the value
// labels above the bars and to measure the tick labels it reserves space for.
// For a linear axis step is the tick interval. A log axis has its ticks at the
// powers of ten between min and max and leaves step zero.
type axisRange struct {
	log            bool
	min, max, step float64
}

// spansDecades reports whether positive values from lo to hi call for a log
// axis. A value that is not positive has no place on a log axis, so then the
// answer is no.
func spansDecades(lo, hi float64) bool {
	return lo > 0 && hi/lo >= logRatio
}

// linearAxis returns an axis that includes zero and [lo, hi], with at least
// head px above hi and foot px below lo when lo is negative. The ends are
// multiples of a 1-2-5 step that gives about five intervals.
func linearAxis(lo, hi, plotH, head, foot float64) axisRange {
	lo, hi = math.Min(lo, 0), math.Max(hi, 0)
	if hi == lo {
		hi = 1
	}
	step := niceStep((hi - lo) / 5)
	a := axisRange{step: step, min: math.Floor(lo/step) * step, max: math.Ceil(hi/step) * step}
	for (a.max-hi)/(a.max-a.min)*plotH < head {
		a.max += step
	}
	for lo < 0 && (lo-a.min)/(a.max-a.min)*plotH < foot {
		a.min -= step
	}
	a.min, a.max = roundTo(a.min, step), roundTo(a.max, step)
	return a
}

// fittedAxis returns an axis around [lo, hi] that need not include zero, for
// a plot of lines where the differences between them matter more than their
// distance from zero. The ends are multiples of a 1-2-5 step that gives about
// five intervals.
func fittedAxis(lo, hi float64) axisRange {
	if hi == lo {
		lo, hi = lo-1, hi+1
	}
	step := niceStep((hi - lo) / 5)
	return axisRange{step: step, min: roundTo(math.Floor(lo/step)*step, step), max: roundTo(math.Ceil(hi/step)*step, step)}
}

// logAxis returns an axis from the power of ten at or below lo to one above
// hi with head px to spare. Both lo and hi must be positive.
func logAxis(lo, hi, plotH, head float64) axisRange {
	a := axisRange{log: true, min: math.Pow(10, math.Floor(math.Log10(lo))), max: math.Pow(10, math.Ceil(math.Log10(hi)))}
	for a.max <= a.min || math.Log10(a.max/hi)/math.Log10(a.max/a.min)*plotH < head {
		a.max *= 10
	}
	return a
}

// niceStep rounds a raw interval up to 1, 2 or 5 times a power of ten.
func niceStep(raw float64) float64 {
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, f := range []float64{1, 2, 5} {
		if f*p >= raw*(1-1e-9) {
			return f * p
		}
	}
	return 10 * p
}

// roundTo removes the binary noise that repeated additions of step leave, so
// that 0.30000000000000004 reaches ECharts as 0.3.
func roundTo(v, step float64) float64 {
	d := decimals(step)
	r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', d, 64), 64)
	return r
}

// decimals is the number of digits after the point that a multiple of step
// needs, which is also how ECharts prints its tick labels.
func decimals(step float64) int {
	return max(0, int(math.Ceil(-math.Log10(step)-1e-9)))
}

// ticks returns the tick labels of an axis as ECharts prints them, with
// thousands separators, so that the space for them can be measured.
func (a axisRange) ticks() []string {
	var out []string
	if a.log {
		for v := a.min; v <= a.max*(1+1e-9); v *= 10 {
			out = append(out, tickText(v, decimals(v)))
		}
		return out
	}
	d := decimals(a.step)
	for i := 0; ; i++ {
		v := a.min + float64(i)*a.step
		if v > a.max+a.step*1e-9 {
			return out
		}
		out = append(out, tickText(v, d))
	}
}

func tickText(v float64, decimals int) string {
	s := strconv.FormatFloat(math.Abs(v), 'f', decimals, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	if v < 0 && strings.Trim(s, "0.") != "" {
		b.WriteByte('-')
	}
	for i := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(intPart[i])
	}
	if frac != "" {
		b.WriteString("." + frac)
	}
	return b.String()
}
