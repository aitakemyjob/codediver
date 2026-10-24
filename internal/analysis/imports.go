package analysis

import (
	"regexp"
	"strings"
)

// Import binding: which names a file imports and exports, so a call can be
// resolved through the calling file's imports instead of by bare name.
// Languages opt in by registering an importBinder; the rest keep pure name
// matching.

// nsName is the imported name of a whole-module binding: a namespace import
// (`import * as api`), `const m = require('./m')` or `export * as ns`.
const nsName = "*"

// maxExportHops bounds how many re-exports (`export … from`, barrels) are
// followed to reach a definition.
const maxExportHops = 8

// importBinding is a local name bound by an import: the repository file it
// comes from ("" when the import doesn't resolve to one) and the name that
// file exports it as ("default", or nsName for the whole module).
type importBinding struct {
	file string
	name string
}

// reexport is an `export … from` in a barrel: exported is the name it is
// exported as (nsName for `export *`), name the source file's export.
type reexport struct {
	exported string
	name     string
	file     string
}

// moduleScope is what one file imports and exports.
type moduleScope struct {
	// bindings maps local names to what they import.
	bindings map[string]importBinding
	// exports maps exported names to local names (`export { a as b }`).
	exports map[string]string
	// defaultLocal is the local name exported as default ("" if unknown).
	defaultLocal string
	reexports    []reexport
}

// newModuleScope returns an empty scope.
func newModuleScope() *moduleScope {
	return &moduleScope{bindings: map[string]importBinding{}, exports: map[string]string{}}
}

// importBinder reads a file's import bindings and exports. resolve maps an
// import specifier to a repository file, or "" when it isn't one.
type importBinder interface {
	bind(gf *genericFile, src, masked string, resolve func(spec string) string) *moduleScope
}

// binders holds the languages whose calls resolve through imports. A new
// language registers its binder here.
var binders = map[string]importBinder{
	"typescript": jsBinder{},
	"javascript": jsBinder{},
}

// moduleGraph is every bound file's scope plus what each file defines at the
// top level, used to follow an import to its definition.
type moduleGraph struct {
	scopes map[string]*moduleScope
	// defs maps file → top-level name → symbol (functions and classes).
	defs map[string]map[string]*Symbol
	// members maps file → "Container.name" → method, for classes and
	// object literals.
	members map[string]map[string]*Symbol
	// resolved memoizes exportTarget by file and export name, so barrels
	// that re-export each other (`export *` both ways, or wide trees of
	// them) are walked once.
	resolved map[[2]string]exportResult
	// limitHits counts lookups cut off by maxExportHops; a miss that
	// depended on one isn't memoized, since a shorter path may succeed.
	limitHits int
}

// exportResult is a memoized exportTarget answer.
type exportResult struct {
	target, local string
	ok            bool
}

// newModuleGraph binds every file whose language has a binder.
func newModuleGraph(gfiles []*genericFile, fileSet map[string]bool) *moduleGraph {
	g := &moduleGraph{scopes: map[string]*moduleScope{}, defs: map[string]map[string]*Symbol{}, members: map[string]map[string]*Symbol{}, resolved: map[[2]string]exportResult{}}
	for _, gf := range gfiles {
		b := binders[gf.lang]
		if b == nil {
			continue
		}
		sp := specs[gf.lang]
		resolve := func(spec string) string { return resolveImport(gf.path, spec, sp, fileSet) }
		g.scopes[gf.path] = b.bind(gf, strings.Join(gf.raw, "\n"), strings.Join(gf.lines, "\n"), resolve)
		defs, members := map[string]*Symbol{}, map[string]*Symbol{}
		for _, s := range gf.syms {
			switch {
			case s.Receiver != "":
				if _, dup := members[s.Qualified]; !dup {
					members[s.Qualified] = s
				}
			case s.Parent == "":
				if _, dup := defs[s.Name]; !dup {
					defs[s.Name] = s
				}
			}
		}
		g.defs[gf.path], g.members[gf.path] = defs, members
	}
	return g
}

// definesLocal reports whether file defines name at the top level: a
// function or class, or an object literal holding methods (`const api = {`).
func (g *moduleGraph) definesLocal(file, name string) bool {
	if g.defs[file][name] != nil {
		return true
	}
	prefix := name + "."
	for q := range g.members[file] {
		if strings.HasPrefix(q, prefix) {
			return true
		}
	}
	return false
}

// exportTarget follows file's export called name through export lists,
// imports it re-exports and barrels to the file and local name defining it.
// A local name of nsName means the export is a whole module. ok is false
// when the definition can't be found within maxExportHops.
func (g *moduleGraph) exportTarget(file, name string, hops int) (target, local string, ok bool) {
	key := [2]string{file, name}
	if r, done := g.resolved[key]; done {
		return r.target, r.local, r.ok
	}
	if g.scopes[file] == nil {
		return "", "", false
	}
	if hops > maxExportHops {
		g.limitHits++
		return "", "", false
	}
	hits := g.limitHits
	g.resolved[key] = exportResult{} // a cycle back here finds nothing
	target, local, ok = g.findExport(file, name, hops)
	if !ok && g.limitHits != hits {
		delete(g.resolved, key)
	} else {
		g.resolved[key] = exportResult{target, local, ok}
	}
	return target, local, ok
}

// findExport does exportTarget's lookup in one file.
func (g *moduleGraph) findExport(file, name string, hops int) (target, local string, ok bool) {
	scope := g.scopes[file]
	local = name
	if l, found := scope.exports[name]; found {
		local = l
	} else if name == "default" && scope.defaultLocal != "" {
		local = scope.defaultLocal
	}
	if g.definesLocal(file, local) {
		return file, local, true
	}
	if b, found := scope.bindings[local]; found {
		return g.follow(b, hops+1)
	}
	for _, re := range scope.reexports {
		if re.exported == name {
			return g.follow(importBinding{file: re.file, name: re.name}, hops+1)
		}
	}
	for _, re := range scope.reexports {
		if re.exported == nsName && re.file != "" {
			if t, l, found := g.exportTarget(re.file, name, hops+1); found {
				return t, l, true
			}
		}
	}
	return "", "", false
}

// follow resolves a binding to the file and local name it reaches.
func (g *moduleGraph) follow(b importBinding, hops int) (target, local string, ok bool) {
	if b.file == "" {
		return "", "", false
	}
	if b.name == nsName {
		return b.file, nsName, true
	}
	return g.exportTarget(b.file, b.name, hops)
}

// target finds the definition a call `recv.name(` or `name(` in file reaches
// through file's imports. handled is false when the call doesn't go through
// an import that resolves, so other strategies should decide.
func (g *moduleGraph) target(file, recv, name string) (sym *Symbol, handled bool) {
	scope := g.scopes[file]
	if scope == nil {
		return nil, false
	}
	if recv == "" {
		b, found := scope.bindings[name]
		if !found {
			return nil, false
		}
		if b.name == nsName {
			// `const save = require('./save'); save()`: module.exports.
			b.name = "default"
		}
		t, local, ok := g.follow(b, 0)
		if !ok || local == nsName {
			return nil, false
		}
		s := g.defs[t][local]
		return s, s != nil
	}
	b, found := scope.bindings[recv]
	if !found {
		return nil, false
	}
	t, local, ok := g.follow(b, 0)
	if !ok {
		return nil, false
	}
	if local == nsName {
		// `api.list()` on `import * as api`: api's export called list.
		t, local, ok = g.exportTarget(t, name, 0)
		if !ok || local == nsName {
			return nil, false
		}
		s := g.defs[t][local]
		return s, s != nil
	}
	// A static method of an imported class, or a method of an imported
	// object literal: `Store.create()`, `api.list()`.
	s := g.members[t][local+"."+name]
	return s, s != nil
}

// jsBinder reads ES module imports and exports and CommonJS require/exports
// in TypeScript and JavaScript.
type jsBinder struct{}

var (
	// jsImportRe matches `import [type] clause from 'spec'`, across lines.
	jsImportRe = regexp.MustCompile(`(?m)^[ \t]*(import)\s+(?:type\s+)?([^'";]*?)\s*from\s*['"]([^'"]+)['"]`)
	// jsReexportRe matches `export * from`, `export * as ns from` and
	// `export { … } from`.
	jsReexportRe = regexp.MustCompile(`(?m)^[ \t]*(export)\s+(?:type\s+)?(\*(?:\s+as\s+[\w$]+)?|\{[^}]*\})\s*from\s*['"]([^'"]+)['"]`)
	// jsExportListRe matches a local export list: `export { a, b as c }`.
	jsExportListRe = regexp.MustCompile(`(?m)^[ \t]*(export)\s*\{([^}]*)\}[ \t]*;?[ \t]*$`)
	// jsExportDefaultRe matches `export default name`.
	jsExportDefaultRe = regexp.MustCompile(`(?m)^[ \t]*(export)\s+default\s+([A-Za-z_$][\w$]*)[ \t]*;?[ \t]*$`)
	// jsRequireRe matches `const x = require('spec')` and
	// `const { a, b: c } = require('spec')`.
	jsRequireRe = regexp.MustCompile(`(?m)\b(const|let|var)\s+(\{[^}]*\}|[A-Za-z_$][\w$]*)\s*=\s*require\(\s*['"]([^'"]+)['"]\s*\)[ \t]*;?[ \t]*$`)
	// jsModuleExportsRe matches `module.exports = name` and
	// `module.exports = { a, b: c }`.
	jsModuleExportsRe = regexp.MustCompile(`(?m)^[ \t]*(module)\.exports\s*=\s*(\{[^}]*\}|[A-Za-z_$][\w$]*)[ \t]*;?[ \t]*$`)
)

// bind reads one TypeScript or JavaScript file's bindings and exports.
// Matches inside comments and strings are skipped: the keyword that starts
// each statement must survive masking.
func (jsBinder) bind(gf *genericFile, src, masked string, resolve func(string) string) *moduleScope {
	scope := newModuleScope()
	each := func(re *regexp.Regexp, fn func(m []string)) {
		for _, idx := range re.FindAllStringSubmatchIndex(src, -1) {
			if masked[idx[2]:idx[3]] != src[idx[2]:idx[3]] {
				continue // in a comment
			}
			m := make([]string, len(idx)/2)
			for i := range m {
				if idx[2*i] >= 0 {
					m[i] = src[idx[2*i]:idx[2*i+1]]
				}
			}
			fn(m)
		}
	}
	each(jsImportRe, func(m []string) { bindJSImportClause(scope, m[2], resolve(m[3])) })
	each(jsRequireRe, func(m []string) {
		file := resolve(m[3])
		if !strings.HasPrefix(m[2], "{") {
			scope.bindings[m[2]] = importBinding{file: file, name: nsName}
			return
		}
		for imported, local := range jsNameList(strings.Trim(m[2], "{}"), ":") {
			scope.bindings[local] = importBinding{file: file, name: imported}
		}
	})
	each(jsReexportRe, func(m []string) {
		file := resolve(m[3])
		clause := m[2]
		if strings.HasPrefix(clause, "*") {
			exported := nsName
			if f := strings.Fields(clause); len(f) == 3 {
				exported = f[2] // export * as ns: the module under one name
			}
			scope.reexports = append(scope.reexports, reexport{exported: exported, name: nsName, file: file})
			return
		}
		for name, exported := range jsNameList(strings.Trim(clause, "{}"), " as ") {
			scope.reexports = append(scope.reexports, reexport{exported: exported, name: name, file: file})
		}
	})
	each(jsExportListRe, func(m []string) {
		for local, exported := range jsNameList(m[2], " as ") {
			if exported == "default" {
				scope.defaultLocal = local
				continue
			}
			scope.exports[exported] = local
		}
	})
	each(jsExportDefaultRe, func(m []string) {
		if !keywords[m[2]] {
			scope.defaultLocal = m[2]
		}
	})
	each(jsModuleExportsRe, func(m []string) {
		if !strings.HasPrefix(m[2], "{") {
			scope.defaultLocal = m[2]
			return
		}
		for exported, local := range jsNameList(strings.Trim(m[2], "{}"), ":") {
			scope.exports[exported] = local
		}
	})
	// `export default function save(`, `export default class Store`.
	for _, s := range gf.syms {
		if s.Parent == "" && s.Receiver == "" && s.Line-1 < len(gf.lines) && jsDefaultDeclRe.MatchString(gf.lines[s.Line-1]) {
			scope.defaultLocal = s.Name
		}
	}
	// `export default {` holds methods qualified "default.…".
	if scope.defaultLocal == "" {
		for _, s := range gf.syms {
			if s.Receiver == "default" && s.Parent == "" {
				scope.defaultLocal = "default"
				break
			}
		}
	}
	return scope
}

// jsDefaultDeclRe matches a declaration exported as default.
var jsDefaultDeclRe = regexp.MustCompile(`^\s*export\s+default\s+(?:abstract\s+|async\s+)*(?:function|class)\b`)

// bindJSImportClause binds the names of an import clause (`def`,
// `{ a, b as c }`, `* as ns`, or a default followed by either) to file.
func bindJSImportClause(scope *moduleScope, clause, file string) {
	named := ""
	if i := strings.IndexByte(clause, '{'); i >= 0 {
		j := strings.IndexByte(clause, '}')
		if j < i {
			return
		}
		named = clause[i+1 : j]
		clause = clause[:i] + clause[j+1:]
	}
	for imported, local := range jsNameList(named, " as ") {
		scope.bindings[local] = importBinding{file: file, name: imported}
	}
	for _, part := range strings.Split(clause, ",") {
		f := strings.Fields(part)
		switch {
		case len(f) == 1 && isJSIdent(f[0]):
			scope.bindings[f[0]] = importBinding{file: file, name: "default"}
		case len(f) == 3 && f[0] == "*" && f[1] == "as" && isJSIdent(f[2]):
			scope.bindings[f[2]] = importBinding{file: file, name: nsName}
		}
	}
}

// jsNameList parses a comma-separated list of names, each optionally renamed
// with sep (`a as b`, `a: b`), into a map from the original name to the new
// one. Inline `type` modifiers are dropped and malformed entries skipped.
func jsNameList(list, sep string) map[string]string {
	out := map[string]string{}
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		item = strings.TrimSpace(strings.TrimPrefix(item, "type "))
		if item == "" {
			continue
		}
		from, to := item, item
		if i := strings.Index(item, sep); i >= 0 {
			from, to = strings.TrimSpace(item[:i]), strings.TrimSpace(item[i+len(sep):])
		}
		if isJSIdent(from) && isJSIdent(to) {
			out[from] = to
		}
	}
	return out
}

// isJSIdent reports whether s is a JavaScript identifier.
func isJSIdent(s string) bool {
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isIdentByte(s[i]) {
			return false
		}
	}
	return true
}
