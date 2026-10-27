package analysis

import (
	"regexp"
	"strings"
)

// Receiver types: for `recv.method()`, find recv's class where the code
// writes it down (`const s = new SqlStore()`, `(s: SqlStore)`,
// `private store: SqlStore`, `s = Store()`, `self.store = Store()`) and
// link the call to that class's method instead of any same-named one.
// Languages opt in with a typeHints entry.

// typePattern finds a name and its class on one line: capture group name
// is the variable or attribute, group class its class (a type annotation,
// or the class constructed).
type typePattern struct {
	re          *regexp.Regexp
	name, class int
}

// typeHints are one language's patterns for where a class is written down.
type typeHints struct {
	// locals are variables in a function body.
	locals []typePattern
	// members are attributes assigned anywhere in a class's methods
	// (`this.x = new C()`, `self.x = C()`).
	members []typePattern
	// fields are attribute declarations in a class body, outside its methods.
	fields []typePattern
	// copies are attributes set from a variable (`self.store = store`):
	// group name is the attribute, group class the variable, whose type
	// comes from the method's parameters and locals.
	copies []typePattern
	// paramModifiers mark constructor parameters that are also attributes
	// (TypeScript's `constructor(private store: SqlStore)`).
	paramModifiers map[string]bool
}

// receiverHints holds the languages whose receivers are typed. A new
// language registers its patterns here.
var receiverHints = map[string]*typeHints{
	"typescript": tsHints,
	"javascript": tsHints,
	"python":     pyHints,
}

var tsHints = &typeHints{
	locals: []typePattern{
		{rx(`\b(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*new\s+([A-Za-z_$][\w$.]*)`), 1, 2},
		{rx(`\b(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*:\s*([A-Za-z_$][\w$.<>]*)\s*(?:[=;]|$)`), 1, 2},
	},
	members: []typePattern{
		{rx(`\bthis\.([A-Za-z_$#][\w$]*)\s*=\s*new\s+([A-Za-z_$][\w$.]*)`), 1, 2},
	},
	fields: []typePattern{
		{rx(`^\s*(?:(?:public|private|protected|readonly|static|declare|override)\s+)*([A-Za-z_$#][\w$]*)\s*[!?]?\s*:\s*([A-Za-z_$][\w$.<>| ]*)`), 1, 2},
		{rx(`^\s*(?:(?:public|private|protected|readonly|static)\s+)*([A-Za-z_$#][\w$]*)\s*=\s*new\s+([A-Za-z_$][\w$.]*)`), 1, 2},
	},
	copies: []typePattern{
		{rx(`\bthis\.([A-Za-z_$#][\w$]*)\s*=\s*([A-Za-z_$][\w$]*)\s*;?\s*$`), 1, 2},
	},
	paramModifiers: map[string]bool{"public": true, "private": true, "protected": true, "readonly": true},
}

var pyHints = &typeHints{
	locals: []typePattern{
		{rx(`^\s*([A-Za-z_]\w*)\s*(?::[^=]+)?=\s*([A-Za-z_][\w.]*)\s*\(`), 1, 2},
		{rx(`^\s*([A-Za-z_]\w*)\s*:\s*([A-Za-z_][\w.\[\]| ]*?)\s*(?:=|$)`), 1, 2},
	},
	members: []typePattern{
		{rx(`\bself\.([A-Za-z_]\w*)\s*(?::[^=]+)?=\s*([A-Za-z_][\w.]*)\s*\(`), 1, 2},
		{rx(`\bself\.([A-Za-z_]\w*)\s*:\s*([A-Za-z_][\w.\[\]| ]*?)\s*(?:=|$)`), 1, 2},
	},
	fields: []typePattern{
		{rx(`^\s+([A-Za-z_]\w*)\s*:\s*([A-Za-z_][\w.\[\]| ]*?)\s*(?:=|$)`), 1, 2},
	},
	copies: []typePattern{
		{rx(`\bself\.([A-Za-z_]\w*)\s*=\s*([A-Za-z_]\w*)\s*$`), 1, 2},
	},
}

// maxExtendsDepth bounds how far a method is looked up through `extends`.
const maxExtendsDepth = 5

// receiverFinder resolves `recv.method()` through recv's written-down
// class. It passes when recv's class isn't known or can't be followed to a
// definition, so name matching decides.
type receiverFinder struct {
	byName  map[string][]*Symbol
	modules *moduleGraph
	files   map[string]*genericFile
	// classes maps file → class name → class symbol.
	classes map[string]map[string]*Symbol
	locals  map[*Symbol]map[string]string
	members map[string]map[string]string
}

// newReceiverFinder indexes the classes of the given files.
func newReceiverFinder(byName map[string][]*Symbol, modules *moduleGraph, gfiles []*genericFile) *receiverFinder {
	f := &receiverFinder{byName: byName, modules: modules, files: map[string]*genericFile{}, classes: map[string]map[string]*Symbol{},
		locals: map[*Symbol]map[string]string{}, members: map[string]map[string]string{}}
	for _, gf := range gfiles {
		f.files[gf.path] = gf
		for _, s := range gf.syms {
			if s.Kind == KindClass {
				if f.classes[gf.path] == nil {
					f.classes[gf.path] = map[string]*Symbol{}
				}
				f.classes[gf.path][s.Name] = s
			}
		}
	}
	return f
}

// find decides `recv.method()` calls whose receiver has a known class
// with that method (directly or through `extends`).
func (f *receiverFinder) find(c callSite) (match, bool) {
	hints := receiverHints[c.file.lang]
	if hints == nil || c.recv == "" || c.recv == "this" || c.recv == "self" || c.recv == "super" {
		return match{}, false
	}
	typ := f.localTypes(c.file, c.from, hints)[c.recv]
	if typ == "" && c.from.Parent != "" {
		typ = f.memberTypes(c.from.Parent, hints)[c.recv]
	}
	if typ == "" {
		return match{}, false
	}
	cls := f.classNamed(c.file, typ)
	if cls == nil {
		return match{}, false
	}
	if m := f.methodOf(cls, c.name, 0); m != nil {
		return match{target: m}, true
	}
	return match{}, false
}

// localTypes returns the classes written down for fn's parameters and
// local variables, by name.
func (f *receiverFinder) localTypes(gf *genericFile, fn *Symbol, hints *typeHints) map[string]string {
	if t, ok := f.locals[fn]; ok {
		return t
	}
	types := map[string]string{}
	for _, p := range fn.Params {
		if name := paramName(p.Name); name != "" && p.Type != "" {
			types[name] = p.Type
		}
	}
	for ln := fn.Line; ln <= fn.EndLine && ln <= len(gf.lines); ln++ {
		for _, pt := range hints.locals {
			for _, m := range pt.re.FindAllStringSubmatch(gf.lines[ln-1], -1) {
				types[m[pt.name]] = m[pt.class]
			}
		}
	}
	f.locals[fn] = types
	return types
}

// memberTypes returns the classes written down for a class's attributes:
// declared fields, attributes assigned in its methods and TypeScript
// constructor parameter properties.
func (f *receiverFinder) memberTypes(classID string, hints *typeHints) map[string]string {
	if t, ok := f.members[classID]; ok {
		return t
	}
	types := map[string]string{}
	f.members[classID] = types
	cls := f.symbol(classID)
	if cls == nil {
		return types
	}
	gf := f.files[cls.File]
	inMethod := make([]bool, len(gf.lines)+1)
	for _, s := range gf.syms {
		if s.Kind.IsCallable() && s.Line >= cls.Line && s.EndLine <= cls.EndLine {
			locals := f.localTypes(gf, s, hints)
			for ln := s.Line; ln <= s.EndLine && ln <= len(gf.lines); ln++ {
				inMethod[ln] = true
				for _, pt := range hints.copies {
					if m := pt.re.FindStringSubmatch(gf.lines[ln-1]); m != nil && locals[m[pt.class]] != "" {
						types[m[pt.name]] = locals[m[pt.class]]
					}
				}
			}
			if s.Name == "constructor" && s.Parent == classID {
				for _, p := range s.Params {
					if words := strings.Fields(p.Name); len(words) > 1 && hints.paramModifiers[words[0]] && p.Type != "" {
						types[words[len(words)-1]] = p.Type
					}
				}
			}
		}
	}
	for ln := cls.Line + 1; ln <= cls.EndLine && ln <= len(gf.lines); ln++ {
		line := gf.lines[ln-1]
		for _, pt := range hints.members {
			for _, m := range pt.re.FindAllStringSubmatch(line, -1) {
				types[m[pt.name]] = m[pt.class]
			}
		}
		if inMethod[ln] {
			continue
		}
		for _, pt := range hints.fields {
			if m := pt.re.FindStringSubmatch(line); m != nil {
				types[m[pt.name]] = m[pt.class]
			}
		}
	}
	return types
}

// symbol finds a symbol by ID among the indexed files' symbols.
func (f *receiverFinder) symbol(id string) *Symbol {
	for _, byName := range f.classes {
		for _, s := range byName {
			if s.ID == id {
				return s
			}
		}
	}
	return nil
}

// classNamed resolves a written-down type to a class in the repository:
// through gf's imports first, then a class in the same file, then the
// only class of that name. Unions with None/null, Optional[…] and generic
// arguments are stripped; anything else (arrays, other unions) is not a
// single class.
func (f *receiverFinder) classNamed(gf *genericFile, typ string) *Symbol {
	qual, name := splitType(cleanType(typ))
	if name == "" {
		return nil
	}
	isClass := func(s *Symbol) bool { return s != nil && s.Kind == KindClass }
	if t, ok := f.modules.target(gf.path, qual, name); ok && isClass(t) {
		return t
	}
	if qual != "" {
		return nil
	}
	if s := f.classes[gf.path][name]; s != nil {
		return s
	}
	var only *Symbol
	for _, s := range f.byName[name] {
		if isClass(s) {
			if only != nil {
				return nil
			}
			only = s
		}
	}
	return only
}

// methodOf finds the method name on cls, or on the classes it extends.
func (f *receiverFinder) methodOf(cls *Symbol, name string, depth int) *Symbol {
	for _, c := range f.byName[name] {
		if c.Kind == KindMethod && c.Parent == cls.ID {
			return c
		}
	}
	gf := f.files[cls.File]
	if depth >= maxExtendsDepth || gf == nil || gf.extends[cls.ID] == "" {
		return nil
	}
	parent := f.classNamed(gf, gf.extends[cls.ID])
	if parent == nil {
		return nil
	}
	return f.methodOf(parent, name, depth+1)
}

// cleanType reduces a written-down type to one class name: it drops
// `| None`, `| null`, `| undefined`, `Optional[…]` and generic arguments,
// and returns "" for arrays, containers and unions of several types.
func cleanType(t string) string {
	t = strings.TrimSpace(t)
	if strings.HasPrefix(t, "Optional[") && strings.HasSuffix(t, "]") {
		t = strings.TrimSuffix(strings.TrimPrefix(t, "Optional["), "]")
	}
	var parts []string
	for _, p := range strings.Split(t, "|") {
		p = strings.TrimSpace(p)
		if p != "None" && p != "null" && p != "undefined" && p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) != 1 {
		return ""
	}
	t = parts[0]
	if i := strings.IndexByte(t, '<'); i > 0 {
		t = t[:i]
	}
	if strings.ContainsAny(t, "[]<>(){} ,") {
		return ""
	}
	return t
}

// splitType splits `ns.Store` into its qualifier and class name.
func splitType(t string) (qual, name string) {
	if i := strings.LastIndexByte(t, '.'); i >= 0 {
		qual, name = t[:i], t[i+1:]
		if j := strings.LastIndexByte(qual, '.'); j >= 0 {
			qual = qual[j+1:]
		}
		return qual, name
	}
	return "", t
}

// paramName returns a parameter's name without TypeScript modifiers and
// optional marker (`private cart?`) or Python's * and ** prefixes.
func paramName(p string) string {
	f := strings.Fields(p)
	if len(f) == 0 {
		return ""
	}
	return strings.TrimRight(strings.TrimLeft(f[len(f)-1], "*"), "?")
}
