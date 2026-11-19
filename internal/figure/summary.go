// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"

	"github.com/galpt/mangohud-log-parser/internal/compare"
	"github.com/galpt/mangohud-log-parser/internal/mangohud"
	"github.com/galpt/mangohud-log-parser/internal/render"
)

// Geometry of the run table, in layout px.
const (
	tableRowH   = 28
	tablePadX   = 24
	headingPad  = 8
	minSummaryW = 900
)

const (
	deltaName = "Change vs EEVDF (%)"
	// deltaFile is the slug of the delta image. It leaves out the "(%)" of
	// deltaName, which would add a "-pct" that says nothing in a file name.
	deltaFile = "change-vs-eevdf"
	// emptyCell stands for a value that a log does not carry.
	emptyCell = "–"
)

// Caption sentences of the bar images.
const (
	barEncoding = "Each bar is one run, computed from every sample of its log, " +
		"and the number above it is the value at the precision shown."
	naCaption     = "n/a marks a run whose log has no column for this statistic."
	deltaEncoding = "Each bar is the percent change of a run against the run named EEVDF, " +
		"100 × (run − EEVDF) / EEVDF, for the statistics that have a better direction. " +
		"The arrow after a statistic points the better way: a rise is better for FPS and a fall is better for frame time."
	deltaZero = "A statistic that is zero for EEVDF has no percent change and is left out."
)

// runTable is the table of runs, one row per run, as text cells.
func runTable(set *compare.Set) (header []string, rows [][]string) {
	header = []string{"Run", "File", "MangoHud", "Samples", "Duration", "OS", "CPU", "GPU", "RAM", "Kernel", "Governor"}
	cell := func(s string) string {
		if s == "" {
			return emptyCell
		}
		return s
	}
	for _, r := range set.Runs() {
		sys := r.Log.System
		cpu := sys.CPU
		if sys.Hardware != "" {
			// The names could not be split, so the whole text goes where a
			// reader looks for the hardware, and a note says why.
			cpu = sys.Hardware
		}
		ram := ""
		if sys.RAMKiB > 0 {
			ram = strconv.FormatFloat(float64(sys.RAMKiB)/(1<<20), 'f', 1, 64) + " GiB"
		}
		rows = append(rows, []string{
			r.Label, r.Log.Source, cell(r.Log.Version), tickText(float64(r.Stats.Samples), 0),
			strconv.FormatFloat(r.Stats.Duration.Seconds(), 'f', 1, 64) + " s",
			cell(sys.OS), cell(cpu), cell(sys.GPU), cell(ram), cell(sys.Kernel), cell(sys.Governor),
		})
	}
	return header, rows
}

// summaryNotes gathers what a reader needs to know about the runs besides the
// table: the remarks of the parser, the fields that differ between runs, and
// what the Governor column means.
func summaryNotes(set *compare.Set) []string {
	var notes []string
	for _, r := range set.Runs() {
		for _, n := range r.Log.Notes {
			notes = append(notes, r.Label+": "+n+".")
		}
	}
	for _, d := range set.Differences() {
		notes = append(notes, d+".")
	}
	return append(notes,
		"Governor is the cpufreq governor of the first CPU, which MangoHud logs as cpuscheduler. It is not the CPU scheduler of the kernel.",
		"Duration is the time from the first to the last sample. Samples is the number of rows, one per frame unless MangoHud was set to log at an interval.")
}

// textBlock lays text out top to bottom from y, wrapped to a width, and
// collects it as titles.
type textBlock struct {
	m      Measure
	width  float64
	y      float64
	titles []opts.Title
}

func (t *textBlock) heading(text string) {
	t.titles = append(t.titles, textAt(text, margin, t.y, headingSize, true, ink))
	t.y += lineHeight(headingSize) + headingPad
}

func (t *textBlock) paragraph(text string, size float64) {
	lines := wrap(text, math.Min(t.width, maxTextW), size, false, t.m)
	t.titles = append(t.titles, textAt(strings.Join(lines, "\n"), margin, t.y, size, false, ink))
	t.y += float64(len(lines)) * lineHeight(size)
}

// table is a booktabs table of text cells. A header cell may hold several
// lines, joined by "\n". bold, when given, parallels rows and marks the cells
// drawn in the bold face, which the column widths account for.
type table struct {
	header []string
	rows   [][]string
	bold   [][]bool
}

func (tb table) isBold(row, col int) bool { return tb.bold != nil && tb.bold[row][col] }

// widths returns the width of every column: its widest cell, the header in
// bold, plus the padding between columns.
func (tb table) widths(m Measure) []float64 {
	w := make([]float64, len(tb.header))
	for i, h := range tb.header {
		for _, line := range strings.Split(h, "\n") {
			w[i] = math.Max(w[i], m(line, bodySize, true))
		}
		for r, row := range tb.rows {
			w[i] = math.Max(w[i], m(row[i], bodySize, tb.isBold(r, i)))
		}
		w[i] += tablePadX
	}
	return w
}

// page is an image made of text and rules alone, laid out top to bottom.
type page struct {
	chart *charts.HeatMap
	miss  missing
	text  *textBlock
}

// newPage starts a page of the given inner width with its title. A heat map
// is the chart type that draws nothing by default, so only the titles and the
// rules end up in the image.
func newPage(title string, inner float64, m Measure) *page {
	hm := charts.NewHeatMap()
	hm.SetGlobalOptions(append(baseOptions(), hiddenAxes()...)...)
	p := &page{chart: hm, text: &textBlock{m: m, width: inner}}
	p.text.titles = append(p.text.titles, textAt(title, margin, titleTop, titleSize, true, ink))
	p.text.y = titleTop + lineHeight(titleSize) + sectionGap
	return p
}

// table draws tb with the column widths colW. It is a booktabs table: heavier
// rules above and below, a light one under the header, and no vertical lines.
// The header cells sit on the rule under them, so a label of several lines
// grows the header upward and leaves the other labels beside its last line.
func (p *page) table(tb table, colW []float64) {
	t := p.text
	tableW := 0.0
	for _, w := range colW {
		tableW += w
	}
	lines := 1
	for _, h := range tb.header {
		lines = max(lines, strings.Count(h, "\n")+1)
	}
	headH := tableRowH + float64(lines-1)*lineHeight(bodySize)
	pad := (tableRowH - lineHeight(bodySize)) / 2

	p.miss.graphic = append(p.miss.graphic, ruleLine(margin, t.y, margin+tableW, t.y, 1.5))
	x := float64(margin)
	for i, h := range tb.header {
		top := t.y + headH - pad - float64(strings.Count(h, "\n")+1)*lineHeight(bodySize)
		t.titles = append(t.titles, textAt(h, x, top, bodySize, true, ink))
		x += colW[i]
	}
	t.y += headH
	p.miss.graphic = append(p.miss.graphic, ruleLine(margin, t.y, margin+tableW, t.y, 0.8))
	for r, row := range tb.rows {
		x := float64(margin)
		for i, text := range row {
			t.titles = append(t.titles, textAt(text, x, t.y+pad, bodySize, tb.isBold(r, i), ink))
			x += colW[i]
		}
		t.y += tableRowH
	}
	p.miss.graphic = append(p.miss.graphic, ruleLine(margin, t.y, margin+tableW, t.y, 1.5))
}

// finish encodes the page as a figure of the given size.
func (p *page) finish(file string, width, height int) (Figure, error) {
	p.miss.titles = p.text.titles
	p.miss.yTick = opts.AxisTick{Show: opts.Bool(false)}
	option, err := p.miss.apply(p.chart)
	if err != nil {
		return Figure{}, err
	}
	return Figure{Path: file, Width: width, Height: height, Option: option}, nil
}

// runsFigure lays out the run table and the notes, top to bottom.
func runsFigure(set *compare.Set, m Measure) (Figure, error) {
	header, rows := runTable(set)
	tb := table{header: header, rows: rows}
	colW := tb.widths(m)
	tableW := 0.0
	for _, w := range colW {
		tableW += w
	}
	title := "MangoHud runs"
	inner := math.Max(tableW, math.Max(minSummaryW-2*margin, m(title, titleSize, true)))
	width := int(math.Ceil(2*margin + inner))
	if width > render.MaxSide {
		return Figure{}, fmt.Errorf("the run table needs an image %d px wide, more than the %d px that render allows", width, render.MaxSide)
	}

	p := newPage(title, inner, m)
	p.table(tb, colW)
	t := p.text
	t.y += 2 * sectionGap
	t.heading("Notes")
	for _, n := range summaryNotes(set) {
		t.paragraph(n, bodySize)
		t.y += 6
	}
	height := int(math.Ceil(t.y + margin))
	if height > render.MaxSide {
		return Figure{}, fmt.Errorf("the notes need an image %d px tall, more than the %d px that render allows", height, render.MaxSide)
	}
	return p.finish(path.Join(summaryDir, "runs.png"), width, height)
}

// hiddenAxes gives the default axes of a chart without a plot a grid that is
// hidden, instead of leaving them to ECharts' default placement.
func hiddenAxes() []charts.GlobalOpts {
	return []charts.GlobalOpts{
		charts.WithGridOpts(opts.Grid{Show: opts.Bool(false), Left: "0", Top: "0", Width: "1", Height: "1"}),
		charts.WithXAxisOpts(opts.XAxis{Show: opts.Bool(false)}),
		charts.WithYAxisOpts(opts.YAxis{Show: opts.Bool(false)}),
	}
}

func runLabels(set *compare.Set) []string {
	runs := set.Runs()
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.Label
	}
	return out
}

// categoryLabels wraps the run labels to the width of their slot, so ECharts
// never wraps them on its own, and returns the room under the plot they need.
func categoryLabels(labels []string, slot float64, m Measure) ([]string, float64) {
	out := make([]string, len(labels))
	lines := 1
	for i, s := range labels {
		w := wrap(s, slot-8, tickSize, false, m)
		lines = max(lines, len(w))
		out[i] = strings.Join(w, "\n")
	}
	return out, 10 + float64(lines)*lineHeight(tickSize)
}

// metricFigure draws one statistic with one bar per run.
func metricFigure(set *compare.Set, mt compare.Metric, sources []string, m Measure) (Figure, error) {
	lo, hi := 0.0, 0.0
	band := float64(minBand)
	hasNA := false
	for _, c := range mt.Cells {
		text := naText
		if c.Valid {
			text = c.Text
			lo, hi = math.Min(lo, c.Value), math.Max(hi, c.Value)
		} else {
			hasNA = true
		}
		band = math.Max(band, m(text, valueSize, false)+bandPad)
	}
	axis := linearAxis(lo, hi, barPlotH, labelHead, labelHead)

	caption := []string{definition(mt), barEncoding}
	if hasNA {
		caption = append(caption, naCaption)
	}
	caption = append(caption, sources...)

	labels := runLabels(set)
	categories, foot := categoryLabels(labels, band, m)
	title := metricName(mt.Label, mt.Unit, mt.Direction)
	l, err := layoutChart(chartSpec{
		minPlotW: float64(len(labels)) * band, axisW: axisWidth(axis.ticks(), m), plotH: barPlotH, footH: foot,
		titleW: titleWidth(title, "", m), legend: labels, caption: caption,
	}, m)
	if err != nil {
		return Figure{}, err
	}

	b := newBarBuilder(l, m)
	b.miss.xLineHeight = lineHeight(tickSize)
	b.header(title, "")
	b.axes(opts.XAxis{
		Type: "category", Data: categories, AxisLine: axisLine(), AxisTick: &opts.AxisTick{Show: opts.Bool(false)},
		AxisLabel: &opts.AxisLabel{Color: ink, FontSize: tickSize, FontFamily: fontFamily, Interval: "0", Margin: 10},
	}, valueAxis(axis))

	// One series per run puts the run in the legend. Every series holds a
	// single bar at its own category, and a gap of -100% stacks the empty
	// slots of the others on top of it so the bar stays centered.
	var values []opts.ScatterData
	for r, label := range labels {
		data := make([]opts.BarData, len(labels))
		for k := range data {
			data[k] = opts.BarData{Value: "-"}
		}
		c := mt.Cells[r]
		if c.Valid {
			data[r] = opts.BarData{Value: c.Value}
			// The label is carried by a point at the top of the bar. No
			// MangoHud column holds a negative value, and one would get its
			// label at zero.
			values = append(values, opts.ScatterData{Name: c.Text, Value: []float64{float64(r), math.Max(c.Value, 0)}})
		} else {
			values = append(values, opts.ScatterData{Name: naText, Value: []float64{float64(r), 0}})
		}
		b.addBar(r, label, data, charts.WithBarChartOpts(opts.BarChart{BarGap: "-100%", BarWidth: "58%"}))
	}
	b.rect.MultiSeries = append(b.rect.MultiSeries, valueLabels(values))
	return b.finish()
}

// deltaCell is the percent change of one run against the baseline.
type deltaCell struct {
	percent float64
	text    string
	valid   bool
}

// deltaRow is one statistic of the delta image, with a cell per run.
type deltaRow struct {
	metric compare.Metric
	cells  []deltaCell
}

// deltaMetrics returns the position of the run labeled exactly EEVDF and the
// changes of the directional statistics of every other run against it. There
// are none when no run has that label, when it is the only run, or when no
// statistic has a value in both.
func deltaMetrics(set *compare.Set) (int, []deltaRow) {
	base := -1
	for i, r := range set.Runs() {
		if r.Label == baselineLabel {
			base = i
		}
	}
	if base < 0 {
		return -1, nil
	}
	var rows []deltaRow
	for _, mt := range set.Metrics() {
		if mt.Direction == mangohud.NoDirection {
			continue
		}
		row := deltaRow{metric: mt, cells: make([]deltaCell, len(mt.Cells))}
		found := false
		b := mt.Cells[base]
		for i, c := range mt.Cells {
			if i == base || !c.Valid || !b.Valid || b.Value == 0 {
				continue
			}
			p := 100 * (c.Value - b.Value) / b.Value
			row.cells[i] = deltaCell{percent: p, text: fmt.Sprintf("%+.1f%%", p), valid: true}
			found = true
		}
		if found {
			rows = append(rows, row)
		}
	}
	return base, rows
}

// deltaGroupGap is the empty space between two groups of the delta image,
// in slots.
const deltaGroupGap = 0.75

// deltaFigure draws the change of every directional statistic against the
// baseline run, one group of bars per statistic and one bar per other run.
func deltaFigure(set *compare.Set, base int, rows []deltaRow, sources []string, m Measure) (Figure, error) {
	var runs []int
	for i := range set.Runs() {
		if i != base {
			runs = append(runs, i)
		}
	}
	// A slot holds one bar and must fit the widest percent label, since the
	// labels of neighboring bars sit side by side. Each group leaves a gap of
	// deltaGroupGap slots to the next.
	slot := 32.0
	lo, hi := 0.0, 0.0
	zero := false
	for _, d := range rows {
		for _, i := range runs {
			if c := d.cells[i]; c.valid {
				slot = math.Max(slot, m(c.text, valueSize, false)+6)
				lo, hi = math.Min(lo, c.percent), math.Max(hi, c.percent)
			}
		}
	}
	for _, mt := range set.Metrics() {
		if mt.Direction != mangohud.NoDirection && mt.Cells[base].Valid && mt.Cells[base].Value == 0 {
			zero = true
		}
	}
	axis := linearAxis(lo, hi, barPlotH, labelHead, labelHead)
	group := (float64(len(runs)) + deltaGroupGap) * slot
	names := make([]string, len(rows))
	for k, d := range rows {
		names[k] = metricName(d.metric.Label, d.metric.Unit, d.metric.Direction)
	}
	categories, foot := categoryLabels(names, group, m)
	legend := make([]string, len(runs))
	labels := runLabels(set)
	for n, i := range runs {
		legend[n] = labels[i]
	}

	caption := []string{deltaEncoding}
	if zero {
		caption = append(caption, deltaZero)
	}
	l, err := layoutChart(chartSpec{
		minPlotW: float64(len(rows)) * group, axisW: axisWidth(axis.ticks(), m), plotH: barPlotH, footH: foot,
		titleW: titleWidth(deltaName, "", m), legend: legend, caption: append(caption, sources...),
	}, m)
	if err != nil {
		return Figure{}, err
	}

	b := newBarBuilder(l, m)
	b.miss.xLineHeight = lineHeight(tickSize)
	b.header(deltaName, "")
	b.axes(opts.XAxis{
		Type: "category", Data: categories, AxisLine: axisLine(), AxisTick: &opts.AxisTick{Show: opts.Bool(false)},
		AxisLabel: &opts.AxisLabel{Color: ink, FontSize: tickSize, FontFamily: fontFamily, Interval: "0", Margin: 10},
	}, valueAxis(axis))

	for n, i := range runs {
		data := make([]opts.BarData, len(rows))
		for k, d := range rows {
			c := d.cells[i]
			if !c.valid {
				data[k] = opts.BarData{Value: "-"}
				continue
			}
			pos := "top"
			if math.Signbit(c.percent) {
				pos = "bottom"
			}
			data[k] = opts.BarData{Value: c.percent, Label: &opts.Label{
				Show: opts.Bool(true), Position: pos, Formatter: types.FuncStr(c.text),
				FontSize: valueSize, FontFamily: fontFamily, Color: ink,
			}}
		}
		// The bars of a group touch the slots of their neighbors only through
		// the 16% gap, which keeps the labels above them apart.
		options := []charts.SeriesOpts{charts.WithBarChartOpts(opts.BarChart{BarWidth: px(slot / 1.16), BarGap: "16%"})}
		if n == 0 {
			// The zero line is drawn over the bars so that it reads as the
			// baseline even where a bar starts at it.
			options = append(options,
				charts.WithMarkLineNameYAxisItemOpts(opts.MarkLineNameYAxisItem{YAxis: 0}),
				charts.WithMarkLineStyleOpts(opts.MarkLineStyle{
					Symbol: []string{"none", "none"}, Label: &opts.Label{Show: opts.Bool(false)},
					LineStyle: &opts.LineStyle{Color: ink, Width: 1, Type: "solid"},
				}))
		}
		b.addBar(i, labels[i], data, options...)
	}
	return b.finish()
}
