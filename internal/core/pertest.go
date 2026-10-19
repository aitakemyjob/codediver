package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aitakemyjob/codediver/internal/qa"
)

// ---- per-test coverage -----------------------------------------------------------
//
// Running the tests that reach a change one by one tells which test runs
// which line, so a change's review can say "TestCreateOrder runs 12 of
// these lines" instead of estimating from the call graph. Only the
// developer starts these runs (the "Run these tests" button); agents read
// the results.

// maxPerTestRun bounds how many tests one per-test run executes.
const maxPerTestRun = 40

// testCoverage is the lines one test ran, from a per-test run.
type testCoverage struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Ran    time.Time        `json:"ran"`
	Passed bool             `json:"passed"`
	Lines  map[string][]int `json:"lines"` // covered lines per repo-relative file
}

// covers reports whether the test ran line of file.
func (tc *testCoverage) covers(file string, line int) bool {
	ls := tc.Lines[file]
	i := sort.SearchInts(ls, line)
	return i < len(ls) && ls[i] == line
}

// perTestPath is where a repository's per-test coverage is saved.
func (s *Service) perTestPath(rt *RepoHandle) string {
	return filepath.Join(s.CacheDir, "coverage", rt.cfg.ID+".tests.json")
}

// perTest returns the repository's per-test coverage, loading the saved
// copy on first use. Callers must not modify it.
func (s *Service) perTest(rt *RepoHandle) map[string]*testCoverage {
	st := &rt.qa
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.perTest == nil {
		st.perTest = map[string]*testCoverage{}
		if data, err := os.ReadFile(s.perTestPath(rt)); err == nil {
			_ = json.Unmarshal(data, &st.perTest)
		}
	}
	return st.perTest
}

// storePerTest records results and saves them.
func (s *Service) storePerTest(rt *RepoHandle, results []*testCoverage) {
	s.perTest(rt)
	st := &rt.qa
	st.mu.Lock()
	defer st.mu.Unlock()
	// Copy on write: readers hold the previous map without the lock.
	next := make(map[string]*testCoverage, len(st.perTest)+len(results))
	for k, v := range st.perTest {
		next[k] = v
	}
	for _, r := range results {
		next[r.ID] = r
	}
	st.perTest = next
	if data, err := json.Marshal(next); err == nil {
		p := s.perTestPath(rt)
		if os.MkdirAll(filepath.Dir(p), 0o755) == nil {
			_ = os.WriteFile(p, data, 0o644)
		}
	}
}

// clearPerTest forgets per-test coverage (with the rest of the coverage).
func (s *Service) clearPerTest(rt *RepoHandle) {
	rt.qa.mu.Lock()
	rt.qa.perTest = map[string]*testCoverage{}
	rt.qa.mu.Unlock()
	_ = os.Remove(s.perTestPath(rt))
}

// RunTests runs tests by ID with per-test coverage: Go tests one at a
// time, pytest tests together with per-test contexts. Tests of other
// frameworks are skipped. Only the desktop UI calls this.
func (s *Service) RunTests(ctx context.Context, rt *RepoHandle, ids []string) (*QARun, error) {
	a, err := s.qaAnalyze(ctx, rt)
	if err != nil {
		return nil, err
	}
	byID := map[string]*qa.TestCase{}
	for i := range a.tests {
		byID[a.tests[i].ID] = &a.tests[i]
	}
	all, _ := s.QASuites(rt)
	var jobs []qa.Suite
	goTests := map[string]string{}            // job ID → test ID
	pyTests := map[string]map[string]string{} // job ID → node id → test ID
	pyJobs := map[string]int{}                // suite ID → index in jobs
	seen := map[string]bool{}
	skipped := 0
	for _, id := range ids {
		tc := byID[id]
		// A subtest runs as part of its top-level test.
		for tc != nil && tc.Kind != qa.KindTest && tc.Parent != "" {
			tc = byID[tc.Parent]
		}
		if tc == nil || seen[tc.ID] || !qa.PerTestFrameworks[tc.Framework] || tc.Kind != qa.KindTest {
			skipped++
			continue
		}
		seen[tc.ID] = true
		tool := map[string]string{"go test": "go", "pytest": "pytest"}[tc.Framework]
		su, ok := innermostSuite(all, tool, tc.File)
		if !ok {
			skipped++
			continue
		}
		switch tool {
		case "go":
			if len(goTests) >= maxPerTestRun {
				skipped++
				continue
			}
			job := qa.Suite{ID: "test:" + tc.ID, Name: tc.Name, Dir: su.Dir, Tool: "go",
				Command: qa.GoTestCommand(tc.Name, path.Dir(qa.RelTo(su.Dir, tc.File))), Report: "{out}/cover.out"}
			goTests[job.ID] = tc.ID
			jobs = append(jobs, job)
		case "pytest":
			i, ok := pyJobs[su.ID]
			if !ok {
				i = len(jobs)
				pyJobs[su.ID] = i
				jobs = append(jobs, qa.Suite{ID: "tests:" + su.ID, Name: "pytest · " + strings.TrimPrefix(su.Name, "pytest · "), Dir: su.Dir, Tool: "pytest", Report: "{out}/contexts.json"})
				pyTests[jobs[i].ID] = map[string]string{}
			}
			if len(pyTests[jobs[i].ID]) >= maxPerTestRun {
				skipped++
				continue
			}
			pyTests[jobs[i].ID][qa.PytestNodeID(qa.RelTo(su.Dir, tc.File), tc.Suite, tc.Name)] = tc.ID
		}
	}
	for i := range jobs {
		if nodes := pyTests[jobs[i].ID]; nodes != nil {
			list := make([]string, 0, len(nodes))
			for n := range nodes {
				list = append(list, n)
			}
			sort.Strings(list)
			jobs[i].Command = qa.PytestCommand(list)
		}
	}
	if len(jobs) == 0 {
		if skipped > 0 {
			return nil, errors.New("per-test coverage runs Go and pytest tests; run the full coverage in the QA view for the others")
		}
		return nil, errors.New("no tests selected")
	}
	collect := func(su qa.Suite, report string, res *qa.Resolver, result qa.RunResult) error {
		now := time.Now()
		if testID, ok := goTests[su.ID]; ok {
			rep, _, err := parseReport(report, su.Name, res)
			if err != nil {
				return err
			}
			tc := &testCoverage{ID: testID, Name: su.Name, Ran: now, Passed: result.ExitCode == 0, Lines: map[string][]int{}}
			for p, fc := range rep.Files {
				if cov, _, _ := fc.Lines(); len(cov) > 0 {
					tc.Lines[p] = cov
				}
			}
			s.storePerTest(rt, []*testCoverage{tc})
			return nil
		}
		nodes := pyTests[su.ID]
		if nodes == nil {
			return fmt.Errorf("unknown job %s", su.ID)
		}
		data, err := os.ReadFile(report)
		if err != nil {
			return err
		}
		ctxs, err := qa.ParseContexts(data)
		if err != nil {
			return err
		}
		failed := map[string]bool{}
		for _, f := range result.Failures {
			failed[qa.NodeIDOf(f)] = true
		}
		base := []string{su.Dir}
		var out []*testCoverage
		for node, testID := range nodes {
			tc := &testCoverage{ID: testID, Name: node, Ran: now, Passed: !failed[node], Lines: map[string][]int{}}
			for raw, lines := range ctxs[node] {
				if p, ok := res.Resolve(raw, base); ok {
					tc.Lines[p] = uniqueSorted(append(tc.Lines[p], lines...))
				}
			}
			out = append(out, tc)
		}
		s.storePerTest(rt, out)
		return nil
	}
	return s.startRun(rt, jobs, collect)
}

// innermostSuite returns the deepest suite of a tool whose directory
// contains file.
func innermostSuite(suites []qa.Suite, tool, file string) (qa.Suite, bool) {
	best, found := qa.Suite{}, false
	for _, su := range suites {
		if su.Tool != tool || (su.Dir != "" && !strings.HasPrefix(file, su.Dir+"/")) {
			continue
		}
		if !found || len(su.Dir) > len(best.Dir) {
			best, found = su, true
		}
	}
	return best, found
}
