package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/qa"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

// qaState is a repository's coverage, last test run, cached analysis and
// agent test plans.
type qaState struct {
	mu sync.Mutex
	// parts holds one coverage report per suite (or imported file); the
	// combined report is their merge.
	parts    map[string]*qa.Report
	combined *qa.Report
	loaded   bool
	version  int
	run      *QARun
	cancel   context.CancelFunc
	analysis *qaAnalysis
	key      string
	plans    []*TestPlan
	// perTest is per-test coverage by test ID (nil until loaded). It is
	// replaced, never modified, so readers may keep it without the lock.
	perTest map[string]*testCoverage
}

// QARun is a coverage run the developer started.
type QARun struct {
	ID       string        `json:"id"`
	Repo     string        `json:"repo"`
	State    string        `json:"state"` // running, done, failed, error, cancelled
	Started  time.Time     `json:"started"`
	Finished time.Time     `json:"finished,omitzero"`
	Suites   []SuiteResult `json:"suites"`
	Log      []string      `json:"log"`
}

// SuiteResult is the outcome of one suite in a run.
type SuiteResult struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	State      string   `json:"state"` // pending, running, passed, failed, error, cancelled
	ExitCode   int      `json:"exitCode"`
	Failures   []string `json:"failures,omitempty"`
	Report     string   `json:"report,omitempty"`
	Files      int      `json:"files"`
	Error      string   `json:"error,omitempty"`
	DurationMS int64    `json:"durationMs"`
}

// snapshot copies a run without its log, safe to publish while the run
// continues. Callers hold the repo's qa lock.
func (r *QARun) snapshot() QARun {
	c := *r
	c.Log = nil
	c.Suites = append([]SuiteResult(nil), r.Suites...)
	for i := range c.Suites {
		c.Suites[i].Failures = append([]string(nil), r.Suites[i].Failures...)
	}
	return c
}

// ---- suites ----------------------------------------------------------------

// QASuites returns the repository's coverage suites: the developer's saved
// ones, or detected defaults (reported by the bool).
func (s *Service) QASuites(rt *RepoHandle) ([]qa.Suite, bool) {
	if rt.cfg.Suites != nil {
		return rt.cfg.Suites, false
	}
	files, err := rt.ws.Files(defaults)
	if err != nil {
		return []qa.Suite{}, true
	}
	suites := qa.DetectSuites(files, func(p string) string {
		if fc, err := rt.ws.ReadFile(p, defaults); err == nil {
			return fc.Content
		}
		return ""
	})
	if suites == nil {
		suites = []qa.Suite{}
	}
	return suites, true
}

// SaveQASuites validates and stores the developer's coverage suites.
func (s *Service) SaveQASuites(rt *RepoHandle, suites []qa.Suite) error {
	seen := map[string]bool{}
	for i := range suites {
		su := &suites[i]
		su.Command = strings.TrimSpace(su.Command)
		dir, err := suiteDir(rt, su.Dir)
		if err != nil {
			return fmt.Errorf("suite %q: %w", su.Name, err)
		}
		su.Dir = dir
		if su.Command == "" {
			return fmt.Errorf("suite %q has no command", su.Name)
		}
		if su.ID == "" {
			su.ID = fmt.Sprintf("custom:%d", time.Now().UnixNano()+int64(i))
		}
		if seen[su.ID] {
			return fmt.Errorf("duplicate suite id %q", su.ID)
		}
		seen[su.ID] = true
	}
	if err := s.Config.SetSuites(rt.cfg.ID, suites); err != nil {
		return err
	}
	if r, ok := s.Config.FindRepo(rt.cfg.ID); ok {
		s.runtime(r)
	}
	s.emit(EvConfig, s.Config.Get())
	return nil
}

// suiteDir normalizes a suite's Folder to a clean repo-relative path ("" for
// the root) and checks that it is an existing directory inside the
// repository. Absolute paths are accepted when they point inside the repo,
// including through symlinks.
func suiteDir(rt *RepoHandle, dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", nil
	}
	if filepath.IsAbs(dir) {
		abs := filepath.Clean(dir)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			abs = real
		}
		rel, err := filepath.Rel(rt.cfg.Path, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("folder %s is outside the repository", dir)
		}
		dir = rel
	}
	dir = path.Clean(filepath.ToSlash(dir))
	if dir == "." {
		return "", nil
	}
	abs, rel, err := rt.ws.Resolve(dir, nil)
	if err != nil {
		return "", fmt.Errorf("folder %s: %w", dir, err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", fmt.Errorf("folder %s doesn't exist in the repository", dir)
	}
	return rel, nil
}

// ResetQASuites drops the developer's suites so detected defaults apply.
func (s *Service) ResetQASuites(rt *RepoHandle) error {
	if err := s.Config.SetSuites(rt.cfg.ID, nil); err != nil {
		return err
	}
	s.emit(EvConfig, s.Config.Get())
	return nil
}

// ---- coverage storage --------------------------------------------------------

// cachePath is where a repo's coverage is saved between sessions.
func (s *Service) cachePath(rt *RepoHandle) string {
	return filepath.Join(s.CacheDir, "coverage", rt.cfg.ID+".json")
}

type storedFile struct {
	Hits    [][2]int `json:"h"`
	Partial []int    `json:"p,omitempty"`
}

type storedPart struct {
	Generated time.Time             `json:"generated"`
	Sources   []qa.Source           `json:"sources"`
	Files     map[string]storedFile `json:"files"`
}

// saveCoverage writes the coverage parts to the cache directory. Callers
// hold st.mu.
func (s *Service) saveCoverage(rt *RepoHandle, st *qaState) {
	out := map[string]storedPart{}
	for k, r := range st.parts {
		sp := storedPart{Generated: r.Generated, Sources: r.Sources, Files: map[string]storedFile{}}
		for p, fc := range r.Files {
			var f storedFile
			for ln, h := range fc.Hits {
				f.Hits = append(f.Hits, [2]int{ln, h})
				if fc.Partial[ln] {
					f.Partial = append(f.Partial, ln)
				}
			}
			sp.Files[p] = f
		}
		out[k] = sp
	}
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	p := s.cachePath(rt)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
		_ = os.WriteFile(p, data, 0o644)
	}
}

// loadCoverage restores saved coverage parts. Callers hold st.mu.
func (s *Service) loadCoverage(rt *RepoHandle, st *qaState) bool {
	data, err := os.ReadFile(s.cachePath(rt))
	if err != nil {
		return false
	}
	var in map[string]storedPart
	if json.Unmarshal(data, &in) != nil {
		return false
	}
	for k, sp := range in {
		r := qa.NewReport()
		r.Generated, r.Sources = sp.Generated, sp.Sources
		for p, f := range sp.Files {
			fc := qa.NewFile(p)
			partial := map[int]bool{}
			for _, ln := range f.Partial {
				partial[ln] = true
			}
			for _, h := range f.Hits {
				fc.SetLine(h[0], h[1], partial[h[0]])
			}
			r.Files[p] = fc
		}
		st.parts[k] = r
	}
	return len(in) > 0
}

// rebuild recomputes the combined report from its parts. Callers hold st.mu.
func (st *qaState) rebuild() {
	st.version++
	st.analysis = nil
	if len(st.parts) == 0 {
		st.combined = nil
		return
	}
	keys := make([]string, 0, len(st.parts))
	for k := range st.parts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c := qa.NewReport()
	for _, k := range keys {
		c.Merge(st.parts[k])
	}
	st.combined = c
}

// resolver maps report paths onto visible repository files.
func (s *Service) resolver(rt *RepoHandle) *qa.Resolver {
	files, _ := rt.ws.Files(defaults)
	r := &qa.Resolver{Root: rt.cfg.Path, Files: map[string]bool{}, Modules: map[string]string{}}
	for _, f := range files {
		r.Files[f] = true
		if path.Base(f) == "go.mod" {
			if fc, err := rt.ws.ReadFile(f, defaults); err == nil {
				if m := goModule.FindStringSubmatch(fc.Content); m != nil {
					dir := path.Dir(f)
					if dir == "." {
						dir = ""
					}
					r.Modules[m[1]] = dir
				}
			}
		}
	}
	suites, _ := s.QASuites(rt)
	for _, su := range suites {
		if su.Dir != "" {
			r.Dirs = append(r.Dirs, su.Dir)
		}
	}
	return r
}

var goModule = regexp.MustCompile(`(?m)^module\s+(\S+)`)

// parseReport reads and parses a coverage file into a report keyed by
// repo-relative paths.
func parseReport(file, suite string, res *qa.Resolver) (*qa.Report, qa.Source, error) {
	st, err := os.Stat(file)
	if err != nil {
		return nil, qa.Source{}, err
	}
	if st.Size() > 512<<20 {
		return nil, qa.Source{}, fmt.Errorf("%s is too large (%d MB)", file, st.Size()>>20)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, qa.Source{}, err
	}
	raw, err := qa.Parse(data)
	if err != nil {
		return nil, qa.Source{}, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	r := qa.NewReport()
	src := r.Add(raw, qa.Source{Path: file, Suite: suite, ModTime: st.ModTime()}, res.Resolve)
	return r, src, nil
}

// coverage returns the repo's combined coverage, loading the saved copy or
// importing reports already present in the repository on first use. It
// may be nil.
func (s *Service) coverage(rt *RepoHandle) (*qa.Report, int) {
	st := &rt.qa
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.loaded {
		st.loaded = true
		if st.parts == nil {
			st.parts = map[string]*qa.Report{}
		}
		if !s.loadCoverage(rt, st) {
			s.importKnownLocked(rt, st)
		}
		st.rebuild()
	}
	return st.combined, st.version
}

// importKnownLocked imports coverage reports that tools left in the
// repository (coverage.out, coverage/lcov.info, ...). Callers hold st.mu.
func (s *Service) importKnownLocked(rt *RepoHandle, st *qaState) int {
	res := s.resolver(rt)
	dirs := []string{""}
	suites, _ := s.QASuites(rt)
	for _, su := range suites {
		if su.Dir != "" {
			dirs = append(dirs, su.Dir)
		}
	}
	n := 0
	for _, d := range dirs {
		for _, k := range qa.KnownReports {
			rel := path.Join(d, k)
			abs, _, err := rt.ws.Resolve(rel, nil)
			if err != nil {
				continue
			}
			r, src, err := parseReport(abs, "", res)
			if err != nil || src.Files == 0 {
				continue
			}
			st.parts["import:"+rel] = r
			n++
		}
	}
	return n
}

// ImportCoverage merges a coverage report the developer points at
// (absolute, or relative to the repository).
func (s *Service) ImportCoverage(rt *RepoHandle, file string) (qa.Source, error) {
	s.coverage(rt)
	abs := file
	if !filepath.IsAbs(abs) {
		a, _, err := rt.ws.Resolve(file, nil)
		if err != nil {
			return qa.Source{}, err
		}
		abs = a
	}
	r, src, err := parseReport(abs, "", s.resolver(rt))
	if err != nil {
		return src, err
	}
	if src.Files == 0 {
		return src, fmt.Errorf("none of the %d files in that report are in this repository", src.Skipped)
	}
	st := &rt.qa
	st.mu.Lock()
	st.parts["import:"+abs] = r
	st.rebuild()
	s.saveCoverage(rt, st)
	st.mu.Unlock()
	s.emit(EvQA, map[string]any{"repo": rt.cfg.ID, "kind": "coverage"})
	return src, nil
}

// ClearCoverage forgets all coverage for a repository.
func (s *Service) ClearCoverage(rt *RepoHandle) {
	st := &rt.qa
	st.mu.Lock()
	st.loaded = true
	st.parts = map[string]*qa.Report{}
	st.rebuild()
	_ = os.Remove(s.cachePath(rt))
	st.mu.Unlock()
	s.clearPerTest(rt)
	s.emit(EvQA, map[string]any{"repo": rt.cfg.ID, "kind": "coverage"})
}

// ---- running -----------------------------------------------------------------

const maxRunLog = 3000

// RunCoverage starts the selected suites (or every enabled one) in the
// background. Only the desktop UI calls this; agents can't run commands.
func (s *Service) RunCoverage(rt *RepoHandle, ids []string) (*QARun, error) {
	s.coverage(rt)
	all, _ := s.QASuites(rt)
	var suites []qa.Suite
	for _, su := range all {
		if (len(ids) == 0 && su.Enabled) || contains(ids, su.ID) {
			suites = append(suites, su)
		}
	}
	if len(suites) == 0 {
		return nil, errors.New("no coverage suites selected; configure one in the QA view")
	}
	return s.startRun(rt, suites, nil)
}

// collectFunc stores a finished job's report itself instead of merging it
// into the repository's coverage (per-test runs).
type collectFunc func(su qa.Suite, report string, res *qa.Resolver, result qa.RunResult) error

// startRun starts suites in the background unless a run is in progress.
func (s *Service) startRun(rt *RepoHandle, suites []qa.Suite, collect collectFunc) (*QARun, error) {
	st := &rt.qa
	st.mu.Lock()
	if st.run != nil && st.run.State == "running" {
		st.mu.Unlock()
		return nil, errors.New("a coverage run is already in progress")
	}
	run := &QARun{ID: fmt.Sprintf("run%d", time.Now().UnixNano()), Repo: rt.cfg.ID, State: "running", Started: time.Now(), Log: []string{}}
	for _, su := range suites {
		run.Suites = append(run.Suites, SuiteResult{ID: su.ID, Name: su.Name, State: "pending"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	st.run, st.cancel = run, cancel
	snapshot := run.snapshot()
	st.mu.Unlock()
	s.emit(EvQA, map[string]any{"repo": rt.cfg.ID, "kind": "run", "run": snapshot})
	go s.runSuites(ctx, cancel, rt, run, suites, collect)
	return &snapshot, nil
}

// contains reports whether list holds v.
func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// runSuites executes suites one after another, streaming log lines in
// batches and importing each suite's report as it finishes (or handing it
// to collect).
func (s *Service) runSuites(ctx context.Context, cancel context.CancelFunc, rt *RepoHandle, run *QARun, suites []qa.Suite, collect collectFunc) {
	defer cancel()
	st := &rt.qa
	var pending []string
	flush := func() {
		st.mu.Lock()
		lines := pending
		pending = nil
		st.mu.Unlock()
		if len(lines) > 0 {
			s.emit(EvQA, map[string]any{"repo": rt.cfg.ID, "kind": "log", "run": run.ID, "lines": lines})
		}
	}
	logLine := func(l string) {
		st.mu.Lock()
		run.Log = append(run.Log, l)
		if len(run.Log) > maxRunLog {
			run.Log = run.Log[len(run.Log)-maxRunLog:]
		}
		pending = append(pending, l)
		st.mu.Unlock()
	}
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				flush()
			case <-stop:
				return
			}
		}
	}()
	update := func(i int, fn func(*SuiteResult)) {
		st.mu.Lock()
		fn(&run.Suites[i])
		snapshot := run.snapshot()
		st.mu.Unlock()
		s.emit(EvQA, map[string]any{"repo": rt.cfg.ID, "kind": "run", "run": snapshot})
	}
	res := s.resolver(rt)
	for i, su := range suites {
		if ctx.Err() != nil {
			update(i, func(r *SuiteResult) { r.State = "cancelled" })
			continue
		}
		update(i, func(r *SuiteResult) { r.State = "running" })
		start := time.Now()
		dir, _, err := rt.ws.Resolve(su.Dir, nil)
		if su.Dir == "" {
			dir, err = rt.cfg.Path, nil
		}
		if err == nil {
			// A missing working directory would otherwise surface as the
			// misleading "fork/exec /usr/bin/sh: no such file or directory".
			if st, serr := os.Stat(dir); serr != nil || !st.IsDir() {
				err = fmt.Errorf("suite folder %q doesn't exist; fix it in the Suites panel", su.Dir)
			}
		}
		out := filepath.Join(s.CacheDir, "runs", rt.cfg.ID, safeName(su.ID))
		if err == nil {
			_ = os.RemoveAll(out)
			err = os.MkdirAll(out, 0o755)
		}
		if err != nil {
			update(i, func(r *SuiteResult) { r.State, r.Error = "error", err.Error() })
			continue
		}
		command := qa.Expand(su.Command, out)
		logLine(fmt.Sprintf("── %s ──", su.Name))
		logLine("in " + dir)
		logLine("$ " + command)
		result, err := qa.Run(ctx, dir, command, logLine)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			state := "error"
			if ctx.Err() != nil {
				state = "cancelled"
			}
			logLine("✗ " + err.Error())
			update(i, func(r *SuiteResult) { r.State, r.Error, r.DurationMS = state, err.Error(), dur })
			continue
		}
		reportPath := qa.ExpandPath(su.Report, out)
		if !filepath.IsAbs(reportPath) {
			reportPath = filepath.Join(dir, filepath.FromSlash(reportPath))
		}
		found, ferr := qa.FindReport(reportPath)
		if ferr != nil {
			st.mu.Lock()
			tail := run.Log[max(0, len(run.Log)-200):]
			hint := qa.SetupHint(tail)
			st.mu.Unlock()
			if hint != "" {
				ferr = fmt.Errorf("%s (%w)", hint, ferr)
			}
		}
		var rep *qa.Report
		var src qa.Source
		if ferr == nil {
			if collect != nil {
				ferr = collect(su, found, res, result)
			} else {
				rep, src, ferr = parseReport(found, su.Name, res)
			}
		}
		update(i, func(r *SuiteResult) {
			r.ExitCode, r.Failures, r.DurationMS = result.ExitCode, result.Failures, dur
			r.State = "passed"
			if result.ExitCode != 0 {
				r.State = "failed"
			}
			if ferr != nil {
				r.Error = "no coverage report: " + ferr.Error()
				if result.ExitCode == 0 {
					r.State = "error"
				}
				return
			}
			r.Report, r.Files = found, src.Files
		})
		if rep != nil {
			st.mu.Lock()
			st.parts[su.ID] = rep
			st.rebuild()
			s.saveCoverage(rt, st)
			st.mu.Unlock()
		}
	}
	close(stop)
	flush()
	st.mu.Lock()
	run.Finished = time.Now()
	run.State = "done"
	for _, r := range run.Suites {
		switch {
		case r.State == "cancelled":
			run.State = "cancelled"
		case r.State == "error" && run.State != "cancelled":
			run.State = "error"
		case r.State == "failed" && run.State == "done":
			run.State = "failed"
		}
	}
	st.cancel = nil
	snapshot := run.snapshot()
	st.mu.Unlock()
	s.emit(EvQA, map[string]any{"repo": rt.cfg.ID, "kind": "run", "run": snapshot})
	s.emit(EvQA, map[string]any{"repo": rt.cfg.ID, "kind": "coverage"})
}

// safeName makes an ID usable as a directory name.
func safeName(id string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == ' ' || r == '.' {
			return '_'
		}
		return r
	}, id)
}

// CancelCoverage stops the running coverage run, if any.
func (s *Service) CancelCoverage(rt *RepoHandle) {
	rt.qa.mu.Lock()
	cancel := rt.qa.cancel
	rt.qa.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StopCoverageRuns cancels every running coverage run (killing the test
// processes they started) and waits up to grace for them to wind down. It
// is called when CodeDiver exits so no test processes are left behind.
func (s *Service) StopCoverageRuns(grace time.Duration) {
	s.mu.Lock()
	handles := make([]*RepoHandle, 0, len(s.repos))
	for _, rt := range s.repos {
		handles = append(handles, rt)
	}
	s.mu.Unlock()
	for _, rt := range handles {
		s.CancelCoverage(rt)
	}
	deadline := time.Now().Add(grace)
	for _, rt := range handles {
		for time.Now().Before(deadline) {
			if r := s.LastRun(rt); r == nil || r.State != "running" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// LastRun returns the most recent coverage run including its log.
func (s *Service) LastRun(rt *RepoHandle) *QARun {
	rt.qa.mu.Lock()
	defer rt.qa.mu.Unlock()
	if rt.qa.run == nil {
		return nil
	}
	r := *rt.qa.run
	r.Log = append([]string(nil), r.Log...)
	r.Suites = append([]SuiteResult(nil), r.Suites...)
	return &r
}

// ---- analysis ----------------------------------------------------------------

// FuncQA is the test status of one function or method.
type FuncQA struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	File       string  `json:"file"`
	Line       int     `json:"line"`
	EndLine    int     `json:"endLine"`
	Complexity int     `json:"complexity"`
	Callers    int     `json:"callers"`
	Exported   bool    `json:"exported"`
	Covered    int     `json:"covered"`
	Lines      int     `json:"lines"` // instrumented lines
	Pct        float64 `json:"pct"`   // -1 when there is no coverage data
	// Tests are IDs of tests that call the function directly.
	Tests    []string `json:"tests,omitempty"`
	Direct   int      `json:"direct"`
	Indirect int      `json:"indirect"` // tests that reach it through calls
	Risk     float64  `json:"risk"`
	Reason   string   `json:"reason,omitempty"`
	// Uncovered and Hints are filled in for gaps.
	Uncovered string    `json:"uncovered,omitempty"`
	Hints     []qa.Hint `json:"hints,omitempty"`
	Heuristic bool      `json:"heuristic,omitempty"`
}

type qaAnalysis struct {
	tests  []qa.TestCase
	funcs  []*FuncQA
	byID   map[string]*FuncQA
	report *qa.Report
	files  []string
}

var (
	jsImport = regexp.MustCompile(`(?:from\s+|require\(\s*|import\s*\(?\s*)['"](\.{1,2}/[^'"]+)['"]`)
	pyImport = regexp.MustCompile(`(?m)^\s*from\s+([\w.]+)\s+import|^\s*import\s+([\w.]+)`)
	helperRe = regexp.MustCompile(`^\s*(?:func\s+(?:\([^)]*\)\s*)?|function\s+|def\s+|(?:export\s+)?(?:const|let)\s+)(\w+)\s*(?:=\s*(?:async\s*)?\(|\()`)
)

// testable reports whether a test case runs code (suites and table rows
// don't on their own).
func testable(tc *qa.TestCase) bool {
	return tc.Kind == qa.KindTest || tc.Kind == qa.KindSubtest || tc.Kind == qa.KindBenchmark || tc.Kind == qa.KindFuzz || tc.Kind == qa.KindExample
}

// qaAnalyze discovers tests, maps them to the code they exercise and
// scores every function, caching the result until code or coverage change.
func (s *Service) qaAnalyze(ctx context.Context, rt *RepoHandle) (*qaAnalysis, error) {
	ix, err := s.Index(ctx, rt)
	if err != nil {
		return nil, err
	}
	report, version := s.coverage(rt)
	rt.idx.mu.Lock()
	key := fmt.Sprintf("%s/%d", rt.idx.print, version)
	rt.idx.mu.Unlock()
	st := &rt.qa
	st.mu.Lock()
	if st.analysis != nil && st.key == key {
		a := st.analysis
		st.mu.Unlock()
		return a, nil
	}
	st.mu.Unlock()

	files, err := rt.ws.Files(defaults)
	if err != nil {
		return nil, err
	}
	a := &qaAnalysis{byID: map[string]*FuncQA{}, report: report, files: files}
	contents := map[string][]string{}
	readLines := func(f string) []string {
		if l, ok := contents[f]; ok {
			return l
		}
		var lines []string
		if fc, err := rt.ws.ReadFile(f, defaults); err == nil && !fc.Binary {
			lines = strings.Split(fc.Content, "\n")
		}
		contents[f] = lines
		return lines
	}

	// Discover tests.
	suites, _ := s.QASuites(rt)
	isTest := map[string]bool{}
	for _, f := range files {
		lang := workspace.Language(f)
		if !workspace.IsCode(lang) {
			continue
		}
		// Tests of fixtures and example projects aren't this project's tests.
		if qa.InTestdata(f) {
			continue
		}
		named := qa.IsTestFile(f)
		if !named && !strings.HasSuffix(f, ".rs") {
			continue
		}
		lines := readLines(f)
		src := strings.Join(lines, "\n")
		if !named && !qa.MayContainTests(f, src) {
			continue
		}
		if named {
			isTest[f] = true
		}
		found := qa.Discover(f, lang, src)
		if tool := suiteTool(suites, f); tool != "" {
			for i := range found {
				if found[i].Framework == "jest/vitest" {
					found[i].Framework = tool
				}
			}
		}
		a.tests = append(a.tests, found...)
	}

	// Callable code symbols by name (test files excluded).
	byName := map[string][]*analysis.Symbol{}
	for _, sym := range ix.Symbols {
		if sym.Kind == analysis.KindExternal || sym.File == "" || isTest[sym.File] {
			continue
		}
		byName[sym.Name] = append(byName[sym.Name], sym)
	}

	// Map each test to the symbols it calls directly.
	importsOf := map[string]map[string]bool{}
	helpersOf := map[string]map[string]string{}
	for i := range a.tests {
		tc := &a.tests[i]
		if !testable(tc) {
			continue
		}
		lines := readLines(tc.File)
		if tc.Line < 1 || tc.Line > len(lines) {
			continue
		}
		if importsOf[tc.File] == nil {
			importsOf[tc.File], helpersOf[tc.File] = testFileContext(tc.File, lines, a.tests)
		}
		body := strings.Join(lines[tc.Line-1:min(tc.EndLine, len(lines))], "\n")
		calls := qa.Calls(body)
		// Follow helpers defined in the test file one level deep.
		for _, c := range calls {
			if hb, ok := helpersOf[tc.File][c.Name]; ok && c.Qual == "" {
				calls = append(calls, qa.Calls(hb)...)
			}
		}
		seen := map[string]bool{}
		family := langFamily(workspace.Language(tc.File))
		for _, c := range calls {
			var cands []*analysis.Symbol
			for _, sym := range byName[c.Name] {
				if langFamily(sym.Language) == family {
					cands = append(cands, sym)
				}
			}
			for _, sym := range pickTargets(tc.File, c, cands, importsOf[tc.File]) {
				if !seen[sym.ID] {
					seen[sym.ID] = true
					tc.Targets = append(tc.Targets, sym.ID)
				}
			}
		}
	}

	// Score every function.
	for _, sym := range ix.Symbols {
		if !sym.Kind.IsCallable() || sym.Kind == analysis.KindExternal || sym.Abstract || sym.File == "" || isTest[sym.File] {
			continue
		}
		f := &FuncQA{ID: sym.ID, Name: sym.Qualified, Kind: string(sym.Kind), File: sym.File, Line: sym.Line, EndLine: max(sym.EndLine, sym.Line),
			Complexity: sym.Complexity, Callers: countCalls(ix.Callers(sym.ID)), Exported: sym.Exported, Pct: -1, Heuristic: sym.Heuristic}
		if fc := report.File(sym.File); fc != nil {
			f.Covered, f.Lines = fc.CountsIn(f.Line, f.EndLine)
			if f.Lines > 0 {
				f.Pct = math.Round(1000*float64(f.Covered)/float64(f.Lines)) / 10
			}
		}
		a.funcs = append(a.funcs, f)
		a.byID[f.ID] = f
	}

	// Direct and transitive test reach.
	reach := map[string]int{}
	for i := range a.tests {
		tc := &a.tests[i]
		if len(tc.Targets) == 0 {
			continue
		}
		visited := map[string]bool{}
		frontier := append([]string(nil), tc.Targets...)
		for _, id := range tc.Targets {
			visited[id] = true
			if f := a.byID[id]; f != nil {
				f.Direct++
				if len(f.Tests) < 25 {
					f.Tests = append(f.Tests, tc.ID)
				}
			}
		}
		for depth := 0; depth < 4 && len(frontier) > 0 && len(visited) < 400; depth++ {
			var next []string
			for _, id := range frontier {
				for _, e := range ix.Callees(id) {
					if e.Kind == analysis.EdgeImport || e.Kind == analysis.EdgeImplement || visited[e.To] {
						continue
					}
					if _, ok := a.byID[e.To]; !ok {
						continue
					}
					visited[e.To] = true
					reach[e.To]++
					next = append(next, e.To)
				}
			}
			frontier = next
		}
	}
	for _, f := range a.funcs {
		f.Indirect = reach[f.ID]
		f.Risk, f.Reason = riskOf(f, report != nil)
	}
	sort.Slice(a.funcs, func(i, j int) bool {
		if a.funcs[i].Risk != a.funcs[j].Risk {
			return a.funcs[i].Risk > a.funcs[j].Risk
		}
		return a.funcs[i].ID < a.funcs[j].ID
	})

	st.mu.Lock()
	st.analysis, st.key = a, key
	st.mu.Unlock()
	return a, nil
}

// suiteTool names the JS test runner (vitest or jest) of the innermost
// suite containing file, or "".
func suiteTool(suites []qa.Suite, file string) string {
	best, tool := -1, ""
	for _, su := range suites {
		if su.Tool != "vitest" && su.Tool != "jest" {
			continue
		}
		if su.Dir == "" || strings.HasPrefix(file, su.Dir+"/") {
			if len(su.Dir) > best {
				best, tool = len(su.Dir), su.Tool
			}
		}
	}
	return tool
}

// testFileContext returns the local modules a test file imports (as
// extension-less repo paths or dotted module suffixes) and the bodies of
// helper functions it defines.
func testFileContext(file string, lines []string, tests []qa.TestCase) (map[string]bool, map[string]string) {
	imports := map[string]bool{}
	src := strings.Join(lines, "\n")
	dir := path.Dir(file)
	for _, m := range jsImport.FindAllStringSubmatch(src, -1) {
		p := path.Join(dir, m[1])
		imports[strings.TrimSuffix(p, path.Ext(p))] = true
		imports[p+"/index"] = true
	}
	if strings.HasSuffix(file, ".py") {
		for _, m := range pyImport.FindAllStringSubmatch(src, -1) {
			mod := m[1] + m[2]
			imports["py:"+strings.ReplaceAll(strings.TrimLeft(mod, "."), ".", "/")] = true
		}
	}
	testLines := map[int]bool{}
	for _, tc := range tests {
		if tc.File == file {
			testLines[tc.Line] = true
		}
	}
	helpers := map[string]string{}
	for i, l := range lines {
		m := helperRe.FindStringSubmatch(l)
		if m == nil || testLines[i+1] || strings.HasPrefix(m[1], "Test") || strings.HasPrefix(m[1], "test") {
			continue
		}
		// Helper bodies end at the next line with no indentation.
		end := i + 1
		for ; end < len(lines) && end < i+80; end++ {
			if t := lines[end]; t != "" && t[0] != ' ' && t[0] != '\t' {
				if strings.HasPrefix(t, "}") || strings.HasPrefix(t, ")") {
					end++
				}
				break
			}
		}
		helpers[m[1]] = strings.Join(lines[i:min(end, len(lines))], "\n")
	}
	return imports, helpers
}

// pickTargets chooses which same-named symbols a call in a test refers
// to, preferring the qualifier's package, the test's directory and the
// modules the test imports. Ambiguous calls resolve to nothing.
func pickTargets(testFile string, c qa.Call, cands []*analysis.Symbol, imports map[string]bool) []*analysis.Symbol {
	if len(cands) == 0 || (len(cands) == 1 && c.Qual == "") {
		return cands
	}
	filter := func(keep func(*analysis.Symbol) bool) []*analysis.Symbol {
		var out []*analysis.Symbol
		for _, s := range cands {
			if keep(s) {
				out = append(out, s)
			}
		}
		return out
	}
	if c.Qual != "" {
		if got := filter(func(s *analysis.Symbol) bool {
			return path.Base(path.Dir(s.File)) == c.Qual || path.Base(s.Package) == c.Qual || s.Receiver == c.Qual
		}); len(got) > 0 && len(got) <= 3 {
			return got
		}
	}
	dir := path.Dir(testFile)
	if path.Base(dir) == "__tests__" || path.Base(dir) == "tests" || path.Base(dir) == "test" {
		dir = path.Dir(dir)
	}
	// Maven and Gradle tests mirror the package they test:
	// src/test/java/com/x tests src/main/java/com/x.
	mirror := ""
	if i := strings.Index(dir, "src/test/"); i >= 0 && (i == 0 || dir[i-1] == '/') {
		mirror = dir[:i] + "src/main/" + dir[i+len("src/test/"):]
	}
	if got := filter(func(s *analysis.Symbol) bool { d := path.Dir(s.File); return d == dir || d == mirror }); len(got) > 0 {
		return got
	}
	if got := filter(func(s *analysis.Symbol) bool {
		noExt := strings.TrimSuffix(s.File, path.Ext(s.File))
		if imports[noExt] {
			return true
		}
		for k := range imports {
			if mod, ok := strings.CutPrefix(k, "py:"); ok && (noExt == mod || strings.HasSuffix(noExt, "/"+mod)) {
				return true
			}
		}
		return false
	}); len(got) > 0 {
		return got
	}
	// obj.Method() on a value of unknown type is only trusted with the
	// evidence above; bare calls may pick between two candidates.
	if c.Qual == "" && len(cands) == 2 {
		return cands
	}
	return nil
}

// langFamily groups languages whose code can call each other directly.
func langFamily(lang string) string {
	switch lang {
	case "typescript", "javascript", "typescriptreact", "javascriptreact", "vue", "svelte":
		return "js"
	case "c", "cpp":
		return "c"
	case "java", "kotlin", "scala", "groovy":
		return "jvm"
	}
	return lang
}

// riskOf scores how much an untested function matters: missing coverage,
// weighted by complexity, how widely it is called, and visibility.
func riskOf(f *FuncQA, haveCoverage bool) (float64, string) {
	var gap float64
	switch {
	case f.Pct >= 0:
		gap = 1 - f.Pct/100
	case haveCoverage && f.Direct == 0 && f.Indirect == 0:
		gap = 1
	case f.Direct > 0:
		gap = 0.15
	case f.Indirect > 0:
		gap = 0.45
	default:
		gap = 1
	}
	if gap <= 0 {
		return 0, ""
	}
	weight := float64(max(f.Complexity, 1)) * (1 + math.Log2(1+float64(f.Callers)))
	if f.Exported {
		weight *= 1.25
	}
	if span := f.EndLine - f.Line + 1; span <= 3 && f.Complexity <= 1 {
		weight *= 0.3
	}
	var why []string
	switch {
	case f.Pct >= 0:
		why = append(why, fmt.Sprintf("%.0f%% covered", f.Pct))
	case haveCoverage:
		why = append(why, "not in the coverage report")
	}
	switch {
	case f.Direct == 0 && f.Indirect == 0:
		why = append(why, "no test reaches it")
	case f.Direct == 0:
		why = append(why, fmt.Sprintf("only reached indirectly by %d test(s)", f.Indirect))
	}
	if f.Complexity >= 5 {
		why = append(why, fmt.Sprintf("complexity %d", f.Complexity))
	}
	if f.Callers > 0 {
		why = append(why, fmt.Sprintf("%d caller(s)", f.Callers))
	}
	return math.Round(gap*weight*100) / 100, strings.Join(why, " · ")
}

// ---- views -----------------------------------------------------------------

// QAOverview is everything the QA view shows.
type QAOverview struct {
	Coverage  *CoverageSummary `json:"coverage,omitempty"`
	Tests     TestStats        `json:"tests"`
	Functions FunctionStats    `json:"functions"`
	Files     []FileQA         `json:"files"`
	TestFiles []TestFileQA     `json:"testFiles"`
	Gaps      []*FuncQA        `json:"gaps"`
	Suites    []qa.Suite       `json:"suites"`
	Detected  bool             `json:"detected"`
	Run       *QARun           `json:"run,omitempty"`
	Plans     []*TestPlan      `json:"plans"`
	Targets   map[string]Named `json:"targets"`
}

// Named is a compact symbol reference.
type Named struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// CoverageSummary describes the merged coverage report.
type CoverageSummary struct {
	Covered   int         `json:"covered"`
	Lines     int         `json:"lines"`
	Pct       float64     `json:"pct"`
	Generated time.Time   `json:"generated"`
	Sources   []qa.Source `json:"sources"`
	Stale     int         `json:"stale"` // files changed since the report
}

// TestStats counts discovered tests.
type TestStats struct {
	Files      int            `json:"files"`
	Tests      int            `json:"tests"`
	Behaviors  int            `json:"behaviors"` // tests + subtests + named cases
	Suites     int            `json:"suites"`
	Skipped    int            `json:"skipped"`
	Frameworks map[string]int `json:"frameworks"`
}

// FunctionStats counts functions by test status.
type FunctionStats struct {
	Total     int `json:"total"`
	Covered   int `json:"covered"`   // any line covered
	Full      int `json:"full"`      // 100% covered
	Uncovered int `json:"uncovered"` // 0% covered
	Tested    int `json:"tested"`    // called directly by a test
	Reached   int `json:"reached"`   // tested directly or indirectly
}

// FileQA is per-file coverage and test status.
type FileQA struct {
	Path      string  `json:"path"`
	Covered   int     `json:"covered"`
	Lines     int     `json:"lines"`
	Pct       float64 `json:"pct"` // -1 unknown
	Functions int     `json:"functions"`
	Untested  int     `json:"untested"`
	Stale     bool    `json:"stale,omitempty"`
}

// TestFileQA groups the tests in one file.
type TestFileQA struct {
	Path      string        `json:"path"`
	Framework string        `json:"framework"`
	Tests     []qa.TestCase `json:"tests"`
}

// QAOverview computes the QA view: coverage, tests and their behaviors,
// per-file status, ranked gaps, suites, the last run and agent test plans.
func (s *Service) QAOverview(ctx context.Context, rt *RepoHandle, gapLimit int) (*QAOverview, error) {
	a, err := s.qaAnalyze(ctx, rt)
	if err != nil {
		return nil, err
	}
	ov := &QAOverview{Files: []FileQA{}, TestFiles: []TestFileQA{}, Gaps: []*FuncQA{}, Plans: s.TestPlans(rt), Targets: map[string]Named{}}
	ov.Suites, ov.Detected = s.QASuites(rt)
	ov.Run = s.LastRun(rt)
	if ov.Run != nil {
		ov.Run.Log = nil
	}
	stale := map[string]bool{}
	if r := a.report; r != nil {
		c, t := r.Totals()
		cs := &CoverageSummary{Covered: c, Lines: t, Generated: r.Generated, Sources: r.Sources}
		if t > 0 {
			cs.Pct = math.Round(1000*float64(c)/float64(t)) / 10
		}
		for p := range r.Files {
			if st, err := os.Stat(filepath.Join(rt.cfg.Path, filepath.FromSlash(p))); err == nil && st.ModTime().After(r.Generated.Add(time.Second)) {
				stale[p] = true
				cs.Stale++
			}
		}
		ov.Coverage = cs
	}

	ov.Tests.Frameworks = map[string]int{}
	byFile := map[string]*TestFileQA{}
	var order []string
	for _, tc := range a.tests {
		tf := byFile[tc.File]
		if tf == nil {
			tf = &TestFileQA{Path: tc.File, Framework: tc.Framework}
			byFile[tc.File] = tf
			order = append(order, tc.File)
		}
		tf.Tests = append(tf.Tests, tc)
		switch tc.Kind {
		case qa.KindSuite:
			ov.Tests.Suites++
		case qa.KindCase, qa.KindSubtest, qa.KindTodo:
			ov.Tests.Behaviors++
		default:
			ov.Tests.Tests++
			ov.Tests.Behaviors++
			ov.Tests.Frameworks[tc.Framework]++
		}
		if tc.Skipped {
			ov.Tests.Skipped++
		}
		for _, id := range tc.Targets {
			if f := a.byID[id]; f != nil {
				ov.Targets[id] = Named{Name: f.Name, File: f.File, Line: f.Line}
			}
		}
	}
	sort.Strings(order)
	for _, p := range order {
		ov.TestFiles = append(ov.TestFiles, *byFile[p])
	}
	ov.Tests.Files = len(order)

	files := map[string]*FileQA{}
	fileOf := func(p string) *FileQA {
		if files[p] == nil {
			files[p] = &FileQA{Path: p, Pct: -1, Stale: stale[p]}
			if fc := a.report.File(p); fc != nil {
				files[p].Covered, files[p].Lines = fc.Counts()
				if files[p].Lines > 0 {
					files[p].Pct = math.Round(1000*float64(files[p].Covered)/float64(files[p].Lines)) / 10
				}
			}
		}
		return files[p]
	}
	if a.report != nil {
		for p := range a.report.Files {
			fileOf(p)
		}
	}
	for _, f := range a.funcs {
		ov.Functions.Total++
		fq := fileOf(f.File)
		fq.Functions++
		if f.Pct > 0 {
			ov.Functions.Covered++
		}
		if f.Pct >= 100 {
			ov.Functions.Full++
		}
		if f.Pct == 0 {
			ov.Functions.Uncovered++
		}
		if f.Direct > 0 {
			ov.Functions.Tested++
		}
		if f.Direct > 0 || f.Indirect > 0 {
			ov.Functions.Reached++
		}
		if f.Pct == 0 || (f.Pct < 0 && f.Direct == 0 && f.Indirect == 0) {
			fq.Untested++
		}
	}
	for _, f := range files {
		ov.Files = append(ov.Files, *f)
	}
	sort.Slice(ov.Files, func(i, j int) bool { return ov.Files[i].Path < ov.Files[j].Path })
	ov.Gaps = s.gaps(rt, a, "", gapLimit)
	return ov, nil
}

// gaps returns the riskiest under-tested functions (optionally under a
// path prefix) with uncovered ranges and testability hints.
func (s *Service) gaps(rt *RepoHandle, a *qaAnalysis, prefix string, limit int) []*FuncQA {
	if limit <= 0 {
		limit = 100
	}
	out := []*FuncQA{}
	lines := map[string][]string{}
	for _, f := range a.funcs {
		if len(out) >= limit {
			break
		}
		if f.Risk <= 0 || (prefix != "" && f.File != prefix && !strings.HasPrefix(f.File, strings.TrimSuffix(prefix, "/")+"/")) {
			continue
		}
		g := *f
		if fc := a.report.File(f.File); fc != nil {
			var miss []int
			for ln, h := range fc.Hits {
				if h == 0 && ln >= f.Line && ln <= f.EndLine {
					miss = append(miss, ln)
				}
			}
			sort.Ints(miss)
			g.Uncovered = qa.Ranges(miss)
		}
		if _, ok := lines[f.File]; !ok {
			if fc, err := rt.ws.ReadFile(f.File, defaults); err == nil {
				lines[f.File] = strings.Split(fc.Content, "\n")
			} else {
				lines[f.File] = nil
			}
		}
		g.Hints = qa.Hints(lines[f.File], f.Line, f.EndLine, f.Complexity, 0)
		out = append(out, &g)
	}
	return out
}

// TestGaps returns ranked under-tested functions below a path prefix.
func (s *Service) TestGaps(ctx context.Context, rt *RepoHandle, prefix string, limit int) ([]*FuncQA, bool, error) {
	a, err := s.qaAnalyze(ctx, rt)
	if err != nil {
		return nil, false, err
	}
	if prefix != "" {
		if _, rel, err := rt.ws.Resolve(prefix, defaults); err == nil {
			prefix = rel
		}
	}
	return s.gaps(rt, a, prefix, limit), a.report != nil, nil
}

// FileCoverageView is line coverage for the editor gutter.
type FileCoverageView struct {
	Path      string    `json:"path"`
	Available bool      `json:"available"`
	Covered   []int     `json:"covered"`
	Uncovered []int     `json:"uncovered"`
	Partial   []int     `json:"partial"`
	Pct       float64   `json:"pct"`
	Stale     bool      `json:"stale,omitempty"`
	Generated time.Time `json:"generated,omitzero"`
	Functions []*FuncQA `json:"functions"`
	// Tests are the test cases defined in this file, when it is a test file.
	Tests []qa.TestCase `json:"tests,omitempty"`
}

// FileCoverage returns line coverage and per-function status for a file.
func (s *Service) FileCoverage(ctx context.Context, rt *RepoHandle, file string) (*FileCoverageView, error) {
	abs, rel, err := rt.ws.Resolve(file, defaults)
	if err != nil {
		return nil, err
	}
	a, err := s.qaAnalyze(ctx, rt)
	if err != nil {
		return nil, err
	}
	v := &FileCoverageView{Path: rel, Covered: []int{}, Uncovered: []int{}, Partial: []int{}, Functions: []*FuncQA{}}
	if fc := a.report.File(rel); fc != nil {
		v.Available = true
		v.Covered, v.Uncovered, v.Partial = fc.Lines()
		if c, t := fc.Counts(); t > 0 {
			v.Pct = math.Round(1000*float64(c)/float64(t)) / 10
		}
		v.Generated = a.report.Generated
		if st, err := os.Stat(abs); err == nil && st.ModTime().After(a.report.Generated.Add(time.Second)) {
			v.Stale = true
		}
	}
	for _, f := range a.funcs {
		if f.File == rel {
			v.Functions = append(v.Functions, f)
		}
	}
	sort.Slice(v.Functions, func(i, j int) bool { return v.Functions[i].Line < v.Functions[j].Line })
	for _, tc := range a.tests {
		if tc.File == rel {
			v.Tests = append(v.Tests, tc)
		}
	}
	return v, nil
}

// SymbolTests is the test status of one function.
type SymbolTests struct {
	Function *FuncQA       `json:"function"`
	Direct   []qa.TestCase `json:"direct"`
	// Indirect are tests that reach the function through its callers.
	Indirect []qa.TestCase `json:"indirect"`
}

// TestsFor returns the tests that exercise a function, directly or via
// its callers (up to four levels).
func (s *Service) TestsFor(ctx context.Context, rt *RepoHandle, ref string) (*SymbolTests, error) {
	sym, ix, err := s.ResolveSymbol(ctx, rt, ref)
	if err != nil {
		return nil, err
	}
	a, err := s.qaAnalyze(ctx, rt)
	if err != nil {
		return nil, err
	}
	out := &SymbolTests{Function: a.byID[sym.ID], Direct: []qa.TestCase{}, Indirect: []qa.TestCase{}}
	if out.Function == nil {
		out.Function = &FuncQA{ID: sym.ID, Name: sym.Qualified, File: sym.File, Line: sym.Line, EndLine: sym.EndLine, Pct: -1}
	}
	out.Direct, out.Indirect = testsReaching(ix, a, sym.ID, 60)
	return out, nil
}

// testsReaching returns the tests that call a symbol directly, and up to
// limit that reach it through its callers (up to four levels).
func testsReaching(ix *analysis.Index, a *qaAnalysis, symID string, limit int) (direct, indirect []qa.TestCase) {
	direct, indirect = []qa.TestCase{}, []qa.TestCase{}
	upstream := map[string]bool{}
	frontier := []string{symID}
	for depth := 0; depth < 4 && len(frontier) > 0; depth++ {
		var next []string
		for _, id := range frontier {
			for _, e := range ix.Callers(id) {
				if e.Kind == analysis.EdgeImport || e.Kind == analysis.EdgeImplement || upstream[e.From] || e.From == symID {
					continue
				}
				upstream[e.From] = true
				next = append(next, e.From)
			}
		}
		frontier = next
	}
	for _, tc := range a.tests {
		isDirect, isIndirect := false, false
		for _, id := range tc.Targets {
			if id == symID {
				isDirect = true
			} else if upstream[id] {
				isIndirect = true
			}
		}
		switch {
		case isDirect:
			direct = append(direct, tc)
		case isIndirect && len(indirect) < limit:
			indirect = append(indirect, tc)
		}
	}
	return direct, indirect
}

// ---- agent test plans --------------------------------------------------------

// PlannedBehavior is one behavior an agent recommends testing.
type PlannedBehavior struct {
	Name     string `json:"name" jsonschema:"the behavior, phrased like a test name (e.g. 'rejects an expired token')"`
	Kind     string `json:"kind,omitempty" jsonschema:"happy, edge, error, regression or property"`
	Given    string `json:"given,omitempty" jsonschema:"setup / preconditions"`
	When     string `json:"when,omitempty" jsonschema:"the action under test"`
	Then     string `json:"then,omitempty" jsonschema:"expected outcome"`
	Line     int    `json:"line,omitempty" jsonschema:"line in the function this behavior exercises (e.g. an uncovered branch)"`
	EndLine  int    `json:"end_line,omitempty"`
	Existing string `json:"existing,omitempty" jsonschema:"name of an existing test that already covers this, if any"`
}

// TestRefactor is a code change needed to make a function testable.
type TestRefactor struct {
	Title   string `json:"title" jsonschema:"short title, e.g. 'Inject the clock'"`
	Detail  string `json:"detail,omitempty" jsonschema:"markdown: what to change and why it enables testing"`
	File    string `json:"file,omitempty" jsonschema:"repo-relative file to change"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
}

// TestPlan is an agent's plan for closing a test gap.
type TestPlan struct {
	ID         string            `json:"id"`
	Repo       string            `json:"repo"`
	Title      string            `json:"title"`
	Summary    string            `json:"summary,omitempty"`
	Symbol     string            `json:"symbol,omitempty"`
	SymbolName string            `json:"symbolName,omitempty"`
	File       string            `json:"file,omitempty"`
	Line       int               `json:"line,omitempty"`
	EndLine    int               `json:"endLine,omitempty"`
	TestFile   string            `json:"testFile,omitempty"`
	Behaviors  []PlannedBehavior `json:"behaviors"`
	Refactors  []TestRefactor    `json:"refactors,omitempty"`
	Created    time.Time         `json:"created"`
}

// ShowTestPlan validates and stores an agent's test plan and opens it in
// the QA view.
func (s *Service) ShowTestPlan(ctx context.Context, rt *RepoHandle, p TestPlan) (*TestPlan, error) {
	if strings.TrimSpace(p.Title) == "" {
		return nil, errors.New("title is required")
	}
	if len(p.Behaviors) == 0 && len(p.Refactors) == 0 {
		return nil, errors.New("a test plan needs behaviors or refactors")
	}
	if p.Symbol != "" {
		sym, _, err := s.ResolveSymbol(ctx, rt, p.Symbol)
		if err != nil {
			return nil, err
		}
		p.Symbol, p.SymbolName, p.File, p.Line, p.EndLine = sym.ID, sym.Qualified, sym.File, sym.Line, sym.EndLine
	}
	if p.TestFile != "" {
		_, rel, err := rt.ws.Resolve(p.TestFile, defaults)
		if err != nil {
			return nil, fmt.Errorf("test_file: %w", err)
		}
		p.TestFile = rel
	}
	for i, r := range p.Refactors {
		if r.File == "" {
			continue
		}
		_, rel, err := rt.ws.Resolve(r.File, defaults)
		if err != nil {
			return nil, fmt.Errorf("refactor %q: %w", r.Title, err)
		}
		p.Refactors[i].File = rel
	}
	p.Repo, p.Created = rt.cfg.ID, time.Now()
	st := &rt.qa
	st.mu.Lock()
	s.session.mu.Lock()
	p.ID = s.session.id("plan")
	s.session.mu.Unlock()
	st.plans = append([]*TestPlan{&p}, st.plans...)
	if len(st.plans) > 50 {
		st.plans = st.plans[:50]
	}
	st.mu.Unlock()
	if p.File != "" {
		s.TouchFile(rt, p.File)
	}
	s.emit(EvQA, map[string]any{"repo": rt.cfg.ID, "kind": "plan", "plan": &p})
	s.Show(Focus{View: "qa", Repo: rt.cfg.ID, Title: "QA", Params: map[string]any{"tab": "plans", "plan": p.ID}})
	return &p, nil
}

// TestPlans lists the agent's test plans, newest first.
func (s *Service) TestPlans(rt *RepoHandle) []*TestPlan {
	rt.qa.mu.Lock()
	defer rt.qa.mu.Unlock()
	return append([]*TestPlan{}, rt.qa.plans...)
}

// DismissTestPlan removes a test plan.
func (s *Service) DismissTestPlan(rt *RepoHandle, id string) {
	rt.qa.mu.Lock()
	defer rt.qa.mu.Unlock()
	out := rt.qa.plans[:0]
	for _, p := range rt.qa.plans {
		if p.ID != id {
			out = append(out, p)
		}
	}
	rt.qa.plans = out
}

// defaultCacheDir returns where CodeDiver keeps coverage and run output.
func defaultCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "codediver")
	}
	return filepath.Join(os.TempDir(), "codediver")
}
