// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/galpt/mangohud-log-parser/internal/compare"
	"github.com/galpt/mangohud-log-parser/internal/figure"
	"github.com/galpt/mangohud-log-parser/internal/mangohud"
	"github.com/galpt/mangohud-log-parser/internal/render"
)

// writeLog writes a made-up versioned log of n frames and returns its path.
// The frame times step through a fixed pattern with a slow frame now and
// then, and the other columns change slowly, so every chart has something to
// draw.
func writeLog(t *testing.T, dir, name string, n int, base float64) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("v1\n0.8.4\n---------------------SYSTEM INFO---------------------\n")
	b.WriteString("os,cpu,gpu,ram,kernel,driver,cpuscheduler\nTest OS,Test CPU,Test GPU,16000000,6.1.0-test,,performance\n")
	b.WriteString("--------------------FRAME METRICS--------------------\n")
	b.WriteString("fps,frametime,cpu_load,cpu_power,gpu_load,cpu_temp,gpu_temp,gpu_core_clock,gpu_mem_clock,gpu_vram_used,gpu_power,ram_used,swap_used,process_rss,cpu_mhz,elapsed\n")
	elapsed := int64(1_000_000_000)
	for i := range n {
		ft := base + float64(i%7)*0.25
		if i%113 == 0 {
			ft *= 4
		}
		elapsed += int64(ft * 1e6)
		fmt.Fprintf(&b, "%.6g,%.6g,%d,%d,%d,%d,%d,2500,7000,5.5,%d,10.5,0,1.25,%d,%d\n",
			1000/ft, ft, 30+i/50%10, 40+i%3, 95+i%5, 60+i/100%5, 55+i/100%4, 250+i%10, 4800+i%200, elapsed)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeLogs(t *testing.T, n int) []string {
	t.Helper()
	dir := t.TempDir()
	var out []string
	for i := range n {
		out = append(out, writeLog(t, dir, fmt.Sprintf("Game_2030-01-0%d_00-00-00.csv", i+1), 400+100*i, 6+float64(i)))
	}
	return out
}

func runCmd(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// chartDirs lists the charts-<digits> directories in the working directory.
func chartDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`^charts-[0-9]+$`)
	var dirs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "charts-") {
			if !e.IsDir() || !pattern.MatchString(e.Name()) {
				t.Errorf("unexpected entry %s", e.Name())
			}
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

// predicted builds the figures the command should have written, with the
// same measurement, to learn their sizes and options.
func predicted(t *testing.T, logs, names []string) map[string]figure.Figure {
	t.Helper()
	var parsed []*mangohud.Log
	for _, p := range logs {
		l, err := mangohud.ParseFile(p)
		if err != nil {
			t.Fatal(err)
		}
		l.Source = filepath.Base(p)
		parsed = append(parsed, l)
	}
	set, err := compare.New(parsed, names)
	if err != nil {
		t.Fatal(err)
	}
	rd, err := render.New()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	figs, err := figure.Build(set, rd.TextWidth)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]figure.Figure{}
	for _, f := range figs {
		out[f.Path] = f
	}
	return out
}

// tree lists every directory and file under dir, relative to it, with
// forward slashes and a trailing one on directories.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == dir {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			rel += "/"
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// wantFiles are the files of a run of the synthetic logs, in the order the
// command prints them.
func wantFiles(eevdf bool) []string {
	summary := []string{
		"average-fps", "1pct-min-fps", "0-1pct-min-fps", "97pct-percentile-fps", "average-frame-time",
		"average-cpu-load", "peak-cpu-load", "average-gpu-load", "peak-gpu-load",
		"average-cpu-temperature", "peak-cpu-temperature", "average-gpu-temperature", "peak-gpu-temperature",
		"average-gpu-vram-used", "peak-gpu-vram-used", "average-ram-used", "peak-ram-used",
		"average-swap-used", "peak-swap-used", "average-process-rss", "peak-process-rss",
		"average-cpu-power", "peak-cpu-power", "average-gpu-power", "peak-gpu-power",
		"average-cpu-clock", "peak-cpu-clock", "average-gpu-core-clock", "peak-gpu-core-clock",
		"average-gpu-memory-clock", "peak-gpu-memory-clock",
	}
	if eevdf {
		summary = append(summary, "change-vs-eevdf")
	}
	timeline := []string{
		"fps", "frametime", "cpu-load", "cpu-power", "gpu-load", "cpu-temp", "gpu-temp", "gpu-core-clock",
		"gpu-mem-clock", "gpu-vram-used", "gpu-power", "ram-used", "swap-used", "process-rss", "cpu-mhz",
	}
	files := []string{"quick-summary.png", "summary/runs.png"}
	for i, s := range summary {
		files = append(files, fmt.Sprintf("summary/%02d-%s.png", i+1, s))
	}
	for i, s := range timeline {
		files = append(files, fmt.Sprintf("timeline/%02d-%s.png", i+1, s))
	}
	return append(files, "distribution/1-frametime-tail.png", "distribution/2-frametime-body.png")
}

// treeOf is the tree that holds files, with each directory listed before
// the files in it, as WalkDir gives it, and the file at the top among them.
func treeOf(files []string) []string {
	var out []string
	for _, dir := range []string{"distribution/", "quick-summary.png", "summary/", "timeline/"} {
		out = append(out, dir)
		if !strings.HasSuffix(dir, "/") {
			continue
		}
		var in []string
		for _, f := range files {
			if strings.HasPrefix(f, dir) {
				in = append(in, f)
			}
		}
		slices.Sort(in)
		out = append(out, in...)
	}
	return out
}

func TestRunWritesOneDirectory(t *testing.T) {
	cases := []struct {
		logs  int
		names []string
		eevdf bool
	}{
		{1, nil, false},
		{2, []string{"scx_flow_5.2.28", "EEVDF"}, true},
		{4, []string{"scx_a", " scx_b", "eevdf ", "Scx_A"}, false},
	}
	for _, c := range cases {
		t.Run(fmt.Sprint(c.logs, " logs"), func(t *testing.T) {
			logs := writeLogs(t, c.logs)
			t.Chdir(t.TempDir())
			args := logs
			if c.names != nil {
				args = append([]string{"-dataname", strings.Join(c.names, ",")}, logs...)
			}
			code, stdout, stderr := runCmd(args...)
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			dirs := chartDirs(t)
			if len(dirs) != 1 {
				t.Fatalf("%d charts directories, want exactly one: %v", len(dirs), dirs)
			}
			dir := dirs[0]
			files := wantFiles(c.eevdf)
			if got, want := tree(t, dir), treeOf(files); !slices.Equal(got, want) {
				t.Errorf("tree = %q\nwant %q", got, want)
			}
			want := []string{dir}
			for _, f := range files {
				want = append(want, filepath.Join(dir, filepath.FromSlash(f)))
			}
			if got := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n"); !slices.Equal(got, want) {
				t.Errorf("stdout = %q\nwant %q", got, want)
			}

			var names []string
			for _, n := range c.names {
				names = append(names, strings.TrimSpace(n))
			}
			figs := predicted(t, logs, names)
			for _, f := range files {
				data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.Decode(bytes.NewReader(data))
				if err != nil {
					t.Fatalf("%s: %v", f, err)
				}
				fig, ok := figs[f]
				got := [2]int{img.Bounds().Dx(), img.Bounds().Dy()}
				if want := [2]int{render.Scale * fig.Width, render.Scale * fig.Height}; !ok || got != want {
					t.Errorf("%s is %v, want %v, twice the layout size", f, got, want)
				}
			}
			// The names label the runs exactly as typed, after trimming.
			var o struct {
				Legend []struct{ Data []string } `json:"legend"`
			}
			if err := json.Unmarshal(figs["summary/01-average-fps.png"].Option, &o); err != nil {
				t.Fatal(err)
			}
			var legend []string
			for _, l := range o.Legend {
				legend = append(legend, l.Data...)
			}
			if names == nil {
				names = []string{"Game_2030-01-01_00-00-00"}
			}
			if !slices.Equal(legend, names) {
				t.Errorf("legend = %q, want %q", legend, names)
			}
		})
	}
}

// A figure that fails to render fails the whole write, whichever worker
// draws it, and the error names it.
func TestWriteFiguresFails(t *testing.T) {
	rd, err := render.New()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	dir := t.TempDir()
	good := func(path string) figure.Figure {
		return figure.Figure{Path: path, Width: 10, Height: 10, Option: []byte(`{"backgroundColor": "#ffffff"}`)}
	}
	bad := figure.Figure{Path: "b/bad.png", Width: 10, Height: 10, Option: []byte(`{"series": [`)}
	_, err = writeFigures(rd, []figure.Figure{good("a/1.png"), good("a/2.png"), bad, good("c/1.png")}, dir)
	if err == nil || !strings.Contains(err.Error(), "b/bad.png") {
		t.Errorf("writeFigures = %v, want an error naming b/bad.png", err)
	}
}

func TestRunErrors(t *testing.T) {
	logs := writeLogs(t, 3)
	bad := filepath.Join(filepath.Dir(logs[0]), "bad.csv")
	if err := os.WriteFile(bad, []byte("fps,frametime,elapsed\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const advice = "; a log records one run, so give one name per file, in the same order"
	cases := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{"no log", nil, 2, "mangohud-parser: no log given"},
		{"names for no log", []string{"-dataname", "a,b,c"}, 2, "mangohud-parser: no log given"},
		{"two names for one log", []string{"--dataname", "scx_flow,EEVDF", logs[0]}, 2, "mangohud-parser: -dataname gives 2 names but only 1 log file" + advice + "\n"},
		{"three names for one log", []string{"-dataname", "a,b,c", logs[0]}, 2, "mangohud-parser: -dataname gives 3 names but only 1 log file" + advice + "\n"},
		{"three names for two logs", []string{"-dataname", "a,b,c", logs[0], logs[1]}, 2, "mangohud-parser: -dataname gives 3 names but only 2 log files" + advice + "\n"},
		{"two names for three logs", []string{"-dataname", "a,b", logs[0], logs[1], logs[2]}, 2, "mangohud-parser: -dataname gives 2 names for 3 log files" + advice + "\n"},
		{"one name for two logs", []string{"-dataname=a", logs[0], logs[1]}, 2, "mangohud-parser: -dataname gives 1 name for 2 log files" + advice + "\n"},
		{"empty names", []string{"-dataname", "", logs[0], logs[1]}, 2, "mangohud-parser: -dataname gives 1 name for 2 log files" + advice + "\n"},
		{"blank name", []string{"-dataname", "a, ", logs[0], logs[1]}, 2, "mangohud-parser: -dataname gives an empty name for run 2; give every run a name\n"},
		{"duplicate names", []string{"-dataname", "a, a", logs[0], logs[1]}, 2, `mangohud-parser: -dataname gives "a" for both run 1 and run 2; give each run its own name` + "\n"},
		{"names after the logs", []string{logs[0], "-dataname", "a"}, 2, "mangohud-parser: -dataname must come before the log files"},
		{"long names after the logs", []string{logs[0], "--dataname=a"}, 2, "mangohud-parser: -dataname must come before the log files"},
		{"unknown flag", []string{"-x", logs[0]}, 2, "flag provided but not defined"},
		{"bad log", []string{logs[0], bad}, 1, "mangohud-parser: " + bad + ":1: not a MangoHud log"},
		{"bad log with names", []string{"-dataname", "a,b", logs[0], bad}, 1, "mangohud-parser: " + bad + ":1: not a MangoHud log"},
		{"missing log", []string{filepath.Join(t.TempDir(), "nope.csv")}, 1, "mangohud-parser: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			code, stdout, stderr := runCmd(c.args...)
			if code != c.code || !strings.HasPrefix(stderr, c.stderr) && !strings.Contains(stderr, c.stderr) {
				t.Errorf("exit %d, stderr %q; want %d and %q", code, stderr, c.code, c.stderr)
			}
			// A mismatch in the names is reported alone, before any log is
			// read and without the usage text.
			if strings.HasSuffix(c.stderr, "\n") && stderr != c.stderr {
				t.Errorf("stderr = %q, want exactly %q", stderr, c.stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if dirs := chartDirs(t); len(dirs) != 0 {
				t.Errorf("a failed run left %v", dirs)
			}
		})
	}
}

func TestRunVersionAndHelp(t *testing.T) {
	t.Chdir(t.TempDir())
	if code, stdout, _ := runCmd("-version"); code != 0 || stdout != "mangohud-parser dev\n" {
		t.Errorf("-version: exit %d, stdout %q", code, stdout)
	}
	if code, stdout, _ := runCmd("--version"); code != 0 || stdout != "mangohud-parser dev\n" {
		t.Errorf("--version: exit %d, stdout %q", code, stdout)
	}
	code, _, stderr := runCmd("-h")
	if code != 0 || !strings.Contains(stderr, "usage: mangohud-parser [-dataname NAMES] [-version] LOG [LOG...]") ||
		!strings.Contains(stderr, "trimmed") || !strings.Contains(stderr, "exactly EEVDF") {
		t.Errorf("-h: exit %d, stderr %q", code, stderr)
	}
	if dirs := chartDirs(t); len(dirs) != 0 {
		t.Errorf("-version and -h left %v", dirs)
	}
}
