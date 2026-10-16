// Package workspace is the sandbox through which every file access flows.
// Paths are always repo-relative, slash-separated, and checked so that
// nothing outside the configured root (or matching an exclude) leaks out.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrOutside = errors.New("path is outside the configured repository")
var ErrExcluded = errors.New("path is excluded from CodeDiver")

// skipDirs are never walked when the repo is not a git checkout.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, ".venv": true, "venv": true, "__pycache__": true, ".idea": true,
	".vscode": true, ".next": true, ".cache": true, "bin": true, "obj": true, ".gradle": true,
}

type Workspace struct {
	Root    string
	Exclude []string

	mu       sync.Mutex
	files    []string
	filesAt  time.Time
	isGit    bool
	gitKnown bool
}

// New creates a workspace rooted at root with per-repo exclude globs.
func New(root string, exclude []string) *Workspace {
	return &Workspace{Root: filepath.Clean(root), Exclude: exclude}
}

// Excluded reports whether a repo-relative path matches an exclude glob.
func (w *Workspace) Excluded(rel string, defaults []string) bool {
	base := path.Base(rel)
	for _, list := range [][]string{defaults, w.Exclude} {
		for _, pat := range list {
			if pat == "" {
				continue
			}
			if ok, _ := path.Match(pat, base); ok {
				return true
			}
			if ok, _ := path.Match(pat, rel); ok {
				return true
			}
			// "dir/" or "dir/**" style excludes everything below a directory.
			p := strings.TrimSuffix(strings.TrimSuffix(pat, "**"), "/")
			if p != "" && !strings.ContainsAny(p, "*?[") && (rel == p || strings.HasPrefix(rel, p+"/")) {
				return true
			}
		}
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".git" {
			return true
		}
	}
	return false
}

// Resolve converts a repo-relative (or absolute-inside-root) path to an
// absolute path, refusing anything that escapes the root, including via
// symlinks.
func (w *Workspace) Resolve(p string, defaults []string) (abs, rel string, err error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "." || p == "/" {
		return w.Root, "", nil
	}
	var candidate string
	if filepath.IsAbs(p) {
		candidate = filepath.Clean(p)
	} else {
		candidate = filepath.Join(w.Root, filepath.FromSlash(strings.TrimPrefix(p, "/")))
	}
	if !within(w.Root, candidate) {
		return "", "", ErrOutside
	}
	if real, err := filepath.EvalSymlinks(candidate); err == nil {
		if !within(w.Root, real) {
			return "", "", ErrOutside
		}
	}
	r, err := filepath.Rel(w.Root, candidate)
	if err != nil {
		return "", "", ErrOutside
	}
	rel = filepath.ToSlash(r)
	if rel == "." {
		rel = ""
	}
	if rel != "" && w.Excluded(rel, defaults) {
		return "", "", ErrExcluded
	}
	return candidate, rel, nil
}

// within reports whether p is root or lies underneath it.
func within(root, p string) bool {
	if p == root {
		return true
	}
	r := root
	if !strings.HasSuffix(r, string(filepath.Separator)) {
		r += string(filepath.Separator)
	}
	return strings.HasPrefix(p, r)
}

// IsGit reports whether the root is inside a git work tree (cached).
func (w *Workspace) IsGit() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.gitKnown {
		cmd := exec.Command("git", "-C", w.Root, "rev-parse", "--is-inside-work-tree")
		out, err := cmd.Output()
		w.isGit = err == nil && strings.TrimSpace(string(out)) == "true"
		w.gitKnown = true
	}
	return w.isGit
}

// Files lists every visible file, honoring .gitignore when the repo is a
// git checkout. Results are cached for a couple of seconds.
func (w *Workspace) Files(defaults []string) ([]string, error) {
	isGit := w.IsGit()
	w.mu.Lock()
	if w.files != nil && time.Since(w.filesAt) < 3*time.Second {
		f := w.files
		w.mu.Unlock()
		return f, nil
	}
	w.mu.Unlock()

	var files []string
	var err error
	if isGit {
		files, err = w.gitFiles()
	}
	if !isGit || err != nil {
		files, err = w.walkFiles()
		if err != nil {
			return nil, err
		}
	}
	out := files[:0]
	for _, f := range files {
		if !w.Excluded(f, defaults) {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	w.mu.Lock()
	w.files, w.filesAt = out, time.Now()
	w.mu.Unlock()
	return out, nil
}

// Invalidate drops the cached file list.
func (w *Workspace) Invalidate() {
	w.mu.Lock()
	w.files = nil
	w.mu.Unlock()
}

// gitFiles lists tracked and untracked-but-not-ignored files via
// `git ls-files`, skipping entries deleted from disk.
func (w *Workspace) gitFiles() ([]string, error) {
	cmd := exec.Command("git", "-C", w.Root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	seen := map[string]bool{}
	for _, f := range bytes.Split(out, []byte{0}) {
		s := string(f)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		// ls-files lists deleted-but-tracked files too; skip those.
		if st, err := os.Lstat(filepath.Join(w.Root, filepath.FromSlash(s))); err != nil || st.IsDir() {
			continue
		}
		files = append(files, s)
	}
	return files, nil
}

// walkFiles lists files by walking the directory tree, skipping hidden and
// common build/dependency directories. Used for non-git folders.
func (w *Workspace) walkFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(w.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != w.Root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(w.Root, p)
		files = append(files, filepath.ToSlash(rel))
		if len(files) > 200000 {
			return fs.SkipAll
		}
		return nil
	})
	return files, err
}

const MaxReadBytes = 2 << 20

type FileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language"`
	Lines    int    `json:"lines"`
	Size     int64  `json:"size"`
	Binary   bool   `json:"binary"`
	Truncate bool   `json:"truncated"`
}

// ReadFile reads a file through the sandbox, truncating at MaxReadBytes and
// flagging binary content instead of returning it.
func (w *Workspace) ReadFile(rel string, defaults []string) (*FileContent, error) {
	abs, rel, err := w.Resolve(rel, defaults)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s is a directory", rel)
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, min(st.Size(), MaxReadBytes))
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	fc := &FileContent{Path: rel, Language: Language(rel), Size: st.Size(), Truncate: st.Size() > MaxReadBytes}
	if IsBinary(buf) {
		fc.Binary = true
		return fc, nil
	}
	fc.Content = string(buf)
	fc.Lines = bytes.Count(buf, []byte{'\n'}) + 1
	return fc, nil
}

// IsBinary reports whether b looks like binary data (contains a NUL byte
// in its first 8000 bytes).
func IsBinary(b []byte) bool {
	if len(b) > 8000 {
		b = b[:8000]
	}
	return bytes.IndexByte(b, 0) >= 0
}

// WriteFile writes content inside the sandbox. Only used when the user
// accepts a proposal in the UI.
func (w *Workspace) WriteFile(rel string, content []byte, defaults []string) error {
	abs, _, err := w.Resolve(rel, defaults)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(abs); err == nil {
		mode = st.Mode().Perm()
	}
	w.Invalidate()
	return os.WriteFile(abs, content, mode)
}

// DeleteFile removes a file inside the sandbox. Only used when the user
// accepts a proposal that deletes a file.
func (w *Workspace) DeleteFile(rel string, defaults []string) error {
	abs, _, err := w.Resolve(rel, defaults)
	if err != nil {
		return err
	}
	w.Invalidate()
	return os.Remove(abs)
}
