// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/galpt/mangohud-log-parser/internal/compare"
	"github.com/galpt/mangohud-log-parser/internal/mangohud"
)

// flatLog is a log of 300 frames that all take ms, with every extra column
// held at one value, so that its statistics are known without a calculation.
// extra is a list of "name=value" pairs.
func flatLog(t *testing.T, source string, ms string, extra ...string) *mangohud.Log {
	t.Helper()
	cols, vals := "fps,frametime", "100,"+ms
	for _, e := range extra {
		name, value, _ := strings.Cut(e, "=")
		cols += "," + name
		vals += "," + value
	}
	var b strings.Builder
	b.WriteString("os,cpu,gpu,ram,kernel,driver,cpuscheduler\nTest OS,Test CPU,Test GPU,16000000,6.1.0-test,,performance\n")
	b.WriteString(cols + ",elapsed\n")
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&b, "%s,%d\n", vals, i*10000000)
	}
	l, err := mangohud.Parse(source, strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func flatSet(t *testing.T, names []string, logs ...*mangohud.Log) *compare.Set {
	t.Helper()
	s, err := compare.New(logs, names)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func rowLabels(tb table) []string {
	var out []string
	for _, r := range tb.rows {
		out = append(out, r[0])
	}
	return out
}

// captionOf returns the texts of the figure with each run of white space,
// the line breaks of the wrapping included, made a single space.
func captionOf(t *testing.T, f Figure) string {
	t.Helper()
	var o struct{ Title []struct{ Text string } }
	if err := json.Unmarshal(f.Option, &o); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ti := range o.Title {
		out = append(out, ti.Text)
	}
	return strings.Join(strings.Fields(strings.Join(out, " ")), " ")
}

func TestQuickSummaryPath(t *testing.T) {
	figs := mustBuild(t, "scx_a", "EEVDF")
	if figs[0].Path != "quick-summary.png" {
		t.Errorf("first figure is %q, want quick-summary.png at the top of the output", figs[0].Path)
	}
	if figs[0].Width < minWidth || figs[0].Height < 1 {
		t.Errorf("quick summary is %dx%d", figs[0].Width, figs[0].Height)
	}
}

func TestQuickRowOrder(t *testing.T) {
	all := []string{"cpu_mhz=4000", "cpu_power=50", "gpu_core_clock=1800", "gpu_power=200"}
	set := flatSet(t, nil, flatLog(t, "a.csv", "10", all...), flatLog(t, "b.csv", "8", all...))
	tb, left := quickTable(set, measure)
	want := []string{
		"Average FPS ↑", "1% min FPS ↑", "0.1% min FPS ↑", "97% percentile FPS ↑",
		"Average CPU clock (MHz)", "Peak CPU clock (MHz)",
		"Average GPU core clock (MHz)", "Peak GPU core clock (MHz)",
		"Average CPU power (W)", "Peak CPU power (W)",
		"Average GPU power (W)", "Peak GPU power (W)",
	}
	if got := rowLabels(tb); !slices.Equal(got, want) {
		t.Errorf("rows = %q\nwant %q", got, want)
	}
	if len(left) != 0 {
		t.Errorf("left out %q, want nothing", left)
	}
	if got, want := strings.Join(tb.header, "|"), "Statistic|a|b"; got != want {
		t.Errorf("header = %q, want %q", got, want)
	}
	if got, want := strings.Join(tb.rows[0][1:], "|"), "100.0|125.0"; got != want {
		t.Errorf("average FPS = %q, want %q", got, want)
	}
	if got, want := strings.Join(tb.rows[8][1:], "|"), "50.0|50.0"; got != want {
		t.Errorf("average CPU power = %q, want %q", got, want)
	}
}

// A statistic that no run has is left out of the table, and the caption names
// it once, however many of its rows went.
func TestQuickRowsNoRunHas(t *testing.T) {
	set := flatSet(t, nil,
		flatLog(t, "a.csv", "10", "gpu_core_clock=1800", "gpu_power=200"),
		flatLog(t, "b.csv", "8", "gpu_core_clock=1900", "gpu_power=210"))
	tb, left := quickTable(set, measure)
	want := []string{
		"Average FPS ↑", "1% min FPS ↑", "0.1% min FPS ↑", "97% percentile FPS ↑",
		"Average GPU core clock (MHz)", "Peak GPU core clock (MHz)", "Average GPU power (W)", "Peak GPU power (W)",
	}
	if got := rowLabels(tb); !slices.Equal(got, want) {
		t.Errorf("rows = %q\nwant %q", got, want)
	}
	if want := []string{"CPU clock", "CPU power"}; !slices.Equal(left, want) {
		t.Errorf("left out = %q, want %q", left, want)
	}
	f, err := quickFigure(set, sourceLines(set), measure)
	if err != nil {
		t.Fatal(err)
	}
	if c := captionOf(t, f); !strings.Contains(c, "Left out because no log has the column: CPU clock, CPU power.") {
		t.Errorf("caption does not name what was left out:\n%s", c)
	}
	if strings.Contains(captionOf(t, f), quickNA) {
		t.Error("the caption explains n/a, but no cell has it")
	}
}

func TestQuickMissingValueIsNA(t *testing.T) {
	set := flatSet(t, nil,
		flatLog(t, "a.csv", "10", "cpu_mhz=4000"),
		flatLog(t, "b.csv", "10"))
	tb, left := quickTable(set, measure)
	if slices.Contains(left, "CPU clock") {
		t.Errorf("left out %q, but one run has the column", left)
	}
	for i, want := range []string{"4000", "n/a"} {
		if got := tb.rows[4][1+i]; got != want {
			t.Errorf("average CPU clock of run %d = %q, want %q", i, got, want)
		}
		if tb.bold[4][1+i] {
			t.Errorf("a clock is bold for run %d", i)
		}
	}
	f, err := quickFigure(set, sourceLines(set), measure)
	if err != nil {
		t.Fatal(err)
	}
	if c := captionOf(t, f); !strings.Contains(c, quickNA) {
		t.Errorf("caption lacks the n/a note:\n%s", c)
	}
}

// boldOf returns the bold flags of the four FPS rows, one string of run
// marks per row.
func boldOf(tb table) []string {
	var out []string
	for _, row := range tb.bold[:4] {
		var s strings.Builder
		for _, b := range row[1:] {
			if b {
				s.WriteByte('B')
			} else {
				s.WriteByte('-')
			}
		}
		out = append(out, s.String())
	}
	return out
}

func TestQuickBold(t *testing.T) {
	cases := []struct {
		name string
		logs []*mangohud.Log
		want string
	}{
		// 8 ms is 125 FPS, 10 ms is 100 FPS, and 12.5 ms is 80.
		{"best", []*mangohud.Log{flatLog(t, "a.csv", "10"), flatLog(t, "b.csv", "8"), flatLog(t, "c.csv", "12.5")}, "-B-"},
		// 10.0001 ms is 99.999 FPS, which shows as 100.0 like 10 ms does.
		{"tie at the shown precision", []*mangohud.Log{flatLog(t, "a.csv", "10"), flatLog(t, "b.csv", "10.0001"), flatLog(t, "c.csv", "12.5")}, "BB-"},
		{"all equal", []*mangohud.Log{flatLog(t, "a.csv", "10"), flatLog(t, "b.csv", "10")}, "BB"},
		{"one run", []*mangohud.Log{flatLog(t, "a.csv", "10")}, "-"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set := flatSet(t, nil, c.logs...)
			tb, _ := quickTable(set, measure)
			for i, got := range boldOf(tb) {
				if got != c.want {
					t.Errorf("row %d bold = %q, want %q", i, got, c.want)
				}
			}
			f, err := quickFigure(set, sourceLines(set), measure)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := strings.Contains(captionOf(t, f), quickBold), strings.Contains(c.want, "B"); got != want {
				t.Errorf("caption explains bold = %v, want %v", got, want)
			}
		})
	}
}

// Only the best cells of the FPS rows are drawn in the bold face, apart from
// the title and the header.
func TestQuickBoldIsDrawn(t *testing.T) {
	set := flatSet(t, []string{"slow", "fast"}, flatLog(t, "a.csv", "10", "cpu_mhz=4000"), flatLog(t, "b.csv", "8", "cpu_mhz=4100"))
	f, err := quickFigure(set, sourceLines(set), measure)
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		Title []struct {
			Text      string
			TextStyle struct{ FontWeight string }
		}
	}
	if err := json.Unmarshal(f.Option, &o); err != nil {
		t.Fatal(err)
	}
	var bold []string
	for _, ti := range o.Title {
		if ti.TextStyle.FontWeight == "bold" {
			bold = append(bold, ti.Text)
		}
	}
	want := []string{"Quick summary", "Statistic", "slow", "fast", "125.0", "125.0", "125.0", "125.0"}
	if !slices.Equal(bold, want) {
		t.Errorf("bold texts = %q, want %q", bold, want)
	}
}

func TestQuickHeaderWrapsLongLabels(t *testing.T) {
	long := "a_scheduler_name_that_is_far_wider_than_a_column should wrap"
	set := flatSet(t, []string{long, "b"}, flatLog(t, "a.csv", "10"), flatLog(t, "b.csv", "8"))
	tb, _ := quickTable(set, measure)
	if !strings.Contains(tb.header[1], "\n") {
		t.Errorf("header %q is not wrapped", tb.header[1])
	}
	if strings.Join(strings.Fields(tb.header[1]), "") != strings.Join(strings.Fields(long), "") {
		t.Errorf("wrapping %q changed its text: %q", long, tb.header[1])
	}
	for _, line := range strings.Split(tb.header[1], "\n") {
		if w := measure(line, bodySize, true); w > quickHeaderW+bodySize*0.6 {
			t.Errorf("line %q is %.0f px wide, wider than %d", line, w, quickHeaderW)
		}
	}
	f, err := quickFigure(set, sourceLines(set), measure)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(f.Option), `\n`) {
		t.Error("the figure has no multi-line header")
	}
}

func TestQuickWidthGrowsAndIsLimited(t *testing.T) {
	prev := 0
	for _, n := range []int{1, 4, 8, 12} {
		var names []string
		var logs []*mangohud.Log
		for i := range n {
			names = append(names, fmt.Sprintf("scheduler_%d", i))
			logs = append(logs, flatLog(t, fmt.Sprintf("r%d.csv", i), "10"))
		}
		set := flatSet(t, names, logs...)
		f, err := quickFigure(set, sourceLines(set), measure)
		if err != nil {
			t.Fatal(err)
		}
		if f.Width <= prev {
			t.Errorf("%d runs give width %d, not more than %d", n, f.Width, prev)
		}
		prev = f.Width
	}
	var names []string
	var logs []*mangohud.Log
	for i := range 100 {
		names = append(names, fmt.Sprintf("scheduler_%d", i))
		logs = append(logs, flatLog(t, fmt.Sprintf("r%d.csv", i), "10"))
	}
	set := flatSet(t, names, logs...)
	if _, err := quickFigure(set, sourceLines(set), measure); err == nil || !strings.Contains(err.Error(), "wide") {
		t.Errorf("100 runs give %v, want an error about the width", err)
	}
}
