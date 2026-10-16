package mcpserver

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/qa"
)

type qaOverviewArgs struct {
	Repo string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Show bool   `json:"show,omitempty" jsonschema:"also open the QA view for the developer"`
}

type coverageArgs struct {
	Repo string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Path string `json:"path,omitempty" jsonschema:"file or directory (repo-relative); empty for the whole repo"`
	Show bool   `json:"show,omitempty" jsonschema:"open the file in CodeDiver with covered/uncovered lines marked in the gutter"`
}

type testsForArgs struct {
	Repo   string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Symbol string `json:"symbol,omitempty" jsonschema:"function or method whose tests to find"`
	Path   string `json:"path,omitempty" jsonschema:"a test file: list the behaviors its tests describe and what they exercise"`
}

type gapsArgs struct {
	Repo  string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Path  string `json:"path,omitempty" jsonschema:"limit to a file or directory"`
	Limit int    `json:"limit,omitempty" jsonschema:"max functions (default 15)"`
	Show  bool   `json:"show,omitempty" jsonschema:"open the QA view's gap list for the developer"`
}

type testPlanArgs struct {
	Repo      string                 `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Title     string                 `json:"title" jsonschema:"short title, e.g. 'Cover CreateOrder error paths'"`
	Summary   string                 `json:"summary,omitempty" jsonschema:"markdown: what is untested today, why it matters and the approach"`
	Symbol    string                 `json:"symbol,omitempty" jsonschema:"the function the plan covers"`
	TestFile  string                 `json:"test_file,omitempty" jsonschema:"where the new tests should live (repo-relative)"`
	Behaviors []core.PlannedBehavior `json:"behaviors" jsonschema:"behaviors to test, each phrased like a test name"`
	Refactors []core.TestRefactor    `json:"refactors,omitempty" jsonschema:"code changes needed first to make the function testable (inject a clock, wrap I/O behind an interface...)"`
}

// pct formats a coverage percentage, or "n/a" when unknown.
func pct(p float64) string {
	if p < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", p)
}

// coverageNote explains where coverage comes from and how fresh it is.
func coverageNote(cs *core.CoverageSummary) string {
	if cs == nil {
		return "No coverage report yet. Test reach below is static (from call graphs). Ask the developer to click Run coverage in CodeDiver's QA view (agents can't run tests here)."
	}
	var srcs []string
	for _, s := range cs.Sources {
		name := s.Suite
		if name == "" {
			name = path.Base(s.Path)
		}
		srcs = append(srcs, fmt.Sprintf("%s (%s)", name, s.Format))
	}
	note := fmt.Sprintf("line coverage %s (%d/%d lines) from %s, generated %s ago", pct(cs.Pct), cs.Covered, cs.Lines, strings.Join(srcs, ", "), time.Since(cs.Generated).Round(time.Minute))
	if cs.Stale > 0 {
		note += fmt.Sprintf("; %d file(s) changed since — ask the developer to re-run coverage for exact numbers", cs.Stale)
	}
	return note
}

// gapLine renders one under-tested function.
func gapLine(sb *strings.Builder, i int, g *core.FuncQA) {
	fmt.Fprintf(sb, "%d. %s  %s:%d-%d  coverage %s  risk %.1f\n", i+1, g.Name, g.File, g.Line, g.EndLine, pct(g.Pct), g.Risk)
	if g.Reason != "" {
		fmt.Fprintf(sb, "   why: %s\n", g.Reason)
	}
	if g.Uncovered != "" {
		fmt.Fprintf(sb, "   uncovered lines: %s\n", g.Uncovered)
	}
	if g.Direct > 0 || g.Indirect > 0 {
		fmt.Fprintf(sb, "   tests: %d direct, %d indirect\n", g.Direct, g.Indirect)
	}
	for _, h := range g.Hints {
		at := ""
		if h.Line > 0 {
			at = fmt.Sprintf(" (L%d %s)", h.Line, h.Match)
		}
		fmt.Fprintf(sb, "   testability: %s%s → %s\n", h.Problem, at, h.Suggestion)
	}
}

// testLine renders a test case as "- Behavior [kind] file:line".
func testLine(tc qa.TestCase) string {
	extra := ""
	if tc.Kind != qa.KindTest {
		extra = " [" + tc.Kind + "]"
	}
	if tc.Skipped {
		extra += " [skipped]"
	}
	suite := ""
	if len(tc.Suite) > 0 {
		suite = strings.Join(tc.Suite, " › ") + " › "
	}
	return fmt.Sprintf("- %s%s%s  %s:%d", suite, tc.Behavior, extra, tc.File, tc.Line)
}

// registerQA adds the testing tools. They read coverage and tests; none of
// them runs anything.
func (s *Server) registerQA() {
	c := s.core

	addTool(s, &mcp.Tool{Name: "get_test_overview", Description: "Testing health of a repository: line coverage (from the developer's last coverage run or an existing report), test counts by framework, the behaviors tests describe, coverage by directory and the riskiest untested functions. Start here for 'what is and isn't tested' questions.", Annotations: readOnly("Test overview")},
		func(qaOverviewArgs) string { return "" }, func(ctx context.Context, a qaOverviewArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			ov, err := c.QAOverview(ctx, rt, 10)
			if err != nil {
				return "", err
			}
			if a.Show {
				c.Show(core.Focus{View: "qa", Repo: rt.ID(), Title: "QA", Params: map[string]any{"tab": "overview"}})
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "%s\n", coverageNote(ov.Coverage))
			var fw []string
			for k, v := range ov.Tests.Frameworks {
				fw = append(fw, fmt.Sprintf("%s:%d", k, v))
			}
			sort.Strings(fw)
			fmt.Fprintf(&sb, "tests: %d tests, %d behaviors (incl. subtests/cases), %d suites, %d skipped, in %d files [%s]\n",
				ov.Tests.Tests, ov.Tests.Behaviors, ov.Tests.Suites, ov.Tests.Skipped, ov.Tests.Files, strings.Join(fw, " "))
			fn := ov.Functions
			fmt.Fprintf(&sb, "functions: %d total, %d called directly by tests, %d reached by tests at all", fn.Total, fn.Tested, fn.Reached)
			if ov.Coverage != nil {
				fmt.Fprintf(&sb, ", %d with any coverage, %d fully covered, %d with zero coverage", fn.Covered, fn.Full, fn.Uncovered)
			}
			sb.WriteString("\n")
			if ov.Coverage != nil {
				type dir struct {
					p          string
					cov, lines int
				}
				dirs := map[string]*dir{}
				for _, f := range ov.Files {
					if f.Lines == 0 {
						continue
					}
					d := path.Dir(f.Path)
					if dirs[d] == nil {
						dirs[d] = &dir{p: d}
					}
					dirs[d].cov += f.Covered
					dirs[d].lines += f.Lines
				}
				var list []*dir
				for _, d := range dirs {
					list = append(list, d)
				}
				sort.Slice(list, func(i, j int) bool {
					return float64(list[i].cov)/float64(list[i].lines) < float64(list[j].cov)/float64(list[j].lines)
				})
				sb.WriteString("\nleast covered directories:\n")
				for i, d := range list {
					if i == 12 {
						break
					}
					name := d.p
					if name == "." {
						name = "(root)"
					}
					fmt.Fprintf(&sb, "- %s %.0f%% (%d/%d lines)\n", name, 100*float64(d.cov)/float64(d.lines), d.cov, d.lines)
				}
			}
			sb.WriteString("\nriskiest under-tested functions (use find_test_gaps for details):\n")
			for i, g := range ov.Gaps {
				fmt.Fprintf(&sb, "%d. %s  %s:%d  %s\n", i+1, g.Name, g.File, g.Line, g.Reason)
			}
			if len(ov.Gaps) == 0 {
				sb.WriteString("(none found)\n")
			}
			sb.WriteString("\nNext: get_tests_for to see what a function's tests cover, get_coverage for exact uncovered lines, show_test_plan to put a plan on screen, then propose_changes with the new tests.")
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "get_coverage", Description: "Line coverage for a file (covered %, uncovered line ranges, partially covered lines, and coverage per function) or for a directory / the whole repo (per-file table, worst first). show=true opens the file with coverage marked in the gutter.", Annotations: readOnly("Coverage")},
		func(a coverageArgs) string { return a.Path }, func(ctx context.Context, a coverageArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			ov, err := c.QAOverview(ctx, rt, 0)
			if err != nil {
				return "", err
			}
			if ov.Coverage == nil {
				return coverageNote(nil), nil
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "%s\n\n", coverageNote(ov.Coverage))
			if a.Path != "" {
				if _, rel, err := rt.WS().Resolve(a.Path, nil); err == nil {
					if fv, err := c.FileCoverage(ctx, rt, rel); err == nil && (fv.Available || len(fv.Functions) > 0) {
						if a.Show {
							c.TouchFile(rt, rel)
							c.Show(core.Focus{View: "code", Repo: rt.ID(), Params: map[string]any{"file": rel, "coverage": true}})
						}
						if !fv.Available {
							fmt.Fprintf(&sb, "%s is not in the coverage report (no test loaded it, or it isn't instrumented).\n", rel)
						} else {
							fmt.Fprintf(&sb, "%s: %s covered\nuncovered lines: %s\n", rel, pct(fv.Pct), orNone(qa.Ranges(fv.Uncovered)))
							if len(fv.Partial) > 0 {
								fmt.Fprintf(&sb, "partially covered (some branches never ran): %s\n", qa.Ranges(fv.Partial))
							}
							if fv.Stale {
								sb.WriteString("note: the file changed after this report; line numbers may be off.\n")
							}
						}
						sb.WriteString("\nfunctions:\n")
						for _, f := range fv.Functions {
							fmt.Fprintf(&sb, "- %s L%d-%d  %s  tests: %d direct, %d indirect\n", f.Name, f.Line, f.EndLine, pct(f.Pct), f.Direct, f.Indirect)
						}
						return sb.String(), nil
					}
					a.Path = rel
				}
			}
			files := ov.Files
			sort.Slice(files, func(i, j int) bool { return files[i].Pct < files[j].Pct })
			n := 0
			for _, f := range files {
				if f.Lines == 0 || (a.Path != "" && !strings.HasPrefix(f.Path, strings.TrimSuffix(a.Path, "/")+"/")) {
					continue
				}
				if n == 60 {
					sb.WriteString("… more files omitted; narrow with path\n")
					break
				}
				n++
				stale := ""
				if f.Stale {
					stale = " (changed since report)"
				}
				fmt.Fprintf(&sb, "- %s %s (%d/%d lines), %d untested function(s)%s\n", f.Path, pct(f.Pct), f.Covered, f.Lines, f.Untested, stale)
			}
			if n == 0 {
				sb.WriteString("no covered files under that path\n")
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "get_tests_for", Description: "Which tests exercise a function — directly (the test calls it) or indirectly (through its callers) — with the behaviors they describe; or, given a test file, every behavior its tests describe and the code each test calls. Use before writing tests to avoid duplicating existing ones.", Annotations: readOnly("Tests for")},
		func(a testsForArgs) string { return a.Symbol + a.Path }, func(ctx context.Context, a testsForArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			if a.Symbol == "" {
				if a.Path == "" {
					return "", fmt.Errorf("pass symbol or path")
				}
				fv, err := c.FileCoverage(ctx, rt, a.Path)
				if err != nil {
					return "", err
				}
				if len(fv.Tests) == 0 {
					return fmt.Sprintf("no tests found in %s", fv.Path), nil
				}
				ov, err := c.QAOverview(ctx, rt, 0)
				if err != nil {
					return "", err
				}
				fmt.Fprintf(&sb, "%d test cases in %s:\n", len(fv.Tests), fv.Path)
				for _, tc := range fv.Tests {
					sb.WriteString(testLine(tc))
					var names []string
					for _, id := range tc.Targets {
						if n, ok := ov.Targets[id]; ok {
							names = append(names, n.Name)
						}
					}
					if len(names) > 0 {
						fmt.Fprintf(&sb, "  → calls %s", strings.Join(names, ", "))
					}
					sb.WriteString("\n")
				}
				return sb.String(), nil
			}
			st, err := c.TestsFor(ctx, rt, a.Symbol)
			if err != nil {
				return "", err
			}
			f := st.Function
			fmt.Fprintf(&sb, "%s %s:%d-%d  coverage %s\n", f.Name, f.File, f.Line, f.EndLine, pct(f.Pct))
			if f.Uncovered == "" {
				if fv, err := c.FileCoverage(ctx, rt, f.File); err == nil && fv.Available {
					var miss []int
					for _, ln := range fv.Uncovered {
						if ln >= f.Line && ln <= f.EndLine {
							miss = append(miss, ln)
						}
					}
					if len(miss) > 0 {
						fmt.Fprintf(&sb, "uncovered lines: %s\n", qa.Ranges(miss))
					}
				}
			}
			fmt.Fprintf(&sb, "\ndirect tests (%d):\n", len(st.Direct))
			for _, tc := range st.Direct {
				sb.WriteString(testLine(tc) + "\n")
			}
			fmt.Fprintf(&sb, "\nindirect tests via callers (%d):\n", len(st.Indirect))
			for i, tc := range st.Indirect {
				if i == 25 {
					fmt.Fprintf(&sb, "… %d more\n", len(st.Indirect)-25)
					break
				}
				sb.WriteString(testLine(tc) + "\n")
			}
			if len(st.Direct) == 0 && len(st.Indirect) == 0 {
				sb.WriteString("No test reaches this function. Read it, then show_test_plan with the behaviors worth covering.\n")
			}
			sb.WriteString("\n(test→code mapping is static: calls in each test body, resolved by name/package; heuristic for non-Go code)")
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "find_test_gaps", Description: "Rank functions that most need tests: low or zero coverage weighted by complexity, number of callers and visibility. Each gap lists uncovered line ranges and testability problems found in the code (hard-wired clock, network, filesystem, env vars, processes, sleeps, os.Exit...) with the refactor that would make it testable.", Annotations: readOnly("Find test gaps")},
		func(a gapsArgs) string { return a.Path }, func(ctx context.Context, a gapsArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			if a.Limit <= 0 {
				a.Limit = 15
			}
			gaps, haveCov, err := c.TestGaps(ctx, rt, a.Path, min(a.Limit, 60))
			if err != nil {
				return "", err
			}
			if a.Show {
				c.Show(core.Focus{View: "qa", Repo: rt.ID(), Title: "QA", Params: map[string]any{"tab": "gaps"}})
			}
			var sb strings.Builder
			if !haveCov {
				sb.WriteString("(no coverage report: ranking uses static test reach only; ask the developer to click Run coverage in the QA view)\n\n")
			}
			for i, g := range gaps {
				gapLine(&sb, i, g)
			}
			if len(gaps) == 0 {
				sb.WriteString("No gaps found.")
			} else {
				sb.WriteString("\nTo close a gap: get_tests_for the function, read it, show_test_plan with behaviors (and refactors if hints apply), then propose_changes with the tests.")
			}
			return sb.String(), nil
		})

	addTool(s, &mcp.Tool{Name: "show_test_plan", Description: "Put a test plan on the developer's screen in the QA view: the behaviors worth testing for a function (happy path, edge cases, errors — each as given/when/then, optionally tied to the uncovered line it exercises) and any refactors needed first to make the code testable. Follow up with propose_changes containing the actual tests once the developer agrees.", Annotations: visual("Show test plan")},
		func(a testPlanArgs) string { return fmt.Sprintf("%s (%d behaviors)", a.Title, len(a.Behaviors)) }, func(ctx context.Context, a testPlanArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			p, err := c.ShowTestPlan(ctx, rt, core.TestPlan{Title: a.Title, Summary: a.Summary, Symbol: a.Symbol, TestFile: a.TestFile, Behaviors: a.Behaviors, Refactors: a.Refactors})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("test plan %s with %d behaviors and %d refactors is on the developer's screen. Ask whether to proceed, then use propose_changes to add the tests.", p.ID, len(p.Behaviors), len(p.Refactors)), nil
		})
}
