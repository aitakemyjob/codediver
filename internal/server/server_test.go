package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/internal/api"
	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/testutil"
)

func setup(t *testing.T) (*httptest.Server, *core.Service) {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := core.New(store, "test")
	if _, err := svc.AddRepo(testutil.Repo(t), "shop"); err != nil {
		t.Fatal(err)
	}
	ui := fstest.MapFS{"index.html": {Data: []byte("<html>codediver</html>")}, "assets/app.js": {Data: []byte("console.log(1)")}}
	srv := New(svc, api.New(svc), Options{UI: ui})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, svc
}

func invoke(t *testing.T, ts *httptest.Server, method string, args any, header bool) (*http.Response, map[string]json.RawMessage) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"method": method, "args": args})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/invoke", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	if header {
		req.Header.Set(HeaderName, "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]json.RawMessage
	// Refusals (403) are plain text; only successful calls must be JSON.
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil && resp.StatusCode == http.StatusOK {
		t.Fatal(err)
	}
	return resp, out
}

func TestInvoke(t *testing.T) {
	ts, _ := setup(t)
	resp, out := invoke(t, ts, "app.state", nil, true)
	if resp.StatusCode != 200 || out["error"] != nil {
		t.Fatalf("app.state = %d %s", resp.StatusCode, out["error"])
	}
	var st api.AppState
	if err := json.Unmarshal(out["result"], &st); err != nil || len(st.Repos) != 1 || st.Version != "test" {
		t.Fatalf("state = %+v %v", st, err)
	}
	_, out = invoke(t, ts, "sym.file", map[string]string{"file": "api/api.go"}, true)
	if !strings.Contains(string(out["result"]), "CreateOrder") {
		t.Errorf("sym.file = %s", out["result"])
	}
	_, out = invoke(t, ts, "graph.trace", map[string]string{"symbol": "Memory.Save"}, true)
	if !strings.Contains(string(out["result"]), `"kind":"trace"`) {
		t.Errorf("graph.trace = %s", out["result"])
	}
	_, out = invoke(t, ts, "heatmap", map[string]string{"metric": "size"}, true)
	if !strings.Contains(string(out["result"]), `"metric":"size"`) {
		t.Errorf("heatmap = %s", out["result"])
	}
	_, out = invoke(t, ts, "nope.nope", nil, true)
	if !strings.Contains(string(out["error"]), "unknown method") {
		t.Errorf("unknown method = %s", out["error"])
	}
	_, out = invoke(t, ts, "fs.read", map[string]string{"path": ".env"}, true)
	if !strings.Contains(string(out["error"]), "excluded") {
		t.Errorf(".env via API = %s", out)
	}
}

func TestGuards(t *testing.T) {
	ts, _ := setup(t)
	if resp, _ := invoke(t, ts, "app.state", nil, false); resp.StatusCode != http.StatusForbidden {
		t.Errorf("missing header status = %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/invoke", strings.NewReader(`{"method":"app.state"}`))
	req.Header.Set(HeaderName, "1")
	req.Header.Set("Origin", "https://evil.example")
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin status = %d", resp.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/healthz", nil)
	req.Host = "evil.example"
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Errorf("rebinding status = %d", resp.StatusCode)
	}
}

func TestSPAFallback(t *testing.T) {
	ts, _ := setup(t)
	for _, p := range []string{"/", "/some/client/route"} {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(b), "codediver") {
			t.Errorf("%s = %q", p, b)
		}
	}
}

func TestEventsStream(t *testing.T) {
	ts, svc := setup(t)
	resp, err := http.Get(ts.URL + "/api/events?gd=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := bufio.NewReader(resp.Body)
	if _, err := r.ReadString('\n'); err != nil { // ": connected"
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		svc.Show(core.Focus{View: "code", Repo: "x"})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"type":"focus"`) {
			return
		}
	}
	t.Fatal("no focus event received")
}

func TestMCPOverHTTP(t *testing.T) {
	ts, _ := setup(t)
	client := mcp.NewClient(&mcp.Implementation{Name: "http-test", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_symbols", Arguments: map[string]any{"query": "validate"}})
	if err != nil || res.IsError {
		t.Fatalf("call = %+v %v", res, err)
	}
	if txt := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(txt, "validate [func]") {
		t.Errorf("result = %s", txt)
	}
}

// Quitting must not hang while clients hold streaming connections open: an
// agent's MCP session and the browser UI's event stream never finish on
// their own. The desktop app shuts down with a context that has no deadline.
func TestShutdownWithOpenStreams(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := core.New(store, "test")
	srv := New(svc, api.New(svc), Options{UI: fstest.MapFS{"index.html": {Data: []byte("x")}}})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}

	// Browser UI event stream.
	req, _ := http.NewRequest(http.MethodGet, srv.URL()+"/api/events", nil)
	req.Header.Set(HeaderName, "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := bufio.NewReader(resp.Body).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	// Agent session with its standalone server-to-client stream.
	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL() + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	done := make(chan error, 1)
	go func() { done <- srv.Shutdown(context.Background()) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown hung with open streaming connections")
	}
	if svc.MCPStatus().Running {
		t.Error("MCP should be marked stopped")
	}
}
