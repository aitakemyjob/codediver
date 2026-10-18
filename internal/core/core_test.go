package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/proposals"
	"github.com/aitakemyjob/codediver/internal/testutil"
)

func newService(t *testing.T) (*Service, *RepoHandle, string) {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := New(store, "test")
	dir := testutil.Repo(t)
	r, err := s.AddRepo(dir, "shop")
	if err != nil {
		t.Fatal(err)
	}
	rt, err := s.Repo(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, rt, dir
}

func needGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func TestOverview(t *testing.T) {
	s, rt, _ := newService(t)
	ov, err := s.Overview(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Stats.Functions < 6 || ov.Stats.Languages["go"] != 3 || ov.Stats.Languages["typescript"] != 2 {
		t.Errorf("stats = %+v", ov.Stats)
	}
	if len(ov.EntryPoints) == 0 || ov.EntryPoints[0].Name != "main" {
		t.Errorf("entry points = %+v", ov.EntryPoints)
	}
	if !strings.Contains(ov.Readme, "tiny example") {
		t.Errorf("readme = %q", ov.Readme)
	}
}

func TestIndexCacheInvalidatesOnChange(t *testing.T) {
	s, rt, dir := newService(t)
	ctx := context.Background()
	ix1, _ := s.Index(ctx, rt)
	ix2, _ := s.Index(ctx, rt)
	if ix1 != ix2 {
		t.Fatal("index should be cached")
	}
	testutil.Must(t, os.WriteFile(filepath.Join(dir, "api", "extra.go"), []byte("package api\n\nfunc Extra() {}\n"), 0o644))
	rt.idx.checkedAt = rt.idx.checkedAt.Add(-1e10) // skip the 2s debounce
	rt.ws.Invalidate()
	ix3, _ := s.Index(ctx, rt)
	if ix3 == ix1 || len(ix3.Find("Extra")) != 1 {
		t.Fatal("index should rebuild when files change")
	}
}

func TestFileVersionsForRootCommit(t *testing.T) {
	needGit(t)
	s, rt, _ := newService(t)
	ctx := context.Background()
	// The fixture repo has a single (root) commit.
	ov, err := s.Diff(ctx, rt, gitx.DiffSpec{Commit: "HEAD"}, false)
	if err != nil || len(ov.Files) == 0 {
		t.Fatalf("root commit diff = %+v %v", ov, err)
	}
	for _, f := range ov.Files {
		if f.Status != gitx.StatusAdded {
			t.Errorf("%s status = %s, want added", f.Path, f.Status)
		}
	}
	fv, err := s.FileVersions(ctx, rt, "store/store.go", "", gitx.DiffSpec{Commit: "HEAD"})
	if err != nil || fv.Original != "" || fv.Modified != testutil.Files["store/store.go"] {
		t.Fatalf("versions = %q / %d bytes, %v", fv.Original, len(fv.Modified), err)
	}
}

func TestDiffMapsSymbolsAndImpact(t *testing.T) {
	needGit(t)
	s, rt, dir := newService(t)
	p := filepath.Join(dir, "api", "api.go")
	data, _ := os.ReadFile(p)
	testutil.Must(t, os.WriteFile(p, []byte(strings.Replace(string(data), "o.Qty <= 0", "o.Qty < 1", 1)), 0o644))
	ov, err := s.Diff(context.Background(), rt, gitx.DiffSpec{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Files) != 1 || len(ov.Files[0].Symbols) != 1 || ov.Files[0].Symbols[0].Name != "validate" {
		t.Fatalf("diff = %+v", ov.Files)
	}
	callers := map[string]bool{}
	for _, n := range ov.Impact.Nodes {
		callers[n.Label] = true
	}
	if !callers["Handler.CreateOrder"] {
		t.Errorf("impact should include the caller: %+v", ov.Impact.Nodes)
	}
	// Files without symbols (untracked, non-code) must serialize symbols as [].
	testutil.Must(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello\n"), 0o644))
	ov2, err := s.Diff(context.Background(), rt, gitx.DiffSpec{}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ov2.Files {
		if f.Symbols == nil {
			t.Errorf("%s: Symbols is nil (serializes as null)", f.Path)
		}
	}
	fv, err := s.FileVersions(context.Background(), rt, "api/api.go", "", gitx.DiffSpec{})
	if err != nil || !strings.Contains(fv.Original, "o.Qty <= 0") || !strings.Contains(fv.Modified, "o.Qty < 1") {
		t.Errorf("versions = %+v %v", fv, err)
	}
	hm, err := s.Heatmap(context.Background(), rt, "changes", "")
	if err != nil || hm.Values["api/api.go"] != 2 || hm.Symbols["api/api.go"][0].Name != "validate" {
		t.Errorf("changes heatmap = %+v %v", hm.Values, err)
	}
}

func TestHeatmaps(t *testing.T) {
	s, rt, _ := newService(t)
	ctx := context.Background()
	for metric := range HeatMetrics {
		hm, err := s.Heatmap(ctx, rt, metric, "")
		if err != nil {
			t.Fatalf("%s: %v", metric, err)
		}
		if hm.Tree == nil || hm.Tree.Files == 0 {
			t.Errorf("%s: empty tree", metric)
		}
	}
	if hm, _ := s.Heatmap(ctx, rt, "complexity", ""); hm.Values["api/api.go"] < 5 {
		t.Errorf("complexity = %v", hm.Values)
	}
	if hm, _ := s.Heatmap(ctx, rt, "bogus", ""); hm.Metric != "churn" {
		t.Errorf("unknown metric should fall back to churn, got %s", hm.Metric)
	}
	s.TouchFile(rt, "api/api.go")
	if hm, _ := s.Heatmap(ctx, rt, "agent", ""); hm.Values["api/api.go"] != 1 {
		t.Errorf("agent heatmap = %v", hm.Values)
	}
}

func TestProposalImpactAndHeat(t *testing.T) {
	s, rt, _ := newService(t)
	ctx := context.Background()
	p, err := s.Propose(ctx, rt, "t", "s", []proposals.ChangeInput{{
		Path:  "store/store.go",
		Edits: []proposals.Edit{{Old: "m.orders[o.ID] = o", New: "m.orders[o.ID] = o // x"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes[0].Symbols) != 1 || !strings.Contains(p.Changes[0].Symbols[0], "Memory).Save") {
		t.Fatalf("symbols = %v", p.Changes[0].Symbols)
	}
	if len(p.Impact) < 3 {
		t.Errorf("impact = %v", p.Impact)
	}
	hm, _ := s.Heatmap(ctx, rt, "proposals", "")
	if hm.Values["store/store.go"] != 2 {
		t.Errorf("proposal heat = %v", hm.Values)
	}
	if _, err := s.Propose(ctx, rt, "dup", "", []proposals.ChangeInput{{Path: "a.go", NewContent: ptr("x")}, {Path: "./a.go", NewContent: ptr("y")}}); err == nil {
		t.Error("duplicate paths should be rejected")
	}
}

func ptr(s string) *string { return &s }

func TestResolveSymbolAmbiguity(t *testing.T) {
	s, rt, _ := newService(t)
	// "Save" matches the interface method and the concrete method; the
	// concrete one wins.
	sym, _, err := s.ResolveSymbol(context.Background(), rt, "Save")
	if err != nil || sym.Abstract {
		t.Fatalf("Save = %+v %v", sym, err)
	}
	if _, _, err := s.ResolveSymbol(context.Background(), rt, "doesNotExist"); err == nil {
		t.Error("expected not-found error")
	}
}

func TestAgentRepoErrors(t *testing.T) {
	s, _, _ := newService(t)
	if _, err := s.AgentRepo("nope"); err == nil || !strings.Contains(err.Error(), "shop") {
		t.Errorf("err should list available repos: %v", err)
	}
}
