package analysis

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var names []string
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return root, names
}

func testLang(p string) string {
	switch path.Ext(p) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".cs":
		return "csharp"
	case ".kt":
		return "kotlin"
	case ".rs":
		return "rust"
	case ".mod":
		return "go.mod"
	}
	return "plaintext"
}

var goFixture = map[string]string{
	"go.mod": "module example.com/app\n\ngo 1.22\n",
	"main.go": `package main

import (
	"fmt"
	"example.com/app/store"
)

func main() {
	s := store.New()
	run(s)
	go worker(s)
	fmt.Println("done")
}

func run(r store.Reader) {
	defer cleanup()
	r.Get("k")
	register(handle)
}

func worker(s *store.Mem) { s.Get("x") }
func cleanup()            {}
func handle()             {}
func register(f func())   { f() }
`,
	"web.go": "package main\n\nimport \"net/http\"\n\ntype srv struct{ c *http.Client }\n\nfunc (s *srv) do(r *http.Request) {}\n\nfunc serve(w http.ResponseWriter, r *http.Request) error { return nil }\n",
	"store/store.go": `package store

// Reader reads values.
type Reader interface {
	Get(key string) (string, bool)
}

// Mem is an in-memory Reader.
type Mem struct {
	data map[string]string
	next *Mem
}

func New() *Mem { return &Mem{data: map[string]string{}} }

// Get returns a value.
func (m *Mem) Get(key string) (string, bool) {
	v, ok := m.data[key]
	return v, ok
}
`,
}

func hasEdge(ix *Index, from, to string, kind EdgeKind) bool {
	for _, e := range ix.Callees(from) {
		if e.To == to && e.Kind == kind {
			return true
		}
	}
	return false
}

func TestGoAnalysis(t *testing.T) {
	root, files := writeTree(t, goFixture)
	ix := Build(root, files, testLang)

	main := "go:example.com/app.main"
	if _, ok := ix.Symbols[main]; !ok {
		var ids []string
		for id := range ix.Symbols {
			ids = append(ids, id)
		}
		t.Fatalf("missing %s; have %v", main, ids)
	}
	checks := []struct {
		from, to string
		kind     EdgeKind
	}{
		{main, "go:example.com/app/store.New", EdgeCall},
		{main, "go:example.com/app.run", EdgeCall},
		{main, "go:example.com/app.worker", EdgeGo},
		{main, "ext:fmt.Println", EdgeCall},
		{"go:example.com/app.run", "go:example.com/app.cleanup", EdgeDefer},
		{"go:example.com/app.run", "go:(example.com/app/store.Reader).Get", EdgeCall},
		{"go:example.com/app.run", "go:example.com/app.handle", EdgeRef},
		{"go:example.com/app.worker", "go:(*example.com/app/store.Mem).Get", EdgeCall},
		{"go:(example.com/app/store.Reader).Get", "go:(*example.com/app/store.Mem).Get", EdgeDispatch},
		{"go:example.com/app/store.Mem", "go:example.com/app/store.Reader", EdgeImplement},
	}
	for _, c := range checks {
		if !hasEdge(ix, c.from, c.to, c.kind) {
			t.Errorf("missing %s edge %s -> %s (have %+v)", c.kind, c.from, c.to, ix.Callees(c.from))
		}
	}

	get := ix.Symbols["go:(*example.com/app/store.Mem).Get"]
	if get.Kind != KindMethod || get.Receiver != "*Mem" || get.Doc != "Get returns a value." ||
		len(get.Params) != 1 || get.Params[0].Type != "string" || len(get.Results) != 2 {
		t.Errorf("Get symbol = %+v", get)
	}
	if run := ix.Symbols["go:example.com/app.run"]; run.Params[0].Type != "store.Reader" {
		t.Errorf("run params = %+v", run.Params)
	}
	if sv := ix.Symbols["go:example.com/app.serve"]; sv.Params[1].Type != "*http.Request" || sv.Results[0].Type != "error" || strings.Contains(sv.Signature, "invalid") {
		t.Errorf("external types should use source spelling: %+v", sv)
	}
	if do := ix.Symbols["go:(*example.com/app.srv).do"]; do == nil || strings.Contains(do.Signature, "invalid") || !strings.Contains(do.Signature, "(s *srv) do(r *http.Request)") {
		t.Errorf("method signature = %+v", do)
	}
	if st := ix.Symbols["go:example.com/app.srv"]; st.Fields[0].Type != "*http.Client" {
		t.Errorf("struct fields = %+v", st.Fields)
	}
	mem := ix.Symbols["go:example.com/app/store.Mem"]
	if mem.Kind != KindStruct || len(mem.Fields) != 2 || len(mem.Implements) != 1 {
		t.Errorf("Mem symbol = %+v", mem)
	}
	if s := ix.SymbolAt("store/store.go", 19); s == nil || s.Name != "Get" {
		t.Errorf("SymbolAt = %+v", s)
	}
	if len(ix.PkgEdges) != 1 || ix.PkgEdges[0].To != "example.com/app/store" {
		t.Errorf("pkg edges = %+v", ix.PkgEdges)
	}
	if got := ix.Find("Mem.Get"); len(got) != 1 {
		t.Errorf("Find(Mem.Get) = %d results", len(got))
	}
	if got := ix.Find("main.go:10"); len(got) != 1 || got[0].Name != "main" {
		t.Errorf("Find(file:line) = %+v", got)
	}
	if got := ix.Search("get", 10); len(got) == 0 || got[0].Name != "Get" {
		t.Errorf("Search = %+v", got)
	}
}

func TestGraphs(t *testing.T) {
	root, files := writeTree(t, goFixture)
	ix := Build(root, files, testLang)
	memGet := "go:(*example.com/app/store.Mem).Get"

	cg := ix.CallGraph("go:example.com/app.main", CallGraphOptions{Depth: 3})
	if !cg.Nodes[0].Root || len(cg.Nodes) < 5 {
		t.Errorf("callgraph nodes = %+v", cg.Nodes)
	}
	for _, n := range cg.Nodes {
		if n.External {
			t.Errorf("external node leaked without IncludeExternal: %s", n.ID)
		}
	}
	trace := ix.TraceTo(memGet, 5, 0)
	ids := map[string]GraphNode{}
	for _, n := range trace.Nodes {
		ids[n.ID] = n
	}
	for _, want := range []string{"go:example.com/app.main", "go:example.com/app.worker", "go:example.com/app.run"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("trace missing %s: %+v", want, trace.Nodes)
		}
	}
	if !contains(ids["go:example.com/app.main"].Badges, "entry") {
		t.Errorf("main should be an entry point: %+v", ids["go:example.com/app.main"])
	}
	impact := ix.Reach([]string{memGet}, 3, nil)
	if impact.Dist["go:example.com/app.main"] == 0 || impact.Dist[memGet] != 0 {
		t.Errorf("impact = %v", impact.Dist)
	}
	// Each caller points one step closer to the change, ending at it.
	for id := "go:example.com/app.main"; id != memGet; id = impact.Via[id] {
		if impact.Via[id] == "" || impact.Dist[impact.Via[id]] != impact.Dist[id]-1 {
			t.Fatalf("via chain broken at %s: %v", id, impact.Via)
		}
	}
	deps := ix.PackageGraph(true, "")
	if len(deps.Edges) < 2 {
		t.Errorf("deps = %+v", deps)
	}
	types := ix.TypeGraph("example.com/app/store")
	if len(types.Nodes) != 2 || len(types.Edges) < 1 {
		t.Errorf("types = %+v", types)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestGoFuncFlow(t *testing.T) {
	root, _ := writeTree(t, map[string]string{"f.go": `package f

func F(xs []int, limit int) (sum int, err error) {
	if limit < 0 {
		return 0, nil
	}
	defer done()
outer:
	for _, x := range xs {
		switch {
		case x > limit:
			break outer
		case x < 0:
			continue
		default:
			sum += x
		}
	}
	if sum > 100 {
		panic("too big")
	}
	return sum, nil
}

func done() {}
`})
	fl, err := GoFuncFlow(root, "f.go", 4)
	if err != nil {
		t.Fatal(err)
	}
	if fl.Name != "F" || len(fl.Inputs) != 2 || fl.Inputs[0].Type != "[]int" || len(fl.Outputs) != 2 || fl.Outputs[1].Name != "err" {
		t.Fatalf("flow header = %+v", fl)
	}
	kinds := map[FlowNodeKind]int{}
	for _, n := range fl.Nodes {
		kinds[n.Kind]++
	}
	for k, want := range map[FlowNodeKind]int{FlowStart: 1, FlowEnd: 1, FlowCond: 2, FlowLoop: 1, FlowSwitch: 1, FlowCase: 3, FlowReturn: 2, FlowDefer: 1, FlowPanic: 1} {
		if kinds[k] != want {
			t.Errorf("kind %s count = %d, want %d (%+v)", k, kinds[k], want, fl.Nodes)
		}
	}
	var back, labelled int
	for _, e := range fl.Edges {
		if e.Back {
			back++
		}
		if e.Label == "true" || e.Label == "false" {
			labelled++
		}
	}
	if back < 2 || labelled != 4 {
		t.Errorf("edges back=%d labelled=%d: %+v", back, labelled, fl.Edges)
	}
	if _, err := GoFuncFlow(root, "f.go", 2); err == nil {
		t.Error("expected error outside a function")
	}
}

func TestGenericTypeScript(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"src/api.ts": `import { save } from './db';

// Handles requests.
export class Api {
  constructor(private name: string) {}

  async handle(req: Request): Promise<void> {
    const msg = "call fake() in a string";
    this.validate(req);
    await save(req.body);
  }

  private validate(req: Request) {
    if (!req) { throw new Error("x"); }
  }
}

export const boot = async (port: number) => {
  const api = new Api("x");
  return api.handle(null as any);
};
`,
		"src/db.ts": `export async function save(data: unknown, retries = 3): Promise<boolean> {
  return true;
}

function fake() {}
`,
	})
	ix := Build(root, files, testLang)
	api := ix.Symbols["h:src/api.ts#Api"]
	if api == nil || api.Kind != KindClass || api.Doc != "Handles requests." || api.EndLine != 16 {
		t.Fatalf("Api = %+v", api)
	}
	handle := ix.Symbols["h:src/api.ts#Api.handle"]
	if handle == nil || handle.Kind != KindMethod || handle.Parent != api.ID || len(handle.Params) != 1 || handle.Params[0].Type != "Request" {
		t.Fatalf("handle = %+v", handle)
	}
	save := ix.Symbols["h:src/db.ts#save"]
	if save == nil || len(save.Params) != 2 || save.Params[1].Name != "retries" || !save.Exported {
		t.Fatalf("save = %+v", save)
	}
	if !hasEdge(ix, handle.ID, save.ID, EdgeCall) {
		t.Errorf("handle should call save: %+v", ix.Callees(handle.ID))
	}
	if !hasEdge(ix, handle.ID, "h:src/api.ts#Api.validate", EdgeCall) {
		t.Errorf("handle should call validate: %+v", ix.Callees(handle.ID))
	}
	if hasEdge(ix, handle.ID, "h:src/db.ts#fake", EdgeCall) {
		t.Error("calls inside string literals must be ignored")
	}
	if !hasEdge(ix, "h:src/api.ts#boot", handle.ID, EdgeCall) {
		t.Errorf("boot should call handle: %+v", ix.Callees("h:src/api.ts#boot"))
	}
	if len(ix.PkgEdges) != 0 {
		t.Errorf("same-dir import should not create a pkg edge: %+v", ix.PkgEdges)
	}
}

func TestGenericPython(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"app/service.py": `from .repo import load


class Service:
    """Coordinates work."""

    def __init__(self, name):
        self.name = name

    def run(self, items: list, *, dry=False):
        for i in items:
            if i:
                self.step(i)
        return load(self.name)

    def step(self, i):
        # helper() in a comment
        pass
`,
		"app/repo.py": `def load(name):
    return name


def helper():
    pass
`,
	})
	ix := Build(root, files, testLang)
	run := ix.Symbols["h:app/service.py#Service.run"]
	if run == nil || run.EndLine != 14 || run.Kind != KindMethod {
		t.Fatalf("run = %+v", run)
	}
	if len(run.Params) != 2 || run.Params[0].Name != "items" || run.Params[0].Type != "list" {
		t.Errorf("run params = %+v", run.Params)
	}
	if svc := ix.Symbols["h:app/service.py#Service"]; svc == nil || svc.Doc != "Coordinates work." {
		t.Errorf("Service = %+v", svc)
	}
	if !hasEdge(ix, run.ID, "h:app/repo.py#load", EdgeCall) || !hasEdge(ix, run.ID, "h:app/service.py#Service.step", EdgeCall) {
		t.Errorf("run callees = %+v", ix.Callees(run.ID))
	}
	if hasEdge(ix, "h:app/service.py#Service.step", "h:app/repo.py#helper", EdgeCall) {
		t.Error("calls in comments must be ignored")
	}
	if run.Complexity < 3 {
		t.Errorf("complexity = %d", run.Complexity)
	}
}

func TestMaskSource(t *testing.T) {
	src := "a := \"x(y)\" // z()\n/* w() */ b()"
	got := maskSource(src, specs["javascript"])
	if strings.Contains(got, "x(") || strings.Contains(got, "z(") || strings.Contains(got, "w(") || !strings.Contains(got, "b()") {
		t.Errorf("masked = %q", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Error("newlines must be preserved")
	}
	// Template literal text is masked, its ${…} interpolations are code.
	got = maskSource("s = `call fake() ${total(o, { a: 1 })} and ${x}` + done()", specs["typescript"])
	if strings.Contains(got, "fake(") || !strings.Contains(got, "total(o, { a: 1 })") || !strings.Contains(got, "done()") {
		t.Errorf("template masked = %q", got)
	}
}

// Python f-string replacement fields are code; their text and {{ }} are
// not (#64).
func TestMaskPythonFStrings(t *testing.T) {
	py := specs["python"]
	cases := []struct{ src, keep, drop string }{
		{`msg = f"total {fmt(o)} of fake()"`, "fmt(o)", "fake("},
		{`msg = F'{a(1)}{b(2)}'`, "b(2)", ""},
		{`msg = rf"{{literal()}} {real(x)}"`, "real(x)", "literal("},
		{"msg = f\"\"\"\n  {total(o)}\n  fake()\n\"\"\"", "total(o)", "fake("},
		{`plain = "not {code(x)}"`, "", "code("},
		{`call(f"{nested({'k': 1})}")`, "nested({", ""},
	}
	for _, c := range cases {
		got := maskSource(c.src, py)
		if c.keep != "" && !strings.Contains(got, c.keep) || c.drop != "" && strings.Contains(got, c.drop) {
			t.Errorf("%q masked = %q", c.src, got)
		}
		if strings.Count(got, "\n") != strings.Count(c.src, "\n") {
			t.Errorf("%q: newlines must be preserved", c.src)
		}
	}
	// A call only inside an f-string is still an edge.
	root, files := writeTree(t, map[string]string{"app.py": "def fmt(x):\n    return x\n\ndef show(o):\n    return f\"total: {fmt(o)}\"\n"})
	if ix := Build(root, files, testLang); !hasEdge(ix, "h:app.py#show", "h:app.py#fmt", EdgeCall) {
		t.Errorf("show should call fmt: %+v", ix.Callees("h:app.py#show"))
	}
}

func TestResolveImport(t *testing.T) {
	files := map[string]bool{"src/db.ts": true, "src/lib/index.ts": true, "pkg/mod.py": true, "pkg/sub/__init__.py": true}
	cases := []struct{ from, spec, lang, want string }{
		{"src/api.ts", "./db", "typescript", "src/db.ts"},
		{"src/api.ts", "./lib", "typescript", "src/lib/index.ts"},
		{"pkg/a.py", ".mod", "python", "pkg/mod.py"},
		{"main.py", "pkg.sub", "python", "pkg/sub/__init__.py"},
		{"src/api.ts", "react", "typescript", ""},
	}
	for _, c := range cases {
		if got := resolveImport(c.from, c.spec, specs[c.lang], files); got != c.want {
			t.Errorf("resolveImport(%s, %s) = %q, want %q", c.from, c.spec, got, c.want)
		}
	}
}

func TestGenericReactComponents(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"src/fmt.ts": `export function money(c: number) { return '$' + c; }
export function total(n: number) { return n * 2; }
export function split(n: number): { whole: number; cents: number } {
  const whole = total(n);
  return { whole, cents: 0 };
}
export async function load(): Promise<{ ok: boolean }> {
  return { ok: !!total(1) };
}
`,
		"src/Summary.tsx": `import { money, total } from './fmt';

export function Summary({ qty }: { qty: number }) {
  const label = useLabel<Props>(qty);
  return (
    <div className={label}>
      <b>{money(total(qty))}</b>
    </div>
  );
}

export const Row = ({ qty }: { qty: number }) => (
  <span>{total(qty)}</span>
);

export const Double = (n: number) => total(n) * 2;

export function useLabel<T>(n: number) { return String(n); }
`,
		"src/App.tsx": `import { Summary, Row } from './Summary';

export function App() {
  return (
    <main>
      <Summary qty={1} />
      <ui.Row qty={2}></ui.Row>
      <Route component={Page} />
    </main>
  );
}

export function Page() {
  const onClick = () => {
    total(3);
  };
  return <button onClick={onClick}>go</button>;
}

const Memoized = memo(Page);
`,
	})
	ix := Build(root, files, testLang)
	sym := func(id string) *Symbol {
		s := ix.Symbols[id]
		if s == nil {
			t.Fatalf("missing %s", id)
		}
		return s
	}
	// An object return type is not the body, even with ; inside it.
	if s := sym("h:src/fmt.ts#split"); s.EndLine != 6 {
		t.Errorf("split ends at %d, want 6", s.EndLine)
	}
	if s := sym("h:src/fmt.ts#load"); s.EndLine != 9 {
		t.Errorf("load ends at %d, want 9", s.EndLine)
	}
	// Destructured props don't end the function on its first line.
	if s := sym("h:src/Summary.tsx#Summary"); s.EndLine != 10 {
		t.Errorf("Summary ends at %d, want 10", s.EndLine)
	}
	// An arrow function with a JSX expression body ends where it closes;
	// a one-line expression body ends on its line.
	if s := sym("h:src/Summary.tsx#Row"); s.EndLine != 14 {
		t.Errorf("Row ends at %d, want 14", s.EndLine)
	}
	if s := sym("h:src/Summary.tsx#Double"); s.EndLine != 16 {
		t.Errorf("Double ends at %d, want 16", s.EndLine)
	}
	for _, e := range [][3]string{
		{"h:src/fmt.ts#split", "h:src/fmt.ts#total", string(EdgeCall)},
		{"h:src/fmt.ts#load", "h:src/fmt.ts#total", string(EdgeCall)},
		{"h:src/Summary.tsx#Summary", "h:src/fmt.ts#money", string(EdgeCall)},
		{"h:src/Summary.tsx#Summary", "h:src/fmt.ts#total", string(EdgeCall)},
		{"h:src/Summary.tsx#Row", "h:src/fmt.ts#total", string(EdgeCall)},
		{"h:src/Summary.tsx#Double", "h:src/fmt.ts#total", string(EdgeCall)},
		// JSX renders count as calls, <ns.Row> included.
		{"h:src/App.tsx#App", "h:src/Summary.tsx#Summary", string(EdgeCall)},
		{"h:src/App.tsx#App", "h:src/Summary.tsx#Row", string(EdgeCall)},
		// A component passed as a prop is a reference.
		{"h:src/App.tsx#App", "h:src/App.tsx#Page", string(EdgeRef)},
		// Calls in a nested function belong to it.
		{"h:src/App.tsx#onClick", "h:src/fmt.ts#total", string(EdgeCall)},
	} {
		if !hasEdge(ix, e[0], e[1], EdgeKind(e[2])) {
			t.Errorf("missing %s edge %s → %s: %+v", e[2], e[0], e[1], ix.Callees(e[0]))
		}
	}
	if hasEdge(ix, "h:src/App.tsx#Page", "h:src/fmt.ts#total", EdgeCall) {
		t.Error("a nested function's calls must not be counted for its parent too")
	}
	// A generic type argument is not a JSX element.
	for _, e := range ix.Callees("h:src/Summary.tsx#Summary") {
		if strings.HasSuffix(e.To, "#Props") {
			t.Errorf("generic treated as JSX: %+v", e)
		}
	}
}

func TestGenericBuiltinMethodNames(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"store.py": `class OrderStore:
    def get(self, key):
        return self.items.get(key)

    def lookup(self, key):
        return self.get(key)
`,
		"api.py": `def handle(data, store):
    name = data.get("name")
    order = store.get(name)
    return order

def other(cache):
    return cache.get("x")
`,
	})
	ix := Build(root, files, testLang)
	get := "h:store.py#OrderStore.get"
	callers := map[string]bool{}
	for _, e := range ix.Callers(get) {
		callers[e.From] = true
	}
	// store.get looks like an OrderStore; self.get is the class's own; data
	// and cache could be anything, and get is a dict method.
	if !callers["h:api.py#handle"] || !callers["h:store.py#OrderStore.lookup"] || callers["h:api.py#other"] || len(callers) != 2 {
		t.Errorf("callers of OrderStore.get = %v", callers)
	}
	if !receiverMatches("order_store", "OrderStore") || receiverMatches("data", "OrderStore") {
		t.Error("receiverMatches")
	}
}

// TypeScript definitions the method and function patterns used to miss
// (#61): constructors, bodies on the signature line, class-field arrows,
// object-literal methods and anonymous default exports.
func TestGenericTypeScriptDefinitions(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"src/store.ts": `export class SqlStore {
  constructor(private db: Db) { this.init() }
  init() { return connect() }
  save(o: Order) { return this.write(o) }
  write(o: Order) {
    return 1
  }
  onClick = () => {
    this.save(null)
  }
  handle = async (e: Event) => this.write(e)
  onChange: (v: string) => void
  delete(id: string) { return drop(id) }
}

export function connect() {}
export function drop(id: string) {}
`,
		"src/worker.ts": `export default {
  async fetch(req: Request, env: Env) {
    return route(req)
  },
  scheduled: async (ev: Event) => {
    sweep()
  },
}

const api = {
  list() { return route(null) },
  limit: 10,
}

function route(req: Request) { return new SqlStore(db) }
function sweep() {}
`,
		"src/main.ts": `export default function () {
  sweep()
}
`,
		"src/types.ts": `export interface Saver {
  save: (o: Order) => void;
  onChange(v: string): void;
}
`,
	})
	ix := Build(root, files, testLang)
	want := map[string]SymbolKind{
		"h:src/store.ts#SqlStore.constructor": KindMethod,
		"h:src/store.ts#SqlStore.init":        KindMethod,
		"h:src/store.ts#SqlStore.save":        KindMethod,
		"h:src/store.ts#SqlStore.onClick":     KindMethod,
		"h:src/store.ts#SqlStore.handle":      KindMethod,
		"h:src/store.ts#SqlStore.delete":      KindMethod,
		"h:src/worker.ts#default.fetch":       KindMethod,
		"h:src/worker.ts#default.scheduled":   KindMethod,
		"h:src/worker.ts#api.list":            KindMethod,
		"h:src/main.ts#default":               KindFunc,
	}
	for id, kind := range want {
		if s := ix.Symbols[id]; s == nil || s.Kind != kind {
			t.Errorf("%s = %+v, want a %s", id, s, kind)
		}
	}
	if s := ix.Symbols["h:src/store.ts#SqlStore.write"]; s == nil || s.EndLine != 7 {
		t.Errorf("write = %+v", s)
	}
	if s := ix.Symbols["h:src/store.ts#SqlStore.save"]; s == nil || s.EndLine != 4 {
		t.Errorf("one-line save = %+v", s)
	}
	// Typed fields and interface members have no body: not functions. The
	// object literals themselves aren't symbols either.
	for _, id := range []string{"h:src/store.ts#SqlStore.onChange", "h:src/types.ts#Saver.save", "h:src/types.ts#Saver.onChange",
		"h:src/worker.ts#default", "h:src/worker.ts#api", "h:src/worker.ts#api.limit"} {
		if s := ix.Symbols[id]; s != nil {
			t.Errorf("%s should not be a symbol: %+v", id, s)
		}
	}
	edges := [][2]string{
		{"h:src/store.ts#SqlStore.constructor", "h:src/store.ts#SqlStore.init"},
		{"h:src/store.ts#SqlStore.init", "h:src/store.ts#connect"},
		{"h:src/store.ts#SqlStore.save", "h:src/store.ts#SqlStore.write"},
		{"h:src/store.ts#SqlStore.onClick", "h:src/store.ts#SqlStore.save"},
		{"h:src/store.ts#SqlStore.handle", "h:src/store.ts#SqlStore.write"},
		{"h:src/store.ts#SqlStore.delete", "h:src/store.ts#drop"},
		{"h:src/worker.ts#default.fetch", "h:src/worker.ts#route"},
		{"h:src/worker.ts#default.scheduled", "h:src/worker.ts#sweep"},
		{"h:src/worker.ts#api.list", "h:src/worker.ts#route"},
		{"h:src/main.ts#default", "h:src/worker.ts#sweep"},
		// new SqlStore() reaches the constructor, not just the class.
		{"h:src/worker.ts#route", "h:src/store.ts#SqlStore.constructor"},
	}
	for _, e := range edges {
		if !hasEdge(ix, e[0], e[1], EdgeCall) {
			t.Errorf("%s should call %s: %+v", e[0], e[1], ix.Callees(e[0]))
		}
	}
}

// Optional chaining keeps its receiver, super calls go to the parent class,
// and multi-line imports count as dependencies (#62).
func TestGenericTypeScriptEdges(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"src/base.ts": `export class Base {
  handle() {
    return 1
  }
}
`,
		"src/child.ts": `import { Base } from './base'

export class Child extends Base {
  handle() {
    super.handle()
    return obj?.helper()
  }
}

export class Orphan extends External {
  handle() {
    return super.handle()
  }
}

export class Widget {
  helper() {
    return 2
  }
  render() {
    return this?.helper()
  }
}
`,
		"src/lib/helper.ts": `export function helper() {}
`,
		"src/app/main.ts": `import {
  helper,
} from '../lib/helper'
export {
  Base,
} from '../base'
`,
	})
	ix := Build(root, files, testLang)
	child := "h:src/child.ts#Child.handle"
	if !hasEdge(ix, child, "h:src/base.ts#Base.handle", EdgeCall) {
		t.Errorf("super.handle() should call Base.handle: %+v", ix.Callees(child))
	}
	if hasEdge(ix, child, child, EdgeCall) {
		t.Error("super.handle() is not recursion")
	}
	// obj?.helper() is a method on obj, not the free function helper.
	if hasEdge(ix, child, "h:src/lib/helper.ts#helper", EdgeCall) {
		t.Errorf("obj?.helper() linked to the free function: %+v", ix.Callees(child))
	}
	if !hasEdge(ix, "h:src/child.ts#Widget.render", "h:src/child.ts#Widget.helper", EdgeCall) {
		t.Errorf("this?.helper() should call Widget.helper: %+v", ix.Callees("h:src/child.ts#Widget.render"))
	}
	// A parent outside the repo: no edge rather than a wrong one.
	if es := ix.Callees("h:src/child.ts#Orphan.handle"); len(es) != 0 {
		t.Errorf("super into an unknown parent = %+v", es)
	}
	deps := map[string]bool{}
	for _, e := range ix.PkgEdges {
		deps[e.From+" → "+e.To] = true
	}
	if !deps["dir:src/app → dir:src/lib"] || !deps["dir:src/app → dir:src"] {
		t.Errorf("multi-line import and re-export should be dependencies: %v", deps)
	}
}

// One-line and expression-bodied methods in Java, C#, Kotlin and Rust's
// fn new (#63).
func TestGenericOneLineMethods(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"Store.java": `public class Store {
    public Store() {}

    public static Store create() { return new Store(); }

    public void save(Order o) { validate(o); }

    private void validate(Order o) {}

    private void log() {
        audit();
    }

    void audit() {}
}
`,
		"Store.cs": `public class Store
{
    public void Save(Order o) => Validate(o);

    private void Validate(Order o) => Check(o);

    private void Check(Order o)
    {
    }
}
`,
		"Store.kt": `class Store {
    fun save(o: Order) {
        validate(o)
    }
    fun validate(o: Order) = check(o)
    private fun check(o: Order): Boolean {
        return true
    }
    fun total(xs: List<Int>, start: Int = 0) =
        xs.sum() + start
    fun after() = check(Order())
}
`,
		"src/store.rs": `pub struct Store {}

impl Store {
    pub fn new() -> Self {
        Store::init()
    }
    fn init() -> Self { Store {} }
}

pub fn run() {
    let s = Store::new();
}
`,
	})
	ix := Build(root, files, testLang)
	syms := map[string][2]int{ // id → lines
		"h:Store.java#Store.save":     {6, 6},
		"h:Store.java#Store.validate": {8, 8},
		"h:Store.java#Store.log":      {10, 12},
		"h:Store.cs#Store.Save":       {3, 3},
		"h:Store.cs#Store.Validate":   {5, 5},
		"h:Store.cs#Store.Check":      {7, 9},
		"h:Store.kt#Store.validate":   {5, 5},
		"h:Store.kt#Store.check":      {6, 8},
		"h:Store.kt#Store.total":      {9, 10},
		"h:Store.kt#Store.after":      {11, 11},
		"h:src/store.rs#Store.new":    {4, 6},
	}
	for id, lines := range syms {
		s := ix.Symbols[id]
		if s == nil || s.Line != lines[0] || s.EndLine != lines[1] {
			t.Errorf("%s = %+v, want lines %d-%d", id, s, lines[0], lines[1])
		}
	}
	edges := [][2]string{
		{"h:Store.java#Store.save", "h:Store.java#Store.validate"},
		// new Store() reaches the constructor named after the class.
		{"h:Store.java#Store.create", "h:Store.java#Store.Store"},
		{"h:Store.java#Store.log", "h:Store.java#Store.audit"},
		{"h:Store.cs#Store.Save", "h:Store.cs#Store.Validate"},
		{"h:Store.cs#Store.Validate", "h:Store.cs#Store.Check"},
		{"h:Store.kt#Store.validate", "h:Store.kt#Store.check"},
		{"h:src/store.rs#Store.new", "h:src/store.rs#Store.init"},
		{"h:src/store.rs#run", "h:src/store.rs#Store.new"},
	}
	for _, e := range edges {
		if !hasEdge(ix, e[0], e[1], EdgeCall) {
			t.Errorf("%s should call %s: %+v", e[0], e[1], ix.Callees(e[0]))
		}
	}
	// int.Parse and String::new are the platform's, not the repo's.
	root2, files2 := writeTree(t, map[string]string{"Handler.cs": `public class Handler
{
    private static int Parse(string s)
    {
        return int.Parse(s);
    }
}
`})
	if es := Build(root2, files2, testLang).Callees("h:Handler.cs#Handler.Parse"); len(es) != 0 {
		t.Errorf("int.Parse linked to the repository's Parse: %+v", es)
	}
	// check's lines belong to check, not to the expression-bodied validate.
	if hasEdge(ix, "h:Store.kt#Store.total", "h:Store.kt#Store.check", EdgeCall) {
		t.Error("total's extent ran into the next function")
	}
}
