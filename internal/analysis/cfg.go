package analysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type FlowNodeKind string

const (
	FlowStart  FlowNodeKind = "start"
	FlowEnd    FlowNodeKind = "end"
	FlowBlock  FlowNodeKind = "block"
	FlowCond   FlowNodeKind = "cond"
	FlowLoop   FlowNodeKind = "loop"
	FlowSwitch FlowNodeKind = "switch"
	FlowCase   FlowNodeKind = "case"
	FlowReturn FlowNodeKind = "return"
	FlowDefer  FlowNodeKind = "defer"
	FlowGo     FlowNodeKind = "go"
	FlowPanic  FlowNodeKind = "panic"
	FlowJump   FlowNodeKind = "jump"
)

type FlowNode struct {
	ID      string       `json:"id"`
	Kind    FlowNodeKind `json:"kind"`
	Label   string       `json:"label"`
	Code    []string     `json:"code,omitempty"`
	Line    int          `json:"line,omitempty"`
	EndLine int          `json:"endLine,omitempty"`
	Calls   []string     `json:"calls,omitempty"`
	// Closures are the control-flow graphs of the function literals
	// written in this node's code, in source order.
	Closures []*Flow `json:"closures,omitempty"`
}

type FlowEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
	Back  bool   `json:"back,omitempty"`
}

// Flow is a function's control-flow graph plus its inputs and outputs.
type Flow struct {
	Symbol    string     `json:"symbol,omitempty"`
	Name      string     `json:"name"`
	File      string     `json:"file"`
	Line      int        `json:"line"`
	EndLine   int        `json:"endLine"`
	Signature string     `json:"signature"`
	Inputs    []Param    `json:"inputs"`
	Outputs   []Param    `json:"outputs"`
	Receiver  *Param     `json:"receiver,omitempty"`
	Nodes     []FlowNode `json:"nodes"`
	Edges     []FlowEdge `json:"edges"`
}

type pend struct{ from, label string }

type breakCtx struct {
	label      string
	isLoop     bool
	breaks     []pend
	continueTo string
}

type cfgBuilder struct {
	fset   *token.FileSet
	src    []byte
	file   string
	nodes  []FlowNode
	edges  []FlowEdge
	exit   string
	stack  []*breakCtx
	label  string // pending label for the next loop/switch
	nextID int
	// labels maps a statement label to the first node of its statement,
	// and gotos are the jumps waiting for their label's node.
	labels map[string]string
	gotos  []pend
	// testers are the function's *testing.T/B/F and testing.TB parameters,
	// whose Fatal, FailNow and Skip methods end the function.
	testers map[string]bool
}

// testingTypes are the parameter types whose stop methods exit a test.
var testingTypes = map[string]bool{"*testing.T": true, "*testing.B": true, "*testing.F": true, "testing.TB": true}

// testStops are the testing methods that stop the test function.
var testStops = map[string]bool{"Fatal": true, "Fatalf": true, "FailNow": true, "Skip": true, "Skipf": true, "SkipNow": true}

// GoFuncFlow builds the control-flow graph of the Go function declared in
// file that contains line.
func GoFuncFlow(root, file string, line int) (*Flow, error) {
	abs := filepath.Join(root, filepath.FromSlash(file))
	src, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, abs, src, parser.ParseComments|parser.SkipObjectResolution)
	if f == nil {
		return nil, err
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fset.Position(fd.Pos()).Line <= line && line <= fset.Position(fd.End()).Line {
			fn = fd
			break
		}
	}
	if fn == nil {
		return nil, fmt.Errorf("no Go function at %s:%d", file, line)
	}
	return buildFlow(fset, src, file, fn), nil
}

// buildFlow constructs the control-flow graph of fn, including its
// signature, receiver, inputs and outputs.
func buildFlow(fset *token.FileSet, src []byte, file string, fn *ast.FuncDecl) *Flow {
	b := newCFGBuilder(fset, src, file)
	fl := b.header(fn.Name.Name, fn.Pos(), fn.End(), fn.Type)
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		r := b.fields(fn.Recv)
		if len(r) > 0 {
			fl.Receiver = &r[0]
		}
	}
	return b.build(fl, fn.Name.Name+"()", fn.Pos(), fn.End(), fn.Body)
}

// buildLitFlow constructs the control-flow graph of a function literal (a
// closure) in file.
func buildLitFlow(fset *token.FileSet, src []byte, file string, lit *ast.FuncLit) *Flow {
	b := newCFGBuilder(fset, src, file)
	name := fmt.Sprintf("func literal (L%d)", fset.Position(lit.Pos()).Line)
	return b.build(b.header(name, lit.Pos(), lit.End(), lit.Type), "func()", lit.Pos(), lit.End(), lit.Body)
}

// newCFGBuilder returns a builder for one function's graph in file.
func newCFGBuilder(fset *token.FileSet, src []byte, file string) *cfgBuilder {
	return &cfgBuilder{fset: fset, src: src, file: file, labels: map[string]string{}, testers: map[string]bool{}}
}

// header fills a flow's name, position, signature, inputs and outputs, and
// notes the testing parameters whose stop methods exit the function.
func (b *cfgBuilder) header(name string, pos, end token.Pos, typ *ast.FuncType) *Flow {
	fl := &Flow{
		Name:    name,
		File:    b.file,
		Line:    b.fset.Position(pos).Line,
		EndLine: b.fset.Position(end).Line,
	}
	fl.Signature = strings.TrimSpace(b.text(pos, typ.End()))
	fl.Inputs = b.fields(typ.Params)
	fl.Outputs = b.fields(typ.Results)
	for _, p := range fl.Inputs {
		if testingTypes[p.Type] && p.Name != "" {
			b.testers[p.Name] = true
		}
	}
	return fl
}

// build threads body between a start node labeled start and an end node,
// and stores the graph in fl.
func (b *cfgBuilder) build(fl *Flow, start string, pos, end token.Pos, body *ast.BlockStmt) *Flow {
	first := b.add(FlowStart, start, pos, pos)
	b.exit = b.add(FlowEnd, "end", end, end)
	if body != nil {
		out := b.stmts(body.List, []pend{{from: first}})
		b.connect(out, b.exit)
	} else {
		b.connect([]pend{{from: first}}, b.exit)
	}
	b.linkGotos()
	fl.Nodes, fl.Edges = b.nodes, b.edges
	return fl
}

// fields flattens a parameter list into Params, keeping each type's
// source spelling.
func (b *cfgBuilder) fields(fl *ast.FieldList) []Param {
	if fl == nil {
		return nil
	}
	var out []Param
	for _, f := range fl.List {
		typ := b.text(f.Type.Pos(), f.Type.End())
		if len(f.Names) == 0 {
			out = append(out, Param{Type: typ})
		}
		for _, n := range f.Names {
			out = append(out, Param{Name: n.Name, Type: typ})
		}
	}
	return out
}

// text returns the source between two positions.
func (b *cfgBuilder) text(from, to token.Pos) string {
	s, e := b.fset.Position(from).Offset, b.fset.Position(to).Offset
	if s < 0 || e > len(b.src) || s > e {
		return ""
	}
	return string(b.src[s:e])
}

// oneLine collapses whitespace in s and truncates it to max bytes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}

// add appends a node spanning pos..end and returns its ID.
func (b *cfgBuilder) add(kind FlowNodeKind, label string, pos, end token.Pos) string {
	id := fmt.Sprintf("n%d", b.nextID)
	b.nextID++
	b.nodes = append(b.nodes, FlowNode{
		ID: id, Kind: kind, Label: label,
		Line: b.fset.Position(pos).Line, EndLine: b.fset.Position(end).Line,
	})
	return id
}

// describe records the calls and closures in n on the node id.
func (b *cfgBuilder) describe(id string, n ast.Node) {
	node := b.node(id)
	node.Calls = append(node.Calls, b.calls(n)...)
	node.Closures = append(node.Closures, b.closures(n)...)
}

// closures builds the flows of the function literals in n that aren't
// nested in another literal (those belong to the outer literal's flow).
func (b *cfgBuilder) closures(n ast.Node) []*Flow {
	var out []*Flow
	ast.Inspect(n, func(x ast.Node) bool {
		if lit, ok := x.(*ast.FuncLit); ok {
			out = append(out, buildLitFlow(b.fset, b.src, b.file, lit))
			return false
		}
		return true
	})
	return out
}

// node returns the node with the given ID, or nil.
func (b *cfgBuilder) node(id string) *FlowNode {
	for i := range b.nodes {
		if b.nodes[i].ID == id {
			return &b.nodes[i]
		}
	}
	return nil
}

// connect adds an edge from every pending predecessor to the node to.
func (b *cfgBuilder) connect(preds []pend, to string) {
	for _, p := range preds {
		b.edges = append(b.edges, FlowEdge{From: p.from, To: to, Label: p.label})
	}
}

// calls lists the distinct functions called within n, excluding calls
// inside nested function literals.
func (b *cfgBuilder) calls(n ast.Node) []string {
	var out []string
	seen := map[string]bool{}
	ast.Inspect(n, func(x ast.Node) bool {
		if _, ok := x.(*ast.FuncLit); ok {
			return false
		}
		if c, ok := x.(*ast.CallExpr); ok {
			name := oneLine(b.text(c.Fun.Pos(), c.Fun.End()), 40)
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
		return true
	})
	return out
}

// stmts threads a statement list, merging runs of simple statements into
// a single block node.
func (b *cfgBuilder) stmts(list []ast.Stmt, preds []pend) []pend {
	cur := ""
	for _, s := range list {
		if preds == nil {
			// Unreachable code after return/break; still draw it, detached.
			preds = []pend{}
		}
		if b.isSimple(s) {
			code := oneLine(b.text(s.Pos(), s.End()), 90)
			if cur != "" && len(preds) == 1 && preds[0].from == cur && preds[0].label == "" {
				n := b.node(cur)
				n.Code = append(n.Code, code)
				n.EndLine = b.fset.Position(s.End()).Line
				b.describe(cur, s)
				continue
			}
			cur = b.add(FlowBlock, "", s.Pos(), s.End())
			n := b.node(cur)
			n.Code = []string{code}
			b.describe(cur, s)
			b.connect(preds, cur)
			preds = []pend{{from: cur}}
			continue
		}
		cur = ""
		preds = b.stmt(s, preds)
	}
	return preds
}

// isSimple reports whether s is a straight-line statement that can be
// merged into a block node.
func (b *cfgBuilder) isSimple(s ast.Stmt) bool {
	switch x := s.(type) {
	case *ast.AssignStmt, *ast.IncDecStmt, *ast.SendStmt, *ast.DeclStmt, *ast.EmptyStmt:
		return true
	case *ast.ExprStmt:
		return !isTerminalCall(x.X) && !b.isTestStop(x.X)
	}
	return false
}

// isTestStop reports whether e stops the test: t.Fatal, b.FailNow,
// tb.Skip and the like on one of the function's testing parameters.
func (b *cfgBuilder) isTestStop(e ast.Expr) bool {
	c, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	f, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := f.X.(*ast.Ident)
	return ok && b.testers[x.Name] && testStops[f.Sel.Name]
}

// isTerminalCall reports whether e is a call that never returns normally:
// panic, os.Exit or log.Fatal*/log.Panic*.
func isTerminalCall(e ast.Expr) bool {
	c, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return f.Name == "panic"
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok {
			n := x.Name + "." + f.Sel.Name
			return n == "os.Exit" || n == "log.Fatal" || n == "log.Fatalf" || n == "log.Fatalln" || n == "log.Panic" || n == "log.Panicf"
		}
	}
	return false
}

// simpleInit threads an optional init statement (if/for/switch) into the
// graph before its parent statement.
func (b *cfgBuilder) simpleInit(s ast.Stmt, preds []pend) []pend {
	if s == nil {
		return preds
	}
	return b.stmts([]ast.Stmt{s}, preds)
}

// stmt adds the nodes for one compound or control statement and returns
// the dangling edges that continue to whatever follows it.
func (b *cfgBuilder) stmt(s ast.Stmt, preds []pend) []pend {
	switch x := s.(type) {
	case *ast.BlockStmt:
		return b.stmts(x.List, preds)

	case *ast.LabeledStmt:
		b.label = x.Label.Name
		first := b.nextID
		out := b.stmt(x.Stmt, preds)
		if b.nextID > first {
			b.labels[x.Label.Name] = fmt.Sprintf("n%d", first)
		}
		b.label = "" // a label on a plain statement belongs to no loop
		return out

	case *ast.ExprStmt: // terminal call (panic, os.Exit, log.Fatal, t.Fatal)
		id := b.add(FlowPanic, oneLine(b.text(x.Pos(), x.End()), 70), x.Pos(), x.End())
		b.connect(preds, id)
		b.edges = append(b.edges, FlowEdge{From: id, To: b.exit, Label: "abort"})
		return nil

	case *ast.ReturnStmt:
		id := b.add(FlowReturn, oneLine(b.text(x.Pos(), x.End()), 70), x.Pos(), x.End())
		b.describe(id, x)
		b.connect(preds, id)
		b.connect([]pend{{from: id}}, b.exit)
		return nil

	case *ast.DeferStmt:
		id := b.add(FlowDefer, oneLine(b.text(x.Pos(), x.End()), 70), x.Pos(), x.End())
		b.describe(id, x.Call)
		b.connect(preds, id)
		return []pend{{from: id}}

	case *ast.GoStmt:
		id := b.add(FlowGo, oneLine(b.text(x.Pos(), x.End()), 70), x.Pos(), x.End())
		b.describe(id, x.Call)
		b.connect(preds, id)
		return []pend{{from: id}}

	case *ast.IfStmt:
		preds = b.simpleInit(x.Init, preds)
		id := b.add(FlowCond, "if "+oneLine(b.text(x.Cond.Pos(), x.Cond.End()), 70), x.Pos(), x.Cond.End())
		b.describe(id, x.Cond)
		b.connect(preds, id)
		out := b.stmts(x.Body.List, []pend{{from: id, label: "true"}})
		if x.Else != nil {
			out = append(out, b.stmt(x.Else, []pend{{from: id, label: "false"}})...)
		} else {
			out = append(out, pend{from: id, label: "false"})
		}
		return out

	case *ast.ForStmt:
		preds = b.simpleInit(x.Init, preds)
		label := "for"
		if x.Cond != nil {
			label = "for " + oneLine(b.text(x.Cond.Pos(), x.Cond.End()), 60)
		}
		if x.Post != nil {
			label += "; " + oneLine(b.text(x.Post.Pos(), x.Post.End()), 30)
		}
		id := b.add(FlowLoop, label, x.Pos(), x.Body.Lbrace)
		b.connect(preds, id)
		ctx := b.push(true, id)
		body := b.stmts(x.Body.List, []pend{{from: id, label: "loop"}})
		b.backEdges(body, id)
		b.pop()
		var out []pend
		if x.Cond != nil {
			out = append(out, pend{from: id, label: "done"})
		}
		return append(out, ctx.breaks...)

	case *ast.RangeStmt:
		label := "range " + oneLine(b.text(x.X.Pos(), x.X.End()), 60)
		if x.Key != nil {
			label = "for " + oneLine(b.text(x.Key.Pos(), x.X.End()), 70)
		}
		id := b.add(FlowLoop, label, x.Pos(), x.Body.Lbrace)
		b.connect(preds, id)
		ctx := b.push(true, id)
		body := b.stmts(x.Body.List, []pend{{from: id, label: "each"}})
		b.backEdges(body, id)
		b.pop()
		return append([]pend{{from: id, label: "done"}}, ctx.breaks...)

	case *ast.SwitchStmt:
		preds = b.simpleInit(x.Init, preds)
		label := "switch"
		if x.Tag != nil {
			label = "switch " + oneLine(b.text(x.Tag.Pos(), x.Tag.End()), 60)
		}
		return b.clauses(label, x.Pos(), x.Body, preds)

	case *ast.TypeSwitchStmt:
		preds = b.simpleInit(x.Init, preds)
		return b.clauses("switch "+oneLine(b.text(x.Assign.Pos(), x.Assign.End()), 60), x.Pos(), x.Body, preds)

	case *ast.SelectStmt:
		return b.clauses("select", x.Pos(), x.Body, preds)

	case *ast.BranchStmt:
		label := x.Tok.String()
		if x.Label != nil {
			label += " " + x.Label.Name
		}
		switch x.Tok {
		case token.BREAK:
			if ctx := b.target(x.Label, false); ctx != nil {
				ctx.breaks = append(ctx.breaks, preds...)
				return nil
			}
		case token.CONTINUE:
			if ctx := b.target(x.Label, true); ctx != nil {
				for _, p := range preds {
					b.edges = append(b.edges, FlowEdge{From: p.from, To: ctx.continueTo, Label: p.label + labelSep(p.label) + "continue", Back: true})
				}
				return nil
			}
		case token.FALLTHROUGH:
			return preds
		}
		id := b.add(FlowJump, label, x.Pos(), x.End())
		b.connect(preds, id)
		if x.Tok == token.GOTO && x.Label != nil {
			b.gotos = append(b.gotos, pend{from: id, label: x.Label.Name})
		}
		return nil
	}
	// Anything else: draw as an opaque block.
	id := b.add(FlowBlock, "", s.Pos(), s.End())
	b.node(id).Code = []string{oneLine(b.text(s.Pos(), s.End()), 90)}
	b.describe(id, s)
	b.connect(preds, id)
	return []pend{{from: id}}
}

// linkGotos connects each goto's jump node to the first node of its
// label's statement; jumps back up the function are back edges.
func (b *cfgBuilder) linkGotos() {
	for _, g := range b.gotos {
		to, ok := b.labels[g.label]
		if !ok {
			continue
		}
		back := b.node(to).Line <= b.node(g.from).Line
		b.edges = append(b.edges, FlowEdge{From: g.from, To: to, Label: "goto", Back: back})
	}
}

// labelSep returns the separator used when appending to an edge label.
func labelSep(l string) string {
	if l == "" {
		return ""
	}
	return " · "
}

// clauses builds a switch, type switch or select: a decision node with one
// case node per clause, plus a "no match" exit when there is no default.
func (b *cfgBuilder) clauses(label string, pos token.Pos, body *ast.BlockStmt, preds []pend) []pend {
	id := b.add(FlowSwitch, label, pos, body.Lbrace)
	b.connect(preds, id)
	ctx := b.push(false, "")
	var out []pend
	hasDefault := false
	for _, cl := range body.List {
		var caseLabel string
		var stmts []ast.Stmt
		var cpos, cend token.Pos
		switch c := cl.(type) {
		case *ast.CaseClause:
			if c.List == nil {
				caseLabel, hasDefault = "default", true
			} else {
				caseLabel = "case " + oneLine(b.text(c.List[0].Pos(), c.List[len(c.List)-1].End()), 50)
			}
			stmts, cpos, cend = c.Body, c.Pos(), c.Colon
		case *ast.CommClause:
			if c.Comm == nil {
				caseLabel, hasDefault = "default", true
			} else {
				caseLabel = "case " + oneLine(b.text(c.Comm.Pos(), c.Comm.End()), 50)
			}
			stmts, cpos, cend = c.Body, c.Pos(), c.Colon
		}
		cid := b.add(FlowCase, caseLabel, cpos, cend)
		b.connect([]pend{{from: id}}, cid)
		out = append(out, b.stmts(stmts, []pend{{from: cid}})...)
	}
	b.pop()
	if !hasDefault && !strings.HasPrefix(label, "select") {
		out = append(out, pend{from: id, label: "no match"})
	}
	return append(out, ctx.breaks...)
}

// backEdges connects the end of a loop body back to the loop header.
func (b *cfgBuilder) backEdges(preds []pend, loop string) {
	for _, p := range preds {
		b.edges = append(b.edges, FlowEdge{From: p.from, To: loop, Label: p.label, Back: true})
	}
}

// push opens a break/continue scope for a loop or switch, consuming any
// pending statement label.
func (b *cfgBuilder) push(isLoop bool, cont string) *breakCtx {
	ctx := &breakCtx{label: b.label, isLoop: isLoop, continueTo: cont}
	b.label = ""
	b.stack = append(b.stack, ctx)
	return ctx
}

// pop closes the innermost break/continue scope.
func (b *cfgBuilder) pop() { b.stack = b.stack[:len(b.stack)-1] }

// target finds the scope a break or continue refers to: the labeled one,
// or the innermost (loop-only for continue).
func (b *cfgBuilder) target(label *ast.Ident, loopOnly bool) *breakCtx {
	for i := len(b.stack) - 1; i >= 0; i-- {
		c := b.stack[i]
		if label != nil {
			if c.label == label.Name {
				return c
			}
			continue
		}
		if !loopOnly || c.isLoop {
			return c
		}
	}
	return nil
}
