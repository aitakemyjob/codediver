// Package gitx wraps the git CLI. Shelling out keeps behavior identical to
// what developers see in their terminal (config, attributes, hooks-free).
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrNotRepo = errors.New("not a git repository")

type Repo struct {
	Dir string
}

// New returns a git wrapper for the repository at dir.
func New(dir string) *Repo { return &Repo{Dir: dir} }

// Available reports whether the git binary is installed.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// run executes git in the repository with a timeout, deterministic output
// (no colors, no quoted paths, C locale) and no interactive prompts.
func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	full := append([]string{"-C", r.Dir, "-c", "core.quotepath=off", "-c", "color.ui=never"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "not a git repository") {
			return "", ErrNotRepo
		}
		return stdout.String(), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, msg)
	}
	return stdout.String(), nil
}

type Info struct {
	IsRepo   bool   `json:"isRepo"`
	Branch   string `json:"branch"`
	Head     string `json:"head"`
	Upstream string `json:"upstream,omitempty"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Dirty    int    `json:"dirty"`
	HasHead  bool   `json:"hasHead"`
}

// Info returns branch, HEAD, upstream tracking and the number of changed
// files. A directory that is not a git repository yields IsRepo=false.
func (r *Repo) Info(ctx context.Context) (Info, error) {
	out, err := r.run(ctx, "status", "--porcelain=v2", "--branch")
	if err != nil {
		if errors.Is(err, ErrNotRepo) {
			return Info{}, nil
		}
		return Info{}, err
	}
	info := Info{IsRepo: true}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			info.Head = strings.TrimPrefix(line, "# branch.oid ")
			info.HasHead = info.Head != "(initial)"
		case strings.HasPrefix(line, "# branch.head "):
			info.Branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.upstream "):
			info.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
		case strings.HasPrefix(line, "# branch.ab "):
			f := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(f) == 2 {
				info.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[0], "+"))
				info.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[1], "-"))
			}
		case line != "" && !strings.HasPrefix(line, "#"):
			info.Dirty++
		}
	}
	return info, nil
}

type StatusEntry struct {
	Path     string     `json:"path"`
	OrigPath string     `json:"origPath,omitempty"`
	Index    string     `json:"index"`
	Worktree string     `json:"worktree"`
	Status   FileStatus `json:"status"`
	Staged   bool       `json:"staged"`
}

// Status lists changed, staged and untracked files.
func (r *Repo) Status(ctx context.Context) ([]StatusEntry, error) {
	out, err := r.run(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parseStatusZ(out), nil
}

// parseStatusZ parses `git status --porcelain=v1 -z` output.
func parseStatusZ(out string) []StatusEntry {
	var entries []StatusEntry
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) < 4 {
			continue
		}
		e := StatusEntry{Index: p[0:1], Worktree: p[1:2], Path: p[3:]}
		x, y := p[0], p[1]
		switch {
		case x == '?':
			e.Status = StatusUntracked
		case x == 'R' || y == 'R':
			e.Status = StatusRenamed
			if i+1 < len(parts) {
				e.OrigPath = parts[i+1]
				i++
			}
		case x == 'A' || y == 'A':
			e.Status = StatusAdded
		case x == 'D' || y == 'D':
			e.Status = StatusDeleted
		default:
			e.Status = StatusModified
		}
		e.Staged = x != ' ' && x != '?'
		entries = append(entries, e)
	}
	return entries
}

// DiffSpec selects what to compare.
//   - zero value: working tree (incl. untracked) vs HEAD
//   - Staged: index vs HEAD
//   - Base only: working tree vs Base
//   - Base and Head: Base vs Head
type DiffSpec struct {
	// Commit, when set, selects the changes that commit introduced: it is
	// compared with its first parent, or with the empty tree for a root
	// commit. Base and Head are ignored.
	Commit  string   `json:"commit,omitempty"`
	Base    string   `json:"base,omitempty"`
	Head    string   `json:"head,omitempty"`
	Staged  bool     `json:"staged,omitempty"`
	Paths   []string `json:"paths,omitempty"`
	Context int      `json:"context,omitempty"`
}

// validRef rejects refs that could be interpreted as command-line options
// or that contain whitespace.
func validRef(ref string) error {
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\n\x00") {
		return fmt.Errorf("invalid ref %q", ref)
	}
	return nil
}

// Diff runs `git diff` for the given spec and parses the result. For
// working-tree diffs, untracked files are included as all-added diffs.
func (r *Repo) Diff(ctx context.Context, spec DiffSpec) ([]FileDiff, error) {
	if spec.Context <= 0 {
		spec.Context = 3
	}
	if spec.Commit != "" {
		base, head, err := r.CommitRange(ctx, spec.Commit)
		if err != nil {
			return nil, err
		}
		spec.Base, spec.Head, spec.Staged = base, head, false
	}
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "-M", "--src-prefix=a/", "--dst-prefix=b/", fmt.Sprintf("-U%d", spec.Context)}
	for _, ref := range []string{spec.Base, spec.Head} {
		if ref != "" {
			if err := validRef(ref); err != nil {
				return nil, err
			}
		}
	}
	working := false
	switch {
	case spec.Staged:
		args = append(args, "--cached")
	case spec.Base != "" && spec.Head != "":
		args = append(args, spec.Base, spec.Head)
	case spec.Base != "":
		args = append(args, spec.Base)
		working = true
	default:
		working = true
		if info, _ := r.Info(ctx); info.HasHead {
			args = append(args, "HEAD")
		}
	}
	if len(spec.Paths) > 0 {
		args = append(args, "--")
		args = append(args, spec.Paths...)
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	files := ParseUnifiedDiff(out)
	if working {
		untracked, err := r.run(ctx, "ls-files", "-z", "--others", "--exclude-standard")
		if err == nil {
			for _, p := range strings.Split(untracked, "\x00") {
				if p == "" || !pathSelected(p, spec.Paths) {
					continue
				}
				data, err := os.ReadFile(filepath.Join(r.Dir, filepath.FromSlash(p)))
				if err != nil {
					continue
				}
				if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
					files = append(files, FileDiff{NewPath: p, Status: StatusUntracked, Binary: true})
					continue
				}
				if len(data) > 1<<20 {
					data = data[:1<<20]
				}
				files = append(files, SyntheticAddDiff(p, string(data)))
			}
		}
	}
	return files, nil
}

// pathSelected reports whether p matches one of the path filters (exact
// file or directory prefix); no filters selects everything.
func pathSelected(p string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		f = strings.TrimSuffix(f, "/")
		if p == f || strings.HasPrefix(p, f+"/") || f == "." {
			return true
		}
	}
	return false
}

// Empty tree object IDs, used as the "before" side of a root commit.
const (
	emptyTreeSHA1   = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	emptyTreeSHA256 = "6ef19b41225c5369f1c104d45d8d85efa9b057b53b14b4b9b939dd74decc5321"
)

// CommitRange returns the revisions to compare to see what a commit
// introduced: its first parent and the commit itself, or the empty tree
// and the commit when it has no parent (the first commit of a history).
func (r *Repo) CommitRange(ctx context.Context, commit string) (base, head string, err error) {
	if err := validRef(commit); err != nil {
		return "", "", err
	}
	out, err := r.run(ctx, "rev-parse", "--verify", "--quiet", commit+"^{commit}")
	if err != nil {
		if errors.Is(err, ErrNotRepo) {
			return "", "", err
		}
		return "", "", fmt.Errorf("unknown commit %q", commit)
	}
	head = strings.TrimSpace(out)
	if out, err := r.run(ctx, "rev-parse", "--verify", "--quiet", head+"^1"); err == nil {
		return strings.TrimSpace(out), head, nil
	}
	if format, _ := r.run(ctx, "rev-parse", "--show-object-format"); strings.TrimSpace(format) == "sha256" {
		return emptyTreeSHA256, head, nil
	}
	return emptyTreeSHA1, head, nil
}

// Show returns a file's content at a ref ("" or "HEAD" for HEAD, ":" for the index).
func (r *Repo) Show(ctx context.Context, ref, path string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	if err := validRef(ref); err != nil {
		return "", err
	}
	spec := ref + ":" + path
	if ref == ":" {
		spec = ":" + path
	}
	return r.run(ctx, "show", spec)
}

type Commit struct {
	Hash      string       `json:"hash"`
	Short     string       `json:"short"`
	Author    string       `json:"author"`
	Email     string       `json:"email"`
	Time      time.Time    `json:"time"`
	Subject   string       `json:"subject"`
	Files     []CommitFile `json:"files,omitempty"`
	Additions int          `json:"additions"`
	Deletions int          `json:"deletions"`
}

type CommitFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

const logFormat = "--format=\x1e%H\x1f%h\x1f%an\x1f%ae\x1f%at\x1f%s"

// Log returns recent commits, optionally restricted to a path, with numstat.
func (r *Repo) Log(ctx context.Context, n int, ref, path string) ([]Commit, error) {
	if n <= 0 || n > 1000 {
		n = 50
	}
	args := []string{"log", logFormat, "--numstat", "-n", strconv.Itoa(n)}
	if ref != "" {
		if err := validRef(ref); err != nil {
			return nil, err
		}
		args = append(args, ref)
	}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") {
			return nil, nil
		}
		return nil, err
	}
	return parseLog(out), nil
}

// parseLog parses the record-separated output produced with logFormat and
// --numstat.
func parseLog(out string) []Commit {
	var commits []Commit
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		lines := strings.Split(rec, "\n")
		f := strings.Split(lines[0], "\x1f")
		if len(f) < 6 {
			continue
		}
		ts, _ := strconv.ParseInt(f[4], 10, 64)
		c := Commit{Hash: f[0], Short: f[1], Author: f[2], Email: f[3], Time: time.Unix(ts, 0), Subject: f[5]}
		for _, l := range lines[1:] {
			cols := strings.SplitN(l, "\t", 3)
			if len(cols) != 3 {
				continue
			}
			add, _ := strconv.Atoi(cols[0])
			del, _ := strconv.Atoi(cols[1])
			c.Files = append(c.Files, CommitFile{Path: renameTarget(cols[2]), Additions: add, Deletions: del})
			c.Additions += add
			c.Deletions += del
		}
		commits = append(commits, c)
	}
	return commits
}

// renameTarget resolves numstat rename notation: "a/{b => c}/d" or "x => y".
func renameTarget(p string) string {
	if !strings.Contains(p, " => ") {
		return p
	}
	if i := strings.Index(p, "{"); i >= 0 {
		if j := strings.Index(p[i:], "}"); j >= 0 {
			inner := p[i+1 : i+j]
			parts := strings.SplitN(inner, " => ", 2)
			res := p[:i] + parts[len(parts)-1] + p[i+j+1:]
			return strings.ReplaceAll(res, "//", "/")
		}
	}
	parts := strings.SplitN(p, " => ", 2)
	return parts[1]
}

type FileChurn struct {
	Path      string    `json:"path"`
	Commits   int       `json:"commits"`
	Additions int       `json:"additions"`
	Deletions int       `json:"deletions"`
	Authors   []string  `json:"authors"`
	LastTime  time.Time `json:"lastTime"`
}

// Churn aggregates per-file change frequency over the last `days`.
func (r *Repo) Churn(ctx context.Context, days int) (map[string]*FileChurn, error) {
	if days <= 0 {
		days = 180
	}
	out, err := r.run(ctx, "log", logFormat, "--numstat", "--no-merges", fmt.Sprintf("--since=%d.days", days), "-n", "5000")
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") {
			return map[string]*FileChurn{}, nil
		}
		return nil, err
	}
	res := map[string]*FileChurn{}
	authors := map[string]map[string]bool{}
	for _, c := range parseLog(out) {
		for _, f := range c.Files {
			fc := res[f.Path]
			if fc == nil {
				fc = &FileChurn{Path: f.Path}
				res[f.Path] = fc
				authors[f.Path] = map[string]bool{}
			}
			fc.Commits++
			fc.Additions += f.Additions
			fc.Deletions += f.Deletions
			authors[f.Path][c.Author] = true
			if c.Time.After(fc.LastTime) {
				fc.LastTime = c.Time
			}
		}
	}
	for p, set := range authors {
		for a := range set {
			res[p].Authors = append(res[p].Authors, a)
		}
		sort.Strings(res[p].Authors)
	}
	return res, nil
}

type Ref struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // branch | remote | tag
	Hash string `json:"hash"`
}

// Refs lists local branches, remote branches and tags.
func (r *Repo) Refs(ctx context.Context) ([]Ref, error) {
	out, err := r.run(ctx, "for-each-ref", "--format=%(refname)\x1f%(refname:short)\x1f%(objectname:short)", "refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, err
	}
	var refs []Ref
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 3 || strings.HasSuffix(f[0], "/HEAD") {
			continue
		}
		kind := "branch"
		switch {
		case strings.HasPrefix(f[0], "refs/remotes/"):
			kind = "remote"
		case strings.HasPrefix(f[0], "refs/tags/"):
			kind = "tag"
		}
		refs = append(refs, Ref{Name: f[1], Kind: kind, Hash: f[2]})
	}
	return refs, nil
}
