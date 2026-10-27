package analysis

import (
	"regexp"
	"strings"
)

// Interface dispatch for the lexical languages: a type that lists an
// interface in its header (`class C implements I`, `class C : IFoo`,
// `impl Trait for Type`) implements it, and each of the interface's
// methods dispatches to the type's method of the same name, as Go's typed
// analysis does.

// addSupertypes records types listed as implemented by the type id.
func (gf *genericFile) addSupertypes(id string, types ...string) {
	if gf.supertypes == nil {
		gf.supertypes = map[string][]string{}
	}
	gf.supertypes[id] = append(gf.supertypes[id], types...)
}

// linkImplementations adds `implements` edges from each type to the
// interfaces its header lists and `dispatch` edges from their methods to
// the type's. A listed class (a C# or Kotlin base class) becomes the type's
// parent for method lookup instead.
func linkImplementations(ix *Index, gfiles []*genericFile, types *receiverFinder, merge map[[3]string]int) {
	for _, gf := range gfiles {
		for id, sups := range gf.supertypes {
			cls := ix.Symbols[id]
			if cls == nil {
				continue
			}
			for _, sup := range sups {
				iface := types.classNamed(gf, sup)
				switch {
				case iface == nil || iface == cls:
					continue
				case iface.Kind != KindInterface:
					if gf.extends == nil {
						gf.extends = map[string]string{}
					}
					if gf.extends[id] == "" {
						gf.extends[id] = sup
					}
					continue
				}
				cls.Implements = appendUnique(cls.Implements, iface.ID)
				ix.addEdge(Edge{From: cls.ID, To: iface.ID, Kind: EdgeImplement, Heuristic: true}, merge)
				for _, mid := range iface.Methods {
					im := ix.Symbols[mid]
					if im == nil {
						continue
					}
					if impl := types.methodOf(cls, im.Name, 0); impl != nil && impl != im {
						ix.addEdge(Edge{From: im.ID, To: impl.ID, Kind: EdgeDispatch, Heuristic: true}, merge)
					}
				}
			}
		}
	}
}

// typeList splits a header's list of supertypes (`IStore, IDisposable`,
// `Base(), Named<T> by impl`) into bare type names.
func typeList(s string) []string {
	var out []string
	for _, part := range splitParams(strings.Split(s, " where ")[0]) {
		name := strings.TrimSpace(part.Type + " " + part.Name)
		if i := strings.IndexAny(name, "(<"); i >= 0 {
			name = name[:i]
		}
		f := strings.Fields(name)
		if len(f) == 0 {
			continue
		}
		if t := lastPathSegment(f[0]); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// lastPathSegment returns the last name of `a::b::C` or `a.b.C`.
func lastPathSegment(t string) string {
	t = strings.ReplaceAll(t, "::", ".")
	return t[strings.LastIndexByte(t, '.')+1:]
}

// extensionType returns the type an extension function extends without
// generic arguments or nullability: `List<T>` is List, `String?` String.
func extensionType(t string) string {
	if i := strings.IndexAny(t, "<["); i > 0 {
		t = t[:i]
	}
	return lastPathSegment(strings.TrimRight(t, "?"))
}

// qualifyExtensions qualifies the top-level functions in the extension
// blocks of a file (Scala 3's `extension (s: String)` followed by indented
// or braced defs) by the type they extend.
func qualifyExtensions(gf *genericFile, block *regexp.Regexp) {
	for i, line := range gf.lines {
		m := block.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		typ := extensionType(m[1])
		indent, end := indentOf(line), i+1
		for j := i + 1; j < len(gf.lines); j++ {
			t := strings.TrimSpace(gf.lines[j])
			if t == "" || t == "{" || t == "}" {
				continue
			}
			if indentOf(gf.lines[j]) <= indent {
				break
			}
			end = j + 1
		}
		for _, s := range gf.syms {
			if s.Line >= i+1 && s.Line <= end && s.Parent == "" && s.Kind == KindFunc && s.Receiver == "" {
				s.Receiver = typ
				s.Qualified = typ + "." + s.Name
				s.ID = "h:" + gf.path + "#" + s.Qualified
			}
		}
	}
}
