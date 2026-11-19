// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/galpt/mangohud-log-parser/internal/compare"
	"github.com/galpt/mangohud-log-parser/internal/mangohud"
	"github.com/galpt/mangohud-log-parser/internal/render"
)

const (
	// quickFile is the one image that is not in a directory of its own. It
	// sits at the top of the output, where a reader looks first.
	quickFile  = "quick-summary.png"
	quickTitle = "Quick summary"
	// quickHeaderW is the width in px that a run label is wrapped to in the
	// header, so that one long name does not widen its column past the values
	// under it.
	quickHeaderW = 150
)

// Caption sentences of the quick summary.
const (
	quickBold       = "Bold marks the best run of each FPS row, and runs that show the same value are all bold."
	quickDefinition = "The statistics are defined as in the summary images, which have one image per statistic."
	quickNA         = "n/a marks a run whose log has no column for the statistic."
)

// quickStats lists the rows of the quick summary in the order they are drawn.
// A row is a statistic of compare.Set.Metrics, found by its key. name is how
// the caption calls the row when no run has the statistic, so the average and
// the peak of one column share a name.
var quickStats = []struct{ key, name string }{
	{"avg_fps", "Average FPS"},
	{"min_fps_1pct", "1% min FPS"},
	{"min_fps_0_1pct", "0.1% min FPS"},
	{"percentile_fps_97", "97% percentile FPS"},
	{"avg_cpu_mhz", "CPU clock"},
	{"peak_cpu_mhz", "CPU clock"},
	{"avg_gpu_core_clock", "GPU core clock"},
	{"peak_gpu_core_clock", "GPU core clock"},
	{"avg_cpu_power", "CPU power"},
	{"peak_cpu_power", "CPU power"},
	{"avg_gpu_power", "GPU power"},
	{"peak_gpu_power", "GPU power"},
}

// bestCells marks the cells of a statistic that hold the best value. Only a
// statistic where higher is better has one. Values are compared as shown, by
// reading the text of the cell back, so two runs that print the same number
// are both best even when their values differ in a digit that is not shown.
// With fewer than two values there is nothing to compare and no cell is
// marked.
func bestCells(mt compare.Metric) []bool {
	best := make([]bool, len(mt.Cells))
	if mt.Direction != mangohud.HigherIsBetter {
		return best
	}
	shown := make([]float64, len(mt.Cells))
	top, count := math.Inf(-1), 0
	for i, c := range mt.Cells {
		if !c.Valid {
			continue
		}
		// The text came from strconv.FormatFloat, which ParseFloat reads back.
		shown[i], _ = strconv.ParseFloat(c.Text, 64)
		top = math.Max(top, shown[i])
		count++
	}
	if count < 2 {
		return best
	}
	for i, c := range mt.Cells {
		best[i] = c.Valid && shown[i] == top
	}
	return best
}

// quickTable is the table of the quick summary, one column per run and one
// row per statistic that at least one run has. It also returns the names of
// the statistics that were left out because no run has them.
func quickTable(set *compare.Set, m Measure) (tb table, leftOut []string) {
	byKey := map[string]compare.Metric{}
	for _, mt := range set.Metrics() {
		byKey[mt.Key] = mt
	}
	tb.header = []string{"Statistic"}
	for _, label := range runLabels(set) {
		tb.header = append(tb.header, strings.Join(wrap(label, quickHeaderW, bodySize, true, m), "\n"))
	}
	for _, st := range quickStats {
		mt, ok := byKey[st.key]
		if !ok {
			leftOut = append(leftOut, st.name)
			continue
		}
		row := []string{metricName(mt.Label, mt.Unit, mt.Direction)}
		bold := []bool{false}
		best := bestCells(mt)
		for i, c := range mt.Cells {
			text := naText
			if c.Valid {
				text = c.Text
			}
			row = append(row, text)
			bold = append(bold, best[i])
		}
		tb.rows = append(tb.rows, row)
		tb.bold = append(tb.bold, bold)
	}
	return tb, slices.Compact(leftOut)
}

// quickFigure draws the main statistics of every run in one table, so that a
// reader can compare the runs without opening the summary directory.
func quickFigure(set *compare.Set, sources []string, m Measure) (Figure, error) {
	tb, leftOut := quickTable(set, m)
	colW := tb.widths(m)
	tableW := 0.0
	for _, w := range colW {
		tableW += w
	}
	inner := math.Max(tableW, math.Max(minWidth-2*margin, m(quickTitle, titleSize, true)))
	width := int(math.Ceil(2*margin + inner))
	if width > render.MaxSide {
		return Figure{}, fmt.Errorf("the quick summary needs an image %d px wide, more than the %d px that render allows", width, render.MaxSide)
	}

	var caption []string
	if slices.ContainsFunc(tb.bold, func(row []bool) bool { return slices.Contains(row, true) }) {
		caption = append(caption, quickBold)
	}
	caption = append(caption, quickDefinition)
	if slices.ContainsFunc(tb.rows, func(row []string) bool { return slices.Contains(row, naText) }) {
		caption = append(caption, quickNA)
	}
	if len(leftOut) > 0 {
		caption = append(caption, "Left out because no log has the column: "+strings.Join(leftOut, ", ")+".")
	}
	caption = append(caption, sources...)

	p := newPage(quickTitle, inner, m)
	p.table(tb, colW)
	p.text.y += sectionGap
	for _, c := range caption {
		p.text.paragraph(c, captionSize)
	}
	height := int(math.Ceil(p.text.y + margin))
	if height > render.MaxSide {
		return Figure{}, fmt.Errorf("the quick summary needs an image %d px tall, more than the %d px that render allows", height, render.MaxSide)
	}
	return p.finish(quickFile, width, height)
}
