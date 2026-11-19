// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package mangohud

import (
	"strings"
	"testing"
)

// FuzzParse checks that no input makes Parse panic or hang, and that a log it
// accepts has the shape the rest of the program relies on.
func FuzzParse(f *testing.F) {
	for _, columns := range []string{columnsV05, columnsV07, columnsV080, columnsV082} {
		f.Add(versioned("0.8.4-test", sysValues, columns, samples(columns, 3)...))
		f.Add(unversioned(sysValues, columns, samples(columns, 3)...))
	}
	full := versioned("0.8.4-test", sysValues, columnsV082, samples(columnsV082, 3)...)
	f.Add(strings.ReplaceAll(full, "\n", "\r\n"))
	f.Add(strings.TrimSuffix(full, "\n"))
	f.Add(full[:len(full)-9])
	f.Add(full + full)
	f.Add(unversioned("O,C,G,H,100,k,,gov", columnsV05, samples(columnsV05, 2)...))
	f.Add(unversioned("O,C,G,100,k,d", columnsV05, samples(columnsV05, 2)...))
	f.Add(unversioned(sysValues, "elapsed,frametime,fps", "5,10,inf", "6,20,-nan"))
	f.Add("")
	f.Add("v1\n")
	f.Fuzz(func(t *testing.T, text string) {
		l, err := Parse("fuzz.csv", strings.NewReader(text))
		if err != nil {
			if l != nil {
				t.Fatal("log returned together with an error")
			}
			return
		}
		if l.Len() == 0 {
			t.Fatal("accepted a log without samples")
		}
		if len(l.Data) != len(l.Columns) {
			t.Fatalf("%d data slices for %d columns", len(l.Data), len(l.Columns))
		}
		for i, d := range l.Data {
			if len(d) != l.Len() {
				t.Fatalf("column %s has %d values for %d samples", l.Columns[i].Name, len(d), l.Len())
			}
		}
		if l.Series(fpsName) == nil || l.Series(frametimeName) == nil || l.Series(elapsedName) != nil {
			t.Fatal("required columns are not as expected")
		}
		for i := 1; i < l.Len(); i++ {
			if l.Elapsed[i] < l.Elapsed[i-1] || l.Elapsed[i-1] < 0 {
				t.Fatalf("elapsed goes back at sample %d", i)
			}
		}
		if l.Versioned != (l.Version != "") || l.System.RAMKiB < 0 {
			t.Fatalf("bad header fields %+v", l)
		}
		if (l.System.Hardware != "") && (l.System.OS != "" || l.System.CPU != "" || l.System.GPU != "") {
			t.Fatalf("split and unsplit hardware together: %+v", l.System)
		}
	})
}
