package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aitakemyjob/codediver/internal/analysis"
)

// ---- demos -------------------------------------------------------------------
//
// A demo is an animated, step-by-step presentation an agent puts on the
// developer's screen: a scene (nodes and edges laid out as an architecture
// or an execution tree) plus steps that reveal parts of it, highlight
// nodes, send packets along edges, attach values and show code.

// DemoNode is one element of a demo scene.
type DemoNode struct {
	ID     string `json:"id" jsonschema:"unique id, referenced by edges and steps"`
	Label  string `json:"label" jsonschema:"text shown on the node"`
	Kind   string `json:"kind,omitempty" jsonschema:"visual style: service, module, function, datastore, queue, external, actor, ui, file, note; icon for a compact icon tile with the label underneath; region for a dashed, tinted container (a network or trust boundary)"`
	Detail string `json:"detail,omitempty" jsonschema:"secondary text under the label (signature, responsibility); a container's subtitle"`
	Group  string `json:"group,omitempty" jsonschema:"layer the node belongs to (e.g. frontend, api, storage); drawn as a lane. Ignored when any node has a parent"`
	Parent string `json:"parent,omitempty" jsonschema:"id of the container node this node sits inside. Any node with children is drawn as a container (title, subtitle, icon) and may itself have a parent, e.g. edge network > data center > DNS. A container of only edge-less icon tiles lays them out as a grid. Edges and flows may connect containers"`
	Icon   string `json:"icon,omitempty" jsonschema:"icon name: shield, dns, network, globe, cdn, cache, database, server, servers, cloud, laptop, phone, tablet, user, users, lock, key, queue, function, cpu, file, mail, payment, bell, search, chart, megaphone, pin, layers, disk, router, firewall, api, code, box, clock, bolt, eye, settings"`
	Symbol string `json:"symbol,omitempty" jsonschema:"code symbol this node represents; fills in file and line"`
	File   string `json:"file,omitempty" jsonschema:"repo-relative file; clicking the node opens it"`
	Line   int    `json:"line,omitempty"`
}

// DemoEdge connects two nodes.
type DemoEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label,omitempty" jsonschema:"bold text drawn on the edge, e.g. Request"`
	Detail string `json:"detail,omitempty" jsonschema:"second line under the label, e.g. example.com/index.html"`
	Kind   string `json:"kind,omitempty" jsonschema:"call, data, async, event or dependency"`
	Color  string `json:"color,omitempty" jsonschema:"blue, green, purple, orange, red, yellow or gray; e.g. one color per client's path"`
}

// DemoFlow animates a packet travelling along an edge during a step.
type DemoFlow struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label,omitempty" jsonschema:"what travels, e.g. 'POST /orders' or 'Order{qty: 0}'"`
	Detail string `json:"detail,omitempty" jsonschema:"second line under the label"`
	Kind   string `json:"kind,omitempty" jsonschema:"request (default), response, error, event or data"`
	Color  string `json:"color,omitempty" jsonschema:"overrides the kind's color: blue, green, purple, orange, red, yellow or gray"`
}

// DemoCode is a code excerpt shown beside a step.
type DemoCode struct {
	File    string `json:"file" jsonschema:"repo-relative file"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
}

// DemoStep is one frame of a demo.
type DemoStep struct {
	Title     string            `json:"title" jsonschema:"short heading for the step"`
	Narration string            `json:"narration,omitempty" jsonschema:"markdown explanation shown with the step"`
	Reveal    []string          `json:"reveal,omitempty" jsonschema:"node ids that appear at this step (build the scene up gradually); nodes never revealed are visible from the start"`
	Focus     []string          `json:"focus,omitempty" jsonschema:"node ids to highlight in this step"`
	Flows     []DemoFlow        `json:"flows,omitempty" jsonschema:"packets animated along edges in this step"`
	Values    map[string]string `json:"values,omitempty" jsonschema:"node id → value shown on the node (arguments, return values, state)"`
	Code      *DemoCode         `json:"code,omitempty" jsonschema:"code to show beside the step"`
}

// Demo is an animated presentation.
type Demo struct {
	ID        string     `json:"id"`
	Repo      string     `json:"repo"`
	Title     string     `json:"title"`
	Summary   string     `json:"summary,omitempty"`
	Layout    string     `json:"layout"` // architecture (LR lanes) or tree (top-down)
	Nodes     []DemoNode `json:"nodes"`
	Edges     []DemoEdge `json:"edges"`
	Steps     []DemoStep `json:"steps"`
	CreatedAt time.Time  `json:"createdAt"`
}

// DemoSeed builds a scene from the real call graph so the agent only has
// to supply the steps.
type DemoSeed struct {
	Symbol string `json:"symbol" jsonschema:"function the execution tree starts from"`
	Depth  int    `json:"depth,omitempty" jsonschema:"call depth to include (default 3, max 5)"`
}

const (
	maxDemoNodes = 120
	maxDemoSteps = 80
	maxDemoDepth = 5
)

// demoColors are the colors edges and flows may ask for.
var demoColors = map[string]bool{"": true, "blue": true, "green": true, "purple": true, "orange": true, "red": true, "yellow": true, "gray": true}

// CreateDemo validates a demo, optionally seeds its scene from the call
// graph, stores it and puts it on the developer's screen. Steps may refer
// to nodes by id, label or symbol name.
func (s *Service) CreateDemo(ctx context.Context, rt *RepoHandle, d Demo, seed *DemoSeed) (*Demo, error) {
	if strings.TrimSpace(d.Title) == "" {
		return nil, errors.New("title is required")
	}
	if seed != nil && seed.Symbol != "" {
		if err := s.seedDemo(ctx, rt, &d, *seed); err != nil {
			return nil, err
		}
	}
	if len(d.Nodes) == 0 {
		return nil, errors.New("a demo needs nodes (or a seed symbol)")
	}
	if len(d.Nodes) > maxDemoNodes {
		return nil, fmt.Errorf("too many nodes (%d, max %d); split it into several demos", len(d.Nodes), maxDemoNodes)
	}
	if len(d.Steps) == 0 {
		return nil, errors.New("a demo needs at least one step")
	}
	if len(d.Steps) > maxDemoSteps {
		return nil, fmt.Errorf("too many steps (%d, max %d)", len(d.Steps), maxDemoSteps)
	}
	switch d.Layout {
	case "", "architecture":
		d.Layout = "architecture"
	case "tree":
	default:
		return nil, fmt.Errorf("layout must be architecture or tree, not %q", d.Layout)
	}

	ix, _ := s.Index(ctx, rt)
	ids := map[string]bool{}
	for i := range d.Nodes {
		n := &d.Nodes[i]
		if n.ID == "" {
			return nil, fmt.Errorf("node %d needs an id", i+1)
		}
		if ids[n.ID] {
			return nil, fmt.Errorf("duplicate node id %q", n.ID)
		}
		ids[n.ID] = true
		if n.Label == "" {
			n.Label = n.ID
		}
		if n.Symbol != "" && ix != nil {
			if found := ix.Find(n.Symbol); len(found) > 0 {
				n.Symbol, n.File, n.Line = found[0].ID, found[0].File, found[0].Line
			} else {
				n.Symbol = ""
			}
		}
		if n.File != "" {
			if _, rel, err := rt.ws.Resolve(n.File, defaults); err == nil {
				n.File = rel
			} else {
				n.File, n.Line = "", 0
			}
		}
	}
	resolve := nodeResolver(d.Nodes)
	if err := resolveParents(d.Nodes, resolve); err != nil {
		return nil, err
	}
	parentOf := map[string]string{}
	for _, n := range d.Nodes {
		parentOf[n.ID] = n.Parent
	}
	// A line from a node to its own container can't be drawn.
	nested := func(a, b string) bool {
		for p := parentOf[a]; p != ""; p = parentOf[p] {
			if p == b {
				return true
			}
		}
		return false
	}
	link := func(from, to, color string) (string, string, error) {
		f, err1 := resolve(from)
		t, err2 := resolve(to)
		if err := errors.Join(err1, err2); err != nil {
			return "", "", err
		}
		if nested(f, t) || nested(t, f) {
			return "", "", fmt.Errorf("%s is inside %s; connect it to a sibling instead", f, t)
		}
		if !demoColors[color] {
			return "", "", fmt.Errorf("unknown color %q (use blue, green, purple, orange, red, yellow or gray)", color)
		}
		return f, t, nil
	}
	for i := range d.Edges {
		e := &d.Edges[i]
		from, to, err := link(e.From, e.To, e.Color)
		if err != nil {
			return nil, fmt.Errorf("edge %d: %w", i+1, err)
		}
		e.From, e.To = from, to
	}
	for i := range d.Steps {
		st := &d.Steps[i]
		if st.Title == "" {
			st.Title = fmt.Sprintf("Step %d", i+1)
		}
		where := fmt.Sprintf("step %d (%s)", i+1, st.Title)
		var err error
		if st.Reveal, err = resolveAll(resolve, st.Reveal); err != nil {
			return nil, fmt.Errorf("%s reveal: %w", where, err)
		}
		if st.Focus, err = resolveAll(resolve, st.Focus); err != nil {
			return nil, fmt.Errorf("%s focus: %w", where, err)
		}
		for j := range st.Flows {
			f := &st.Flows[j]
			from, to, err := link(f.From, f.To, f.Color)
			if err != nil {
				return nil, fmt.Errorf("%s flow %d: %w", where, j+1, err)
			}
			f.From, f.To = from, to
		}
		if len(st.Values) > 0 {
			vals := map[string]string{}
			for ref, v := range st.Values {
				id, err := resolve(ref)
				if err != nil {
					return nil, fmt.Errorf("%s values: %w", where, err)
				}
				vals[id] = v
			}
			st.Values = vals
		}
		if st.Code != nil {
			_, rel, err := rt.ws.Resolve(st.Code.File, defaults)
			if err != nil {
				return nil, fmt.Errorf("%s code: %w", where, err)
			}
			st.Code.File = rel
			if st.Code.EndLine < st.Code.Line {
				st.Code.EndLine = st.Code.Line
			}
		}
	}
	if d.Edges == nil {
		d.Edges = []DemoEdge{}
	}

	d.Repo, d.CreatedAt = rt.cfg.ID, time.Now()
	s.session.mu.Lock()
	d.ID = s.session.id("demo")
	s.session.demos[d.ID] = &d
	s.session.mu.Unlock()
	s.emit(EvDemo, &d)
	s.Show(Focus{View: "demo", Repo: rt.cfg.ID, Title: d.Title, Params: map[string]any{"id": d.ID}})
	return &d, nil
}

// seedDemo fills a demo's scene with the call tree below a symbol.
func (s *Service) seedDemo(ctx context.Context, rt *RepoHandle, d *Demo, seed DemoSeed) error {
	depth := seed.Depth
	if depth <= 0 {
		depth = 3
	}
	g, err := s.CallGraph(ctx, rt, seed.Symbol, analysis.CallGraphOptions{Direction: "callees", Depth: min(depth, 5), MaxNodes: 60})
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	have := map[string]bool{}
	for _, n := range d.Nodes {
		have[n.ID] = true
	}
	for _, n := range g.Nodes {
		if have[n.ID] {
			continue
		}
		kind := "function"
		if n.External {
			kind = "external"
		}
		d.Nodes = append(d.Nodes, DemoNode{ID: n.ID, Label: n.Label, Kind: kind, File: n.File, Line: n.Line, Group: n.Group})
		have[n.ID] = true
	}
	for _, e := range g.Edges {
		if e.Kind == "call" || e.Kind == "dispatch" || e.Kind == "go" || e.Kind == "defer" {
			d.Edges = append(d.Edges, DemoEdge{From: e.From, To: e.To, Kind: "call"})
		}
	}
	if d.Layout == "" {
		d.Layout = "tree"
	}
	return nil
}

// nodeResolver maps a reference to a node id: an exact id, an exact label,
// or a symbol/label suffix such as "validate" for "api.validate". Unknown
// or ambiguous references are errors that list valid ids.
func nodeResolver(nodes []DemoNode) func(string) (string, error) {
	return func(ref string) (string, error) {
		ref = strings.TrimSpace(ref)
		var byLabel, bySuffix []string
		for _, n := range nodes {
			switch {
			case n.ID == ref:
				return n.ID, nil
			case n.Label == ref:
				byLabel = append(byLabel, n.ID)
			case strings.HasSuffix(n.Label, "."+ref) || strings.HasSuffix(n.ID, "#"+ref) || strings.HasSuffix(n.ID, "."+ref):
				bySuffix = append(bySuffix, n.ID)
			}
		}
		for _, list := range [][]string{byLabel, bySuffix} {
			if len(list) == 1 {
				return list[0], nil
			}
			if len(list) > 1 {
				return "", fmt.Errorf("%q matches several nodes (%s); use an id", ref, strings.Join(list, ", "))
			}
		}
		var known []string
		for _, n := range nodes {
			known = append(known, n.ID)
		}
		sort.Strings(known)
		if len(known) > 12 {
			known = append(known[:12], "…")
		}
		return "", fmt.Errorf("unknown node %q (nodes: %s)", ref, strings.Join(known, ", "))
	}
}

// resolveParents resolves each node's parent reference to an id and
// rejects self-parents, cycles and nesting deeper than maxDemoDepth.
func resolveParents(nodes []DemoNode, resolve func(string) (string, error)) error {
	parentOf := map[string]string{}
	for i := range nodes {
		n := &nodes[i]
		if n.Parent == "" {
			continue
		}
		p, err := resolve(n.Parent)
		if err != nil {
			return fmt.Errorf("node %s parent: %w", n.ID, err)
		}
		if p == n.ID {
			return fmt.Errorf("node %s can't be its own parent", n.ID)
		}
		n.Parent = p
		parentOf[n.ID] = p
	}
	for id := range parentOf {
		depth := 0
		for p := parentOf[id]; p != ""; p = parentOf[p] {
			if p == id {
				return fmt.Errorf("node %s is inside itself (parent cycle)", id)
			}
			if depth++; depth > maxDemoDepth {
				return fmt.Errorf("node %s is nested more than %d levels deep", id, maxDemoDepth)
			}
		}
	}
	return nil
}

// resolveAll resolves a list of node references.
func resolveAll(resolve func(string) (string, error), refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		id, err := resolve(r)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// Demos lists a repository's demos, newest first.
func (s *Service) Demos(repo string) []*Demo {
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	var out []*Demo
	for _, d := range s.session.demos {
		if repo == "" || d.Repo == repo {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ---- free-form animations ----------------------------------------------------

// Animation is a self-contained SVG or HTML/JS animation written by an
// agent. The UI renders it in a sandboxed frame with no network access,
// no access to CodeDiver's API and no same-origin privileges.
type Animation struct {
	ID        string    `json:"id"`
	Repo      string    `json:"repo"`
	Title     string    `json:"title"`
	Caption   string    `json:"caption,omitempty"`
	Format    string    `json:"format"` // svg or html
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
}

const maxAnimationBytes = 512 << 10

// CreateAnimation validates and stores an agent's animation and puts it on
// the developer's screen.
func (s *Service) CreateAnimation(rt *RepoHandle, a Animation) (*Animation, error) {
	if strings.TrimSpace(a.Title) == "" {
		return nil, errors.New("title is required")
	}
	a.Source = strings.TrimSpace(a.Source)
	if a.Source == "" {
		return nil, errors.New("source is required")
	}
	if len(a.Source) > maxAnimationBytes {
		return nil, fmt.Errorf("source is %d KB; keep animations under %d KB", len(a.Source)>>10, maxAnimationBytes>>10)
	}
	switch a.Format {
	case "":
		a.Format = "html"
		if strings.HasPrefix(a.Source, "<svg") {
			a.Format = "svg"
		}
	case "svg", "html":
	default:
		return nil, fmt.Errorf("format must be svg or html, not %q", a.Format)
	}
	if a.Format == "svg" && !strings.Contains(a.Source, "<svg") {
		return nil, errors.New("svg source must contain an <svg> element")
	}
	a.Repo, a.CreatedAt = rt.cfg.ID, time.Now()
	s.session.mu.Lock()
	a.ID = s.session.id("anim")
	s.session.animations[a.ID] = &a
	s.session.mu.Unlock()
	s.emit(EvAnimation, &a)
	s.Show(Focus{View: "animation", Repo: rt.cfg.ID, Title: a.Title, Params: map[string]any{"id": a.ID}})
	return &a, nil
}

// Animations lists a repository's animations, newest first.
func (s *Service) Animations(repo string) []*Animation {
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	var out []*Animation
	for _, a := range s.session.animations {
		if repo == "" || a.Repo == repo {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
