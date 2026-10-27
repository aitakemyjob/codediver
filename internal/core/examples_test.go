package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aitakemyjob/codediver/internal/analysis"
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
	missing bool // must not be a function under test at all
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
	// callerStatus is that caller's test status under the committed
	// coverage: none of the examples' tests runs its call into validate.
	callerStatus string
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
		validate: "Validate", file: "order/order.go", edit: proposals.Edit{Old: "if o.Qty > 1000 {", New: "if o.Qty > 500 {"}, caller: "Handler.CreateOrder", callerStatus: StatusCaller,
	},
	{
		name: "ts-react", framework: "vitest", tests: 5, covFiles: 16,
		funcs: map[string]fnExpect{
			"validate":    {cover: "partial", tested: true, callers: 1}, // from OrderForm's submit
			"total":       {cover: "full", tested: true, callers: 4},    // OrderSummary's JSX, Cart.subtotal, receipt's format, and grandTotal as orderTotal
			"formatCents": {cover: "full", tested: true, callers: 7},    // OrderSummary, summarize, giftSummary, Checkout.due, both formats and ReceiptPage
			"unitPrice":   {cover: "full", tested: true, callers: 1},
			"legacyTotal": {cover: "partial", callers: 0}, // the declaration line runs on import
			// Components rendered only through JSX in App.
			"OrderSummary": {cover: "full", tested: true, callers: 1},
			"OrderForm":    {cover: "0", callers: 1},
			// Rendered from main.tsx's top level: its <module> calls it (#69).
			"App": {cover: "0", callers: 1},
			// The cart: a constructor, one-line methods, a class field
			// holding an arrow, super and ?. calls.
			"Cart.constructor": {cover: "full", tested: true, callers: 1}, // new Cart() in a test and in CartPage
			"Cart.reset":       {cover: "full", tested: true, callers: 2}, // through the constructor
			"Cart.clear":       {cover: "partial", callers: 0},            // only passed to onClick
			// giftSummary (cart: GiftCart) and Checkout.due (this.cart: GiftCart);
			// name matching alone would send both to Cart.subtotal (#67).
			"GiftCart.subtotal": {cover: "full", tested: true, callers: 2},
			"Cart.subtotal":     {cover: "full", tested: true, callers: 3}, // super.subtotal(), cart?.subtotal() and cart/format.ts
			// An object literal's methods and an anonymous default export.
			"ordersApi.create": {cover: "0", callers: 0},
			"default":          {cover: "0", callers: 0},
		},
		hardToTest: "createOrder", hints: []string{"network", "clock"},
		validate: "validate", file: "src/order.ts", edit: proposals.Edit{Old: "if (o.qty > 1000)", New: "if (o.qty > 500)"}, caller: "submit", callerStatus: StatusNotRun,
	},
	{
		name: "python-api", framework: "pytest", tests: 4, covFiles: 12,
		funcs: map[string]fnExpect{
			"validate":     {cover: "partial", tested: true, callers: 2},
			"total":        {cover: "full", tested: true, callers: 4}, // create_order, a test, receipt's format and grand_total as pricing.total
			"_unit_price":  {cover: "partial", tested: true, callers: 1},
			"legacy_total": {cover: "partial", callers: 0}, // the def line runs on import
			"get_order":    {cover: "partial", callers: 0},
			// get_order, get_receipt and the store test; data.get(...) dict
			// calls don't count.
			"MemoryStore.get": {cover: "partial", tested: true, callers: 3},
		},
		hardToTest: "send", hints: []string{"env", "clock", "network"},
		validate: "validate", file: "orders/order.py", edit: proposals.Edit{Old: "if o.qty > 1000:", New: "if o.qty > 500:"}, caller: "create_order", callerStatus: StatusCaller,
	},
	{
		name: "java-maven", framework: "junit", tests: 4, covFiles: 5,
		funcs: map[string]fnExpect{
			"Orders.validate":       {cover: "partial", tested: true, callers: 2},
			"Orders.total":          {cover: "full", tested: true, callers: 3}, // OrderHandler, a test, and grandTotal through Orders::total
			"Orders.unitPrice":      {cover: "partial", tested: true, callers: 1},
			"Orders.legacyTotal":    {cover: "0", callers: 0},
			"OrderHandler.getOrder": {cover: "0", callers: 0},
			// The test lives in the mirrored package under src/test/java.
			"MemoryStore.save": {cover: "full", tested: true, callers: 2},
		},
		hardToTest: "Notifier.send", hints: []string{"env", "clock", "network"},
		validate: "Orders.validate", file: "src/main/java/com/example/orders/Orders.java", edit: proposals.Edit{Old: "if (o.qty() > 1000) {", New: "if (o.qty() > 500) {"}, caller: "OrderHandler.createOrder", callerStatus: StatusCaller,
	},
	{
		// Tests live in #[cfg(test)] modules next to the code.
		name: "rust-cargo", framework: "rust", tests: 4, covFiles: 4,
		funcs: map[string]fnExpect{
			"validate":         {cover: "partial", tested: true, callers: 2},
			"total":            {cover: "full", tested: true, callers: 2},
			"unit_price":       {cover: "partial", tested: true, callers: 1},
			"legacy_total":     {cover: "0", callers: 0},
			"MemoryStore.new":  {cover: "full", tested: true, callers: 1}, // MemoryStore::new() in a test
			"MemoryStore.save": {cover: "full", tested: true, callers: 2},
			// Test functions and helpers in the test module aren't code
			// under test.
			"save_and_find": {missing: true},
			"order":         {missing: true},
		},
		hardToTest: "send", hints: []string{"env", "clock", "network"},
		validate: "validate", file: "src/order.rs", edit: proposals.Edit{Old: "if o.qty > 1000 {", New: "if o.qty > 500 {"}, caller: "Handler.create_order", callerStatus: StatusCaller,
	},
	{
		name: "dotnet-xunit", framework: "dotnet", tests: 4, covFiles: 5,
		funcs: map[string]fnExpect{
			"Orders.Validate":    {cover: "partial", tested: true, callers: 2},
			"Orders.Total":       {cover: "full", tested: true, callers: 2}, // expression-bodied
			"Orders.UnitPrice":   {cover: "full", tested: true, callers: 1},
			"Orders.LegacyTotal": {cover: "0", callers: 0},
			// store.Save in a test: the receiver names the class.
			"MemoryStore.Save": {cover: "full", tested: true, callers: 2},
			// int.Parse inside Parse isn't a call to Parse.
			"OrderHandler.Parse":        {cover: "partial", tested: true, callers: 1},
			"OrderHandler.OrderHandler": {cover: "full", tested: true, callers: 1}, // the constructor
		},
		hardToTest: "Notifier.SendAsync", hints: []string{"env", "clock", "network"},
		validate: "Orders.Validate", file: "src/Orders/Orders.cs", edit: proposals.Edit{Old: "if (o.Qty > 1000)", New: "if (o.Qty > 500)"}, caller: "OrderHandler.CreateOrderAsync", callerStatus: StatusCaller,
	},
	{
		name: "ruby-rspec", framework: "rspec", tests: 4, covFiles: 5,
		funcs: map[string]fnExpect{
			"Orders.validate":     {cover: "partial", tested: true, callers: 1},
			"Orders.total":        {cover: "full", tested: true, callers: 1},
			"Orders.unit_price":   {cover: "partial", tested: true, callers: 1},
			"Orders.legacy_total": {cover: "partial", callers: 0}, // the def line runs on load
			// Orders::MemoryStore.new runs initialize.
			"MemoryStore.initialize": {cover: "full", tested: true, callers: 0},
			"MemoryStore.find":       {cover: "full", tested: true, callers: 1},
			// Called without parentheses (#99): `@store.save order` in
			// create_order, `count.zero?` in empty?, and `@store.empty?` /
			// `#{@store.count}` in summary.
			"MemoryStore.save":   {cover: "full", tested: true, callers: 1},
			"MemoryStore.count":  {cover: "partial", callers: 2},
			"MemoryStore.empty?": {cover: "partial", callers: 1},
			// Called only as "#{timestamp}" in Notifier.deliver, so the tests
			// of deliver reach it (#79).
			"Notifier.timestamp": {cover: "partial", tested: true, callers: 1},
		},
		hardToTest: "Notifier.deliver", hints: []string{"env", "network"},
		validate: "Orders.validate", file: "lib/orders/order.rb", edit: proposals.Edit{Old: "if order.qty > 1000", New: "if order.qty > 500"}, caller: "OrderHandler.create_order", callerStatus: StatusCaller,
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
				if want.missing {
					if f != nil {
						t.Errorf("%s should not be code under test: %+v", name, f)
					}
					continue
				}
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

			// The change report puts that caller first in the blast radius,
			// and the changed line was never run by the committed coverage.
			r, err := s.ChangeReport(ctx, rt, ChangeRef{Proposal: p.ID})
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Impact.Rings) == 0 || r.Impact.Rings[0].Depth != 1 || !strings.Contains(nodeNames(r.Impact.Rings[0].Nodes), ex.caller) {
				t.Fatalf("direct callers = %+v", r.Impact.Rings)
			}
			// Its status is measured, and a caller whose call into the
			// change no test runs needs review, whatever static reach says.
			for _, n := range r.Impact.Rings[0].Nodes {
				if n.Name == ex.caller && (n.Status != ex.callerStatus || !n.Measured || n.Tested || !n.Review) {
					t.Errorf("%s = %+v, want %s", n.Name, n, ex.callerStatus)
				}
			}
			// Tests aren't callers in the blast radius, including Rust
			// tests in #[cfg(test)] modules next to the code.
			for _, ring := range r.Impact.Rings {
				for _, n := range ring.Nodes {
					if a.isTestCode(ix.Symbols[n.ID]) {
						t.Errorf("test in the blast radius: %s (%s:%d)", n.Name, n.File, n.Line)
					}
				}
			}
			if c := r.Impact.Changed[0]; c.Status != StatusPath || !c.Measured {
				t.Errorf("changed = %+v", c)
			}
			if !r.Coverage.Available || r.Coverage.Lines != 1 || r.Coverage.Uncovered != 1 || len(r.Coverage.Tests) == 0 {
				t.Errorf("change coverage = %+v", r.Coverage)
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

// TestExampleTypeScriptConstructs pins the TypeScript shapes the cart
// code uses (#61, #62): each must produce its call edge.
func TestExampleTypeScriptConstructs(t *testing.T) {
	s, rt := openExample(t, "ts-react")
	ix, err := s.Index(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	id := func(file, name string) string { return "h:src/" + file + "#" + name }
	cart, page, api := "cart/cart.ts", "pages/CartPage.tsx", "api.ts"
	for _, e := range [][2]string{
		{id(cart, "Cart.constructor"), id(cart, "Cart.reset")},              // one-line constructor body
		{id(cart, "Cart.add"), id(cart, "Cart.count")},                      // one-line method
		{id(cart, "Cart.clear"), id(cart, "Cart.reset")},                    // class field holding an arrow
		{id(cart, "GiftCart.subtotal"), id(cart, "Cart.subtotal")},          // super.subtotal()
		{id(cart, "summarize"), id(cart, "Cart.subtotal")},                  // cart?.subtotal()
		{id(api, "ordersApi.create"), id(api, "createOrder")},               // object-literal method
		{id("pages/CartPage.tsx", "default"), id(cart, "Cart.constructor")}, // anonymous default export; new Cart()
	} {
		found := false
		for _, c := range ix.Callees(e[0]) {
			found = found || c.To == e[1]
		}
		if !found {
			t.Errorf("%s should call %s: %+v", e[0], e[1], ix.Callees(e[0]))
		}
	}
	if s := ix.Symbols[id(page, "default")]; s == nil || s.Kind != "func" {
		t.Errorf("CartPage default export = %+v", s)
	}
	// cart.ts imports '../order' over several lines.
	found := false
	for _, e := range ix.PkgEdges {
		found = found || e.From == "dir:src/cart" && e.To == "dir:src"
	}
	if !found {
		t.Errorf("multi-line import missing: %+v", ix.PkgEdges)
	}
}

// TestExampleTypeScriptImports pins calls the ts-react receipt page and
// cart page make through imports (#65): each links to the definition its
// file imports, even under another name or through a barrel, and never to
// the same-named function in another module.
func TestExampleTypeScriptImports(t *testing.T) {
	s, rt := openExample(t, "ts-react")
	ix, err := s.Index(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	id := func(file, name string) string { return "h:src/" + file + "#" + name }
	receipt, cartPage := id("pages/ReceiptPage.tsx", "ReceiptPage"), id("pages/CartPage.tsx", "default")
	calls := func(from, to string) bool {
		for _, c := range ix.Callees(from) {
			if c.To == to {
				return true
			}
		}
		return false
	}
	for _, e := range []struct{ from, to, how string }{
		// Name matching happens to get these two right as well: the two
		// formats are equally far from pages/, and grandTotal is unique.
		{cartPage, id("cart/format.ts", "format"), "named import of one of two same-named functions"},
		{receipt, id("receipt/totals.ts", "grandTotal"), "namespace import through export *"},
		// These need the import: name matching picks the format beside
		// cartLine, and finds nothing called formatLine, emit or orderTotal.
		{id("receipt/summary.ts", "cartLine"), id("cart/format.ts", "format"), "named import, with the other format in the caller's own folder"},
		{receipt, id("receipt/format.ts", "format"), "namespace import, barrel rename (formatLine)"},
		{receipt, id("receipt/print.ts", "printReceipt"), "default import under another name (emit)"},
		{id("receipt/totals.ts", "grandTotal"), id("order.ts", "total"), "aliased import (orderTotal)"},
	} {
		if !calls(e.from, e.to) {
			t.Errorf("%s: %s should call %s: %+v", e.how, e.from, e.to, ix.Callees(e.from))
		}
	}
	if calls(cartPage, id("receipt/format.ts", "format")) || calls(receipt, id("cart/format.ts", "format")) || calls(id("receipt/summary.ts", "cartLine"), id("receipt/format.ts", "format")) {
		t.Errorf("a call linked to the same-named format it doesn't import")
	}
}

// TestExamplePythonImports pins calls the python-api receipt code makes
// through imports (#66): each links to the definition its module imports,
// under an alias, through a package __init__.py or a module alias, and
// never to the same-named format in another module.
func TestExamplePythonImports(t *testing.T) {
	s, rt := openExample(t, "python-api")
	ix, err := s.Index(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	id := func(file, name string) string { return "h:orders/" + file + "#" + name }
	text, line := id("text.py", "format"), id("receipt/lines.py", "format")
	calls := func(from, to string) bool {
		for _, c := range ix.Callees(from) {
			if c.To == to {
				return true
			}
		}
		return false
	}
	for _, e := range []struct{ from, to, how string }{
		// These need the import: name matching picks the format beside
		// heading, and finds nothing called dollars or line.
		{id("receipt/heading.py", "heading"), text, "from ..text import format, with another format in the caller's own folder"},
		{line, text, "from ..text import format as dollars, called in an f-string"},
		{id("api.py", "get_receipt"), line, "from . import receipt; receipt.line(), re-exported by __init__.py as line"},
		// Name matching happens to get these right too: the names are unique.
		{id("receipt/totals.py", "grand_total"), id("order.py", "total"), "import orders.order as pricing; pricing.total()"},
		{id("api.py", "get_receipt"), id("receipt/totals.py", "grand_total"), "receipt.grand_total(), re-exported by from .totals import *"},
		{id("api.py", "get_receipt"), id("receipt/heading.py", "heading"), "receipt.heading(), re-exported by __init__.py"},
	} {
		if !calls(e.from, e.to) {
			t.Errorf("%s: %s should call %s: %+v", e.how, e.from, e.to, ix.Callees(e.from))
		}
	}
	if calls(id("receipt/heading.py", "heading"), line) {
		t.Errorf("heading linked to the format beside it, which it doesn't import")
	}
}

// TestExampleGoFlow pins what go-service shows of Go control flow and
// generics (#74): a call through store.Repo[order.Order] dispatches to
// Memory.Get, ListOrders's sort.Slice closure has its own flow, and
// notify.Send's goto retry jumps back to its label.
func TestExampleGoFlow(t *testing.T) {
	s, rt := openExample(t, "go-service")
	ctx := context.Background()
	ix, err := s.Index(ctx, rt)
	if err != nil {
		t.Fatal(err)
	}
	get, mem := "go:(example.com/orders/store.Repo[T any]).Get", "go:(*example.com/orders/store.Memory).Get"
	dispatches := false
	for _, e := range ix.Callees(get) {
		dispatches = dispatches || e.To == mem && e.Kind == analysis.EdgeDispatch
	}
	if !dispatches {
		t.Errorf("Repo.Get should dispatch to Memory.Get: %+v", ix.Callees(get))
	}

	fl, _, err := s.FunctionFlow(ctx, rt, "go:(*example.com/orders/api.Handler).ListOrders")
	if err != nil {
		t.Fatal(err)
	}
	var lits []*analysis.Flow
	for _, n := range fl.Nodes {
		lits = append(lits, n.Closures...)
	}
	if len(lits) != 1 {
		t.Fatalf("ListOrders closures = %d, want the sort.Slice one", len(lits))
	}
	kinds := map[analysis.FlowNodeKind]int{}
	for _, n := range lits[0].Nodes {
		kinds[n.Kind]++
	}
	if lits[0].Signature != "func(i, j int) bool" || kinds[analysis.FlowCond] != 1 || kinds[analysis.FlowReturn] != 2 {
		t.Errorf("sort closure = %s %v", lits[0].Signature, kinds)
	}

	fl, _, err = s.FunctionFlow(ctx, rt, "go:example.com/orders/notify.Send")
	if err != nil {
		t.Fatal(err)
	}
	back := false
	for _, e := range fl.Edges {
		back = back || e.Label == "goto" && e.Back
	}
	if !back {
		t.Errorf("goto retry should jump back to its label: %+v", fl.Edges)
	}
}

// TestExampleReceiverTypes pins calls the examples make on receivers whose
// class is written down (#67): each reaches that class's method although
// another class's same-named method is closer by name, and none of them
// is a guess.
func TestExampleReceiverTypes(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct{ example, from, to, other, how string }{
		{"ts-react", "h:src/cart/cart.ts#giftSummary", "h:src/cart/cart.ts#GiftCart.subtotal", "h:src/cart/cart.ts#Cart.subtotal", "parameter (cart: GiftCart)"},
		{"ts-react", "h:src/cart/checkout.ts#Checkout.due", "h:src/cart/cart.ts#GiftCart.subtotal", "h:src/cart/cart.ts#Cart.subtotal", "constructor(private cart: GiftCart), this.cart.subtotal()"},
		{"ts-react", "h:src/cart/cart.ts#summarize", "h:src/cart/cart.ts#Cart.subtotal", "h:src/cart/cart.ts#GiftCart.subtotal", "optional parameter (cart?: Cart)"},
		{"python-api", "h:orders/api.py#create_order", "h:orders/store.py#MemoryStore.save", "", "store: MemoryStore"},
		{"python-api", "h:orders/api.py#create_order", "h:orders/audit.py#AuditLog.save", "", "audit: AuditLog | None"},
	} {
		s, rt := openExample(t, c.example)
		ix, err := s.Index(ctx, rt)
		if err != nil {
			t.Fatal(err)
		}
		var found *analysis.Edge
		for _, e := range ix.Callees(c.from) {
			if e.To == c.to {
				found = &e
			}
			if c.other != "" && e.To == c.other {
				t.Errorf("%s: %s also links to %s", c.how, c.from, c.other)
			}
		}
		if found == nil || found.Guessed {
			t.Errorf("%s: %s should call %s without a guess: %+v", c.how, c.from, c.to, ix.Callees(c.from))
		}
	}
	// replay's target has no annotation: its save is matched by name among
	// MemoryStore.save and AuditLog.save, and marked as a guess.
	s, rt := openExample(t, "python-api")
	ix, err := s.Index(ctx, rt)
	if err != nil {
		t.Fatal(err)
	}
	guessed := false
	for _, e := range ix.Callees("h:orders/maintenance.py#replay") {
		guessed = guessed || strings.HasSuffix(e.To, ".save") && e.Guessed
	}
	if !guessed {
		t.Errorf("replay's untyped target.save() should be a marked guess: %+v", ix.Callees("h:orders/maintenance.py#replay"))
	}
}

// TestExampleCallbacks pins functions the examples pass as values (#68):
// each gets a ref edge from the function passing it, so Trace to here
// reaches a callback used only through .then.
func TestExampleCallbacks(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct{ example, from, to, how string }{
		{"ts-react", "h:src/api.ts#placeOrder", "h:src/api.ts#rememberOrder", "createOrder(o).then(rememberOrder)"},
		{"python-api", "h:orders/maintenance.py#replay", "h:orders/maintenance.py#_by_id", "sorted(orders, key=_by_id)"},
		{"python-api", "h:orders/maintenance.py#replay_in_background", "h:orders/maintenance.py#replay", "threading.Thread(target=replay)"},
		{"java-maven", "h:src/main/java/com/example/orders/Orders.java#Orders.grandTotal", "h:src/main/java/com/example/orders/Orders.java#Orders.total", "mapToInt(Orders::total)"},
	} {
		s, rt := openExample(t, c.example)
		ix, err := s.Index(ctx, rt)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range ix.Callers(c.to) {
			found = found || e.From == c.from && e.Kind == analysis.EdgeRef
		}
		if !found {
			t.Errorf("%s: %s should reference %s: %+v", c.how, c.from, c.to, ix.Callers(c.to))
		}
		traced := false
		for _, n := range ix.TraceTo(c.to, 6, 200).Nodes {
			traced = traced || n.ID == c.from
		}
		if !traced {
			t.Errorf("%s: Trace to %s should reach %s", c.how, c.to, c.from)
		}
	}
}

// TestExampleRubyParenless pins Ruby calls written without parentheses
// in ruby-rspec (#99).
func TestExampleRubyParenless(t *testing.T) {
	s, rt := openExample(t, "ruby-rspec")
	ix, err := s.Index(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	store, handler := "h:lib/orders/memory_store.rb#MemoryStore.", "h:lib/orders/order_handler.rb#OrderHandler."
	for _, e := range []struct{ from, to, how string }{
		{handler + "create_order", store + "save", "@store.save order"},
		{store + "empty?", store + "count", "count.zero?"},
		{handler + "summary", store + "empty?", "@store.empty?"},
		{handler + "summary", store + "count", `"#{@store.count} orders"`},
	} {
		found := false
		for _, c := range ix.Callees(e.from) {
			found = found || c.To == e.to
		}
		if !found {
			t.Errorf("%s: %s should call %s: %+v", e.how, e.from, e.to, ix.Callees(e.from))
		}
	}
}

// TestExampleRubyHelpers pins that a helper defined inside a describe
// block (order_spec.rb's order) belongs to that block: each test maps only
// to what it calls.
func TestExampleRubyHelpers(t *testing.T) {
	s, rt := openExample(t, "ruby-rspec")
	a, err := s.qaAnalyze(context.Background(), rt)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"rejects a zero quantity":   "h:lib/orders/order.rb#Orders.validate",
		"applies the bulk discount": "h:lib/orders/order.rb#Orders.total",
	}
	for _, tc := range a.tests {
		if w, ok := want[tc.Name]; ok && strings.Join(tc.Targets, ",") != w {
			t.Errorf("%q targets = %v, want %s", tc.Name, tc.Targets, w)
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

// nodeNames joins blast-radius node names.
func nodeNames(ns []ImpactNode) string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Name)
	}
	return strings.Join(out, ",")
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
