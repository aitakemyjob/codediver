package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setup(t *testing.T) *Workspace {
	t.Helper()
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n\nfunc main() {}\n")
	write("pkg/util/util.go", "package util\n\n// Hello greets.\nfunc Hello() string { return \"hi\" }\n")
	write(".env", "SECRET=1\n")
	write("node_modules/x/index.js", "module.exports = 1\n")
	write("secrets/key.txt", "nope\n")
	return New(root, []string{"secrets/"})
}

var defaults = []string{".env", "*.pem"}

func TestResolveRejectsEscapes(t *testing.T) {
	w := setup(t)
	cases := []struct {
		in   string
		want error
	}{
		{"../etc/passwd", ErrOutside},
		{"pkg/../../x", ErrOutside},
		{".env", ErrExcluded},
		{"secrets/key.txt", ErrExcluded},
		{".git/config", ErrExcluded},
	}
	for _, c := range cases {
		if _, _, err := w.Resolve(c.in, defaults); !errors.Is(err, c.want) {
			t.Errorf("Resolve(%q) err = %v, want %v", c.in, err, c.want)
		}
	}
	// On Unix an absolute path outside the root is refused; on Windows a
	// leading slash is not absolute and is treated as repo-relative.
	if runtime.GOOS != "windows" {
		if _, _, err := w.Resolve("/etc/passwd", defaults); !errors.Is(err, ErrOutside) {
			t.Errorf("absolute outside path err = %v", err)
		}
	}
	abs, rel, err := w.Resolve("pkg/util/util.go", defaults)
	if err != nil || rel != "pkg/util/util.go" || abs != filepath.Join(w.Root, "pkg", "util", "util.go") {
		t.Fatalf("Resolve ok path = %q %q %v", abs, rel, err)
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	w := setup(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(w.Root, "link")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	if _, _, err := w.Resolve("link", defaults); !errors.Is(err, ErrOutside) {
		t.Fatalf("symlink escape err = %v", err)
	}
}

func TestFilesSkipsIgnoredAndExcluded(t *testing.T) {
	w := setup(t)
	files, err := w.Files(defaults)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"main.go": true, "pkg/util/util.go": true}
	if len(files) != len(want) {
		t.Fatalf("files = %v", files)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q", f)
		}
	}
}

func TestTreeAggregates(t *testing.T) {
	w := setup(t)
	tree, err := w.Tree(defaults, true)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Files != 2 || tree.Lines != 7 {
		t.Fatalf("tree files=%d lines=%d", tree.Files, tree.Lines)
	}
	if !tree.Children[0].Dir || tree.Children[0].Name != "pkg" {
		t.Fatalf("dirs should sort first: %+v", tree.Children[0])
	}
}

func TestSearch(t *testing.T) {
	w := setup(t)
	m, truncated, err := w.Search(SearchOptions{Query: "func hello"}, defaults)
	if err != nil || truncated || len(m) != 1 || m[0].Path != "pkg/util/util.go" || m[0].Line != 4 {
		t.Fatalf("search = %+v %v %v", m, truncated, err)
	}
	m, _, _ = w.Search(SearchOptions{Query: "func hello", CaseSensitive: true}, defaults)
	if len(m) != 0 {
		t.Fatalf("case-sensitive search matched %+v", m)
	}
	m, _, _ = w.Search(SearchOptions{Query: `func \w+\(\)`, Regex: true, Glob: "*.go"}, defaults)
	if len(m) != 2 {
		t.Fatalf("regex search = %+v", m)
	}
}

func TestReadWrite(t *testing.T) {
	w := setup(t)
	if err := w.WriteFile("new/dir/a.txt", []byte("x\ny\n"), defaults); err != nil {
		t.Fatal(err)
	}
	fc, err := w.ReadFile("new/dir/a.txt", defaults)
	if err != nil || fc.Content != "x\ny\n" || fc.Language != "plaintext" {
		t.Fatalf("read = %+v %v", fc, err)
	}
	if err := w.WriteFile("../evil", []byte("x"), defaults); !errors.Is(err, ErrOutside) {
		t.Fatalf("write outside err = %v", err)
	}
}

func TestLanguage(t *testing.T) {
	for in, want := range map[string]string{"a/b.go": "go", "x.TSX": "typescript", "Dockerfile": "dockerfile", "README": "plaintext"} {
		if got := Language(in); got != want {
			t.Errorf("Language(%q) = %q, want %q", in, got, want)
		}
	}
}
