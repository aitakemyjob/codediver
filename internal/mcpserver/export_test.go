package mcpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/internal/core"
)

// fakeWindow answers render requests the way an open CodeDiver window does.
func fakeWindow(t *testing.T, h *harness, png []byte) {
	t.Helper()
	ch, cancel := h.core.Bus.Subscribe()
	t.Cleanup(cancel)
	go func() {
		for ev := range ch {
			req, ok := ev.Data.(core.RenderRequest)
			if !ok {
				continue
			}
			r := core.RenderResult{ID: req.ID, Width: 40, Height: 20}
			switch {
			case req.Theme == "dark":
				r.Error = "boom"
			case req.Format == "svg":
				r.Data = `<svg xmlns="http://www.w3.org/2000/svg"/>`
			default:
				r.Data = base64.StdEncoding.EncodeToString(png)
			}
			_ = h.core.CompleteRender(r)
		}
	}()
}

func TestShowAndExportDemo(t *testing.T) {
	h := newHarness(t)
	png := []byte("\x89PNG\r\n\x1a\nfake")
	fakeWindow(t, h, png)

	h.mustCall("show_demo", map[string]any{
		"title": "CDN",
		"nodes": []map[string]any{
			{"id": "edge", "label": "Edge", "kind": "region"},
			{"id": "dc", "label": "Data center", "parent": "edge", "icon": "pin"},
			{"id": "dns", "label": "DNS", "kind": "icon", "icon": "dns", "parent": "dc"},
			{"id": "client", "label": "Client", "kind": "icon"},
		},
		"edges": []map[string]any{{"from": "client", "to": "dc", "label": "Request", "detail": "example.com", "color": "green"}},
		"steps": []map[string]any{{"title": "Request"}, {"title": "Cached", "values": map[string]string{"dc": "HIT"}}},
	}, "4 nodes and 2 steps")

	// PNG comes back as an image the agent can see.
	res, err := h.cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_demo", Arguments: map[string]any{"step": 1, "theme": "light"}})
	if err != nil || res.IsError {
		t.Fatalf("export png: %v %+v", err, res)
	}
	var img *mcp.ImageContent
	var text string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.ImageContent:
			img = v
		case *mcp.TextContent:
			text += v.Text
		}
	}
	if img == nil || img.MIMEType != "image/png" || !bytes.Equal(img.Data, png) {
		t.Fatalf("image = %+v", img)
	}
	if !strings.Contains(text, `step 1 of 2 ("Request")`) || !strings.Contains(text, "40×20") {
		t.Errorf("caption = %q", text)
	}

	// SVG comes back as text; the default step is the last.
	h.mustCall("export_demo", map[string]any{"format": "svg"}, "<svg", "step 2 of 2")

	// Errors from the window, bad arguments and unknown demos are tool errors.
	for _, args := range []map[string]any{
		{"theme": "dark"},
		{"format": "gif"},
		{"step": 9},
		{"demo": "demo999"},
	} {
		if out, isErr := h.call("export_demo", args); !isErr {
			t.Errorf("%v should fail, got %q", args, out)
		}
	}
	if out, _ := h.call("export_demo", map[string]any{"theme": "dark"}); !strings.Contains(out, "boom") {
		t.Errorf("window error not passed on: %q", out)
	}
}

func TestShowDemoRejectsBadNesting(t *testing.T) {
	h := newHarness(t)
	out, isErr := h.call("show_demo", map[string]any{
		"title": "bad",
		"nodes": []map[string]any{{"id": "a", "label": "A", "parent": "b"}, {"id": "b", "label": "B", "parent": "a"}},
		"steps": []map[string]any{{"title": "x"}},
	})
	if !isErr || !strings.Contains(out, "cycle") {
		t.Errorf("parent cycle accepted: %q", out)
	}
}
