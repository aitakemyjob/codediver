package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCreateDemo(t *testing.T) {
	s, rt, _ := newService(t)
	ctx := context.Background()
	d, err := s.CreateDemo(ctx, rt, Demo{
		Title: "Placing an order",
		Nodes: []DemoNode{
			{ID: "user", Label: "Customer", Kind: "actor"},
			{ID: "api", Label: "CreateOrder", Group: "api", Symbol: "Handler.CreateOrder"},
			{ID: "db", Label: "Memory store", Kind: "datastore", File: "store/store.go"},
			{ID: "secret", Label: "env", File: ".env"},
		},
		Edges: []DemoEdge{{From: "user", To: "api"}, {From: "CreateOrder", To: "db"}},
		Steps: []DemoStep{
			{Title: "Request", Flows: []DemoFlow{{From: "user", To: "api", Label: "POST /orders"}}, Focus: []string{"api"}},
			{Reveal: []string{"db"}, Values: map[string]string{"CreateOrder": "qty=2"}, Code: &DemoCode{File: "api/api.go", Line: 20}},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Layout != "architecture" || d.Nodes[1].File != "api/api.go" || d.Nodes[1].Line == 0 || d.Nodes[3].File != "" {
		t.Errorf("nodes = %+v", d.Nodes)
	}
	if d.Edges[1].From != "api" || d.Steps[1].Title != "Step 2" || d.Steps[1].Values["api"] != "qty=2" || d.Steps[1].Code.EndLine != 20 {
		t.Errorf("resolution = %+v %+v", d.Edges, d.Steps[1])
	}
	if len(s.Demos(rt.ID())) != 1 {
		t.Fatal("demo not stored")
	}

	bad := []Demo{
		{Title: "", Nodes: []DemoNode{{ID: "a"}}, Steps: []DemoStep{{}}},
		{Title: "no steps", Nodes: []DemoNode{{ID: "a"}}},
		{Title: "dup", Nodes: []DemoNode{{ID: "a"}, {ID: "a"}}, Steps: []DemoStep{{}}},
		{Title: "edge", Nodes: []DemoNode{{ID: "a"}}, Edges: []DemoEdge{{From: "a", To: "zz"}}, Steps: []DemoStep{{}}},
		{Title: "focus", Nodes: []DemoNode{{ID: "a"}}, Steps: []DemoStep{{Focus: []string{"nope"}}}},
		{Title: "code", Nodes: []DemoNode{{ID: "a"}}, Steps: []DemoStep{{Code: &DemoCode{File: ".env"}}}},
		{Title: "layout", Layout: "spiral", Nodes: []DemoNode{{ID: "a"}}, Steps: []DemoStep{{}}},
	}
	for _, b := range bad {
		if _, err := s.CreateDemo(ctx, rt, b, nil); err == nil {
			t.Errorf("demo %q should be rejected", b.Title)
		}
	}
}

func TestCreateDemoNesting(t *testing.T) {
	s, rt, _ := newService(t)
	ctx := context.Background()
	nodes := []DemoNode{
		{ID: "edge", Label: "Edge network", Kind: "region"},
		{ID: "dc1", Label: "Data Center 1", Parent: "Edge network", Icon: "pin"},
		{ID: "dns", Label: "DNS", Kind: "icon", Icon: "dns", Parent: "dc1"},
		{ID: "client", Label: "Client", Kind: "icon", Icon: "laptop"},
	}
	d, err := s.CreateDemo(ctx, rt, Demo{
		Title: "nested",
		Nodes: nodes,
		Edges: []DemoEdge{{From: "client", To: "dc1", Label: "Request", Detail: "example.com", Color: "green"}},
		Steps: []DemoStep{{Flows: []DemoFlow{{From: "client", To: "dc1", Color: "purple"}}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Nodes[1].Parent != "edge" || d.Edges[0].Detail != "example.com" {
		t.Errorf("parent by label not resolved: %+v", d.Nodes[1])
	}

	clone := func(edit func([]DemoNode)) []DemoNode {
		out := append([]DemoNode(nil), nodes...)
		edit(out)
		return out
	}
	bad := map[string]Demo{
		"unknown parent":        {Nodes: clone(func(n []DemoNode) { n[1].Parent = "nope" })},
		"self parent":           {Nodes: clone(func(n []DemoNode) { n[0].Parent = "edge" })},
		"cycle":                 {Nodes: clone(func(n []DemoNode) { n[0].Parent = "dns" })},
		"edge to own container": {Nodes: nodes, Edges: []DemoEdge{{From: "dns", To: "edge"}}},
		"flow to own container": {Nodes: nodes, Steps: []DemoStep{{Flows: []DemoFlow{{From: "edge", To: "dc1"}}}}},
		"bad color":             {Nodes: nodes, Edges: []DemoEdge{{From: "client", To: "dc1", Color: "magenta"}}},
	}
	for name, b := range bad {
		b.Title = name
		if len(b.Steps) == 0 {
			b.Steps = []DemoStep{{}}
		}
		if _, err := s.CreateDemo(ctx, rt, b, nil); err == nil {
			t.Errorf("%s should be rejected", name)
		}
	}
}

func TestCreateDemoFromSeed(t *testing.T) {
	s, rt, _ := newService(t)
	d, err := s.CreateDemo(context.Background(), rt, Demo{
		Title: "Order validation",
		Steps: []DemoStep{
			{Title: "Handler runs", Focus: []string{"Handler.CreateOrder"}},
			{Title: "Validation fails", Focus: []string{"validate"}, Values: map[string]string{"validate": "ErrInvalid"}},
		},
	}, &DemoSeed{Symbol: "Handler.CreateOrder", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if d.Layout != "tree" || len(d.Nodes) < 2 || len(d.Edges) == 0 {
		t.Fatalf("seeded scene = %+v", d)
	}
	if v := d.Steps[1].Values; len(v) != 1 || !strings.HasSuffix(d.Steps[1].Focus[0], "validate") {
		t.Errorf("steps should resolve function names: %+v", d.Steps[1])
	}
	if _, err := s.CreateDemo(context.Background(), rt, Demo{Title: "x", Steps: []DemoStep{{}}}, &DemoSeed{Symbol: "NoSuchFunc"}); err == nil {
		t.Error("unknown seed symbol should fail")
	}
}

func TestCreateAnimation(t *testing.T) {
	s, rt, _ := newService(t)
	a, err := s.CreateAnimation(rt, Animation{Title: "Bubble sort", Source: `<svg viewBox="0 0 10 10"><rect width="1" height="1"><animate attributeName="x" to="9" dur="1s"/></rect></svg>`})
	if err != nil || a.Format != "svg" {
		t.Fatalf("svg = %+v %v", a, err)
	}
	if a, err := s.CreateAnimation(rt, Animation{Title: "Cache", Source: "<!doctype html><canvas></canvas><script>1</script>"}); err != nil || a.Format != "html" {
		t.Fatalf("html = %+v %v", a, err)
	}
	for _, bad := range []Animation{
		{Title: "", Source: "<svg/>"},
		{Title: "empty"},
		{Title: "fmt", Format: "gif", Source: "x"},
		{Title: "notsvg", Format: "svg", Source: "<div/>"},
		{Title: "big", Source: strings.Repeat("x", maxAnimationBytes+1)},
	} {
		if _, err := s.CreateAnimation(rt, bad); err == nil {
			t.Errorf("animation %q should be rejected", bad.Title)
		}
	}
	if len(s.Animations(rt.ID())) != 2 {
		t.Error("animations not stored")
	}
}

func TestRenderDemo(t *testing.T) {
	s, rt, _ := newService(t)
	ctx := context.Background()
	if _, _, err := s.RenderDemo(ctx, rt.ID(), "", 0, "png", ""); err == nil {
		t.Fatal("rendering with no demos should fail")
	}
	d, err := s.CreateDemo(ctx, rt, Demo{Title: "x", Nodes: []DemoNode{{ID: "a"}}, Steps: []DemoStep{{}, {}}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A fake window: answers render requests, or reports an error for svg.
	ch, cancel := s.Bus.Subscribe()
	defer cancel()
	go func() {
		for ev := range ch {
			if req, ok := ev.Data.(RenderRequest); ok && ev.Type == EvRender {
				r := RenderResult{ID: req.ID, Data: "aGk=", Width: 10, Height: 5}
				if req.Format == "svg" {
					r = RenderResult{ID: req.ID, Error: "layout failed"}
				}
				if req.Step != 1 || req.Demo.ID != d.ID || req.Theme != "current" {
					r = RenderResult{ID: req.ID, Error: fmt.Sprintf("unexpected request %+v", req)}
				}
				_ = s.CompleteRender(r)
			}
		}
	}()
	r, _, err := s.RenderDemo(ctx, rt.ID(), "", 0, "", "")
	if err != nil || r.Data != "aGk=" || r.Width != 10 {
		t.Fatalf("render = %+v, %v", r, err)
	}
	if _, _, err := s.RenderDemo(ctx, rt.ID(), d.ID, 2, "svg", ""); err == nil || !strings.Contains(err.Error(), "layout failed") {
		t.Errorf("window error not passed on: %v", err)
	}
	for _, bad := range [][2]string{{"gif", ""}, {"png", "neon"}} {
		if _, _, err := s.RenderDemo(ctx, rt.ID(), d.ID, 1, bad[0], bad[1]); err == nil {
			t.Errorf("%v should be rejected", bad)
		}
	}
	if _, _, err := s.RenderDemo(ctx, rt.ID(), d.ID, 3, "png", ""); err == nil {
		t.Error("step out of range should be rejected")
	}
	if _, _, err := s.RenderDemo(ctx, "other-repo", d.ID, 1, "png", ""); err == nil {
		t.Error("another repo's demo must not be found")
	}
	if err := s.CompleteRender(RenderResult{ID: "render999"}); err != nil {
		t.Errorf("a late answer should be ignored, got %v", err)
	}
}

func TestRenderDemoTimesOut(t *testing.T) {
	s, rt, _ := newService(t)
	ctx := context.Background()
	if _, err := s.CreateDemo(ctx, rt, Demo{Title: "x", Nodes: []DemoNode{{ID: "a"}}, Steps: []DemoStep{{}}}, nil); err != nil {
		t.Fatal(err)
	}
	old := renderTimeout
	renderTimeout = 50 * time.Millisecond
	defer func() { renderTimeout = old }()
	if _, _, err := s.RenderDemo(ctx, rt.ID(), "", 0, "png", ""); err == nil || !strings.Contains(err.Error(), "no CodeDiver window") {
		t.Errorf("err = %v", err)
	}
}
