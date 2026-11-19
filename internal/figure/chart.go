// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"math"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

const (
	naText = "n/a"
	// markGap is the space between a title and the scale mark after it.
	markGap = 12
	// curveWidth is the width of a line in layout px. It is drawn at twice
	// that in the PNG, which stays clear of the lines next to it even when
	// eight runs share a plot.
	curveWidth = 1.2
)

// chartBuilder accumulates the option of one image with a single plot. Bar
// images are built on a charts.Bar, whose AddSeries knows the bar options,
// and line images on a charts.Line, whose series are added here.
type chartBuilder struct {
	rect    *charts.RectChart
	bar     *charts.Bar
	measure Measure
	layout  chartLayout
	miss    missing
}

func newBuilder(rect *charts.RectChart, l chartLayout, m Measure) *chartBuilder {
	b := &chartBuilder{rect: rect, measure: m, layout: l,
		miss: missing{decals: map[int]*decal{}, yTick: opts.AxisTick{Show: opts.Bool(true), LineStyle: &opts.LineStyle{Color: ink, Width: 1}}}}
	rect.SetGlobalOptions(baseOptions()...)
	return b
}

func newBarBuilder(l chartLayout, m Measure) *chartBuilder {
	bar := charts.NewBar()
	b := newBuilder(&bar.RectChart, l, m)
	b.bar = bar
	return b
}

func newLineBuilder(l chartLayout, m Measure) *chartBuilder {
	return newBuilder(&charts.NewLine().RectChart, l, m)
}

// titleWidth is the width of an image title with the scale mark after it.
func titleWidth(title, mark string, m Measure) float64 {
	w := m(title, titleSize, true)
	if mark != "" {
		w += markGap + m(mark, legendSize, false)
	}
	return w
}

// header adds the title, the scale mark after it when there is one, and the
// legend rows.
func (b *chartBuilder) header(title, mark string) {
	b.miss.titles = append(b.miss.titles, textAt(title, margin, titleTop, titleSize, true, ink))
	if mark != "" {
		// The mark is its own title because ECharts sets the plain part of a
		// rich title in its default style, which would lose the bold.
		x := margin + b.measure(title, titleSize, true) + markGap
		t := textAt(mark, x, titleTop+(titleSize-legendSize)/2+1, legendSize, false, ink)
		t.TitleStyle.FontStyle = "italic"
		b.miss.titles = append(b.miss.titles, t)
	}
	// ECharts draws the legend of a line series as its line with a circle
	// over it, even when the line has no symbols. The circle is made
	// transparent, so the swatch is the line itself, dash included.
	icon, style := "rect", (*opts.ItemStyle)(nil)
	if b.bar == nil {
		icon, style = "", &opts.ItemStyle{Opacity: opts.Float(0)}
	}
	for i, row := range b.layout.legendRows {
		b.miss.legends = append(b.miss.legends, opts.Legend{
			// Without padding the swatches start at the margin, where the
			// rows were measured from.
			Data: row, Left: px(margin), Top: px(b.layout.legendTop + float64(i)*legendRowH), Padding: 0,
			Icon: icon, ItemWidth: legendIconW, ItemHeight: legendIconH, ItemStyle: style,
			TextStyle: &opts.TextStyle{FontFamily: fontFamily, FontSize: legendSize, Color: ink},
		})
	}
}

// axes places the one plot of the image.
func (b *chartBuilder) axes(x opts.XAxis, y opts.YAxis) {
	l := b.layout
	b.rect.SetGlobalOptions(
		charts.WithGridOpts(opts.Grid{Left: px(l.plotLeft), Top: px(l.plotTop), Width: px(l.plotW), Height: px(l.plotH)}),
		charts.WithXAxisOpts(x, 0), charts.WithYAxisOpts(y, 0),
	)
}

// addBar adds a bar series in the color and pattern of run.
func (b *chartBuilder) addBar(run int, label string, data []opts.BarData, options ...charts.SeriesOpts) {
	if d := runDecal(run); d != nil {
		b.miss.decals[len(b.rect.MultiSeries)] = d
	}
	b.bar.AddSeries(label, data, append(options, charts.WithItemStyleOpts(opts.ItemStyle{Color: runColor(run), BorderColor: barEdge, BorderWidth: 0.8}))...)
}

// addLine adds a line through points in the color and dash of run. The item
// color is what the legend draws its line in.
//
// The points go into a dataset of two columns rather than into the series as
// pairs. A pair is an array of its own in the JavaScript runtime, so a column
// layout holds a run of many thousand frames in a small part of the memory
// and parses faster.
func (b *chartBuilder) addLine(run int, label string, points [][2]float64) {
	x, y := make([]float64, len(points)), make([]float64, len(points))
	for i, p := range points {
		x[i], y[i] = p[0], p[1]
	}
	b.miss.datasets = append(b.miss.datasets, dataset{Source: columns{X: x, Y: y}})
	b.rect.MultiSeries = append(b.rect.MultiSeries, charts.SingleSeries{
		Name: label, Type: types.ChartLine, DatasetIndex: len(b.miss.datasets) - 1,
		Encode: &opts.Encode{X: "x", Y: "y"}, Symbol: "none", ShowSymbol: opts.Bool(false),
		ItemStyle: &opts.ItemStyle{Color: runColor(run)},
		LineStyle: &opts.LineStyle{Color: runColor(run), Width: curveWidth, Type: runDash(run)},
	})
}

// finish adds the caption and encodes the option.
func (b *chartBuilder) finish() (Figure, error) {
	l := b.layout
	b.miss.titles = append(b.miss.titles, textAt(strings.Join(l.caption, "\n"), margin, l.captionTop, captionSize, false, ink))
	option, err := b.miss.apply(b.rect)
	if err != nil {
		return Figure{}, err
	}
	return Figure{Width: l.width, Height: l.height, Option: option}, nil
}

// titlePad is the padding that ECharts puts around every title, which
// opts.Title has no field to change.
const titlePad = 5

// textAt is a line or block of text placed with its top-left corner at x, y.
// The title is moved up and left by its padding, so that the text itself
// starts at x, y where the layout measured it.
func textAt(text string, x, y, size float64, bold bool, color string) opts.Title {
	weight := "normal"
	if bold {
		weight = "bold"
	}
	return opts.Title{Title: text, Left: px(x - titlePad), Top: px(y - titlePad), TitleStyle: &opts.TextStyle{
		FontFamily: fontFamily, FontSize: int(size), FontWeight: weight, Color: color, LineHeight: int(lineHeight(size)),
	}}
}

// axisWidth is the room left of a plot for its tick labels, which sit 8px
// left of the axis line, after the 5px ticks.
func axisWidth(ticks []string, m Measure) float64 {
	w := 0.0
	for _, s := range ticks {
		w = math.Max(w, m(s, tickSize, false))
	}
	return w + 18
}

func axisLine() *opts.AxisLine {
	return &opts.AxisLine{Show: opts.Bool(true), OnZero: opts.Bool(false), LineStyle: &opts.LineStyle{Color: ink, Width: 1}}
}

func splitLine() *opts.SplitLine {
	return &opts.SplitLine{Show: opts.Bool(true), LineStyle: &opts.LineStyle{Color: rule, Width: 1}}
}

func tickLabel() *opts.AxisLabel {
	return &opts.AxisLabel{Color: ink, FontSize: tickSize, FontFamily: fontFamily, Margin: 8}
}

// valueAxis is the y axis of a plot with the ends chosen by axisRange.
func valueAxis(a axisRange) opts.YAxis {
	y := opts.YAxis{Type: "value", Min: a.min, Max: a.max, AxisLine: axisLine(), SplitLine: splitLine(), AxisLabel: tickLabel()}
	if a.log {
		y.Type = "log"
	} else {
		// Pinning both interval bounds makes ECharts tick at the step the
		// ends were rounded to.
		y.MinInterval, y.MaxInterval = a.step, a.step
	}
	return y
}

// footH is the room under a plot whose x axis has tick labels and a name.
var footH = 2*lineHeight(tickSize) + 16

// valueXAxis is a numeric x axis with the ends chosen by axisRange and a name
// centered under its tick labels. format is the ECharts template of a tick
// label.
func valueXAxis(a axisRange, name, format string) opts.XAxis {
	label := tickLabel()
	label.Formatter = types.FuncStr(format)
	x := opts.XAxis{
		Type: "value", Min: a.min, Max: a.max, Name: name, NameLocation: "middle", NameGap: int(lineHeight(tickSize) + 14),
		AxisLine: axisLine(), SplitLine: splitLine(), AxisLabel: label,
		AxisTick: &opts.AxisTick{Show: opts.Bool(true), LineStyle: &opts.LineStyle{Color: ink, Width: 1}},
	}
	if a.log {
		x.Type = "log"
	} else {
		x.MinInterval, x.MaxInterval = a.step, a.step
	}
	return x
}

// valueLabels is an invisible scatter series that carries the value labels
// of a bar plot. A scatter series is drawn at an opacity of 0.8 by default,
// and its labels with it, which would turn the black text gray.
func valueLabels(data []opts.ScatterData) charts.SingleSeries {
	return charts.SingleSeries{
		Type: types.ChartScatter, Data: data, SymbolSize: 0, ItemStyle: &opts.ItemStyle{Opacity: opts.Float(1)},
		Label: &opts.Label{Show: opts.Bool(true), Position: "top", Formatter: "{b}", FontSize: valueSize, FontFamily: fontFamily, Color: ink},
	}
}
