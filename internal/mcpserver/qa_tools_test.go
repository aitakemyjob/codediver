package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	h.core.UpdateRepo(r)
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
