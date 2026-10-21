package core

import (
	"context"
	"math"
	"os"
	"path"
	"sort"
	"time"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/proposals"
	"github.com/aitakemyjob/codediver/internal/qa"
)

// ---- blast radius --------------------------------------------------------------
//
// The blast radius of a change is everything that calls the changed code,
// directly or through other calls. It is shown distance first: direct
// callers are the likeliest to break and are listed in full, callers
// further out are grouped by package, and the far end is summed up as the
// entry points the change reaches. Every caller carries its path back to
// the change and, when the QA analysis has it, its test status. Lists are
// capped so a change to a widely used helper stays small to send and draw.

// impactDepth is how many calls away from the change the radius reaches.
const impactDepth = 3

// ringCap bounds the callers listed per distance; the rest are counted.
var ringCap = [impactDepth + 1]int{0, 150, 80, 80}

const entryCap = 25

// Test status of a blast-radius node, strongest evidence first. Measured
// coverage always decides over the static test mapping.
const (
	StatusPath    = "path"    // a test ran its call toward the change
	StatusCaller  = "caller"  // it ran, but not its call toward the change
	StatusNotRun  = "notrun"  // coverage shows none of its lines ran
	StatusLikely  = "likely"  // not measured; a test calls it directly
	StatusUnknown = "unknown" // not measured, and no test calls it directly
)

// statusRank orders statuses for review, weakest first.
var statusRank = map[string]int{StatusNotRun: 0, StatusCaller: 1, StatusUnknown: 2, StatusLikely: 3, StatusPath: 4}

// ImpactNode is a changed symbol or one of its callers.
type ImpactNode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Package string `json:"package"`
	Depth   int    `json:"depth"`
	// Via is the symbol this one calls on its way to the change.
	Via   string `json:"via,omitempty"`
	ViaID string `json:"viaId,omitempty"`
	// Pct is line coverage, -1 without coverage data.
	Pct float64 `json:"pct"`
	// Direct and Indirect count tests that call it, or might reach it
	// through other calls, by the static test mapping.
	Direct   int `json:"direct"`
	Indirect int `json:"indirect"`
	// Status is the test status (StatusPath…), empty for types, which
	// tests don't run. Measured says it comes from coverage rather than the
	// static mapping; Stale that the coverage is older than the file.
	Status   string `json:"status,omitempty"`
	Measured bool   `json:"measured"`
	Stale    bool   `json:"stale,omitempty"`
	// Tests names per-test runs that ran its call toward the change.
	Tests []string `json:"tests,omitempty"`
	// Tested means a test runs the path or, unmeasured, calls it directly.
	Tested bool `json:"tested"`
	// Review marks direct callers no test is known to run the path of.
	Review    bool `json:"review,omitempty"`
	Heuristic bool `json:"heuristic,omitempty"`
	// Entry says why a symbol with no callers is the top of a call path:
	// main, exported or uncalled.
	Entry string `json:"entry,omitempty"`
}

// ImpactPackage counts a ring's callers in one package.
type ImpactPackage struct {
	Path   string `json:"path"`
	Count  int    `json:"count"`
	Tested int    `json:"tested"`
}

// ImpactRing is the callers at one distance from the change.
type ImpactRing struct {
	Depth  int `json:"depth"`
	Total  int `json:"total"`
	Tested int `json:"tested"`
	// Statuses counts callers by test status.
	Statuses map[string]int `json:"statuses"`
	// AvgPct is the mean coverage of callers with coverage data, or -1.
	AvgPct   float64         `json:"avgPct"`
	Packages []ImpactPackage `json:"packages"`
	Nodes    []ImpactNode    `json:"nodes"`
	// More counts callers left out of Nodes.
	More int `json:"more,omitempty"`
}

// ImpactEdge links two packages in the blast radius: Count callers in From
// call toward the change through To.
type ImpactEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// Impact is the blast radius of a change.
type Impact struct {
	Changed []ImpactNode `json:"changed"`
	Rings   []ImpactRing `json:"rings"`
	// Entries are callers nothing else calls: where the change surfaces.
	Entries []ImpactNode `json:"entries"`
	// Beyond counts paths that continue past the outermost ring.
	Beyond    int  `json:"beyond,omitempty"`
	Heuristic int  `json:"heuristic,omitempty"`
	Tests     bool `json:"tests"`    // tests were found, so Tested means something
	Coverage  bool `json:"coverage"` // a coverage report exists
	// PackageEdges and PackageDepth are the radius as a package graph, for
	// the on-demand view.
	PackageEdges []ImpactEdge            `json:"packageEdges"`
	PackageDepth map[string]int          `json:"packageDepth"`
	Summary      proposals.ImpactSummary `json:"summary"`
}

// impactOf computes the blast radius of changed symbols. The QA analysis
// supplies test status; without it nodes carry no test information.
func (s *Service) impactOf(ctx context.Context, rt *RepoHandle, ix *analysis.Index, changed []string) *Impact {
	a, _ := s.qaAnalyze(ctx, rt)
	r := ix.Reach(changed, impactDepth, a.isTestCode)
	return buildImpact(ix, r, a, s.impactCoverage(rt, a))
}

// impactCoverage is the measured coverage a blast radius is judged by.
type impactCoverage struct {
	report  *qa.Report
	perTest map[string]*testCoverage
	// mtime is a file's modification time, zero when unknown.
	mtime func(file string) time.Time
}

// impactCoverage gathers the repository's coverage and per-test results.
func (s *Service) impactCoverage(rt *RepoHandle, a *qaAnalysis) *impactCoverage {
	cv := &impactCoverage{perTest: s.perTest(rt)}
	if a != nil {
		cv.report = a.report
	}
	times := map[string]time.Time{}
	cv.mtime = func(file string) time.Time {
		t, ok := times[file]
		if !ok {
			if abs, _, err := rt.ws.Resolve(file, defaults); err == nil {
				if st, err := os.Stat(abs); err == nil {
					t = st.ModTime()
				}
			}
			times[file] = t
		}
		return t
	}
	return cv
}

// status judges whether tests run sym's calls toward the change, made at
// lines sites. The changed code itself has no sites and is judged whole.
// Per-test runs newer than the file name the tests; otherwise the
// coverage report decides; without either, the static mapping estimates.
func (cv *impactCoverage) status(sym *analysis.Symbol, sites []int, f *FuncQA) (n ImpactNode) {
	n.Status = StatusUnknown
	if f != nil && f.Direct > 0 {
		n.Status = StatusLikely
	}
	if cv == nil {
		return n
	}
	end := max(sym.EndLine, sym.Line)
	fc := cv.report.File(sym.File)
	mtime := cv.mtime(sym.File)
	// The lines to look for: where a call statement is instrumented, or
	// the whole function.
	var want []int
	if len(sites) > 0 {
		want = siteLines(fc, sites, sym.Line)
	} else {
		for ln := sym.Line; ln <= end; ln++ {
			want = append(want, ln)
		}
	}
	for _, tc := range cv.perTest {
		if !tc.Ran.After(mtime) {
			continue
		}
		for _, ln := range want {
			if tc.covers(sym.File, ln) {
				n.Tests = append(n.Tests, tc.Name)
				break
			}
		}
	}
	if len(n.Tests) > 0 {
		sort.Strings(n.Tests)
		n.Tests = n.Tests[:min(len(n.Tests), 5)]
		n.Status, n.Measured = StatusPath, true
		return n
	}
	if fc == nil {
		return n
	}
	covered, lines := fc.CountsIn(sym.Line, end)
	if lines == 0 {
		return n
	}
	n.Measured = true
	n.Stale = !mtime.IsZero() && mtime.After(cv.report.Generated.Add(time.Second))
	switch {
	case covered == 0:
		n.Status = StatusNotRun
	case len(sites) == 0 || len(want) == 0:
		// The changed code, or a call coverage doesn't instrument: the
		// function running is the best measure there is.
		n.Status = StatusPath
	default:
		n.Status = StatusCaller
		for _, ln := range want {
			if fc.Hits[ln] > 0 {
				n.Status = StatusPath
				break
			}
		}
	}
	return n
}

// siteLines maps call sites to the lines coverage instruments for them:
// the site itself, or the nearest instrumented line above it within the
// function (where a statement spanning lines starts). Sites without one
// are dropped; without coverage the sites are returned as they are.
func siteLines(fc *qa.FileCoverage, sites []int, from int) []int {
	if fc == nil {
		return sites
	}
	var out []int
	for _, s := range sites {
		for ln := s; ln >= from && ln > s-5; ln-- {
			if _, ok := fc.Hits[ln]; ok {
				out = append(out, ln)
				break
			}
		}
	}
	return out
}

// callSites are the lines where id calls a symbol one step closer to the
// change.
func callSites(ix *analysis.Index, r *analysis.Reach, id string, d int) []int {
	var out []int
	for _, e := range ix.Callees(id) {
		if e.Kind == analysis.EdgeImport || e.Kind == analysis.EdgeImplement {
			continue
		}
		if dt, ok := r.Dist[e.To]; ok && dt == d-1 {
			out = append(out, e.Lines...)
		}
	}
	return uniqueSorted(out)
}

// buildImpact shapes a reach into rings, entries and the package graph.
func buildImpact(ix *analysis.Index, r *analysis.Reach, a *qaAnalysis, cv *impactCoverage) *Impact {
	im := &Impact{Changed: []ImpactNode{}, Rings: []ImpactRing{}, Entries: []ImpactNode{}, PackageEdges: []ImpactEdge{},
		PackageDepth: map[string]int{}, Beyond: r.Beyond}
	if a != nil {
		im.Tests, im.Coverage = len(a.tests) > 0, a.report != nil
	}
	node := func(id string, d int) ImpactNode {
		sym := ix.Symbols[id]
		n := ImpactNode{ID: id, Name: sym.Qualified, Kind: string(sym.Kind), File: sym.File, Line: sym.Line, Package: pkgOf(sym), Depth: d, Pct: -1, Heuristic: sym.Heuristic}
		if via := r.Via[id]; via != "" {
			n.ViaID, n.Via = via, ix.Symbols[via].Qualified
		}
		var f *FuncQA
		if a != nil {
			if f = a.byID[id]; f != nil {
				n.Pct, n.Direct, n.Indirect = f.Pct, f.Direct, f.Indirect
			}
		}
		// Only code that runs has a test status: a changed struct or
		// interface is neither tested nor untested.
		if sym.Kind.IsCallable() {
			var sites []int
			if d > 0 {
				sites = callSites(ix, r, id, d)
			}
			st := cv.status(sym, sites, f)
			n.Status, n.Measured, n.Stale, n.Tests = st.Status, st.Measured, st.Stale, st.Tests
			n.Tested = n.Status == StatusPath || n.Status == StatusLikely
			n.Review = d == 1 && !n.Tested && (im.Tests || im.Coverage)
		}
		if !ix.HasCallers(id) {
			switch {
			case sym.Name == "main" && sym.Kind == analysis.KindFunc:
				n.Entry = "main"
			case sym.Exported:
				n.Entry = "exported"
			default:
				n.Entry = "uncalled"
			}
		}
		return n
	}

	byDepth := make([][]ImpactNode, impactDepth+1)
	files, pkgs := map[string]bool{}, map[string]bool{}
	edges := map[[2]string]int{}
	for id, d := range r.Dist {
		n := node(id, d)
		byDepth[d] = append(byDepth[d], n)
		files[n.File], pkgs[n.Package] = true, true
		if old, ok := im.PackageDepth[n.Package]; !ok || d < old {
			im.PackageDepth[n.Package] = d
		}
		if n.Heuristic {
			im.Heuristic++
		}
		if d > 0 {
			if n.Entry != "" {
				im.Entries = append(im.Entries, n)
			}
			if to := pkgOf(ix.Symbols[n.ViaID]); to != n.Package {
				edges[[2]string{n.Package, to}]++
			}
		}
	}
	for d := range byDepth {
		sortImpact(byDepth[d])
	}
	im.Changed = byDepth[0]
	if im.Changed == nil {
		im.Changed = []ImpactNode{}
	}
	im.Summary = proposals.ImpactSummary{Changed: len(im.Changed), Callers: make([]int, impactDepth), Files: len(files), Packages: len(pkgs), Entries: len(im.Entries)}
	for d := 1; d <= impactDepth; d++ {
		nodes := byDepth[d]
		ring := ImpactRing{Depth: d, Total: len(nodes), AvgPct: -1, Packages: []ImpactPackage{}, Statuses: map[string]int{}}
		perPkg := map[string]*ImpactPackage{}
		sum, withPct := 0.0, 0
		for _, n := range nodes {
			p := perPkg[n.Package]
			if p == nil {
				p = &ImpactPackage{Path: n.Package}
				perPkg[n.Package] = p
			}
			p.Count++
			if n.Status != "" {
				ring.Statuses[n.Status]++
			}
			if n.Tested {
				p.Tested++
				ring.Tested++
			}
			if n.Pct >= 0 {
				sum += n.Pct
				withPct++
			}
			if n.Review {
				im.Summary.Untested++
			}
		}
		if withPct > 0 {
			ring.AvgPct = math.Round(10*sum/float64(withPct)) / 10
		}
		for _, p := range perPkg {
			ring.Packages = append(ring.Packages, *p)
		}
		sort.Slice(ring.Packages, func(i, j int) bool {
			if ring.Packages[i].Count != ring.Packages[j].Count {
				return ring.Packages[i].Count > ring.Packages[j].Count
			}
			return ring.Packages[i].Path < ring.Packages[j].Path
		})
		ring.Nodes = nodes[:min(len(nodes), ringCap[d])]
		if ring.Nodes == nil {
			ring.Nodes = []ImpactNode{}
		}
		ring.More = len(nodes) - len(ring.Nodes)
		im.Summary.Callers[d-1] = len(nodes)
		if len(nodes) > 0 {
			im.Rings = append(im.Rings, ring)
		}
	}
	// Entries closest to the change first: they are the likeliest to show it.
	sort.SliceStable(im.Entries, func(i, j int) bool { return im.Entries[i].Depth < im.Entries[j].Depth })
	im.Entries = im.Entries[:min(len(im.Entries), entryCap)]
	for k, n := range edges {
		im.PackageEdges = append(im.PackageEdges, ImpactEdge{From: k[0], To: k[1], Count: n})
	}
	sort.Slice(im.PackageEdges, func(i, j int) bool {
		a, b := im.PackageEdges[i], im.PackageEdges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})
	return im
}

// sortImpact orders a ring for review: direct callers to review first,
// then by test status weakest first, the least covered, then by name.
func sortImpact(ns []ImpactNode) {
	sort.Slice(ns, func(i, j int) bool {
		a, b := ns[i], ns[j]
		if a.Review != b.Review {
			return a.Review
		}
		if ra, rb := statusRank[a.Status], statusRank[b.Status]; ra != rb {
			return ra < rb
		}
		if a.Pct != b.Pct {
			return a.Pct < b.Pct
		}
		return a.Name < b.Name
	})
}

// pkgOf is the directory a symbol lives in, the grouping used across
// languages.
func pkgOf(sym *analysis.Symbol) string {
	if sym == nil || sym.File == "" {
		return ""
	}
	return path.Dir(sym.File)
}
