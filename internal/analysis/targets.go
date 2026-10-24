package analysis

// Choosing which definition a lexical call reaches is a chain of
// strategies, most precise first. Each one either decides (possibly that
// there is no edge) or passes the call on to the next.

// callSite is one call found in a function body: `recv.name(` or `name(`.
type callSite struct {
	from *Symbol
	file *genericFile
	recv string
	name string
}

// targetFinder is one strategy for resolving a call. handled reports that
// it decided; target may then be nil, meaning no edge.
type targetFinder interface {
	find(c callSite) (target *Symbol, handled bool)
}

// finderChain asks each finder in turn until one decides.
type finderChain []targetFinder

// find returns the first decision in the chain, or nil when none decides.
func (fc finderChain) find(c callSite) *Symbol {
	for _, f := range fc {
		if t, ok := f.find(c); ok {
			return t
		}
	}
	return nil
}

// newFinderChain builds the strategies for the heuristic analyzer: super
// calls, then calls through imports, then name matching.
func newFinderChain(byName map[string][]*Symbol, modules *moduleGraph) finderChain {
	return finderChain{superFinder{byName}, importFinder{modules}, nameFinder{byName}}
}

// superFinder resolves `super.name()` to the parent class's method, when
// the parent is in the repository; never to the caller's own.
type superFinder struct{ byName map[string][]*Symbol }

// find decides every super call.
func (f superFinder) find(c callSite) (*Symbol, bool) {
	if c.recv != "super" {
		return nil, false
	}
	return superTarget(c.file.extends[c.from.Parent], f.byName[c.name]), true
}

// importFinder resolves calls through the calling file's imports:
// `format()` after `import { format } from './fmt'`, aliases, default and
// namespace imports, re-exports and require.
type importFinder struct{ modules *moduleGraph }

// find decides calls whose name or receiver is bound by an import that
// leads to a definition in the repository.
func (f importFinder) find(c callSite) (*Symbol, bool) {
	if c.recv == "this" || c.recv == "self" {
		return nil, false
	}
	return f.modules.target(c.file.path, c.recv, c.name)
}

// nameFinder links a call to a same-named definition, preferring the
// closest one (see pickTarget).
type nameFinder struct{ byName map[string][]*Symbol }

// find decides every call it is asked about.
func (f nameFinder) find(c callSite) (*Symbol, bool) {
	return pickTarget(c.from, c.recv, f.byName[c.name], c.file.path), true
}
