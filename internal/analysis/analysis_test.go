package analysis

import (
	"fmt"
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
	case ".js":
		return "javascript"
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

// Interpolated code in each language's strings stays code; the string text
// around it, and strings that don't interpolate, are masked (#79).
func TestMaskInterpolation(t *testing.T) {
	cases := []struct{ lang, src, keep, drop string }{
		{"ruby", `s = "fake() at #{total(o)}"`, "total(o)", "fake("},
		{"ruby", `s = "#{a("x")} and \#{escaped(1)}"`, "a(", "escaped("},
		{"ruby", `s = 'no #{code(x)}'`, "", "code("},
		{"kotlin", `val s = "fake() ${total(o)}"`, "total(o)", "fake("},
		{"kotlin", `val s = "$name fake()"`, "", "fake("},
		{"csharp", `var s = $"fake() {Total(o)}";`, "Total(o)", "fake("},
		{"csharp", `var s = $@"{Total(o)} {{Literal()}}";`, "Total(o)", "Literal("},
		{"csharp", `var s = @$"{Total(o)}";`, "Total(o)", ""},
		{"csharp", `var s = "{NotCode(o)}";`, "", "NotCode("},
		{"swift", `let s = "fake() \(total(o, f(x)))"`, "total(o, f(x))", "fake("},
		{"swift", `let s = "\\(notCode(x))"`, "", "notCode("},
		{"dart", `var s = 'fake() ${total(o)}';`, "total(o)", "fake("},
		{"scala", `val s = s"fake() ${total(o)}"`, "total(o)", "fake("},
		{"scala", `val s = f"${rate(o)}%.2f"`, "rate(o)", ""},
		{"scala", `val s = "${notCode(o)}"`, "", "notCode("},
		{"php", `$s = "fake() {$o->total()}";`, "total()", "fake("},
		{"php", `$s = 'no {$o->code()}';`, "", "code("},
		{"typescript", "s = `fake() ${total(o)}`", "total(o)", "fake("},
	}
	for _, c := range cases {
		got := maskSource(c.src, specs[c.lang])
		if c.keep != "" && !strings.Contains(got, c.keep) || c.drop != "" && strings.Contains(got, c.drop) {
			t.Errorf("%s %q masked = %q", c.lang, c.src, got)
		}
		if len(got) != len(c.src) {
			t.Errorf("%s %q: masking must keep offsets", c.lang, c.src)
		}
	}
}

// Calls inside interpolation are edges, and a bare Ruby `#{name}` calls a
// method of the caller's own class (#79).
func TestGenericInterpolatedCalls(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"Report.kt": `class Report {
    fun total(o: Int): Int = o
    fun show(o: Int): String = "total: ${total(o)}"
}
`,
		"notifier.rb": `class Notifier
  def self.deliver(order)
    name = "x"
    "sent #{name} at #{timestamp} #{format(order)}"
  end

  def self.timestamp
    Time.now
  end

  def self.format(order)
    order.to_s
  end
end

class Other
  def name
    "o"
  end
end
`,
	})
	lang := func(p string) string {
		switch path.Ext(p) {
		case ".rb":
			return "ruby"
		}
		return testLang(p)
	}
	ix := Build(root, files, lang)
	if !hasEdge(ix, "h:Report.kt#Report.show", "h:Report.kt#Report.total", EdgeCall) {
		t.Errorf("show should call total: %+v", ix.Callees("h:Report.kt#Report.show"))
	}
	deliver := "h:notifier.rb#Notifier.deliver"
	for _, to := range []string{"h:notifier.rb#Notifier.timestamp", "h:notifier.rb#Notifier.format"} {
		if !hasEdge(ix, deliver, to, EdgeCall) {
			t.Errorf("deliver should call %s: %+v", to, ix.Callees(deliver))
		}
	}
	// #{name} is a local variable here; Other#name is another class's.
	if hasEdge(ix, deliver, "h:notifier.rb#Other.name", EdgeCall) {
		t.Errorf("bare #{name} linked to another class's method: %+v", ix.Callees(deliver))
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

// Calls resolve through the calling file's imports before falling back to
// name matching (#65).
func TestGenericTypeScriptImports(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"src/a/fmt.ts": `export function format(x: number) { return String(x) }
`,
		"src/b/fmt.ts": `export function format(s: string) { return s.trim() }
`,
		"src/a/use.ts": `import { format } from '../b/fmt'

export function show() {
  return format(" x ")
}
`,
		"src/b/use.ts": `import { format as fmt } from '../a/fmt'

export function show() {
  return fmt(1)
}
`,
		"src/save.ts": `export default async function persist(data: unknown) {}
`,
		"src/api/list.ts": `export function list() { return [] }
function hidden() {}
export { hidden as reveal }
`,
		"src/api/index.ts": `export * from './list'
export { default as store } from '../save'
export * as raw from './list'
`,
		"src/store.ts": `export class Store {
  static create() { return new Store() }
}
export const api = {
  remove(id: string) {},
}
`,
		"src/cycle/x.ts": `export * from './y'
`,
		"src/cycle/y.ts": `export * from './x'
`,
		"src/legacy.js": `function load() {}
function save() {}
module.exports = { load, store: save }
`,
		"src/main.ts": `import persist from './save'
import * as api from './api'
import { store, raw, reveal } from './api'
import { Store, api as objs } from './store'
import { ghost } from './cycle/x'
const { load, store: put } = require('./legacy')

export function main() {
  persist(1)
  api.list()
  store(2)
  raw.list()
  reveal()
  Store.create()
  objs.remove("id")
  ghost()
  load()
  put()
}

function list() {}
`,
	})
	ix := Build(root, files, testLang)
	edges := []struct{ from, to string }{
		{"h:src/a/use.ts#show", "h:src/b/fmt.ts#format"},
		{"h:src/b/use.ts#show", "h:src/a/fmt.ts#format"},
		{"h:src/main.ts#main", "h:src/save.ts#persist"},
		{"h:src/main.ts#main", "h:src/api/list.ts#list"},
		{"h:src/main.ts#main", "h:src/api/list.ts#hidden"},
		{"h:src/main.ts#main", "h:src/store.ts#Store.create"},
		{"h:src/main.ts#main", "h:src/store.ts#api.remove"},
		{"h:src/main.ts#main", "h:src/legacy.js#load"},
		{"h:src/main.ts#main", "h:src/legacy.js#save"},
	}
	for _, e := range edges {
		if !hasEdge(ix, e.from, e.to, EdgeCall) {
			t.Errorf("%s should call %s: %+v", e.from, e.to, ix.Callees(e.from))
		}
	}
	// Same-named functions: each importer reaches only what it imports.
	if hasEdge(ix, "h:src/a/use.ts#show", "h:src/a/fmt.ts#format", EdgeCall) || hasEdge(ix, "h:src/b/use.ts#show", "h:src/b/fmt.ts#format", EdgeCall) {
		t.Error("a call linked to the same-named function it doesn't import")
	}
	// api.list() goes through the namespace import, not main.ts's own list.
	if hasEdge(ix, "h:src/main.ts#main", "h:src/main.ts#list", EdgeCall) {
		t.Errorf("api.list() linked to the local list: %+v", ix.Callees("h:src/main.ts#main"))
	}
}

func TestNameList(t *testing.T) {
	got := nameList(" a, type B, c as d, default as e, , 1x as y ", " as ")
	want := map[string]string{"a": "a", "B": "B", "c": "d", "default": "e"}
	if len(got) != len(want) {
		t.Fatalf("nameList = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("nameList[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestJSBinderSkipsComments(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"src/a.ts": `export function go() {}
`,
		"src/b.ts": `export function go() {}
`,
		"src/main.ts": `// import { go } from './a'
/* import { go } from './a' */
import { go } from './b'

export function run() { go() }
`,
	})
	ix := Build(root, files, testLang)
	if !hasEdge(ix, "h:src/main.ts#run", "h:src/b.ts#go", EdgeCall) || hasEdge(ix, "h:src/main.ts#run", "h:src/a.ts#go", EdgeCall) {
		t.Errorf("run's callees = %+v", ix.Callees("h:src/main.ts#run"))
	}
}

// Wide trees of barrels resolve, and are walked once per name rather than
// once per path, which for a name none of them exports would be
// width^depth lookups (#65).
func TestGenericTypeScriptBarrelFanOut(t *testing.T) {
	const width, depth = 8, 8
	tree := map[string]string{
		"src/main.ts":     "import { missing, target } from './l0/f0'\n\nexport function run() {\n  missing()\n  target()\n}\n",
		"src/lib/leaf.ts": "export function target() {}\n",
		// Name matching alone would pick this one, in main.ts's directory.
		"src/decoy.ts": "export function target() {}\n",
	}
	for d := range depth {
		for w := range width {
			var sb strings.Builder
			for n := range width {
				next := fmt.Sprintf("../l%d/f%d", d+1, n)
				if d == depth-1 {
					next = "../lib/leaf"
				}
				fmt.Fprintf(&sb, "export * from '%s'\n", next)
			}
			tree[fmt.Sprintf("src/l%d/f%d.ts", d, w)] = sb.String()
		}
	}
	root, files := writeTree(t, tree)
	ix := Build(root, files, testLang)
	if !hasEdge(ix, "h:src/main.ts#run", "h:src/lib/leaf.ts#target", EdgeCall) {
		t.Errorf("run's callees = %+v", ix.Callees("h:src/main.ts#run"))
	}
}

// A miss cut off by the re-export hop limit isn't remembered: the same
// barrel reached by a shorter path still resolves (#65).
func TestGenericTypeScriptHopLimitNotMemoized(t *testing.T) {
	tree := map[string]string{
		"src/lib/leaf.ts": "export function target() {}\n",
		// Name matching alone would pick this one, in main.ts's directory.
		"src/decoy.ts": "export function target() {}\n",
		"src/main.ts":  "import { target } from './hub'\n\nexport function run() { target() }\n",
		"src/hub.ts":   "export * from './c0'\nexport * from './mid'\n",
		"src/mid.ts":   "export * from './lib/leaf'\n",
	}
	// hub (hop 0) → c0 … c6 → mid at the last allowed hop, so leaf is cut off.
	for i := range maxExportHops - 1 {
		next := fmt.Sprintf("./c%d", i+1)
		if i == maxExportHops-2 {
			next = "./mid"
		}
		tree[fmt.Sprintf("src/c%d.ts", i)] = "export * from '" + next + "'\n"
	}
	root, files := writeTree(t, tree)
	ix := Build(root, files, testLang)
	if !hasEdge(ix, "h:src/main.ts#run", "h:src/lib/leaf.ts#target", EdgeCall) {
		t.Errorf("run's callees = %+v", ix.Callees("h:src/main.ts#run"))
	}
}

// Python calls resolve through imports and aliases before falling back to
// name matching (#66). Each package has a same-named decoy next to the
// caller, which name matching alone would pick.
func TestGenericPythonImports(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"orders/lib/util.py":     "def fmt(x):\n    return str(x)\n",
		"orders/lib/__init__.py": "from .util import fmt\n",
		"orders/store.py": `class Store:
    def __init__(self):
        self.items = {}

    @classmethod
    def create(cls):
        return cls()
`,
		"orders/decoy.py": "def fmt(x):\n    pass\n\n\nclass Store:\n    @classmethod\n    def create(cls):\n        pass\n",
		"orders/api.py": `from .lib.util import fmt as f
from .store import Store as S
import orders.lib.util as u
from .lib import util
from .lib import fmt
from orders.store import (
    Store,  # the class
)


def handle(x):
    f(x)
    S()
    u.fmt(x)
    util.fmt(x)
    fmt(x)
    Store.create()
`,
		"orders/sub/decoy.py": "def fmt(x):\n    pass\n",
		"orders/sub/job.py": `from ..lib.util import *


def run(x):
    return fmt(x)
`,
		"orders/sub/plain.py": `import orders.lib.util


def go(x):
    return orders.lib.util.fmt(x)
`,
	})
	ix := Build(root, files, testLang)
	target := "h:orders/lib/util.py#fmt"
	for _, from := range []string{"h:orders/api.py#handle", "h:orders/sub/job.py#run", "h:orders/sub/plain.py#go"} {
		if !hasEdge(ix, from, target, EdgeCall) {
			t.Errorf("%s should call %s: %+v", from, target, ix.Callees(from))
		}
	}
	handle := "h:orders/api.py#handle"
	for _, to := range []string{"h:orders/store.py#Store.__init__", "h:orders/store.py#Store.create"} {
		if !hasEdge(ix, handle, to, EdgeCall) {
			t.Errorf("handle should call %s: %+v", to, ix.Callees(handle))
		}
	}
	for _, from := range []string{handle, "h:orders/sub/job.py#run", "h:orders/sub/plain.py#go"} {
		for _, e := range ix.Callees(from) {
			if strings.Contains(e.To, "decoy") {
				t.Errorf("%s linked to a decoy: %+v", from, e)
			}
		}
	}
}

// A file's top-level code is its <module> symbol, which calls what that
// code calls: TS/JS and Python module statements and Go package-level var
// initializers. Test files, decorators and definitions don't count (#69).
func TestModuleLevelCallers(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.22\n",
		"vars.go": `package m

var total = compute(2)

var handlers = map[string]func(){"a": handleA}

func compute(n int) int { return n }

func handleA() {}

func unused() int { return compute(1) }
`,
		"src/App.tsx": "export function App() { return null }\n",
		"src/main.tsx": `import { App } from './App'
import { render } from './render'

render(<App />)
`,
		"src/render.ts": "export function render(x: unknown) {}\n",
		"src/App.test.tsx": `import { App } from './App'

it('renders', () => { App() })
`,
		"app/cli.py": `import functools


@functools.lru_cache()
def main():
    run()


def run():
    pass


if __name__ == "__main__":
    main()
`,
		"app/quiet.py": "def f():\n    pass\n",
	})
	ix := Build(root, files, testLang)
	for _, e := range []struct{ from, to string }{
		{"mod:vars.go", "go:example.com/m.compute"},
		{"mod:src/main.tsx", "h:src/render.ts#render"},
		{"mod:src/main.tsx", "h:src/App.tsx#App"},
		{"mod:app/cli.py", "h:app/cli.py#main"},
	} {
		if !hasEdge(ix, e.from, e.to, EdgeCall) {
			t.Errorf("%s should call %s: %+v", e.from, e.to, ix.Callees(e.from))
		}
	}
	if !hasEdge(ix, "mod:vars.go", "go:example.com/m.handleA", EdgeRef) {
		t.Errorf("a function value in an initializer is a ref: %+v", ix.Callees("mod:vars.go"))
	}
	mod := ix.Symbols["mod:src/main.tsx"]
	if mod == nil || mod.Kind != KindModule || mod.Qualified != "main.tsx <module>" || !mod.Heuristic {
		t.Fatalf("main.tsx module = %+v", mod)
	}
	if s := ix.SymbolAt("src/main.tsx", 4); s != nil {
		t.Errorf("SymbolAt picked %+v; the module isn't a listed symbol", s)
	}
	if ix.Symbols["mod:src/App.test.tsx"] != nil || ix.Symbols["mod:app/quiet.py"] != nil {
		t.Error("test files and files without top-level calls get no module")
	}
	// The decorator belongs to main, not the module.
	for _, e := range ix.Callees("mod:app/cli.py") {
		if e.To != "h:app/cli.py#main" {
			t.Errorf("unexpected module callee %+v", e)
		}
	}
	if st := ix.Stats(); st.Symbols != len(ix.Symbols)-3-countKind(ix, KindExternal) {
		t.Errorf("stats count modules: %+v", st)
	}
}

func countKind(ix *Index, k SymbolKind) int {
	n := 0
	for _, s := range ix.Symbols {
		if s.Kind == k {
			n++
		}
	}
	return n
}

// Calls through an instantiated generic interface dispatch to the
// concrete types that implement that instantiation, generic or not (#74).
func TestGoGenericDispatch(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"go.mod": "module example.com/gen\n\ngo 1.22\n",
		"repo.go": `package gen

type Repo[T any] interface {
	Get(id int) T
}

type intRepo struct{}

func (intRepo) Get(id int) int { return id }

type memRepo[T any] struct{ items map[int]T }

func (m *memRepo[T]) Get(id int) T { return m.items[id] }

type strRepo struct{}

func (strRepo) Get(id int) string { return "" }

func Load(r Repo[int]) int { return r.Get(1) }

func Use() {
	Load(intRepo{})
	var m Repo[int] = &memRepo[int]{}
	m.Get(2)
}
`,
	})
	ix := Build(root, files, testLang)
	get := "go:(example.com/gen.Repo[T any]).Get"
	if !hasEdge(ix, "go:example.com/gen.Load", get, EdgeCall) {
		t.Fatalf("Load should call Repo.Get: %+v", ix.Callees("go:example.com/gen.Load"))
	}
	for _, to := range []string{"go:(example.com/gen.intRepo).Get", "go:(*example.com/gen.memRepo[T]).Get"} {
		if !hasEdge(ix, get, to, EdgeDispatch) {
			t.Errorf("Repo.Get should dispatch to %s: %+v", to, ix.Callees(get))
		}
	}
	// strRepo implements Repo[string], which the code never uses.
	if hasEdge(ix, get, "go:(example.com/gen.strRepo).Get", EdgeDispatch) {
		t.Errorf("dispatch to a type that doesn't implement Repo[int]: %+v", ix.Callees(get))
	}
}

// goto jumps reach their label's statement, and t.Fatal-style calls on
// the test's own *testing.T end it (#74).
func TestGoFuncFlowGotoAndTestStops(t *testing.T) {
	root, _ := writeTree(t, map[string]string{"f_test.go": `package f

import "testing"

func G(n int) int {
	i := 0
retry:
	i++
	if i < n {
		goto retry
	}
	if n < 0 {
		goto done
	}
	for j := 0; j < n; j++ {
		continue
	}
done:
	return i
}

func TestX(t *testing.T) {
	if t == nil {
		t.Fatalf("no t")
	}
	got := G(3)
	if got != 3 {
		t.Errorf("got %d", got)
	}
	other.Fatal("not a test stop")
}
`})
	fl, err := GoFuncFlow(root, "f_test.go", 5)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]FlowNode{}
	for _, n := range fl.Nodes {
		byID[n.ID] = n
	}
	gotos := map[string]FlowEdge{}
	for _, e := range fl.Edges {
		if e.Label == "goto" {
			gotos[byID[e.From].Label] = e
		}
	}
	if e, ok := gotos["goto retry"]; !ok || !e.Back || byID[e.To].Line != 8 {
		t.Errorf("goto retry = %+v (to %+v)", e, byID[e.To])
	}
	if e, ok := gotos["goto done"]; !ok || e.Back || byID[e.To].Kind != FlowReturn {
		t.Errorf("goto done = %+v (to %+v)", e, byID[e.To])
	}

	fl, err = GoFuncFlow(root, "f_test.go", 23)
	if err != nil {
		t.Fatal(err)
	}
	var stops []string
	for _, n := range fl.Nodes {
		if n.Kind == FlowPanic {
			stops = append(stops, n.Label)
		}
	}
	if len(stops) != 1 || stops[0] != `t.Fatalf("no t")` {
		t.Errorf("test stops = %q", stops)
	}
}

// Function literals get their own flow on the node that holds them, built
// the same way: t.Fatal inside a t.Run closure ends the closure (#74).
func TestGoFuncFlowClosures(t *testing.T) {
	root, _ := writeTree(t, map[string]string{"c_test.go": `package c

import "testing"

func TestC(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		if t == nil {
			t.Fatal("x")
		}
		inner := func() { other() }
		inner()
	})
	defer func() { cleanup() }()
}
`})
	fl, err := GoFuncFlow(root, "c_test.go", 5)
	if err != nil {
		t.Fatal(err)
	}
	var lits []*Flow
	for _, n := range fl.Nodes {
		lits = append(lits, n.Closures...)
	}
	if len(lits) != 2 || lits[0].Line != 6 || lits[1].Line != 13 {
		t.Fatalf("closures = %+v", lits)
	}
	sub := lits[0]
	if sub.Signature != "func(t *testing.T)" || len(sub.Inputs) != 1 {
		t.Errorf("closure header = %+v", sub)
	}
	var stop bool
	var nested int
	for _, n := range sub.Nodes {
		stop = stop || n.Kind == FlowPanic && n.Label == `t.Fatal("x")`
		nested += len(n.Closures)
	}
	if !stop || nested != 1 {
		t.Errorf("closure flow: stop=%v nested=%d nodes=%+v", stop, nested, sub.Nodes)
	}
	if len(lits[1].Nodes) < 3 || lits[1].Nodes[2].Calls[0] != "cleanup" {
		t.Errorf("deferred closure = %+v", lits[1].Nodes)
	}
}

// A call on a receiver whose class is written down links to that class's
// method, not the first same-named one; a call still matched by name among
// several candidates is marked as guessed (#67).
func TestGenericReceiverTypes(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"src/cache.ts": `export class Cache {
  save(k: string) {}
}
`,
		"src/sql.ts": `export class SqlStore {
  save(k: string) {}
}

export class AuditedStore extends SqlStore {
  audit() {}
}
`,
		"src/use.ts": `import { SqlStore, AuditedStore } from './sql'
import { Cache } from './cache'

export class Service {
  private cache: Cache
  constructor(private store: SqlStore, audited: AuditedStore) {
    this.cache = new Cache()
    this.log = audited
  }
  log: AuditedStore

  run(s: SqlStore) {
    const c = new Cache()
    s.save('a')
    c.save('b')
    this.store.save('c')
    this.cache.save('d')
    this.log.save('e')
  }
}

export function guess(x: any) {
  x.save('f')
}

export function maybe(s?: SqlStore) {
  s?.save('g')
}
`,
		"app/stores.py": `class Store:
    def save(self, k):
        pass


class Cache:
    def save(self, k):
        pass
`,
		"app/use.py": `from .stores import Store, Cache


class Service:
    backup: Cache

    def __init__(self, store: Store):
        self.store = store
        self.cache = Cache()

    def run(self, s: Store):
        c = Cache()
        s.save("a")
        c.save("b")
        self.store.save("c")
        self.cache.save("d")
        self.backup.save("e")


def guess(x):
    x.save("f")
`,
	})
	ix := Build(root, files, testLang)
	edgeTo := func(from string, line int) (string, bool) {
		for _, e := range ix.Callees(from) {
			for _, l := range e.Lines {
				if l == line {
					return e.To, e.Guessed
				}
			}
		}
		return "", false
	}
	for _, c := range []struct {
		from string
		line int
		to   string
	}{
		{"h:src/use.ts#Service.run", 14, "h:src/sql.ts#SqlStore.save"}, // (s: SqlStore)
		{"h:src/use.ts#Service.run", 15, "h:src/cache.ts#Cache.save"},  // const c = new Cache()
		{"h:src/use.ts#Service.run", 16, "h:src/sql.ts#SqlStore.save"}, // constructor(private store: SqlStore)
		{"h:src/use.ts#Service.run", 17, "h:src/cache.ts#Cache.save"},  // this.cache = new Cache()
		{"h:src/use.ts#Service.run", 18, "h:src/sql.ts#SqlStore.save"}, // log: AuditedStore, through extends
		{"h:app/use.py#Service.run", 13, "h:app/stores.py#Store.save"}, // (s: Store)
		{"h:app/use.py#Service.run", 14, "h:app/stores.py#Cache.save"}, // c = Cache()
		{"h:app/use.py#Service.run", 15, "h:app/stores.py#Store.save"}, // self.store = store (store: Store)
		{"h:app/use.py#Service.run", 16, "h:app/stores.py#Cache.save"}, // self.cache = Cache()
		{"h:app/use.py#Service.run", 17, "h:app/stores.py#Cache.save"}, // backup: Cache
	} {
		to, guessed := edgeTo(c.from, c.line)
		if to != c.to || guessed {
			t.Errorf("%s line %d → %s (guessed %v), want %s", c.from, c.line, to, guessed, c.to)
		}
	}
	if to, guessed := edgeTo("h:src/use.ts#maybe", 27); to != "h:src/sql.ts#SqlStore.save" || guessed {
		t.Errorf("optional parameter (s?: SqlStore) → %s guessed=%v", to, guessed)
	}
	for _, from := range []string{"h:src/use.ts#guess", "h:app/use.py#guess"} {
		if to, guessed := edgeTo(from, map[string]int{"h:src/use.ts#guess": 23, "h:app/use.py#guess": 21}[from]); to == "" || !guessed {
			t.Errorf("%s: an untyped receiver's call should be a marked guess, got %q guessed=%v", from, to, guessed)
		}
	}
}

// Overloads of one method aren't a guess; same-named methods of different
// classes are.
func TestDistinctDefinitions(t *testing.T) {
	a1 := &Symbol{File: "A.java", Qualified: "A.A"}
	a2 := &Symbol{File: "A.java", Qualified: "A.A"}
	b := &Symbol{File: "B.java", Qualified: "B.save"}
	c := &Symbol{File: "C.java", Qualified: "C.save"}
	if distinctDefinitions([]*Symbol{a1, a2}) || !distinctDefinitions([]*Symbol{b, c}) {
		t.Error("overloads must not count as a guess; different classes must")
	}
}

func TestCleanType(t *testing.T) {
	for in, want := range map[string]string{
		"Store": "Store", "Optional[Store]": "Store", "Store | None": "Store", "None | Store": "Store",
		"SqlStore<T>": "SqlStore", "Store | null | undefined": "Store", "models.Store": "models.Store",
		"list[Store]": "", "Store[]": "", "A | B": "", "": "",
	} {
		if got := cleanType(in); got != want {
			t.Errorf("cleanType(%q) = %q, want %q", in, got, want)
		}
	}
}

// Functions passed as values become ref edges from the function passing
// them, when the name resolves to a function; parameters, literals and
// import lists don't (#68).
func TestGenericValueRefs(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"src/fmt.ts": "export function format(x: number) { return String(x) }\n",
		"src/use.ts": `import { format } from './fmt'

function render(v: unknown) {}
function notify() {}
function create() {}
function tick() {}

export class View {
  run(items: number[], emitter: any, p: Promise<unknown>) {
    items.map(format)
    p.then(render).then(this.done)
    emitter.on('done', notify)
    setTimeout(tick, 10)
    return { handler: create, other: null }
  }
  done() {}
}

export function shadow(format: (x: number) => string, xs: number[]) {
  return xs.map(format)
}
`,
		"app/jobs.py": `import threading


def keyfn(x):
    return x


def run():
    pass


class Worker:
    def start(self, xs):
        sorted(xs, key=keyfn)
        threading.Thread(target=run)
        list(map(self.step, xs))
        if xs == run:
            pass

    def step(self, x):
        return x
`,
		"src/Store.java": `class Store {
  void save(String s) {}
}
class Batch {
  void flush(java.util.List<String> xs) {
    xs.forEach(this::log);
    xs.forEach(Store::save);
  }
  void log(String s) {}
}
`,
	})
	lang := func(p string) string {
		if strings.HasSuffix(p, ".java") {
			return "java"
		}
		return testLang(p)
	}
	ix := Build(root, files, lang)
	refs := func(from string) map[string]bool {
		out := map[string]bool{}
		for _, e := range ix.Callees(from) {
			if e.Kind == EdgeRef {
				out[e.To] = true
			}
		}
		return out
	}
	run := refs("h:src/use.ts#View.run")
	for _, to := range []string{"h:src/fmt.ts#format", "h:src/use.ts#render", "h:src/use.ts#View.done", "h:src/use.ts#notify", "h:src/use.ts#tick", "h:src/use.ts#create"} {
		if !run[to] {
			t.Errorf("View.run should reference %s: %v", to, run)
		}
	}
	if r := refs("h:src/use.ts#shadow"); len(r) != 0 {
		t.Errorf("a parameter named format is not the imported function: %v", r)
	}
	if r := refs("mod:src/use.ts"); len(r) != 0 {
		t.Errorf("an import list isn't a value: %v", r)
	}
	start := refs("h:app/jobs.py#Worker.start")
	for _, to := range []string{"h:app/jobs.py#keyfn", "h:app/jobs.py#run", "h:app/jobs.py#Worker.step"} {
		if !start[to] {
			t.Errorf("Worker.start should reference %s: %v", to, start)
		}
	}
	flush := refs("h:src/Store.java#Batch.flush")
	for _, to := range []string{"h:src/Store.java#Batch.log", "h:src/Store.java#Store.save"} {
		if !flush[to] {
			t.Errorf("Batch.flush should reference %s: %v", to, flush)
		}
	}
}

func TestValueNames(t *testing.T) {
	got := valueNames("f(a, this.b) == c; g(x == y, z >= w) {k: v}", "(,:=", ",)}")
	var names []string
	for _, n := range got {
		names = append(names, n.recv+"."+n.name)
	}
	if strings.Join(names, " ") != ".a this.b .v" {
		t.Errorf("valueNames = %v", names)
	}
}

// Ruby calls without parentheses: receiver calls go through the usual
// strategies, bare names link only to the caller's own class, and local
// variables, parameters, hash keys and attribute writes aren't calls (#99).
func TestGenericRubyParenless(t *testing.T) {
	root, files := writeTree(t, map[string]string{
		"store.rb": `class Store
  def save(order)
    @items << order
  end

  def count
    @items.size
  end

  def empty?
    count.zero?
  end

  def report
    "#{count} stored"
  end
end
`,
		"cache.rb": `class Cache
  def save(order)
  end
end
`,
		"handler.rb": `class Handler
  def initialize(store)
    @store = store
    @cache = Cache.new
  end

  def create(order)
    validate order
    @store.save order
    @cache.save order
    self.notify
    save! if order
  end

  def check(validate)
    total = 1
    send_it = { validate: 1, total: 2 }
    order.status = :done
    order.sku.to_s.empty?
    [1].each { |count| count + total }
  end

  def validate(order)
  end

  def notify
  end

  def save!
  end
end
`,
	})
	lang := func(p string) string { return "ruby" }
	ix := Build(root, files, lang)
	callees := func(from string) map[string]bool {
		out := map[string]bool{}
		for _, e := range ix.Callees(from) {
			out[e.To] = true
		}
		return out
	}
	create := callees("h:handler.rb#Handler.create")
	for _, to := range []string{"h:handler.rb#Handler.validate", "h:handler.rb#Handler.notify", "h:handler.rb#Handler.save!", "h:cache.rb#Cache.save"} {
		if !create[to] {
			t.Errorf("Handler.create should call %s: %v", to, create)
		}
	}
	// @store is untyped: store.save matches by name among Store.save and
	// Cache.save; @cache = Cache.new is typed, so it doesn't.
	if !create["h:store.rb#Store.save"] && !create["h:cache.rb#Cache.save"] {
		t.Errorf("@store.save order should link to a save: %v", create)
	}
	if c := callees("h:handler.rb#Handler.check"); len(c) != 0 {
		t.Errorf("parameters, locals, hash keys, attribute writes, block params and chained built-ins aren't calls: %v", c)
	}
	empty := callees("h:store.rb#Store.empty?")
	if !empty["h:store.rb#Store.count"] {
		t.Errorf("count.zero? calls count: %v", empty)
	}
	if r := callees("h:store.rb#Store.report"); !r["h:store.rb#Store.count"] {
		t.Errorf(`"#{count}" calls count: %v`, r)
	}
}
