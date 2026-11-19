// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

// Package figure turns a set of compared MangoHud logs into ECharts options
// laid out like the figures of a paper, each with the pixel size it needs.
//
// The summary directory holds a table of the runs and one bar chart per
// statistic, with one bar per run, and the changes against a run named EEVDF
// when there is one. The timeline directory has one image per frame metric
// with a line per run over the time of the run, and the distribution directory
// shows how the frame times of each run are spread, with the slow tail drawn
// so it can be read. Every image stands on its own with a title, a legend and
// a caption, so it can be read at its own size and shown without the others.
// The options are built with go-echarts types, and the few things go-echarts
// cannot express are added in one place (see missing). Every size is worked
// out here from the content: text is measured with the same font the renderer
// draws with, so an image can reserve the space its labels need instead of
// leaving ECharts to guess and overlap them.
package figure

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/galpt/mangohud-log-parser/internal/compare"
)

// Measure returns the width in px of text drawn at sizePx in the regular or
// the bold face of the font that the renderer uses.
type Measure func(text string, sizePx float64, bold bool) float64

// Figure is one image to render, Width by Height in layout px. Option is the
// ECharts option as JSON. Path is where the image is meant to be saved,
// relative to the output directory, with forward slashes and exactly one
// directory in front of the file name.
type Figure struct {
	Path          string
	Width, Height int
	Option        []byte
}

// The directories of the output. Their names are fixed, so nothing that comes
// from a log or a run name ends up in a directory name.
const (
	summaryDir      = "summary"
	timelineDir     = "timeline"
	distributionDir = "distribution"
)

// baselineLabel is the run label that the change chart is measured against.
// It is compared byte for byte, so a run has to be named exactly EEVDF.
const baselineLabel = "EEVDF"

// Build makes every image of the set: the summary images first, then the
// timelines, then the distributions. Each directory numbers its images in the
// order they are returned.
func Build(set *compare.Set, m Measure) ([]Figure, error) {
	sources := sourceLines(set)
	var figs []Figure
	add := func(f Figure, err error, what string) error {
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		figs = append(figs, f)
		return nil
	}
	f, err := runsFigure(set, m)
	if err := add(f, err, "run table"); err != nil {
		return nil, err
	}

	metrics := set.Metrics()
	base, deltas := deltaMetrics(set)
	count := len(metrics)
	if len(deltas) > 0 {
		count++
	}
	for i, mt := range metrics {
		f, err := metricFigure(set, mt, sources, m)
		f.Path = numbered(summaryDir, i, count, slug(mt.Label))
		if err := add(f, err, mt.Label); err != nil {
			return nil, err
		}
	}
	if len(deltas) > 0 {
		f, err := deltaFigure(set, base, deltas, sources, m)
		f.Path = numbered(summaryDir, len(metrics), count, deltaFile)
		if err := add(f, err, deltaName); err != nil {
			return nil, err
		}
	}

	cols := set.Timeline()
	for i, c := range cols {
		f, err := timelineFigure(set, c, sources, m)
		f.Path = numbered(timelineDir, i, len(cols), slug(c.Name))
		if err := add(f, err, "timeline of "+c.Name); err != nil {
			return nil, err
		}
	}

	dists := newDistributions(set)
	builders := []struct {
		name string
		make func([]distribution, []string, Measure) (Figure, error)
	}{
		{"frametime-tail", tailFigure},
		{"frametime-body", bodyFigure},
	}
	for i, b := range builders {
		f, err := b.make(dists, sources, m)
		f.Path = numbered(distributionDir, i, len(builders), b.name)
		if err := add(f, err, b.name); err != nil {
			return nil, err
		}
	}
	return figs, nil
}

// numbered is the path of the i-th of count images in dir. The number is zero
// padded to the width of count, so the files sort in the order they were
// made, and it alone already makes every name in the directory unique.
func numbered(dir string, i, count int, name string) string {
	return fmt.Sprintf("%s/%0*d-%s.png", dir, len(strconv.Itoa(count)), i+1, name)
}

// slug turns a label into the readable part of a file name. Letters and
// digits are kept and lowercased, '%' becomes "pct", and every other run of
// characters, non-ASCII ones included, becomes a single '-', with none at
// either end. "0.1% min FPS" becomes "0-1pct-min-fps" and "cpu_load" becomes
// "cpu-load". A label with nothing left gets "metric", which the number in
// front of it still keeps apart from any other.
func slug(label string) string {
	var b strings.Builder
	dash := false
	for _, r := range label {
		var part string
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			part = string(r)
		case r >= 'A' && r <= 'Z':
			part = string(r - 'A' + 'a')
		case r == '%':
			part = "pct"
		default:
			dash = true
			continue
		}
		if dash && b.Len() > 0 {
			b.WriteByte('-')
		}
		dash = false
		b.WriteString(part)
	}
	if b.Len() == 0 {
		return "metric"
	}
	return b.String()
}

// sourceLines are the caption lines that name each run and the file it was
// read from, followed by one note for every system field that is not the
// same in all runs, since such runs are not strictly comparable.
func sourceLines(set *compare.Set) []string {
	runs := set.Runs()
	parts := make([]string, len(runs))
	for i, r := range runs {
		parts[i] = r.Label + " (" + r.Log.Source + ")"
	}
	word := "Run: "
	if len(runs) > 1 {
		word = "Runs: "
	}
	lines := []string{word + strings.Join(parts, ", ") + "."}
	for _, d := range set.Differences() {
		lines = append(lines, "Note: "+d+".")
	}
	return lines
}

// px formats a coordinate for the string-typed position fields of go-echarts.
func px(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
