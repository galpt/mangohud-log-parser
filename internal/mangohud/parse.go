// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

package mangohud

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// maxLine bounds one log line. A real line is a few hundred bytes, so this
// only stops a corrupt file from exhausting memory.
const maxLine = 1 << 20

// The marker lines of the versioned layout, which MangoHud prints when
// log_versioning is on.
const (
	versionedFirstLine = "v1"
	systemMarker       = "---------------------SYSTEM INFO---------------------"
	frameMarker        = "--------------------FRAME METRICS--------------------"
)

const (
	truncatedNote = "the last line was cut off and skipped"
	unsplitNote   = "the operating system, CPU and GPU names hold a comma, so they are shown as one text"
)

type parser struct {
	name string
	sc   *bufio.Scanner
	// line is the 1-based number of the line being handled and text is that
	// line without its line break.
	line int
	text string
	// unterminated tells that the line being handled has no line break, which
	// only the last line of a file can lack.
	unterminated bool
}

// Parse reads one log. The name only labels errors, which read
// "<name>:<line>: <problem>: <quoted line>".
//
// A log must have the fps, frametime and elapsed columns, at least one sample,
// the same number of fields in every row and an elapsed time that never goes
// down. The one malformed line that is not an error is a last line that has
// no line break and too few fields, which is what a game killed in the middle
// of a write leaves behind. Parse drops that line and adds a note to the log.
func Parse(name string, r io.Reader) (*Log, error) {
	p := &parser{name: name, sc: bufio.NewScanner(r)}
	p.sc.Buffer(nil, maxLine)
	p.sc.Split(p.splitLines)

	l := &Log{Source: name}
	k, err := p.header(l)
	if err != nil {
		return nil, err
	}
	if err := p.system(l, k); err != nil {
		return nil, err
	}
	if err := p.frames(l); err != nil {
		return nil, err
	}
	return l, nil
}

// ParseFile parses the log at path.
func ParseFile(path string) (*Log, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(path, f)
}

// splitLines cuts the input at "\n" like bufio.ScanLines, and also records
// whether the line it hands out had one.
func (p *parser) splitLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		p.unterminated = false
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		p.unterminated = true
		return len(data), data, nil
	}
	return 0, nil, nil
}

// next moves to the next line. A line written on Windows or edited by hand can
// carry a CR, which MangoHud never prints.
func (p *parser) next() bool {
	if !p.sc.Scan() {
		return false
	}
	p.line++
	p.text = strings.TrimSuffix(p.sc.Text(), "\r")
	return true
}

// need moves to the next line and fails if there is none. The what argument
// names the line that the format expects there.
func (p *parser) need(what string) error {
	if p.next() {
		return nil
	}
	if err := p.sc.Err(); err != nil {
		return p.scanError(err)
	}
	if p.line == 0 {
		return fmt.Errorf("%s:1: empty file", p.name)
	}
	return fmt.Errorf("%s:%d: unexpected end of file, missing %s", p.name, p.line+1, what)
}

func (p *parser) scanError(err error) error {
	if errors.Is(err, bufio.ErrTooLong) {
		return fmt.Errorf("%s:%d: line is longer than %d bytes", p.name, p.line+1, maxLine)
	}
	return fmt.Errorf("%s:%d: %w", p.name, p.line+1, err)
}

// fail builds an error for the current line.
func (p *parser) fail(format string, args ...any) error {
	return fmt.Errorf("%s:%d: %s: %q", p.name, p.line, fmt.Sprintf(format, args...), p.text)
}

// header reads the first lines of either layout up to and including the system
// information header, and returns the number of names that header has.
func (p *parser) header(l *Log) (int, error) {
	if err := p.need("the first line"); err != nil {
		return 0, err
	}
	if k := systemHeaderLen(p.text); k > 0 {
		return k, nil
	}
	if p.text != versionedFirstLine {
		return 0, p.fail("not a MangoHud log, expected %q or the system information header", versionedFirstLine)
	}
	l.Versioned = true
	if err := p.need("the MangoHud version"); err != nil {
		return 0, err
	}
	if l.Version = strings.TrimSpace(p.text); l.Version == "" {
		return 0, p.fail("the MangoHud version is empty")
	}
	if err := p.need("the system information marker"); err != nil {
		return 0, err
	}
	if p.text != systemMarker {
		return 0, p.fail("expected the system information marker %q", systemMarker)
	}
	if err := p.need("the system information header"); err != nil {
		return 0, err
	}
	k := systemHeaderLen(p.text)
	if k == 0 {
		return 0, p.fail("expected the system information header %q", strings.Join(systemNames, ","))
	}
	return k, nil
}

// system reads the values below the system information header and, for the
// versioned layout, the marker that ends the section.
func (p *parser) system(l *Log, k int) error {
	if err := p.need("the system information values"); err != nil {
		return err
	}
	var err error
	if l.System, err = parseSystem(p.text, k); err != nil {
		return p.fail("%v", err)
	}
	if l.System.Hardware != "" {
		l.Notes = append(l.Notes, unsplitNote)
	}
	if !l.Versioned {
		return nil
	}
	if err := p.need("the frame metrics marker"); err != nil {
		return err
	}
	if p.text != frameMarker {
		return p.fail("expected the frame metrics marker %q", frameMarker)
	}
	return nil
}

// frames reads the column header and every sample below it.
func (p *parser) frames(l *Log) error {
	if err := p.need("the column header"); err != nil {
		return err
	}
	names := strings.Split(p.text, ",")
	elapsedAt, err := p.columns(l, names)
	if err != nil {
		return err
	}
	for p.next() {
		// A second session in the same file starts with the first line of a
		// layout, which would otherwise only show as a row of the wrong width.
		if p.text == versionedFirstLine || systemHeaderLen(p.text) > 0 {
			return p.fail("this file holds more than one logging session")
		}
		if n := strings.Count(p.text, ",") + 1; n != len(names) {
			if p.unterminated && n < len(names) {
				l.Notes = append(l.Notes, truncatedNote)
				break
			}
			return p.fail("expected %d fields, found %d", len(names), n)
		}
		if err := p.row(l, elapsedAt); err != nil {
			return err
		}
	}
	if err := p.sc.Err(); err != nil {
		return p.scanError(err)
	}
	if l.Len() == 0 {
		return fmt.Errorf("%s:%d: no data rows", p.name, p.line+1)
	}
	return nil
}

// columns checks the header names, fills in l.Columns and l.Data, and returns
// the position of the elapsed column. Elapsed is not part of l.Columns.
func (p *parser) columns(l *Log, names []string) (int, error) {
	seen := map[string]bool{}
	elapsedAt := -1
	for i, n := range names {
		switch {
		case n == "":
			return 0, p.fail("column %d has no name", i+1)
		case seen[n]:
			return 0, p.fail("column %q appears twice", n)
		}
		seen[n] = true
		if n == elapsedName {
			elapsedAt = i
			continue
		}
		l.Columns = append(l.Columns, describe(n))
	}
	for _, required := range []string{fpsName, frametimeName, elapsedName} {
		if !seen[required] {
			return 0, p.fail("the %s column is missing", required)
		}
	}
	l.Data = make([][]float64, len(l.Columns))
	return elapsedAt, nil
}

// row reads the fields of the current line, which has the right count.
func (p *parser) row(l *Log, elapsedAt int) error {
	rest := p.text
	col := 0
	for i := 0; i <= len(l.Columns); i++ {
		var field string
		field, rest, _ = strings.Cut(rest, ",")
		if i == elapsedAt {
			v, err := strconv.ParseInt(field, 10, 64)
			if err != nil || v < 0 {
				return p.fail("elapsed %q is not a count of nanoseconds", field)
			}
			if n := len(l.Elapsed); n > 0 && v < l.Elapsed[n-1] {
				return p.fail("elapsed goes back from %d to %d", l.Elapsed[n-1], v)
			}
			l.Elapsed = append(l.Elapsed, v)
			continue
		}
		v, err := parseNumber(field)
		if err != nil {
			return p.fail("%q in column %s is not a number", field, l.Columns[col].Name)
		}
		l.Data[col] = append(l.Data[col], v)
		col++
	}
	return nil
}
