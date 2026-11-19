// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

// Command mangohud-parser turns MangoHud CSV logs into PNG charts.
//
// Each log is one run, usually one per CPU scheduler, and every run is drawn
// side by side in the same images. The charts go into a new
// charts-<timestamp> directory under the current one, with a quick summary
// image and a summary, a timeline and a distribution directory.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/galpt/mangohud-log-parser/internal/compare"
	"github.com/galpt/mangohud-log-parser/internal/figure"
	"github.com/galpt/mangohud-log-parser/internal/mangohud"
	"github.com/galpt/mangohud-log-parser/internal/render"
)

// version is set by the release build with -ldflags "-X main.version=...".
var version = "dev"

const name = "mangohud-parser"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole command. It returns the exit status: 0 on success, 1 when
// a log or an image fails and 2 for bad usage.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	dataname := fs.String("dataname", "", "comma-separated run names, one per LOG in the same order; the spaces around each name are trimmed and the rest is kept as typed")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: "+name+" [-dataname NAMES] [-version] LOG [LOG...]")
		fmt.Fprintln(stderr, "Writes the charts into a new charts-<timestamp> directory under the current one.")
		fmt.Fprintln(stderr, "Without -dataname each run is named after its file. Name a run exactly EEVDF to get the change chart.")
		fs.PrintDefaults()
	}
	usage := func(msg string) int {
		fmt.Fprintln(stderr, name+": "+msg)
		fs.Usage()
		return 2
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, name, version)
		return 0
	}
	// The flag package stops at the first file, so a -dataname after the
	// files would otherwise be read as the name of a log.
	for _, a := range fs.Args() {
		if a == "-dataname" || a == "--dataname" || strings.HasPrefix(a, "-dataname=") || strings.HasPrefix(a, "--dataname=") {
			return usage("-dataname must come before the log files")
		}
	}
	if fs.NArg() == 0 {
		return usage("no log given")
	}
	var names []string
	if isSet(fs, "dataname") {
		for _, n := range strings.Split(*dataname, ",") {
			names = append(names, strings.TrimSpace(n))
		}
		// The names are checked before any log is read, so a mistake in
		// them is reported at once, however large the logs are.
		if err := compare.ValidateNames(names, fs.NArg()); err != nil {
			fmt.Fprintln(stderr, name+":", err)
			return 2
		}
	}
	if err := generate(fs.Args(), names, stdout); err != nil {
		fmt.Fprintln(stderr, name+":", err)
		return 1
	}
	return 0
}

// isSet tells whether the flag was given at all, so that an empty -dataname
// is reported as a name list that does not fit instead of being ignored.
func isSet(fs *flag.FlagSet, flagName string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == flagName })
	return set
}

// generate parses every log and builds every figure before it creates
// anything, so a bad log leaves no directory behind. Once the directory
// exists, any failure removes it again, and a run leaves either a complete
// set of charts or none.
func generate(paths, names []string, stdout io.Writer) error {
	logs := make([]*mangohud.Log, len(paths))
	for i, p := range paths {
		l, err := mangohud.ParseFile(p)
		if err != nil {
			return err
		}
		// The figures name each log by its file, and the directory it was
		// read from says nothing to a reader.
		l.Source = filepath.Base(p)
		logs[i] = l
	}
	set, err := compare.New(logs, names)
	if err != nil {
		return err
	}
	rd, err := render.New()
	if err != nil {
		return err
	}
	defer rd.Close()
	figs, err := figure.Build(set, rd.TextWidth)
	if err != nil {
		return err
	}

	// A nanosecond timestamp keeps two runs from sharing a directory, and
	// Mkdir rather than MkdirAll makes a collision an error instead of a
	// silent mix of two runs.
	dir := fmt.Sprintf("charts-%d", time.Now().UnixNano())
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}
	written, err := writeFigures(rd, figs, dir)
	if err != nil {
		return errors.Join(err, os.RemoveAll(dir))
	}
	fmt.Fprintln(stdout, dir)
	for _, p := range written {
		fmt.Fprintln(stdout, p)
	}
	return nil
}

// renderWorkers bounds the number of Renderers that draw at once. A run has
// dozens of images and each Renderer is single threaded, so three of them
// in parallel cut the wall time by more than half. Each one holds its own
// JavaScript runtime and wasm memory, so a fourth adds more memory than the
// time it saves is worth.
const renderWorkers = 3

// writeFigures renders every figure into dir and returns the paths written,
// in the order of figs. The directories are created first, in order, so that
// the workers only ever write files, and a figure at the top of dir needs no
// directory. rd is used as one of the workers, and the others are created here
// and closed again before it returns.
func writeFigures(rd *render.Renderer, figs []figure.Figure, dir string) ([]string, error) {
	paths := make([]string, len(figs))
	made := map[string]bool{}
	for i, f := range figs {
		paths[i] = filepath.Join(dir, filepath.FromSlash(f.Path))
		if sub := filepath.Dir(paths[i]); sub != dir && !made[sub] {
			if err := os.Mkdir(sub, 0o755); err != nil {
				return nil, err
			}
			made[sub] = true
		}
	}

	workers, err := extraRenderers(min(renderWorkers, runtime.GOMAXPROCS(0), len(figs)) - 1)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, w := range workers {
			_ = w.Close()
		}
	}()
	workers = append(workers, rd)

	jobs := make(chan int)
	errs := make([]error, len(figs))
	var failed atomic.Bool
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Go(func() {
			for i := range jobs {
				if errs[i] = writeFigure(w, figs[i], paths[i]); errs[i] != nil {
					failed.Store(true)
				}
			}
		})
	}
	// After the first failure the run is lost anyway, so no new image is
	// started.
	for i := range figs {
		if failed.Load() {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return paths, nil
}

// extraRenderers creates n Renderers at the same time. The wasm module was
// compiled by the first Renderer, so what is left for each is loading
// ECharts and instantiating the module, which run well in parallel.
func extraRenderers(n int) ([]*render.Renderer, error) {
	rds := make([]*render.Renderer, max(n, 0))
	errs := make([]error, len(rds))
	var wg sync.WaitGroup
	for i := range rds {
		wg.Go(func() { rds[i], errs[i] = render.New() })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		for _, rd := range rds {
			if rd != nil {
				_ = rd.Close()
			}
		}
		return nil, err
	}
	return rds, nil
}

func writeFigure(rd *render.Renderer, f figure.Figure, path string) error {
	png, err := rd.Render(f.Option, f.Width, f.Height)
	if err != nil {
		return fmt.Errorf("%s: %w", f.Path, err)
	}
	return os.WriteFile(path, png, 0o644)
}
