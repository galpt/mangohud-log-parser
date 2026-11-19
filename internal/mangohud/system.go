// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package mangohud

import (
	"fmt"
	"strconv"
	"strings"
)

// systemNames are the system information fields in the order MangoHud prints
// them. Releases before v0.6.2 stop after driver, so a header that is a prefix
// of this list is accepted as long as it reaches ram.
var systemNames = []string{"os", "cpu", "gpu", "ram", "kernel", "driver", "cpuscheduler"}

// leadingNames is the number of fields that can be told apart only from the
// left. They are the ones before ram.
const leadingNames = 3

// systemHeaderLen returns how many names the line holds if it is a system
// information header, and 0 if it is not one.
func systemHeaderLen(line string) int {
	for k := len(systemNames); k > leadingNames; k-- {
		if line == strings.Join(systemNames[:k], ",") {
			return k
		}
	}
	return 0
}

// parseSystem reads the values line of a header with k names. When the line has
// more values than names, a comma sits inside the operating system, CPU or GPU
// name. The fields from ram on are then taken from the right, which stays
// reliable because ram is a number, and the text before them is kept whole.
func parseSystem(line string, k int) (System, error) {
	values := strings.Split(line, ",")
	if len(values) < k {
		return System{}, fmt.Errorf("expected %d values for the system information, found %d", k, len(values))
	}
	var s System
	tail := values[leadingNames:k]
	if len(values) == k {
		s.OS, s.CPU, s.GPU = values[0], values[1], values[2]
	} else {
		tail = values[len(values)-(k-leadingNames):]
		s.Hardware = strings.Join(values[:len(values)-len(tail)], ",")
	}
	ram, err := strconv.ParseUint(tail[0], 10, 63)
	if err != nil {
		return System{}, fmt.Errorf("ram is not an integer")
	}
	s.RAMKiB = int64(ram)
	for i, dst := range []*string{&s.Kernel, &s.Driver, &s.Governor}[:len(tail)-1] {
		*dst = tail[i+1]
	}
	return s, nil
}
