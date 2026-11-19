// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

// Package summary computes the per-run statistics of a MangoHud log.
//
// Nineteen of the statistics are the ones MangoHud writes to its own
// _summary.csv file: the three percentiles, the average FPS and frame time,
// and the means and peaks of the loads, temperatures and memory. They are
// calculated the way src/logging.cpp and src/fps_metrics.h do it, over every
// sample of the log. The rest are the same means and peaks for the other known
// columns the log has, so a clock or a power draw can be compared between runs
// like the loads and temperatures.
//
// MangoHud keeps the temperature sums and the average temperatures in
// integers, which truncates the average. Here the average temperature is the
// true mean. This is the one place where a result differs from MangoHud on
// purpose. Sums are also kept in float64 instead of float32, so the last
// printed digit can differ from a _summary.csv, which was made from values
// that the log rounded to six significant digits.
package summary

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/galpt/mangohud-log-parser/internal/mangohud"
)

// Stat is one statistic of one run.
//
// Key names the statistic in a stable way and Order is its place in the one
// order that every log shares, so that statistics of different runs line up
// even when a run lacks some of them. Decimals is how many digits after the
// decimal point a chart should print.
type Stat struct {
	Key       string
	Label     string
	Unit      string
	Direction mangohud.Direction
	Decimals  int
	Order     int
	Value     float64
}

// Text formats the value with the decimals of the statistic.
func (s Stat) Text() string {
	return strconv.FormatFloat(s.Value, 'f', s.Decimals, 64)
}

// Result holds the statistics of one log. A statistic whose column the log does
// not have is not in Stats. Stats is sorted by Order.
type Result struct {
	Samples  int
	Duration time.Duration
	Stats    []Stat
}

// The percentiles MangoHud reports, as the float32 values it parses from the
// strings "0.001", "0.01" and "0.97".
const (
	pctLow  float32 = 0.001
	pctMid  float32 = 0.01
	pctHigh float32 = 0.97
)

// Orders of the statistics that come from the frame times. The statistics of
// the other columns follow, two for each entry of meanPeakColumns.
const (
	orderAvgFPS = iota
	orderMin1Pct
	orderMin01Pct
	orderP97
	orderAvgFrametime
	orderColumns
)

// meanPeakColumns lists the columns that get a mean and a peak, in the order
// the statistics are reported: loads, temperatures, memory, power and clocks.
// The frame time columns are left out because their statistics are the FPS
// ones above.
var meanPeakColumns = []string{
	"cpu_load", "gpu_load",
	"cpu_temp", "gpu_temp",
	"gpu_vram_used", "ram_used", "swap_used", "process_rss",
	"cpu_power", "gpu_power",
	"cpu_mhz", "gpu_core_clock", "gpu_mem_clock",
}

// Compute calculates the statistics of l. A log that Parse accepted has the
// columns and samples this needs. For a log that was built some other way and
// has none, the Result has no statistics.
func Compute(l *mangohud.Log) *Result {
	index := map[string]int{}
	for i, c := range l.Columns {
		index[c.Name] = i
	}
	at, ok := index["frametime"]
	if !ok || l.Len() == 0 || len(l.Data[at]) != l.Len() {
		return &Result{}
	}
	ft := l.Data[at]
	r := &Result{Samples: l.Len(), Duration: time.Duration(l.Elapsed[l.Len()-1] - l.Elapsed[0])}

	sorted := slices.Clone(ft)
	slices.SortFunc(sorted, func(a, b float64) int { return cmp.Compare(b, a) })
	fps := func(p float32) float64 { return 1000 / sorted[percentileIndex(p, len(sorted))] }
	meanFT := mean(ft)

	r.add(Stat{Key: "avg_fps", Label: "Average FPS", Direction: mangohud.HigherIsBetter, Order: orderAvgFPS, Value: 1000 / meanFT})
	r.add(Stat{Key: "min_fps_1pct", Label: "1% min FPS", Direction: mangohud.HigherIsBetter, Order: orderMin1Pct, Value: fps(pctMid)})
	r.add(Stat{Key: "min_fps_0_1pct", Label: "0.1% min FPS", Direction: mangohud.HigherIsBetter, Order: orderMin01Pct, Value: fps(pctLow)})
	r.add(Stat{Key: "percentile_fps_97", Label: "97% percentile FPS", Direction: mangohud.HigherIsBetter, Order: orderP97, Value: fps(pctHigh)})
	r.add(Stat{Key: "avg_frametime", Label: "Average frame time", Unit: "ms", Direction: mangohud.LowerIsBetter, Order: orderAvgFrametime, Value: meanFT})

	for i, name := range meanPeakColumns {
		at, ok := index[name]
		if !ok {
			continue
		}
		col, vals := l.Columns[at], l.Data[at]
		order := orderColumns + 2*i
		r.add(Stat{Key: "avg_" + name, Label: "Average " + col.Label, Unit: col.Unit, Order: order, Value: mean(vals)})
		r.add(Stat{Key: "peak_" + name, Label: "Peak " + col.Label, Unit: col.Unit, Order: order + 1, Value: slices.Max(vals)})
	}
	return r
}

// add appends a statistic and gives it the precision of its unit.
func (r *Result) add(s Stat) {
	switch s.Unit {
	case "ms", "GiB":
		s.Decimals = 2
	case "MHz":
		s.Decimals = 0
	default:
		s.Decimals = 1
	}
	r.Stats = append(r.Stats, s)
}

// percentileIndex is the position MangoHud reads from a list of n frame times
// that is sorted from the slowest frame to the fastest. The product is
// rounded to float32 as C++ does for a float times a size_t. The explicit
// conversion also stops Go from fusing the multiplication with the
// subtraction, which would skip that rounding. A product below 1 gives an
// index of 0, because the conversion to an integer truncates toward zero.
func percentileIndex(p float32, n int) int64 {
	k := float32(p * float32(n))
	return int64(k - 1)
}

// mean sums in float64, where MangoHud sums in float32.
func mean(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}
