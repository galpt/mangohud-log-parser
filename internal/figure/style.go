// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"math"
	"strings"

	"github.com/galpt/mangohud-log-parser/internal/mangohud"
)

// fontFamily is named in every option even though render maps any family to
// it, so the JSON says what the image will show.
const fontFamily = "Liberation Serif"

const (
	white   = "#ffffff"
	ink     = "#000000"
	muted   = "#404040"
	rule    = "#d9d9d9"
	barEdge = "#262626"
)

// Font sizes in layout px. They are shared by the layout and the option so
// that the space reserved for a piece of text is measured at the size it is
// drawn at. Each image is meant to be read at its own size, so the sizes are
// chosen to read comfortably on a screen at 100%.
const (
	titleSize   = 20
	headingSize = 17
	legendSize  = 15
	tickSize    = 14
	valueSize   = 14
	bodySize    = 14
	captionSize = 13
)

// palette is Okabe and Ito's color-blind-safe set without its black, which
// would be lost against the black axes and text. Yellow is last because it is
// the weakest on white, and every bar gets a dark edge for the same reason.
var palette = []string{
	"#0072B2", "#E69F00", "#009E73", "#D55E00", "#CC79A7", "#56B4E9", "#F0E442",
}

// decal is an ECharts decal pattern, which go-echarts has no type for.
type decal struct {
	Symbol     string    `json:"symbol"`
	SymbolSize float64   `json:"symbolSize"`
	DashArrayX []float64 `json:"dashArrayX"`
	DashArrayY []float64 `json:"dashArrayY"`
	Rotation   float64   `json:"rotation"`
	Color      string    `json:"color"`
}

// patterns tells apart the bars of runs that share a palette color. The first
// round of colors is plain and each later round adds the next pattern, so the
// first 28 runs all differ in color, pattern or both.
var patterns = []*decal{
	nil,
	{Symbol: "rect", SymbolSize: 1, DashArrayX: []float64{1, 0}, DashArrayY: []float64{2, 4}, Rotation: -math.Pi / 4, Color: "rgba(0,0,0,0.55)"},
	{Symbol: "rect", SymbolSize: 1, DashArrayX: []float64{1, 0}, DashArrayY: []float64{2, 4}, Rotation: math.Pi / 4, Color: "rgba(0,0,0,0.55)"},
	{Symbol: "circle", SymbolSize: 0.8, DashArrayX: []float64{4, 2}, DashArrayY: []float64{4, 2}, Color: "rgba(0,0,0,0.6)"},
}

// dashes does for the lines of a run what patterns does for its bars. A line
// is too thin to carry a decal, so each later round of colors gets a dash
// instead, and the first 21 runs differ in color, dash or both.
var dashes = []string{"solid", "dashed", "dotted"}

// runColor, runDecal and runDash are indexed by the position of a run in
// compare.Set.Runs, so a run looks the same in every image.
func runColor(i int) string { return palette[i%len(palette)] }

func runDecal(i int) *decal { return patterns[(i/len(palette))%len(patterns)] }

func runDash(i int) string { return dashes[(i/len(palette))%len(dashes)] }

// metricName is a metric as the charts print it, with its unit in
// parentheses and an arrow for the better direction.
func metricName(label, unit string, d mangohud.Direction) string {
	s := label
	if unit != "" {
		s += " (" + unit + ")"
	}
	switch d {
	case mangohud.HigherIsBetter:
		s += " ↑"
	case mangohud.LowerIsBetter:
		s += " ↓"
	}
	return s
}

// lineHeight is the advance between two lines of text at a font size.
func lineHeight(size float64) float64 { return math.Ceil(size * 1.4) }

// wrap breaks text into lines no wider than width. A word that is wider than
// width on its own is cut between characters, since a line that runs past the
// image edge would be clipped.
func wrap(text string, width, size float64, bold bool, m Measure) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		try := word
		if line != "" {
			try = line + " " + word
		}
		if m(try, size, bold) <= width {
			line = try
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
		line = ""
		for _, r := range word {
			if line != "" && m(line+string(r), size, bold) > width {
				lines = append(lines, line)
				line = ""
			}
			line += string(r)
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
