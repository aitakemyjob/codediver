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
	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

var defaults = config.DefaultExcludes

// ---- files ---------------------------------------------------------------

// Tree returns the repository's visible file tree.
func (s *Service) Tree(rt *RepoHandle) (*workspace.TreeNode, error) {
	return rt.ws.Tree(defaults, false)
}

// ReadFile reads a file through the sandbox.
func (s *Service) ReadFile(rt *RepoHandle, p string) (*workspace.FileContent, error) {
	return rt.ws.ReadFile(p, defaults)
}

// Search greps the repository; the bool reports truncated results.
func (s *Service) Search(rt *RepoHandle, opts workspace.SearchOptions) ([]workspace.SearchMatch, bool, error) {
	return rt.ws.Search(opts, defaults)
}

// ---- symbols ---------------------------------------------------------------

// FileSymbol is a symbol plus call counts, used for editor code lenses.
type FileSymbol struct {
	*analysis.Symbol
	Callers int `json:"callers"`
	Callees int `json:"callees"`
}

// FileSymbols lists a file's symbols with caller and callee counts.
func (s *Service) FileSymbols(ctx context.Context, rt *RepoHandle, file string) ([]FileSymbol, error) {
	if _, _, err := rt.ws.Resolve(file, defaults); err != nil {
		return nil, err
	}
	ix, err := s.Index(ctx, rt)
	if err != nil {
		return nil, err
	}
	var out []FileSymbol
	for _, sym := range ix.SymbolsInFile(file) {
		out = append(out, FileSymbol{Symbol: sym, Callers: countCalls(ix.Callers(sym.ID)), Callees: countCalls(ix.Callees(sym.ID))})
	}
	return out, nil
}

// countCalls counts call-like edges, ignoring imports and implements.
func countCalls(es []analysis.Edge) int {
	n := 0
	for _, e := range es {
		if e.Kind != analysis.EdgeImport && e.Kind != analysis.EdgeImplement {
			n++
		}
	}
	return n
}

type EdgeRef struct {
	Symbol *analysis.Symbol `json:"symbol"`
	Kind   string           `json:"kind"`
	Lines  []int            `json:"lines,omitempty"`
}

type SymbolDetail struct {
	Symbol          *analysis.Symbol   `json:"symbol"`
	Callers         []EdgeRef          `json:"callers"`
	Callees         []EdgeRef          `json:"callees"`
	Implementations []*analysis.Symbol `json:"implementations,omitempty"`
	Implements      []*analysis.Symbol `json:"implements,omitempty"`
	Methods         []*analysis.Symbol `json:"methods,omitempty"`
	Code            string             `json:"code,omitempty"`
	Explanation     *Explanation       `json:"explanation,omitempty"`
}

// ResolveSymbol finds exactly one symbol or explains the ambiguity.
func (s *Service) ResolveSymbol(ctx context.Context, rt *RepoHandle, ref string) (*analysis.Symbol, *analysis.Index, error) {
	ix, err := s.Index(ctx, rt)
	if err != nil {
		return nil, nil, err
	}
	found := ix.Find(ref)
	switch {
	case len(found) == 0:
		return nil, ix, fmt.Errorf("no symbol matches %q; try search_symbols or a file:line reference", ref)
	case len(found) > 1:
		var opts []string
		for i, f := range found {
			if i == 8 {
				opts = append(opts, fmt.Sprintf("…and %d more", len(found)-8))
				break
			}
			opts = append(opts, fmt.Sprintf("%s (%s:%d)", f.ID, f.File, f.Line))
		}
		// Prefer a unique non-abstract match (e.g. concrete method over interface).
		var concrete []*analysis.Symbol
		for _, f := range found {
			if !f.Abstract && f.Kind.IsCallable() {
				concrete = append(concrete, f)
			}
		}
		if len(concrete) == 1 {
			return concrete[0], ix, nil
		}
		return nil, ix, fmt.Errorf("%q is ambiguous; use one of these ids: %s", ref, strings.Join(opts, "; "))
	}
	return found[0], ix, nil
}

// SymbolDetail returns everything the inspector shows for a symbol:
// callers, callees, implementations, methods, optional source and any
// agent explanation.
func (s *Service) SymbolDetail(ctx context.Context, rt *RepoHandle, ref string, withCode bool) (*SymbolDetail, error) {
	sym, ix, err := s.ResolveSymbol(ctx, rt, ref)
	if err != nil {
		return nil, err
	}
	d := &SymbolDetail{Symbol: sym, Callers: []EdgeRef{}, Callees: []EdgeRef{}}
	for _, e := range ix.Callers(sym.ID) {
		switch e.Kind {
		case analysis.EdgeImplement:
			d.Implementations = append(d.Implementations, ix.Symbols[e.From])
		case analysis.EdgeDispatch:
			// Interface method callers are listed on the interface method.
			d.Callers = append(d.Callers, EdgeRef{Symbol: ix.Symbols[e.From], Kind: string(e.Kind)})
		default:
			d.Callers = append(d.Callers, EdgeRef{Symbol: ix.Symbols[e.From], Kind: string(e.Kind), Lines: e.Lines})
		}
	}
	for _, e := range ix.Callees(sym.ID) {
		switch e.Kind {
		case analysis.EdgeImplement:
			d.Implements = append(d.Implements, ix.Symbols[e.To])
		case analysis.EdgeDispatch:
			d.Implementations = append(d.Implementations, ix.Symbols[e.To])
		default:
			d.Callees = append(d.Callees, EdgeRef{Symbol: ix.Symbols[e.To], Kind: string(e.Kind), Lines: e.Lines})
		}
	}
	for _, m := range sym.Methods {
		if ms := ix.Symbols[m]; ms != nil {
			d.Methods = append(d.Methods, ms)
		}
	}
	if withCode && sym.File != "" {
		if fc, err := rt.ws.ReadFile(sym.File, defaults); err == nil && !fc.Binary {
			d.Code = sliceLines(fc.Content, sym.Line, sym.EndLine)
		}
	}
	d.Explanation = s.session.explanationFor(rt.cfg.ID, sym.ID)
	return d, nil
}

// sliceLines returns lines from..to (1-based, inclusive) of content.
func sliceLines(content string, from, to int) string {
	lines := strings.Split(content, "\n")
	from = max(from, 1)
	to = min(max(to, from), len(lines))
	if from > len(lines) {
		return ""
	}
	return strings.Join(lines[from-1:to], "\n")
}

// SearchSymbols fuzzy-searches symbol names for the palette and agents.
func (s *Service) SearchSymbols(ctx context.Context, rt *RepoHandle, q string, limit int) ([]*analysis.Symbol, error) {
	ix, err := s.Index(ctx, rt)
	if err != nil {
		return nil, err
	}
	return ix.Search(q, limit), nil
}

// ---- graphs ----------------------------------------------------------------

// CallGraph resolves ref and returns its call graph.
func (s *Service) CallGraph(ctx context.Context, rt *RepoHandle, ref string, o analysis.CallGraphOptions) (*analysis.Graph, error) {
	sym, ix, err := s.ResolveSymbol(ctx, rt, ref)
	if err != nil {
		return nil, err
	}
	return ix.CallGraph(sym.ID, o), nil
}

// TraceTo resolves ref and returns every caller path leading to it.
func (s *Service) TraceTo(ctx context.Context, rt *RepoHandle, ref string, depth int) (*analysis.Graph, error) {
	sym, ix, err := s.ResolveSymbol(ctx, rt, ref)
	if err != nil {
		return nil, err
	}
	return ix.TraceTo(sym.ID, depth, 0), nil
}

// PackageGraph returns the package dependency graph, optionally focused
// on one package's neighbours and including external modules.
func (s *Service) PackageGraph(ctx context.Context, rt *RepoHandle, external bool, focus string) (*analysis.Graph, error) {
	ix, err := s.Index(ctx, rt)
	if err != nil {
		return nil, err
	}
	return ix.PackageGraph(external, focus), nil
}

// TypeGraph returns the type diagram for a package, directory or file.
func (s *Service) TypeGraph(ctx context.Context, rt *RepoHandle, scope string) (*analysis.Graph, error) {
	ix, err := s.Index(ctx, rt)
	if err != nil {
		return nil, err
	}
	return ix.TypeGraph(scope), nil
}

// FunctionFlow returns the control-flow graph for a Go function, or a
// skeleton (signature only) for heuristic languages.
func (s *Service) FunctionFlow(ctx context.Context, rt *RepoHandle, ref string) (*analysis.Flow, *analysis.Symbol, error) {
	sym, _, err := s.ResolveSymbol(ctx, rt, ref)
	if err != nil {
		return nil, nil, err
	}
	if !sym.Kind.IsCallable() || sym.Kind == analysis.KindExternal {
		return nil, sym, fmt.Errorf("%s is a %s, not a function", sym.Qualified, sym.Kind)
	}
	if sym.Language == "go" && !sym.Abstract {
		fl, err := analysis.GoFuncFlow(rt.cfg.Path, sym.File, sym.Line)
		if err != nil {
			return nil, sym, err
		}
		fl.Symbol = sym.ID
		return fl, sym, nil
	}
	fl := &analysis.Flow{
		Symbol: sym.ID, Name: sym.Qualified, File: sym.File, Line: sym.Line, EndLine: sym.EndLine,
		Signature: sym.Signature, Inputs: sym.Params, Outputs: sym.Results,
		Nodes: []analysis.FlowNode{
			{ID: "n0", Kind: analysis.FlowStart, Label: sym.Name + "()", Line: sym.Line},
			{ID: "n1", Kind: analysis.FlowBlock, Label: "body", Code: []string{"control flow is available for Go; ask your agent to explain this function"}, Line: sym.Line, EndLine: sym.EndLine},
			{ID: "n2", Kind: analysis.FlowEnd, Label: "end", Line: sym.EndLine},
		},
		Edges: []analysis.FlowEdge{{From: "n0", To: "n1"}, {From: "n1", To: "n2"}},
	}
	return fl, sym, nil
}

// ---- git -------------------------------------------------------------------

// filterDiffs drops diffs for paths hidden by the sandbox excludes.
func filterDiffs(rt *RepoHandle, diffs []gitx.FileDiff) []gitx.FileDiff {
	out := diffs[:0]
	for _, d := range diffs {
		if rt.ws.Excluded(d.Path(), defaults) {
			continue
		}
		out = append(out, d)
	}
	return out
}

type ChangedSymbol struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Line  int    `json:"line"`
	Lines int    `json:"lines"` // changed lines inside the symbol
}

type DiffFile struct {
	gitx.FileDiff
	Path     string          `json:"path"`
	Language string          `json:"language"`
	Symbols  []ChangedSymbol `json:"symbols"`
}

type DiffOverview struct {
	Spec      gitx.DiffSpec `json:"spec"`
	Files     []DiffFile    `json:"files"`
	Additions int           `json:"additions"`
	Deletions int           `json:"deletions"`
	Impact    *Impact       `json:"impact,omitempty"`
	Note      string        `json:"note,omitempty"`
}

// changedSymbols maps changed line numbers onto the innermost symbols.
func changedSymbols(ix *analysis.Index, file string, lines []int) []ChangedSymbol {
	counts := map[string]int{}
	for _, l := range lines {
		if sym := ix.SymbolAt(file, l); sym != nil {
			counts[sym.ID]++
		}
	}
	out := []ChangedSymbol{} // never nil: the UI iterates it
	for id, n := range counts {
		sym := ix.Symbols[id]
		out = append(out, ChangedSymbol{ID: id, Name: sym.Qualified, Kind: string(sym.Kind), Line: sym.Line, Lines: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

// Diff computes a git diff, maps changed lines onto the functions they
// touch and, when requested, the blast radius of those changes.
func (s *Service) Diff(ctx context.Context, rt *RepoHandle, spec gitx.DiffSpec, withImpact bool) (*DiffOverview, error) {
	diffs, err := rt.git.Diff(ctx, spec)
	if err != nil {
		return nil, err
	}
	diffs = filterDiffs(rt, diffs)
	ov := &DiffOverview{Spec: spec, Files: []DiffFile{}}
	ix, _ := s.Index(ctx, rt)
	var changed []string
	for _, d := range diffs {
		df := DiffFile{FileDiff: d, Path: d.Path(), Language: workspace.Language(d.Path()), Symbols: []ChangedSymbol{}}
		if ix != nil && d.Status != gitx.StatusDeleted {
			df.Symbols = changedSymbols(ix, df.Path, d.ChangedNewLines())
			for _, cs := range df.Symbols {
				changed = append(changed, cs.ID)
			}
		}
		ov.Additions += d.Additions
		ov.Deletions += d.Deletions
		ov.Files = append(ov.Files, df)
	}
	if spec.Head != "" {
		ov.Note = "symbol mapping uses the working tree, so it is approximate for commit-to-commit diffs"
	}
	if withImpact && ix != nil && len(changed) > 0 {
		ov.Impact = s.impactOf(ctx, rt, ix, changed)
	}
	return ov, nil
}

type FileVersions struct {
	Path     string `json:"path"`
	Original string `json:"original"`
	Modified string `json:"modified"`
	Language string `json:"language"`
}

// FileVersions fetches both sides of a diff for the side-by-side viewer.
func (s *Service) FileVersions(ctx context.Context, rt *RepoHandle, p, oldPath string, spec gitx.DiffSpec) (*FileVersions, error) {
	if _, _, err := rt.ws.Resolve(p, defaults); err != nil {
		return nil, err
	}
	if oldPath == "" {
		oldPath = p
	}
	if spec.Commit != "" {
		base, head, err := rt.git.CommitRange(ctx, spec.Commit)
		if err != nil {
			return nil, err
		}
		spec = gitx.DiffSpec{Base: base, Head: head}
	}
	fv := &FileVersions{Path: p, Language: workspace.Language(p)}
	show := func(ref, file string) string {
		out, err := rt.git.Show(ctx, ref, file)
		if err != nil {
			return ""
		}
		return out
	}
	working := func() string {
		fc, err := rt.ws.ReadFile(p, defaults)
		if err != nil || fc.Binary {
			return ""
		}
		return fc.Content
	}
	switch {
	case spec.Staged:
		fv.Original, fv.Modified = show("HEAD", oldPath), show(":", p)
	case spec.Base != "" && spec.Head != "":
		fv.Original, fv.Modified = show(spec.Base, oldPath), show(spec.Head, p)
	case spec.Base != "":
		fv.Original, fv.Modified = show(spec.Base, oldPath), working()
	default:
		fv.Original, fv.Modified = show("HEAD", oldPath), working()
	}
	return fv, nil
}

// GitLog returns recent commits, hiding excluded files from each commit.
func (s *Service) GitLog(ctx context.Context, rt *RepoHandle, n int, ref, p string) ([]gitx.Commit, error) {
	commits, err := rt.git.Log(ctx, n, ref, p)
	if err != nil {
		return nil, err
	}
	for i := range commits {
		files := commits[i].Files[:0]
		for _, f := range commits[i].Files {
			if !rt.ws.Excluded(f.Path, defaults) {
				files = append(files, f)
			}
		}
		commits[i].Files = files
	}
	return commits, nil
}

// ---- heatmaps --------------------------------------------------------------

type Heatmap struct {
	Metric  string               `json:"metric"`
	Label   string               `json:"label"`
	Tree    *workspace.TreeNode  `json:"tree"`
	Values  map[string]float64   `json:"values"`
	Details map[string]string    `json:"details,omitempty"`
	Max     float64              `json:"max"`
	Total   float64              `json:"total"`
	Symbols map[string][]SymHeat `json:"symbols,omitempty"`
}

type SymHeat struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Line  int     `json:"line"`
	Value float64 `json:"value"`
}

var HeatMetrics = map[string]string{
	"churn":      "Commits touching each file",
	"changes":    "Lines changed in the working tree",
	"proposals":  "Lines changed by pending agent proposals",
	"complexity": "Total cyclomatic complexity",
	"agent":      "Agent attention this session",
	"size":       "Lines of code",
	"coverage":   "Lines not covered by tests",
}

var treeCache sync.Map // repo id -> cachedTree

type cachedTree struct {
	at   time.Time
	tree *workspace.TreeNode
}

// linesTree returns the file tree with line counts, cached briefly because
// counting lines reads every file.
func (s *Service) linesTree(rt *RepoHandle) (*workspace.TreeNode, error) {
	if v, ok := treeCache.Load(rt.cfg.ID); ok {
		if c := v.(cachedTree); time.Since(c.at) < 10*time.Second {
			return c.tree, nil
		}
	}
	t, err := rt.ws.Tree(defaults, true)
	if err != nil {
		return nil, err
	}
	treeCache.Store(rt.cfg.ID, cachedTree{time.Now(), t})
	return t, nil
}

// Heatmap computes a per-file (and per-function, where meaningful) metric
// over the tree: churn, changes, proposals, complexity, agent or size.
func (s *Service) Heatmap(ctx context.Context, rt *RepoHandle, metric, proposalID string) (*Heatmap, error) {
	if _, ok := HeatMetrics[metric]; !ok {
		metric = "churn"
	}
	tree, err := s.linesTree(rt)
	if err != nil {
		return nil, err
	}
	hm := &Heatmap{Metric: metric, Label: HeatMetrics[metric], Tree: tree, Values: map[string]float64{}, Details: map[string]string{}, Symbols: map[string][]SymHeat{}}
	ix, _ := s.Index(ctx, rt)
	addSym := func(file string, sym *analysis.Symbol, v float64) {
		for i, sh := range hm.Symbols[file] {
			if sh.ID == sym.ID {
				hm.Symbols[file][i].Value += v
				return
			}
		}
		hm.Symbols[file] = append(hm.Symbols[file], SymHeat{ID: sym.ID, Name: sym.Qualified, Line: sym.Line, Value: v})
	}
	switch metric {
	case "churn":
		cfg := s.Config.Get()
		churn, err := rt.git.Churn(ctx, cfg.Settings.ChurnDays)
		if err != nil && !errors.Is(err, gitx.ErrNotRepo) {
			return nil, err
		}
		for p, c := range churn {
			hm.Values[p] = float64(c.Commits)
			hm.Details[p] = fmt.Sprintf("%d commits · +%d −%d · %s", c.Commits, c.Additions, c.Deletions, strings.Join(c.Authors, ", "))
		}
	case "changes":
		ov, err := s.Diff(ctx, rt, gitx.DiffSpec{}, false)
		if err != nil {
			return nil, err
		}
		for _, f := range ov.Files {
			hm.Values[f.Path] = float64(f.Additions + f.Deletions)
			hm.Details[f.Path] = fmt.Sprintf("%s · +%d −%d", f.Status, f.Additions, f.Deletions)
			for _, cs := range f.Symbols {
				hm.Symbols[f.Path] = append(hm.Symbols[f.Path], SymHeat{ID: cs.ID, Name: cs.Name, Line: cs.Line, Value: float64(cs.Lines)})
			}
		}
	case "proposals":
		for _, p := range s.Proposals.List(rt.cfg.ID) {
			if proposalID != "" && p.ID != proposalID {
				continue
			}
			if proposalID == "" && p.Status != "pending" && p.Status != "partial" {
				continue
			}
			for _, c := range p.Changes {
				hm.Values[c.Path] += float64(c.Additions + c.Deletions)
				hm.Details[c.Path] = fmt.Sprintf("%s · +%d −%d · %s", c.Action, c.Additions, c.Deletions, p.Title)
				if ix != nil {
					for _, id := range c.Symbols {
						if sym := ix.Symbols[id]; sym != nil {
							addSym(c.Path, sym, 1)
						}
					}
				}
			}
		}
	case "complexity":
		if ix != nil {
			for file := range ix.ByFile {
				total := 0
				for _, sym := range ix.SymbolsInFile(file) {
					if sym.Complexity > 0 {
						total += sym.Complexity
						addSym(file, sym, float64(sym.Complexity))
					}
				}
				if total > 0 {
					hm.Values[file] = float64(total)
					hm.Details[file] = fmt.Sprintf("complexity %d across %d symbols", total, len(ix.ByFile[file]))
				}
			}
		}
	case "agent":
		for file, n := range s.session.attention(rt.cfg.ID) {
			hm.Values[file] = float64(n)
			hm.Details[file] = fmt.Sprintf("agent looked here %d×", n)
		}
	case "coverage":
		report, _ := s.coverage(rt)
		if report == nil {
			break
		}
		for p, fc := range report.Files {
			c, t := fc.Counts()
			if t == 0 {
				continue
			}
			hm.Values[p] = float64(t - c)
			hm.Details[p] = fmt.Sprintf("%.0f%% covered · %d of %d lines not run by tests", 100*float64(c)/float64(t), t-c, t)
			if ix != nil {
				for _, sym := range ix.SymbolsInFile(p) {
					if !sym.Kind.IsCallable() {
						continue
					}
					if cv, tot := fc.CountsIn(sym.Line, sym.EndLine); tot > cv {
						addSym(p, sym, float64(tot-cv))
					}
				}
			}
		}
	case "size":
		var walk func(n *workspace.TreeNode)
		walk = func(n *workspace.TreeNode) {
			if !n.Dir {
				hm.Values[n.Path] = float64(n.Lines)
				return
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(tree)
	}
	for file, list := range hm.Symbols {
		sort.Slice(list, func(i, j int) bool { return list[i].Value > list[j].Value })
		hm.Symbols[file] = list
	}
	for _, v := range hm.Values {
		hm.Total += v
		hm.Max = max(hm.Max, v)
	}
	return hm, nil
}

// ---- overview --------------------------------------------------------------

type Overview struct {
	Repo        config.Repo        `json:"repo"`
	Git         gitx.Info          `json:"git"`
	Stats       analysis.Stats     `json:"stats"`
	TopDirs     []DirSummary       `json:"topDirs"`
	EntryPoints []*analysis.Symbol `json:"entryPoints"`
	Hotspots    []Hotspot          `json:"hotspots"`
	Readme      string             `json:"readme,omitempty"`
}

type DirSummary struct {
	Path      string         `json:"path"`
	Files     int            `json:"files"`
	Lines     int            `json:"lines"`
	Languages map[string]int `json:"languages"`
}

type Hotspot struct {
	Path       string  `json:"path"`
	Churn      int     `json:"churn"`
	Complexity int     `json:"complexity"`
	Score      float64 `json:"score"`
}

// Overview summarizes a repository for the dashboard and repo_overview:
// stats, top-level directories, entry points, hotspots and the README.
func (s *Service) Overview(ctx context.Context, rt *RepoHandle) (*Overview, error) {
	ov := &Overview{Repo: rt.cfg, TopDirs: []DirSummary{}, EntryPoints: []*analysis.Symbol{}, Hotspots: []Hotspot{}}
	ov.Git, _ = rt.git.Info(ctx)
	ix, err := s.Index(ctx, rt)
	if err != nil {
		return nil, err
	}
	ov.Stats = ix.Stats()
	tree, err := s.linesTree(rt)
	if err != nil {
		return nil, err
	}
	for _, c := range tree.Children {
		ds := DirSummary{Path: c.Path, Files: max(c.Files, 1), Lines: c.Lines, Languages: map[string]int{}}
		if !c.Dir {
			ds.Files = 1
		}
		var walk func(n *workspace.TreeNode)
		walk = func(n *workspace.TreeNode) {
			if !n.Dir {
				if workspace.IsCode(n.Language) {
					ds.Languages[n.Language]++
				}
				return
			}
			for _, k := range n.Children {
				walk(k)
			}
		}
		walk(c)
		ov.TopDirs = append(ov.TopDirs, ds)
	}
	for _, sym := range ix.Symbols {
		if !sym.Kind.IsCallable() || sym.Kind == analysis.KindExternal || sym.Abstract {
			continue
		}
		isMain := sym.Name == "main" && sym.Kind == analysis.KindFunc
		if isMain || (len(ix.Callers(sym.ID)) == 0 && sym.Exported && len(ix.Callees(sym.ID)) >= 3) {
			ov.EntryPoints = append(ov.EntryPoints, sym)
		}
	}
	sort.Slice(ov.EntryPoints, func(i, j int) bool {
		a, b := ov.EntryPoints[i], ov.EntryPoints[j]
		if (a.Name == "main") != (b.Name == "main") {
			return a.Name == "main"
		}
		return len(ix.Callees(a.ID)) > len(ix.Callees(b.ID))
	})
	if len(ov.EntryPoints) > 15 {
		ov.EntryPoints = ov.EntryPoints[:15]
	}
	churn, _ := rt.git.Churn(ctx, s.Config.Get().Settings.ChurnDays)
	for file, ids := range ix.ByFile {
		cx := 0
		for _, id := range ids {
			cx += ix.Symbols[id].Complexity
		}
		ch := 0
		if c := churn[file]; c != nil {
			ch = c.Commits
		}
		if ch == 0 || cx == 0 {
			continue
		}
		ov.Hotspots = append(ov.Hotspots, Hotspot{Path: file, Churn: ch, Complexity: cx, Score: float64(ch) * float64(cx)})
	}
	sort.Slice(ov.Hotspots, func(i, j int) bool { return ov.Hotspots[i].Score > ov.Hotspots[j].Score })
	if len(ov.Hotspots) > 10 {
		ov.Hotspots = ov.Hotspots[:10]
	}
	for _, name := range []string{"README.md", "README", "readme.md", "README.rst"} {
		if fc, err := rt.ws.ReadFile(name, defaults); err == nil && !fc.Binary {
			txt := fc.Content
			if len(txt) > 4000 {
				txt = txt[:4000] + "\n…"
			}
			ov.Readme = txt
			break
		}
	}
	return ov, nil
}

// FileSummary is everything declared in one file, for the summary panel.
type FileSummary struct {
	Path      string              `json:"path"`
	Language  string              `json:"language"`
	Lines     int                 `json:"lines"`
	Functions []FileSymbol        `json:"functions"`
	Types     []FileSymbol        `json:"types"`
	Variables []analysis.Variable `json:"variables"`
	Imports   []string            `json:"imports"`
	Heuristic bool                `json:"heuristic"`
}

// FileSummary lists everything declared in one file: functions, types,
// top-level variables and constants, and imports.
func (s *Service) FileSummary(ctx context.Context, rt *RepoHandle, file string) (*FileSummary, error) {
	fc, err := rt.ws.ReadFile(file, defaults)
	if err != nil {
		return nil, err
	}
	syms, err := s.FileSymbols(ctx, rt, fc.Path)
	if err != nil {
		return nil, err
	}
	fs := &FileSummary{Path: fc.Path, Language: fc.Language, Lines: fc.Lines, Functions: []FileSymbol{}, Types: []FileSymbol{}, Variables: []analysis.Variable{}, Imports: []string{}}
	for _, sym := range syms {
		if sym.Kind.IsCallable() {
			fs.Functions = append(fs.Functions, sym)
		} else {
			fs.Types = append(fs.Types, sym)
		}
		fs.Heuristic = fs.Heuristic || sym.Heuristic
	}
	if !fc.Binary && workspace.IsCode(fc.Language) {
		if o, err := analysis.FileOutline(rt.cfg.Path, fc.Path, fc.Language); err == nil {
			fs.Variables, fs.Imports = o.Variables, o.Imports
		}
	}
	return fs, nil
}
