package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aitakemyjob/codediver/internal/config"
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

// realDefaults are the excludes every repo gets.
var realDefaults = config.DefaultExcludes

// On macOS and Windows these names open the protected file itself, so the
// sandbox must refuse them on every OS.
func TestResolveRejectsCaseAndWindowsVariants(t *testing.T) {
	w := setup(t)
	for _, p := range []string{
		".ENV", ".Env.Local", "certs/Server.PEM", "deploy/prod.KEY", "ID_RSA.pub",
		".GIT/config", "pkg/.Git/HEAD",
		".git./config", ".env.", ".env ", "key.pem.", // Windows drops trailing dots and spaces
		".env::$DATA", "certs/a.pem:stream", ".git:x/config", // alternate data streams
		".env/production", "deploy/.ENV/keys", "old.pem/cert", // inside a directory named like a secret
	} {
		if _, _, err := w.Resolve(p, realDefaults); !errors.Is(err, ErrExcluded) {
			t.Errorf("Resolve(%q) err = %v, want ErrExcluded", p, err)
		}
	}
	// Ordinary files that only look similar stay readable.
	for _, p := range []string{"pkg/env.go", "docs/pem-files.md", "gitignore.go"} {
		if _, _, err := w.Resolve(p, realDefaults); err != nil {
			t.Errorf("Resolve(%q) err = %v", p, err)
		}
	}
	// User excludes match case-insensitively too.
	w.Exclude = []string{"Secrets/"}
	if _, _, err := w.Resolve("SECRETS/token.txt", nil); !errors.Is(err, ErrExcluded) {
		t.Errorf("user exclude: err = %v", err)
	}
}

// FuzzResolve checks the sandbox's guarantees for arbitrary input: no
// panics, only sandbox errors, and every accepted path stays inside the
// root and never names .git or a default-excluded secret under any
// spelling a case-insensitive or Windows file system would accept.
func FuzzResolve(f *testing.F) {
	root, err := filepath.EvalSymlinks(f.TempDir()) // macOS: /var is a symlink
	if err != nil {
		f.Fatal(err)
	}
	for name, body := range map[string]string{"pkg/a.go": "package pkg", ".env": "SECRET=1", ".git/config": "[core]", "certs/a.pem": "key"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			f.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			f.Fatal(err)
		}
	}
	outside := f.TempDir()
	_ = os.Symlink(outside, filepath.Join(root, "out"))
	w := New(root, []string{"private/"})
	for _, seed := range []string{
		"", ".", "/", "pkg/a.go", "../x", "pkg/../../x", "/etc/passwd", "out/x", ".env", ".ENV", ".git/config",
		".GIT/config", ".git./config", ".env::$DATA", "certs/A.PEM", "Private/x", "pkg//a.go", "./pkg/a.go",
		"..\\x", "pkg\\..\\..\\x", "a\x00b", " .env", "pkg/./../.env",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, p string) {
		abs, rel, err := w.Resolve(p, realDefaults)
		if err != nil {
			if !errors.Is(err, ErrOutside) && !errors.Is(err, ErrExcluded) {
				t.Fatalf("Resolve(%q): unexpected error %v", p, err)
			}
			return
		}
		if !within(root, abs) {
			t.Fatalf("Resolve(%q) = %q, outside the root", p, abs)
		}
		if want := filepath.Join(root, filepath.FromSlash(rel)); rel != "" && want != abs {
			t.Fatalf("Resolve(%q): rel %q doesn't match abs %q", p, rel, abs)
		}
		if real, err := filepath.EvalSymlinks(abs); err == nil && !within(root, real) {
			t.Fatalf("Resolve(%q) = %q, which leads outside to %q", p, abs, real)
		}
		for i, seg := range strings.Split(rel, "/") {
			if seg == ".." {
				t.Fatalf("Resolve(%q): rel %q climbs out", p, rel)
			}
			if j := strings.IndexByte(seg, ':'); j >= 0 {
				seg = seg[:j]
			}
			seg = strings.TrimRight(seg, ". ")
			// "private/" is anchored at the root; the defaults match any segment.
			if strings.EqualFold(seg, ".git") || strings.EqualFold(seg, ".env") || (i == 0 && strings.EqualFold(seg, "private")) ||
				(len(seg) > 4 && strings.EqualFold(seg[len(seg)-4:], ".pem")) {
				t.Fatalf("Resolve(%q) let protected %q through", p, rel)
			}
		}
	})
}
