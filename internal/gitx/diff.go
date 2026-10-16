package gitx

import (
	"strconv"
	"strings"
)

type LineKind string

const (
	LineContext LineKind = "context"
	LineAdd     LineKind = "add"
	LineDelete  LineKind = "delete"
)

type DiffLine struct {
	Kind    LineKind `json:"kind"`
	Content string   `json:"content"`
	OldNo   int      `json:"oldNo,omitempty"`
	NewNo   int      `json:"newNo,omitempty"`
}

type Hunk struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Header   string     `json:"header"`
	Lines    []DiffLine `json:"lines"`
}

type FileStatus string

const (
	StatusModified  FileStatus = "modified"
	StatusAdded     FileStatus = "added"
	StatusDeleted   FileStatus = "deleted"
	StatusRenamed   FileStatus = "renamed"
	StatusUntracked FileStatus = "untracked"
)

type FileDiff struct {
	OldPath   string     `json:"oldPath"`
	NewPath   string     `json:"newPath"`
	Status    FileStatus `json:"status"`
	Binary    bool       `json:"binary"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Hunks     []Hunk     `json:"hunks"`
}

// Path is the most relevant path: the new one, or the old one for deletions.
func (f FileDiff) Path() string {
	if f.Status == StatusDeleted || f.NewPath == "" {
		return f.OldPath
	}
	return f.NewPath
}

// ChangedNewLines returns the line numbers (in the new file) that were added,
// plus, for pure deletions, the line where the deletion happened. Used to map
// a diff onto symbols.
func (f FileDiff) ChangedNewLines() []int {
	var out []int
	seen := map[int]bool{}
	add := func(n int) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, h := range f.Hunks {
		lastNew := h.NewStart - 1
		for _, l := range h.Lines {
			switch l.Kind {
			case LineAdd:
				add(l.NewNo)
				lastNew = l.NewNo
			case LineDelete:
				add(max(lastNew+1, 1))
			case LineContext:
				lastNew = l.NewNo
			}
		}
	}
	return out
}

// stripPrefix removes the a/ or b/ prefix from a diff path, unquotes
// C-style quoted paths, and maps /dev/null to "".
func stripPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "/dev/null" {
		return ""
	}
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if u, err := strconv.Unquote(p); err == nil {
			p = u
		}
	}
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

// ParseUnifiedDiff parses `git diff` output into structured file diffs.
func ParseUnifiedDiff(text string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var hunk *Hunk
	oldNo, newNo := 0, 0

	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			if cur.Status == "" {
				cur.Status = StatusModified
			}
			files = append(files, *cur)
		}
		cur = nil
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			break
		}
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushFile()
			cur = &FileDiff{}
			// Best-effort path guess; ---/+++ and rename lines refine it.
			rest := strings.TrimPrefix(line, "diff --git ")
			if idx := strings.Index(rest, " b/"); idx >= 0 {
				cur.OldPath = stripPrefix(rest[:idx])
				cur.NewPath = stripPrefix(rest[idx+1:])
			}
		case cur == nil:
			continue
		case hunk != nil && len(line) > 0 && (line[0] == ' ' || line[0] == '+' || line[0] == '-'):
			switch line[0] {
			case ' ':
				oldNo++
				newNo++
				hunk.Lines = append(hunk.Lines, DiffLine{Kind: LineContext, Content: line[1:], OldNo: oldNo, NewNo: newNo})
			case '+':
				newNo++
				cur.Additions++
				hunk.Lines = append(hunk.Lines, DiffLine{Kind: LineAdd, Content: line[1:], NewNo: newNo})
			case '-':
				oldNo++
				cur.Deletions++
				hunk.Lines = append(hunk.Lines, DiffLine{Kind: LineDelete, Content: line[1:], OldNo: oldNo})
			}
		case hunk != nil && line == "":
			// Some tools strip the leading space from empty context lines.
			oldNo++
			newNo++
			hunk.Lines = append(hunk.Lines, DiffLine{Kind: LineContext, OldNo: oldNo, NewNo: newNo})
		case strings.HasPrefix(line, "\\ No newline"):
			continue
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			h := parseHunkHeader(line)
			hunk = &h
			oldNo, newNo = h.OldStart-1, h.NewStart-1
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = StatusAdded
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = StatusDeleted
		case strings.HasPrefix(line, "rename from "):
			cur.Status = StatusRenamed
			cur.OldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			cur.Status = StatusRenamed
			cur.NewPath = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files "):
			cur.Binary = true
		case strings.HasPrefix(line, "--- "):
			if p := stripPrefix(line[4:]); p != "" {
				cur.OldPath = p
			} else {
				cur.Status = StatusAdded
			}
		case strings.HasPrefix(line, "+++ "):
			if p := stripPrefix(line[4:]); p != "" {
				cur.NewPath = p
			} else {
				cur.Status = StatusDeleted
			}
		}
	}
	flushFile()
	for i := range files {
		switch files[i].Status {
		case StatusAdded:
			files[i].OldPath = ""
		case StatusDeleted:
			files[i].NewPath = ""
		}
	}
	return files
}

// parseHunkHeader parses "@@ -a,b +c,d @@ header".
func parseHunkHeader(line string) Hunk {
	h := Hunk{OldLines: 1, NewLines: 1}
	body := strings.TrimPrefix(line, "@@")
	end := strings.Index(body, "@@")
	if end < 0 {
		return h
	}
	h.Header = strings.TrimSpace(body[end+2:])
	for _, part := range strings.Fields(body[:end]) {
		if len(part) < 2 {
			continue
		}
		nums := strings.SplitN(part[1:], ",", 2)
		start, _ := strconv.Atoi(nums[0])
		count := 1
		if len(nums) == 2 {
			count, _ = strconv.Atoi(nums[1])
		}
		switch part[0] {
		case '-':
			h.OldStart, h.OldLines = start, count
		case '+':
			h.NewStart, h.NewLines = start, count
		}
	}
	return h
}

// SyntheticAddDiff builds a diff for an untracked file: every line is new.
func SyntheticAddDiff(path, content string) FileDiff {
	fd := FileDiff{NewPath: path, Status: StatusUntracked}
	if content == "" {
		return fd
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	h := Hunk{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: len(lines)}
	for i, l := range lines {
		h.Lines = append(h.Lines, DiffLine{Kind: LineAdd, Content: l, NewNo: i + 1})
	}
	fd.Additions = len(lines)
	fd.Hunks = []Hunk{h}
	return fd
}
