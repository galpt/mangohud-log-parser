// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package figure

import (
	"fmt"
	"strings"

	"github.com/galpt/mangohud-log-parser/internal/compare"
)

// percentileRule says how MangoHud picks a frame for its percentile FPS
// values, which the captions of the three of them share.
const percentileRule = "%s follows MangoHud, which sorts the frame times of the run from the slowest to the fastest and takes the one at " +
	"position trunc(%s × N − 1), counted from 0 with N the number of frames, so position 0 when the product is below 1. " +
	"The value is 1000 divided by that frame time in ms. There is no interpolation."

// refreshNote is added to the statistics of the columns other than fps and
// frametime, which MangoHud does not measure per frame.
const refreshNote = "MangoHud refreshes this column about every 500 ms and repeats it on the frames in between, " +
	"so the mean weights each reading by the frames drawn while it was current."

// definition is the caption sentence that says how a statistic is computed.
// The FPS and frame time statistics follow MangoHud's _summary.csv, and the
// rest are the mean or the maximum of a column over every sample.
func definition(mt compare.Metric) string {
	switch mt.Key {
	case "avg_fps":
		return "Average FPS is frames over time, 1000 / (mean frame time in ms), as in MangoHud's summary. " +
			"It can differ from the mean of the fps column, which counts a long frame as much as a short one."
	case "min_fps_1pct":
		return fmt.Sprintf(percentileRule, mt.Label, "0.01") + " About 1% of the frames are as slow or slower."
	case "min_fps_0_1pct":
		return fmt.Sprintf(percentileRule, mt.Label, "0.001") + " About 0.1% of the frames are as slow or slower."
	case "percentile_fps_97":
		return fmt.Sprintf(percentileRule, mt.Label, "0.97") + " About 3% of the frames are faster."
	case "avg_frametime":
		return "Average frame time is the mean of the frametime column over every frame, the sum divided by N."
	}
	if col, ok := strings.CutPrefix(mt.Key, "avg_"); ok {
		s := mt.Label + " is the mean of the " + col + " column over every sample."
		if strings.HasSuffix(col, "_temp") {
			s += " It is the true mean, where MangoHud's own summary truncates it to whole degrees."
		}
		return s + " " + refreshNote
	}
	col, _ := strings.CutPrefix(mt.Key, "peak_")
	return mt.Label + " is the highest value of the " + col + " column over every sample."
}
