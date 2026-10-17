package analysis

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Build analyzes the given repo-relative files under root.
func Build(root string, files []string, langOf func(string) string) *Index {
	start := time.Now()
	ix := newIndex(root)
	merge := map[[3]string]int{}
	var goFiles, other []string
	for _, f := range files {
		lang := langOf(f)
		if lang == "plaintext" || lang == "" {
			continue
		}
		if lang == "go" || specs[lang] != nil {
			ix.Languages[lang]++
		}
		if lang == "go" {
			goFiles = append(goFiles, f)
		} else if specs[lang] != nil {
			other = append(other, f)
		}
	}
	if len(goFiles) > 0 {
		analyzeGo(root, goFiles, ix, merge)
	}
	if len(other) > 0 {
		analyzeGeneric(root, other, langOf, ix, merge)
	}
	ix.finalize()
	ix.BuiltAt = time.Now()
	ix.Duration = time.Since(start)
	return ix
}

// Find resolves a user/agent supplied reference to a symbol. Accepted forms:
// an exact ID, "file:line", a qualified name ("Server.Start"), or a bare name.
func (ix *Index) Find(ref string) []*Symbol {
	ref = strings.TrimSpace(ref)
	if s, ok := ix.Symbols[ref]; ok {
		return []*Symbol{s}
	}
	if i := strings.LastIndex(ref, ":"); i > 0 {
		if line, err := strconv.Atoi(ref[i+1:]); err == nil {
			if s := ix.SymbolAt(ref[:i], line); s != nil {
				return []*Symbol{s}
			}
		}
	}
	var exactQ, exactN, suffix []*Symbol
	lower := strings.ToLower(ref)
	for _, s := range ix.Symbols {
		if s.Kind == KindExternal {
			continue
		}
		switch {
		case s.Qualified == ref:
			exactQ = append(exactQ, s)
		case s.Name == ref:
			exactN = append(exactN, s)
		case strings.HasSuffix(strings.ToLower(s.ID), lower) || strings.HasSuffix(strings.ToLower(s.Qualified), "."+lower):
			suffix = append(suffix, s)
		}
	}
	for _, list := range [][]*Symbol{exactQ, exactN, suffix} {
		if len(list) > 0 {
			sortSymbols(list)
			return list
		}
	}
	return nil
}

// sortSymbols orders symbols by file, then line.
func sortSymbols(list []*Symbol) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].File != list[j].File {
			return list[i].File < list[j].File
		}
		return list[i].Line < list[j].Line
	})
}

// Search does a fuzzy-ish name search for the command palette.
func (ix *Index) Search(q string, limit int) []*Symbol {
	q = strings.ToLower(strings.TrimSpace(q))
	type scored struct {
		s     *Symbol
		score int
	}
	var res []scored
	for _, s := range ix.Symbols {
		if s.Kind == KindExternal {
			continue
		}
		name := strings.ToLower(s.Qualified)
		score := 0
		switch {
		case q == "":
			score = 1
		case strings.ToLower(s.Name) == q:
			score = 100
		case strings.HasPrefix(strings.ToLower(s.Name), q):
			score = 80
		case strings.Contains(name, q):
			score = 60
		case subsequence(name, q):
			score = 20
		}
		if score == 0 {
			continue
		}
		if s.Exported {
			score += 5
		}
		res = append(res, scored{s, score - len(name)/8})
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].score != res[j].score {
			return res[i].score > res[j].score
		}
		return res[i].s.Qualified < res[j].s.Qualified
	})
	if limit <= 0 {
		limit = 50
	}
	out := make([]*Symbol, 0, min(limit, len(res)))
	for i := 0; i < len(res) && i < limit; i++ {
		out = append(out, res[i].s)
	}
	return out
}

// subsequence reports whether q's characters appear in s in order.
func subsequence(s, q string) bool {
	i := 0
	for j := 0; j < len(s) && i < len(q); j++ {
		if s[j] == q[i] {
			i++
		}
	}
	return i == len(q)
}

// ---- graphs ---------------------------------------------------------------

type GraphNode struct {
	ID        string     `json:"id"`
	Label     string     `json:"label"`
	Kind      string     `json:"kind"`
	Sublabel  string     `json:"sublabel,omitempty"`
	File      string     `json:"file,omitempty"`
	Line      int        `json:"line,omitempty"`
	EndLine   int        `json:"endLine,omitempty"`
	Group     string     `json:"group,omitempty"`
	Detail    string     `json:"detail,omitempty"`
	Depth     int        `json:"depth,omitempty"`
	Root      bool       `json:"root,omitempty"`
	External  bool       `json:"external,omitempty"`
	Heat      float64    `json:"heat,omitempty"`
	Badges    []string   `json:"badges,omitempty"`
	Params    []Param    `json:"params,omitempty"`
	Results   []Param    `json:"results,omitempty"`
	Fields    []Param    `json:"fields,omitempty"`
	Members   []string   `json:"members,omitempty"`
	Symbol    string     `json:"symbol,omitempty"`
	SymKind   SymbolKind `json:"symKind,omitempty"`
	Heuristic bool       `json:"heuristic,omitempty"`
}

type GraphEdge struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Kind      string `json:"kind"`
	Label     string `json:"label,omitempty"`
	Count     int    `json:"count,omitempty"`
	Highlight bool   `json:"highlight,omitempty"`
	Heuristic bool   `json:"heuristic,omitempty"`
}

type Graph struct {
	Kind      string      `json:"kind"`
	Title     string      `json:"title"`
	Root      string      `json:"root,omitempty"`
	Direction string      `json:"direction,omitempty"`
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges"`
	Truncated bool        `json:"truncated,omitempty"`
	Note      string      `json:"note,omitempty"`
}

// symbolNode converts a symbol into a graph node with location, signature
// and badges (complex, interface, exported).
func (ix *Index) symbolNode(s *Symbol) GraphNode {
	n := GraphNode{
		ID: s.ID, Label: s.Qualified, Kind: string(s.Kind), File: s.File, Line: s.Line, EndLine: s.EndLine,
		Group: s.Package, Params: s.Params, Results: s.Results, Symbol: s.ID, SymKind: s.Kind,
		Heuristic: s.Heuristic, External: s.Kind == KindExternal,
	}
	if s.Kind == KindExternal {
		n.Sublabel = s.Package
	} else if s.File != "" {
		n.Sublabel = s.File + ":" + strconv.Itoa(s.Line)
	}
	if s.Complexity > 10 {
		n.Badges = append(n.Badges, "complex")
	}
	if s.Abstract {
		n.Badges = append(n.Badges, "interface")
	}
	if s.Exported && s.Kind != KindExternal {
		n.Badges = append(n.Badges, "exported")
	}
	if s.Doc != "" {
		n.Detail = firstLine(s.Doc)
	}
	return n
}

// firstLine returns s up to its first newline.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

type CallGraphOptions struct {
	Direction       string `json:"direction"` // callees | callers | both
	Depth           int    `json:"depth"`
	IncludeExternal bool   `json:"includeExternal"`
	IncludeRefs     bool   `json:"includeRefs"`
	MaxNodes        int    `json:"maxNodes"`
}

// normalize clamps call-graph options to sane bounds and defaults.
func (o *CallGraphOptions) normalize() {
	if o.Depth <= 0 {
		o.Depth = 2
	}
	if o.Depth > 8 {
		o.Depth = 8
	}
	if o.MaxNodes <= 0 || o.MaxNodes > 600 {
		o.MaxNodes = 150
	}
	switch o.Direction {
	case "callers", "both", "callees":
	default:
		o.Direction = "callees"
	}
}

// edgeAllowed reports whether an edge belongs in a call graph under the
// given options (imports never; refs and externals only on request).
func (ix *Index) edgeAllowed(e Edge, o CallGraphOptions) bool {
	switch e.Kind {
	case EdgeImport, EdgeImplement:
		return false
	case EdgeRef:
		if !o.IncludeRefs {
			return false
		}
	}
	if !o.IncludeExternal {
		if s := ix.Symbols[e.To]; s != nil && s.Kind == KindExternal {
			return false
		}
	}
	return true
}

// CallGraph explores outward from a symbol breadth-first.
func (ix *Index) CallGraph(id string, o CallGraphOptions) *Graph {
	o.normalize()
	root := ix.Symbols[id]
	g := &Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Kind: "callgraph", Root: id, Direction: o.Direction}
	if root == nil {
		return g
	}
	g.Title = "Call graph · " + root.Qualified
	nodes := map[string]int{id: 0}
	edges := map[string]bool{}
	frontier := []string{id}
	addEdge := func(e Edge) {
		key := e.From + "→" + e.To + "|" + string(e.Kind)
		if edges[key] {
			return
		}
		edges[key] = true
		g.Edges = append(g.Edges, GraphEdge{ID: key, From: e.From, To: e.To, Kind: string(e.Kind), Count: max(1, len(e.Lines)), Heuristic: e.Heuristic})
	}
	for depth := 1; depth <= o.Depth && len(frontier) > 0; depth++ {
		var next []string
		for _, cur := range frontier {
			var cand []Edge
			if o.Direction != "callers" {
				cand = append(cand, ix.Callees(cur)...)
			}
			if o.Direction != "callees" {
				cand = append(cand, ix.Callers(cur)...)
			}
			for _, e := range cand {
				if !ix.edgeAllowed(e, o) {
					continue
				}
				other := e.To
				if other == cur {
					other = e.From
				}
				if _, seen := nodes[other]; !seen {
					if len(nodes) >= o.MaxNodes {
						g.Truncated = true
						continue
					}
					nodes[other] = depth
					if s := ix.Symbols[other]; s != nil && s.Kind != KindExternal {
						next = append(next, other)
					}
				}
				addEdge(e)
			}
		}
		frontier = next
	}
	for nid, depth := range nodes {
		s := ix.Symbols[nid]
		if s == nil {
			continue
		}
		n := ix.symbolNode(s)
		n.Depth = depth
		n.Root = nid == id
		g.Nodes = append(g.Nodes, n)
	}
	sortNodes(g)
	return g
}

// sortNodes orders graph nodes by depth then ID, and edges by ID, so
// output is deterministic.
func sortNodes(g *Graph) {
	sort.Slice(g.Nodes, func(i, j int) bool {
		if g.Nodes[i].Depth != g.Nodes[j].Depth {
			return g.Nodes[i].Depth < g.Nodes[j].Depth
		}
		return g.Nodes[i].ID < g.Nodes[j].ID
	})
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
}

// TraceTo answers "how does execution reach this function?": it walks
// callers upward and keeps only nodes that lie on a path to the target,
// marking entry points (no further callers) as roots.
func (ix *Index) TraceTo(id string, depth int, maxNodes int) *Graph {
	if depth <= 0 {
		depth = 6
	}
	if maxNodes <= 0 {
		maxNodes = 200
	}
	target := ix.Symbols[id]
	g := &Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Kind: "trace", Root: id, Direction: "LR"}
	if target == nil {
		return g
	}
	g.Title = "Flow to · " + target.Qualified
	dist := map[string]int{id: 0}
	frontier := []string{id}
	type ek struct{ from, to string }
	edgeSet := map[ek]Edge{}
	for d := 1; d <= depth && len(frontier) > 0; d++ {
		var next []string
		for _, cur := range frontier {
			for _, e := range ix.Callers(cur) {
				if e.Kind == EdgeImport || e.Kind == EdgeImplement {
					continue
				}
				edgeSet[ek{e.From, e.To}] = e
				if _, ok := dist[e.From]; !ok {
					if len(dist) >= maxNodes {
						g.Truncated = true
						continue
					}
					dist[e.From] = d
					next = append(next, e.From)
				}
			}
		}
		frontier = next
	}
	maxD := 0
	for _, d := range dist {
		maxD = max(maxD, d)
	}
	for nid, d := range dist {
		s := ix.Symbols[nid]
		if s == nil {
			continue
		}
		n := ix.symbolNode(s)
		n.Depth = maxD - d // entry points on the left
		n.Root = nid == id
		if nid != id && len(ix.Callers(nid)) == 0 {
			n.Badges = append(n.Badges, "entry")
		}
		g.Nodes = append(g.Nodes, n)
	}
	for _, e := range edgeSet {
		key := e.From + "→" + e.To + "|" + string(e.Kind)
		g.Edges = append(g.Edges, GraphEdge{ID: key, From: e.From, To: e.To, Kind: string(e.Kind), Count: max(1, len(e.Lines)), Highlight: true, Heuristic: e.Heuristic})
	}
	sortNodes(g)
	return g
}

// Impact returns the transitive callers ("blast radius") of a set of
// changed symbols, with distance from the change.
func (ix *Index) Impact(changed []string, depth int) map[string]int {
	if depth <= 0 {
		depth = 3
	}
	dist := map[string]int{}
	var frontier []string
	for _, c := range changed {
		if _, ok := ix.Symbols[c]; ok {
			dist[c] = 0
			frontier = append(frontier, c)
		}
	}
	for d := 1; d <= depth && len(frontier) > 0; d++ {
		var next []string
		for _, cur := range frontier {
			for _, e := range ix.Callers(cur) {
				if e.Kind == EdgeImport || e.Kind == EdgeImplement {
					continue
				}
				if _, ok := dist[e.From]; !ok {
					dist[e.From] = d
					next = append(next, e.From)
				}
			}
		}
		frontier = next
	}
	return dist
}

// ImpactGraph renders changed symbols plus their callers as a graph.
func (ix *Index) ImpactGraph(changed []string, depth int) *Graph {
	dist := ix.Impact(changed, depth)
	g := &Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Kind: "impact", Title: "Change impact", Direction: "RL"}
	for id, d := range dist {
		s := ix.Symbols[id]
		n := ix.symbolNode(s)
		n.Depth = d
		n.Root = d == 0
		n.Heat = 1 / float64(d+1)
		if d == 0 {
			n.Badges = append(n.Badges, "changed")
		}
		g.Nodes = append(g.Nodes, n)
	}
	for id := range dist {
		for _, e := range ix.Callers(id) {
			if _, ok := dist[e.From]; ok && e.Kind != EdgeImport && e.Kind != EdgeImplement {
				key := e.From + "→" + e.To + "|" + string(e.Kind)
				g.Edges = append(g.Edges, GraphEdge{ID: key, From: e.From, To: e.To, Kind: string(e.Kind), Count: max(1, len(e.Lines)), Heuristic: e.Heuristic})
			}
		}
	}
	sortNodes(g)
	return g
}

// PackageGraph renders internal package dependencies. External imports can
// be folded into one node per top-level module.
func (ix *Index) PackageGraph(includeExternal bool, focus string) *Graph {
	g := &Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Kind: "deps", Title: "Package dependencies", Direction: "TB"}
	keep := map[string]bool{}
	if focus != "" {
		keep[focus] = true
		for _, e := range ix.PkgEdges {
			if e.From == focus || e.To == focus {
				keep[e.From], keep[e.To] = true, true
			}
		}
		g.Title = "Dependencies · " + focus
		g.Root = focus
	}
	inDeg := map[string]int{}
	for _, e := range ix.PkgEdges {
		inDeg[e.To]++
	}
	for id, p := range ix.Packages {
		if focus != "" && !keep[id] {
			continue
		}
		label := p.Dir
		if label == "" {
			label = p.Name
		}
		if p.Language == "go" {
			label = p.Name
		}
		n := GraphNode{ID: id, Label: label, Kind: "package", Sublabel: p.Dir, Group: p.Language, Detail: id, Root: id == focus}
		n.Badges = append(n.Badges, strconv.Itoa(len(p.Files))+" files")
		if inDeg[id] > 0 {
			n.Badges = append(n.Badges, strconv.Itoa(inDeg[id])+" dependents")
		}
		if p.Name == "main" {
			n.Badges = append(n.Badges, "entry")
		}
		g.Nodes = append(g.Nodes, n)
	}
	for _, e := range ix.PkgEdges {
		if focus != "" && (!keep[e.From] || !keep[e.To]) {
			continue
		}
		key := e.From + "→" + e.To
		g.Edges = append(g.Edges, GraphEdge{ID: key, From: e.From, To: e.To, Kind: "import", Heuristic: e.Heuristic})
	}
	if includeExternal {
		ext := map[string]bool{}
		for id, p := range ix.Packages {
			if focus != "" && id != focus {
				continue
			}
			for _, im := range p.Imports {
				if _, internal := ix.Packages[im]; internal || strings.HasPrefix(im, "dir:") {
					continue
				}
				mod := externalModule(im)
				eid := "extpkg:" + mod
				if !ext[eid] {
					ext[eid] = true
					g.Nodes = append(g.Nodes, GraphNode{ID: eid, Label: mod, Kind: "external", External: true})
				}
				key := id + "→" + eid
				g.Edges = append(g.Edges, GraphEdge{ID: key, From: id, To: eid, Kind: "import"})
			}
		}
	}
	dedupeEdges(g)
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	return g
}

// dedupeEdges removes edges with duplicate IDs and sorts the rest.
func dedupeEdges(g *Graph) {
	seen := map[string]bool{}
	out := g.Edges[:0]
	for _, e := range g.Edges {
		if !seen[e.ID] {
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	g.Edges = out
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
}

// externalModule folds an import path into something readable:
// stdlib "net/http" -> "net/http", "github.com/a/b/c" -> "github.com/a/b".
func externalModule(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) > 0 && strings.Contains(parts[0], ".") && len(parts) > 3 {
		return strings.Join(parts[:3], "/")
	}
	return p
}

// TypeGraph renders types (structs/classes/interfaces) in a package or file
// with their fields, methods and implements relationships.
func (ix *Index) TypeGraph(scope string) *Graph {
	g := &Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}, Kind: "types", Title: "Types · " + scope, Direction: "TB"}
	in := func(s *Symbol) bool {
		if scope == "" {
			return true
		}
		return s.Package == scope || s.File == scope || strings.HasPrefix(s.File, strings.TrimSuffix(scope, "/")+"/") || "dir:"+dirOf(s.File) == scope
	}
	added := map[string]bool{}
	for _, s := range ix.Symbols {
		switch s.Kind {
		case KindStruct, KindClass, KindInterface, KindType:
		default:
			continue
		}
		if !in(s) {
			continue
		}
		n := ix.symbolNode(s)
		n.Fields = s.Fields
		for _, m := range s.Methods {
			if ms := ix.Symbols[m]; ms != nil {
				sig := ms.Name + "(" + paramList(ms.Params) + ")"
				if len(ms.Results) > 0 {
					sig += " " + paramList(ms.Results)
				}
				n.Members = append(n.Members, sig)
			}
		}
		g.Nodes = append(g.Nodes, n)
		added[s.ID] = true
	}
	for _, e := range ix.Edges {
		if e.Kind == EdgeImplement && added[e.From] && added[e.To] {
			key := e.From + "→" + e.To
			g.Edges = append(g.Edges, GraphEdge{ID: key, From: e.From, To: e.To, Kind: "implements", Label: "implements"})
		}
	}
	// Field-type references between shown types ("has a").
	byName := map[string]string{}
	for id := range added {
		byName[ix.Symbols[id].Name] = id
	}
	for id := range added {
		for _, f := range ix.Symbols[id].Fields {
			t := strings.TrimLeft(f.Type, "*[]")
			if i := strings.LastIndex(t, "."); i >= 0 {
				t = t[i+1:]
			}
			if to, ok := byName[t]; ok && to != id {
				key := id + "→" + to + "|field"
				g.Edges = append(g.Edges, GraphEdge{ID: key, From: id, To: to, Kind: "field", Label: f.Name})
			}
		}
	}
	dedupeEdges(g)
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	if len(g.Nodes) > 120 {
		g.Nodes = g.Nodes[:120]
		g.Truncated = true
	}
	return g
}

// dirOf returns the directory part of a slash-separated path.
func dirOf(f string) string {
	if i := strings.LastIndex(f, "/"); i >= 0 {
		return f[:i]
	}
	return ""
}

// paramList renders params as `name type, ...`.
func paramList(ps []Param) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		if p.Name != "" && p.Type != "" {
			parts[i] = p.Name + " " + p.Type
		} else {
			parts[i] = p.Name + p.Type
		}
	}
	return strings.Join(parts, ", ")
}
