// Package qa analyzes a repository's tests: line coverage read from
// standard report formats, the behaviors that tests describe, and the
// hard-wired dependencies that make untested code difficult to test.
//
// Nothing in this package executes code. Coverage comes from report files
// that the developer's own test tooling produced.
package qa

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Report formats understood by Parse.
const (
	FormatGo        = "go"
	FormatLCOV      = "lcov"
	FormatCobertura = "cobertura"
	FormatJaCoCo    = "jacoco"
)

// FileCoverage is line coverage for one file.
type FileCoverage struct {
	Path string
	// Hits maps every instrumented line to its execution count.
	Hits map[int]int
	// Partial marks executed lines where some branches or blocks did not run.
	Partial map[int]bool
}

// newFile creates empty coverage for a path.
func newFile(p string) *FileCoverage {
	return &FileCoverage{Path: p, Hits: map[int]int{}, Partial: map[int]bool{}}
}

// Counts returns the number of covered and instrumented lines.
func (f *FileCoverage) Counts() (covered, total int) {
	for _, h := range f.Hits {
		total++
		if h > 0 {
			covered++
		}
	}
	return covered, total
}

// CountsIn returns covered and instrumented line counts within [from, to].
func (f *FileCoverage) CountsIn(from, to int) (covered, total int) {
	for ln, h := range f.Hits {
		if ln < from || ln > to {
			continue
		}
		total++
		if h > 0 {
			covered++
		}
	}
	return covered, total
}

// Lines splits instrumented lines into covered, uncovered and partial,
// each sorted ascending. Partial lines also appear in covered.
func (f *FileCoverage) Lines() (covered, uncovered, partial []int) {
	// Never nil: these go to the UI as JSON arrays, and a nil slice would be
	// null (a file with no covered lines, say).
	covered, uncovered, partial = []int{}, []int{}, []int{}
	for ln, h := range f.Hits {
		if h > 0 {
			covered = append(covered, ln)
			if f.Partial[ln] {
				partial = append(partial, ln)
			}
		} else {
			uncovered = append(uncovered, ln)
		}
	}
	sort.Ints(covered)
	sort.Ints(uncovered)
	sort.Ints(partial)
	return covered, uncovered, partial
}

// merge folds o into f: execution counts add up, and a line stays partial
// only if no other report fully covered it.
func (f *FileCoverage) merge(o *FileCoverage) {
	for ln, h := range o.Hits {
		prev, seen := f.Hits[ln]
		f.Hits[ln] = prev + h
		switch {
		case !seen:
			f.Partial[ln] = o.Partial[ln]
		case h > 0 && !o.Partial[ln], prev > 0 && !f.Partial[ln]:
			f.Partial[ln] = false
		default:
			f.Partial[ln] = f.Partial[ln] || o.Partial[ln]
		}
	}
}

// Source describes one report file merged into a Report.
type Source struct {
	Path    string    `json:"path"`
	Format  string    `json:"format"`
	Suite   string    `json:"suite,omitempty"`
	Files   int       `json:"files"`
	Skipped int       `json:"skipped,omitempty"` // entries outside the repo or hidden
	ModTime time.Time `json:"modTime"`
}

// Report is merged line coverage keyed by repo-relative path.
type Report struct {
	Files   map[string]*FileCoverage
	Sources []Source
	// Generated is when the newest merged report was written.
	Generated time.Time
}

// NewReport creates an empty report.
func NewReport() *Report { return &Report{Files: map[string]*FileCoverage{}} }

// Totals returns covered and instrumented lines across all files.
func (r *Report) Totals() (covered, total int) {
	for _, f := range r.Files {
		c, t := f.Counts()
		covered += c
		total += t
	}
	return covered, total
}

// Merge folds another report into r.
func (r *Report) Merge(o *Report) {
	for p, fc := range o.Files {
		dst := r.Files[p]
		if dst == nil {
			dst = newFile(p)
			r.Files[p] = dst
		}
		dst.merge(fc)
	}
	r.Sources = append(r.Sources, o.Sources...)
	if o.Generated.After(r.Generated) {
		r.Generated = o.Generated
	}
}

// File returns coverage for one path, or nil.
func (r *Report) File(p string) *FileCoverage {
	if r == nil {
		return nil
	}
	return r.Files[p]
}

// SetLine records a line's hit count (used when restoring saved reports).
func (f *FileCoverage) SetLine(line, hits int, partial bool) {
	f.Hits[line] = hits
	if partial {
		f.Partial[line] = true
	}
}

// NewFile creates empty coverage for a repo-relative path.
func NewFile(p string) *FileCoverage { return newFile(p) }

// Add merges parsed coverage into the report, mapping each raw path
// through resolve. Paths that resolve to nothing are counted as skipped.
func (r *Report) Add(raw *Raw, src Source, resolve func(raw string, bases []string) (string, bool)) Source {
	src.Format = raw.Format
	for p, fc := range raw.Files {
		rel, ok := resolve(p, raw.Bases)
		if !ok {
			src.Skipped++
			continue
		}
		dst := r.Files[rel]
		if dst == nil {
			dst = newFile(rel)
			r.Files[rel] = dst
		}
		dst.merge(fc)
		src.Files++
	}
	r.Sources = append(r.Sources, src)
	if src.ModTime.After(r.Generated) {
		r.Generated = src.ModTime
	}
	return src
}

// Raw is a parsed report whose paths are still as the tool wrote them.
type Raw struct {
	Format string
	Files  map[string]*FileCoverage
	// Bases are source roots declared by the report (Cobertura <source>).
	Bases []string
}

// ErrUnknownFormat is returned when data is not a recognized report.
var ErrUnknownFormat = errors.New("not a recognized coverage report (expected Go coverprofile, LCOV, Cobertura or JaCoCo XML)")

// Detect identifies a report's format from its first bytes.
func Detect(data []byte) string {
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}
	t := bytes.TrimSpace(head)
	switch {
	case bytes.HasPrefix(t, []byte("mode:")):
		return FormatGo
	case bytes.Contains(head, []byte("<report")) && (bytes.Contains(head, []byte("jacoco")) || bytes.Contains(head, []byte("<sessioninfo")) || bytes.Contains(head, []byte("<package"))):
		return FormatJaCoCo
	case bytes.Contains(head, []byte("<coverage")):
		return FormatCobertura
	case bytes.HasPrefix(t, []byte("TN:")) || bytes.HasPrefix(t, []byte("SF:")) || bytes.Contains(head, []byte("\nSF:")):
		return FormatLCOV
	}
	return ""
}

// Parse reads a coverage report in any supported format.
func Parse(data []byte) (*Raw, error) {
	switch Detect(data) {
	case FormatGo:
		return parseGo(data)
	case FormatLCOV:
		return parseLCOV(data)
	case FormatCobertura:
		return parseCobertura(data)
	case FormatJaCoCo:
		return parseJaCoCo(data)
	}
	return nil, ErrUnknownFormat
}

// parseGo reads a `go test -coverprofile` file. Blocks are first merged
// across test binaries (with -coverpkg the same block appears once per
// package), then spread onto lines.
func parseGo(data []byte) (*Raw, error) {
	type block struct {
		file           string
		sl, sc, el, ec int
	}
	counts := map[block]int{}
	var order []block
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// path/file.go:12.34,15.2 3 1
		colon := strings.LastIndex(line, ":")
		if colon < 0 {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		if len(fields) != 3 {
			continue
		}
		var b block
		b.file = line[:colon]
		if _, err := fmt.Sscanf(fields[0], "%d.%d,%d.%d", &b.sl, &b.sc, &b.el, &b.ec); err != nil {
			continue
		}
		n, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		if prev, ok := counts[b]; !ok {
			order = append(order, b)
			counts[b] = n
		} else {
			counts[b] = max(prev, n)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	raw := &Raw{Format: FormatGo, Files: map[string]*FileCoverage{}}
	missed := map[string]map[int]bool{}
	for _, b := range order {
		fc := raw.Files[b.file]
		if fc == nil {
			fc = newFile(b.file)
			raw.Files[b.file] = fc
			missed[b.file] = map[int]bool{}
		}
		n := counts[b]
		for ln := b.sl; ln <= b.el; ln++ {
			fc.Hits[ln] = max(fc.Hits[ln], n)
			if n == 0 {
				missed[b.file][ln] = true
			}
		}
	}
	for f, lines := range missed {
		for ln := range lines {
			if raw.Files[f].Hits[ln] > 0 {
				raw.Files[f].Partial[ln] = true
			}
		}
	}
	return raw, nil
}

// parseLCOV reads an LCOV tracefile (Jest, Vitest, c8, nyc, pytest-cov,
// cargo-llvm-cov, lcov itself...).
func parseLCOV(data []byte) (*Raw, error) {
	raw := &Raw{Format: FormatLCOV, Files: map[string]*FileCoverage{}}
	var cur *FileCoverage
	taken := map[int][2]int{} // line -> {branches taken, branches not taken}
	flush := func() {
		if cur == nil {
			return
		}
		for ln, tb := range taken {
			if tb[0] > 0 && tb[1] > 0 && cur.Hits[ln] > 0 {
				cur.Partial[ln] = true
			}
		}
		if prev := raw.Files[cur.Path]; prev != nil {
			prev.merge(cur)
		} else {
			raw.Files[cur.Path] = cur
		}
		cur, taken = nil, map[int][2]int{}
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		key, val, _ := strings.Cut(line, ":")
		switch key {
		case "SF":
			flush()
			cur = newFile(val)
		case "DA":
			if cur == nil {
				continue
			}
			parts := strings.Split(val, ",")
			if len(parts) < 2 {
				continue
			}
			ln, err1 := strconv.Atoi(parts[0])
			n, err2 := strconv.ParseFloat(parts[1], 64)
			if err1 != nil || err2 != nil || ln <= 0 {
				continue
			}
			cur.Hits[ln] = max(cur.Hits[ln], int(n))
		case "BRDA":
			if cur == nil {
				continue
			}
			parts := strings.Split(val, ",")
			if len(parts) < 4 {
				continue
			}
			ln, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			tb := taken[ln]
			if n, err := strconv.Atoi(parts[3]); err == nil && n > 0 {
				tb[0]++
			} else {
				tb[1]++
			}
			taken[ln] = tb
		case "end_of_record":
			flush()
		}
	}
	flush()
	return raw, sc.Err()
}

// xmlAttr returns an attribute value from a start element.
func xmlAttr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// xmlDecoder returns a decoder that tolerates DOCTYPEs and other charsets.
func xmlDecoder(data []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return d
}

// parseCobertura reads Cobertura XML (pytest-cov, coverage.py, .NET
// coverlet, gcovr, many CI tools).
func parseCobertura(data []byte) (*Raw, error) {
	raw := &Raw{Format: FormatCobertura, Files: map[string]*FileCoverage{}}
	d := xmlDecoder(data)
	var cur *FileCoverage
	inSource := false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "source":
				inSource = true
			case "class":
				fn := xmlAttr(e, "filename")
				cur = raw.Files[fn]
				if cur == nil {
					cur = newFile(fn)
					raw.Files[fn] = cur
				}
			case "line":
				if cur == nil {
					continue
				}
				ln, err := strconv.Atoi(xmlAttr(e, "number"))
				if err != nil || ln <= 0 {
					continue
				}
				n, _ := strconv.ParseFloat(xmlAttr(e, "hits"), 64)
				cur.Hits[ln] = max(cur.Hits[ln], int(n))
				// condition-coverage="50% (1/2)"
				if cc := xmlAttr(e, "condition-coverage"); cc != "" && !strings.HasPrefix(cc, "100%") && n > 0 {
					cur.Partial[ln] = true
				}
			}
		case xml.CharData:
			if inSource {
				if s := strings.TrimSpace(string(e)); s != "" {
					raw.Bases = append(raw.Bases, s)
				}
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "source":
				inSource = false
			case "class":
				cur = nil
			}
		}
	}
	return raw, nil
}

// parseJaCoCo reads a JaCoCo XML report (Java, Kotlin, Scala).
func parseJaCoCo(data []byte) (*Raw, error) {
	raw := &Raw{Format: FormatJaCoCo, Files: map[string]*FileCoverage{}}
	d := xmlDecoder(data)
	pkg := ""
	var cur *FileCoverage
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "package":
				pkg = xmlAttr(e, "name")
			case "sourcefile":
				p := path.Join(pkg, xmlAttr(e, "name"))
				cur = raw.Files[p]
				if cur == nil {
					cur = newFile(p)
					raw.Files[p] = cur
				}
			case "line":
				if cur == nil {
					continue
				}
				ln, err := strconv.Atoi(xmlAttr(e, "nr"))
				if err != nil || ln <= 0 {
					continue
				}
				mi, _ := strconv.Atoi(xmlAttr(e, "mi"))
				ci, _ := strconv.Atoi(xmlAttr(e, "ci"))
				mb, _ := strconv.Atoi(xmlAttr(e, "mb"))
				cb, _ := strconv.Atoi(xmlAttr(e, "cb"))
				if mi+ci == 0 {
					continue
				}
				cur.Hits[ln] = max(cur.Hits[ln], ci)
				if ci > 0 && (mi > 0 || (mb > 0 && cb > 0)) {
					cur.Partial[ln] = true
				}
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "sourcefile":
				cur = nil
			case "package":
				pkg = ""
			}
		}
	}
	return raw, nil
}

// Resolver maps the paths written in coverage reports to repo-relative
// paths of files that exist (and are visible) in the workspace.
type Resolver struct {
	// Root is the repository's absolute path.
	Root string
	// Files is the set of visible repo-relative files.
	Files map[string]bool
	// Modules maps Go module paths to the repo-relative directory of their
	// go.mod, for Go coverprofiles.
	Modules map[string]string
	// Dirs are repo-relative directories relative report paths may be
	// written against (each suite's working directory).
	Dirs []string

	bySuffix map[string][]string
}

// Resolve returns the repo-relative path for a raw report path.
func (r *Resolver) Resolve(raw string, bases []string) (string, bool) {
	p := strings.TrimPrefix(strings.TrimSpace(raw), "file://")
	p = filepath.ToSlash(p)
	if p == "" {
		return "", false
	}
	try := func(c string) (string, bool) {
		c = path.Clean(strings.TrimPrefix(c, "./"))
		if r.Files[c] {
			return c, true
		}
		return "", false
	}
	if isAbs(p) {
		root := filepath.ToSlash(r.Root)
		if rel, ok := strings.CutPrefix(p, strings.TrimSuffix(root, "/")+"/"); ok {
			return try(rel)
		}
	}
	for mod, dir := range r.Modules {
		if rest, ok := strings.CutPrefix(p, mod+"/"); ok {
			if rel, ok := try(path.Join(dir, rest)); ok {
				return rel, true
			}
		}
	}
	for _, b := range bases {
		b = filepath.ToSlash(b)
		if isAbs(b) {
			root := strings.TrimSuffix(filepath.ToSlash(r.Root), "/")
			if b == root {
				b = ""
			} else if rel, ok := strings.CutPrefix(b, root+"/"); ok {
				b = rel
			} else {
				continue
			}
		}
		if rel, ok := try(path.Join(b, p)); ok {
			return rel, true
		}
	}
	for _, d := range append(r.Dirs, "") {
		if rel, ok := try(path.Join(d, p)); ok {
			return rel, true
		}
	}
	return r.suffix(strings.TrimPrefix(p, "/"))
}

// suffix matches p against the end of a unique workspace path. Bare file
// names must be unambiguous.
func (r *Resolver) suffix(p string) (string, bool) {
	if r.bySuffix == nil {
		r.bySuffix = map[string][]string{}
		for f := range r.Files {
			parts := strings.Split(f, "/")
			for i := range parts {
				k := strings.Join(parts[i:], "/")
				r.bySuffix[k] = append(r.bySuffix[k], f)
			}
		}
	}
	// Strip leading segments until something matches (handles absolute
	// paths from another machine, e.g. CI runners).
	parts := strings.Split(p, "/")
	for i := range parts {
		if list := r.bySuffix[strings.Join(parts[i:], "/")]; len(list) == 1 {
			return list[0], true
		} else if len(list) > 1 {
			return "", false
		}
	}
	return "", false
}

// isAbs reports whether a slash path is absolute on any OS.
func isAbs(p string) bool {
	return strings.HasPrefix(p, "/") || (len(p) > 2 && p[1] == ':' && (p[2] == '/' || p[2] == '\\'))
}

// Ranges compresses sorted line numbers into "3-7, 12, 20-21".
func Ranges(lines []int) string {
	var sb strings.Builder
	for i := 0; i < len(lines); {
		j := i
		for j+1 < len(lines) && lines[j+1] == lines[j]+1 {
			j++
		}
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		if i == j {
			fmt.Fprintf(&sb, "%d", lines[i])
		} else {
			fmt.Fprintf(&sb, "%d-%d", lines[i], lines[j])
		}
		i = j + 1
	}
	return sb.String()
}
