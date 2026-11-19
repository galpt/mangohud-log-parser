// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"math"
	"strings"

	"github.com/galpt/mangohud-log-parser/internal/compare"
	"github.com/galpt/mangohud-log-parser/internal/mangohud"
)

const (
	timeAxisName = "Time since the first sample (s)"
	// lttb is ECharts' largest-triangle-three-buckets sampling. It keeps the
	// points that shape a line, peaks included, when a line has more points
	// than the plot has pixels across.
	lttb            = "lttb"
	timelineDrawing = "Each line is one run, against the seconds since its first sample. " +
		"The lines are thinned for drawing with largest-triangle-three-buckets (LTTB) sampling, which keeps the peaks. " +
		"Every statistic in the summary images uses all samples."
)

// timelineFigure draws one column of the logs, one line per run that has it.
func timelineFigure(set *compare.Set, c mangohud.Column, sources []string, m Measure) (Figure, error) {
	type curve struct {
		run    int
		label  string
		points [][2]float64
	}
	var curves []curve
	var absent []string
	lo, hi, end := 0.0, 0.0, 0.0
	for i, r := range set.Runs() {
		values := r.Log.Series(c.Name)
		if values == nil {
			absent = append(absent, r.Label)
			continue
		}
		points := make([][2]float64, len(values))
		first := r.Log.Elapsed[0]
		for k, v := range values {
			points[k] = [2]float64{float64(r.Log.Elapsed[k]-first) / 1e9, v}
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		end = math.Max(end, points[len(points)-1][0])
		curves = append(curves, curve{run: i, label: r.Label, points: points})
	}
	x := linearAxis(0, end, curvePlotW, 0, 0)
	y := linearAxis(lo, hi, curvePlotH, 0, 0)

	caption := []string{timelineDrawing}
	// A column that MangoHud is not known to write has no unit, and nothing
	// is known about how often it changes.
	if c.Name != "fps" && c.Name != "frametime" && c.Unit != "" {
		caption = append(caption, "MangoHud refreshes this column about every 500 ms and repeats it on the frames in between.")
	}
	if len(absent) > 0 {
		caption = append(caption, strings.Join(absent, ", ")+" "+verb(len(absent))+" no "+c.Name+" column and no line.")
	}
	legend := make([]string, len(curves))
	for i, cv := range curves {
		legend[i] = cv.label
	}
	title := metricName(c.Label+" over time", c.Unit, c.Direction)
	l, err := layoutChart(chartSpec{
		minPlotW: curvePlotW, axisW: axisWidth(y.ticks(), m), plotH: curvePlotH, footH: footH,
		titleW: titleWidth(title, "", m), legend: legend, caption: append(caption, sources...),
	}, m)
	if err != nil {
		return Figure{}, err
	}
	b := newLineBuilder(l, m)
	b.miss.sampling = lttb
	b.header(title, "")
	b.axes(valueXAxis(x, timeAxisName, "{value}"), valueAxis(y))
	for _, cv := range curves {
		b.addLine(cv.run, cv.label, cv.points)
	}
	return b.finish()
}

func verb(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}
