package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aitakemyjob/codediver/internal/analysis"
)

// session holds ephemeral, in-memory collaboration state for this run:
// agent-authored walkthroughs, diagrams, explanations and annotations, the
// developer's current focus, and the activity log.
type session struct {
	mu           sync.Mutex
	seq          int
	walkthroughs map[string]*Walkthrough
	diagrams     map[string]*Diagram
	demos        map[string]*Demo
	animations   map[string]*Animation
	renders      map[string]chan RenderResult // pending renders by id
	explains     map[string]*Explanation      // by repo|symbol
	annotations  map[string][]Annotation      // by repo
	ctx          UIContext
	activity     []Activity
	attn         map[string]map[string]int // repo -> file -> count
}

// newSession creates empty session state.
func newSession() *session {
	return &session{
		walkthroughs: map[string]*Walkthrough{},
		diagrams:     map[string]*Diagram{},
		demos:        map[string]*Demo{},
		animations:   map[string]*Animation{},
		renders:      map[string]chan RenderResult{},
		explains:     map[string]*Explanation{},
		annotations:  map[string][]Annotation{},
		attn:         map[string]map[string]int{},
	}
}

// id returns a new session-unique ID with the given prefix. Callers must
// hold s.mu.
func (s *session) id(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s%d", prefix, s.seq)
}

const maxActivity = 500

// addActivity appends to the activity ring buffer, assigning an ID and
// timestamp.
func (s *session) addActivity(a Activity) Activity {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	a.ID = s.seq
	if a.Time.IsZero() {
		a.Time = time.Now()
	}
	s.activity = append(s.activity, a)
	if len(s.activity) > maxActivity {
		s.activity = s.activity[len(s.activity)-maxActivity:]
	}
	return a
}

// activities returns a copy of the activity log.
func (s *session) activities() []Activity {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Activity{}, s.activity...)
}

// touch counts one agent visit to a file for the attention heatmap.
func (s *session) touch(repo, file string) {
	if file == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.attn[repo]
	if m == nil {
		m = map[string]int{}
		s.attn[repo] = m
	}
	m[file]++
}

// attention returns per-file agent visit counts for a repository.
func (s *session) attention(repo string) map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]int{}
	for k, v := range s.attn[repo] {
		out[k] = v
	}
	return out
}

// explanationFor returns the agent's explanation of a symbol, if any.
func (s *session) explanationFor(repo, sym string) *Explanation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.explains[repo+"|"+sym]
}

// TouchFile records agent attention for the "agent" heatmap.
func (s *Service) TouchFile(rt *RepoHandle, file string) { s.session.touch(rt.cfg.ID, file) }

// ---- focus -----------------------------------------------------------------

// Focus asks the UI to navigate somewhere. Views: code, graph, trace, flow,
// heatmap, diff, walkthrough, diagram, proposal, explain, overview.
type Focus struct {
	View    string         `json:"view"`
	Repo    string         `json:"repo"`
	Title   string         `json:"title,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
	Message string         `json:"message,omitempty"`
}

// Show asks connected UIs to navigate to a view (they follow it when
// "Follow agent" is on).
func (s *Service) Show(f Focus) { s.emit(EvFocus, f) }

// ---- UI context ------------------------------------------------------------

// UIContext is what the developer is looking at, so agents can answer
// "explain this" without the developer copying code around.
type UIContext struct {
	Repo      string    `json:"repo"`
	View      string    `json:"view"`
	File      string    `json:"file,omitempty"`
	Line      int       `json:"line,omitempty"`
	EndLine   int       `json:"endLine,omitempty"`
	Selection string    `json:"selection,omitempty"`
	Symbol    string    `json:"symbol,omitempty"`
	Title     string    `json:"title,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SetUIContext records what the developer is looking at; selections are
// capped at 20k characters.
func (s *Service) SetUIContext(c UIContext) {
	c.UpdatedAt = time.Now()
	if len(c.Selection) > 20000 {
		c.Selection = c.Selection[:20000]
	}
	s.session.mu.Lock()
	s.session.ctx = c
	s.session.mu.Unlock()
}

// UIContext returns what the developer is currently looking at.
func (s *Service) UIContext() UIContext {
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	return s.session.ctx
}

// ---- walkthroughs ----------------------------------------------------------

type WalkStep struct {
	Title       string `json:"title" jsonschema:"short step title"`
	File        string `json:"file,omitempty" jsonschema:"repo-relative file path (optional when symbol is given)"`
	Line        int    `json:"line,omitempty" jsonschema:"first line to highlight (1-based)"`
	EndLine     int    `json:"end_line,omitempty" jsonschema:"last line to highlight"`
	Symbol      string `json:"symbol,omitempty" jsonschema:"optional symbol id or name this step is about; fills in file/line when omitted"`
	Explanation string `json:"explanation" jsonschema:"markdown explanation of what happens here and why it matters"`
}

type Walkthrough struct {
	ID        string     `json:"id"`
	Repo      string     `json:"repo"`
	Title     string     `json:"title"`
	Summary   string     `json:"summary,omitempty"`
	Steps     []WalkStep `json:"steps"`
	CreatedAt time.Time  `json:"createdAt"`
}

// CreateWalkthrough validates a guided tour (resolving symbol steps to
// file ranges and sandboxing paths), stores it and shows it in the UI.
func (s *Service) CreateWalkthrough(ctx context.Context, rt *RepoHandle, w Walkthrough) (*Walkthrough, error) {
	if len(w.Steps) == 0 {
		return nil, errors.New("a walkthrough needs at least one step")
	}
	ix, _ := s.Index(ctx, rt)
	for i := range w.Steps {
		st := &w.Steps[i]
		if st.Symbol != "" && ix != nil {
			if found := ix.Find(st.Symbol); len(found) > 0 {
				sym := found[0]
				st.Symbol = sym.ID
				if st.File == "" {
					st.File = sym.File
				}
				if st.Line == 0 {
					st.Line, st.EndLine = sym.Line, sym.EndLine
				}
			}
		}
		if st.File == "" {
			return nil, fmt.Errorf("step %d (%q) needs a file or a resolvable symbol", i+1, st.Title)
		}
		if _, rel, err := rt.ws.Resolve(st.File, defaults); err != nil {
			return nil, fmt.Errorf("step %d: %s: %w", i+1, st.File, err)
		} else {
			st.File = rel
		}
		if st.EndLine < st.Line {
			st.EndLine = st.Line
		}
		s.session.touch(rt.cfg.ID, st.File)
	}
	s.session.mu.Lock()
	w.ID = s.session.id("w")
	w.Repo = rt.cfg.ID
	w.CreatedAt = time.Now()
	s.session.walkthroughs[w.ID] = &w
	s.session.mu.Unlock()
	s.emit(EvWalkthrough, &w)
	s.Show(Focus{View: "walkthrough", Repo: rt.cfg.ID, Title: w.Title, Params: map[string]any{"id": w.ID}})
	return &w, nil
}

// Walkthroughs lists a repository's walkthroughs, newest first.
func (s *Service) Walkthroughs(repo string) []*Walkthrough {
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	var out []*Walkthrough
	for _, w := range s.session.walkthroughs {
		if repo == "" || w.Repo == repo {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ---- diagrams --------------------------------------------------------------

type DiagramNode struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Kind   string  `json:"kind,omitempty" jsonschema:"visual style: function, class, service, data, decision, start, end, external, note, step"`
	Detail string  `json:"detail,omitempty" jsonschema:"secondary text shown under the label"`
	File   string  `json:"file,omitempty" jsonschema:"repo-relative file; makes the node clickable"`
	Line   int     `json:"line,omitempty"`
	Group  string  `json:"group,omitempty" jsonschema:"optional cluster name"`
	Heat   float64 `json:"heat,omitempty" jsonschema:"0..1 highlight intensity (e.g. how much this node changes)"`
}

type DiagramEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind,omitempty" jsonschema:"call, data, async, dependency, change"`
}

type Diagram struct {
	ID          string          `json:"id"`
	Repo        string          `json:"repo"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Mermaid     string          `json:"mermaid,omitempty"`
	Graph       *analysis.Graph `json:"graph,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// CreateDiagram stores a Mermaid or node/edge diagram, validating that
// edges reference known nodes and node files stay inside the sandbox.
func (s *Service) CreateDiagram(rt *RepoHandle, title, desc, mermaid, layout string, nodes []DiagramNode, edges []DiagramEdge) (*Diagram, error) {
	d := &Diagram{Repo: rt.cfg.ID, Title: title, Description: desc, Mermaid: strings.TrimSpace(mermaid)}
	if d.Mermaid == "" && len(nodes) == 0 {
		return nil, errors.New("provide either mermaid source or nodes/edges")
	}
	if len(nodes) > 0 {
		g := &analysis.Graph{Kind: "custom", Title: title, Direction: layout}
		known := map[string]bool{}
		for _, n := range nodes {
			if n.ID == "" {
				return nil, errors.New("every node needs an id")
			}
			if n.File != "" {
				if _, rel, err := rt.ws.Resolve(n.File, defaults); err == nil {
					n.File = rel
				} else {
					n.File = ""
				}
			}
			known[n.ID] = true
			g.Nodes = append(g.Nodes, analysis.GraphNode{ID: n.ID, Label: n.Label, Kind: n.Kind, Detail: n.Detail, File: n.File, Line: n.Line, Group: n.Group, Heat: n.Heat})
		}
		for i, e := range edges {
			if !known[e.From] || !known[e.To] {
				return nil, fmt.Errorf("edge %d references unknown node (%s → %s)", i+1, e.From, e.To)
			}
			g.Edges = append(g.Edges, analysis.GraphEdge{ID: fmt.Sprintf("e%d", i), From: e.From, To: e.To, Label: e.Label, Kind: e.Kind})
		}
		d.Graph = g
	}
	s.session.mu.Lock()
	d.ID = s.session.id("d")
	d.CreatedAt = time.Now()
	s.session.diagrams[d.ID] = d
	s.session.mu.Unlock()
	s.emit(EvDiagram, d)
	s.Show(Focus{View: "diagram", Repo: rt.cfg.ID, Title: title, Params: map[string]any{"id": d.ID}})
	return d, nil
}

// Diagrams lists a repository's diagrams, newest first.
func (s *Service) Diagrams(repo string) []*Diagram {
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	var out []*Diagram
	for _, d := range s.session.diagrams {
		if repo == "" || d.Repo == repo {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ---- explanations ----------------------------------------------------------

type ParamNote struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type LogicStep struct {
	Line    int    `json:"line,omitempty" jsonschema:"first line of the code this step describes"`
	EndLine int    `json:"end_line,omitempty"`
	Text    string `json:"text" jsonschema:"what this part of the function does"`
}

type Explanation struct {
	ID          string           `json:"id"`
	Repo        string           `json:"repo"`
	Symbol      string           `json:"symbol"`
	Summary     string           `json:"summary"`
	Inputs      []ParamNote      `json:"inputs,omitempty"`
	Outputs     []ParamNote      `json:"outputs,omitempty"`
	Steps       []LogicStep      `json:"steps,omitempty"`
	SideEffects []string         `json:"sideEffects,omitempty"`
	Notes       string           `json:"notes,omitempty"`
	Flow        *analysis.Flow   `json:"flow,omitempty"`
	Sym         *analysis.Symbol `json:"sym,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
}

// Explain attaches an agent's explanation to a function, together with its
// control-flow graph, and opens the logic view.
func (s *Service) Explain(ctx context.Context, rt *RepoHandle, ex Explanation) (*Explanation, error) {
	flow, sym, err := s.FunctionFlow(ctx, rt, ex.Symbol)
	if sym == nil {
		return nil, err
	}
	ex.Symbol, ex.Sym, ex.Flow = sym.ID, sym, flow
	ex.Repo = rt.cfg.ID
	ex.CreatedAt = time.Now()
	s.session.mu.Lock()
	ex.ID = s.session.id("x")
	s.session.explains[rt.cfg.ID+"|"+sym.ID] = &ex
	s.session.mu.Unlock()
	s.session.touch(rt.cfg.ID, sym.File)
	s.emit(EvExplain, &ex)
	s.Show(Focus{View: "explain", Repo: rt.cfg.ID, Title: sym.Qualified, Params: map[string]any{"symbol": sym.ID}})
	return &ex, nil
}

// Explanations lists a repository's explanations, newest first.
func (s *Service) Explanations(repo string) []*Explanation {
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	var out []*Explanation
	for _, e := range s.session.explains {
		if repo == "" || e.Repo == repo {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ---- annotations -----------------------------------------------------------

type Annotation struct {
	ID      string `json:"id,omitempty" jsonschema:"assigned by CodeDiver; leave empty"`
	File    string `json:"file" jsonschema:"repo-relative file path"`
	Line    int    `json:"line" jsonschema:"1-based line"`
	EndLine int    `json:"end_line,omitempty"`
	Text    string `json:"text" jsonschema:"note shown inline in the editor (markdown)"`
	Kind    string `json:"kind,omitempty" jsonschema:"info (default), warning, bug, idea, question"`
}

// Annotate adds (or with replace, replaces) inline notes shown in the
// editor gutter.
func (s *Service) Annotate(rt *RepoHandle, notes []Annotation, replace bool) ([]Annotation, error) {
	for i := range notes {
		_, rel, err := rt.ws.Resolve(notes[i].File, defaults)
		if err != nil {
			return nil, fmt.Errorf("annotation %d: %s: %w", i+1, notes[i].File, err)
		}
		notes[i].File = rel
		if notes[i].Kind == "" {
			notes[i].Kind = "info"
		}
		if notes[i].EndLine < notes[i].Line {
			notes[i].EndLine = notes[i].Line
		}
	}
	s.session.mu.Lock()
	if replace {
		s.session.annotations[rt.cfg.ID] = nil
	}
	for i := range notes {
		notes[i].ID = s.session.id("a")
		s.session.annotations[rt.cfg.ID] = append(s.session.annotations[rt.cfg.ID], notes[i])
	}
	all := append([]Annotation(nil), s.session.annotations[rt.cfg.ID]...)
	s.session.mu.Unlock()
	s.emit(EvAnnotations, map[string]any{"repo": rt.cfg.ID, "annotations": all})
	return all, nil
}

// Annotations returns a repository's inline notes.
func (s *Service) Annotations(repo string) []Annotation {
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	return append([]Annotation{}, s.session.annotations[repo]...)
}

// ClearAnnotations removes all inline notes for a repository.
func (s *Service) ClearAnnotations(repo string) {
	s.session.mu.Lock()
	delete(s.session.annotations, repo)
	s.session.mu.Unlock()
	s.emit(EvAnnotations, map[string]any{"repo": repo, "annotations": []Annotation{}})
}
