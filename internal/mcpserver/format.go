package mcpserver

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

// Text renderers. Agents get compact, line-oriented text rather than raw
// JSON: it is cheaper in tokens and easier to reason about.

// symLine renders a symbol as `Name [kind] file:start-end id=...`.
func symLine(s *analysis.Symbol) string {
	if s == nil {
		return "(unknown)"
	}
	if s.Kind == analysis.KindExternal {
		return fmt.Sprintf("%s [external %s]", s.Qualified, s.Package)
	}
	h := ""
	if s.Heuristic {
		h = " ~heuristic"
	}
	return fmt.Sprintf("%s [%s] %s:%d-%d id=%s%s", s.Qualified, s.Kind, s.File, s.Line, s.EndLine, s.ID, h)
}

// params renders a parameter list as `name type, ...`.
func params(ps []analysis.Param) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = strings.TrimSpace(p.Name + " " + p.Type)
	}
	return strings.Join(parts, ", ")
}

// withLineNumbers prefixes lines from..to (1-based) with right-aligned
// line numbers; to <= 0 means the end of the content.
func withLineNumbers(content string, from, to int) string {
	lines := strings.Split(content, "\n")
	if from < 1 {
		from = 1
	}
	if to <= 0 || to > len(lines) {
		to = len(lines)
	}
	var sb strings.Builder
	width := len(fmt.Sprint(to))
	for i := from; i <= to && i <= len(lines); i++ {
		fmt.Fprintf(&sb, "%*d│ %s\n", width, i, lines[i-1])
	}
	return sb.String()
}

// renderTree draws a directory tree with box-drawing characters, down to
// maxDepth levels.
func renderTree(n *workspace.TreeNode, maxDepth int) string {
	var sb strings.Builder
	var walk func(n *workspace.TreeNode, prefix string, depth int)
	walk = func(n *workspace.TreeNode, prefix string, depth int) {
		for i, c := range n.Children {
			last := i == len(n.Children)-1
			branch, next := "├── ", "│   "
			if last {
				branch, next = "└── ", "    "
			}
			if c.Dir {
				fmt.Fprintf(&sb, "%s%s%s/ (%d files)\n", prefix, branch, c.Name, c.Files)
				if depth < maxDepth {
					walk(c, prefix+next, depth+1)
				}
			} else {
				fmt.Fprintf(&sb, "%s%s%s\n", prefix, branch, c.Name)
			}
		}
	}
	walk(n, "", 1)
	return sb.String()
}

// renderGraph lists a graph's nodes and edges in compact text, adding the
// package to labels that would otherwise be ambiguous.
func renderGraph(g *analysis.Graph) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s — %d nodes, %d edges", g.Title, len(g.Nodes), len(g.Edges))
	if g.Truncated {
		sb.WriteString(" (truncated)")
	}
	sb.WriteString("\n\nNodes:\n")
	label := map[string]string{}
	seen := map[string]int{}
	for _, n := range g.Nodes {
		seen[n.Label]++
	}
	for _, n := range g.Nodes {
		if seen[n.Label] > 1 && n.Group != "" {
			n.Label += " (" + path.Base(n.Group) + ")"
		}
		label[n.ID] = n.Label
		loc := ""
		switch {
		case n.File != "":
			loc = fmt.Sprintf(" %s:%d", n.File, n.Line)
		case n.Kind == "package" && n.ID != n.Label:
			loc = " id=" + n.ID
		}
		extra := ""
		if len(n.Badges) > 0 {
			extra = " {" + strings.Join(n.Badges, ",") + "}"
		}
		root := ""
		if n.Root {
			root = "★ "
		}
		fmt.Fprintf(&sb, "  %s%s [%s]%s%s\n", root, n.Label, n.Kind, loc, extra)
	}
	sb.WriteString("\nEdges:\n")
	for _, e := range g.Edges {
		count := ""
		if e.Count > 1 {
			count = fmt.Sprintf(" ×%d", e.Count)
		}
		h := ""
		if e.Heuristic {
			h = " ~"
		}
		fmt.Fprintf(&sb, "  %s → %s (%s%s)%s\n", label[e.From], label[e.To], e.Kind, count, h)
	}
	if g.Note != "" {
		sb.WriteString("\n" + g.Note + "\n")
	}
	sb.WriteString(precisionNote(g))
	return sb.String()
}

// precisionNote says how far to trust a graph with name-matched (~) edges,
// so agents treat it as approximate; "" when every edge is type-checked.
func precisionNote(g *analysis.Graph) string {
	lexical := false
	for _, e := range g.Edges {
		lexical = lexical || e.Heuristic
	}
	if !lexical {
		return ""
	}
	note := "\nPrecision: ~ edges are lexical name matches, not type-checked: a same-named function may be the real target, and calls through values, aliases or framework wiring are missed, so callers may be incomplete."
	if g.Kind == "deps" {
		note = "\nPrecision: ~ edges come from import statements resolved to files in the repository; path aliases, workspace packages and dynamic imports aren't followed, so some dependencies may be missing."
	}
	if g.Kind == "trace" {
		note += " An {entry} badge means no caller was found, not necessarily a real entry point."
	}
	lang := ""
	for _, n := range g.Nodes {
		if n.Root && n.File != "" {
			lang = workspace.Language(n.File)
		}
	}
	if _, ok := languageNames[lang]; ok {
		return note + fmt.Sprintf(" Known gaps: get_language_support language=%s.\n", lang)
	}
	return note + " Known gaps per language: get_language_support.\n"
}

// renderFlow describes a control-flow graph: signature, inputs, outputs
// and one line per node with its outgoing edges.
func renderFlow(fl *analysis.Flow) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s  (%s:%d-%d)\n", fl.Signature, fl.File, fl.Line, fl.EndLine)
	if fl.Receiver != nil {
		fmt.Fprintf(&sb, "receiver: %s %s\n", fl.Receiver.Name, fl.Receiver.Type)
	}
	fmt.Fprintf(&sb, "inputs:  %s\noutputs: %s\n\ncontrol flow:\n", orNone(params(fl.Inputs)), orNone(params(fl.Outputs)))
	out := map[string][]analysis.FlowEdge{}
	for _, e := range fl.Edges {
		out[e.From] = append(out[e.From], e)
	}
	for _, n := range fl.Nodes {
		text := n.Label
		if len(n.Code) > 0 {
			text = strings.Join(n.Code, "; ")
			if len(text) > 160 {
				text = text[:157] + "..."
			}
		}
		fmt.Fprintf(&sb, "  %s [%s L%d] %s", n.ID, n.Kind, n.Line, text)
		var targets []string
		for _, e := range out[n.ID] {
			t := e.To
			if e.Label != "" {
				t = e.Label + "→" + t
			}
			if e.Back {
				t += "↺"
			}
			targets = append(targets, t)
		}
		if len(targets) > 0 {
			sb.WriteString("  ⇒ " + strings.Join(targets, ", "))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// orNone returns s, or "(none)" when s is empty.
func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// topN ranks the n highest values with their details.
func topN(values map[string]float64, details map[string]string, n int) string {
	type kv struct {
		k string
		v float64
	}
	var list []kv
	for k, v := range values {
		if v > 0 {
			list = append(list, kv{k, v})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v || (list[i].v == list[j].v && list[i].k < list[j].k) })
	var sb strings.Builder
	for i, e := range list {
		if i == n {
			fmt.Fprintf(&sb, "  …%d more\n", len(list)-n)
			break
		}
		fmt.Fprintf(&sb, "  %6.0f  %s  %s\n", e.v, e.k, details[e.k])
	}
	if len(list) == 0 {
		sb.WriteString("  (no data)\n")
	}
	return sb.String()
}
