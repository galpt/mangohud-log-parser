// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/galpt/mangohud-log-parser/internal/compare"
	"github.com/galpt/mangohud-log-parser/internal/mangohud"
	"github.com/galpt/mangohud-log-parser/internal/render"
)

// measure stands in for the font: every character is 0.55 em, or 0.6 em in
// bold. The layout only needs widths that grow with the text.
func measure(text string, size float64, bold bool) float64 {
	em := 0.55
	if bold {
		em = 0.6
	}
	return float64(utf8.RuneCountInString(text)) * size * em
}

// header is the column set of the synthetic logs, the one MangoHud v0.5
// wrote, plus a column that no release writes.
const header = "fps,frametime,cpu_load,gpu_load,cpu_temp,gpu_temp,gpu_core_clock,gpu_mem_clock,gpu_vram_used,gpu_power,ram_used,mystery,elapsed"

// synthLog makes a log of n frames whose frame times scatter around base ms
// with a slow frame every 97 frames, the way a game stutters.
func synthLog(t *testing.T, source string, n int, base float64, seed int64) *mangohud.Log {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	var b strings.Builder
	b.WriteString("os,cpu,gpu,ram,kernel,driver,cpuscheduler\nTest OS,Test CPU,Test GPU,16000000,6.1.0-test,,performance\n")
	b.WriteString(header + "\n")
	elapsed := int64(0)
	for i := range n {
		ft := base * (0.8 + 0.4*rng.Float64())
		if i%97 == 96 {
			ft *= 6
		}
		elapsed += int64(ft * 1e6)
		fmt.Fprintf(&b, "%.6g,%.6g,%.1f,%d,%d,%d,1800,7000,4.5,%d,9.25,%d,%d\n",
			1000/ft, ft, 30+10*rng.Float64(), 90+rng.Intn(10), 60+rng.Intn(5), 55+rng.Intn(5), 200+rng.Intn(20), rng.Intn(3), elapsed)
	}
	l, err := mangohud.Parse(source, strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func synthSet(t *testing.T, names ...string) *compare.Set {
	t.Helper()
	var logs []*mangohud.Log
	for i := range names {
		logs = append(logs, synthLog(t, fmt.Sprintf("run%d.csv", i), 1500+500*i, 6+float64(i), int64(i+1)))
	}
	s, err := compare.New(logs, names)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustBuild(t *testing.T, names ...string) []Figure {
	t.Helper()
	figs, err := Build(synthSet(t, names...), measure)
	if err != nil {
		t.Fatal(err)
	}
	return figs
}

// option is the part of an ECharts option that the tests look at.
type option struct {
	Grid    []json.RawMessage         `json:"grid"`
	Title   []struct{ Text string }   `json:"title"`
	Legend  []struct{ Data []string } `json:"legend"`
	XAxis   []axisJSON                `json:"xAxis"`
	YAxis   []axisJSON                `json:"yAxis"`
	Dataset []struct {
		Source struct{ X, Y []float64 }
	} `json:"dataset"`
	Series []struct {
		Type         string
		Name         string
		Sampling     string
		DatasetIndex int
		Data         []json.RawMessage
		ItemStyle    struct{ Decal *decal } `json:"itemStyle"`
		LineStyle    struct{ Type string }  `json:"lineStyle"`
	} `json:"series"`
}

type axisJSON struct {
	Type    string
	Min     float64
	Max     float64
	Inverse bool
}

func decode(t *testing.T, f Figure) option {
	t.Helper()
	var o option
	if err := json.Unmarshal(f.Option, &o); err != nil {
		t.Fatalf("%s: %v", f.Path, err)
	}
	return o
}

// curve returns the points of line series i from its dataset.
func curve(t *testing.T, o option, i int) [][2]float64 {
	t.Helper()
	src := o.Dataset[o.Series[i].DatasetIndex].Source
	if len(src.X) != len(src.Y) {
		t.Fatalf("series %d has %d x and %d y values", i, len(src.X), len(src.Y))
	}
	out := make([][2]float64, len(src.X))
	for k := range src.X {
		out[k] = [2]float64{src.X[k], src.Y[k]}
	}
	return out
}

func paths(figs []Figure) []string {
	out := make([]string, len(figs))
	for i, f := range figs {
		out[i] = f.Path
	}
	return out
}

func byPath(t *testing.T, figs []Figure, path string) Figure {
	t.Helper()
	for _, f := range figs {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no figure %s in %q", path, paths(figs))
	return Figure{}
}

func legendOf(o option) []string {
	var out []string
	for _, l := range o.Legend {
		out = append(out, l.Data...)
	}
	return out
}

// summaryNames are the statistic images of a synthetic log, in order.
var summaryNames = []string{
	"average-fps", "1pct-min-fps", "0-1pct-min-fps", "97pct-percentile-fps", "average-frame-time",
	"average-cpu-load", "peak-cpu-load", "average-gpu-load", "peak-gpu-load",
	"average-cpu-temperature", "peak-cpu-temperature", "average-gpu-temperature", "peak-gpu-temperature",
	"average-gpu-vram-used", "peak-gpu-vram-used", "average-ram-used", "peak-ram-used",
	"average-gpu-power", "peak-gpu-power",
	"average-gpu-core-clock", "peak-gpu-core-clock", "average-gpu-memory-clock", "peak-gpu-memory-clock",
}

var timelineNames = []string{
	"fps", "frametime", "cpu-load", "gpu-load", "cpu-temp", "gpu-temp", "gpu-core-clock", "gpu-mem-clock",
	"gpu-vram-used", "gpu-power", "ram-used", "mystery",
}

func wantPaths(eevdf bool) []string {
	want := []string{"summary/runs.png"}
	names := slices.Clone(summaryNames)
	if eevdf {
		names = append(names, "change-vs-eevdf")
	}
	for i, n := range names {
		want = append(want, fmt.Sprintf("summary/%02d-%s.png", i+1, n))
	}
	for i, n := range timelineNames {
		want = append(want, fmt.Sprintf("timeline/%02d-%s.png", i+1, n))
	}
	return append(want, "distribution/1-frametime-tail.png", "distribution/2-frametime-body.png")
}

func TestBuildPaths(t *testing.T) {
	if got, want := paths(mustBuild(t, "scx_a", "EEVDF")), wantPaths(true); !slices.Equal(got, want) {
		t.Errorf("paths = %q\nwant %q", got, want)
	}
	// The baseline is the exact label EEVDF, so no other spelling selects it.
	for _, names := range [][]string{{"scx_a", "eevdf"}, {"scx_a", "Eevdf"}, {"scx_a", "EEVDF "}, {"EEVDF"}} {
		if got, want := paths(mustBuild(t, names...)), wantPaths(false); !slices.Equal(got, want) {
			t.Errorf("%q: paths = %q\nwant %q", names, got, want)
		}
	}
}

func TestNamesAreShownAsTyped(t *testing.T) {
	names := []string{"scx_flow_5.2.28", "scx_mlfq_2.0.2", "EEVDF", "eevdf"}
	figs := mustBuild(t, names...)
	for _, f := range figs {
		if f.Path == "summary/runs.png" {
			for _, n := range names {
				if !strings.Contains(string(f.Option), `"`+n+`"`) {
					t.Errorf("run table lacks %q", n)
				}
			}
			continue
		}
		got := legendOf(decode(t, f))
		want := names
		if strings.HasSuffix(f.Path, deltaFile+".png") {
			want = slices.Delete(slices.Clone(names), 2, 3)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: legend = %q, want %q", f.Path, got, want)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Average FPS":        "average-fps",
		"1% min FPS":         "1pct-min-fps",
		"0.1% min FPS":       "0-1pct-min-fps",
		"97% percentile FPS": "97pct-percentile-fps",
		"cpu_load":           "cpu-load",
		"  GPU (°C) ":        "gpu-c",
		"°":                  "metric",
		"a__b%%":             "a-bpctpct",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNumbered(t *testing.T) {
	if got := numbered("timeline", 0, 9, "fps"); got != "timeline/1-fps.png" {
		t.Errorf("got %s", got)
	}
	if got := numbered("summary", 4, 100, "x"); got != "summary/005-x.png" {
		t.Errorf("got %s", got)
	}
}

// Every chart image has exactly one plot, and the run table has none that
// shows.
func TestOnePlotPerImage(t *testing.T) {
	for _, f := range mustBuild(t, "scx_a", "EEVDF", "scx_b") {
		o := decode(t, f)
		if len(o.Grid) != 1 || len(o.XAxis) != 1 || len(o.YAxis) != 1 {
			t.Errorf("%s has %d grids, %d x axes and %d y axes", f.Path, len(o.Grid), len(o.XAxis), len(o.YAxis))
		}
		if f.Width < minWidth || f.Width > render.MaxSide || f.Height < 1 || f.Height > render.MaxSide {
			t.Errorf("%s is %dx%d", f.Path, f.Width, f.Height)
		}
	}
}

func TestBarImages(t *testing.T) {
	set := synthSet(t, "scx_a", "EEVDF", "scx_b")
	figs, err := Build(set, measure)
	if err != nil {
		t.Fatal(err)
	}
	o := decode(t, byPath(t, figs, "summary/01-average-fps.png"))
	var bars []string
	var labels []string
	for _, s := range o.Series {
		switch s.Type {
		case "bar":
			bars = append(bars, s.Name)
		case "scatter":
			for _, d := range s.Data {
				var p struct{ Name string }
				if err := json.Unmarshal(d, &p); err != nil {
					t.Fatal(err)
				}
				labels = append(labels, p.Name)
			}
		default:
			t.Errorf("unexpected %s series", s.Type)
		}
	}
	if !slices.Equal(bars, []string{"scx_a", "EEVDF", "scx_b"}) {
		t.Errorf("bars = %q", bars)
	}
	var want []string
	for _, c := range set.Metrics()[0].Cells {
		want = append(want, c.Text)
	}
	if !slices.Equal(labels, want) {
		t.Errorf("value labels = %q, want %q", labels, want)
	}
	if title := o.Title[0].Text; title != "Average FPS ↑" {
		t.Errorf("title = %q", title)
	}
	if title := decode(t, byPath(t, figs, "summary/06-average-cpu-load.png")).Title[0].Text; title != "Average CPU load (%)" {
		t.Errorf("a statistic without a direction is titled %q", title)
	}
	caption := o.Title[len(o.Title)-1].Text
	for _, s := range []string{"1000 / (mean frame time in ms)", "scx_a (run0.csv)", "EEVDF (run1.csv)"} {
		if !strings.Contains(caption, s) {
			t.Errorf("caption lacks %q: %s", s, caption)
		}
	}
	caption = decode(t, byPath(t, figs, "summary/02-1pct-min-fps.png")).Title[1].Text
	if !strings.Contains(strings.ReplaceAll(caption, "\n", " "), "trunc(0.01 × N − 1)") {
		t.Errorf("1%% caption lacks the formula: %s", caption)
	}
}

func TestDeltaImage(t *testing.T) {
	set := synthSet(t, "scx_a", "EEVDF", "scx_b")
	base, rows := deltaMetrics(set)
	if base != 1 || len(rows) != 5 {
		t.Fatalf("base %d, %d rows, want 1 and the 5 directional statistics", base, len(rows))
	}
	m := set.Metrics()[0]
	want := 100 * (m.Cells[2].Value - m.Cells[1].Value) / m.Cells[1].Value
	if c := rows[0].cells[2]; !c.valid || math.Abs(c.percent-want) > 1e-9 || c.text != fmt.Sprintf("%+.1f%%", want) {
		t.Errorf("change of scx_b = %+v, want %v", c, want)
	}
	if rows[0].cells[1].valid {
		t.Error("the baseline has a change against itself")
	}
	figs, err := Build(set, measure)
	if err != nil {
		t.Fatal(err)
	}
	o := decode(t, byPath(t, figs, "summary/24-change-vs-eevdf.png"))
	if got := legendOf(o); !slices.Equal(got, []string{"scx_a", "scx_b"}) {
		t.Errorf("legend = %q", got)
	}
	if _, rows := deltaMetrics(synthSet(t, "EEVDF")); len(rows) != 0 {
		t.Error("a lone EEVDF run has changes")
	}
}

func TestTimelineImages(t *testing.T) {
	figs := mustBuild(t, "scx_a", "EEVDF")
	for _, f := range figs {
		if !strings.HasPrefix(f.Path, timelineDir+"/") {
			continue
		}
		o := decode(t, f)
		if len(o.Series) != 2 || len(o.Dataset) != 2 {
			t.Errorf("%s has %d series and %d datasets", f.Path, len(o.Series), len(o.Dataset))
		}
		for i, s := range o.Series {
			if s.Type != "line" || s.Sampling != "lttb" || s.DatasetIndex != i {
				t.Errorf("%s: series %s is %s with sampling %q on dataset %d", f.Path, s.Name, s.Type, s.Sampling, s.DatasetIndex)
			}
		}
		if o.XAxis[0].Type != "value" || o.XAxis[0].Min != 0 {
			t.Errorf("%s: x axis %+v", f.Path, o.XAxis[0])
		}
	}
	o := decode(t, byPath(t, figs, "timeline/01-fps.png"))
	// Every sample is drawn, with the time in seconds since the first one.
	pts := curve(t, o, 0)
	first, last := pts[0], pts[len(pts)-1]
	if len(pts) != 1500 || first[0] != 0 || last[0] <= 0 || last[0] > o.XAxis[0].Max {
		t.Errorf("fps line has %d points from %v to %v on an axis to %v", len(pts), first, last, o.XAxis[0].Max)
	}
	caption := o.Title[len(o.Title)-1].Text
	if !strings.Contains(caption, "LTTB") || !strings.Contains(caption, "all samples") {
		t.Errorf("caption = %s", caption)
	}
}

// The ninth run takes the second round of colors, so its line is dashed
// where the first is solid.
func TestManyRunsDifferInStyle(t *testing.T) {
	var names []string
	for i := range 9 {
		names = append(names, "run_"+strconv.Itoa(i))
	}
	figs := mustBuild(t, names...)
	o := decode(t, byPath(t, figs, "timeline/01-fps.png"))
	if o.Series[0].LineStyle.Type != "solid" || o.Series[7].LineStyle.Type != "dashed" {
		t.Errorf("line styles %q and %q", o.Series[0].LineStyle.Type, o.Series[7].LineStyle.Type)
	}
	bars := decode(t, byPath(t, figs, "summary/01-average-fps.png"))
	if bars.Series[0].ItemStyle.Decal != nil || bars.Series[7].ItemStyle.Decal == nil {
		t.Error("the eighth bar has no pattern")
	}
}

func TestTailRanks(t *testing.T) {
	for _, n := range []int{1, 2, 99, 100, 101, 1000, 15000, 1000003} {
		r := tailRanks(n)
		if r[0] != 1 || r[len(r)-1] != n {
			t.Fatalf("n=%d: ranks %v", n, r)
		}
		for i := 1; i < len(r); i++ {
			if r[i] <= r[i-1] {
				t.Fatalf("n=%d: ranks repeat at %d", n, i)
			}
			// Past the ranks drawn one by one, no gap is wider than the step
			// per decade.
			if r[i]-r[i-1] > 1 && float64(r[i]) > float64(r[i-1])*math.Pow(10, 1.0/tailPerDecade)+1 {
				t.Fatalf("n=%d: gap from %d to %d", n, r[i-1], r[i])
			}
		}
		if n >= 100 && !slices.Equal(r[:100], func() []int {
			var all []int
			for k := 1; k <= 100; k++ {
				all = append(all, k)
			}
			return all
		}()) {
			t.Errorf("n=%d: the 100 slowest frames are not all drawn", n)
		}
	}
	if n := len(tailRanks(1000000)); n > 600 {
		t.Errorf("a million frames take %d points", n)
	}
}

// bodyIndex is the inverse of the empirical distribution function, checked
// against a count of the frames at or below each value.
func TestBodyIndex(t *testing.T) {
	for _, n := range []int{1, 2, 7, 999, 1000, 1001, 12345} {
		for j := 0; j <= bodySteps; j++ {
			i := bodyIndex(j, n)
			if i < 0 || i >= n {
				t.Fatalf("n=%d j=%d: index %d", n, j, i)
			}
			// With distinct values the share at or below position i is
			// (i+1)/n, which must reach p, and the position before must not.
			p := float64(j) / 1000
			if j > 0 && (float64(i+1)/float64(n) < p-1e-12 || float64(i)/float64(n) >= p+1e-12) {
				t.Fatalf("n=%d j=%d: index %d is not the smallest that reaches p", n, j, i)
			}
		}
	}
	if bodyIndex(500, 1000) != 499 || bodyIndex(1, 1000) != 0 || bodyIndex(990, 15000) != 14849 {
		t.Error("bodyIndex is off")
	}
}

func TestDistributionImages(t *testing.T) {
	set := synthSet(t, "scx_a", "EEVDF")
	figs, err := Build(set, measure)
	if err != nil {
		t.Fatal(err)
	}
	tail := decode(t, byPath(t, figs, "distribution/1-frametime-tail.png"))
	x := tail.XAxis[0]
	if x.Type != "log" || !x.Inverse || x.Max != 100 || x.Min > 100.0/2000 {
		t.Errorf("tail x axis %+v", x)
	}
	// The last point of a curve is its slowest frame, at x = 100/N.
	for i, s := range tail.Series {
		p := curve(t, tail, i)[0]
		ft := set.Runs()[i].Log.Series("frametime")
		if want := [2]float64{100 / float64(len(ft)), slices.Max(ft)}; p != want {
			t.Errorf("%s starts at %v, want %v", s.Name, p, want)
		}
	}
	body := decode(t, byPath(t, figs, "distribution/2-frametime-body.png"))
	for i, s := range body.Series {
		pts := curve(t, body, i)
		if len(pts) != bodySteps+1 {
			t.Errorf("%s has %d points", s.Name, len(pts))
		}
		p := pts[500]
		ft := slices.Sorted(slices.Values(set.Runs()[i].Log.Series("frametime")))
		if want := ft[len(ft)/2-1]; p[0] != 50 || p[1] != want {
			t.Errorf("%s has %v at the median, want %v", s.Name, p, want)
		}
	}
}

// No log axis gets a bound or a value that is not positive, whatever the
// spread of the frame times.
func TestLogAxesArePositive(t *testing.T) {
	check := func(name string, figs []Figure) {
		for _, f := range figs {
			o := decode(t, f)
			for i, axes := range [][]axisJSON{o.XAxis, o.YAxis} {
				for _, a := range axes {
					if a.Type != "log" {
						continue
					}
					if a.Min <= 0 || a.Max <= a.Min {
						t.Errorf("%s %s: log axis %+v", name, f.Path, a)
					}
					for k := range o.Series {
						for _, p := range curve(t, o, k) {
							if p[i] <= 0 {
								t.Errorf("%s %s: point %v on a log axis", name, f.Path, p)
							}
						}
					}
				}
			}
		}
	}
	check("stutter", mustBuild(t, "scx_a", "EEVDF"))
	// A frame time of zero would have no place on a log y axis, so the tail
	// image falls back to a linear one.
	l := synthLog(t, "zero.csv", 300, 5, 9)
	l.Data[1][10] = 0
	l.Data[1][20] = 400
	set, err := compare.New([]*mangohud.Log{l}, nil)
	if err != nil {
		t.Fatal(err)
	}
	figs, err := Build(set, measure)
	if err != nil {
		t.Fatal(err)
	}
	check("zero", figs)
	if y := decode(t, byPath(t, figs, "distribution/1-frametime-tail.png")).YAxis[0]; y.Type == "log" {
		t.Errorf("a zero frame time gives a log axis %+v", y)
	}
	// A slow frame 80 times the fastest switches the y axis to log.
	l = synthLog(t, "spike.csv", 300, 5, 9)
	l.Data[1][20] = 400
	set, err = compare.New([]*mangohud.Log{l}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if figs, err = Build(set, measure); err != nil {
		t.Fatal(err)
	}
	check("spike", figs)
	if y := decode(t, byPath(t, figs, "distribution/1-frametime-tail.png")).YAxis[0]; y.Type != "log" {
		t.Errorf("a wide spread gives a %s axis", y.Type)
	}
}

func TestLinearAxis(t *testing.T) {
	a := linearAxis(0, 397, 200, 20, 0)
	if a.min != 0 || a.step != 100 || a.max != 500 {
		t.Errorf("0..397 gives %+v, want 0..500 by 100", a)
	}
	if a := linearAxis(0, 0, 200, 20, 0); a.max <= 0 {
		t.Errorf("an all-zero plot gives %+v", a)
	}
	if got := strings.Join(linearAxis(0, 0.25, 200, 20, 0).ticks(), " "); got != "0.00 0.05 0.10 0.15 0.20 0.25 0.30" {
		t.Errorf("ticks = %s", got)
	}
	if a := fittedAxis(4.1, 8.6); a.min != 4 || a.max != 9 || a.step != 1 {
		t.Errorf("4.1..8.6 gives %+v", a)
	}
	if a := fittedAxis(5, 5); a.min >= 5 || a.max <= 5 {
		t.Errorf("a flat curve gives %+v", a)
	}
	if got := strings.Join(logAxis(3, 300, 200, 0).ticks(), " "); got != "1 10 100 1,000" {
		t.Errorf("log ticks = %s", got)
	}
}

func TestWrap(t *testing.T) {
	lines := wrap("one two three four five", measure("one two", 10, false), 10, false, measure)
	if strings.Join(lines, "|") != "one two|three|four|five" {
		t.Errorf("wrap = %q", lines)
	}
	if lines := wrap("abcdefghij", measure("abcd", 10, false), 10, false, measure); strings.Join(lines, "|") != "abcd|efgh|ij" {
		t.Errorf("long word wraps as %q", lines)
	}
}

// barSpec is the spec of a bar image of n runs whose labels fit minBand.
func barSpec(n int) chartSpec {
	s := chartSpec{minPlotW: float64(n) * minBand, axisW: 50, plotH: barPlotH, footH: plotFoot, titleW: 200, caption: []string{"A caption."}}
	for i := range n {
		s.legend = append(s.legend, "run_"+strconv.Itoa(i))
	}
	return s
}

// TestLayoutGrowsWithRuns checks the layout formula: a bar image is
// 2*margin + axisW + runs*minBand wide once that passes minWidth, and an image
// of lines keeps its width and gains a legend row each time a row is full.
func TestLayoutGrowsWithRuns(t *testing.T) {
	prev := 0
	for n := 1; n <= 40; n++ {
		l, err := layoutChart(barSpec(n), measure)
		if err != nil {
			t.Fatal(err)
		}
		want := max(minWidth, 2*margin+50+n*minBand)
		if l.width != want {
			t.Errorf("%d runs give width %d, want %d", n, l.width, want)
		}
		if n > 4 && l.width <= prev {
			t.Errorf("%d runs give width %d, not more than %d", n, l.width, prev)
		}
		prev = l.width
	}

	lines := func(n int) chartLayout {
		s := barSpec(n)
		s.minPlotW, s.plotH, s.footH = curvePlotW, curvePlotH, footH
		l, err := layoutChart(s, measure)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	one, many := lines(1), lines(30)
	if one.width != many.width {
		t.Errorf("line images are %d and %d wide", one.width, many.width)
	}
	if len(one.legendRows) != 1 || len(many.legendRows) < 3 {
		t.Errorf("legend rows %d and %d", len(one.legendRows), len(many.legendRows))
	}
	if got, want := many.height-one.height, (len(many.legendRows)-1)*legendRowH; got != want {
		t.Errorf("extra legend rows add %d px, want %d", got, want)
	}
	if many.captionTop-many.plotTop != one.captionTop-one.plotTop {
		t.Error("legend rows change the plot height")
	}
}

func TestLayoutTooWide(t *testing.T) {
	_, err := layoutChart(barSpec(100), measure)
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(render.MaxSide)) {
		t.Errorf("100 runs give %v, want an error naming the limit", err)
	}
}

func TestRunTable(t *testing.T) {
	l := synthLog(t, "a.csv", 50, 5, 1)
	l.System = mangohud.System{Hardware: "OS, with a comma,CPU,GPU", RAMKiB: 1 << 20, Kernel: "6.1.0-test"}
	l.Notes = []string{"the last line was cut off and skipped"}
	set, err := compare.New([]*mangohud.Log{l, synthLog(t, "b.csv", 50, 5, 2)}, []string{"x", "y"})
	if err != nil {
		t.Fatal(err)
	}
	_, rows := runTable(set)
	if got := strings.Join(rows[0], "|"); got != "x|a.csv|–|50|"+rows[0][4]+"|–|OS, with a comma,CPU,GPU|–|1.0 GiB|6.1.0-test|–" {
		t.Errorf("row = %s", got)
	}
	notes := strings.Join(summaryNotes(set), "\n")
	for _, want := range []string{"x: the last line was cut off and skipped.", "RAM differs: 1048576 KiB (x), 16000000 KiB (y).", "cpufreq governor"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q:\n%s", want, notes)
		}
	}
}
