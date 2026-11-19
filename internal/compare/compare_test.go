// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package compare

import (
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/galpt/mangohud-log-parser/internal/mangohud"
)

// column returns the registry description of a column through a real parse,
// so that the tests do not repeat labels and units.
func columns(t *testing.T, names ...string) []mangohud.Column {
	t.Helper()
	head := strings.Join(append([]string{"fps", "frametime"}, names...), ",") + ",elapsed"
	row := strings.Repeat("1,", len(names)+2) + "1"
	text := "os,cpu,gpu,ram,kernel,driver,cpuscheduler\nO,C,G,1,k,,g\n" + head + "\n" + row + "\n"
	l, err := mangohud.Parse("t.csv", strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return l.Columns
}

// logOf makes a log with the given frame times. The extra columns are
// described by the registry and hold the same value in every sample.
func logOf(t *testing.T, source string, frametimes []float64, extra map[string]float64) *mangohud.Log {
	t.Helper()
	var names []string
	for name := range extra {
		names = append(names, name)
	}
	l := &mangohud.Log{Source: source, Versioned: true, Version: "0.8.4", System: sampleSystem()}
	l.Columns = columns(t, names...)
	l.Data = make([][]float64, len(l.Columns))
	for i, ft := range frametimes {
		l.Elapsed = append(l.Elapsed, int64(i)*10_000_000)
		for j, c := range l.Columns {
			switch c.Name {
			case "fps":
				l.Data[j] = append(l.Data[j], 1000/ft)
			case "frametime":
				l.Data[j] = append(l.Data[j], ft)
			default:
				l.Data[j] = append(l.Data[j], extra[c.Name])
			}
		}
	}
	return l
}

func sampleSystem() mangohud.System {
	return mangohud.System{OS: "Test OS", CPU: "Test CPU", GPU: "Test GPU", RAMKiB: 16000000, Kernel: "6.1.0-test", Governor: "performance"}
}

func mustNew(t *testing.T, logs []*mangohud.Log, names []string) *Set {
	t.Helper()
	s, err := New(logs, names)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func labels(s *Set) []string {
	var out []string
	for _, r := range s.Runs() {
		out = append(out, r.Label)
	}
	return out
}

func TestDefaultNames(t *testing.T) {
	cases := []struct {
		source string
		want   string
	}{
		{"game_2000-01-01_00-00-00.csv", "game_2000-01-01_00-00-00"},
		{"/some/dir/game.csv", "game"},
		{"rel/dir/game.CSV", "game"},
		{"game.txt", "game.txt"},
		{"game.csv.csv", "game.csv"},
		{".csv", ".csv"},
		{"dir.csv/game", "game"},
		{"game", "game"},
	}
	for _, tc := range cases {
		if got := defaultName(tc.source); got != tc.want {
			t.Errorf("defaultName(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}

	var logs []*mangohud.Log
	for _, src := range []string{"a/one.csv", "b/two.csv", "c/one.csv", "d/ONE.csv"} {
		logs = append(logs, logOf(t, src, []float64{10}, nil))
	}
	s := mustNew(t, logs, nil)
	if want := []string{"one", "two", "one #3", "ONE"}; !reflect.DeepEqual(labels(s), want) {
		t.Errorf("labels = %q, want %q", labels(s), want)
	}
	for i, r := range s.Runs() {
		if r.Log != logs[i] || r.Stats == nil || r.Stats.Samples != 1 {
			t.Errorf("run %d = %+v", i, r)
		}
	}
}

func TestDataNames(t *testing.T) {
	logs := []*mangohud.Log{logOf(t, "a.csv", []float64{10}, nil), logOf(t, "b.csv", []float64{10}, nil), logOf(t, "c.csv", []float64{10}, nil)}
	s := mustNew(t, logs, []string{"scx_flow", "EEVDF", "scx_lavd"})
	if want := []string{"scx_flow", "EEVDF", "scx_lavd"}; !reflect.DeepEqual(labels(s), want) {
		t.Errorf("labels = %q", labels(s))
	}
	if s.Runs()[1].Log != logs[1] {
		t.Error("runs are not in the order of the logs")
	}
	if _, err := New(logs, []string{"a", "b", "a"}); err == nil {
		t.Error("New accepted duplicate names")
	}
	// Names are taken as typed, so names that differ only in case are two
	// runs and keep their spelling.
	verbatim := []string{"EEVDF", "eevdf", "scx_flow_5.2.28"}
	if s := mustNew(t, logs, verbatim); !reflect.DeepEqual(labels(s), verbatim) {
		t.Errorf("labels = %q, want %q", labels(s), verbatim)
	}
	if _, err := New(logs, []string{"a"}); err == nil {
		t.Error("New accepted the wrong number of names")
	}
	if _, err := New(logs, []string{}); err == nil {
		t.Error("New accepted an empty list of names")
	}
}

func TestNewRefusesBadInput(t *testing.T) {
	if _, err := New(nil, nil); err == nil {
		t.Error("New accepted no logs")
	}
	if _, err := New([]*mangohud.Log{logOf(t, "a.csv", []float64{10}, nil), nil}, nil); err == nil || !strings.Contains(err.Error(), "log 1 is nil") {
		t.Errorf("error = %v", err)
	}
}

func TestValidateNames(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		count int
		want  string
	}{
		{"more names", []string{"a", "b"}, 1, "-dataname gives 2 names but only 1 log file; a log records one run, so give one name per file, in the same order"},
		{"more names than three", []string{"a", "b", "c", "d"}, 3, "-dataname gives 4 names but only 3 log files; a log records one run, so give one name per file, in the same order"},
		{"fewer names", []string{"a"}, 2, "-dataname gives 1 name for 2 log files; a log records one run, so give one name per file, in the same order"},
		{"no names", nil, 3, "-dataname gives 0 names for 3 log files; a log records one run, so give one name per file, in the same order"},
		{"empty name", []string{"a", ""}, 2, "-dataname gives an empty name for run 2; give every run a name"},
		{"blank name", []string{" \t", "b"}, 2, "-dataname gives an empty name for run 1; give every run a name"},
		{"duplicate", []string{"a", "b", "a"}, 3, `-dataname gives "a" for both run 1 and run 3; give each run its own name`},
		{"exact duplicate", []string{"scx_flow_5.2.28", "x", "scx_flow_5.2.28"}, 3, `-dataname gives "scx_flow_5.2.28" for both run 1 and run 3; give each run its own name`},
		{"case differs", []string{"EEVDF", "x", "eevdf"}, 3, ""},
		{"fold differs", []string{"s", "ſ", "S"}, 3, ""},
		{"fine", []string{"a", "b", "A b", "a "}, 4, ""},
		{"one", []string{"x"}, 1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNames(tc.names, tc.count)
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("error = %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFreeLabel(t *testing.T) {
	cases := []struct {
		taken []string
		name  string
		k     int
		want  string
	}{
		{nil, "a", 1, "a"},
		{[]string{"a"}, "a", 2, "a #2"},
		{[]string{"a"}, "A", 2, "A"},
		{[]string{"a", "a #2"}, "a", 2, "a #2-2"},
		{[]string{"a", "a #2", "a #2-2", "a #2-3"}, "a", 2, "a #2-4"},
		{[]string{"a", "a #2", "a #2-2", "A #2-3"}, "a", 2, "a #2-3"},
		{[]string{"a #2"}, "a #2", 5, "a #2 #5"},
	}
	for _, tc := range cases {
		taken := map[string]bool{}
		for _, l := range tc.taken {
			taken[l] = true
		}
		if got := freeLabel(taken, tc.name, tc.k); got != tc.want {
			t.Errorf("freeLabel(%q, %q, %d) = %q, want %q", tc.taken, tc.name, tc.k, got, tc.want)
		}
	}
}

// randomName builds a name from fragments that appear in the labels the rules
// build, so that names collide with the labels of other runs.
func randomName(rng *rand.Rand) string {
	fragments := []string{"EEVDF", "eevdf", "EeVdF", "x", "X", " ", "  ", "(", ")", "#", "1", "2", "-2", " #1", " #2", " #3", " #3-2", "ſ", "s", "S"}
	var b strings.Builder
	for n := 1 + rng.Intn(4); n > 0; n-- {
		b.WriteString(fragments[rng.Intn(len(fragments))])
	}
	return b.String()
}

// TestLabelsAreUniqueForAnyNames draws many random sources, whose default names
// collide often and imitate numbered labels, and checks that no two labels are
// the same string and that each label starts with its name. The fragments
// that differ only in case, or that case folding would join like "ſ" and "s",
// are distinct names and must stay distinct labels.
func TestLabelsAreUniqueForAnyNames(t *testing.T) {
	rng := rand.New(rand.NewSource(4321))
	for i := 0; i < 5000; i++ {
		var logs []*mangohud.Log
		var names []string
		for n := rng.Intn(8) + 1; n > 0; n-- {
			name := randomName(rng)
			names = append(names, name)
			logs = append(logs, &mangohud.Log{Source: "dir/" + name + ".csv", Columns: columns(t), Data: [][]float64{{1}, {1}}, Elapsed: []int64{0}})
		}
		s := mustNew(t, logs, nil)
		seen := map[string]bool{}
		for j, r := range s.Runs() {
			if seen[r.Label] {
				t.Fatalf("label %q is not unique in %q (names %q)", r.Label, labels(s), names)
			}
			seen[r.Label] = true
			if !slices.Contains(labels(s)[:j], names[j]) && r.Label != names[j] {
				t.Fatalf("label %q was changed from %q, which no earlier run had taken", r.Label, names[j])
			}
			if !strings.HasPrefix(r.Label, defaultName(logs[j].Source)) {
				t.Fatalf("label %q does not start with its name %q", r.Label, names[j])
			}
		}
	}
}

func TestMetricsAcrossRuns(t *testing.T) {
	a := logOf(t, "a.csv", []float64{10, 20}, map[string]float64{"cpu_load": 40, "gpu_temp": 70})
	b := logOf(t, "b.csv", []float64{20, 20}, map[string]float64{"gpu_temp": 72, "cpu_power": 100})
	c := logOf(t, "c.csv", []float64{5}, nil)
	s := mustNew(t, []*mangohud.Log{a, b, c}, nil)

	var keys []string
	byKey := map[string]Metric{}
	for _, m := range s.Metrics() {
		keys = append(keys, m.Key)
		byKey[m.Key] = m
		if len(m.Cells) != 3 {
			t.Errorf("%s has %d cells", m.Key, len(m.Cells))
		}
	}
	want := []string{
		"avg_fps", "min_fps_1pct", "min_fps_0_1pct", "percentile_fps_97", "avg_frametime",
		"avg_cpu_load", "peak_cpu_load", "avg_gpu_temp", "peak_gpu_temp", "avg_cpu_power", "peak_cpu_power",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}

	fps := byKey["avg_fps"]
	if fps.Label != "Average FPS" || fps.Direction != mangohud.HigherIsBetter || fps.Decimals != 1 {
		t.Errorf("avg_fps = %+v", fps)
	}
	for i, wantText := range []string{"66.7", "50.0", "200.0"} {
		if c := fps.Cells[i]; !c.Valid || c.Text != wantText {
			t.Errorf("avg_fps cell %d = %+v, want text %s", i, c, wantText)
		}
	}
	temp := byKey["avg_gpu_temp"]
	if temp.Unit != "°C" || !temp.Cells[0].Valid || !temp.Cells[1].Valid || temp.Cells[2].Valid {
		t.Errorf("avg_gpu_temp = %+v", temp)
	}
	if temp.Cells[0].Value != 70 || temp.Cells[1].Value != 72 || temp.Cells[0].Text != "70.0" {
		t.Errorf("avg_gpu_temp cells = %+v", temp.Cells)
	}
	load := byKey["peak_cpu_load"]
	if !load.Cells[0].Valid || load.Cells[1].Valid || load.Cells[2].Valid || load.Cells[1].Text != "" {
		t.Errorf("peak_cpu_load cells = %+v", load.Cells)
	}
	if power := byKey["avg_cpu_power"]; power.Cells[0].Valid || !power.Cells[1].Valid {
		t.Errorf("avg_cpu_power cells = %+v", power.Cells)
	}
}

func TestTimelineIsTheUnionInFirstSeenOrder(t *testing.T) {
	a := logOf(t, "a.csv", []float64{10}, map[string]float64{"cpu_load": 1})
	b := logOf(t, "b.csv", []float64{10}, map[string]float64{"gpu_temp": 1, "cpu_load": 1})
	c := logOf(t, "c.csv", []float64{10}, map[string]float64{"cpu_power": 1})
	s := mustNew(t, []*mangohud.Log{a, b, c}, nil)
	var names []string
	for _, col := range s.Timeline() {
		names = append(names, col.Name)
	}
	if want := []string{"fps", "frametime", "cpu_load", "gpu_temp", "cpu_power"}; !reflect.DeepEqual(names, want) {
		t.Errorf("timeline = %v, want %v", names, want)
	}
	if s.Timeline()[2].Label != "CPU load" || s.Timeline()[2].Unit != "%" {
		t.Errorf("column = %+v", s.Timeline()[2])
	}
}

func TestDifferences(t *testing.T) {
	mk := func(source string, edit func(l *mangohud.Log)) *mangohud.Log {
		l := logOf(t, source, []float64{10}, nil)
		edit(l)
		return l
	}
	same := func(*mangohud.Log) {}
	cases := []struct {
		name string
		logs []*mangohud.Log
		want []string
	}{
		{"one run", []*mangohud.Log{mk("a.csv", same)}, nil},
		{"identical runs", []*mangohud.Log{mk("a.csv", same), mk("b.csv", same), mk("c.csv", same)}, nil},
		{"every field", []*mangohud.Log{
			mk("a.csv", same),
			mk("b.csv", func(l *mangohud.Log) {
				l.Version = "0.7.2"
				l.System.CPU = "Other CPU"
				l.System.GPU = "Other GPU"
				l.System.RAMKiB = 8000000
				l.System.Kernel = "6.2.0-test"
				l.System.Governor = "powersave"
			}),
			mk("c.csv", same),
		}, []string{
			"MangoHud version differs: 0.8.4 (a, c), 0.7.2 (b)",
			"CPU differs: Test CPU (a, c), Other CPU (b)",
			"GPU differs: Test GPU (a, c), Other GPU (b)",
			"RAM differs: 16000000 KiB (a, c), 8000000 KiB (b)",
			"Kernel differs: 6.1.0-test (a, c), 6.2.0-test (b)",
			"Governor differs: performance (a, c), powersave (b)",
		}},
		{"unversioned and no governor", []*mangohud.Log{
			mk("a.csv", same),
			mk("b.csv", func(l *mangohud.Log) { l.Versioned, l.Version, l.System.Governor = false, "", "" }),
		}, []string{
			"MangoHud version differs: 0.8.4 (a), unknown (b)",
			"Governor differs: performance (a), unknown (b)",
		}},
		{"os alone is not a difference", []*mangohud.Log{
			mk("a.csv", same),
			mk("b.csv", func(l *mangohud.Log) { l.System.OS = "Another OS"; l.System.Driver = "drv" }),
		}, nil},
		{"unsplit hardware is left out of cpu and gpu", []*mangohud.Log{
			mk("a.csv", same),
			mk("b.csv", func(l *mangohud.Log) {
				l.System.OS, l.System.CPU, l.System.GPU, l.System.Hardware = "", "", "", "Test OS,Test CPU,Test GPU, Mobile"
			}),
		}, nil},
		{"unsplit hardware still counts for the rest", []*mangohud.Log{
			mk("a.csv", same),
			mk("b.csv", func(l *mangohud.Log) {
				l.System.OS, l.System.CPU, l.System.GPU, l.System.Hardware = "", "", "", "x,y,z,w"
				l.System.Kernel = "6.3.0"
			}),
			mk("c.csv", func(l *mangohud.Log) { l.System.CPU = "Other CPU" }),
		}, []string{
			"CPU differs: Test CPU (a), Other CPU (c)",
			"Kernel differs: 6.1.0-test (a, c), 6.3.0 (b)",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustNew(t, tc.logs, []string{"a", "b", "c"}[:len(tc.logs)])
			if got := s.Differences(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Differences =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}
