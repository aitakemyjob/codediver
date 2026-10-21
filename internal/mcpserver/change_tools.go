package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/qa"
)

type changeTestsArgs struct {
	Repo     string `json:"repo,omitempty" jsonschema:"repository name or id; defaults to the repo open in CodeDiver"`
	Proposal string `json:"proposal,omitempty" jsonschema:"a proposal id from propose_changes; otherwise a git change (working tree vs HEAD by default)"`
	Commit   string `json:"commit,omitempty" jsonschema:"what this one commit changed"`
	Base     string `json:"base,omitempty" jsonschema:"base ref (commit, branch, tag). Empty = HEAD"`
	Head     string `json:"head,omitempty" jsonschema:"head ref. Empty = working tree"`
	Staged   bool   `json:"staged,omitempty" jsonschema:"the staged changes"`
	Show     bool   `json:"show,omitempty" jsonschema:"open the change's Tests panel in CodeDiver"`
}

// registerChange adds get_tests_for_change.
func (s *Server) registerChange() {
	c := s.core
	addTool(s, &mcp.Tool{Name: "get_tests_for_change", Description: "Which tests cover a change — a proposal, the working tree, staged changes, a branch or a commit: how many changed lines tests run, which changed lines no test runs, the tests to run to verify it, and changed functions no test reaches. Results say whether they are measured (the developer ran per-test coverage) or estimated (static test-to-code mapping). Also lists the blast radius: direct callers and their test status. Nothing is run.", Annotations: readOnly("Tests for change")},
		func(a changeTestsArgs) string {
			if a.Proposal != "" {
				return a.Proposal
			}
			return diffLabel(a.Commit, a.Base, a.Head, a.Staged)
		}, func(ctx context.Context, a changeTestsArgs) (string, error) {
			rt, err := c.AgentRepo(a.Repo)
			if err != nil {
				return "", err
			}
			ref := core.ChangeRef{Proposal: a.Proposal}
			if a.Proposal == "" {
				ref.Spec = &gitx.DiffSpec{Commit: a.Commit, Base: a.Base, Head: a.Head, Staged: a.Staged}
			}
			r, err := c.ChangeReport(ctx, rt, ref)
			if err != nil {
				return "", err
			}
			if a.Show {
				if a.Proposal != "" {
					c.Show(core.Focus{View: "proposal", Repo: rt.ID(), Params: map[string]any{"id": a.Proposal, "pane": "tests"}})
				} else {
					c.Show(core.Focus{View: "diff", Repo: rt.ID(), Params: map[string]any{"commit": a.Commit, "base": a.Base, "head": a.Head, "staged": a.Staged, "pane": "tests"}})
				}
			}
			var sb strings.Builder
			writeChangeCoverage(&sb, &r.Coverage, true)
			if r.Impact != nil {
				sb.WriteString("\n")
				writeImpact(&sb, r.Impact)
			}
			return sb.String(), nil
		})
}

// diffLabel names a git change for the activity feed.
func diffLabel(commit, base, head string, staged bool) string {
	switch {
	case commit != "":
		return "commit " + commit
	case staged:
		return "staged"
	case base == "" && head == "":
		return "working tree"
	}
	return base + ".." + head
}

// writeChangeCoverage describes a change's coverage: the line counts, the
// tests and, when detail is set, the uncovered lines per file.
func writeChangeCoverage(sb *strings.Builder, cc *core.ChangeCoverage, detail bool) {
	switch {
	case !cc.Available:
		sb.WriteString("No coverage report yet, so changed lines can't be checked; ask the developer to click Run coverage in the QA view.\n")
	case cc.Lines == 0 && cc.Added > 0:
		fmt.Fprintf(sb, "The change only adds code: %s no test has run yet.\n", plural(cc.Added, "new line"))
	case cc.Lines == 0:
		sb.WriteString("No executable lines changed.\n")
	default:
		fmt.Fprintf(sb, "Changed lines: %d of %d covered by tests", cc.Covered, cc.Lines)
		if cc.Uncovered > 0 {
			fmt.Fprintf(sb, ", %d not run by any test", cc.Uncovered)
		}
		if cc.Unknown > 0 {
			fmt.Fprintf(sb, ", %d in files without coverage data", cc.Unknown)
		}
		if cc.Added > 0 {
			fmt.Fprintf(sb, "; plus %s", plural(cc.Added, "new line"))
		}
		sb.WriteString(".\n")
	}
	if cc.Stale {
		sb.WriteString("Coverage is stale: files changed after the last coverage run.\n")
	}
	if cc.Approximate {
		sb.WriteString("Line numbers come from a commit and are matched against current coverage, so treat them as approximate.\n")
	}
	if detail {
		for _, f := range cc.Files {
			var miss []int
			for _, l := range f.Lines {
				if l.State == "uncovered" {
					miss = append(miss, l.Line)
				}
			}
			if len(miss) > 0 {
				fmt.Fprintf(sb, "  not covered: %s:%s\n", f.Path, qa.Ranges(miss))
			}
		}
	}
	if len(cc.Untested) > 0 {
		names := make([]string, len(cc.Untested))
		for i, n := range cc.Untested {
			names[i] = n.Name
		}
		fmt.Fprintf(sb, "Changed functions no test reaches: %s\n", strings.Join(names, ", "))
	}
	if len(cc.Tests) == 0 {
		sb.WriteString("No tests exercise the change.\n")
		return
	}
	// Measured tests either run changed lines or were shown not to; the
	// rest are estimates from the static mapping.
	var runs, misses, estimated []core.ChangeTest
	for _, t := range cc.Tests {
		switch {
		case t.Measured && t.Lines > 0:
			runs = append(runs, t)
		case t.Measured:
			misses = append(misses, t)
		default:
			estimated = append(estimated, t)
		}
	}
	limit := len(cc.Tests)
	if !detail {
		limit = 8
	}
	group := func(title, note string, ts []core.ChangeTest) {
		if len(ts) == 0 || limit <= 0 {
			if len(ts) > 0 {
				fmt.Fprintf(sb, "%s: %d more\n", title, len(ts))
			}
			return
		}
		fmt.Fprintf(sb, "%s (%d)%s:\n", title, len(ts), note)
		shown := ts[:min(len(ts), limit)]
		limit -= len(shown)
		for _, t := range shown {
			var notes []string
			if t.Measured && t.Lines > 0 {
				notes = append(notes, "runs "+plural(t.Lines, "changed line"))
			}
			if t.Direct {
				notes = append(notes, "calls the changed code")
			} else if len(t.Functions) > 0 {
				notes = append(notes, "reaches "+strings.Join(t.Functions, ", "))
			}
			if t.Failed {
				notes = append(notes, "FAILED last run")
			}
			fmt.Fprintf(sb, "- %s  %s:%d", t.Name, t.File, t.Line)
			if len(notes) > 0 {
				fmt.Fprintf(sb, " (%s)", strings.Join(notes, "; "))
			}
			sb.WriteString("\n")
		}
		if more := len(ts) - len(shown); more > 0 {
			fmt.Fprintf(sb, "  … %d more\n", more)
		}
	}
	group("Tests that run the changed lines", ", measured", runs)
	group("Tests that call or reach the changed functions", ", estimated: the developer can click Run these tests to measure", estimated)
	group("Tests that reach the changed functions but don't run these lines", ", measured", misses)
	if cc.MoreTests > 0 {
		fmt.Fprintf(sb, "  … %d more tests not listed\n", cc.MoreTests)
	}
}

// plural formats a count with a noun, adding "s" unless it is one.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// statusOrder lists test statuses weakest first, with how they read.
var statusOrder = []struct{ status, label string }{
	{core.StatusNotRun, "not run"},
	{core.StatusCaller, "run but not the call toward the change"},
	{core.StatusUnknown, "unknown"},
	{core.StatusLikely, "likely tested (estimated)"},
	{core.StatusPath, "tests run the path"},
}

// statusCounts summarizes a ring's callers by test status.
func statusCounts(counts map[string]int) string {
	var parts []string
	for _, s := range statusOrder {
		if n := counts[s.status]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s.label))
		}
	}
	return strings.Join(parts, " · ")
}

// statusText says how far tests reach a caller, and how that is known.
func statusText(n core.ImpactNode) string {
	var t string
	switch n.Status {
	case core.StatusPath:
		t = "tests run this call"
		if len(n.Tests) > 0 {
			t += " (" + strings.Join(n.Tests, ", ") + ")"
		}
	case core.StatusCaller:
		t = "runs, but no test runs this call"
	case core.StatusNotRun:
		t = "no test runs it"
	case core.StatusLikely:
		t = "likely tested: a test calls it (not measured)"
	default:
		t = "unknown: no coverage and no test calls it"
	}
	if n.Pct >= 0 {
		t += ", " + pct(n.Pct) + " covered"
	}
	if n.Stale {
		t += ", coverage older than the file"
	}
	return t
}

// writeImpact describes a blast radius closest first: direct callers by
// name, further rings as counts, then where the change surfaces.
func writeImpact(sb *strings.Builder, im *core.Impact) {
	if len(im.Rings) == 0 {
		sb.WriteString("Blast radius: no existing callers.\n")
		return
	}
	sum := im.Summary
	fmt.Fprintf(sb, "Blast radius: %s in %s (direct %d · 2 calls away %d · 3 calls away %d)", plural(sum.Total(), "caller"), plural(sum.Files, "file"), sum.Callers[0], sum.Callers[1], sum.Callers[2])
	if im.Beyond > 0 {
		fmt.Fprintf(sb, ", %s continue further", plural(im.Beyond, "path"))
	}
	sb.WriteString(".\n")
	for _, r := range im.Rings {
		if r.Depth != 1 {
			continue
		}
		fmt.Fprintf(sb, "Direct callers (most likely to break)")
		if im.Tests || im.Coverage {
			fmt.Fprintf(sb, "; %s", statusCounts(r.Statuses))
		}
		sb.WriteString(":\n")
		for _, n := range r.Nodes[:min(len(r.Nodes), 25)] {
			fmt.Fprintf(sb, "- %s  %s:%d → %s", n.Name, n.File, n.Line, n.Via)
			if im.Tests || im.Coverage {
				fmt.Fprintf(sb, "  %s", statusText(n))
			}
			if n.Review {
				sb.WriteString("  REVIEW")
			}
			sb.WriteString("\n")
		}
		if more := r.Total - min(len(r.Nodes), 25); more > 0 {
			fmt.Fprintf(sb, "  … %d more\n", more)
		}
	}
	for _, r := range im.Rings {
		if r.Depth == 1 {
			continue
		}
		var pk []string
		for _, p := range r.Packages[:min(len(r.Packages), 6)] {
			pk = append(pk, fmt.Sprintf("%s %d", p.Path, p.Count))
		}
		fmt.Fprintf(sb, "%d calls away: %s (%s)", r.Depth, plural(r.Total, "caller"), strings.Join(pk, ", "))
		if im.Tests || im.Coverage {
			fmt.Fprintf(sb, "; %s", statusCounts(r.Statuses))
		}
		sb.WriteString("\n")
	}
	if len(im.Entries) > 0 {
		var es []string
		for _, e := range im.Entries[:min(len(im.Entries), 10)] {
			es = append(es, e.Name)
		}
		fmt.Fprintf(sb, "Surfaces at: %s\n", strings.Join(es, ", "))
	}
}
