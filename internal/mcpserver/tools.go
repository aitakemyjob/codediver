package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/proposals"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

type noArgs struct{}

type repoArgs struct {
	Repo string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
}

type listFilesArgs struct {
	Repo  string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Path  string `json:"path,omitempty" jsonschema:"directory to list (repo-relative); empty for the root"`
	Depth int    `json:"depth,omitempty" jsonschema:"how many directory levels to expand (default 3)"`
}

type readFileArgs struct {
	Repo      string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Path      string `json:"path" jsonschema:"repo-relative file path"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first line to return (1-based)"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"last line to return"`
	Show      bool   `json:"show,omitempty" jsonschema:"also open the file in CodeDiver with the range highlighted"`
}

type searchArgs struct {
	Repo          string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Query         string `json:"query" jsonschema:"text or regular expression to search for"`
	Regex         bool   `json:"regex,omitempty" jsonschema:"treat query as a Go regular expression"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
	Glob          string `json:"glob,omitempty" jsonschema:"limit to files matching this glob (e.g. *.go) or directory prefix"`
	Limit         int    `json:"limit,omitempty" jsonschema:"max matches (default 100)"`
}

type fileArgs struct {
	Repo string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Path string `json:"path" jsonschema:"repo-relative file path"`
	Show bool   `json:"show,omitempty" jsonschema:"also open the file with its summary panel in CodeDiver"`
}

type symbolSearchArgs struct {
	Repo  string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Query string `json:"query" jsonschema:"symbol name or fragment"`
	Limit int    `json:"limit,omitempty"`
}

type symbolArgs struct {
	Repo        string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Symbol      string `json:"symbol" jsonschema:"symbol id, qualified name (Type.Method), bare name, or file:line"`
	IncludeCode bool   `json:"include_code,omitempty" jsonschema:"include the symbol's source"`
	Show        bool   `json:"show,omitempty" jsonschema:"also reveal the symbol in CodeDiver"`
}

type callGraphArgs struct {
	Repo            string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Symbol          string `json:"symbol" jsonschema:"function or method to center the graph on"`
	Direction       string `json:"direction,omitempty" jsonschema:"callees (default), callers or both"`
	Depth           int    `json:"depth,omitempty" jsonschema:"levels to expand (default 2, max 8)"`
	IncludeExternal bool   `json:"include_external,omitempty" jsonschema:"include calls into stdlib/dependencies"`
	IncludeRefs     bool   `json:"include_refs,omitempty" jsonschema:"include functions passed as values (handlers, callbacks)"`
	Show            bool   `json:"show,omitempty" jsonschema:"display the graph to the developer"`
}

type traceArgs struct {
	Repo   string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Symbol string `json:"symbol" jsonschema:"function whose incoming execution paths to trace"`
	Depth  int    `json:"depth,omitempty" jsonschema:"max caller levels (default 6)"`
	Show   bool   `json:"show,omitempty" jsonschema:"display the trace to the developer"`
}

type flowArgs struct {
	Repo   string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Symbol string `json:"symbol" jsonschema:"function or method"`
	Show   bool   `json:"show,omitempty" jsonschema:"display the control-flow graph to the developer"`
}

type depsArgs struct {
	Repo            string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Focus           string `json:"focus,omitempty" jsonschema:"package id to focus on (shows only its direct neighbours)"`
	IncludeExternal bool   `json:"include_external,omitempty"`
	Show            bool   `json:"show,omitempty"`
}

type typesArgs struct {
	Repo  string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Scope string `json:"scope" jsonschema:"package id, directory, or file whose types to diagram"`
	Show  bool   `json:"show,omitempty"`
}

type diffArgs struct {
	Repo   string   `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Commit string   `json:"commit,omitempty" jsonschema:"show what this one commit changed (vs. its parent; works for the first commit too). Overrides base/head"`
	Base   string   `json:"base,omitempty" jsonschema:"base ref (commit, branch, tag). Empty = HEAD"`
	Head   string   `json:"head,omitempty" jsonschema:"head ref. Empty = working tree"`
	Staged bool     `json:"staged,omitempty" jsonschema:"diff the index instead of the working tree"`
	Paths  []string `json:"paths,omitempty" jsonschema:"limit to these paths"`
	Patch  bool     `json:"patch,omitempty" jsonschema:"include full patch text (otherwise a summary with changed symbols)"`
	Show   bool     `json:"show,omitempty" jsonschema:"open the diff review in CodeDiver"`
}

type logArgs struct {
	Repo  string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Limit int    `json:"limit,omitempty" jsonschema:"number of commits (default 20)"`
	Ref   string `json:"ref,omitempty"`
	Path  string `json:"path,omitempty" jsonschema:"only commits touching this path"`
}

type heatArgs struct {
	Repo   string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Metric string `json:"metric,omitempty" jsonschema:"churn (default), changes, proposals, complexity, agent, size"`
	Show   bool   `json:"show,omitempty"`
}

type showCodeArgs struct {
	Repo    string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Path    string `json:"path" jsonschema:"repo-relative file path"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
	Message string `json:"message,omitempty" jsonschema:"short note shown to the developer"`
}

type walkthroughArgs struct {
	Repo    string          `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Title   string          `json:"title"`
	Summary string          `json:"summary,omitempty" jsonschema:"markdown intro shown before the first step"`
	Steps   []core.WalkStep `json:"steps" jsonschema:"ordered stops through the code"`
}

type explainArgs struct {
	Repo        string           `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Symbol      string           `json:"symbol" jsonschema:"function or method being explained"`
	Summary     string           `json:"summary" jsonschema:"markdown: what the function is for and how it works"`
	Inputs      []core.ParamNote `json:"inputs,omitempty" jsonschema:"meaning of each parameter (and receiver)"`
	Outputs     []core.ParamNote `json:"outputs,omitempty" jsonschema:"meaning of each return value"`
	Steps       []core.LogicStep `json:"steps,omitempty" jsonschema:"the function's logic as ordered steps tied to line ranges"`
	SideEffects []string         `json:"side_effects,omitempty" jsonschema:"I/O, mutations, goroutines, panics, etc."`
	Notes       string           `json:"notes,omitempty" jsonschema:"gotchas, edge cases, or suggestions"`
}

type diagramArgs struct {
	Repo        string             `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Title       string             `json:"title"`
	Description string             `json:"description,omitempty" jsonschema:"markdown caption"`
	Mermaid     string             `json:"mermaid,omitempty" jsonschema:"mermaid source (sequenceDiagram, flowchart, classDiagram, stateDiagram, erDiagram...)"`
	Nodes       []core.DiagramNode `json:"nodes,omitempty" jsonschema:"interactive graph nodes (alternative to mermaid)"`
	Edges       []core.DiagramEdge `json:"edges,omitempty"`
	Layout      string             `json:"layout,omitempty" jsonschema:"LR (default) or TB for nodes/edges diagrams"`
}

type annotateArgs struct {
	Repo        string            `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Annotations []core.Annotation `json:"annotations"`
	Replace     bool              `json:"replace,omitempty" jsonschema:"clear previous annotations first"`
}

type proposeArgs struct {
	Repo    string                  `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Title   string                  `json:"title" jsonschema:"short title of the change set"`
	Summary string                  `json:"summary" jsonschema:"markdown: what changes, why, and how to verify"`
	Changes []proposals.ChangeInput `json:"changes"`
}

type proposalArgs struct {
	Repo string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	ID   string `json:"id" jsonschema:"proposal id returned by propose_changes"`
}

// register adds every MCP tool. Each handler resolves the repository
// through AgentRepo, which enforces the user's access switches.
func (s *Server) register() {
	c := s.core
	none := func(noArgs) string { return "" }

	addTool(s, &mcp.Tool{Name: "list_repos", Description: "List repositories shared with agents in CodeDiver, which one is active, and their git state.", Annotations: readOnly("List repositories")},
		none, func(ctx context.Context, _ noArgs) (string, error) {
			cfg := c.Config.Get()
			if !cfg.Settings.MCPEnabled {
				return "", core.ErrAgentDisabled
			}
			var sb strings.Builder
			for _, r := range c.ListRepos(ctx) {
				if !r.MCPEnabled {
					continue
				}
				active := ""
				if r.ID == cfg.ActiveRepo {
					active = " (active)"
				}
				git := "not a git repo"
				if r.Git.IsRepo {
					git = fmt.Sprintf("branch %s, %d changed files", r.Git.Branch, r.Git.Dirty)
				}
				fmt.Fprintf(&sb, "- %s%s id=%s — %s\n", r.Name, active, r.ID, git)
			}
			if sb.Len() == 0 {
				return "No repositories are shared with agents. Ask the developer to add one in CodeDiver.", nil
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "get_developer_context", Description: "What the developer is currently looking at in CodeDiver: repo, view, file, cursor line, selected text and symbol. Use this when they say 'this function' or 'here'.", Annotations: readOnly("Developer context")},
		none, func(ctx context.Context, _ noArgs) (string, error) {
			u := c.UIContext()
			if u.Repo == "" {
				return "The developer has not opened anything yet.", nil
			}
			rt, err := c.AgentRepo(u.Repo)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "repo: %s\nview: %s\n", rt.Name(), u.View)
			if u.Title != "" {
				fmt.Fprintf(&sb, "title: %s\n", u.Title)
			}
			if u.File != "" {
				if _, _, err := rt.WS().Resolve(u.File, nil); err == nil {
					fmt.Fprintf(&sb, "file: %s line %d-%d\n", u.File, u.Line, max(u.EndLine, u.Line))
				}
			}
			if u.Symbol != "" {
				if d, err := c.SymbolDetail(ctx, rt, u.Symbol, false); err == nil {
					fmt.Fprintf(&sb, "symbol: %s\nsignature: %s\n", symLine(d.Symbol), d.Symbol.Signature)
				}
			}
			if u.Selection != "" {
				fmt.Fprintf(&sb, "selection:\n```\n%s\n```\n", u.Selection)
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "repo_overview", Description: "High-level structure of a repository: languages, size, top-level directories, likely entry points, churn×complexity hotspots and README excerpt. A good first call.", Annotations: readOnly("Repository overview")},
		func(a repoArgs) string { return a.Repo }, func(ctx context.Context, a repoArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			ov, err := c.Overview(ctx, rt)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "# %s\n%s\n", ov.Repo.Name, ov.Repo.Path)
			if ov.Git.IsRepo {
				fmt.Fprintf(&sb, "git: branch %s, %d changed files\n", ov.Git.Branch, ov.Git.Dirty)
			}
			st := ov.Stats
			fmt.Fprintf(&sb, "\n%d analyzed files · %d functions · %d types · %d packages · %d call edges\nlanguages: %s\n", st.Files, st.Functions, st.Types, st.Packages, st.Edges, langList(st.Languages))
			sb.WriteString("\n## Top-level\n")
			for _, d := range ov.TopDirs {
				fmt.Fprintf(&sb, "- %s — %d files, %d lines %s\n", d.Path, d.Files, d.Lines, langList(d.Languages))
			}
			if len(ov.EntryPoints) > 0 {
				sb.WriteString("\n## Likely entry points\n")
				for _, e := range ov.EntryPoints {
					fmt.Fprintf(&sb, "- %s\n", symLine(e))
				}
			}
			if len(ov.Hotspots) > 0 {
				sb.WriteString("\n## Hotspots (churn × complexity)\n")
				for _, h := range ov.Hotspots {
					fmt.Fprintf(&sb, "- %s — %d commits, complexity %d\n", h.Path, h.Churn, h.Complexity)
				}
			}
			if ov.Readme != "" {
				fmt.Fprintf(&sb, "\n## README (excerpt)\n%s\n", ov.Readme)
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "list_files", Description: "Directory tree of the repository (respects .gitignore and CodeDiver excludes).", Annotations: readOnly("List files")},
		func(a listFilesArgs) string { return a.Path }, func(ctx context.Context, a listFilesArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			tree, err := c.Tree(rt)
			if err != nil {
				return "", err
			}
			node := tree
			if p := strings.Trim(a.Path, "/"); p != "" && p != "." {
				node = findNode(tree, p)
				if node == nil {
					return "", fmt.Errorf("directory %q not found", a.Path)
				}
			}
			if a.Depth <= 0 {
				a.Depth = 3
			}
			return fmt.Sprintf("%s/ (%d files)\n%s", node.Path, node.Files, renderTree(node, a.Depth)), nil
		})

	addTool(s, &mcp.Tool{Name: "read_file", Description: "Read a file (with line numbers). Optionally show it to the developer with the range highlighted.", Annotations: readOnly("Read file")},
		func(a readFileArgs) string { return lineRange(a.Path, a.StartLine, a.EndLine) }, func(ctx context.Context, a readFileArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			fc, err := c.ReadFile(rt, a.Path)
			if err != nil {
				return "", err
			}
			c.TouchFile(rt, fc.Path)
			if fc.Binary {
				return fmt.Sprintf("%s is a binary file (%d bytes)", fc.Path, fc.Size), nil
			}
			if a.Show {
				c.Show(core.Focus{View: "code", Repo: rt.ID(), Params: map[string]any{"file": fc.Path, "line": a.StartLine, "endLine": a.EndLine}})
			}
			head := fmt.Sprintf("%s (%s, %d lines)", fc.Path, fc.Language, fc.Lines)
			if fc.Truncate {
				head += " [truncated at 2MB]"
			}
			return head + "\n" + withLineNumbers(fc.Content, a.StartLine, a.EndLine), nil
		})

	addTool(s, &mcp.Tool{Name: "search_code", Description: "Grep the repository for text or a regular expression.", Annotations: readOnly("Search code")},
		func(a searchArgs) string { return a.Query }, func(ctx context.Context, a searchArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			if a.Limit <= 0 {
				a.Limit = 100
			}
			matches, more, err := c.Search(rt, workspace.SearchOptions{Query: a.Query, Regex: a.Regex, CaseSensitive: a.CaseSensitive, Glob: a.Glob, Limit: a.Limit})
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			for _, m := range matches {
				fmt.Fprintf(&sb, "%s:%d: %s\n", m.Path, m.Line, strings.TrimSpace(m.Text))
			}
			if more {
				sb.WriteString("… more matches omitted; narrow the query or glob\n")
			}
			if len(matches) == 0 {
				return "no matches", nil
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "get_file_summary", Description: "Everything declared in one file: functions and methods (signature, inputs → outputs, complexity, caller/callee counts, doc), types with fields and methods, top-level variables and constants, and imports. Cheaper than reading the whole file when you need its shape.", Annotations: readOnly("File summary")},
		func(a fileArgs) string { return a.Path }, func(ctx context.Context, a fileArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			fs, err := c.FileSummary(ctx, rt, a.Path)
			if err != nil {
				return "", err
			}
			c.TouchFile(rt, fs.Path)
			if a.Show {
				c.Show(core.Focus{View: "code", Repo: rt.ID(), Params: map[string]any{"file": fs.Path, "summary": true}})
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "%s (%s, %d lines)\n", fs.Path, fs.Language, fs.Lines)
			if len(fs.Imports) > 0 {
				fmt.Fprintf(&sb, "\nimports: %s\n", strings.Join(fs.Imports, ", "))
			}
			if len(fs.Types) > 0 {
				sb.WriteString("\ntypes:\n")
				for _, t := range fs.Types {
					fmt.Fprintf(&sb, "  L%d %s %s", t.Line, t.Kind, t.Qualified)
					if len(t.Fields) > 0 {
						fmt.Fprintf(&sb, " {%s}", params(t.Fields))
					}
					if len(t.Methods) > 0 {
						fmt.Fprintf(&sb, " · %d methods", len(t.Methods))
					}
					sb.WriteString("\n")
				}
			}
			if len(fs.Functions) > 0 {
				sb.WriteString("\nfunctions:\n")
				for _, f := range fs.Functions {
					fmt.Fprintf(&sb, "  L%d-%d %s(%s)", f.Line, f.EndLine, f.Qualified, params(f.Params))
					if len(f.Results) > 0 {
						fmt.Fprintf(&sb, " → %s", params(f.Results))
					}
					fmt.Fprintf(&sb, " · cx %d · %d callers · %d calls", f.Complexity, f.Callers, f.Callees)
					if f.Doc != "" {
						fmt.Fprintf(&sb, " — %s", firstDocLine(f.Doc))
					}
					sb.WriteString("\n")
				}
			}
			if len(fs.Variables) > 0 {
				sb.WriteString("\nvariables & constants:\n")
				for _, v := range fs.Variables {
					fmt.Fprintf(&sb, "  L%d %s %s", v.Line, v.Kind, v.Name)
					if v.Type != "" {
						fmt.Fprintf(&sb, " %s", v.Type)
					}
					if v.Value != "" {
						fmt.Fprintf(&sb, " = %s", v.Value)
					}
					sb.WriteString("\n")
				}
			}
			if fs.Heuristic {
				sb.WriteString("\n(non-Go file: symbols found heuristically)\n")
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "search_symbols", Description: "Find functions, methods, types and classes by name.", Annotations: readOnly("Search symbols")},
		func(a symbolSearchArgs) string { return a.Query }, func(ctx context.Context, a symbolSearchArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			syms, err := c.SearchSymbols(ctx, rt, a.Query, a.Limit)
			if err != nil {
				return "", err
			}
			if len(syms) == 0 {
				return "no symbols found", nil
			}
			var sb strings.Builder
			for _, sym := range syms {
				sb.WriteString(symLine(sym) + "\n")
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "get_symbol", Description: "Details of a function/method/type: signature, parameters, results, doc, callers, callees, implementations and (optionally) source.", Annotations: readOnly("Get symbol")},
		func(a symbolArgs) string { return a.Symbol }, func(ctx context.Context, a symbolArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			d, err := c.SymbolDetail(ctx, rt, a.Symbol, a.IncludeCode)
			if err != nil {
				return "", err
			}
			sym := d.Symbol
			c.TouchFile(rt, sym.File)
			if a.Show {
				c.Show(core.Focus{View: "code", Repo: rt.ID(), Params: map[string]any{"file": sym.File, "line": sym.Line, "endLine": sym.EndLine, "symbol": sym.ID}})
			}
			var sb strings.Builder
			sb.WriteString(symLine(sym) + "\n")
			if sym.Signature != "" {
				fmt.Fprintf(&sb, "signature: %s\n", sym.Signature)
			}
			if len(sym.Params) > 0 {
				fmt.Fprintf(&sb, "params: %s\n", params(sym.Params))
			}
			if len(sym.Results) > 0 {
				fmt.Fprintf(&sb, "results: %s\n", params(sym.Results))
			}
			if len(sym.Fields) > 0 {
				fmt.Fprintf(&sb, "fields: %s\n", params(sym.Fields))
			}
			if sym.Complexity > 0 {
				fmt.Fprintf(&sb, "cyclomatic complexity: %d\n", sym.Complexity)
			}
			if sym.Doc != "" {
				fmt.Fprintf(&sb, "doc: %s\n", sym.Doc)
			}
			section := func(title string, refs []core.EdgeRef) {
				if len(refs) == 0 {
					return
				}
				fmt.Fprintf(&sb, "\n%s (%d):\n", title, len(refs))
				for _, r := range refs {
					fmt.Fprintf(&sb, "  - %s via %s %s\n", symLine(r.Symbol), r.Kind, linesStr(r.Lines))
				}
			}
			section("callers", d.Callers)
			section("callees", d.Callees)
			for title, list := range map[string][]*analysis.Symbol{"implementations": d.Implementations, "implements": d.Implements, "methods": d.Methods} {
				if len(list) > 0 {
					fmt.Fprintf(&sb, "\n%s:\n", title)
					for _, x := range list {
						fmt.Fprintf(&sb, "  - %s\n", symLine(x))
					}
				}
			}
			if d.Code != "" {
				fmt.Fprintf(&sb, "\nsource:\n%s", withLineNumbers(d.Code, 1, 0))
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "get_call_graph", Description: "Call graph around a function: what it calls (callees), what calls it (callers), or both. Go is analyzed precisely with type information, including interface dispatch; other languages use name-matching heuristics (marked ~).", Annotations: readOnly("Call graph")},
		func(a callGraphArgs) string { return a.Symbol + " " + a.Direction }, func(ctx context.Context, a callGraphArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			g, err := c.CallGraph(ctx, rt, a.Symbol, analysis.CallGraphOptions{Direction: a.Direction, Depth: a.Depth, IncludeExternal: a.IncludeExternal, IncludeRefs: a.IncludeRefs})
			if err != nil {
				return "", err
			}
			if a.Show {
				c.Show(core.Focus{View: "graph", Repo: rt.ID(), Title: g.Title, Params: map[string]any{"kind": "callgraph", "symbol": g.Root, "direction": g.Direction, "depth": a.Depth, "includeExternal": a.IncludeExternal, "includeRefs": a.IncludeRefs}})
			}
			return renderGraph(g), nil
		})

	addTool(s, &mcp.Tool{Name: "trace_to", Description: "How does execution reach this function? Walks callers back to entry points and returns every path. Great for 'how is this triggered' questions.", Annotations: readOnly("Trace to function")},
		func(a traceArgs) string { return a.Symbol }, func(ctx context.Context, a traceArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			g, err := c.TraceTo(ctx, rt, a.Symbol, a.Depth)
			if err != nil {
				return "", err
			}
			if a.Show {
				c.Show(core.Focus{View: "graph", Repo: rt.ID(), Title: g.Title, Params: map[string]any{"kind": "trace", "symbol": g.Root, "depth": a.Depth}})
			}
			return renderGraph(g), nil
		})

	addTool(s, &mcp.Tool{Name: "get_function_flow", Description: "Control-flow graph of a function (branches, loops, switch cases, early returns, defers, goroutines, panics) plus its inputs and outputs. Precise for Go.", Annotations: readOnly("Function flow")},
		func(a flowArgs) string { return a.Symbol }, func(ctx context.Context, a flowArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			fl, sym, err := c.FunctionFlow(ctx, rt, a.Symbol)
			if err != nil {
				return "", err
			}
			c.TouchFile(rt, sym.File)
			if a.Show {
				c.Show(core.Focus{View: "flow", Repo: rt.ID(), Title: sym.Qualified, Params: map[string]any{"symbol": sym.ID}})
			}
			out := renderFlow(fl)
			if sym.Language != "go" {
				out += "\n(control flow is only derived for Go; read the code and use explain_function to describe the logic)\n"
			}
			return out, nil
		})

	addTool(s, &mcp.Tool{Name: "get_dependencies", Description: "Package/module dependency graph (Go packages, or directories for other languages).", Annotations: readOnly("Dependencies")},
		func(a depsArgs) string { return a.Focus }, func(ctx context.Context, a depsArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			g, err := c.PackageGraph(ctx, rt, a.IncludeExternal, a.Focus)
			if err != nil {
				return "", err
			}
			if a.Show {
				c.Show(core.Focus{View: "graph", Repo: rt.ID(), Title: g.Title, Params: map[string]any{"kind": "deps", "focus": a.Focus, "includeExternal": a.IncludeExternal}})
			}
			return renderGraph(g), nil
		})

	addTool(s, &mcp.Tool{Name: "get_types", Description: "Class/type diagram for a package, directory or file: structs, classes, interfaces, fields, methods and implements relationships.", Annotations: readOnly("Type diagram")},
		func(a typesArgs) string { return a.Scope }, func(ctx context.Context, a typesArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			g, err := c.TypeGraph(ctx, rt, a.Scope)
			if err != nil {
				return "", err
			}
			if a.Show {
				c.Show(core.Focus{View: "graph", Repo: rt.ID(), Title: g.Title, Params: map[string]any{"kind": "types", "scope": a.Scope}})
			}
			var sb strings.Builder
			sb.WriteString(renderGraph(g))
			for _, n := range g.Nodes {
				if len(n.Fields)+len(n.Members) == 0 {
					continue
				}
				fmt.Fprintf(&sb, "\n%s:\n", n.Label)
				for _, f := range n.Fields {
					fmt.Fprintf(&sb, "  field %s %s\n", f.Name, f.Type)
				}
				for _, m := range n.Members {
					fmt.Fprintf(&sb, "  method %s\n", m)
				}
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "git_status", Description: "Branch and changed files in the working tree.", Annotations: readOnly("Git status")},
		func(a repoArgs) string { return "" }, func(ctx context.Context, a repoArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			info, err := rt.Git().Info(ctx)
			if err != nil {
				return "", err
			}
			if !info.IsRepo {
				return "not a git repository", nil
			}
			st, err := rt.Git().Status(ctx)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "branch %s (ahead %d, behind %d)\n", info.Branch, info.Ahead, info.Behind)
			for _, e := range st {
				if rt.WS().Excluded(e.Path, nil) {
					continue
				}
				staged := ""
				if e.Staged {
					staged = " (staged)"
				}
				fmt.Fprintf(&sb, "  %-9s %s%s\n", e.Status, e.Path, staged)
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "git_diff", Description: "Diff summary (files, +/-, and which functions changed) or full patch. Defaults to working tree vs HEAD; pass commit to see what a single commit changed. Set show=true to open CodeDiver's visual review with heatmap and impact graph.", Annotations: readOnly("Git diff")},
		func(a diffArgs) string {
			if a.Commit != "" {
				return "commit " + a.Commit
			}
			if a.Base == "" && a.Head == "" {
				if a.Staged {
					return "staged"
				}
				return "working tree"
			}
			return a.Base + ".." + a.Head
		}, func(ctx context.Context, a diffArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			spec := gitx.DiffSpec{Commit: a.Commit, Base: a.Base, Head: a.Head, Staged: a.Staged, Paths: a.Paths}
			ov, err := c.Diff(ctx, rt, spec, true)
			if err != nil {
				return "", err
			}
			if a.Show {
				c.Show(core.Focus{View: "diff", Repo: rt.ID(), Params: map[string]any{"commit": a.Commit, "base": a.Base, "head": a.Head, "staged": a.Staged}})
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "%d files changed, +%d −%d\n", len(ov.Files), ov.Additions, ov.Deletions)
			for _, f := range ov.Files {
				fmt.Fprintf(&sb, "\n%s %s +%d −%d\n", f.Status, f.Path, f.Additions, f.Deletions)
				for _, cs := range f.Symbols {
					fmt.Fprintf(&sb, "    ~ %s [%s] L%d (%d lines)\n", cs.Name, cs.Kind, cs.Line, cs.Lines)
				}
				if a.Patch {
					sb.WriteString(patchText(f.FileDiff))
				}
			}
			if ov.Impact != nil && len(ov.Impact.Nodes) > 0 {
				var callers []string
				for _, n := range ov.Impact.Nodes {
					if n.Depth > 0 {
						callers = append(callers, n.Label)
					}
				}
				sort.Strings(callers)
				if len(callers) > 0 {
					fmt.Fprintf(&sb, "\nPotentially affected callers (%d): %s\n", len(callers), strings.Join(callers, ", "))
				}
			}
			if ov.Note != "" {
				sb.WriteString("\nnote: " + ov.Note + "\n")
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "git_log", Description: "Recent commits with changed files.", Annotations: readOnly("Git log")},
		func(a logArgs) string { return a.Path }, func(ctx context.Context, a logArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			if a.Limit <= 0 {
				a.Limit = 20
			}
			commits, err := c.GitLog(ctx, rt, a.Limit, a.Ref, a.Path)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			for _, cm := range commits {
				fmt.Fprintf(&sb, "%s %s %s — %s (+%d −%d)\n", cm.Short, cm.Time.Format("2006-01-02"), cm.Author, cm.Subject, cm.Additions, cm.Deletions)
				for i, f := range cm.Files {
					if i == 8 {
						fmt.Fprintf(&sb, "    … %d more files\n", len(cm.Files)-8)
						break
					}
					fmt.Fprintf(&sb, "    %s +%d −%d\n", f.Path, f.Additions, f.Deletions)
				}
			}
			if len(commits) == 0 {
				return "no commits", nil
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "get_heatmap", Description: "Rank files by a metric: churn (commits), changes (working tree diff), proposals (pending agent changes), complexity, agent (what you've looked at), size. show=true renders a treemap heatmap for the developer.", Annotations: readOnly("Heatmap")},
		func(a heatArgs) string { return a.Metric }, func(ctx context.Context, a heatArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			hm, err := c.Heatmap(ctx, rt, a.Metric, "")
			if err != nil {
				return "", err
			}
			if a.Show {
				c.Show(core.Focus{View: "heatmap", Repo: rt.ID(), Params: map[string]any{"metric": hm.Metric}})
			}
			return fmt.Sprintf("%s (%s)\n%s", hm.Metric, hm.Label, topN(hm.Values, hm.Details, 40)), nil
		})

	// ---- visual / collaborative tools ----

	addTool(s, &mcp.Tool{Name: "show_code", Description: "Open a file in CodeDiver and highlight a line range, with an optional message to the developer.", Annotations: visual("Show code")},
		func(a showCodeArgs) string { return lineRange(a.Path, a.Line, a.EndLine) }, func(ctx context.Context, a showCodeArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			_, rel, err := rt.WS().Resolve(a.Path, nil)
			if err != nil {
				return "", err
			}
			c.TouchFile(rt, rel)
			c.Show(core.Focus{View: "code", Repo: rt.ID(), Message: a.Message, Params: map[string]any{"file": rel, "line": a.Line, "endLine": a.EndLine}})
			return "shown " + lineRange(rel, a.Line, a.EndLine), nil
		})

	addTool(s, &mcp.Tool{Name: "start_walkthrough", Description: "Present a guided, step-by-step tour through the code. Each step highlights a file range (or symbol) and shows your markdown explanation; CodeDiver also draws the path between steps. Use for 'walk me through how X works' or to narrate a change.", Annotations: visual("Start walkthrough")},
		func(a walkthroughArgs) string { return fmt.Sprintf("%s (%d steps)", a.Title, len(a.Steps)) }, func(ctx context.Context, a walkthroughArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			w, err := c.CreateWalkthrough(ctx, rt, core.Walkthrough{Title: a.Title, Summary: a.Summary, Steps: a.Steps})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("walkthrough %s with %d steps is now on the developer's screen", w.ID, len(w.Steps)), nil
		})

	addTool(s, &mcp.Tool{Name: "explain_function", Description: "Show an explanation of a function next to its signature card and control-flow graph: what each input means, what each output means, and the logic as steps tied to line ranges. Read the function first.", Annotations: visual("Explain function")},
		func(a explainArgs) string { return a.Symbol }, func(ctx context.Context, a explainArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			ex, err := c.Explain(ctx, rt, core.Explanation{Symbol: a.Symbol, Summary: a.Summary, Inputs: a.Inputs, Outputs: a.Outputs, Steps: a.Steps, SideEffects: a.SideEffects, Notes: a.Notes})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("explanation for %s is displayed", ex.Sym.Qualified), nil
		})

	addTool(s, &mcp.Tool{Name: "show_diagram", Description: "Display a custom diagram: either mermaid source (sequence, flowchart, class, state, ER) or an interactive node/edge graph whose nodes can link to files. Use to illustrate architecture, request flows, data flow or a planned change.", Annotations: visual("Show diagram")},
		func(a diagramArgs) string { return a.Title }, func(ctx context.Context, a diagramArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			d, err := c.CreateDiagram(rt, a.Title, a.Description, a.Mermaid, a.Layout, a.Nodes, a.Edges)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("diagram %s displayed", d.ID), nil
		})

	addTool(s, &mcp.Tool{Name: "annotate_code", Description: "Attach inline notes (info, warning, bug, idea, question) to lines of code; they appear in CodeDiver's editor gutter.", Annotations: visual("Annotate code")},
		func(a annotateArgs) string { return fmt.Sprintf("%d notes", len(a.Annotations)) }, func(ctx context.Context, a annotateArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			all, err := c.Annotate(rt, a.Annotations, a.Replace)
			if err != nil {
				return "", err
			}
			if len(a.Annotations) > 0 {
				first := a.Annotations[0]
				c.Show(core.Focus{View: "code", Repo: rt.ID(), Params: map[string]any{"file": first.File, "line": first.Line, "endLine": first.EndLine}})
			}
			return fmt.Sprintf("%d annotations now visible", len(all)), nil
		})

	f := false
	addTool(s, &mcp.Tool{Name: "propose_changes", Description: "Propose code changes for the developer to review visually (side-by-side diffs, a heatmap of touched files/functions, and a blast-radius graph of affected callers). Nothing is written until the developer accepts. Prefer `edits` (exact search/replace) for targeted changes and `new_content` for new files or rewrites. Returns a proposal id; poll get_proposal for the decision.",
		Annotations: &mcp.ToolAnnotations{Title: "Propose changes", DestructiveHint: &f, OpenWorldHint: &f}},
		func(a proposeArgs) string { return fmt.Sprintf("%s (%d files)", a.Title, len(a.Changes)) }, func(ctx context.Context, a proposeArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			p, err := c.Propose(ctx, rt, a.Title, a.Summary, a.Changes)
			if err != nil {
				return "", err
			}
			add, del := p.Additions()
			var sb strings.Builder
			fmt.Fprintf(&sb, "proposal %s is awaiting the developer's review: %d files, +%d −%d\n", p.ID, len(p.Changes), add, del)
			for _, ch := range p.Changes {
				fmt.Fprintf(&sb, "  %s %s +%d −%d", ch.Action, ch.Path, ch.Additions, ch.Deletions)
				if len(ch.Symbols) > 0 {
					fmt.Fprintf(&sb, " touches %d symbols", len(ch.Symbols))
				}
				sb.WriteString("\n")
			}
			if n := len(p.Impact); n > 0 {
				fmt.Fprintf(&sb, "blast radius: %d symbols (changed + transitive callers)\n", n)
			}
			sb.WriteString("Call get_proposal with this id to learn whether it was accepted.")
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "get_proposal", Description: "Status of a proposal: pending, applied, rejected, partial or conflict, per file, plus any feedback the developer left.", Annotations: readOnly("Get proposal")},
		func(a proposalArgs) string { return a.ID }, func(ctx context.Context, a proposalArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			p, ok := c.Proposals.Get(a.ID)
			if !ok || p.Repo != rt.ID() {
				return "", fmt.Errorf("unknown proposal %q", a.ID)
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "proposal %s %q: %s\n", p.ID, p.Title, p.Status)
			for _, ch := range p.Changes {
				fmt.Fprintf(&sb, "  %-8s %s %s", ch.Status, ch.Action, ch.Path)
				if ch.Error != "" {
					fmt.Fprintf(&sb, " — %s", ch.Error)
				}
				sb.WriteString("\n")
			}
			if p.Feedback != "" {
				fmt.Fprintf(&sb, "developer feedback: %s\n", p.Feedback)
			}
			return sb.String(), nil
		})
}

// findNode returns the directory node at path p, or nil.
func findNode(n *workspace.TreeNode, p string) *workspace.TreeNode {
	if n.Path == p {
		return n
	}
	for _, c := range n.Children {
		if c.Dir && (c.Path == p || strings.HasPrefix(p, c.Path+"/")) {
			if r := findNode(c, p); r != nil {
				return r
			}
		}
	}
	return nil
}

// firstDocLine returns the first line of a doc comment.
func firstDocLine(doc string) string {
	if i := strings.IndexByte(doc, '\n'); i >= 0 {
		return doc[:i]
	}
	return doc
}

// lineRange formats path:start-end, path:start or path.
func lineRange(p string, from, to int) string {
	switch {
	case from > 0 && to > from:
		return fmt.Sprintf("%s:%d-%d", p, from, to)
	case from > 0:
		return fmt.Sprintf("%s:%d", p, from)
	}
	return p
}

// linesStr formats call-site lines, abbreviating after five.
func linesStr(lines []int) string {
	if len(lines) == 0 {
		return ""
	}
	parts := make([]string, 0, len(lines))
	for i, l := range lines {
		if i == 5 {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmt.Sprint(l))
	}
	return "at L" + strings.Join(parts, ",")
}

// langList renders language counts, most common first.
func langList(m map[string]int) string {
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	var parts []string
	for _, e := range list {
		parts = append(parts, fmt.Sprintf("%s:%d", e.k, e.v))
	}
	return strings.Join(parts, " ")
}

// patchText renders a file diff's hunks as unified diff text.
func patchText(f gitx.FileDiff) string {
	var sb strings.Builder
	for _, h := range f.Hunks {
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@ %s\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines, h.Header)
		for _, l := range h.Lines {
			switch l.Kind {
			case gitx.LineAdd:
				sb.WriteString("+")
			case gitx.LineDelete:
				sb.WriteString("-")
			default:
				sb.WriteString(" ")
			}
			sb.WriteString(l.Content + "\n")
		}
	}
	return sb.String()
}
