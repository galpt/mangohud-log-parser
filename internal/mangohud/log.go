// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

// Package mangohud reads the CSV logs that MangoHud writes while it records
// the frames of a game.
//
// A log holds one logging session. The writer opens its file in append mode,
// so two sessions that start in the same second share one file, and Parse
// refuses such a file instead of mixing the runs. The parser follows the
// writer in src/logging.cpp of MangoHud: the same two layouts (with and
// without log_versioning), the system information line whose names are never
// escaped, and the column sets of every release since v0.5.
package mangohud

// Direction tells which way a better value points.
type Direction int

const (
	// NoDirection marks a value that describes a run without ranking it, such
	// as a temperature or a clock.
	NoDirection Direction = iota
	// HigherIsBetter marks a value such as FPS.
	HigherIsBetter
	// LowerIsBetter marks a value such as frame time.
	LowerIsBetter
)

// Column describes one frame metric of a log. Label reads well in a chart
// and Unit is empty when the value has none. A column that this package does
// not know keeps its header name as the label and has no unit and no
// direction.
type Column struct {
	Name      string
	Unit      string
	Label     string
	Direction Direction
}

// System is the system information that MangoHud prints before the samples.
// Fields the log does not carry are empty, or zero for RAMKiB.
//
// MangoHud joins the fields without any escaping, so a comma inside a name
// shifts every field after it. The last fields can still be told apart from
// the right, but the leading operating system, CPU and GPU names can not be
// separated again. In that case they are kept together in Hardware and OS,
// CPU and GPU stay empty.
type System struct {
	OS, CPU, GPU string
	Hardware     string
	RAMKiB       int64
	Kernel       string
	Driver       string
	// Governor is the cpufreq governor of the first CPU, which the header calls
	// cpuscheduler. It is not the scheduler of the kernel.
	Governor string
}

// Log is one parsed MangoHud log.
//
// Columns lists the frame metrics in the order of the header, except elapsed,
// and Data holds their values, one slice per column and one value per sample.
// Elapsed holds the time of each sample in nanoseconds since logging began.
// Every slice has the same length, which is at least one.
type Log struct {
	// Source is the name given to Parse. It is usually the path of the file.
	Source string
	// Versioned tells whether the log carries the "v1" layout that holds the
	// MangoHud version. Version is empty when it does not.
	Versioned bool
	Version   string
	System    System
	Columns   []Column
	Data      [][]float64
	Elapsed   []int64
	// Notes holds remarks about the file that are not errors, such as a last
	// line that was cut off.
	Notes []string
}

// Len returns the number of samples.
func (l *Log) Len() int { return len(l.Elapsed) }

// Series returns the values of the named column, or nil when the log has no
// such column. The slice belongs to the log and must not be modified.
func (l *Log) Series(name string) []float64 {
	for i, c := range l.Columns {
		if c.Name == name {
			return l.Data[i]
		}
	}
	return nil
}
