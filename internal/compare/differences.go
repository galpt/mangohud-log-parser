// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package compare

import (
	"strconv"
	"strings"
)

// unknown stands for a field that a log does not carry, such as the version of
// a log written without log_versioning.
const unknown = "unknown"

// differences compares the fields that make two runs hard to compare. The CPU
// and the GPU are read from runs whose system information could be split, so a
// run with an unsplit hardware text is not counted as different from the rest.
func differences(runs []Run) []string {
	fields := []struct {
		name  string
		value func(r Run) (string, bool)
	}{
		{"MangoHud version", func(r Run) (string, bool) { return r.Log.Version, true }},
		{"CPU", func(r Run) (string, bool) { return r.Log.System.CPU, r.Log.System.Hardware == "" }},
		{"GPU", func(r Run) (string, bool) { return r.Log.System.GPU, r.Log.System.Hardware == "" }},
		{"RAM", func(r Run) (string, bool) {
			if r.Log.System.RAMKiB == 0 {
				return "", true
			}
			return strconv.FormatInt(r.Log.System.RAMKiB, 10) + " KiB", true
		}},
		{"Kernel", func(r Run) (string, bool) { return r.Log.System.Kernel, true }},
		{"Governor", func(r Run) (string, bool) { return r.Log.System.Governor, true }},
	}
	var out []string
	for _, f := range fields {
		var values []string
		labels := map[string][]string{}
		for _, r := range runs {
			v, ok := f.value(r)
			if !ok {
				continue
			}
			if v == "" {
				v = unknown
			}
			if _, seen := labels[v]; !seen {
				values = append(values, v)
			}
			labels[v] = append(labels[v], r.Label)
		}
		if len(values) < 2 {
			continue
		}
		parts := make([]string, len(values))
		for i, v := range values {
			parts[i] = v + " (" + strings.Join(labels[v], ", ") + ")"
		}
		out = append(out, f.name+" differs: "+strings.Join(parts, ", "))
	}
	return out
}
