package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/events"
	"github.com/aitakemyjob/codediver/internal/testutil"
)

type harness struct {
	t    *testing.T
	core *core.Service
	cs   *mcp.ClientSession
	repo config.Repo
	dir  string
	evs  <-chan events.Event
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := core.New(store, "test")
	dir := testutil.Repo(t)
	repo, err := svc.AddRepo(dir, "shop")
	if err != nil {
		t.Fatal(err)
	}
	evs, cancel := svc.Bus.Subscribe()
	t.Cleanup(cancel)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	srv := New(svc)
	if _, err := srv.MCP().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return &harness{t: t, core: svc, cs: cs, repo: repo, dir: dir, evs: evs}
}

func (h *harness) call(name string, args map[string]any) (string, bool) {
	h.t.Helper()
	res, err := h.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		h.t.Fatalf("%s: protocol error: %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

func (h *harness) mustCall(name string, args map[string]any, wants ...string) string {
	h.t.Helper()
	out, isErr := h.call(name, args)
	if isErr {
		h.t.Fatalf("%s returned tool error: %s", name, out)
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			h.t.Errorf("%s output missing %q:\n%s", name, w, out)
		}
	}
	return out
}

// waitFor drains events until one of the given type arrives.
func (h *harness) waitFor(typ string) events.Event {
	h.t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-h.evs:
			if ev.Type == typ {
				return ev
			}
		case <-timeout:
			h.t.Fatalf("no %s event", typ)
		}
	}
}

func TestToolsAreListed(t *testing.T) {
	h := newHarness(t)
	res, err := h.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
	}
	for _, want := range []string{"list_repos", "get_developer_context", "repo_overview", "read_file", "get_call_graph", "trace_to",
		"get_function_flow", "git_diff", "get_heatmap", "start_walkthrough", "explain_function", "show_diagram", "propose_changes", "get_proposal"} {
		if !names[want] {
			t.Errorf("missing tool %s", want)
		}
	}
}

func TestExploreTools(t *testing.T) {
	h := newHarness(t)
	h.mustCall("list_repos", nil, "shop (active)")
	h.mustCall("repo_overview", nil, "functions", "main [func]")
	h.mustCall("list_files", map[string]any{"path": "api"}, "api.go")
	h.mustCall("read_file", map[string]any{"path": "store/store.go", "start_line": 23, "end_line": 24}, "23│", "func (m *Memory) Save")
	h.mustCall("search_code", map[string]any{"query": "ErrInvalid"}, "api/api.go:", "store/store.go:5:")
	h.mustCall("search_symbols", map[string]any{"query": "Save"}, "Memory.Save")
	h.mustCall("get_file_summary", map[string]any{"path": "store/store.go"},
		"imports: errors", "struct Memory", "interface Store", "Memory.Save(o Order) → error", "var ErrInvalid = errors.New(\"invalid order\")")
	h.mustCall("get_symbol", map[string]any{"symbol": "Handler.CreateOrder", "include_code": true},
		"callees", "validate", "source:", "w http.ResponseWriter")
	h.mustCall("get_call_graph", map[string]any{"symbol": "Handler.CreateOrder", "depth": 3},
		"Store.Save", "Memory.Save", "dispatch")
	// Type-checked Go needs no caveat; name-matched TypeScript says how far to trust it.
	if out := h.mustCall("trace_to", map[string]any{"symbol": "Memory.Save"}, "Handler.CreateOrder", "main", "entry"); strings.Contains(out, "Precision:") {
		t.Errorf("Go trace has a precision note:\n%s", out)
	}
	if out := h.mustCall("get_dependencies", map[string]any{}, "api → store"); strings.Contains(out, "Precision:") {
		t.Errorf("Go dependencies have a precision note:\n%s", out)
	}
	// Lexical import edges get a note about imports, not about calls.
	deps := &analysis.Graph{Kind: "deps", Edges: []analysis.GraphEdge{{From: "dir:a", To: "dir:b", Heuristic: true}}}
	if n := precisionNote(deps); !strings.Contains(n, "edges come from import statements") || strings.Contains(n, "same-named function") {
		t.Errorf("deps note = %q", n)
	}
	h.mustCall("trace_to", map[string]any{"symbol": "post"}, "submit", "~", "Precision: ~ edges are lexical name matches",
		"{entry} badge means no caller was found", "Known gaps: get_language_support language=typescript.")
	h.mustCall("get_function_flow", map[string]any{"symbol": "Handler.CreateOrder"}, "inputs:  w http.ResponseWriter", "[cond", "[return")
	h.mustCall("get_dependencies", map[string]any{}, "example.com/shop/api", "→")
	h.mustCall("get_types", map[string]any{"scope": "example.com/shop/store"}, "Memory", "implements")
	h.mustCall("get_heatmap", map[string]any{"metric": "complexity"}, "api/api.go")

	// Sandbox: secrets and escapes are refused.
	if out, isErr := h.call("read_file", map[string]any{"path": ".env"}); !isErr || !strings.Contains(out, "excluded") {
		t.Errorf(".env should be excluded: %s", out)
	}
	if out, isErr := h.call("read_file", map[string]any{"path": "../../etc/passwd"}); !isErr || !strings.Contains(out, "outside") {
		t.Errorf("escape should fail: %s", out)
	}
	if out, _ := h.call("search_code", map[string]any{"query": "hunter2"}); strings.Contains(out, "hunter2") {
		t.Errorf("search leaked an excluded file: %s", out)
	}
	if len(h.core.Activity()) < 10 {
		t.Errorf("activity not recorded: %d", len(h.core.Activity()))
	}
}

func TestGitTools(t *testing.T) {
	h := newHarness(t)
	if !strings.Contains(h.mustCall("git_status", nil), "branch main") {
		t.Skip("git unavailable")
	}
	p := filepath.Join(h.dir, "store", "store.go")
	data, _ := os.ReadFile(p)
	testutil.Must(t, os.WriteFile(p, []byte(strings.Replace(string(data), "m.orders[o.ID] = o", "m.orders[o.ID] = o // saved", 1)), 0o644))
	h.mustCall("git_diff", map[string]any{"show": true}, "modified store/store.go +1 −1", "~ Memory.Save", "Blast radius:", "Direct callers", "→ Memory.Save")
	if ev := h.waitFor(core.EvFocus); ev.Data.(core.Focus).View != "diff" {
		t.Errorf("focus = %+v", ev.Data)
	}
	h.mustCall("git_diff", map[string]any{"patch": true}, "+\tm.orders[o.ID] = o // saved")
	// The fixture's only commit is a root commit: everything shows as added.
	h.mustCall("git_diff", map[string]any{"commit": "HEAD"}, "added store/store.go", "added api/api.go")
	h.mustCall("git_log", nil, "initial")
}

func TestVisualTools(t *testing.T) {
	h := newHarness(t)
	h.mustCall("start_walkthrough", map[string]any{
		"title": "Order flow",
		"steps": []map[string]any{
			{"title": "Entry", "symbol": "main", "explanation": "starts the server"},
			{"title": "Handler", "file": "api/api.go", "line": 17, "end_line": 32, "explanation": "decodes and validates"},
		},
	}, "2 steps")
	if ev := h.waitFor(core.EvWalkthrough); len(ev.Data.(*core.Walkthrough).Steps[0].File) == 0 {
		t.Error("symbol step should resolve to a file")
	}
	if out, isErr := h.call("start_walkthrough", map[string]any{"title": "bad", "steps": []map[string]any{{"title": "x", "file": "../nope", "explanation": "y"}}}); !isErr {
		t.Errorf("walkthrough outside repo should fail: %s", out)
	}

	h.mustCall("explain_function", map[string]any{
		"symbol":  "validate",
		"summary": "Checks quantity",
		"inputs":  []map[string]any{{"name": "o", "description": "order"}},
		"steps":   []map[string]any{{"line": 35, "text": "reject non-positive qty"}},
	}, "validate")
	ex := h.waitFor(core.EvExplain).Data.(*core.Explanation)
	if ex.Flow == nil || len(ex.Flow.Nodes) < 4 {
		t.Errorf("explanation should carry a control-flow graph: %+v", ex.Flow)
	}

	h.mustCall("show_diagram", map[string]any{"title": "Seq", "mermaid": "sequenceDiagram\nA->>B: hi"}, "displayed")
	if out, isErr := h.call("show_diagram", map[string]any{"title": "bad", "nodes": []map[string]any{{"id": "a", "label": "A"}}, "edges": []map[string]any{{"from": "a", "to": "zzz"}}}); !isErr {
		t.Errorf("dangling edge should fail: %s", out)
	}
	h.mustCall("annotate_code", map[string]any{"annotations": []map[string]any{{"file": "api/api.go", "line": 20, "text": "decode can fail", "kind": "warning"}}}, "1 annotations")
	h.mustCall("show_code", map[string]any{"path": "api/api.go", "line": 3}, "shown api/api.go:3")

	h.core.SetUIContext(core.UIContext{Repo: h.repo.ID, View: "code", File: "api/api.go", Line: 19, Symbol: "Handler.CreateOrder", Selection: "var o store.Order"})
	h.mustCall("get_developer_context", nil, "file: api/api.go line 19", "func (h *Handler) CreateOrder", "var o store.Order")
}

func TestProposalLifecycle(t *testing.T) {
	h := newHarness(t)
	out := h.mustCall("propose_changes", map[string]any{
		"title":   "Reject huge orders",
		"summary": "cap qty",
		"changes": []map[string]any{
			{"path": "api/api.go", "edits": []map[string]any{{"old": "if o.Qty <= 0 {", "new": "if o.Qty <= 0 || o.Qty > 1000 {"}}, "rationale": "cap"},
			{"path": "api/limits.go", "new_content": "package api\n\nconst MaxQty = 1000\n"},
		},
	}, "proposal p1", "modify api/api.go +1 −1", "create api/limits.go +3", "Blast radius:", "No coverage report yet")
	_ = out
	p := h.waitFor(core.EvProposal).Data.(interface{ Additions() (int, int) })
	if add, _ := p.Additions(); add != 4 {
		t.Errorf("additions = %d", add)
	}
	// Nothing was written yet.
	if _, err := os.Stat(filepath.Join(h.dir, "api", "limits.go")); !os.IsNotExist(err) {
		t.Fatal("proposal must not touch disk before acceptance")
	}
	h.mustCall("get_proposal", map[string]any{"id": "p1"}, "pending")

	rt, _ := h.core.Repo(h.repo.ID)
	if _, err := h.core.ResolveProposal(rt, "p1", []string{"api/limits.go"}, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.core.ResolveProposal(rt, "p1", nil, false, "keep the check as is"); err != nil {
		t.Fatal(err)
	}
	h.mustCall("get_proposal", map[string]any{"id": "p1"}, "partial", "applied  create api/limits.go", "rejected modify api/api.go", "keep the check as is")
	if data, _ := os.ReadFile(filepath.Join(h.dir, "api", "limits.go")); !strings.Contains(string(data), "MaxQty") {
		t.Error("accepted file not written")
	}

	if out, isErr := h.call("propose_changes", map[string]any{"title": "x", "summary": "y", "changes": []map[string]any{{"path": ".env", "new_content": "x"}}}); !isErr || !strings.Contains(out, "excluded") {
		t.Errorf("proposing to excluded file should fail: %s", out)
	}
	if out, isErr := h.call("propose_changes", map[string]any{"title": "x", "summary": "y", "changes": []map[string]any{{"path": "api/api.go", "edits": []map[string]any{{"old": "nope", "new": "x"}}}}}); !isErr || !strings.Contains(out, "not found") {
		t.Errorf("bad edit should fail helpfully: %s", out)
	}
}

func TestAccessSwitches(t *testing.T) {
	h := newHarness(t)
	r := h.repo
	r.MCPEnabled = false
	if err := h.core.UpdateRepo(r); err != nil {
		t.Fatal(err)
	}
	if out, isErr := h.call("read_file", map[string]any{"path": "README.md"}); !isErr || !strings.Contains(out, "shared") {
		t.Errorf("unshared repo should be refused: %s", out)
	}
	st := h.core.Config.Get().Settings
	st.MCPEnabled = false
	testutil.Must(t, h.core.UpdateSettings(st))
	if out, isErr := h.call("list_repos", nil); !isErr || !strings.Contains(out, "disabled") {
		t.Errorf("disabled agent access should be refused: %s", out)
	}
}

// docs/MCP.md is the public tool reference; keep it complete.
func TestToolsAreDocumented(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "MCP.md"))
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t)
	res, err := h.cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if !strings.Contains(string(doc), "`"+tool.Name+"`") {
			t.Errorf("tool %s is missing from docs/MCP.md", tool.Name)
		}
	}
}

// Closures are rendered as nested flows under the node holding them.
func TestRenderFlowClosures(t *testing.T) {
	lit := &analysis.Flow{Signature: "func()", File: "a.go", Line: 3, EndLine: 3, Nodes: []analysis.FlowNode{{ID: "n0", Kind: analysis.FlowStart, Label: "func()", Line: 3}}}
	fl := &analysis.Flow{Signature: "func F()", File: "a.go", Line: 1, EndLine: 4, Nodes: []analysis.FlowNode{
		{ID: "n0", Kind: analysis.FlowStart, Label: "F()", Line: 1},
		{ID: "n2", Kind: analysis.FlowBlock, Code: []string{"go func() {}()"}, Line: 3, Closures: []*analysis.Flow{lit}},
	}}
	out := renderFlow(fl)
	for _, want := range []string{"    closure in n2:\n", "      func()  (a.go:3-3)\n", "        n0 [start L3] func()\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderFlow missing %q:\n%s", want, out)
		}
	}
}
