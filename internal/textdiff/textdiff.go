// Package textdiff computes line diffs (Myers' O(ND) algorithm) and groups
// them into unified-diff hunks compatible with gitx.
package textdiff

import (
	"fmt"
	"strings"

	"github.com/aitakemyjob/codediver/internal/gitx"
)

type opKind int

const (
	opEqual opKind = iota
	opInsert
	opDelete
)

type op struct {
	kind opKind
	a, b int // index into a (old) / b (new)
}

// SplitLines splits text into lines without trailing newlines.
func SplitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// diff returns an edit script turning a into b.
func diff(a, b []string) []op {
	n, m := len(a), len(b)
	// Trim common prefix/suffix to keep the core problem small.
	pre := 0
	for pre < n && pre < m && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && a[n-1-suf] == b[m-1-suf] {
		suf++
	}
	var ops []op
	for i := 0; i < pre; i++ {
		ops = append(ops, op{opEqual, i, i})
	}
	ops = append(ops, myers(a[pre:n-suf], b[pre:m-suf], pre, pre)...)
	for i := 0; i < suf; i++ {
		ops = append(ops, op{opEqual, n - suf + i, m - suf + i})
	}
	return ops
}

// myers computes the shortest edit script between a and b with Myers'
// O(ND) algorithm. offA/offB shift the indices it reports.
func myers(a, b []string, offA, offB int) []op {
	n, m := len(a), len(b)
	if n == 0 {
		out := make([]op, m)
		for j := range out {
			out[j] = op{opInsert, offA, offB + j}
		}
		return out
	}
	if m == 0 {
		out := make([]op, n)
		for i := range out {
			out[i] = op{opDelete, offA + i, offB}
		}
		return out
	}
	maxD := n + m
	v := make([]int, 2*maxD+2)
	var trace [][]int
	for d := 0; d <= maxD; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[maxD+k-1] < v[maxD+k+1]) {
				x = v[maxD+k+1]
			} else {
				x = v[maxD+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[maxD+k] = x
			if x >= n && y >= m {
				return backtrack(trace, a, b, maxD, d, offA, offB)
			}
		}
	}
	return nil
}

// backtrack walks the saved Myers trace backwards to recover the edit
// script, returned in forward order.
func backtrack(trace [][]int, a, b []string, maxD, d, offA, offB int) []op {
	x, y := len(a), len(b)
	var rev []op
	for ; d > 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && v[maxD+k-1] < v[maxD+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[maxD+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x--
			y--
			rev = append(rev, op{opEqual, offA + x, offB + y})
		}
		if x == prevX {
			y--
			rev = append(rev, op{opInsert, offA + x, offB + y})
		} else {
			x--
			rev = append(rev, op{opDelete, offA + x, offB + y})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		rev = append(rev, op{opEqual, offA + x, offB + y})
	}
	out := make([]op, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

// Result is a computed diff between two texts.
type Result struct {
	Hunks     []gitx.Hunk `json:"hunks"`
	Additions int         `json:"additions"`
	Deletions int         `json:"deletions"`
}

// Lines diffs two texts, producing hunks with `context` lines around changes.
func Lines(oldText, newText string, context int) Result {
	if context < 0 {
		context = 3
	}
	a, b := SplitLines(oldText), SplitLines(newText)
	ops := diff(a, b)
	var res Result
	// Find change indices, then group into hunks.
	var changeIdx []int
	for i, o := range ops {
		if o.kind != opEqual {
			changeIdx = append(changeIdx, i)
		}
	}
	if len(changeIdx) == 0 {
		return res
	}
	type span struct{ start, end int } // inclusive op indices
	var spans []span
	cur := span{max(0, changeIdx[0]-context), min(len(ops)-1, changeIdx[0]+context)}
	for _, ci := range changeIdx[1:] {
		s := max(0, ci-context)
		if s <= cur.end+1 {
			cur.end = min(len(ops)-1, ci+context)
			continue
		}
		spans = append(spans, cur)
		cur = span{s, min(len(ops)-1, ci+context)}
	}
	spans = append(spans, cur)

	for _, sp := range spans {
		h := gitx.Hunk{}
		first := ops[sp.start]
		h.OldStart, h.NewStart = first.a+1, first.b+1
		for _, o := range ops[sp.start : sp.end+1] {
			switch o.kind {
			case opEqual:
				h.OldLines++
				h.NewLines++
				h.Lines = append(h.Lines, gitx.DiffLine{Kind: gitx.LineContext, Content: a[o.a], OldNo: o.a + 1, NewNo: o.b + 1})
			case opDelete:
				h.OldLines++
				res.Deletions++
				h.Lines = append(h.Lines, gitx.DiffLine{Kind: gitx.LineDelete, Content: a[o.a], OldNo: o.a + 1})
			case opInsert:
				h.NewLines++
				res.Additions++
				h.Lines = append(h.Lines, gitx.DiffLine{Kind: gitx.LineAdd, Content: b[o.b], NewNo: o.b + 1})
			}
		}
		if h.OldLines == 0 {
			h.OldStart--
		}
		if h.NewLines == 0 {
			h.NewStart--
		}
		res.Hunks = append(res.Hunks, h)
	}
	return res
}

// Unified renders hunks as unified diff text (for agents).
func Unified(path string, r Result) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", path, path)
	for _, h := range r.Hunks {
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, l := range h.Lines {
			switch l.Kind {
			case gitx.LineAdd:
				sb.WriteString("+")
			case gitx.LineDelete:
				sb.WriteString("-")
			default:
				sb.WriteString(" ")
			}
			sb.WriteString(l.Content)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
