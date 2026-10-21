package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/proposals"
	"github.com/aitakemyjob/codediver/internal/qa"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

// ---- tests and blast radius of a change ----------------------------------------
//
// For an agent's proposal or a git change (working tree, staged, branches,
// a commit) this answers what a reviewer asks: are the changed lines run
// by any test, which tests prove the change, and what calls the changed
// code. Line coverage comes from the merged report. Which test ran a line
// is measured where per-test runs exist and estimated from the static
// test-to-code mapping otherwise, and the report says which.

// ChangeRef names a change: a proposal, or a git diff.
type ChangeRef struct {
	Proposal string         `json:"proposal,omitempty"`
	Spec     *gitx.DiffSpec `json:"spec,omitempty"`
}

// ChangeLine is the coverage of one changed line.
type ChangeLine struct {
	Line int `json:"line"`
	// State is covered, partial, uncovered or unknown (no coverage data
	// for the file).
	State string `json:"state"`
	// Tests are tests measured to run the line.
	Tests []string `json:"tests,omitempty"`
}

// ChangeFile is the coverage of one file's changed lines.
type ChangeFile struct {
	Path string `json:"path"`
	// Side says which side of the diff Lines are numbered on: original
	// (the code as it is, for proposals) or modified.
	Side      string       `json:"side"`
	Lines     []ChangeLine `json:"lines"`
	Covered   int          `json:"covered"`
	Uncovered int          `json:"uncovered"`
	Unknown   int          `json:"unknown"`
	// Added counts new lines no test can have run yet (proposals).
	Added    int  `json:"added,omitempty"`
	New      bool `json:"new,omitempty"`
	Deleted  bool `json:"deleted,omitempty"`
	Stale    bool `json:"stale,omitempty"`
	Measured bool `json:"measured,omitempty"`
}

// ChangeTest is a test that exercises the change.
type ChangeTest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Behavior  string `json:"behavior"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Framework string `json:"framework"`
	// Lines counts changed lines the test was measured to run.
	Lines int `json:"lines"`
	// Functions are changed functions the test reaches.
	Functions []string `json:"functions,omitempty"`
	// Direct is set when the test calls a changed function itself.
	Direct bool `json:"direct"`
	// Measured is set when a per-test run recorded what the test ran.
	Measured bool      `json:"measured"`
	Ran      time.Time `json:"ran,omitzero"`
	Failed   bool      `json:"failed,omitempty"`
	// Runnable tests can be run with per-test coverage.
	Runnable bool `json:"runnable"`
}

// ChangeCoverage is the test coverage of a change.
type ChangeCoverage struct {
	// Available is set when a coverage report exists.
	Available bool `json:"available"`
	// Measured is set when per-test runs attribute some changed lines to
	// tests; otherwise tests are an estimate from the static mapping.
	Measured  bool      `json:"measured"`
	Stale     bool      `json:"stale,omitempty"`
	Generated time.Time `json:"generated,omitzero"`
	// Approximate is set for diffs between commits: line numbers are
	// matched against the working tree's coverage.
	Approximate bool         `json:"approximate,omitempty"`
	Files       []ChangeFile `json:"files"`
	Lines       int          `json:"lines"`
	Covered     int          `json:"covered"`
	Uncovered   int          `json:"uncovered"`
	Unknown     int          `json:"unknown"`
	Added       int          `json:"added"`
	Tests       []ChangeTest `json:"tests"`
	MoreTests   int          `json:"moreTests,omitempty"`
	// Untested are changed functions no test reaches.
	Untested []ImpactNode `json:"untested"`
}

// ChangeReport is the coverage and blast radius of a change.
type ChangeReport struct {
	Title    string         `json:"title,omitempty"`
	Coverage ChangeCoverage `json:"coverage"`
	Impact   *Impact        `json:"impact"`
}

const maxChangeTests = 100

// changedFile is one file of a change, in coverage coordinates.
type changedFile struct {
	path    string
	side    string
	lines   []int // existing lines the change touches
	added   int
	created bool
	deleted bool
}

// ChangeReport computes the tests and blast radius of a proposal or diff.
func (s *Service) ChangeReport(ctx context.Context, rt *RepoHandle, ref ChangeRef) (*ChangeReport, error) {
	ix, err := s.Index(ctx, rt)
	if err != nil {
		return nil, err
	}
	var files []changedFile
	var changed []string
	out := &ChangeReport{}
	approximate := false
	switch {
	case ref.Proposal != "":
		p, ok := s.Proposals.Get(ref.Proposal)
		if !ok || p.Repo != rt.cfg.ID {
			return nil, fmt.Errorf("unknown proposal %q", ref.Proposal)
		}
		out.Title = p.Title
		for _, c := range p.Changes {
			files = append(files, proposalFile(c))
			changed = append(changed, c.Symbols...)
		}
	case ref.Spec != nil:
		diffs, err := rt.git.Diff(ctx, *ref.Spec)
		if err != nil {
			return nil, err
		}
		for _, d := range filterDiffs(rt, diffs) {
			f := changedFile{path: d.Path(), side: "modified", deleted: d.Status == gitx.StatusDeleted, created: d.Status == gitx.StatusAdded || d.Status == gitx.StatusUntracked}
			if !f.deleted {
				for _, h := range d.Hunks {
					for _, l := range h.Lines {
						if l.Kind == gitx.LineAdd {
							f.lines = append(f.lines, l.NewNo)
						}
					}
				}
				for _, cs := range changedSymbols(ix, f.path, d.ChangedNewLines()) {
					changed = append(changed, cs.ID)
				}
			}
			files = append(files, f)
		}
		approximate = ref.Spec.Commit != "" || ref.Spec.Head != ""
	default:
		return nil, errors.New("name a proposal or a diff")
	}

	a, _ := s.qaAnalyze(ctx, rt)
	out.Impact = buildImpact(ix, ix.Reach(changed, impactDepth, func(sym *analysis.Symbol) bool { return qa.IsTestFile(sym.File) }), a, s.impactCoverage(rt, a))
	out.Coverage = s.changeCoverage(rt, ix, a, files, out.Impact)
	out.Coverage.Approximate = approximate
	return out, nil
}

// proposalFile describes a proposed change in the coordinates of the code
// as it is: the lines it replaces or removes, plus the lines it adds.
func proposalFile(c *proposals.FileChange) changedFile {
	f := changedFile{path: c.Path, side: "original", created: c.Action == proposals.ActionCreate, deleted: c.Action == proposals.ActionDelete}
	if f.created {
		f.side, f.added = "modified", c.Additions
		return f
	}
	for _, h := range c.Hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case gitx.LineDelete:
				f.lines = append(f.lines, l.OldNo)
			case gitx.LineAdd:
				f.added++
			}
		}
	}
	return f
}

// changeCoverage measures changed lines against coverage and collects
// the tests that exercise them.
func (s *Service) changeCoverage(rt *RepoHandle, ix *analysis.Index, a *qaAnalysis, files []changedFile, im *Impact) ChangeCoverage {
	cc := ChangeCoverage{Files: []ChangeFile{}, Tests: []ChangeTest{}, Untested: []ImpactNode{}}
	var report *qa.Report
	if a != nil {
		report = a.report
	}
	if report != nil {
		cc.Available, cc.Generated = true, report.Generated
	}
	perTest := s.perTest(rt)
	tests := map[string]*ChangeTest{}
	byID := map[string]*qa.TestCase{}
	if a != nil {
		for i := range a.tests {
			byID[a.tests[i].ID] = &a.tests[i]
		}
	}
	addTest := func(id string) *ChangeTest {
		if t := tests[id]; t != nil {
			return t
		}
		tc := byID[id]
		if tc == nil {
			return nil
		}
		t := &ChangeTest{ID: tc.ID, Name: tc.Name, Behavior: tc.Behavior, File: tc.File, Line: tc.Line, Framework: tc.Framework,
			Runnable: qa.PerTestFrameworks[tc.Framework] && (tc.Kind == qa.KindTest || tc.Kind == qa.KindSubtest)}
		tests[id] = t
		return t
	}

	for _, f := range files {
		// Coverage is about code under test: skip docs, data and the tests.
		if !workspace.IsCode(workspace.Language(f.path)) || qa.IsTestFile(f.path) {
			continue
		}
		cf := ChangeFile{Path: f.path, Side: f.side, Lines: []ChangeLine{}, Added: f.added, New: f.created, Deleted: f.deleted}
		var mtime time.Time
		if abs, _, err := rt.ws.Resolve(f.path, defaults); err == nil {
			if st, err := os.Stat(abs); err == nil {
				mtime = st.ModTime()
			}
		}
		var fc *qa.FileCoverage
		if report != nil {
			fc = report.File(f.path)
			if fc != nil && mtime.After(report.Generated.Add(time.Second)) {
				cf.Stale, cc.Stale = true, true
			}
		}
		// Per-test results newer than the file still describe it.
		var fresh []*testCoverage
		for _, tc := range perTest {
			if tc.Ran.After(mtime) && len(tc.Lines[f.path]) > 0 {
				fresh = append(fresh, tc)
			}
		}
		sort.Slice(fresh, func(i, j int) bool { return fresh[i].ID < fresh[j].ID })
		cf.Measured = len(fresh) > 0
		for _, ln := range uniqueSorted(f.lines) {
			cl := ChangeLine{Line: ln}
			switch h, ok := hitsAt(fc, ln); {
			case fc == nil:
				cl.State = "unknown"
				cf.Unknown++
			case !ok:
				continue // not executable: blank, comment, declaration
			case h == 0:
				cl.State = "uncovered"
				cf.Uncovered++
			case fc.Partial[ln]:
				cl.State = "partial"
				cf.Covered++
			default:
				cl.State = "covered"
				cf.Covered++
			}
			for _, tc := range fresh {
				if tc.covers(f.path, ln) {
					cl.Tests = append(cl.Tests, tc.ID)
					if t := addTest(tc.ID); t != nil {
						t.Lines++
					}
				}
			}
			cf.Lines = append(cf.Lines, cl)
		}
		cc.Measured = cc.Measured || cf.Measured
		cc.Covered += cf.Covered
		cc.Uncovered += cf.Uncovered
		cc.Unknown += cf.Unknown
		cc.Added += cf.Added
		cc.Files = append(cc.Files, cf)
	}
	cc.Lines = cc.Covered + cc.Uncovered + cc.Unknown

	// The static estimate: tests that call or reach the changed functions.
	if a != nil {
		for _, n := range im.Changed {
			direct, indirect := testsReaching(ix, a, n.ID, 40)
			for _, tc := range direct {
				if t := addTest(tc.ID); t != nil {
					t.Direct = true
					t.Functions = appendUnique(t.Functions, n.Name)
				}
			}
			for _, tc := range indirect {
				if t := addTest(tc.ID); t != nil {
					t.Functions = appendUnique(t.Functions, n.Name)
				}
			}
			if !n.Tested {
				cc.Untested = append(cc.Untested, n)
			}
		}
	}
	for id, t := range tests {
		if r := perTest[id]; r != nil {
			t.Ran, t.Failed = r.Ran, !r.Passed
			// A fresh run that touched none of the changed lines is a
			// measurement too.
			t.Measured = t.Lines > 0 || ranAfterAll(r, rt, files)
		}
		cc.Tests = append(cc.Tests, *t)
	}
	sort.Slice(cc.Tests, func(i, j int) bool {
		a, b := cc.Tests[i], cc.Tests[j]
		if a.Lines != b.Lines {
			return a.Lines > b.Lines
		}
		if a.Direct != b.Direct {
			return a.Direct
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	if len(cc.Tests) > maxChangeTests {
		cc.MoreTests = len(cc.Tests) - maxChangeTests
		cc.Tests = cc.Tests[:maxChangeTests]
	}
	return cc
}

// hitsAt returns a line's execution count and whether it is instrumented.
func hitsAt(fc *qa.FileCoverage, line int) (int, bool) {
	if fc == nil {
		return 0, false
	}
	h, ok := fc.Hits[line]
	return h, ok
}

// ranAfterAll reports whether a per-test run is newer than every changed
// file, so its results describe the code under review.
func ranAfterAll(r *testCoverage, rt *RepoHandle, files []changedFile) bool {
	for _, f := range files {
		abs, _, err := rt.ws.Resolve(f.path, defaults)
		if err != nil {
			continue
		}
		if st, err := os.Stat(abs); err == nil && st.ModTime().After(r.Ran) {
			return false
		}
	}
	return true
}

// uniqueSorted returns xs sorted without duplicates.
func uniqueSorted(xs []int) []int {
	out := append([]int(nil), xs...)
	sort.Ints(out)
	n := 0
	for i, x := range out {
		if i == 0 || x != out[n-1] {
			out[n] = x
			n++
		}
	}
	return out[:n]
}

// appendUnique appends v unless xs already holds it.
func appendUnique(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}
