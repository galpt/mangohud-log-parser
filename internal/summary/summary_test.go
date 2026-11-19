// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package summary

import (
	"math"
	"math/rand"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/galpt/mangohud-log-parser/internal/mangohud"
)

// newLog parses a synthetic log. The columns map holds the values of each
// column besides elapsed, which advances by 10 ms per sample, and fps, which
// is derived from the frame times.
func newLog(t *testing.T, frametimes []float64, columns map[string][]float64) *mangohud.Log {
	t.Helper()
	names := []string{"fps", "frametime"}
	for name := range columns {
		names = append(names, name)
	}
	slices.Sort(names[2:])
	text := "os,cpu,gpu,ram,kernel,driver,cpuscheduler\nOS,CPU,GPU,1,k,,g\n" + strings.Join(names, ",") + ",elapsed\n"
	for i, ft := range frametimes {
		fields := []string{num(1000 / ft), num(ft)}
		for _, name := range names[2:] {
			fields = append(fields, num(columns[name][i]))
		}
		fields = append(fields, strconv.Itoa(i*10_000_000))
		text += strings.Join(fields, ",") + "\n"
	}
	l, err := mangohud.Parse("t.csv", strings.NewReader(text))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return l
}

func num(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func stat(t *testing.T, r *Result, key string) Stat {
	t.Helper()
	for _, s := range r.Stats {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("no statistic %q in %v", key, keys(r))
	return Stat{}
}

func keys(r *Result) []string {
	var out []string
	for _, s := range r.Stats {
		out = append(out, s.Key)
	}
	return out
}

func near(got, want float64) bool { return math.Abs(got-want) <= 1e-9*math.Max(1, math.Abs(want)) }

// referenceIndex is the percentile index worked out another way than the
// package does. A float32 times an integer below 2^24 is exact in float64,
// so rounding that product once to float32 gives the float32 product.
func referenceIndex(p float64, n int) int {
	product := float64(float32(p)) * float64(float32(n))
	return int(math.Trunc(float64(float32(product)) - 1))
}

func TestPercentileIndex(t *testing.T) {
	cases := []struct {
		p    float32
		n    int
		want int64
	}{
		{pctMid, 1, 0}, {pctLow, 1, 0}, {pctHigh, 1, 0},
		{pctMid, 99, 0}, {pctMid, 100, 0}, {pctMid, 101, 0},
		{pctLow, 999, 0}, {pctLow, 1000, 0}, {pctLow, 1001, 0},
		{pctMid, 999, 8}, {pctMid, 1000, 9}, {pctMid, 1001, 9},
		{pctHigh, 100, 96}, {pctHigh, 1000, 969},
		{pctLow, 12345, 11}, {pctMid, 12345, 122}, {pctHigh, 12345, 11973},
	}
	for _, tc := range cases {
		if got := percentileIndex(tc.p, tc.n); got != tc.want {
			t.Errorf("percentileIndex(%v, %d) = %d, want %d", tc.p, tc.n, got, tc.want)
		}
	}
}

func TestPercentiles(t *testing.T) {
	for _, n := range []int{1, 99, 100, 101, 999, 1000, 1001, 12345} {
		// The frame times are 1..n in a shuffled order, so that the slowest
		// frame is n ms and the frame at index i of the sorted list is n-i.
		ft := make([]float64, n)
		for i := range ft {
			ft[i] = float64(i + 1)
		}
		rand.New(rand.NewSource(int64(n))).Shuffle(n, func(i, j int) { ft[i], ft[j] = ft[j], ft[i] })
		r := Compute(newLog(t, ft, nil))
		for _, tc := range []struct {
			key string
			p   float64
		}{{"min_fps_0_1pct", 0.001}, {"min_fps_1pct", 0.01}, {"percentile_fps_97", 0.97}} {
			idx := referenceIndex(tc.p, n)
			if idx < 0 || idx >= n {
				t.Fatalf("reference index %d out of range for n = %d", idx, n)
			}
			if got, want := stat(t, r, tc.key).Value, 1000/float64(n-idx); !near(got, want) {
				t.Errorf("n = %d, %s = %v, want %v (index %d)", n, tc.key, got, want, idx)
			}
		}
	}
}

func TestMeansAndPeaks(t *testing.T) {
	ft := []float64{10, 20, 40, 20}
	l := newLog(t, ft, map[string][]float64{
		"cpu_load":      {10, 20, 30, 40},
		"gpu_load":      {90, 100, 80, 70},
		"cpu_temp":      {60, 61, 61, 61},
		"gpu_temp":      {50, 51, 51, 52},
		"gpu_vram_used": {1.5, 2, 2.5, 2},
		"ram_used":      {10, 10, 11, 11},
		"swap_used":     {0, 0, 0, 0},
		"cpu_power":     {100, 120, 140, 160},
		"cpu_mhz":       {5000, 5100, 5200, 5300},
	})
	r := Compute(l)
	want := map[string]float64{
		"avg_fps":            1000 / 22.5,
		"avg_frametime":      22.5,
		"avg_cpu_load":       25,
		"peak_cpu_load":      40,
		"avg_gpu_load":       85,
		"peak_gpu_load":      100,
		"avg_cpu_temp":       60.75,
		"peak_cpu_temp":      61,
		"avg_gpu_temp":       51,
		"peak_gpu_temp":      52,
		"avg_gpu_vram_used":  2,
		"peak_gpu_vram_used": 2.5,
		"avg_ram_used":       10.5,
		"peak_ram_used":      11,
		"avg_swap_used":      0,
		"peak_swap_used":     0,
		"avg_cpu_power":      130,
		"peak_cpu_power":     160,
		"avg_cpu_mhz":        5150,
		"peak_cpu_mhz":       5300,
	}
	for key, v := range want {
		if got := stat(t, r, key).Value; !near(got, v) {
			t.Errorf("%s = %v, want %v", key, got, v)
		}
	}
	if len(r.Stats) != len(want)+3 {
		t.Errorf("%d statistics, want %d: %v", len(r.Stats), len(want)+3, keys(r))
	}
	if r.Samples != 4 || r.Duration != 30*time.Millisecond {
		t.Errorf("Samples = %d, Duration = %v", r.Samples, r.Duration)
	}
}

func TestMetadata(t *testing.T) {
	ft := []float64{8, 9}
	cols := map[string][]float64{}
	for _, name := range meanPeakColumns {
		cols[name] = []float64{1.23456, 2.34567}
	}
	r := Compute(newLog(t, ft, cols))
	if len(r.Stats) != 5+2*len(meanPeakColumns) {
		t.Fatalf("%d statistics", len(r.Stats))
	}
	seen := map[string]bool{}
	for i, s := range r.Stats {
		if seen[s.Key] {
			t.Errorf("key %s repeats", s.Key)
		}
		seen[s.Key] = true
		if i > 0 && s.Order <= r.Stats[i-1].Order {
			t.Errorf("%s has order %d after %d", s.Key, s.Order, r.Stats[i-1].Order)
		}
		if s.Label == "" {
			t.Errorf("%s has no label", s.Key)
		}
	}

	type meta struct {
		label string
		unit  string
		dir   mangohud.Direction
		dec   int
		text  string
	}
	cases := map[string]meta{
		"avg_fps":             {"Average FPS", "", mangohud.HigherIsBetter, 1, "117.6"},
		"min_fps_1pct":        {"1% min FPS", "", mangohud.HigherIsBetter, 1, "111.1"},
		"min_fps_0_1pct":      {"0.1% min FPS", "", mangohud.HigherIsBetter, 1, "111.1"},
		"percentile_fps_97":   {"97% percentile FPS", "", mangohud.HigherIsBetter, 1, "111.1"},
		"avg_frametime":       {"Average frame time", "ms", mangohud.LowerIsBetter, 2, "8.50"},
		"avg_cpu_load":        {"Average CPU load", "%", mangohud.NoDirection, 1, "1.8"},
		"peak_gpu_temp":       {"Peak GPU temperature", "°C", mangohud.NoDirection, 1, "2.3"},
		"avg_gpu_vram_used":   {"Average GPU VRAM used", "GiB", mangohud.NoDirection, 2, "1.79"},
		"peak_process_rss":    {"Peak Process RSS", "GiB", mangohud.NoDirection, 2, "2.35"},
		"avg_cpu_power":       {"Average CPU power", "W", mangohud.NoDirection, 1, "1.8"},
		"peak_gpu_core_clock": {"Peak GPU core clock", "MHz", mangohud.NoDirection, 0, "2"},
	}
	for key, want := range cases {
		s := stat(t, r, key)
		got := meta{s.Label, s.Unit, s.Direction, s.Decimals, s.Text()}
		if got != want {
			t.Errorf("%s = %+v, want %+v", key, got, want)
		}
	}
}

func TestOrderIsSharedAcrossLogs(t *testing.T) {
	ft := []float64{10, 12}
	small := Compute(newLog(t, ft, map[string][]float64{"gpu_temp": {50, 51}}))
	big := Compute(newLog(t, ft, map[string][]float64{"gpu_temp": {50, 51}, "cpu_power": {1, 2}, "cpu_load": {3, 4}}))
	if stat(t, small, "avg_gpu_temp").Order != stat(t, big, "avg_gpu_temp").Order {
		t.Error("the order of a statistic depends on the other columns")
	}
	if !(stat(t, big, "avg_cpu_load").Order < stat(t, big, "avg_gpu_temp").Order && stat(t, big, "avg_gpu_temp").Order < stat(t, big, "avg_cpu_power").Order) {
		t.Errorf("loads, temperatures and power are out of order: %v", keys(big))
	}
}

func TestAbsentColumns(t *testing.T) {
	r := Compute(newLog(t, []float64{10, 10}, nil))
	want := []string{"avg_fps", "min_fps_1pct", "min_fps_0_1pct", "percentile_fps_97", "avg_frametime"}
	if !reflect.DeepEqual(keys(r), want) {
		t.Errorf("keys = %v, want %v", keys(r), want)
	}
	r = Compute(newLog(t, []float64{10, 10}, map[string][]float64{"mystery": {1, 2}, "elapsed_like": {1, 2}}))
	if len(r.Stats) != 5 {
		t.Errorf("unknown columns got statistics: %v", keys(r))
	}
}

func TestUnsortedInput(t *testing.T) {
	ft := []float64{30, 5, 12, 12, 80, 7, 7, 21, 16, 9, 40, 11}
	l := newLog(t, ft, nil)
	before := slices.Clone(l.Series("frametime"))
	got := Compute(l)
	if !reflect.DeepEqual(l.Series("frametime"), before) {
		t.Error("Compute changed the frame times of the log")
	}
	sorted := slices.Clone(ft)
	slices.Sort(sorted)
	if want := Compute(newLog(t, sorted, nil)); !reflect.DeepEqual(got.Stats, want.Stats) {
		t.Errorf("an unsorted log gives %v, the sorted one %v", got.Stats, want.Stats)
	}
	// With 12 samples every percentile reads the slowest frame but the 97th,
	// which reads index 10 of the descending list, the second fastest frame.
	if v := stat(t, got, "min_fps_1pct").Value; !near(v, 1000/80.0) {
		t.Errorf("1%% min = %v", v)
	}
	if v := stat(t, got, "percentile_fps_97").Value; !near(v, 1000/7.0) {
		t.Errorf("97%% percentile = %v, want %v", v, 1000/7.0)
	}
}

func TestEqualFrametimes(t *testing.T) {
	ft := make([]float64, 250)
	for i := range ft {
		ft[i] = 16
	}
	r := Compute(newLog(t, ft, nil))
	for _, key := range []string{"avg_fps", "min_fps_1pct", "min_fps_0_1pct", "percentile_fps_97"} {
		if v := stat(t, r, key).Value; !near(v, 62.5) {
			t.Errorf("%s = %v, want 62.5", key, v)
		}
	}
	if v := stat(t, r, "avg_frametime").Value; !near(v, 16) {
		t.Errorf("avg_frametime = %v", v)
	}
}

func TestSingleSample(t *testing.T) {
	r := Compute(newLog(t, []float64{4}, map[string][]float64{"cpu_temp": {70}}))
	if r.Samples != 1 || r.Duration != 0 {
		t.Errorf("Samples = %d, Duration = %v", r.Samples, r.Duration)
	}
	for _, key := range []string{"avg_fps", "min_fps_1pct", "min_fps_0_1pct", "percentile_fps_97"} {
		if v := stat(t, r, key).Value; !near(v, 250) {
			t.Errorf("%s = %v, want 250", key, v)
		}
	}
}

func TestNonFiniteValues(t *testing.T) {
	text := "os,cpu,gpu,ram,kernel,driver,cpuscheduler\nO,C,G,1,k,,g\nfps,frametime,cpu_load,elapsed\ninf,0,nan,1\n5,inf,4,2\n"
	l, err := mangohud.Parse("t.csv", strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	r := Compute(l)
	if len(r.Stats) != 7 {
		t.Errorf("keys = %v", keys(r))
	}
}

func TestEmptyLog(t *testing.T) {
	for _, l := range []*mangohud.Log{{}, {Elapsed: []int64{1}}, {Columns: []mangohud.Column{{Name: "frametime"}}, Data: [][]float64{{1, 2}}, Elapsed: []int64{1}}} {
		if r := Compute(l); r.Samples != 0 || len(r.Stats) != 0 {
			t.Errorf("Compute(%+v) = %+v", l, r)
		}
	}
}
