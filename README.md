# mangohud-log-parser

`mangohud-parser` turns the CSV logs of [MangoHud](https://github.com/flightlessmango/MangoHud) into PNG charts. MangoHud is a Vulkan and OpenGL overlay for Linux that can log every frame of a game to a CSV file, with the frame time and the CPU, GPU and memory readings of that moment. A typical use is to play the same scene once per CPU scheduler and compare the runs.

The parser reads several logs, one run each, and draws them side by side in the style of the figures of a paper: a bar chart per statistic, a timeline per column and the distribution of the frame times. It is strict. A line it does not understand is an error, since a skipped row would put a wrong number in a chart.

## Install

### Release binaries

Prebuilt archives are attached to the [GitHub releases](https://github.com/galpt/mangohud-log-parser/releases). Each archive holds the binary, the README and the license files, and `SHA256SUMS` is published next to them.

| System | Architectures | Archive |
|---|---|---|
| Linux | amd64, arm64, armv7, armv6, 386 | `.tar.gz` |
| Windows | amd64, arm64 | `.zip` |
| macOS | amd64, arm64 | `.tar.gz` |
| FreeBSD | amd64, arm64 | `.tar.gz` |

### Go

This needs Go 1.26.0, the version in `go.mod`.

```sh
go install github.com/galpt/mangohud-log-parser/cmd/mangohud-parser@latest
```

### From source

```sh
go build ./cmd/mangohud-parser
```

`mangohud-parser -version` prints the version of a release build, and `dev` for a build made this way.

## Usage

```sh
mangohud-parser [-dataname NAMES] [-version] LOG [LOG...]
```

Every run creates one new directory, `charts-<nanosecond timestamp>`, in the current directory, however many logs are given. The program prints the directory and then every file it wrote.

### Naming the runs

Without `-dataname`, each run is named after its file without `.csv`, and two files with the same name from different directories get ` #k` after it, with k the position of the file. `-dataname` gives one comma-separated name per log, in the order of the logs.

```sh
mangohud-parser -dataname scx_example,EEVDF Game_2026-01-01_12-00-00.csv Game_2026-01-01_12-10-00.csv
```

The flag has to come before the log files, and `--dataname` works as well. Names are case sensitive, so `EEVDF` and `eevdf` are two different names. To get the change chart, name the EEVDF run exactly `EEVDF`. That spelling, and only that one, makes it the baseline. These rules apply in this order:

| Rule | What happens |
|---|---|
| The flag comes after a log file | An error, with exit status 2 |
| The spaces around a name | Trimmed, so `scx_flow, EEVDF` works |
| The name count differs from the log count | An error before any log is read, with exit status 2 |
| A name is empty, or two names are the same string | An error before any log is read, with exit status 2 |
| Anything else in a name | Kept exactly as typed |

### Output

```
charts-1795000000000000000/
  quick-summary.png
  summary/
    runs.png
    01-average-fps.png
    ...
    31-peak-gpu-memory-clock.png
    32-change-vs-eevdf.png
  timeline/
    01-fps.png
    ...
    15-cpu-mhz.png
  distribution/
    1-frametime-tail.png
    2-frametime-body.png
```

| Path | Files | Shows |
|---|---|---|
| `quick-summary.png` | One file, at the top | The main statistics of every run in one table |
| `summary/` | `runs.png`, `NN-<statistic>.png`, `NN-change-vs-eevdf.png` | The runs, one statistic per image, the change against EEVDF |
| `timeline/` | `NN-<column>.png` | One column of the logs over time |
| `distribution/` | `1-frametime-tail.png`, `2-frametime-body.png` | How the frame times are spread |

The number in front of each file name keeps the order of the images when the files are sorted, and it is zero padded to the width of the image count of its directory. The rest of the name is the statistic or the column in lowercase: `%` becomes `pct`, and any other run of characters that are not ASCII letters or digits becomes a single `-`, so `0.1% min FPS` becomes `0-1pct-min-fps` and `cpu_load` becomes `cpu-load`.

### Errors

All logs are parsed and every image is laid out before anything is created, so a failure leaves no directory behind.

| Situation | What happens |
|---|---|
| No log is given, or a flag is unknown | The usage text is printed, with exit status 2 |
| A log line is not understood | The error names the file and line, with exit status 1 |
| A log lacks the `elapsed`, `fps` or `frametime` column | An error, with exit status 1 |
| A log holds more than one logging session | An error, with exit status 1 |
| The last line of a log was cut off, as when the game was killed | The line is skipped and noted under the run table |
| An image cannot be drawn | The directory is removed again, with exit status 1 |

### Combining logs

Each log is one run and one bar, line or curve in every image, in the order the logs were given. A run has the same color in every image. Up to seven runs differ in color alone, and later runs repeat the colors with a pattern on their bars and a dash on their lines, so the first 21 runs all look different.

A statistic or a column that a log does not have, as with logs of older MangoHud versions, is shown as `n/a` or left out for that run, and the caption says so. If the runs differ in the MangoHud version, the CPU, the GPU, the RAM, the kernel or the governor, the run table and every caption say so, since such runs are not strictly comparable.

## What the images show

Each image stands on its own, with a title, a legend of the runs, one plot and a caption that says how the values were computed and which file each run came from. The PNG files have twice the pixels of the layout on each side, so text stays sharp when zoomed in and on high density screens.

### Quick summary

`quick-summary.png` is one table with a column per run and a row for each of Average FPS, 1% min FPS, 0.1% min FPS, 97% percentile FPS, and the average and the peak of the CPU clock, the GPU core clock, the CPU power and the GPU power. The numbers are those of the images in `summary/`, at the same precision. In each FPS row the best run is bold, and runs that show the same value are all bold.

A run whose log lacks a column shows `n/a`. A row that no run has is left out, and the caption names it.

### Run table

`summary/runs.png` has one row per run, with its name, file, MangoHud version, samples, duration, operating system, CPU, GPU, RAM, kernel and governor. The governor is the cpufreq governor that MangoHud logs as `cpuscheduler`, not the CPU scheduler of the kernel. Notes below the table list a cut off last line, system names that hold a comma and could not be split, and the fields that differ between runs.

### Statistic images

`summary/NN-<statistic>.png` has one bar per run, with the value above it. The title gives the unit and, for the FPS and frame time statistics, an arrow for the better direction. The loads, temperatures, memory, power and clocks have no arrow, since they describe a run without ranking it.

The statistics come in this order: Average FPS, 1% min FPS, 0.1% min FPS, 97% percentile FPS and Average frame time, then the average and the peak of CPU load, GPU load, CPU temperature, GPU temperature, VRAM used, RAM used, swap used, process RSS, CPU power, GPU power, CPU clock, GPU core clock and GPU memory clock, for the columns the logs have.

### Change against EEVDF

`summary/NN-change-vs-eevdf.png` is drawn when a run is named exactly `EEVDF` and there is at least one other run. It gives the percent change of every other run against EEVDF, `100 × (run − EEVDF) / EEVDF`, for the FPS statistics and the average frame time, one group of bars per statistic. A statistic that is zero for EEVDF has no percent change and is left out.

### Timelines

`timeline/NN-<column>.png` has one line per run for each column of the logs, against the seconds since the first sample of that run. ECharts thins the lines for drawing with largest-triangle-three-buckets (LTTB) sampling, which keeps the peaks, and every statistic uses all samples. MangoHud refreshes the CPU, GPU and memory columns about every 500 ms and repeats them on the frames in between, which shows as steps.

### Frame time distribution

Both images draw the full sorted frame times of each run, with nothing interpolated, smoothed or binned.

| Image | X axis | Y axis |
|---|---|---|
| `1-frametime-tail.png` | Share of frames at least this slow, log scale | Frame time, log if the slowest is 20 times the fastest or more |
| `2-frametime-body.png` | Percentile, linear, up to the 99th | Frame time, linear |

In the tail image a point at x % and y ms means that the slowest x % of the frames took y ms or longer. The k-th slowest of N frames is drawn at `x = 100 × k / N`, with the 100 slowest frames all drawn and 100 ranks per decade beyond them. MangoHud's 1% and 0.1% min FPS are 1000 divided by the values near x = 1 and x = 0.1. In the body image the value at percentile p is the frame time at position `ceil(p × N / 100)` of the frame times sorted from the fastest, drawn every 0.1 percentile.

## The statistics

The FPS and frame time statistics follow the formulas of MangoHud's `_summary.csv` (`src/logging.cpp` and `src/fps_metrics.h`), over every sample of the log, with N the number of samples.

| Statistic | Definition |
|---|---|
| Average FPS | `1000 / (sum of frame times / N)`, frames over time, with the frame times in ms |
| 1% min FPS | `1000 / frametime[idx]` for p = 0.01 |
| 0.1% min FPS | `1000 / frametime[idx]` for p = 0.001 |
| 97% percentile FPS | `1000 / frametime[idx]` for p = 0.97 |
| Average frame time | The sum of the frame times divided by N |
| Average of a load, temperature, memory, power or clock | The mean over every sample |
| Peak of the same columns | The maximum over every sample |

Average FPS can differ from tools that average the `fps` column, which counts a long frame as much as a short one. The percentile statistics sort the frame times from the slowest to the fastest and use `idx = trunc(p × N − 1)`, counted from 0. The product is computed in single precision, as MangoHud does, and an index below 0 becomes 0. There is no interpolation.

### Differences from MangoHud

MangoHud keeps the average temperatures in integers, which truncates them to whole degrees. Here they are true means, which is the one place where a value differs from MangoHud on purpose. Sums are kept in double precision, so the last digit can differ from a `_summary.csv`.

## How it renders

go-echarts builds ECharts options. ECharts 5.6.0 renders them to SVG inside goja, a JavaScript engine written in Go, and resvg, running as WebAssembly on wazero, rasterizes the SVG to PNG at twice its size. Text is measured with the embedded Liberation Serif font that draws it, so the figures reserve the space their labels need. Up to three renderers draw in parallel.

The binary is static (`CGO_ENABLED=0`). It needs no browser, network connection or installed fonts, so the output is the same on every host.

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
