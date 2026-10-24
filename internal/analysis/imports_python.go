package analysis

import (
	"regexp"
	"strings"
)

// pyBinder reads Python imports: `from m import a, b as c` (parenthesized
// across lines too), `from m import *`, `import a.b as c` and `import a`,
// relative or absolute. A package's `__init__.py` re-exports what it
// imports, so following a name through one needs nothing more.
type pyBinder struct{}

var (
	// pyFromRe matches `from spec import names`; names may be a
	// parenthesized list spanning lines.
	pyFromRe = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+(\.*[\w.]*)[ \t]+import[ \t]*(\([^)]*\)|[^\n;]*)`)
	// pyImportRe matches `import a.b as c, d`.
	pyImportRe = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+([^\n;]+)`)
)

// bind reads one Python file's import bindings. Python imports hold no
// string literals, so the masked source (comments blanked) is read.
func (pyBinder) bind(_ *genericFile, _, masked string, resolve func(string) string) *moduleScope {
	scope := newModuleScope()
	for _, m := range pyFromRe.FindAllStringSubmatch(masked, -1) {
		spec, names := m[1], strings.Trim(strings.TrimSpace(m[2]), "()")
		if strings.TrimSpace(names) == "*" {
			file := resolve(spec)
			scope.reexports = append(scope.reexports, reexport{exported: nsName, name: nsName, file: file})
			if file != "" {
				scope.wildcards = append(scope.wildcards, file)
			}
			continue
		}
		for imported, local := range nameList(strings.ReplaceAll(names, "\n", " "), " as ") {
			// `from . import util` / `from orders import util` names a
			// submodule when one exists, else something the module defines.
			if sub := resolve(pySubmodule(spec, imported)); sub != "" {
				scope.bindings[local] = importBinding{file: sub, name: nsName}
				continue
			}
			scope.bindings[local] = importBinding{file: resolve(spec), name: imported}
		}
	}
	for _, m := range pyImportRe.FindAllStringSubmatch(masked, -1) {
		for _, item := range strings.Split(m[1], ",") {
			f := strings.Fields(item)
			var module, local string
			switch {
			case len(f) == 1:
				// `import orders.util` is used as orders.util.fmt(); the
				// call's receiver is its last part.
				module, local = f[0], f[0][strings.LastIndexByte(f[0], '.')+1:]
			case len(f) == 3 && f[1] == "as":
				module, local = f[0], f[2]
			default:
				continue
			}
			if isIdent(local) {
				scope.bindings[local] = importBinding{file: resolve(module), name: nsName}
			}
		}
	}
	return scope
}

// pySubmodule returns the import spec of submodule name inside spec:
// `.` + util → `.util`, `orders` + util → `orders.util`.
func pySubmodule(spec, name string) string {
	if strings.Trim(spec, ".") == "" {
		return spec + name
	}
	return spec + "." + name
}
