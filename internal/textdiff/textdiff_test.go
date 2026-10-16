package textdiff

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/aitakemyjob/codediver/internal/gitx"
)

// apply reconstructs the new text from the old text and hunks.
func apply(oldText string, r Result) string {
	a := SplitLines(oldText)
	var out []string
	pos := 0
	for _, h := range r.Hunks {
		start := h.OldStart - 1
		if h.OldLines == 0 {
			start = h.OldStart
		}
		out = append(out, a[pos:start]...)
		pos = start
		for _, l := range h.Lines {
			switch l.Kind {
			case gitx.LineContext:
				out = append(out, l.Content)
				pos++
			case gitx.LineDelete:
				pos++
			case gitx.LineAdd:
				out = append(out, l.Content)
			}
		}
	}
	out = append(out, a[pos:]...)
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

func TestLinesBasic(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n"
	neu := "a\nB\nc\nd\ne\nf\ng\nh\ni\nj\nk\n"
	r := Lines(old, neu, 1)
	if r.Additions != 2 || r.Deletions != 1 || len(r.Hunks) != 2 {
		t.Fatalf("result = %+v", r)
	}
	if h := r.Hunks[0]; h.OldStart != 1 || h.OldLines != 3 || h.NewStart != 1 || h.NewLines != 3 {
		t.Errorf("hunk0 = %+v", h)
	}
	if got := apply(old, r); got != neu {
		t.Errorf("apply = %q", got)
	}
	if !strings.Contains(Unified("x.txt", r), "-b\n+B\n") {
		t.Error("unified output missing change")
	}
}

func TestLinesEdges(t *testing.T) {
	for _, c := range [][2]string{{"", "x\ny\n"}, {"x\ny\n", ""}, {"same\n", "same\n"}, {"a\n", "b\n"}} {
		r := Lines(c[0], c[1], 3)
		if got := apply(c[0], r); got != c[1] {
			t.Errorf("apply(%q→%q) = %q (%+v)", c[0], c[1], got, r)
		}
	}
}

func TestLinesRandomRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	words := []string{"a", "b", "c", "d", "e"}
	gen := func() string {
		n := rng.Intn(30)
		var sb strings.Builder
		for i := 0; i < n; i++ {
			sb.WriteString(words[rng.Intn(len(words))])
			sb.WriteString("\n")
		}
		return sb.String()
	}
	for i := 0; i < 500; i++ {
		a, b := gen(), gen()
		r := Lines(a, b, rng.Intn(4))
		if got := apply(a, r); got != b {
			t.Fatalf("round trip %d failed\nold=%q\nnew=%q\ngot=%q", i, a, b, got)
		}
	}
}
