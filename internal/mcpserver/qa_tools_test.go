package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/qa"
	"github.com/aitakemyjob/codediver/internal/testutil"
)

// withTests adds Go tests to the harness repo and optionally a coverage
// profile in which validate never ran.
func withTests(t *testing.T, h *harness, coverage bool) {
	t.Helper()
	h.core.CacheDir = t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(h.dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("api/api_test.go", "package api\n\nimport (\n\t\"testing\"\n\n\t\"example.com/shop/store\"\n)\n\nfunc TestCreateOrderRejectsBadJSON(t *testing.T) {\n\th := New(store.NewMemory())\n\th.CreateOrder(nil, nil)\n}\n")
	if coverage {
		api := strings.Split(testutil.Files["api/api.go"], "\n")
		var create, validate int
		for i, l := range api {
			if strings.HasPrefix(l, "func (h *Handler) CreateOrder") {
				create = i + 1
			}
			if strings.HasPrefix(l, "func validate") {
				validate = i + 1
			}
		}
		write("coverage.out", fmt.Sprintf("mode: set\nexample.com/shop/api/api.go:%d.80,%d.2 3 1\nexample.com/shop/api/api.go:%d.36,%d.2 2 0\n",
			create, create+3, validate, validate+5))
	}
	rt, err := h.core.Repo(h.repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.core.Reindex(rt)
}

func TestQAToolsWithoutCoverage(t *testing.T) {
	h := newHarness(t)
	withTests(t, h, false)
	h.mustCall("get_test_overview", nil, "No coverage report yet", "1 tests", "go test:1", "riskiest under-tested")
	h.mustCall("get_coverage", nil, "Run coverage")
	h.mustCall("find_test_gaps", nil, "no coverage report", "validate", "no test reaches it")
	h.mustCall("get_tests_for", map[string]any{"symbol": "Handler.CreateOrder"}, "direct tests (1)", "Create order rejects bad JSON")
	h.mustCall("get_tests_for", map[string]any{"path": "api/api_test.go"}, "1 test cases", "→ calls", "Handler.CreateOrder")
	if out, isErr := h.call("get_tests_for", map[string]any{}); !isErr {
		t.Errorf("expected error without symbol or path: %s", out)
	}
}

func TestQAToolsWithCoverage(t *testing.T) {
	h := newHarness(t)
	withTests(t, h, true)
	h.mustCall("get_test_overview", map[string]any{"show": true}, "line coverage 40.0% (4/10 lines)", "least covered directories", "- api 40%")
	if ev := h.waitFor("focus"); !strings.Contains(jsonOf(t, ev.Data), `"view":"qa"`) {
		t.Errorf("focus = %+v", ev.Data)
	}
	h.mustCall("get_coverage", map[string]any{"path": "api/api.go", "show": true}, "api/api.go: 40.0% covered", "uncovered lines:", "validate", "0.0%")
	h.mustCall("get_coverage", map[string]any{"path": "api"}, "- api/api.go 40.0% (4/10 lines)")
	h.mustCall("find_test_gaps", map[string]any{"limit": 3}, "1. validate", "coverage 0.0%", "uncovered lines:")
	h.mustCall("get_tests_for", map[string]any{"symbol": "validate"}, "indirect tests via callers (1)", "uncovered lines")

	out := h.mustCall("show_test_plan", map[string]any{
		"title":     "Cover validate",
		"symbol":    "validate",
		"test_file": "api/api_test.go",
		"behaviors": []map[string]any{{"name": "rejects zero quantity", "kind": "error", "given": "an order with qty 0", "then": "ErrInvalid"}},
		"refactors": []map[string]any{{"title": "none needed"}},
	}, "1 behaviors", "propose_changes")
	if !strings.Contains(out, "plan") {
		t.Error(out)
	}
	if out, isErr := h.call("show_test_plan", map[string]any{"title": "x", "behaviors": []map[string]any{{"name": "a"}}, "test_file": ".env"}); !isErr {
		t.Errorf("hidden test file should be refused: %s", out)
	}
}

func TestQAToolsRespectAgentSwitch(t *testing.T) {
	h := newHarness(t)
	withTests(t, h, true)
	r := h.repo
	r.MCPEnabled = false
	testutil.Must(t, h.core.UpdateRepo(r))
	for _, tool := range []string{"get_test_overview", "get_coverage", "find_test_gaps"} {
		if out, isErr := h.call(tool, nil); !isErr {
			t.Errorf("%s should be refused when the repo is hidden from agents: %s", tool, out)
		}
	}
}

// jsonOf encodes v for substring assertions.
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTestsForChange(t *testing.T) {
	h := newHarness(t)
	withTests(t, h, true)
	if !strings.Contains(h.mustCall("git_status", nil), "branch main") {
		t.Skip("git unavailable")
	}
	p := filepath.Join(h.dir, "api", "api.go")
	testutil.Must(t, os.WriteFile(p, []byte(strings.Replace(testutil.Files["api/api.go"], "if o.Qty <= 0 {", "if o.Qty < 1 {", 1)), 0o644))
	h.mustCall("get_tests_for_change", map[string]any{"show": true}, "Changed lines: 0 of 1 covered", "1 not run by any test",
		"not covered: api/api.go:", "Tests that call or reach the changed functions (1), estimated", "TestCreateOrderRejectsBadJSON", "estimated", "Direct callers", "Handler.CreateOrder", "→ validate")
	if ev := h.waitFor("focus"); !strings.Contains(jsonOf(t, ev.Data), `"pane":"tests"`) {
		t.Errorf("focus = %+v", ev.Data)
	}
	out := h.mustCall("propose_changes", map[string]any{
		"title":   "Cap qty",
		"summary": "cap",
		"changes": []map[string]any{{"path": "api/api.go", "edits": []map[string]any{{"old": "if o.Qty < 1 {", "new": "if o.Qty < 1 || o.Qty > 99 {"}}}},
	}, "Changed lines: 0 of 1 covered", "Direct callers")
	id := strings.Fields(strings.TrimPrefix(out, "proposal "))[0]
	h.mustCall("get_tests_for_change", map[string]any{"proposal": id}, "Changed lines: 0 of 1 covered", "plus 1 new line.")
	h.mustCall("get_proposal", map[string]any{"id": id}, "pending", "Changed lines")
}

func TestChangeCoverageText(t *testing.T) {
	cc := &core.ChangeCoverage{Available: true, Measured: true, Lines: 2, Covered: 2, Added: 1, Tests: []core.ChangeTest{
		{Name: "TestRuns", File: "a_test.go", Line: 3, Measured: true, Lines: 2, Direct: true},
		{Name: "TestOne", File: "a_test.go", Line: 9, Measured: true, Lines: 1, Functions: []string{"F"}},
		{Name: "it formats", File: "a.test.ts", Line: 1, Functions: []string{"fmt"}},
		{Name: "TestElsewhere", File: "b_test.go", Line: 5, Measured: true, Functions: []string{"F"}},
	}}
	var sb strings.Builder
	writeChangeCoverage(&sb, cc, true)
	out := sb.String()
	for _, want := range []string{
		"Changed lines: 2 of 2 covered by tests; plus 1 new line.",
		"Tests that run the changed lines (2), measured:\n- TestRuns  a_test.go:3 (runs 2 changed lines; calls the changed code)\n- TestOne  a_test.go:9 (runs 1 changed line; reaches F)",
		"Tests that call or reach the changed functions (1), estimated: the developer can click Run these tests to measure:\n- it formats",
		"Tests that reach the changed functions but don't run these lines (1), measured:\n- TestElsewhere",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if plural(1, "caller") != "1 caller" || plural(3, "caller") != "3 callers" {
		t.Error("plural")
	}
}

func TestGapLineHint(t *testing.T) {
	var sb strings.Builder
	gapLine(&sb, 0, &core.FuncQA{Name: "send", File: "a.ts", Line: 1, EndLine: 9, Pct: 0, Hints: []qa.Hint{{Kind: qa.HintNetwork, Line: 7, Match: "fetch(", Problem: "talks to the network directly", Suggestion: "inject the client"}}})
	if want := "testability: talks to the network directly (line 7: fetch) → inject the client"; !strings.Contains(sb.String(), want) {
		t.Errorf("gap line = %q", sb.String())
	}
}
