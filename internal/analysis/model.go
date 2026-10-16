// Package analysis builds a language-aware index of a repository: symbols,
// call edges, package dependencies and per-function control flow. Go is
// analyzed precisely with go/types; other languages use a lexical heuristic
// analyzer whose results are flagged as such.
package analysis

import (
	"sort"
	"time"
)

type SymbolKind string

const (
	KindFunc      SymbolKind = "func"
	KindMethod    SymbolKind = "method"
	KindClass     SymbolKind = "class"
	KindStruct    SymbolKind = "struct"
	KindInterface SymbolKind = "interface"
	KindType      SymbolKind = "type"
	KindExternal  SymbolKind = "external"
)

// IsCallable reports whether a symbol kind participates in call graphs.
func (k SymbolKind) IsCallable() bool { return k == KindFunc || k == KindMethod || k == KindExternal }

type Param struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Symbol struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Qualified  string     `json:"qualified"`
	Kind       SymbolKind `json:"kind"`
	Language   string     `json:"language"`
	File       string     `json:"file,omitempty"`
	Line       int        `json:"line,omitempty"`
	EndLine    int        `json:"endLine,omitempty"`
	Package    string     `json:"package,omitempty"`
	Receiver   string     `json:"receiver,omitempty"`
	Parent     string     `json:"parent,omitempty"` // owning type/class symbol ID
	Signature  string     `json:"signature,omitempty"`
	Params     []Param    `json:"params,omitempty"`
	Results    []Param    `json:"results,omitempty"`
	Fields     []Param    `json:"fields,omitempty"`
	Methods    []string   `json:"methods,omitempty"`
	Implements []string   `json:"implements,omitempty"`
	Doc        string     `json:"doc,omitempty"`
	Exported   bool       `json:"exported"`
	Complexity int        `json:"complexity,omitempty"`
	Abstract   bool       `json:"abstract,omitempty"` // interface method
	Heuristic  bool       `json:"heuristic,omitempty"`
}

type EdgeKind string

const (
	EdgeCall      EdgeKind = "call"
	EdgeGo        EdgeKind = "go"
	EdgeDefer     EdgeKind = "defer"
	EdgeRef       EdgeKind = "ref"        // function referenced as a value (handlers, callbacks)
	EdgeDispatch  EdgeKind = "dispatch"   // interface method -> concrete implementation
	EdgeImport    EdgeKind = "import"     // package/file dependency
	EdgeImplement EdgeKind = "implements" // type -> interface
)

type Edge struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Kind      EdgeKind `json:"kind"`
	Lines     []int    `json:"lines,omitempty"`
	Heuristic bool     `json:"heuristic,omitempty"`
}

type Package struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Dir      string   `json:"dir"`
	Language string   `json:"language"`
	Files    []string `json:"files"`
	Imports  []string `json:"imports"` // every import, internal or not
	Internal bool     `json:"internal"`
}

// Index is an immutable snapshot of a repository's structure.
type Index struct {
	Root      string              `json:"root"`
	Symbols   map[string]*Symbol  `json:"-"`
	ByFile    map[string][]string `json:"-"`
	Edges     []Edge              `json:"-"`
	Packages  map[string]*Package `json:"-"`
	PkgEdges  []Edge              `json:"-"`
	Languages map[string]int      `json:"languages"`
	Errors    []string            `json:"errors,omitempty"`
	BuiltAt   time.Time           `json:"builtAt"`
	Duration  time.Duration       `json:"duration"`

	callees map[string][]int
	callers map[string][]int
}

// newIndex creates an empty index for the repository at root.
func newIndex(root string) *Index {
	return &Index{
		Root:      root,
		Symbols:   map[string]*Symbol{},
		ByFile:    map[string][]string{},
		Packages:  map[string]*Package{},
		Languages: map[string]int{},
	}
}

// addSymbol registers a symbol and its file position. The first symbol
// with a given ID wins; duplicates are ignored.
func (ix *Index) addSymbol(s *Symbol) {
	if _, dup := ix.Symbols[s.ID]; dup {
		return
	}
	ix.Symbols[s.ID] = s
	if s.File != "" {
		ix.ByFile[s.File] = append(ix.ByFile[s.File], s.ID)
	}
}

// addEdge merges repeated edges of the same kind, accumulating call lines.
func (ix *Index) addEdge(e Edge, merge map[[3]string]int) {
	key := [3]string{e.From, e.To, string(e.Kind)}
	if i, ok := merge[key]; ok {
		ix.Edges[i].Lines = append(ix.Edges[i].Lines, e.Lines...)
		return
	}
	merge[key] = len(ix.Edges)
	ix.Edges = append(ix.Edges, e)
}

// finalize sorts per-file symbols and builds adjacency lists.
func (ix *Index) finalize() {
	for f, ids := range ix.ByFile {
		sort.Slice(ids, func(i, j int) bool {
			a, b := ix.Symbols[ids[i]], ix.Symbols[ids[j]]
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			return a.EndLine > b.EndLine
		})
		ix.ByFile[f] = ids
	}
	ix.callees = map[string][]int{}
	ix.callers = map[string][]int{}
	for i, e := range ix.Edges {
		ix.callees[e.From] = append(ix.callees[e.From], i)
		ix.callers[e.To] = append(ix.callers[e.To], i)
	}
}

// Callees returns the outgoing edges of a symbol (what it calls,
// references or dispatches to).
func (ix *Index) Callees(id string) []Edge { return ix.pick(ix.callees[id]) }

// Callers returns the incoming edges of a symbol (what calls or
// references it).
func (ix *Index) Callers(id string) []Edge { return ix.pick(ix.callers[id]) }

// pick resolves edge indexes from an adjacency list into edges.
func (ix *Index) pick(idx []int) []Edge {
	out := make([]Edge, len(idx))
	for i, j := range idx {
		out[i] = ix.Edges[j]
	}
	return out
}

// SymbolsInFile returns the file's symbols ordered by position.
func (ix *Index) SymbolsInFile(file string) []*Symbol {
	ids := ix.ByFile[file]
	out := make([]*Symbol, 0, len(ids))
	for _, id := range ids {
		out = append(out, ix.Symbols[id])
	}
	return out
}

// SymbolAt returns the innermost symbol whose range contains line. Callable
// symbols win over the types that contain them.
func (ix *Index) SymbolAt(file string, line int) *Symbol {
	var best *Symbol
	for _, s := range ix.SymbolsInFile(file) {
		if s.Line <= line && line <= max(s.EndLine, s.Line) {
			if best == nil || (s.EndLine-s.Line) <= (best.EndLine-best.Line) {
				best = s
			}
		}
	}
	return best
}

// Stats summarizes the index for overviews.
type Stats struct {
	Files      int            `json:"files"`
	Symbols    int            `json:"symbols"`
	Functions  int            `json:"functions"`
	Types      int            `json:"types"`
	Edges      int            `json:"edges"`
	Packages   int            `json:"packages"`
	Languages  map[string]int `json:"languages"`
	Errors     int            `json:"errors"`
	BuiltAt    time.Time      `json:"builtAt"`
	DurationMS int64          `json:"durationMs"`
}

// Stats summarizes the index: file, symbol, edge and package counts,
// languages and build time.
func (ix *Index) Stats() Stats {
	st := Stats{Files: len(ix.ByFile), Edges: len(ix.Edges), Languages: ix.Languages, Errors: len(ix.Errors), BuiltAt: ix.BuiltAt, DurationMS: ix.Duration.Milliseconds()}
	for _, s := range ix.Symbols {
		if s.Kind == KindExternal {
			continue
		}
		st.Symbols++
		if s.Kind.IsCallable() {
			st.Functions++
		} else {
			st.Types++
		}
	}
	for _, p := range ix.Packages {
		if p.Internal {
			st.Packages++
		}
	}
	return st
}
