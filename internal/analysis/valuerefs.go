package analysis

import (
	"regexp"
	"strings"
)

// Functions passed as values: `items.map(format)`, `promise.then(render)`,
// `{ handler: create }`, `sorted(xs, key=keyfn)`, `Thread(target=run)`,
// `this::save`. Each becomes a ref edge from the function passing it, so a
// callback's caller is visible. Only names that resolve to a function
// count: imported, defined at the top level of the same file, or a method
// of the caller's class through this/self. Languages opt in with a
// valueHints entry.

// valueHints are one language's rules for spotting a function used as a
// value rather than called.
type valueHints struct {
	// before and after are the characters that may surround a name used as
	// a value: a whole argument sits between ( or , and , or ); an object
	// or dict value between : and , or }; a keyword argument between =
	// and , or ).
	before, after string
	// selfRefs are the receivers that name the caller's own object.
	selfRefs []string
	// methodRef matches method references (`this::save`, `Store::save`):
	// group 1 the receiver, group 2 the method.
	methodRef *regexp.Regexp
	// imports matches import and re-export lines, whose name lists aren't
	// values.
	imports *regexp.Regexp
}

var (
	jsImportLine = regexp.MustCompile(`^\s*(?:import\b|export\s*(?:type\s*)?[{*])`)
	pyImportLine = regexp.MustCompile(`^\s*(?:from\s+\S+\s+)?import\b`)
)

// valueRefHints holds the languages whose callbacks become ref edges.
var valueRefHints = map[string]*valueHints{
	"typescript": {before: "(,:", after: ",)}", selfRefs: []string{"this"}, imports: jsImportLine},
	"javascript": {before: "(,:", after: ",)}", selfRefs: []string{"this"}, imports: jsImportLine},
	"python":     {before: "(,:=", after: ",)}", selfRefs: []string{"self"}, imports: pyImportLine},
	"java":       {methodRef: regexp.MustCompile(`\b(this|[A-Z][\w$]*)::([A-Za-z_$][\w$]*)\b`)},
}

// valueLiterals are names that are values but never functions.
var valueLiterals = map[string]bool{"true": true, "false": true, "null": true, "undefined": true, "None": true, "True": true, "False": true, "this": true, "self": true}

// valueRefs finds the functions a line passes as values.
type valueRefs struct {
	byName  map[string][]*Symbol
	modules *moduleGraph
	// topLevel maps file → name → function defined at its top level.
	topLevel map[string]map[string]*Symbol
	classes  map[string]map[string]*Symbol
}

// newValueRefs indexes the top-level functions and classes of the files.
func newValueRefs(byName map[string][]*Symbol, modules *moduleGraph, gfiles []*genericFile) *valueRefs {
	v := &valueRefs{byName: byName, modules: modules, topLevel: map[string]map[string]*Symbol{}, classes: map[string]map[string]*Symbol{}}
	for _, gf := range gfiles {
		fns, cls := map[string]*Symbol{}, map[string]*Symbol{}
		for _, s := range gf.syms {
			switch {
			case s.Kind == KindFunc && s.Parent == "":
				if _, dup := fns[s.Name]; !dup {
					fns[s.Name] = s
				}
			case s.Kind == KindClass:
				cls[s.Name] = s
			}
		}
		v.topLevel[gf.path], v.classes[gf.path] = fns, cls
	}
	return v
}

// find returns the functions passed as values on one masked line of from.
func (v *valueRefs) find(from *Symbol, gf *genericFile, text string) []*Symbol {
	h := valueRefHints[gf.lang]
	if h == nil || h.imports != nil && h.imports.MatchString(text) {
		return nil
	}
	var out []*Symbol
	add := func(s *Symbol) {
		if s != nil && s.ID != from.ID && s.Kind.IsCallable() {
			out = append(out, s)
		}
	}
	if h.before != "" {
		for _, r := range valueNames(text, h.before, h.after) {
			if r.recv != "" {
				if containsString(h.selfRefs, r.recv) {
					add(v.ownMethod(from, r.name))
				}
				continue
			}
			add(v.resolve(from, gf, r.name))
		}
	}
	if h.methodRef != nil {
		for _, m := range h.methodRef.FindAllStringSubmatch(text, -1) {
			if m[1] == "this" {
				add(v.ownMethod(from, m[2]))
			} else if cls := v.classes[gf.path][m[1]]; cls != nil {
				add(v.methodOf(cls, m[2]))
			} else {
				add(v.methodOfUnique(m[1], m[2]))
			}
		}
	}
	return out
}

// resolve finds the function a bare name passed as a value means: what the
// file imports under that name, else a function defined at the top of the
// same file. A parameter of the same name shadows both.
func (v *valueRefs) resolve(from *Symbol, gf *genericFile, name string) *Symbol {
	if valueLiterals[name] || keywords[name] {
		return nil
	}
	for _, p := range from.Params {
		if paramName(p.Name) == name {
			return nil
		}
	}
	if t, ok := v.modules.target(gf.path, "", name); ok {
		return t
	}
	return v.topLevel[gf.path][name]
}

// ownMethod finds the method name on from's class (this.render, self.run).
func (v *valueRefs) ownMethod(from *Symbol, name string) *Symbol {
	if from.Parent == "" {
		return nil
	}
	for _, c := range v.byName[name] {
		if c.Kind == KindMethod && c.Parent == from.Parent {
			return c
		}
	}
	return nil
}

// methodOf finds the method name declared on cls.
func (v *valueRefs) methodOf(cls *Symbol, name string) *Symbol {
	for _, c := range v.byName[name] {
		if c.Kind == KindMethod && c.Parent == cls.ID {
			return c
		}
	}
	return nil
}

// methodOfUnique finds the method name on the only class called class.
func (v *valueRefs) methodOfUnique(class, name string) *Symbol {
	var cls *Symbol
	for _, c := range v.byName[class] {
		if c.Kind == KindClass {
			if cls != nil {
				return nil
			}
			cls = c
		}
	}
	if cls == nil {
		return nil
	}
	return v.methodOf(cls, name)
}

// valueName is a name used as a value, with its receiver when written as
// `this.name` / `self.name`.
type valueName struct{ recv, name string }

// valueNames finds the names on a line that stand alone as values: an
// identifier (optionally `recv.name`) whose nearest non-space neighbours
// are one of before on the left and one of after on the right. `==` and
// `=>` on the left don't count.
func valueNames(text, before, after string) []valueName {
	var out []valueName
	for _, loc := range valueNameRe.FindAllStringSubmatchIndex(text, -1) {
		start, end := loc[0], loc[1]
		if start > 0 && isIdentByte(text[start-1]) {
			continue // part of a longer identifier
		}
		l := strings.TrimRight(text[:start], " \t")
		r := strings.TrimLeft(text[end:], " \t")
		if l == "" || r == "" || !strings.ContainsRune(before, rune(l[len(l)-1])) || !strings.ContainsRune(after, rune(r[0])) {
			continue
		}
		if c := l[len(l)-1]; c == '=' && len(l) > 1 && strings.ContainsRune("=!<>", rune(l[len(l)-2])) {
			continue // a comparison, not a keyword argument
		}
		recv, name := "", text[loc[4]:loc[5]]
		if loc[2] >= 0 {
			recv = text[loc[2]:loc[3]]
		}
		out = append(out, valueName{recv: recv, name: name})
	}
	return out
}

// valueNameRe matches `name` or `recv.name`.
var valueNameRe = regexp.MustCompile(`(?:([A-Za-z_$][\w$]*)\.)?([A-Za-z_$][\w$]*)`)

// containsString reports whether list holds s.
func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
