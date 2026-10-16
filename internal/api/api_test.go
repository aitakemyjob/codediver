package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/testutil"
)

func setup(t *testing.T) *API {
	t.Helper()
	store, _ := config.Open(filepath.Join(t.TempDir(), "c.json"))
	svc := core.New(store, "test")
	svc.CacheDir = t.TempDir()
	if _, err := svc.AddRepo(testutil.Repo(t), "shop"); err != nil {
		t.Fatal(err)
	}
	return New(svc)
}

func call(t *testing.T, a *API, method string, args any) string {
	t.Helper()
	raw, _ := json.Marshal(args)
	out, err := a.Invoke(context.Background(), method, raw)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return string(out)
}

// Every method must at least accept empty args against a real repo without
// panicking; methods needing arguments may return errors.
func TestAllMethodsSmoke(t *testing.T) {
	a := setup(t)
	// qa.run would really execute the detected `go test` suite.
	skip := map[string]bool{"repos.remove": true, "proposals.dismiss": true, "qa.run": true}
	for _, m := range a.Methods() {
		if skip[m] {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s panicked: %v", m, r)
				}
			}()
			a.Invoke(context.Background(), m, nil)
		}()
	}
}

func TestCoreFlows(t *testing.T) {
	a := setup(t)
	if out := call(t, a, "fs.tree", nil); !strings.Contains(out, `"api.go"`) || strings.Contains(out, ".env") {
		t.Errorf("tree = %s", out)
	}
	if out := call(t, a, "fs.search", map[string]any{"query": "Save"}); !strings.Contains(out, `"matches"`) {
		t.Errorf("search = %s", out)
	}
	if out := call(t, a, "graph.flow", map[string]any{"symbol": "Handler.CreateOrder"}); !strings.Contains(out, `"kind":"cond"`) {
		t.Errorf("flow = %s", out)
	}
	if out := call(t, a, "graph.call", map[string]any{"symbol": "main", "direction": "callees", "depth": 2, "includeRefs": true}); !strings.Contains(out, "Handler.CreateOrder") {
		t.Errorf("callgraph should follow method values passed to HandleFunc, got %s", out)
	}
	if out := call(t, a, "sym.detail", map[string]any{"symbol": "Store", "code": true}); !strings.Contains(out, "implementations") {
		t.Errorf("detail = %s", out)
	}
	call(t, a, "settings.update", config.Settings{Theme: "nord", MCPEnabled: true})
	if out := call(t, a, "app.state", nil); !strings.Contains(out, `"theme":"nord"`) {
		t.Errorf("settings not applied: %s", out)
	}
	if _, err := a.Invoke(context.Background(), "repos.pick", nil); err == nil {
		t.Error("picker should be unavailable without a desktop shell")
	}
}

// The frontend iterates these directly, so they must never be JSON null.
func TestEmptyListsAreArrays(t *testing.T) {
	a := setup(t)
	// An untracked non-code file has no symbols; it must still be [] not null.
	root := a.core.Config.Get().Repos[0].Path
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, m := range []struct {
		method string
		args   any
	}{
		{"activity.list", nil},
		{"proposals.list", nil},
		{"graph.trace", map[string]any{"symbol": "main"}},
		{"graph.call", map[string]any{"symbol": "validate"}},
		{"git.diff", map[string]any{"spec": map[string]any{}, "impact": true}},
		{"git.status", nil},
		{"git.log", nil},
		{"fs.search", map[string]any{"query": "zzz-no-match"}},
		{"sym.search", map[string]any{"q": "zzz-no-match"}},
		{"sym.file", map[string]any{"file": "README.md"}},
		{"session.all", nil},
	} {
		out := call(t, a, m.method, m.args)
		bad := out == "null"
		for _, field := range []string{"nodes", "edges", "symbols", "files", "matches", "entries", "changes", "walkthroughs", "diagrams", "explanations", "annotations"} {
			bad = bad || strings.Contains(out, `"`+field+`":null`)
		}
		if bad {
			t.Errorf("%s returned nulls: %s", m.method, out)
		}
	}
}

func TestQAMethods(t *testing.T) {
	a := setup(t)
	if out := call(t, a, "qa.overview", nil); !strings.Contains(out, `"suites":[{"id":"go:"`) || !strings.Contains(out, `"gaps":[`) {
		t.Errorf("overview = %.400s", out)
	}
	call(t, a, "qa.suites.save", map[string]any{"suites": []map[string]any{{"id": "x", "name": "X", "command": "true", "enabled": true}}})
	if out := call(t, a, "app.state", nil); !strings.Contains(out, `"command":"true"`) {
		t.Errorf("suites not saved: %.300s", out)
	}
	call(t, a, "qa.suites.reset", nil)
	if out := call(t, a, "qa.file", map[string]any{"path": "api/api.go"}); !strings.Contains(out, `"available":false`) {
		t.Errorf("file = %s", out)
	}
	if out := call(t, a, "qa.testsFor", map[string]any{"symbol": "validate"}); !strings.Contains(out, `"direct":[]`) {
		t.Errorf("testsFor = %s", out)
	}
}
