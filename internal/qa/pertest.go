package qa

import (
	"encoding/json"
	"fmt"
	"path"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// ---- per-test coverage -----------------------------------------------------------
//
// Merged suite coverage says whether a line ran, not which test ran it. To
// attribute lines to tests CodeDiver runs the tests a change touches on
// their own, when the developer asks: Go tests one at a time with their
// own coverprofile, and pytest once with coverage.py's per-test contexts.

// PerTestFrameworks are the frameworks whose tests can be run for
// per-test coverage.
var PerTestFrameworks = map[string]bool{"go test": true, "pytest": true}

// GoTestCommand runs one top-level Go test in pkg (a directory relative
// to the module root) with coverage of the whole module.
func GoTestCommand(name, pkg string) string {
	if pkg == "" || pkg == "." {
		pkg = "."
	} else {
		pkg = "./" + strings.TrimPrefix(pkg, "./")
	}
	return fmt.Sprintf(`go test -count=1 -run "^%s$" -coverpkg=./... -coverprofile={out}/cover.out %s`, name, shellQuote(pkg))
}

// PytestNodeID is pytest's id for a test: file::Class::name, with file
// relative to the directory pytest runs in.
func PytestNodeID(file string, classes []string, name string) string {
	return strings.Join(append(append([]string{file}, classes...), name), "::")
}

// PytestCommand runs the given pytest node ids recording which test ran
// each line, then writes coverage.py's JSON report with those contexts.
// The report is written even when tests fail.
func PytestCommand(ids []string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = shellQuote(id)
	}
	sep := "; "
	if runtime.GOOS == "windows" {
		sep = " & "
	}
	return "python -m pytest -q --cov --cov-context=test --cov-report= " + strings.Join(quoted, " ") +
		sep + "python -m coverage json --show-contexts -o {out}/contexts.json"
}

// ParseContexts reads a coverage.py JSON report written with
// --show-contexts and returns, per pytest node id, the lines each file
// ran (paths as the report wrote them).
func ParseContexts(data []byte) (map[string]map[string][]int, error) {
	var rep struct {
		Files map[string]struct {
			Contexts map[string][]string `json:"contexts"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("not a coverage.py JSON report: %w", err)
	}
	out := map[string]map[string][]int{}
	for file, f := range rep.Files {
		for ln, ctxs := range f.Contexts {
			n, err := strconv.Atoi(ln)
			if err != nil {
				continue
			}
			for _, c := range ctxs {
				id := NodeIDOf(c)
				if id == "" {
					continue
				}
				if out[id] == nil {
					out[id] = map[string][]int{}
				}
				out[id][file] = append(out[id][file], n)
			}
		}
	}
	// A line can run in a test's setup and its body: count it once.
	for _, files := range out {
		for f, ls := range files {
			sort.Ints(ls)
			files[f] = slices.Compact(ls)
		}
	}
	return out, nil
}

// NodeIDOf strips the phase pytest-cov appends to a context
// ("tests/test_a.py::test_x|run") and any parametrization, or returns ""
// for code that ran outside a test.
func NodeIDOf(context string) string {
	id, _, _ := strings.Cut(context, "|")
	if i := strings.IndexByte(id, '['); i >= 0 {
		id = id[:i]
	}
	if !strings.Contains(id, "::") {
		return ""
	}
	return id
}

// RelTo makes a repo-relative path relative to dir ("" for the root).
func RelTo(dir, p string) string {
	if dir == "" || dir == "." {
		return p
	}
	if rel, ok := strings.CutPrefix(p, dir+"/"); ok {
		return rel
	}
	return path.Clean(p)
}
