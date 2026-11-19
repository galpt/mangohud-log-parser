// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package mangohud

// elapsedName, fpsName and frametimeName are the columns every log must have.
const (
	elapsedName   = "elapsed"
	fpsName       = "fps"
	frametimeName = "frametime"
)

// known maps the columns that MangoHud has ever written to their description.
// Units follow the overlay: loads are percents, memory is GiB, and a clock is
// in MHz. The temperature columns hold whole degrees Celsius.
var known = map[string]Column{
	"fps":            {Name: "fps", Label: "FPS", Direction: HigherIsBetter},
	"frametime":      {Name: "frametime", Unit: "ms", Label: "Frame time", Direction: LowerIsBetter},
	"cpu_load":       {Name: "cpu_load", Unit: "%", Label: "CPU load"},
	"cpu_power":      {Name: "cpu_power", Unit: "W", Label: "CPU power"},
	"gpu_load":       {Name: "gpu_load", Unit: "%", Label: "GPU load"},
	"cpu_temp":       {Name: "cpu_temp", Unit: "°C", Label: "CPU temperature"},
	"gpu_temp":       {Name: "gpu_temp", Unit: "°C", Label: "GPU temperature"},
	"gpu_core_clock": {Name: "gpu_core_clock", Unit: "MHz", Label: "GPU core clock"},
	"gpu_mem_clock":  {Name: "gpu_mem_clock", Unit: "MHz", Label: "GPU memory clock"},
	"gpu_vram_used":  {Name: "gpu_vram_used", Unit: "GiB", Label: "GPU VRAM used"},
	"gpu_power":      {Name: "gpu_power", Unit: "W", Label: "GPU power"},
	"ram_used":       {Name: "ram_used", Unit: "GiB", Label: "RAM used"},
	"swap_used":      {Name: "swap_used", Unit: "GiB", Label: "Swap used"},
	"process_rss":    {Name: "process_rss", Unit: "GiB", Label: "Process RSS"},
	"cpu_mhz":        {Name: "cpu_mhz", Unit: "MHz", Label: "CPU clock"},
}

// describe returns the description of a column, or a bare one for a name that
// no release of MangoHud is known to write.
func describe(name string) Column {
	if c, ok := known[name]; ok {
		return c
	}
	return Column{Name: name, Label: name}
}
