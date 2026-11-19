// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package mangohud

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
)

// The column sets of the MangoHud releases that changed them.
var (
	columnsV05  = "fps,frametime,cpu_load,gpu_load,cpu_temp,gpu_temp,gpu_core_clock,gpu_mem_clock,gpu_vram_used,gpu_power,ram_used,elapsed"
	columnsV07  = "fps,frametime,cpu_load,gpu_load,cpu_temp,gpu_temp,gpu_core_clock,gpu_mem_clock,gpu_vram_used,gpu_power,ram_used,swap_used,process_rss,elapsed"
	columnsV080 = "fps,frametime,cpu_load,cpu_power,gpu_load,cpu_temp,gpu_temp,gpu_core_clock,gpu_mem_clock,gpu_vram_used,gpu_power,ram_used,swap_used,process_rss,elapsed"
	columnsV082 = "fps,frametime,cpu_load,cpu_power,gpu_load,cpu_temp,gpu_temp,gpu_core_clock,gpu_mem_clock,gpu_vram_used,gpu_power,ram_used,swap_used,process_rss,cpu_mhz,elapsed"
)

const (
	sysHeader = "os,cpu,gpu,ram,kernel,driver,cpuscheduler"
	sysValues = "Test OS 1,Test CPU 9000,Test GPU 5000,16000000,6.1.0-test,,performance"
)

// samples makes n rows for the given header. The values are made up, and the
// elapsed time advances by 10 ms each row.
func samples(columns string, n int) []string {
	names := strings.Split(columns, ",")
	rows := make([]string, n)
	for i := range rows {
		fields := make([]string, len(names))
		for j, name := range names {
			switch name {
			case "elapsed":
				fields[j] = strconv.Itoa((i + 1) * 10_000_000)
			case "fps":
				fields[j] = strconv.FormatFloat(1000/(10+float64(i)), 'g', 6, 64)
			case "frametime":
				fields[j] = strconv.FormatFloat(10+float64(i), 'g', 6, 64)
			default:
				fields[j] = strconv.Itoa(i + j)
			}
		}
		rows[i] = strings.Join(fields, ",")
	}
	return rows
}

func lines(l ...string) string { return strings.Join(l, "\n") + "\n" }

// unversioned builds a log of the default layout.
func unversioned(sys, columns string, rows ...string) string {
	return lines(append([]string{sysHeader, sys, columns}, rows...)...)
}

// versioned builds a log of the log_versioning layout.
func versioned(version, sys, columns string, rows ...string) string {
	head := []string{"v1", version, systemMarker, sysHeader, sys, frameMarker, columns}
	return lines(append(head, rows...)...)
}

func mustParse(t *testing.T, text string) *Log {
	t.Helper()
	l, err := Parse("t.csv", strings.NewReader(text))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return l
}

func TestParseVersioned(t *testing.T) {
	l := mustParse(t, versioned("0.8.4-test", sysValues, columnsV082, samples(columnsV082, 3)...))
	if !l.Versioned || l.Version != "0.8.4-test" || l.Source != "t.csv" {
		t.Errorf("header fields: %+v", l)
	}
	want := System{OS: "Test OS 1", CPU: "Test CPU 9000", GPU: "Test GPU 5000", RAMKiB: 16000000, Kernel: "6.1.0-test", Governor: "performance"}
	if l.System != want {
		t.Errorf("System = %+v, want %+v", l.System, want)
	}
	if l.Len() != 3 || !reflect.DeepEqual(l.Elapsed, []int64{10_000_000, 20_000_000, 30_000_000}) {
		t.Errorf("Elapsed = %v", l.Elapsed)
	}
	if len(l.Columns) != 15 || len(l.Data) != 15 {
		t.Fatalf("%d columns and %d data slices, want 15 each", len(l.Columns), len(l.Data))
	}
	for i, c := range l.Columns {
		if c.Name == "elapsed" {
			t.Errorf("column %d is elapsed", i)
		}
		if len(l.Data[i]) != 3 {
			t.Errorf("column %s has %d values", c.Name, len(l.Data[i]))
		}
	}
	if got := l.Series("frametime"); !reflect.DeepEqual(got, []float64{10, 11, 12}) {
		t.Errorf("frametime = %v", got)
	}
	if l.Series("nope") != nil {
		t.Error("Series of a missing column is not nil")
	}
	if len(l.Notes) != 0 {
		t.Errorf("Notes = %v", l.Notes)
	}
}

func TestParseUnversioned(t *testing.T) {
	l := mustParse(t, unversioned(sysValues, columnsV082, samples(columnsV082, 2)...))
	if l.Versioned || l.Version != "" {
		t.Errorf("Versioned = %v, Version = %q", l.Versioned, l.Version)
	}
	if l.System.OS != "Test OS 1" || l.Len() != 2 {
		t.Errorf("System = %+v, Len = %d", l.System, l.Len())
	}
}

func TestColumnSets(t *testing.T) {
	cases := []struct {
		name    string
		columns string
		count   int
	}{
		{"v0.5", columnsV05, 11},
		{"v0.7", columnsV07, 13},
		{"v0.8.0", columnsV080, 14},
		{"v0.8.2", columnsV082, 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, text := range []string{
				unversioned(sysValues, tc.columns, samples(tc.columns, 4)...),
				versioned("0.7.1", sysValues, tc.columns, samples(tc.columns, 4)...),
			} {
				l := mustParse(t, text)
				if len(l.Columns) != tc.count {
					t.Errorf("%d columns, want %d", len(l.Columns), tc.count)
				}
				names := strings.Split(tc.columns, ",")
				for i, c := range l.Columns {
					if c.Name != names[i] {
						t.Errorf("column %d is %s, want %s", i, c.Name, names[i])
					}
				}
			}
		})
	}
}

func TestColumnDescriptions(t *testing.T) {
	cols := columnsV082 + ",mystery"
	cols = strings.Replace(cols, ",elapsed,mystery", ",mystery,elapsed", 1)
	l := mustParse(t, unversioned(sysValues, cols, samples(cols, 1)...))
	byName := map[string]Column{}
	for _, c := range l.Columns {
		byName[c.Name] = c
	}
	for name, want := range map[string]Column{
		"fps":         {Name: "fps", Label: "FPS", Direction: HigherIsBetter},
		"frametime":   {Name: "frametime", Unit: "ms", Label: "Frame time", Direction: LowerIsBetter},
		"gpu_temp":    {Name: "gpu_temp", Unit: "°C", Label: "GPU temperature"},
		"cpu_mhz":     {Name: "cpu_mhz", Unit: "MHz", Label: "CPU clock"},
		"process_rss": {Name: "process_rss", Unit: "GiB", Label: "Process RSS"},
		"mystery":     {Name: "mystery", Label: "mystery"},
	} {
		if byName[name] != want {
			t.Errorf("%s = %+v, want %+v", name, byName[name], want)
		}
	}
	if l.Columns[len(l.Columns)-1].Name != "mystery" {
		t.Errorf("the unknown column is not last: %+v", l.Columns)
	}
	for _, c := range l.Columns {
		if _, ok := known[c.Name]; !ok && c.Name != "mystery" {
			t.Errorf("column %s is missing from the registry", c.Name)
		}
	}
}

func TestElapsedMayBeAnywhere(t *testing.T) {
	text := unversioned(sysValues, "elapsed,frametime,fps", "5,10,100", "6,20,50")
	l := mustParse(t, text)
	if !reflect.DeepEqual(l.Elapsed, []int64{5, 6}) || !reflect.DeepEqual(l.Series("frametime"), []float64{10, 20}) {
		t.Errorf("Elapsed = %v, frametime = %v", l.Elapsed, l.Series("frametime"))
	}
	if l.Columns[0].Name != "frametime" || l.Columns[1].Name != "fps" {
		t.Errorf("Columns = %+v", l.Columns)
	}
}

func TestSystemInfo(t *testing.T) {
	cases := []struct {
		name   string
		header string
		values string
		want   System
		notes  int
	}{
		{
			"empty driver", sysHeader, "O,C,G,100,k,,gov",
			System{OS: "O", CPU: "C", GPU: "G", RAMKiB: 100, Kernel: "k", Governor: "gov"}, 0,
		},
		{
			"comma in the gpu name", sysHeader, "O,C,Test GPU, Mobile,100,k,,gov",
			System{Hardware: "O,C,Test GPU, Mobile", RAMKiB: 100, Kernel: "k", Governor: "gov"}, 1,
		},
		{
			"commas in every leading name", sysHeader, "O,x,C,y,G,z,100,k,drv,gov",
			System{Hardware: "O,x,C,y,G,z", RAMKiB: 100, Kernel: "k", Driver: "drv", Governor: "gov"}, 1,
		},
		{
			"no cpuscheduler", "os,cpu,gpu,ram,kernel,driver", "O,C,G,100,k,drv",
			System{OS: "O", CPU: "C", GPU: "G", RAMKiB: 100, Kernel: "k", Driver: "drv"}, 0,
		},
		{
			"no cpuscheduler and a comma", "os,cpu,gpu,ram,kernel,driver", "O,C,G,H,100,k,drv",
			System{Hardware: "O,C,G,H", RAMKiB: 100, Kernel: "k", Driver: "drv"}, 1,
		},
		{
			"ending at ram", "os,cpu,gpu,ram", "O,C,G,100",
			System{OS: "O", CPU: "C", GPU: "G", RAMKiB: 100}, 0,
		},
		{
			"empty names", sysHeader, ",,,1,,,", System{RAMKiB: 1}, 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := lines(tc.header, tc.values, columnsV05, samples(columnsV05, 1)[0])
			l := mustParse(t, text)
			if l.System != tc.want {
				t.Errorf("System = %+v, want %+v", l.System, tc.want)
			}
			if len(l.Notes) != tc.notes {
				t.Errorf("Notes = %v, want %d", l.Notes, tc.notes)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	good := samples(columnsV05, 3)
	short := strings.Join(strings.Split(good[1], ",")[:11], ",")
	long := good[1] + ",7"
	v := lines
	cases := []struct {
		name string
		text string
		want string
	}{
		{"empty file", "", `t.csv:1: empty file`},
		{"not a log", "hello\n", `t.csv:1: not a MangoHud log, expected "v1" or the system information header: "hello"`},
		{"unknown layout", "v2\n0.9\n", `t.csv:1: not a MangoHud log, expected "v1" or the system information header: "v2"`},
		{"header too short", "os,cpu,gpu\nO,C,G\n", `t.csv:1: not a MangoHud log`},
		{"header out of order", "os,gpu,cpu,ram\nO,C,G,1\n", `t.csv:1: not a MangoHud log`},
		{"only the version", "v1\n", `t.csv:2: unexpected end of file, missing the MangoHud version`},
		{"empty version", "v1\n\n", `t.csv:2: the MangoHud version is empty: ""`},
		{"no system marker", "v1\n0.8\n", `t.csv:3: unexpected end of file, missing the system information marker`},
		{"wrong system marker", v("v1", "0.8", "---"), `t.csv:3: expected the system information marker`},
		{"no system header", v("v1", "0.8", systemMarker), `t.csv:4: unexpected end of file, missing the system information header`},
		{"wrong system header", v("v1", "0.8", systemMarker, "os,cpu"), `t.csv:4: expected the system information header "os,cpu,gpu,ram,kernel,driver,cpuscheduler": "os,cpu"`},
		{"no system values", sysHeader + "\n", `t.csv:2: unexpected end of file, missing the system information values`},
		{"too few system values", v(sysHeader, "O,C,G,1,k,d"), `t.csv:2: expected 7 values for the system information, found 6: "O,C,G,1,k,d"`},
		{"ram not an integer", v(sysHeader, "O,C,G,16.5,k,d,g"), `t.csv:2: ram is not an integer: "O,C,G,16.5,k,d,g"`},
		{"ram empty", v(sysHeader, "O,C,G,,k,d,g"), `t.csv:2: ram is not an integer`},
		{"ram negative", v(sysHeader, "O,C,G,-4,k,d,g"), `t.csv:2: ram is not an integer`},
		{"ram misplaced by a comma in the kernel", v(sysHeader, "O,C,G,100,k,x,d,g"), `t.csv:2: ram is not an integer`},
		{"no frame marker", v("v1", "0.8", systemMarker, sysHeader, sysValues), `t.csv:6: unexpected end of file, missing the frame metrics marker`},
		{"wrong frame marker", v("v1", "0.8", systemMarker, sysHeader, sysValues, "x"), `t.csv:6: expected the frame metrics marker`},
		{"no column header", v(sysHeader, sysValues), `t.csv:3: unexpected end of file, missing the column header`},
		{"empty column name", v(sysHeader, sysValues, "fps,,elapsed"), `t.csv:3: column 2 has no name: "fps,,elapsed"`},
		{"repeated column", v(sysHeader, sysValues, "fps,frametime,fps,elapsed"), `t.csv:3: column "fps" appears twice`},
		{"no fps", v(sysHeader, sysValues, "frametime,elapsed", "1,2"), `t.csv:3: the fps column is missing`},
		{"no frametime", v(sysHeader, sysValues, "fps,elapsed", "1,2"), `t.csv:3: the frametime column is missing`},
		{"no elapsed", v(sysHeader, sysValues, "fps,frametime", "1,2"), `t.csv:3: the elapsed column is missing`},
		{"no rows", v(sysHeader, sysValues, columnsV05), `t.csv:4: no data rows`},
		{"too few fields", unversioned(sysValues, columnsV05, good[0], short, good[2]), `t.csv:5: expected 12 fields, found 11: "` + short + `"`},
		{"too many fields", unversioned(sysValues, columnsV05, good[0], long), `t.csv:5: expected 12 fields, found 13: "` + long + `"`},
		{"blank row", unversioned(sysValues, columnsV05, good[0], "", good[2]), `t.csv:5: expected 12 fields, found 1: ""`},
		{"too many fields on an unterminated last line", noBreak(unversioned(sysValues, columnsV05, good[0], long)), `t.csv:5: expected 12 fields, found 13`},
		{"bad number", unversioned(sysValues, columnsV05, strings.Replace(good[0], "100", "abc", 1)), `t.csv:4: "abc" in column fps is not a number`},
		{"hex number", unversioned(sysValues, columnsV05, strings.Replace(good[0], "100", "0x10", 1)), `t.csv:4: "0x10" in column fps is not a number`},
		{"underscore number", unversioned(sysValues, columnsV05, strings.Replace(good[0], "100", "1_0", 1)), `t.csv:4: "1_0" in column fps is not a number`},
		{"hex float", unversioned(sysValues, columnsV05, strings.Replace(good[0], "100", "0x1p4", 1)), `t.csv:4: "0x1p4" in column fps is not a number`},
		{"empty cell", unversioned(sysValues, columnsV05, strings.Replace(good[0], "100", "", 1)), `t.csv:4: "" in column fps is not a number`},
		{"fractional elapsed", unversioned(sysValues, columnsV05, "1,2,3,4,5,6,7,8,9,10,11,12.5"), `t.csv:4: elapsed "12.5" is not a count of nanoseconds`},
		{"exponent elapsed", unversioned(sysValues, columnsV05, "1,2,3,4,5,6,7,8,9,10,11,1e+06"), `t.csv:4: elapsed "1e+06" is not a count of nanoseconds`},
		{"negative elapsed", unversioned(sysValues, columnsV05, "1,2,3,4,5,6,7,8,9,10,11,-5"), `t.csv:4: elapsed "-5" is not a count of nanoseconds`},
		{"elapsed too large", unversioned(sysValues, columnsV05, "1,2,3,4,5,6,7,8,9,10,11,9223372036854775808"), `t.csv:4: elapsed "9223372036854775808"`},
		{"decreasing elapsed", unversioned(sysValues, columnsV05, "1,2,3,4,5,6,7,8,9,10,11,50", "1,2,3,4,5,6,7,8,9,10,11,49"), `t.csv:5: elapsed goes back from 50 to 49`},
		{"second unversioned session", unversioned(sysValues, columnsV05, good...) + unversioned(sysValues, columnsV05, good...), `t.csv:7: this file holds more than one logging session: "` + sysHeader + `"`},
		{"second versioned session", versioned("0.8", sysValues, columnsV05, good...) + versioned("0.8", sysValues, columnsV05, good...), `t.csv:11: this file holds more than one logging session: "v1"`},
		{"first line too long", strings.Repeat("a", maxLine+1), `t.csv:1: line is longer than 1048576 bytes`},
		{"line too long", unversioned(sysValues, columnsV05, strings.Repeat("9", maxLine+1)), `t.csv:4: line is longer than 1048576 bytes`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := Parse("t.csv", strings.NewReader(tc.text))
			if err == nil {
				t.Fatalf("Parse accepted the log: %+v", l)
			}
			if l != nil {
				t.Error("a log was returned together with an error")
			}
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("error = %q, want prefix %q", err, tc.want)
			}
		})
	}
}

func TestCRLF(t *testing.T) {
	for _, text := range []string{
		unversioned(sysValues, columnsV082, samples(columnsV082, 5)...),
		versioned("0.8.4", sysValues, columnsV082, samples(columnsV082, 5)...),
	} {
		want := mustParse(t, text)
		got := mustParse(t, strings.ReplaceAll(text, "\n", "\r\n"))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("CRLF log differs:\n got %+v\nwant %+v", got, want)
		}
	}
}

// noBreak drops the line break that ends the text.
func noBreak(text string) string { return strings.TrimSuffix(text, "\n") }

func TestTruncatedLastLine(t *testing.T) {
	good := samples(columnsV05, 3)
	cut := strings.Join(strings.Split(good[2], ",")[:7], ",")
	cases := []struct {
		name  string
		text  string
		rows  int
		notes []string
	}{
		{"cut short", noBreak(unversioned(sysValues, columnsV05, good[0], good[1], cut)), 2, []string{truncatedNote}},
		{"cut after a comma", noBreak(unversioned(sysValues, columnsV05, good[0], good[1], cut+",")), 2, []string{truncatedNote}},
		{"cut to one field", noBreak(unversioned(sysValues, columnsV05, good[0], good[1], "12")), 2, []string{truncatedNote}},
		{"complete without a line break", noBreak(unversioned(sysValues, columnsV05, good...)), 3, nil},
		{"complete with a line break", unversioned(sysValues, columnsV05, good...), 3, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := mustParse(t, tc.text)
			if l.Len() != tc.rows || !reflect.DeepEqual(l.Notes, tc.notes) {
				t.Errorf("Len = %d, Notes = %v; want %d and %v", l.Len(), l.Notes, tc.rows, tc.notes)
			}
			for i, d := range l.Data {
				if len(d) != tc.rows {
					t.Errorf("column %s has %d values", l.Columns[i].Name, len(d))
				}
			}
		})
	}

	t.Run("short line in the middle", func(t *testing.T) {
		text := unversioned(sysValues, columnsV05, good[0], cut, good[2])
		if _, err := Parse("t.csv", strings.NewReader(text)); err == nil || !strings.HasPrefix(err.Error(), "t.csv:5: expected 12 fields, found 7") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("short last line with a line break", func(t *testing.T) {
		text := unversioned(sysValues, columnsV05, good[0], cut)
		if _, err := Parse("t.csv", strings.NewReader(text)); err == nil || !strings.HasPrefix(err.Error(), "t.csv:5: expected 12 fields, found 7") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("only a cut line", func(t *testing.T) {
		text := noBreak(unversioned(sysValues, columnsV05, cut))
		if _, err := Parse("t.csv", strings.NewReader(text)); err == nil || !strings.HasPrefix(err.Error(), "t.csv:5: no data rows") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("cut line with a CR", func(t *testing.T) {
		text := strings.ReplaceAll(unversioned(sysValues, columnsV05, good[0], cut), "\n", "\r\n")
		l := mustParse(t, strings.TrimSuffix(text, "\n"))
		if l.Len() != 1 || len(l.Notes) != 1 {
			t.Errorf("Len = %d, Notes = %v", l.Len(), l.Notes)
		}
	})
}

func TestNumberForms(t *testing.T) {
	ok := map[string]float64{
		"0": 0, "12": 12, "-3": -3, "+4": 4, "0.5": 0.5, ".5": 0.5, "1e+06": 1e6, "1.5e-05": 1.5e-5,
		"inf": math.Inf(1), "-inf": math.Inf(-1), "INF": math.Inf(1), "infinity": math.Inf(1),
	}
	for s, want := range ok {
		if got, err := parseNumber(s); err != nil || got != want {
			t.Errorf("parseNumber(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
	for _, s := range []string{"nan", "-nan", "NaN", "+nan"} {
		if got, err := parseNumber(s); err != nil || !math.IsNaN(got) {
			t.Errorf("parseNumber(%q) = %v, %v; want NaN", s, got, err)
		}
	}
	for _, s := range []string{"", " ", "1 ", " 1", "abc", "0x10", "0X1P3", "1_000", "1e", "--1", "1,5", "1e999", "nan(0x1)", "0b11"} {
		if got, err := parseNumber(s); err == nil {
			t.Errorf("parseNumber(%q) = %v, want an error", s, got)
		}
	}

	row := "inf,-nan,1e+06,4,5,6,7,8,9,10,11,12"
	l := mustParse(t, unversioned(sysValues, columnsV05, row))
	if !math.IsInf(l.Data[0][0], 1) || !math.IsNaN(l.Data[1][0]) || l.Data[2][0] != 1e6 {
		t.Errorf("row values = %v %v %v", l.Data[0][0], l.Data[1][0], l.Data[2][0])
	}
}

func TestParseFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game_2000-01-01_00-00-00.csv")
	if err := os.WriteFile(path, []byte(unversioned(sysValues, columnsV05, samples(columnsV05, 2)...)), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.Source != path || l.Len() != 2 {
		t.Errorf("Source = %q, Len = %d", l.Source, l.Len())
	}
	if _, err := ParseFile(path + ".missing"); err == nil {
		t.Error("ParseFile accepted a missing file")
	}
	bad := filepath.Join(t.TempDir(), "bad.csv")
	if err := os.WriteFile(bad, []byte("junk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(bad); err == nil || !strings.HasPrefix(err.Error(), bad+":1: ") {
		t.Errorf("error = %v", err)
	}
}

func TestLargeLog(t *testing.T) {
	rows := samples(columnsV082, 20000)
	l := mustParse(t, versioned("0.8.4", sysValues, columnsV082, rows...))
	if l.Len() != 20000 || l.Elapsed[19999] != 200_000_000_000 {
		t.Errorf("Len = %d, last elapsed = %d", l.Len(), l.Elapsed[19999])
	}
}

func TestReadError(t *testing.T) {
	head := unversioned(sysValues, columnsV05, samples(columnsV05, 2)...)
	r := io.MultiReader(strings.NewReader(head), iotest.ErrReader(errors.New("disk fell off")))
	l, err := Parse("t.csv", r)
	if err == nil || l != nil || !strings.HasPrefix(err.Error(), "t.csv:") || !strings.Contains(err.Error(), "disk fell off") {
		t.Errorf("Parse = %v, %v", l, err)
	}
	// A read error that arrives in the middle of a line must not pass for a
	// cut-off last line.
	r = io.MultiReader(strings.NewReader(head+"1,2,3"), iotest.ErrReader(errors.New("disk fell off")))
	if _, err := Parse("t.csv", r); err == nil || !strings.Contains(err.Error(), "disk fell off") {
		t.Errorf("error = %v", err)
	}
}
