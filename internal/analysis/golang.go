package analysis

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// maxGoFiles bounds type-checking work on very large monorepos.
const maxGoFiles = 6000

type goPkg struct {
	importPath string
	dir        string // repo-relative
	name       string
	files      []*ast.File
	fileNames  []string
	types      *types.Package
	checking   bool
}

type goAnalyzer struct {
	root  string
	fset  *token.FileSet
	pkgs  map[string]*goPkg // by import path
	info  *types.Info
	ix    *Index
	merge map[[3]string]int
}

// analyzeGo type-checks every Go package in files and records symbols and
// edges into ix. External imports are replaced with empty fake packages, so
// intra-repo resolution stays exact while stdlib/deps calls are recorded by
// name only.
func analyzeGo(root string, files []string, ix *Index, merge map[[3]string]int) {
	a := &goAnalyzer{
		root:  root,
		fset:  token.NewFileSet(),
		pkgs:  map[string]*goPkg{},
		ix:    ix,
		merge: merge,
		info: &types.Info{
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
			Types:      map[ast.Expr]types.TypeAndValue{},
			Instances:  map[*ast.Ident]types.Instance{},
		},
	}
	a.load(files)
	paths := make([]string, 0, len(a.pkgs))
	for p := range a.pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		a.check(a.pkgs[p])
	}
	for _, p := range paths {
		a.collect(a.pkgs[p])
	}
	for _, p := range paths {
		a.collectCalls(a.pkgs[p])
	}
	a.linkImplementations()
}

// load parses the repository's non-test Go files into packages, one per
// directory, honoring build constraints and the nearest go.mod's module path.
func (a *goAnalyzer) load(files []string) {
	byDir := map[string][]string{}
	n := 0
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") {
			continue
		}
		if n++; n > maxGoFiles {
			a.ix.Errors = append(a.ix.Errors, fmt.Sprintf("go: more than %d files, remaining skipped", maxGoFiles))
			break
		}
		byDir[path.Dir(f)] = append(byDir[path.Dir(f)], f)
	}
	modCache := map[string][2]string{} // dir -> (modRootRel, modulePath)
	for dir, names := range byDir {
		modRoot, modPath := a.findModule(dir, modCache)
		importPath := modPath
		if rel := strings.TrimPrefix(strings.TrimPrefix(dir, modRoot), "/"); rel != "" && rel != "." {
			if importPath == "" {
				importPath = rel
			} else {
				importPath += "/" + rel
			}
		}
		if importPath == "" {
			importPath = "main"
		}
		pkg := &goPkg{importPath: importPath, dir: strings.TrimPrefix(dir, "./")}
		if pkg.dir == "." {
			pkg.dir = ""
		}
		counts := map[string]int{}
		var parsed []*ast.File
		var parsedNames []string
		for _, name := range names {
			abs := filepath.Join(a.root, filepath.FromSlash(name))
			if ok, err := build.Default.MatchFile(filepath.Dir(abs), filepath.Base(abs)); err == nil && !ok {
				continue
			}
			f, err := parser.ParseFile(a.fset, abs, nil, parser.ParseComments|parser.SkipObjectResolution)
			if f == nil {
				a.ix.Errors = append(a.ix.Errors, err.Error())
				continue
			}
			counts[f.Name.Name]++
			parsed = append(parsed, f)
			parsedNames = append(parsedNames, name)
		}
		// Pick the dominant package name; drop stray files (e.g. `package main` generators).
		best := ""
		for name, c := range counts {
			if c > counts[best] || (c == counts[best] && name < best) {
				best = name
			}
		}
		for i, f := range parsed {
			if f.Name.Name == best {
				pkg.files = append(pkg.files, f)
				pkg.fileNames = append(pkg.fileNames, parsedNames[i])
			}
		}
		if len(pkg.files) == 0 {
			continue
		}
		pkg.name = best
		a.pkgs[importPath] = pkg
	}
}

// findModule walks up from dir to the nearest go.mod inside the repo.
func (a *goAnalyzer) findModule(dir string, cache map[string][2]string) (string, string) {
	if v, ok := cache[dir]; ok {
		return v[0], v[1]
	}
	cur := dir
	for {
		gomod := filepath.Join(a.root, filepath.FromSlash(cur), "go.mod")
		if mp := readModulePath(gomod); mp != "" {
			cache[dir] = [2]string{cur, mp}
			return cur, mp
		}
		if cur == "." || cur == "" || cur == "/" {
			break
		}
		cur = path.Dir(cur)
	}
	cache[dir] = [2]string{".", ""}
	return ".", ""
}

// readModulePath returns the module path declared in a go.mod file, or ""
// if the file is missing or has no module line.
func readModulePath(gomod string) string {
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "module") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "module")), `"`)
		}
	}
	return ""
}

type goImporter struct{ a *goAnalyzer }

// Import implements types.Importer: repository packages are type-checked
// on demand; anything else becomes an empty stand-in package so checking
// can continue without the Go toolchain or downloaded dependencies.
func (imp goImporter) Import(p string) (*types.Package, error) {
	if pkg, ok := imp.a.pkgs[p]; ok {
		if t := imp.a.check(pkg); t != nil {
			return t, nil
		}
	}
	// Unknown (stdlib or third-party): an empty package keeps checking going.
	name := path.Base(p)
	if strings.HasPrefix(name, "v") && len(name) <= 3 && strings.Contains(p, "/") {
		name = path.Base(path.Dir(p)) // module/v2 -> module
	}
	name = strings.TrimSuffix(strings.TrimPrefix(name, "go-"), ".go")
	name = strings.ReplaceAll(name, "-", "_")
	if i := strings.Index(name, "."); i > 0 {
		name = name[:i] // gopkg.in/yaml.v3 -> yaml
	}
	fake := types.NewPackage(p, name)
	fake.MarkComplete()
	return fake, nil
}

// check type-checks a package once (memoized), tolerating errors, and
// registers it in the index. Import cycles return nil instead of recursing.
func (a *goAnalyzer) check(pkg *goPkg) *types.Package {
	if pkg.types != nil || pkg.checking {
		return pkg.types
	}
	pkg.checking = true
	defer func() { pkg.checking = false }()
	errs := 0
	conf := types.Config{
		Importer:    goImporter{a},
		FakeImportC: true,
		Error:       func(error) { errs++ },
	}
	t, _ := conf.Check(pkg.importPath, a.fset, pkg.files, a.info)
	pkg.types = t
	a.ix.Packages[pkg.importPath] = &Package{
		ID: pkg.importPath, Name: pkg.name, Dir: pkg.dir, Language: "go", Files: pkg.fileNames, Internal: true,
	}
	return t
}

// relFile converts a token position into a repo-relative path and line.
func (a *goAnalyzer) relFile(pos token.Pos) (string, int) {
	p := a.fset.Position(pos)
	rel, err := filepath.Rel(a.root, p.Filename)
	if err != nil {
		return p.Filename, p.Line
	}
	return filepath.ToSlash(rel), p.Line
}

// goSymbolID is the stable ID for a Go function or method symbol.
func goSymbolID(fn *types.Func) string { return "go:" + fn.FullName() }

// qualifierFor prints types from pkg unqualified and others by package
// name, matching how the code reads inside pkg.
func qualifierFor(pkg *types.Package) types.Qualifier {
	return func(other *types.Package) string {
		if other == pkg {
			return ""
		}
		return other.Name()
	}
}

// tupleParams converts a parameter or result tuple into Params, rendering
// the final variadic parameter as ...T.
func tupleParams(t *types.Tuple, q types.Qualifier, variadic bool) []Param {
	var out []Param
	for i := 0; i < t.Len(); i++ {
		v := t.At(i)
		typ := types.TypeString(v.Type(), q)
		if variadic && i == t.Len()-1 {
			if s, ok := v.Type().(*types.Slice); ok {
				typ = "..." + types.TypeString(s.Elem(), q)
			}
		}
		out = append(out, Param{Name: v.Name(), Type: typ})
	}
	return out
}

// astParams flattens a field list into params using source text for types.
func astParams(fl *ast.FieldList) []Param {
	if fl == nil {
		return nil
	}
	var out []Param
	for _, f := range fl.List {
		typ := types.ExprString(f.Type)
		if len(f.Names) == 0 {
			out = append(out, Param{Type: typ})
		}
		for _, n := range f.Names {
			out = append(out, Param{Name: n.Name, Type: typ})
		}
	}
	return out
}

// patchInvalid replaces types the checker could not resolve (because they
// come from stubbed external packages) with their source spelling.
func patchInvalid(typed, src []Param) []Param {
	if len(typed) != len(src) {
		return typed
	}
	for i := range typed {
		if strings.Contains(typed[i].Type, "invalid type") {
			typed[i].Type = src[i].Type
		}
	}
	return typed
}

// collect records a package's imports, functions, methods and types into
// the index, plus package-level dependency edges.
func (a *goAnalyzer) collect(pkg *goPkg) {
	if pkg.types == nil {
		return
	}
	q := qualifierFor(pkg.types)
	ixPkg := a.ix.Packages[pkg.importPath]
	seenImports := map[string]bool{}
	for _, f := range pkg.files {
		for _, im := range f.Imports {
			p := strings.Trim(im.Path.Value, `"`)
			if !seenImports[p] {
				seenImports[p] = true
				ixPkg.Imports = append(ixPkg.Imports, p)
			}
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				a.collectFunc(pkg, d, q)
			case *ast.GenDecl:
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					a.collectType(pkg, d, spec.(*ast.TypeSpec), q)
				}
			}
		}
	}
	sort.Strings(ixPkg.Imports)
	for _, p := range ixPkg.Imports {
		if _, ok := a.pkgs[p]; ok {
			a.ix.PkgEdges = append(a.ix.PkgEdges, Edge{From: pkg.importPath, To: p, Kind: EdgeImport})
		}
	}
}

// collectFunc records a function or method symbol with its signature,
// parameters, results, doc comment and cyclomatic complexity.
func (a *goAnalyzer) collectFunc(pkg *goPkg, d *ast.FuncDecl, q types.Qualifier) {
	obj, ok := a.info.Defs[d.Name].(*types.Func)
	if !ok {
		return
	}
	sig := obj.Type().(*types.Signature)
	file, line := a.relFile(d.Pos())
	_, end := a.relFile(d.End())
	id := goSymbolID(obj)
	if d.Name.Name == "init" || d.Name.Name == "_" {
		id = fmt.Sprintf("%s@%s:%d", id, file, line)
	}
	s := &Symbol{
		ID: id, Name: obj.Name(), Kind: KindFunc, Language: "go", File: file, Line: line, EndLine: end,
		Package: pkg.importPath, Exported: obj.Exported(), Qualified: obj.Name(),
		Params:     tupleParams(sig.Params(), q, sig.Variadic()),
		Results:    tupleParams(sig.Results(), q, false),
		Signature:  "func " + obj.Name() + strings.TrimPrefix(types.TypeString(sig, q), "func"),
		Complexity: cyclomatic(d.Body),
	}
	s.Params = patchInvalid(s.Params, astParams(d.Type.Params))
	s.Results = patchInvalid(s.Results, astParams(d.Type.Results))
	if d.Doc != nil {
		s.Doc = strings.TrimSpace(d.Doc.Text())
	}
	if recv := sig.Recv(); recv != nil {
		s.Kind = KindMethod
		rt := recv.Type()
		ptr := ""
		if p, ok := rt.(*types.Pointer); ok {
			rt, ptr = p.Elem(), "*"
		}
		if named, ok := rt.(*types.Named); ok {
			s.Receiver = ptr + named.Obj().Name()
			s.Qualified = named.Obj().Name() + "." + obj.Name()
			s.Parent = "go:" + named.Obj().Pkg().Path() + "." + named.Obj().Name()
		}
		s.Signature = fmt.Sprintf("func (%s %s) %s%s", recv.Name(), types.TypeString(recv.Type(), q), obj.Name(), strings.TrimPrefix(types.TypeString(sig, q), "func"))
	}
	if strings.Contains(s.Signature, "invalid type") {
		s.Signature = strings.TrimSpace(types.ExprString(d.Type))
		if d.Recv != nil {
			s.Signature = "func (" + paramList(astParams(d.Recv)) + ") " + d.Name.Name + strings.TrimPrefix(s.Signature, "func")
		} else {
			s.Signature = "func " + d.Name.Name + strings.TrimPrefix(s.Signature, "func")
		}
	}
	a.ix.addSymbol(s)
}

// collectType records a named type: struct fields, interface methods
// (as abstract method symbols) and the type's method set.
func (a *goAnalyzer) collectType(pkg *goPkg, gd *ast.GenDecl, ts *ast.TypeSpec, q types.Qualifier) {
	obj, ok := a.info.Defs[ts.Name].(*types.TypeName)
	if !ok {
		return
	}
	file, line := a.relFile(ts.Pos())
	_, end := a.relFile(ts.End())
	id := "go:" + pkg.importPath + "." + obj.Name()
	s := &Symbol{
		ID: id, Name: obj.Name(), Qualified: obj.Name(), Kind: KindType, Language: "go", File: file, Line: line,
		EndLine: end, Package: pkg.importPath, Exported: obj.Exported(),
	}
	doc := ts.Doc
	if doc == nil && len(gd.Specs) == 1 {
		doc = gd.Doc
	}
	if doc != nil {
		s.Doc = strings.TrimSpace(doc.Text())
	}
	under := obj.Type().Underlying()
	s.Signature = "type " + obj.Name() + " " + types.TypeString(under, q)
	switch u := under.(type) {
	case *types.Struct:
		s.Kind = KindStruct
		s.Signature = "type " + obj.Name() + " struct"
		var srcFields []Param
		if st, ok := ts.Type.(*ast.StructType); ok {
			srcFields = astParams(st.Fields)
		}
		defer func() {
			if len(srcFields) == len(s.Fields) {
				for i := range s.Fields {
					if strings.Contains(s.Fields[i].Type, "invalid type") {
						s.Fields[i].Type = srcFields[i].Type
					}
				}
			}
		}()
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			name := f.Name()
			if f.Embedded() {
				name = "(embedded)"
			}
			s.Fields = append(s.Fields, Param{Name: name, Type: types.TypeString(f.Type(), q)})
		}
	case *types.Interface:
		s.Kind = KindInterface
		s.Signature = "type " + obj.Name() + " interface"
		for i := 0; i < u.NumExplicitMethods(); i++ {
			m := u.ExplicitMethod(i)
			sig := m.Type().(*types.Signature)
			mfile, mline := a.relFile(m.Pos())
			ms := &Symbol{
				ID: goSymbolID(m), Name: m.Name(), Qualified: obj.Name() + "." + m.Name(), Kind: KindMethod,
				Language: "go", File: mfile, Line: mline, EndLine: mline, Package: pkg.importPath,
				Exported: m.Exported(), Receiver: obj.Name(), Parent: id, Abstract: true,
				Params:    tupleParams(sig.Params(), q, sig.Variadic()),
				Results:   tupleParams(sig.Results(), q, false),
				Signature: m.Name() + strings.TrimPrefix(types.TypeString(sig, q), "func"),
			}
			a.ix.addSymbol(ms)
			s.Methods = append(s.Methods, ms.ID)
		}
	}
	if named, ok := obj.Type().(*types.Named); ok {
		for i := 0; i < named.NumMethods(); i++ {
			s.Methods = append(s.Methods, goSymbolID(named.Method(i)))
		}
	}
	a.ix.addSymbol(s)
}

// calleeObject resolves the function object a call targets, if any.
func (a *goAnalyzer) calleeObject(fun ast.Expr) types.Object {
	switch f := ast.Unparen(fun).(type) {
	case *ast.Ident:
		return a.info.Uses[f]
	case *ast.SelectorExpr:
		if sel, ok := a.info.Selections[f]; ok {
			return sel.Obj()
		}
		return a.info.Uses[f.Sel]
	case *ast.IndexExpr:
		return a.calleeObject(f.X)
	case *ast.IndexListExpr:
		return a.calleeObject(f.X)
	}
	return nil
}

// externalName names a call into code outside the repo, e.g. "fmt.Println".
func (a *goAnalyzer) externalName(fun ast.Expr) (id, label string) {
	switch f := ast.Unparen(fun).(type) {
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok {
			if pn, ok := a.info.Uses[x].(*types.PkgName); ok {
				p := pn.Imported().Path()
				return "ext:" + p + "." + f.Sel.Name, pn.Imported().Name() + "." + f.Sel.Name
			}
		}
		if obj := a.calleeObject(f); obj != nil && obj.Pkg() != nil {
			if fn, ok := obj.(*types.Func); ok {
				return "ext:" + fn.FullName(), shortFull(fn)
			}
		}
		return "", ""
	case *ast.Ident:
		if fn, ok := a.info.Uses[f].(*types.Func); ok && fn.Pkg() != nil {
			return "ext:" + fn.FullName(), fn.Name()
		}
	}
	return "", ""
}

// shortFull renders an external function as Type.Method or pkg.Func for
// display.
func shortFull(fn *types.Func) string {
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		t := sig.Recv().Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		if n, ok := t.(*types.Named); ok {
			return n.Obj().Name() + "." + fn.Name()
		}
	}
	return fn.Pkg().Name() + "." + fn.Name()
}

// symbolForFunc returns the index ID of fn (or of its generic origin) when
// fn is declared in the repository, and "" otherwise.
func (a *goAnalyzer) symbolForFunc(fn *types.Func) string {
	if fn == nil {
		return ""
	}
	if o := fn.Origin(); o != nil {
		fn = o
	}
	id := goSymbolID(fn)
	if _, ok := a.ix.Symbols[id]; ok {
		return id
	}
	return ""
}

// collectCalls walks every function body in a package and records its
// outgoing edges.
func (a *goAnalyzer) collectCalls(pkg *goPkg) {
	for _, f := range pkg.files {
		a.collectInitializers(f)
		for _, decl := range f.Decls {
			d, ok := decl.(*ast.FuncDecl)
			if !ok || d.Body == nil {
				continue
			}
			obj, ok := a.info.Defs[d.Name].(*types.Func)
			if !ok {
				continue
			}
			from := goSymbolID(obj)
			if d.Name.Name == "init" || d.Name.Name == "_" {
				file, line := a.relFile(d.Pos())
				from = fmt.Sprintf("%s@%s:%d", from, file, line)
			}
			a.walkBody(from, d.Body)
		}
	}
}

// collectInitializers records the calls in a file's package-level var
// initializers (`var x = compute()`), which run when the package is
// initialized, as made by the file's <module> symbol. The symbol is added
// only when there are such calls.
func (a *goAnalyzer) collectInitializers(f *ast.File) {
	file, _ := a.relFile(f.Pos())
	end := a.fset.Position(f.End()).Line
	mod := newModuleSymbol(file, "go", end)
	before := len(a.ix.Edges)
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			for _, v := range spec.(*ast.ValueSpec).Values {
				a.walkBody(mod.ID, v)
			}
		}
	}
	if len(a.ix.Edges) > before {
		a.ix.Symbols[mod.ID] = mod
	}
}

// walkBody records the edges leaving one function body: calls (including
// go and defer), calls into external packages, and functions or methods
// referenced as values (callbacks, handlers).
func (a *goAnalyzer) walkBody(from string, body ast.Node) {
	callFuns := map[ast.Expr]EdgeKind{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.GoStmt:
			callFuns[ast.Unparen(s.Call.Fun)] = EdgeGo
		case *ast.DeferStmt:
			callFuns[ast.Unparen(s.Call.Fun)] = EdgeDefer
		case *ast.CallExpr:
			fun := ast.Unparen(s.Fun)
			if _, ok := callFuns[fun]; !ok {
				callFuns[fun] = EdgeCall
			}
		}
		return true
	})
	calleeIdents := map[*ast.Ident]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			fun := ast.Unparen(x.Fun)
			kind := callFuns[fun]
			_, line := a.relFile(x.Pos())
			switch f := fun.(type) {
			case *ast.Ident:
				calleeIdents[f] = true
			case *ast.SelectorExpr:
				calleeIdents[f.Sel] = true
			}
			obj := a.calleeObject(fun)
			if fn, ok := obj.(*types.Func); ok {
				if to := a.symbolForFunc(fn); to != "" {
					a.ix.addEdge(Edge{From: from, To: to, Kind: kind, Lines: []int{line}}, a.merge)
					return true
				}
			}
			if _, ok := obj.(*types.Builtin); ok {
				return true
			}
			if _, ok := obj.(*types.TypeName); ok {
				return true // conversion
			}
			if id, label := a.externalName(fun); id != "" {
				if _, ok := a.ix.Symbols[id]; !ok {
					pkgPath := strings.TrimPrefix(id, "ext:")
					if i := strings.LastIndex(pkgPath, "."); i > 0 {
						pkgPath = pkgPath[:i]
					}
					a.ix.addSymbol(&Symbol{ID: id, Name: label, Qualified: label, Kind: KindExternal, Language: "go", Package: pkgPath})
				}
				a.ix.addEdge(Edge{From: from, To: id, Kind: kind, Lines: []int{line}}, a.merge)
			}
		case *ast.Ident:
			if calleeIdents[x] {
				return true
			}
			if fn, ok := a.info.Uses[x].(*types.Func); ok {
				if to := a.symbolForFunc(fn); to != "" && to != from {
					_, line := a.relFile(x.Pos())
					a.ix.addEdge(Edge{From: from, To: to, Kind: EdgeRef, Lines: []int{line}}, a.merge)
				}
			}
		case *ast.SelectorExpr:
			// Method values like s.handle passed as callbacks.
			if calleeIdents[x.Sel] {
				return true
			}
			if sel, ok := a.info.Selections[x]; ok && sel.Kind() == types.MethodVal {
				if fn, ok := sel.Obj().(*types.Func); ok {
					// Only a reference if this selector isn't itself being called.
					if _, called := callFuns[x]; !called {
						if to := a.symbolForFunc(fn); to != "" && to != from {
							_, line := a.relFile(x.Pos())
							a.ix.addEdge(Edge{From: from, To: to, Kind: EdgeRef, Lines: []int{line}}, a.merge)
						}
					}
				}
				calleeIdents[x.Sel] = true
			}
		}
		return true
	})
}

// linkImplementations connects interface methods to concrete methods and
// types to the interfaces they satisfy.
func (a *goAnalyzer) linkImplementations() {
	type iface struct {
		sym *Symbol
		t   *types.Interface
	}
	var ifaces []iface
	var concretes []*types.Named
	for _, pkg := range a.pkgs {
		if pkg.types == nil {
			continue
		}
		scope := pkg.types.Scope()
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok || named.TypeParams().Len() > 0 {
				continue
			}
			if it, ok := named.Underlying().(*types.Interface); ok {
				if it.NumMethods() == 0 {
					continue
				}
				if sym := a.ix.Symbols["go:"+pkg.importPath+"."+name]; sym != nil {
					ifaces = append(ifaces, iface{sym, it})
				}
				continue
			}
			concretes = append(concretes, named)
		}
	}
	// Generic types take part through the instantiations the code uses:
	// Repo[int] is matched against intRepo, memRepo[int] against Repo[int].
	seen := map[string]bool{}
	for _, inst := range a.info.Instances {
		named, ok := inst.Type.(*types.Named)
		if !ok || named.Obj().Pkg() == nil || seen[types.TypeString(named, nil)] {
			continue
		}
		seen[types.TypeString(named, nil)] = true
		if it, ok := named.Underlying().(*types.Interface); ok {
			if sym := a.ix.Symbols["go:"+named.Obj().Pkg().Path()+"."+named.Obj().Name()]; sym != nil && it.NumMethods() > 0 {
				ifaces = append(ifaces, iface{sym, it})
			}
			continue
		}
		concretes = append(concretes, named)
	}
	if len(ifaces)*len(concretes) > 4_000_000 {
		return
	}
	for _, c := range concretes {
		csym := a.ix.Symbols["go:"+c.Obj().Pkg().Path()+"."+c.Obj().Name()]
		if csym == nil {
			continue
		}
		for _, in := range ifaces {
			var impl types.Type
			switch {
			case types.Implements(c, in.t):
				impl = c
			case types.Implements(types.NewPointer(c), in.t):
				impl = types.NewPointer(c)
			default:
				continue
			}
			csym.Implements = appendUnique(csym.Implements, in.sym.ID)
			a.ix.addEdge(Edge{From: csym.ID, To: in.sym.ID, Kind: EdgeImplement}, a.merge)
			mset := types.NewMethodSet(impl)
			for i := 0; i < in.t.NumMethods(); i++ {
				im := in.t.Method(i)
				sel := mset.Lookup(im.Pkg(), im.Name())
				if sel == nil {
					continue
				}
				to := a.symbolForFunc(sel.Obj().(*types.Func))
				from := goSymbolID(im.Origin())
				if to != "" && a.ix.Symbols[from] != nil {
					a.ix.addEdge(Edge{From: from, To: to, Kind: EdgeDispatch}, a.merge)
				}
			}
		}
	}
}

// cyclomatic approximates McCabe complexity of a function body.
func cyclomatic(body *ast.BlockStmt) int {
	if body == nil {
		return 0
	}
	c := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			c++
		case *ast.CaseClause:
			if x.List != nil {
				c++
			}
		case *ast.CommClause:
			if x.Comm != nil {
				c++
			}
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}
