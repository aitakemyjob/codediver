package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/proposals"
)

// The example projects in testdata/examples are one small orders service
// per language, with tests, committed coverage and gaps on purpose. These
// tests pin down what CodeDiver finds in each, so they double as the
// language support matrix in testdata/examples/README.md: when analysis
// improves (or regresses), a check here changes, and so does the README.

// openExample registers an example project as a repository.
func openExample(t *testing.T, name string) (*Service, *RepoHandle) {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := New(store, "test")
	s.CacheDir = t.TempDir()
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "examples", name))
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.AddRepo(abs, name)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := s.Repo(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, rt
}

// fnExpect is what a function's QA status should be. cover is "0" (never
// ran), "partial" or "full".
type fnExpect struct {
	cover   string
	tested  bool // some test reaches it, directly or indirectly
	callers int  // -1 skips the check
}

type exampleCase struct {
	name       string
	framework  string
	tests      int
	covFiles   int
	funcs      map[string]fnExpect
	hardToTest string   // the function testability hints should flag
	hints      []string // hints expected on it
	validate   string   // the function the demo change edits
	edit       proposals.Edit
	file       string
	caller     string // its direct caller in the blast radius
}

var examples = []exampleCase{
	{
		name: "go-service", framework: "go test", tests: 4, covFiles: 5,
		funcs: map[string]fnExpect{
			"Validate":            {cover: "partial", tested: true, callers: 1},
			"Total":               {cover: "full", tested: true, callers: 1},
			"unitPrice":           {cover: "partial", tested: true, callers: 1}, // only reached through Total
			"LegacyTotal":         {cover: "0", callers: 0},
			"Handler.GetOrder":    {cover: "0", callers: 1},
			"Handler.CreateOrder": {cover: "partial", tested: true, callers: 1},
		},
		hardToTest: "Send", hints: []string{"env", "clock", "network"},
		validate: "Validate", file: "order/order.go", edit: proposals.Edit{Old: "if o.Qty > 1000 {", New: "if o.Qty > 500 {"}, caller: "Handler.CreateOrder",
	},
	{
		name: "ts-react", framework: "vitest", tests: 3, covFiles: 6,
		funcs: map[string]fnExpect{
			"validate":    {cover: "partial", tested: true, callers: 1}, // from OrderForm's submit
			"total":       {cover: "full", tested: true, callers: 1},    // inside OrderSummary's JSX
			"formatCents": {cover: "full", tested: true, callers: 1},    // reached through the OrderSummary test
			"unitPrice":   {cover: "full", tested: true, callers: 1},
			"legacyTotal": {cover: "partial", callers: 0}, // the declaration line runs on import
			// Components rendered only through JSX in App.
			"OrderSummary": {cover: "full", tested: true, callers: 1},
			"OrderForm":    {cover: "0", callers: 1},
		},
		hardToTest: "createOrder", hints: []string{"network", "clock"},
		validate: "validate", file: "src/order.ts", edit: proposals.Edit{Old: "if (o.qty > 1000)", New: "if (o.qty > 500)"}, caller: "submit",
	},
	{
		name: "python-api", framework: "pytest", tests: 4, covFiles: 5,
		funcs: map[string]fnExpect{
			"validate":     {cover: "partial", tested: true, callers: 2},
			"total":        {cover: "full", tested: true, callers: 2},
			"_unit_price":  {cover: "partial", tested: true, callers: 1},
			"legacy_total": {cover: "partial", callers: 0}, // the def line runs on import
			"get_order":    {cover: "partial", callers: 0},
			// get_order and the store test; data.get(...) dict calls don't count.
			"MemoryStore.get": {cover: "partial", tested: true, callers: 2},
		},
		hardToTest: "send", hints: []string{"env", "clock", "network"},
		validate: "validate", file: "orders/order.py", edit: proposals.Edit{Old: "if o.qty > 1000:", New: "if o.qty > 500:"}, caller: "create_order",
	},
	{
		name: "java-maven", framework: "junit", tests: 4, covFiles: 5,
		funcs: map[string]fnExpect{
			"Orders.validate":       {cover: "partial", tested: true, callers: 2},
			"Orders.total":          {cover: "full", tested: true, callers: 2},
			"Orders.unitPrice":      {cover: "partial", tested: true, callers: 1},
			"Orders.legacyTotal":    {cover: "0", callers: 0},
			"OrderHandler.getOrder": {cover: "0", callers: 0},
			// The test lives in the mirrored package under src/test/java.
			"MemoryStore.save": {cover: "full", tested: true, callers: 2},
		},
		hardToTest: "Notifier.send", hints: []string{"env", "clock", "network"},
		validate: "Orders.validate", file: "src/main/java/com/example/orders/Orders.java", edit: proposals.Edit{Old: "if (o.qty() > 1000) {", New: "if (o.qty() > 500) {"}, caller: "OrderHandler.createOrder",
	},
}

func TestExamples(t *testing.T) {
	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			s, rt := openExample(t, ex.name)
			ctx := context.Background()

			// Tests are discovered and coverage is imported from the committed report.
			ov, err := s.QAOverview(ctx, rt, 50)
			if err != nil {
				t.Fatal(err)
			}
			if ov.Tests.Tests != ex.tests || ov.Tests.Frameworks[ex.framework] != ex.tests {
				t.Errorf("tests = %+v", ov.Tests)
			}
			if ov.Coverage == nil || len(ov.Coverage.Sources) != 1 || ov.Coverage.Sources[0].Files != ex.covFiles {
				t.Fatalf("coverage = %+v", ov.Coverage)
			}

			// Each function's coverage, test reach and callers.
			a, err := s.qaAnalyze(ctx, rt)
			if err != nil {
				t.Fatal(err)
			}
			byName := map[string]*FuncQA{}
			for _, f := range a.funcs {
				byName[f.Name] = f
			}
			for name, want := range ex.funcs {
				f := byName[name]
				if f == nil {
					t.Errorf("%s not found", name)
					continue
				}
				cover := "partial"
				switch f.Pct {
				case 0:
					cover = "0"
				case 100:
					cover = "full"
				}
				if cover != want.cover || (f.Direct+f.Indirect > 0) != want.tested || (want.callers >= 0 && f.Callers != want.callers) {
					t.Errorf("%s: pct=%v direct=%d indirect=%d callers=%d, want %+v", name, f.Pct, f.Direct, f.Indirect, f.Callers, want)
				}
			}

			// The hard-to-test function is a gap with the right testability hints.
			gaps, _, err := s.TestGaps(ctx, rt, "", 30)
			if err != nil {
				t.Fatal(err)
			}
			var hints []string
			for _, g := range gaps {
				if g.Name == ex.hardToTest {
					for _, h := range g.Hints {
						hints = append(hints, h.Kind)
					}
				}
			}
			for _, h := range ex.hints {
				if !strings.Contains(strings.Join(hints, ","), h) {
					t.Errorf("%s hints = %v, want %s", ex.hardToTest, hints, h)
				}
			}

			// The demo change maps onto the validate function, whose direct
			// caller is the request handler.
			p, err := s.Propose(ctx, rt, "Lower the limit", "", []proposals.ChangeInput{{Path: ex.file, Edits: []proposals.Edit{ex.edit}}})
			if err != nil {
				t.Fatal(err)
			}
			ix, err := s.Index(ctx, rt)
			if err != nil {
				t.Fatal(err)
			}
			if syms := p.Changes[0].Symbols; len(syms) != 1 || ix.Symbols[syms[0]].Qualified != ex.validate {
				t.Fatalf("changed symbols = %v", syms)
			}
			var callers []string
			for _, e := range ix.Callers(p.Changes[0].Symbols[0]) {
				callers = append(callers, ix.Symbols[e.From].Qualified)
			}
			if !strings.Contains(strings.Join(callers, ","), ex.caller) {
				t.Errorf("callers of %s = %v, want %s", ex.validate, callers, ex.caller)
			}
		})
	}
}

func TestExampleMonorepo(t *testing.T) {
	s, rt := openExample(t, "polyglot-monorepo")
	ctx := context.Background()
	// One suite per component, each with its committed report merged.
	suites, _ := s.QASuites(rt)
	var dirs []string
	for _, su := range suites {
		dirs = append(dirs, su.Tool+":"+su.Dir)
	}
	if strings.Join(dirs, ",") != "pytest:jobs,go:services/billing,vitest:web" {
		t.Errorf("suites = %v", dirs)
	}
	ov, err := s.QAOverview(ctx, rt, 50)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Coverage == nil || len(ov.Coverage.Sources) != 3 {
		t.Fatalf("coverage = %+v", ov.Coverage)
	}
	for _, fw := range []string{"go test", "vitest", "pytest"} {
		if ov.Tests.Frameworks[fw] != 1 {
			t.Errorf("tests = %+v", ov.Tests.Frameworks)
		}
	}
	a, err := s.qaAnalyze(ctx, rt)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range a.funcs {
		switch f.Name {
		case "dollars": // called inside a template literal
			if f.Callers != 1 || f.Indirect != 1 {
				t.Errorf("dollars = %+v", f)
			}
		case "applyTax", "_is_late": // reached only through a caller
			if f.Direct != 0 || f.Indirect != 1 || f.Pct <= 0 {
				t.Errorf("%s = %+v", f.Name, f)
			}
		}
	}
	gaps, _, err := s.TestGaps(ctx, rt, "", 30)
	if err != nil {
		t.Fatal(err)
	}
	hints := map[string][]string{}
	for _, g := range gaps {
		for _, h := range g.Hints {
			hints[g.Name] = append(hints[g.Name], h.Kind)
		}
	}
	if got := strings.Join(hints["send_reminders"], ","); !strings.Contains(got, "network") || !strings.Contains(got, "clock") {
		t.Errorf("send_reminders hints = %v", got)
	}
	if got := strings.Join(hints["fetchInvoices"], ","); !strings.Contains(got, "network") {
		t.Errorf("fetchInvoices hints = %v", got)
	}
}

// TestExampleKnownGaps pins analysis gaps the examples expose, so the
// README's support matrix stays honest. When one of these fails because
// analysis got better, update the matrix and move the check above.
func TestExampleKnownGaps(t *testing.T) {
	s, rt := openExample(t, "ts-react")
	a, err := s.qaAnalyze(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range a.funcs {
		// main.tsx renders <App /> from module-level code, which isn't a
		// function, so App has no caller and counts as an entry point.
		if f.Name == "App" && f.Callers != 0 {
			t.Errorf("App callers = %d: module-level code now counts as a caller; update the support matrix", f.Callers)
		}
	}
}

func TestExampleScript(t *testing.T) {
	needGit(t)
	if testing.Short() {
		t.Skip("runs the demo script")
	}
	dest := filepath.Join(t.TempDir(), "demo")
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "example.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script, "polyglot-monorepo", dest)
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", strings.TrimSuffix(script, ".sh")+".ps1", "polyglot-monorepo", dest)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dest}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if log := git("log", "--format=%s"); log != "Add US sales tax\nInitial billing system\n" {
		t.Errorf("log = %q", log)
	}
	if st := git("status", "--short"); st != " M services/billing/invoice/invoice.go\n M web/src/invoices.ts\n" {
		t.Errorf("status = %q", st)
	}
	if _, err := os.Stat(filepath.Join(dest, ".demo")); !os.IsNotExist(err) {
		t.Error(".demo should not be copied")
	}
}

func TestTestdataIsNotThisProjectsCode(t *testing.T) {
	// Opening a repository that holds example projects under testdata
	// must not detect their suites or count their tests as its own.
	s, rt, dir := qaRepo(t)
	for name, body := range map[string]string{
		"testdata/examples/demo/go.mod":       "module example.com/demo\n",
		"testdata/examples/demo/demo_test.go": "package demo\n\nimport \"testing\"\n\nfunc TestDemo(t *testing.T) {}\n",
		"testdata/examples/web/package.json":  `{"devDependencies": {"vitest": "1"}}`,
		"testdata/examples/web/src/a.test.ts": "it('works', () => {});\n",
	} {
		writeTestFile(t, filepath.Join(dir, filepath.FromSlash(name)), body)
	}
	s.Reindex(rt)
	suites, _ := s.QASuites(rt)
	for _, su := range suites {
		if strings.HasPrefix(su.Dir, "testdata") {
			t.Errorf("suite detected in testdata: %+v", su)
		}
	}
	a, err := s.qaAnalyze(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range a.tests {
		if strings.HasPrefix(tc.File, "testdata/") {
			t.Errorf("test discovered in testdata: %+v", tc)
		}
	}
}

// writeTestFile writes a file, creating its directory.
func writeTestFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
