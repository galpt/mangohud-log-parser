// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"cmp"
	"math"
	"slices"

	"github.com/go-echarts/go-echarts/v2/opts"

	"github.com/galpt/mangohud-log-parser/internal/compare"
)

// The frame times are drawn twice. A curve over a linear percentile axis
// shows the bulk of the frames, but the slowest 1% of them, where stutter
// lives and where MangoHud's 1% and 0.1% min FPS come from, would be squeezed
// into the last pixels at the right edge. The tail image therefore puts the
// share of frames that are at least as slow on a log axis, which gives every
// decade from half of the frames down to the single slowest frame the same
// width. The body image covers the rest on a linear axis and stops at the
// 99th percentile, so the spikes of the tail do not flatten it. Both curves
// are exact order statistics of the sorted frame times, with nothing
// interpolated, smoothed or binned.
const (
	tailTitle = "Frame time of the slowest frames (ms) ↓"
	tailAxis  = "Frames at least this slow (% of the run's frames, log scale)"
	tailText  = "Each curve is one run. A point at x % and y ms means that the slowest x % of the run's frames took y ms or longer: " +
		"the k-th slowest of N frames is drawn at x = 100 × k / N, so the right end of a curve is its slowest frame. " +
		"Every one of the 100 slowest frames is drawn, and beyond them the ranks are taken 100 per decade of x, " +
		"all exact values of the sorted frame times, joined by straight lines. " +
		"MangoHud's 1% and 0.1% min FPS are 1000 divided by the frame time at rank max(1, floor(0.01 × N)) and max(1, floor(0.001 × N)), " +
		"which lie at x = 1 and x = 0.1 when N is a multiple of 1000."
	bodyTitle = "Frame time by percentile, fastest frame to 99th percentile (ms) ↓"
	bodyAxis  = "Percentile (% of the run's frames that are this fast or faster)"
	bodyText  = "Each curve is one run. The value at percentile p is the frame time at position ceil(p × N / 100) of the N frame times " +
		"sorted from the fastest, the inverse of the empirical distribution function with no interpolation, " +
		"and position 1 at p = 0. It is drawn every 0.1 percentile and joined by straight lines. " +
		"The curves stop at the 99th percentile, and the slowest 1% are in the image of the slowest frames. " +
		"The y axis does not start at zero."
)

// tailEvery is the number of slowest frames that the tail image draws one by
// one, and tailPerDecade the number of ranks it takes per decade of x beyond
// them. Past rank 100 a step of 1/tailPerDecade of a decade is more than two
// frames, so the curve keeps its detail where every frame counts and stays a
// few hundred points long for any number of frames.
const (
	tailEvery     = 100
	tailPerDecade = 100
)

// bodySteps is the number of steps of 0.1 percentile from 0 to 99.
const bodySteps = 990

// distribution holds the frame times of one run, sorted from the slowest.
type distribution struct {
	run   int
	label string
	desc  []float64
}

func newDistributions(set *compare.Set) []distribution {
	var out []distribution
	for i, r := range set.Runs() {
		desc := slices.Clone(r.Log.Series("frametime"))
		slices.SortFunc(desc, func(a, b float64) int { return cmp.Compare(b, a) })
		out = append(out, distribution{run: i, label: r.Label, desc: desc})
	}
	return out
}

// tailRanks lists the 1-based ranks that the tail image draws for n frames:
// every rank up to tailEvery, then ranks that grow by a factor of
// 10^(1/tailPerDecade), and always n itself, so the curve reaches the fastest
// frame.
func tailRanks(n int) []int {
	step := math.Pow(10, 1.0/tailPerDecade)
	var ranks []int
	for k := 1; k < n; {
		ranks = append(ranks, k)
		if k < tailEvery {
			k++
		} else {
			k = max(k+1, int(math.Ceil(float64(k)*step)))
		}
	}
	return append(ranks, n)
}

// bodyIndex is the 0-based position, in frame times sorted from the fastest,
// of percentile j/10 out of n frames: ceil(j × n / 1000) − 1, and 0 for j = 0. The
// integer arithmetic keeps it exact.
func bodyIndex(j, n int) int {
	return max(0, (j*n+999)/1000-1)
}

func runLegend(ds []distribution) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.label
	}
	return out
}

// tailFigure draws the slow tail of every run's frame times.
func tailFigure(ds []distribution, sources []string, m Measure) (Figure, error) {
	curves := make([][][2]float64, len(ds))
	lo, hi, minX := math.Inf(1), 0.0, 100.0
	for i, d := range ds {
		n := len(d.desc)
		for _, k := range tailRanks(n) {
			curves[i] = append(curves[i], [2]float64{100 * float64(k) / float64(n), d.desc[k-1]})
		}
		lo, hi = math.Min(lo, d.desc[n-1]), math.Max(hi, d.desc[0])
		minX = math.Min(minX, 100/float64(n))
	}
	// The axis spans at least one decade, which a run of a single frame would
	// not reach on its own.
	x := axisRange{log: true, min: math.Min(math.Pow(10, math.Floor(math.Log10(minX))), 10), max: 100}
	var y axisRange
	mark := "log x axis"
	if spansDecades(lo, hi) {
		y = logAxis(lo, hi, curvePlotH, 0)
		mark = "log x and y axes"
	} else {
		y = linearAxis(lo, hi, curvePlotH, 0, 0)
	}
	xAxis := valueXAxis(x, tailAxis, "{value}%")
	// The slowest frames are on the right, where a reader looks for the end
	// of a curve.
	xAxis.Inverse = opts.Bool(true)
	return curveFigure(ds, curves, tailTitle, mark, xAxis, y, tailText, sources, m)
}

// bodyFigure draws every run's frame times from the fastest frame to the
// 99th percentile.
func bodyFigure(ds []distribution, sources []string, m Measure) (Figure, error) {
	curves := make([][][2]float64, len(ds))
	lo, hi := math.Inf(1), math.Inf(-1)
	for i, d := range ds {
		n := len(d.desc)
		for j := 0; j <= bodySteps; j++ {
			// The k-th fastest frame is the (n-k+1)-th slowest.
			v := d.desc[n-1-bodyIndex(j, n)]
			curves[i] = append(curves[i], [2]float64{float64(j) / 10, v})
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	x := axisRange{min: 0, max: 100, step: 10}
	return curveFigure(ds, curves, bodyTitle, "", valueXAxis(x, bodyAxis, "{value}"), fittedAxis(lo, hi), bodyText, sources, m)
}

// curveFigure lays out an image with one line per run.
func curveFigure(ds []distribution, curves [][][2]float64, title, mark string, x opts.XAxis, y axisRange, text string, sources []string, m Measure) (Figure, error) {
	l, err := layoutChart(chartSpec{
		minPlotW: curvePlotW, axisW: axisWidth(y.ticks(), m), plotH: curvePlotH, footH: footH,
		titleW: titleWidth(title, mark, m), legend: runLegend(ds), caption: append([]string{text}, sources...),
	}, m)
	if err != nil {
		return Figure{}, err
	}
	b := newLineBuilder(l, m)
	b.header(title, mark)
	b.axes(x, valueAxis(y))
	for i, d := range ds {
		b.addLine(d.run, d.label, curves[i])
	}
	return b.finish()
}
