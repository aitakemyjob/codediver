package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/proposals"
	"github.com/aitakemyjob/codediver/internal/qa"
	"github.com/aitakemyjob/codediver/internal/testutil"
)

// testID returns the ID of the discovered test called name.
func testID(t *testing.T, s *Service, rt *RepoHandle, name string) string {
	t.Helper()
	a, err := s.qaAnalyze(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range a.tests {
		if tc.Name == name {
			return tc.ID
		}
	}
	t.Fatalf("no test %s", name)
	return ""
}

func TestChangeReportForProposal(t *testing.T) {
	s, rt, _ := qaRepo(t)
	ctx := context.Background()
	p, err := s.Propose(ctx, rt, "Accept qty 1", "", []proposals.ChangeInput{{
		Path:  "api/api.go",
		Edits: []proposals.Edit{{Old: "if o.Qty <= 0 {", New: "if o.Qty < 1 {"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.ChangeReport(ctx, rt, ChangeRef{Proposal: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	cc := r.Coverage
	// validate never ran in the profile, so the replaced line is uncovered.
	if !cc.Available || cc.Measured || cc.Lines != 1 || cc.Uncovered != 1 || cc.Added != 1 {
		t.Fatalf("coverage = %+v", cc)
	}
	line := lineOf(t, testutil.Files["api/api.go"], "if o.Qty <= 0")
	if f := cc.Files[0]; f.Side != "original" || len(f.Lines) != 1 || f.Lines[0].Line != line || f.Lines[0].State != "uncovered" {
		t.Errorf("file = %+v", f)
	}
	// TestCreateOrder reaches validate through CreateOrder: an estimate.
	var found bool
	for _, tc := range cc.Tests {
		if tc.Name == "TestCreateOrder" {
			found = true
			if tc.Measured || tc.Direct || !tc.Runnable || len(tc.Functions) != 1 || tc.Functions[0] != "validate" {
				t.Errorf("test = %+v", tc)
			}
		}
	}
	if !found {
		t.Errorf("tests = %+v", cc.Tests)
	}

	// Direct callers come first, with their path to the change. Two tests
	// call CreateOrder, but neither runs its call to validate: coverage
	// decides over the static mapping, so it needs review.
	im := r.Impact
	if len(im.Changed) != 1 || im.Changed[0].Name != "validate" || len(im.Rings) == 0 {
		t.Fatalf("impact = %+v", im)
	}
	direct := im.Rings[0]
	if n := direct.Nodes[0]; direct.Depth != 1 || direct.Total != 1 || n.Name != "Handler.CreateOrder" || n.Via != "validate" ||
		n.Status != StatusCaller || !n.Measured || n.Direct == 0 || n.Tested || !n.Review || direct.Statuses[StatusCaller] != 1 {
		t.Errorf("direct = %+v", direct)
	}
	if c := im.Changed[0]; c.Status != StatusNotRun || c.Tested {
		t.Errorf("changed = %+v", c)
	}
	if im.Summary.Callers[0] != 1 || im.Summary.Untested != 1 || !im.Tests || !im.Coverage {
		t.Errorf("summary = %+v", im.Summary)
	}

	// A per-test run newer than the file turns the estimate into a measurement.
	id := testID(t, s, rt, "TestCreateOrder")
	s.storePerTest(rt, []*testCoverage{{ID: id, Name: "TestCreateOrder", Ran: time.Now().Add(time.Minute), Passed: true, Lines: map[string][]int{"api/api.go": {line - 1, line}}}})
	r, err = s.ChangeReport(ctx, rt, ChangeRef{Proposal: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	cc = r.Coverage
	if !cc.Measured || !cc.Files[0].Measured || len(cc.Files[0].Lines[0].Tests) != 1 || cc.Files[0].Lines[0].Tests[0] != id {
		t.Fatalf("measured coverage = %+v", cc)
	}
	if top := cc.Tests[0]; top.ID != id || top.Lines != 1 || !top.Measured {
		t.Errorf("top test = %+v", top)
	}
	// Clearing coverage forgets per-test results too.
	s.ClearCoverage(rt)
	if len(s.perTest(rt)) != 0 {
		t.Error("per-test coverage should be cleared")
	}
}

func TestChangeReportForDiff(t *testing.T) {
	needGit(t)
	s, rt, dir := qaRepo(t)
	ctx := context.Background()
	old := time.Now().Add(-time.Hour)
	testutil.Must(t, os.Chtimes(filepath.Join(dir, "coverage.out"), old, old))
	s.ClearCoverage(rt) // reload with the older report
	rt.qa.mu.Lock()
	rt.qa.loaded = false
	rt.qa.mu.Unlock()
	testutil.Must(t, os.WriteFile(filepath.Join(dir, "store", "store.go"), []byte(strings.Replace(testutil.Files["store/store.go"], "m.orders[o.ID] = o", "m.orders[o.ID] = o // saved", 1)), 0o644))
	r, err := s.ChangeReport(ctx, rt, ChangeRef{Spec: &gitx.DiffSpec{}})
	if err != nil {
		t.Fatal(err)
	}
	var f *ChangeFile
	for i := range r.Coverage.Files {
		if r.Coverage.Files[i].Path == "store/store.go" {
			f = &r.Coverage.Files[i]
		}
	}
	// The edited file is newer than coverage.out, so its numbers are stale.
	if f == nil || f.Side != "modified" || f.Covered != 1 || !f.Stale || r.Coverage.Approximate {
		t.Fatalf("store.go = %+v (%+v)", f, r.Coverage)
	}
	// Tests, coverage.out and other untracked non-code files aren't measured.
	if len(r.Coverage.Files) != 1 || r.Coverage.Lines != 1 {
		t.Errorf("files = %+v", r.Coverage.Files)
	}
	if len(r.Impact.Changed) != 1 || !strings.Contains(r.Impact.Changed[0].Name, "Save") {
		t.Errorf("changed = %+v", r.Impact.Changed)
	}
	if _, err := s.ChangeReport(ctx, rt, ChangeRef{}); err == nil {
		t.Error("an empty ref should be refused")
	}
	if _, err := s.ChangeReport(ctx, rt, ChangeRef{Proposal: "nope"}); err == nil {
		t.Error("an unknown proposal should be refused")
	}
}

func TestImpactCapsAndOrdersRings(t *testing.T) {
	s, rt, _ := qaRepo(t)
	ctx := context.Background()
	ix, err := s.Index(ctx, rt)
	if err != nil {
		t.Fatal(err)
	}
	var save string
	for id, sym := range ix.Symbols {
		if sym.Qualified == "Memory.Save" || strings.HasSuffix(id, "Memory).Save") {
			save = id
		}
	}
	old := ringCap
	ringCap = [impactDepth + 1]int{0, 0, 0, 0}
	defer func() { ringCap = old }()
	im := s.impactOf(ctx, rt, ix, []string{save})
	if len(im.Rings) == 0 || len(im.Rings[0].Nodes) != 0 || im.Rings[0].More != im.Rings[0].Total || im.Rings[0].Total == 0 {
		t.Fatalf("capped ring = %+v", im.Rings)
	}
	if len(im.Rings[0].Packages) == 0 || len(im.PackageEdges) == 0 || im.PackageDepth[im.Changed[0].Package] != 0 {
		t.Errorf("packages = %+v %+v %+v", im.Rings[0].Packages, im.PackageEdges, im.PackageDepth)
	}
	// Test functions are not part of the blast radius.
	for _, r := range im.Rings {
		for _, n := range r.Nodes {
			if qa.IsTestFile(n.File) {
				t.Errorf("test in radius: %+v", n)
			}
		}
	}
}

func TestRunTestsMeasuresGoTests(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	s, rt, _ := qaRepo(t)
	ctx := context.Background()
	id := testID(t, s, rt, "TestMemorySave")
	if _, err := s.RunTests(ctx, rt, []string{"web/app.test.ts:4"}); err == nil {
		t.Error("vitest tests can't be run per test")
	}
	run, err := s.RunTests(ctx, rt, []string{id})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Suites) != 1 || run.Suites[0].Name != "TestMemorySave" {
		t.Fatalf("run = %+v", run)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for s.LastRun(rt).State == "running" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if last := s.LastRun(rt); last.State != "done" {
		t.Fatalf("run = %+v\n%s", last, strings.Join(last.Log, "\n"))
	}
	tc := s.perTest(rt)[id]
	save := lineOf(t, testutil.Files["store/store.go"], "m.orders[o.ID] = o")
	if tc == nil || !tc.Passed || !tc.covers("store/store.go", save) || len(tc.Lines["api/api.go"]) != 0 {
		t.Fatalf("per-test coverage = %+v", tc)
	}
}

func TestImpactStatus(t *testing.T) {
	gen := time.Now()
	fc := &qa.FileCoverage{Path: "api.go", Hits: map[int]int{10: 1, 11: 1, 12: 0, 14: 0, 20: 0, 21: 0}, Partial: map[int]bool{}}
	report := &qa.Report{Files: map[string]*qa.FileCoverage{"api.go": fc}, Generated: gen}
	mtime := gen.Add(-time.Hour)
	cv := &impactCoverage{report: report, perTest: map[string]*testCoverage{}, mtime: func(string) time.Time { return mtime }}
	handler := &analysis.Symbol{File: "api.go", Line: 10, EndLine: 15}
	cases := []struct {
		name  string
		sym   *analysis.Symbol
		sites []int
		f     *FuncQA
		want  string
	}{
		// The call at 11 ran; at 12 it didn't, though the handler did.
		{"call ran", handler, []int{11}, nil, StatusPath},
		{"call not run", handler, []int{12}, &FuncQA{Direct: 2, Indirect: 3}, StatusCaller},
		// 13 continues the statement starting at 12.
		{"continuation", handler, []int{13}, nil, StatusCaller},
		{"changed code ran", handler, nil, nil, StatusPath},
		{"nothing ran", &analysis.Symbol{File: "api.go", Line: 20, EndLine: 22}, []int{21}, &FuncQA{Direct: 1}, StatusNotRun},
		// Not measured: a direct test is a likely estimate, indirect reach is not.
		{"likely", &analysis.Symbol{File: "web.ts", Line: 1, EndLine: 5}, []int{2}, &FuncQA{Direct: 1}, StatusLikely},
		{"unknown", &analysis.Symbol{File: "web.ts", Line: 1, EndLine: 5}, []int{2}, &FuncQA{Indirect: 4}, StatusUnknown},
	}
	for _, c := range cases {
		if got := cv.status(c.sym, c.sites, c.f); got.Status != c.want {
			t.Errorf("%s: status = %+v, want %s", c.name, got, c.want)
		}
	}
	if (*impactCoverage)(nil).status(handler, []int{11}, &FuncQA{Direct: 1}).Status != StatusLikely {
		t.Error("without coverage the static mapping estimates")
	}

	// A per-test run newer than the file names the tests running the call.
	cv.perTest["t1"] = &testCoverage{ID: "t1", Name: "TestCreate", Ran: gen, Lines: map[string][]int{"api.go": {10, 12}}}
	cv.perTest["t0"] = &testCoverage{ID: "t0", Name: "TestOld", Ran: mtime.Add(-time.Minute), Lines: map[string][]int{"api.go": {12}}}
	if n := cv.status(handler, []int{12}, nil); n.Status != StatusPath || !n.Measured || len(n.Tests) != 1 || n.Tests[0] != "TestCreate" {
		t.Errorf("per-test = %+v", n)
	}

	// Coverage older than the file is still used, but marked stale.
	mtime = gen.Add(time.Hour)
	if n := cv.status(handler, []int{11}, nil); n.Status != StatusPath || !n.Stale {
		t.Errorf("stale = %+v", n)
	}
}
