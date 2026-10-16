package workspace

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type TreeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Dir      bool        `json:"dir"`
	Size     int64       `json:"size,omitempty"`
	Lines    int         `json:"lines,omitempty"`
	Language string      `json:"language,omitempty"`
	Files    int         `json:"files,omitempty"`
	Children []*TreeNode `json:"children,omitempty"`
}

// Tree builds a nested directory tree. When withLines is set every text file
// is read to count lines, which the heatmap uses for area.
func (w *Workspace) Tree(defaults []string, withLines bool) (*TreeNode, error) {
	files, err := w.Files(defaults)
	if err != nil {
		return nil, err
	}
	root := &TreeNode{Name: filepath.Base(w.Root), Path: "", Dir: true}
	dirs := map[string]*TreeNode{"": root}
	var ensure func(p string) *TreeNode
	ensure = func(p string) *TreeNode {
		if n, ok := dirs[p]; ok {
			return n
		}
		parent := ensure(parentDir(p))
		n := &TreeNode{Name: path.Base(p), Path: p, Dir: true}
		parent.Children = append(parent.Children, n)
		dirs[p] = n
		return n
	}
	for _, f := range files {
		parent := ensure(parentDir(f))
		n := &TreeNode{Name: path.Base(f), Path: f, Language: Language(f)}
		if st, err := os.Stat(filepath.Join(w.Root, filepath.FromSlash(f))); err == nil {
			n.Size = st.Size()
			if withLines && st.Size() < MaxReadBytes {
				n.Lines = countLines(filepath.Join(w.Root, filepath.FromSlash(f)))
			}
		}
		parent.Children = append(parent.Children, n)
	}
	var finish func(n *TreeNode)
	finish = func(n *TreeNode) {
		sort.Slice(n.Children, func(i, j int) bool {
			a, b := n.Children[i], n.Children[j]
			if a.Dir != b.Dir {
				return a.Dir
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
		for _, c := range n.Children {
			if c.Dir {
				finish(c)
				n.Files += c.Files
				n.Lines += c.Lines
				n.Size += c.Size
			} else {
				n.Files++
				n.Lines += c.Lines
				n.Size += c.Size
			}
		}
	}
	finish(root)
	return root, nil
}

// parentDir returns the slash-separated parent of p, or "" at the root.
func parentDir(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}

// countLines counts the lines of a text file; binary or unreadable files
// count as zero.
func countLines(p string) int {
	data, err := os.ReadFile(p)
	if err != nil || IsBinary(data) {
		return 0
	}
	n := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}

type SearchMatch struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

type SearchOptions struct {
	Query         string `json:"query"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	Glob          string `json:"glob"`
	Limit         int    `json:"limit"`
}

// Search greps every visible text file.
func (w *Workspace) Search(opts SearchOptions, defaults []string) ([]SearchMatch, bool, error) {
	if opts.Limit <= 0 || opts.Limit > 2000 {
		opts.Limit = 200
	}
	pat := opts.Query
	if !opts.Regex {
		pat = regexp.QuoteMeta(pat)
	}
	if !opts.CaseSensitive {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, false, err
	}
	files, err := w.Files(defaults)
	if err != nil {
		return nil, false, err
	}
	var out []SearchMatch
	for _, f := range files {
		if opts.Glob != "" {
			okBase, _ := path.Match(opts.Glob, path.Base(f))
			okFull, _ := path.Match(opts.Glob, f)
			if !okBase && !okFull && !strings.HasPrefix(f, strings.TrimSuffix(opts.Glob, "/")+"/") {
				continue
			}
		}
		fh, err := os.Open(filepath.Join(w.Root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		head := make([]byte, 512)
		n, _ := fh.Read(head)
		if IsBinary(head[:n]) {
			fh.Close()
			continue
		}
		fh.Seek(0, 0)
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		line := 0
		for sc.Scan() {
			line++
			t := sc.Text()
			if loc := re.FindStringIndex(t); loc != nil {
				if len(t) > 300 {
					start := max(0, loc[0]-100)
					t = t[start:min(len(t), start+300)]
				}
				out = append(out, SearchMatch{Path: f, Line: line, Column: loc[0] + 1, Text: t})
				if len(out) >= opts.Limit {
					fh.Close()
					return out, true, nil
				}
			}
		}
		fh.Close()
	}
	return out, false, nil
}

var langByExt = map[string]string{
	".go": "go", ".ts": "typescript", ".tsx": "typescript", ".mts": "typescript", ".cts": "typescript",
	".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
	".py": "python", ".pyi": "python", ".rs": "rust", ".java": "java", ".kt": "kotlin", ".kts": "kotlin",
	".cs": "csharp", ".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp", ".cxx": "cpp", ".hpp": "cpp", ".hh": "cpp",
	".rb": "ruby", ".php": "php", ".swift": "swift", ".scala": "scala", ".dart": "dart", ".lua": "lua",
	".sh": "shell", ".bash": "shell", ".zsh": "shell", ".ps1": "powershell",
	".json": "json", ".yaml": "yaml", ".yml": "yaml", ".toml": "ini", ".ini": "ini", ".xml": "xml",
	".html": "html", ".htm": "html", ".css": "css", ".scss": "scss", ".less": "less", ".vue": "html", ".svelte": "html",
	".md": "markdown", ".mdx": "markdown", ".sql": "sql", ".proto": "protobuf", ".graphql": "graphql", ".gql": "graphql",
	".tf": "hcl", ".hcl": "hcl", ".ex": "elixir", ".exs": "elixir", ".erl": "erlang", ".clj": "clojure",
	".r": "r", ".m": "objective-c", ".mm": "objective-c", ".pl": "perl", ".zig": "zig",
}

var langByName = map[string]string{
	"Dockerfile": "dockerfile", "Makefile": "makefile", "go.mod": "go.mod", "go.sum": "plaintext",
	"CMakeLists.txt": "cmake", "Jenkinsfile": "groovy",
}

// Language maps a path to a Monaco-compatible language id.
func Language(p string) string {
	base := path.Base(p)
	if l, ok := langByName[base]; ok {
		return l
	}
	if l, ok := langByExt[strings.ToLower(path.Ext(base))]; ok {
		return l
	}
	return "plaintext"
}

// IsCode reports whether a language is one CodeDiver analyzes for symbols.
func IsCode(lang string) bool {
	switch lang {
	case "go", "typescript", "javascript", "python", "rust", "java", "kotlin", "csharp", "c", "cpp",
		"ruby", "php", "swift", "scala", "dart", "lua":
		return true
	}
	return false
}
