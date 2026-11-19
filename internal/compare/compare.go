// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

// Package compare combines several MangoHud logs into the tables that a chart
// can draw side by side.
//
// One log is one run, and a run has a name that the user gives it or that
// comes from the file. The runs keep the order they were given in. A statistic
// or a column that a run does not have is left out of that run instead of
// being guessed.
package compare

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/galpt/mangohud-log-parser/internal/mangohud"
	"github.com/galpt/mangohud-log-parser/internal/summary"
)

// Run is one log with its label and statistics. The label is unique within the
// Set, and it is the name the run is shown under.
type Run struct {
	Label string
	Log   *mangohud.Log
	Stats *summary.Result
}

// Cell is one statistic of one run. Valid is false when the run has no such
// statistic, and the other fields mean nothing then. Text is the value with
// the precision of its metric.
type Cell struct {
	Value float64
	Text  string
	Valid bool
}

// Metric is one statistic across all runs, and Cells is parallel to the runs
// of the Set.
type Metric struct {
	Key       string
	Label     string
	Unit      string
	Direction mangohud.Direction
	Decimals  int
	Cells     []Cell
}

// Set is the combination of several logs. It is read-only after New, and the
// slices it returns must not be modified.
type Set struct {
	runs     []Run
	metrics  []Metric
	timeline []mangohud.Column
	diffs    []string
}

// New combines the logs in the order given. The names label the runs and must
// pass ValidateNames. A nil slice names each run after the base name of its
// log's source, without the ".csv" extension.
func New(logs []*mangohud.Log, names []string) (*Set, error) {
	if len(logs) == 0 {
		return nil, fmt.Errorf("no logs to compare")
	}
	for i, l := range logs {
		if l == nil {
			return nil, fmt.Errorf("log %d is nil", i)
		}
	}
	if names != nil {
		if err := ValidateNames(names, len(logs)); err != nil {
			return nil, err
		}
	}
	s := &Set{}
	taken := map[string]bool{}
	for i, l := range logs {
		name := defaultName(l.Source)
		if names != nil {
			name = names[i]
		}
		label := freeLabel(taken, name, i+1)
		taken[label] = true
		s.runs = append(s.runs, Run{Label: label, Log: l, Stats: summary.Compute(l)})
	}
	s.buildMetrics()
	s.buildTimeline()
	s.diffs = differences(s.runs)
	return s, nil
}

// Runs returns the runs in the order of the logs given to New.
func (s *Set) Runs() []Run { return s.runs }

// Metrics returns every statistic that at least one run has, in the order of
// summary.Stat.Order.
func (s *Set) Metrics() []Metric { return s.metrics }

// Timeline returns the union of the frame metric columns of the runs, in the
// order they first appear. The values of a column are in each run's Log.
func (s *Set) Timeline() []mangohud.Column { return s.timeline }

// Differences lists the system fields that are not the same in every run, one
// line per field, such as "Kernel differs: 6.1.0 (a, b), 6.2.0 (c)". Runs are
// named by their labels. It is empty when the runs agree, and when there is
// only one run.
func (s *Set) Differences() []string { return s.diffs }

// defaultName is the base name of a log's source without a ".csv" extension.
func defaultName(source string) string {
	base := filepath.Base(source)
	if n := len(base) - len(".csv"); n > 0 && strings.EqualFold(base[n:], ".csv") {
		return base[:n]
	}
	return base
}

// buildMetrics lines the statistics of all runs up by their order.
func (s *Set) buildMetrics() {
	byOrder := map[int]*Metric{}
	for i, r := range s.runs {
		for _, st := range r.Stats.Stats {
			m := byOrder[st.Order]
			if m == nil {
				m = &Metric{Key: st.Key, Label: st.Label, Unit: st.Unit, Direction: st.Direction, Decimals: st.Decimals, Cells: make([]Cell, len(s.runs))}
				byOrder[st.Order] = m
			}
			m.Cells[i] = Cell{Value: st.Value, Text: st.Text(), Valid: true}
		}
	}
	for _, order := range slices.Sorted(maps.Keys(byOrder)) {
		s.metrics = append(s.metrics, *byOrder[order])
	}
}

func (s *Set) buildTimeline() {
	seen := map[string]bool{}
	for _, r := range s.runs {
		for _, c := range r.Log.Columns {
			if !seen[c.Name] {
				seen[c.Name] = true
				s.timeline = append(s.timeline, c)
			}
		}
	}
}

// freeLabel returns name if no earlier run has exactly that label. Otherwise
// it returns the first of "name #k", "name #k-2", "name #k-3" and so on that
// is free, with k the 1-based position of the run. Labels are compared byte
// for byte, so "EEVDF" and "eevdf" are two different labels.
//
// A name can hold spaces, digits and "#", so it can look exactly like the
// numbered label of another run, and a single rule can not promise distinct
// labels. The search does. It keeps the set of every label handed out so far,
// and a label enters the set only after it was found absent, so no two runs
// end up with the same label. The candidates for one run are pairwise
// different strings, because they differ in a suffix of digits, "#", "-" and
// a space. The set is finite, so one of them is free and the search ends.
// When every name is already unique, nothing is changed.
func freeLabel(taken map[string]bool, name string, k int) string {
	if !taken[name] {
		return name
	}
	numbered := name + " #" + strconv.Itoa(k)
	if !taken[numbered] {
		return numbered
	}
	for n := 2; ; n++ {
		if candidate := numbered + "-" + strconv.Itoa(n); !taken[candidate] {
			return candidate
		}
	}
}

// ValidateNames checks the names given for the runs of count logs. There must
// be one name per log, no name may be empty, and no two names may be the same
// string, because two runs need two names. Names are compared byte for byte,
// so "EEVDF" and "eevdf" name two runs. The messages speak of the -dataname
// flag that supplies the names.
func ValidateNames(names []string, count int) error {
	const advice = "a log records one run, so give one name per file, in the same order"
	switch {
	case len(names) > count:
		return fmt.Errorf("-dataname gives %s but only %s; %s", plural(len(names), "name"), plural(count, "log file"), advice)
	case len(names) < count:
		return fmt.Errorf("-dataname gives %s for %s; %s", plural(len(names), "name"), plural(count, "log file"), advice)
	}
	first := map[string]int{}
	for i, n := range names {
		if strings.TrimSpace(n) == "" {
			return fmt.Errorf("-dataname gives an empty name for run %d; give every run a name", i+1)
		}
		if j, ok := first[n]; ok {
			return fmt.Errorf("-dataname gives %q for both run %d and run %d; give each run its own name", n, j+1, i+1)
		}
		first[n] = i
	}
	return nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
