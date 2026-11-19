// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package render

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

// The plot area is pinned by the grid below so that the tests can compute
// where things land. With an 800x450 canvas it spans x 60..780 and y 60..410,
// the y axis is fixed to 0..100, and each of the 3 categories is 240px wide.
const (
	gridLeft, gridTop, gridRight, gridBottom = 60, 60, 20, 40
	testW, testH                             = 800, 450
)

// testOption has a title, two bar series and a black line that passes 70 on
// the first category, above the bars, which reach 40 and 50.
const testOption = `{
  "backgroundColor": "#ffffff",
  "animation": false,
  "textStyle": {"fontFamily": "Liberation Serif"},
  "title": {"text": "Scheduler comparison", "left": "center", "top": 10,
            "textStyle": {"color": "#000000", "fontSize": 24}},
  "grid": {"left": 60, "top": 60, "right": 20, "bottom": 40},
  "xAxis": {"type": "category", "data": ["frame time", "input lag", "compile"]},
  "yAxis": {"type": "value", "min": 0, "max": 100},
  "series": [
    {"type": "bar", "name": "A", "data": [40, 60, 30], "itemStyle": {"color": "#9ecae1"}},
    {"type": "bar", "name": "B", "data": [50, 45, 35], "itemStyle": {"color": "#fdae6b"}},
    {"type": "line", "name": "trend", "symbol": "none",
     "lineStyle": {"color": "#111111", "width": 2},
     "data": [70, 70, 70]}
  ]
}`

var shared *Renderer

func TestMain(m *testing.M) {
	r, err := New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "New:", err)
		os.Exit(1)
	}
	shared = r
	code := m.Run()
	if err := r.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "Close:", err)
		code = 1
	}
	os.Exit(code)
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return img
}

// dark reports whether the pixel at layout position x, y is close to black,
// which no fill or gridline in testOption is. The image has Scale output
// pixels per layout px, and the first of them is the one tested.
func dark(img image.Image, x, y int) bool {
	r, g, b, _ := img.At(x*Scale, y*Scale).RGBA()
	return (r+g+b)/3 < 0x5000
}

// valueY maps a y axis value in testOption to a pixel row.
func valueY(v float64) int {
	return testH - gridBottom - int(v/100*float64(testH-gridTop-gridBottom))
}

func TestRenderChart(t *testing.T) {
	data, err := shared.Render([]byte(testOption), testW, testH)
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, data)
	if b := img.Bounds(); b.Dx() != Scale*testW || b.Dy() != Scale*testH {
		t.Fatalf("size = %dx%d, want %dx%d", b.Dx(), b.Dy(), Scale*testW, Scale*testH)
	}

	titleInk := 0
	for y := 0; y < gridTop; y++ {
		for x := 0; x < testW; x++ {
			if dark(img, x, y) {
				titleInk++
			}
		}
	}
	if titleInk < 100 {
		t.Errorf("title band has %d dark pixels, want text", titleInk)
	}

	// The line crosses the center of the first band at 70, and the column
	// is clear between it and the taller bar, which reaches 50.
	center := gridLeft + (testW-gridLeft-gridRight)/6
	barTop, lineY := valueY(50), valueY(70)
	hit := false
	for y := lineY - 1; y <= lineY+1; y++ {
		hit = hit || dark(img, center, y)
	}
	if !hit {
		t.Errorf("no line at row %d of column %d", lineY, center)
	}
	for y := lineY + 4; y < barTop-2; y++ {
		if dark(img, center, y) {
			t.Errorf("ink at row %d between the line and the bar", y)
			break
		}
	}
	if dark(img, center, lineY-6) {
		t.Errorf("the line reaches above 70 at row %d", lineY-6)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	first, err := shared.Render([]byte(testOption), testW, testH)
	if err != nil {
		t.Fatal(err)
	}
	second, err := shared.Render([]byte(testOption), testW, testH)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("two renders of the same option differ")
	}
}

func TestRenderSeveralSizes(t *testing.T) {
	// The last size is the widest that Render accepts, whose pixmap is at
	// the wasm limit on that side.
	for _, size := range [][2]int{{400, 300}, {1024, 600}, {400, 300}, {MaxSide, 2}} {
		data, err := shared.Render([]byte(testOption), size[0], size[1])
		if err != nil {
			t.Fatalf("%dx%d: %v", size[0], size[1], err)
		}
		if b := decode(t, data).Bounds(); b.Dx() != Scale*size[0] || b.Dy() != Scale*size[1] {
			t.Errorf("size = %dx%d, want %dx%d", b.Dx(), b.Dy(), Scale*size[0], Scale*size[1])
		}
	}
	if MaxSide*Scale != maxPixels {
		t.Errorf("MaxSide %d at scale %d does not reach the pixmap limit %d", MaxSide, Scale, maxPixels)
	}
}

func TestRenderErrors(t *testing.T) {
	tests := []struct {
		name          string
		option        string
		width, height int
		want          string
	}{
		{"invalid JSON", `{"series": [`, 100, 100, "not valid JSON"},
		{"not an object", `[1, 2]`, 100, 100, "must be a JSON object"},
		{"zero width", testOption, 0, 100, "outside"},
		{"zero height", testOption, 100, 0, "outside"},
		{"width too large", testOption, 20000, 100, "outside"},
		{"height too large", testOption, 100, MaxSide + 1, "outside"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := shared.Render([]byte(tt.option), tt.width, tt.height)
			if err == nil {
				t.Fatalf("got %d bytes, want an error", len(data))
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

// A failed render must not leave the Renderer unusable.
func TestRenderRecoversAfterError(t *testing.T) {
	if _, err := shared.Render([]byte(`[1, 2]`), 100, 100); err == nil {
		t.Fatal("want an error")
	}
	if _, err := shared.Render([]byte(testOption), testW, testH); err != nil {
		t.Fatalf("render after error: %v", err)
	}
}

func TestTextWidth(t *testing.T) {
	text := "FRAME TIME p99 (ms)"
	regular := shared.TextWidth(text, 12, false)
	if want := shared.measure.width(text, "12px sans-serif"); regular != want {
		t.Errorf("TextWidth = %v, want the measurer's %v", regular, want)
	}
	if bold := shared.TextWidth(text, 12, true); bold <= regular {
		t.Errorf("bold width %v is not wider than regular %v", bold, regular)
	}
	if double := shared.TextWidth(text, 24, false); math.Abs(double-2*regular) > 0.01*double {
		t.Errorf("24px width %v is not twice the 12px width %v", double, regular)
	}
	if w := shared.TextWidth(text, 12.5, false); w <= regular {
		t.Errorf("12.5px width %v is not wider than 12px width %v", w, regular)
	}
}
