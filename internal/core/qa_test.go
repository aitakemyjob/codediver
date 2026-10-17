package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/qa"
	"github.com/aitakemyjob/codediver/internal/testutil"
)

// lineOf returns the 1-based line of the first line containing sub.
func lineOf(t *testing.T, content, sub string) int {
	t.Helper()
	for i, l := range strings.Split(content, "\n") {
		if strings.Contains(l, sub) {
			return i + 1
		}
	}
	t.Fatalf("%q not found", sub)
	return 0
}

// qaRepo is the shop fixture plus Go and TypeScript tests and a Go
// coverage profile in which validate never ran.
func qaRepo(t *testing.T) (*Service, *RepoHandle, string) {
	t.Helper()
	s, rt, dir := newService(t)
	s.CacheDir = t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		testutil.Must(t, os.MkdirAll(filepath.Dir(p), 0o755))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("store/store_test.go", "package store\n\nimport \"testing\"\n\nfunc TestMemorySave(t *testing.T) {\n\tm := NewMemory()\n\tif err := m.Save(Order{ID: \"a\", Qty: 1}); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n")
	write("api/api_test.go", "package api\n\nimport (\n\t\"testing\"\n\n\t\"example.com/shop/store\"\n)\n\nfunc TestCreateOrder(t *testing.T) {\n\th := New(store.NewMemory())\n\tt.Run(\"rejects bad json\", func(t *testing.T) {\n\t\th.CreateOrder(nil, nil)\n\t})\n}\n")
	write("web/app.test.ts", "import { it, expect } from 'vitest';\nimport { submit } from './app';\n\nit('posts the quantity', async () => {\n  expect(await submit(2)).toBeTruthy();\n  m.Save(1);\n});\n")
	api := testutil.Files["api/api.go"]
	createLine, validateLine := lineOf(t, api, "func (h *Handler) CreateOrder"), lineOf(t, api, "func validate")
	store := testutil.Files["store/store.go"]
	saveLine := lineOf(t, store, "func (m *Memory) Save")
	profile := fmt.Sprintf("mode: set\nexample.com/shop/api/api.go:%d.80,%d.2 3 1\nexample.com/shop/api/api.go:%d.36,%d.2 2 0\nexample.com/shop/store/store.go:%d.38,%d.2 2 1\nexample.com/elsewhere/x.go:1.1,2.2 1 1\n",
		createLine, createLine+3, validateLine, validateLine+5, saveLine, saveLine+3)
	write("coverage.out", profile)
	s.Reindex(rt)
	return s, rt, dir
}

func TestQAOverview(t *testing.T) {
	s, rt, _ := qaRepo(t)
	ctx := context.Background()
	ov, err := s.QAOverview(ctx, rt, 50)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Tests.Tests != 3 || ov.Tests.Files != 3 || ov.Tests.Behaviors != 4 || ov.Tests.Frameworks["go test"] != 2 || ov.Tests.Frameworks["vitest"] != 1 {
		t.Errorf("tests = %+v", ov.Tests)
	}
	// coverage.out was imported automatically; the external file is skipped.
	if ov.Coverage == nil || ov.Coverage.Lines != 14 || ov.Coverage.Covered != 8 || len(ov.Coverage.Sources) != 1 || ov.Coverage.Sources[0].Skipped != 1 {
		t.Fatalf("coverage = %+v", ov.Coverage)
	}
	if len(ov.Gaps) == 0 || !strings.HasSuffix(ov.Gaps[0].Name, "validate") || ov.Gaps[0].Pct != 0 || ov.Gaps[0].Uncovered == "" {
		t.Fatalf("top gap = %+v", ov.Gaps[0])
	}
	if !ov.Detected || len(ov.Suites) == 0 || ov.Suites[0].Tool != "go" {
		t.Errorf("suites = %+v", ov.Suites)
	}
	var apiFile *FileQA
	for i := range ov.Files {
		if ov.Files[i].Path == "api/api.go" {
			apiFile = &ov.Files[i]
		}
	}
	if apiFile == nil || apiFile.Pct <= 0 || apiFile.Pct >= 100 || apiFile.Untested < 1 {
		t.Errorf("api file = %+v", apiFile)
	}
	if ov.Functions.Tested < 3 || ov.Functions.Uncovered < 1 {
		t.Errorf("functions = %+v", ov.Functions)
	}
	if _, ok := ov.Targets[ov.TestFiles[0].Tests[0].Targets[0]]; !ok {
		t.Error("targets should describe every referenced symbol")
	}
}

func TestQATestsFor(t *testing.T) {
	s, rt, _ := qaRepo(t)
	ctx := context.Background()
	st, err := s.TestsFor(ctx, rt, "NewMemory")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tc := range st.Direct {
		names = append(names, tc.Name)
	}
	if strings.Join(names, ",") != "TestCreateOrder,TestMemorySave" && strings.Join(names, ",") != "TestMemorySave,TestCreateOrder" {
		t.Errorf("direct tests of NewMemory = %v", names)
	}
	v, err := s.TestsFor(ctx, rt, "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Direct) != 0 || len(v.Indirect) == 0 || v.Function.Pct != 0 {
		t.Errorf("validate = direct %d indirect %d fn %+v", len(v.Direct), len(v.Indirect), v.Function)
	}
	// The TypeScript test's m.Save() must not map onto the Go method.
	save, _ := s.TestsFor(ctx, rt, "Memory.Save")
	for _, tc := range save.Direct {
		if tc.Framework != "go test" {
			t.Errorf("cross-language match: %+v", tc)
		}
	}
	if len(save.Direct) != 1 {
		t.Errorf("Memory.Save direct tests = %+v", save.Direct)
	}
	sub, _ := s.TestsFor(ctx, rt, "post")
	if len(sub.Indirect) != 1 || sub.Indirect[0].Framework != "vitest" {
		t.Errorf("post should be reached through submit: %+v", sub)
	}
}

func TestQAFileCoverageAndHeatmap(t *testing.T) {
	s, rt, dir := qaRepo(t)
	ctx := context.Background()
	fc, err := s.FileCoverage(ctx, rt, "api/api.go")
	if err != nil {
		t.Fatal(err)
	}
	if !fc.Available || len(fc.Covered) != 4 || len(fc.Uncovered) != 6 || len(fc.Functions) < 3 {
		t.Fatalf("file coverage = %+v", fc)
	}
	tf, _ := s.FileCoverage(ctx, rt, "api/api_test.go")
	if len(tf.Tests) != 2 {
		t.Errorf("tests in test file = %+v", tf.Tests)
	}
	if _, err := s.FileCoverage(ctx, rt, ".env"); err == nil {
		t.Error("hidden files must stay hidden")
	}
	hm, err := s.Heatmap(ctx, rt, "coverage", "")
	if err != nil || hm.Values["api/api.go"] != 6 || len(hm.Symbols["api/api.go"]) == 0 {
		t.Fatalf("heatmap = %+v %v", hm, err)
	}
	// Editing a file after the report marks it stale.
	future := time.Now().Add(time.Hour)
	testutil.Must(t, os.Chtimes(filepath.Join(dir, "api", "api.go"), future, future))
	if fc, _ := s.FileCoverage(ctx, rt, "api/api.go"); !fc.Stale {
		t.Error("expected stale coverage")
	}
}

func TestQARunCoverageAndCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("suite command uses sh")
	}
	s, rt, _ := qaRepo(t)
	ctx := context.Background()
	suites := []qa.Suite{{ID: "custom:web", Name: "Web", Dir: "web", Enabled: true,
		Command: `printf 'SF:app.ts\nDA:3,1\nDA:4,0\nend_of_record\n' > {out}/lcov.info && echo done`, Report: "{out}/lcov.info"}}
	if err := s.SaveQASuites(rt, suites); err != nil {
		t.Fatal(err)
	}
	rt, _ = s.Repo(rt.ID())
	if got, detected := s.QASuites(rt); detected || len(got) != 1 {
		t.Fatalf("saved suites = %+v %v", got, detected)
	}
	run, err := s.RunCoverage(rt, nil)
	if err != nil || run.State != "running" {
		t.Fatalf("run = %+v %v", run, err)
	}
	if _, err := s.RunCoverage(rt, nil); err == nil {
		// The command is quick; a second start only fails while running.
		if last := s.LastRun(rt); last.State == "running" {
			t.Error("concurrent runs should be refused")
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for s.LastRun(rt).State == "running" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	last := s.LastRun(rt)
	if last.State != "done" || last.Suites[0].State != "passed" || last.Suites[0].Files != 1 || !strings.Contains(strings.Join(last.Log, "\n"), "done") {
		t.Fatalf("last run = %+v", last)
	}
	fc, _ := s.FileCoverage(ctx, rt, "web/app.ts")
	if !fc.Available || len(fc.Covered) != 1 || len(fc.Uncovered) != 1 {
		t.Fatalf("web coverage = %+v", fc)
	}

	// A new service with the same cache directory restores both parts.
	store, _ := config.Open(filepath.Join(t.TempDir(), "c.json"))
	s2 := New(store, "test")
	s2.CacheDir = s.CacheDir
	r2, _ := s2.AddRepo(rt.cfg.Path, "shop")
	rt2, _ := s2.Repo(r2.ID)
	if rep, _ := s2.coverage(rt2); rep == nil || len(rep.Files) != 3 || len(rep.Sources) != 2 {
		t.Fatalf("restored = %+v", rep)
	}
	s2.ClearCoverage(rt2)
	if rep, _ := s2.coverage(rt2); rep != nil {
		t.Fatal("coverage should be cleared")
	}
}

func TestQARunReportsMissingReport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("suite command uses sh")
	}
	s, rt, _ := qaRepo(t)
	testutil.Must(t, s.SaveQASuites(rt, []qa.Suite{{ID: "x", Name: "Broken", Enabled: true, Command: "exit 1", Report: "{out}/none.info"}}))
	rt, _ = s.Repo(rt.ID())
	if _, err := s.RunCoverage(rt, nil); err != nil {
		t.Fatal(err)
	}
	for s.LastRun(rt).State == "running" {
		time.Sleep(20 * time.Millisecond)
	}
	last := s.LastRun(rt)
	if last.State != "failed" || last.Suites[0].ExitCode != 1 || !strings.Contains(last.Suites[0].Error, "no coverage report") {
		t.Fatalf("run = %+v", last)
	}
}

func TestQASuiteValidation(t *testing.T) {
	s, rt, _ := qaRepo(t)
	if err := s.SaveQASuites(rt, []qa.Suite{{Name: "empty"}}); err == nil {
		t.Error("expected error for empty command")
	}
	if err := s.SaveQASuites(rt, []qa.Suite{{Name: "out", Command: "x", Dir: "../.."}}); err == nil {
		t.Error("expected error for a dir outside the repo")
	}
	if _, err := s.RunCoverage(rt, []string{"nope"}); err == nil {
		t.Error("expected error when no suite matches")
	}
	if _, err := s.ImportCoverage(rt, "README.md"); err == nil {
		t.Error("expected error importing a non-report")
	}
}

func TestQATestPlans(t *testing.T) {
	s, rt, _ := qaRepo(t)
	ctx := context.Background()
	ch, cancel := s.Bus.Subscribe()
	defer cancel()
	p, err := s.ShowTestPlan(ctx, rt, TestPlan{Title: "Cover validate", Symbol: "validate", TestFile: "api/api_test.go",
		Behaviors: []PlannedBehavior{{Name: "rejects zero quantity", Kind: "error"}},
		Refactors: []TestRefactor{{Title: "none needed", File: "api/api.go"}}})
	if err != nil {
		t.Fatal(err)
	}
	if p.File != "api/api.go" || p.SymbolName == "" || p.ID == "" {
		t.Errorf("plan = %+v", p)
	}
	var kinds []string
	for len(kinds) < 2 {
		select {
		case ev := <-ch:
			if ev.Type == EvQA || ev.Type == EvFocus {
				kinds = append(kinds, ev.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("events = %v", kinds)
		}
	}
	if kinds[0] != EvQA || kinds[1] != EvFocus {
		t.Errorf("events = %v", kinds)
	}
	if _, err := s.ShowTestPlan(ctx, rt, TestPlan{Title: "x"}); err == nil {
		t.Error("expected error for an empty plan")
	}
	if _, err := s.ShowTestPlan(ctx, rt, TestPlan{Title: "x", TestFile: ".env", Behaviors: []PlannedBehavior{{Name: "a"}}}); err == nil {
		t.Error("expected error for a hidden test file")
	}
	if len(s.TestPlans(rt)) != 1 {
		t.Fatal("plan not stored")
	}
	s.DismissTestPlan(rt, p.ID)
	if len(s.TestPlans(rt)) != 0 {
		t.Fatal("plan not dismissed")
	}
}
