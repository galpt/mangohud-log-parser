# mangohud-log-parser

`mangohud-parser` turns the CSV logs of [MangoHud](https://github.com/flightlessmango/MangoHud) into PNG charts.

MangoHud is a Vulkan and OpenGL overlay for Linux that can log every frame of a game to a CSV file, with the frame time and the CPU, GPU and memory readings of the moment. A typical use is to play the same scene once per CPU scheduler and compare the runs.

`mangohud-parser` reads several of these logs, one run each, and draws them side by side in the style of the figures of a paper: a bar chart per statistic, a timeline per column and the distribution of the frame times. The parser is strict. A line it does not understand is an error, because a skipped row would put a wrong number in a chart. The one exception is a last line that was cut off when the game was killed in the middle of a write, which is skipped and noted.

## Install

### Release binaries

Prebuilt archives are attached to the [GitHub releases](https://github.com/galpt/mangohud-log-parser/releases). Each archive holds the binary and the license files, and `SHA256SUMS` is published next to them. The targets are:

- Linux: amd64, arm64, armv7, armv6, 386
- Windows: amd64, arm64
- macOS: amd64, arm64
- FreeBSD: amd64, arm64

### Go

This needs the Go version given in `go.mod` (1.26.0):

```sh
go install github.com/galpt/mangohud-log-parser/cmd/mangohud-parser@latest
```

### From source

To build from a clone:

```sh
go build ./cmd/mangohud-parser
```

`mangohud-parser -version` prints the version of a release build, and `dev` for a build made this way.

## Usage

```sh
mangohud-parser [-dataname NAMES] [-version] LOG [LOG...]
```

### Naming the runs

`-dataname` names the runs, one comma-separated name per log in the same order. The spaces around each name are trimmed, so `scx_flow, EEVDF` works, and everything else is kept exactly as typed: case, dots, underscores and any other characters. `EEVDF` and `eevdf` are two different names. Two names that are the same string are an error, and so is a name count that does not match the number of logs. The names are checked before any log is read, and a mistake there exits with status 2:

```
$ mangohud-parser -dataname scx_flow,EEVDF Game_2026-01-01_12-00-00.csv
mangohud-parser: -dataname gives 2 names but only 1 log file; a log records one run, so give one name per file, in the same order
```

`-dataname` has to come before the log files, as with every flag, and `--dataname` works as well. Without it each run is named after its file without `.csv`. Two files with the same name from different directories get ` #2`, ` #3` and so on after the name, numbered by their position.

To get the change chart, name the EEVDF run exactly `EEVDF`. That spelling, and only that one, makes it the baseline.

### Output

Every run creates one new directory, `charts-<nanosecond timestamp>`, in the current directory, however many logs are given:

```
$ mangohud-parser -dataname scx_example,EEVDF Game_2026-01-01_12-00-00.csv Game_2026-01-01_12-10-00.csv
charts-1795000000000000000
charts-1795000000000000000/summary/runs.png
charts-1795000000000000000/summary/01-average-fps.png
charts-1795000000000000000/summary/02-1pct-min-fps.png
charts-1795000000000000000/summary/03-0-1pct-min-fps.png
charts-1795000000000000000/summary/04-97pct-percentile-fps.png
charts-1795000000000000000/summary/05-average-frame-time.png
charts-1795000000000000000/summary/06-average-cpu-load.png
...
charts-1795000000000000000/summary/31-peak-gpu-memory-clock.png
charts-1795000000000000000/summary/32-change-vs-eevdf.png
charts-1795000000000000000/timeline/01-fps.png
charts-1795000000000000000/timeline/02-frametime.png
charts-1795000000000000000/timeline/03-cpu-load.png
...
charts-1795000000000000000/timeline/15-cpu-mhz.png
charts-1795000000000000000/distribution/1-frametime-tail.png
charts-1795000000000000000/distribution/2-frametime-body.png
```

The program prints the directory and then every file it wrote. The number in front of each file name keeps the order of the images when the files are sorted, and it is zero padded to the width of the image count of its directory. The rest of the name is the statistic or the column in lowercase: `%` becomes `pct`, and any other run of characters that are not ASCII letters or digits becomes a single `-`, so `0.1% min FPS` becomes `0-1pct-min-fps` and `cpu_load` becomes `cpu-load`. The directory names are fixed, so no run name or column name ends up in one.

The charts of a run share a directory so that the whole set is easy to zip and share. If anything fails, no directory is left behind: all logs are parsed and every image is laid out before anything is created, and a failure while drawing removes the directory again.

### Combining logs

Each log is one run and one bar, line or curve in every image, in the order the logs were given. A run has the same color in every image. Up to seven runs differ in color alone. Later runs repeat the colors with a pattern on their bars and a dash on their lines, so the first 21 runs all look different. Bar images grow wider with the number of runs, and every image adds legend rows as the names need them.

A statistic or a column that a log does not have, as with logs of older MangoHud versions, is shown as `n/a` or left out for that run, and the caption says so. If the runs differ in the MangoHud version, the CPU, the GPU, the RAM, the kernel or the governor, the run table and every caption say so, since such runs are not strictly comparable.

## What the images show

Each image stands on its own, with a title, a legend of the runs, one plot and a caption that says how the values were computed and which file each run came from. The PNG files have twice the pixels of the layout on each side, so text stays sharp when zoomed in and on high density screens.

### Run table

`summary/runs.png` has a table of the runs, one row per log, with the file, the MangoHud version, the number of samples, the duration, the operating system, CPU, GPU, RAM, kernel and governor. The governor is the cpufreq governor that MangoHud logs as `cpuscheduler`, not the CPU scheduler of the kernel. Notes below the table list a cut off last line, system information whose names hold a comma and could not be split, and the fields that differ between runs.

### Statistic images

`summary/NN-<statistic>.png` has one bar per run, with the value above it. The title gives the unit and, for the FPS and frame time statistics, an arrow for the better direction. The loads, temperatures, memory, power and clocks have no arrow, because they describe a run without ranking it. The statistics come in this order: Average FPS, 1% min FPS, 0.1% min FPS, 97% percentile FPS and Average frame time, then the average and the peak of CPU load, GPU load, CPU temperature, GPU temperature, VRAM used, RAM used, swap used, process RSS, CPU power, GPU power, CPU clock, GPU core clock and GPU memory clock, for the columns the logs have.

### Change against EEVDF

`summary/NN-change-vs-eevdf.png` is drawn when a run is named exactly `EEVDF` and there is at least one other run. It gives the percent change of every other run against EEVDF, `100 × (run − EEVDF) / EEVDF`, for the FPS statistics and the average frame time, one group of bars per statistic.

### Timelines

`timeline/NN-<column>.png` has one line per run for each column of the logs, in the order of the header, against the seconds since the first sample of that run. ECharts thins the lines for drawing with its largest-triangle-three-buckets (LTTB) sampling, which keeps the peaks. Every statistic uses all samples. MangoHud refreshes the CPU, GPU and memory columns about every 500 ms and repeats them on the frames in between, which shows as steps.

### Frame time distribution

`distribution/` shows the frame times of each run from its full data, sorted, with nothing interpolated, smoothed or binned.

### Slowest frames

`1-frametime-tail.png` puts the share of frames that are at least as slow on a log x axis, so every decade from all frames down to the single slowest frame gets the same width. A point at x % and y ms means that the slowest x % of the frames took y ms or longer. The k-th slowest of N frames is drawn at `x = 100 × k / N`. The 100 slowest frames are all drawn, and beyond them 100 ranks per decade. This is where stutter shows, and MangoHud's 1% and 0.1% min FPS are 1000 divided by the values near x = 1 and x = 0.1. The y axis turns logarithmic when the slowest frame is 20 times the fastest or more.

A linear percentile axis alone would squeeze the slowest 1% of the frames into the last pixels of the plot, and a histogram would hide them in a few nearly empty bins, which is why the tail has an image of its own.

### Body of the frame times

`2-frametime-body.png` shows the rest on a linear percentile axis, from the fastest frame to the 99th percentile, so the spikes of the tail do not flatten it. The value at percentile p is the frame time at position `ceil(p × N / 100)` of the frame times sorted from the fastest, which is the inverse of the empirical distribution function, drawn every 0.1 percentile.

## The statistics

The FPS and frame time statistics follow the formulas of MangoHud's `_summary.csv` (`src/logging.cpp` and `src/fps_metrics.h`), over every sample of the log, with N the number of samples.

### Average FPS

Average FPS is frames over time, `1000 / (sum of frame times / N)` with the frame times in ms. It can differ from tools that average the `fps` column, which counts a long frame as much as a short one.

### Percentile statistics

1% min FPS, 0.1% min FPS and 97% percentile FPS sort the frame times from the slowest to the fastest and take `1000 / frametime[idx]` with `idx = trunc(p × N − 1)` for p = 0.01, 0.001 and 0.97, counted from 0. The product is computed in single precision, as MangoHud does, and an index below 0 becomes 0. There is no interpolation.

### Average frame time

Average frame time is the sum of the frame times divided by N.

### Averages and peaks

The averages of the loads, temperatures, memory, power and clocks are means over every sample, and the peaks are maxima.

### Temperatures and precision

MangoHud keeps the average temperatures in integers, which truncates them to whole degrees. Here they are true means, which is the one place where a value differs from MangoHud on purpose. Sums are kept in double precision, so the last digit can differ from a `_summary.csv`.

## How it renders

go-echarts builds ECharts options. ECharts 5.6.0 renders them to SVG in server-side mode inside goja, a JavaScript engine written in Go. resvg, running as WebAssembly on wazero, rasterizes the SVG to PNG at twice its size with an embedded Liberation Serif font. Text is measured with that same font, so the figures reserve the space their labels need. Three renderers draw in parallel.

The binary is static (`CGO_ENABLED=0`). It does not need a browser, a network connection or installed fonts, and the output is the same on every host. See `internal/render/render.go`.

## Project layout

```
cmd/mangohud-parser  the command
internal/mangohud    parser of one log
internal/summary     statistics of one run
internal/compare     run labels and tables of several runs
internal/figure      ECharts options and layout of every image
internal/render      ECharts in goja and resvg in wazero
```

## License

`mangohud-parser` is licensed under GPL-3.0-only (see `LICENSE`). The resvg-go binding it links is GPL-3.0, which sets the terms for the whole program.

It embeds ECharts, which is Apache-2.0 (`internal/render/assets/LICENSE-echarts` and `NOTICE-echarts`), and Liberation Serif, which is under the SIL Open Font License 1.1 (`internal/render/assets/LICENSE-liberation`).
