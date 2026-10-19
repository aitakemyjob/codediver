// Package proposals holds agent-proposed code changes. Proposals are only
// ever written to disk when the user accepts them in the UI, and acceptance
// refuses to clobber a file that changed since the proposal was made.
package proposals

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/textdiff"
)

type Action string

const (
	ActionModify Action = "modify"
	ActionCreate Action = "create"
	ActionDelete Action = "delete"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApplied  Status = "applied"
	StatusRejected Status = "rejected"
	StatusConflict Status = "conflict"
	StatusPartial  Status = "partial"
)

// Edit is a search/replace operation. Old must match exactly once.
type Edit struct {
	Old string `json:"old" jsonschema:"exact existing text to replace (must occur exactly once in the file)"`
	New string `json:"new" jsonschema:"replacement text"`
}

// ChangeInput is what an agent submits for one file.
type ChangeInput struct {
	Path       string  `json:"path" jsonschema:"repo-relative file path"`
	Action     Action  `json:"action,omitempty" jsonschema:"modify (default), create or delete"`
	NewContent *string `json:"new_content,omitempty" jsonschema:"full new file content; use for create or full rewrites"`
	Edits      []Edit  `json:"edits,omitempty" jsonschema:"search/replace edits applied in order; preferred for targeted modifications"`
	Rationale  string  `json:"rationale,omitempty" jsonschema:"why this file changes"`
}

type FileChange struct {
	Path         string      `json:"path"`
	Action       Action      `json:"action"`
	Rationale    string      `json:"rationale,omitempty"`
	Original     string      `json:"original"`
	Proposed     string      `json:"proposed"`
	OriginalHash string      `json:"originalHash"`
	Language     string      `json:"language"`
	Status       Status      `json:"status"`
	Additions    int         `json:"additions"`
	Deletions    int         `json:"deletions"`
	Hunks        []gitx.Hunk `json:"hunks"`
	Symbols      []string    `json:"symbols,omitempty"` // affected symbol IDs
	Error        string      `json:"error,omitempty"`
}

// ChangedNewLines lists proposed-file lines touched by the change.
func (c *FileChange) ChangedNewLines() []int {
	return gitx.FileDiff{Hunks: c.Hunks}.ChangedNewLines()
}

// ChangedOldLines lists original-file lines touched by the change.
func (c *FileChange) ChangedOldLines() []int {
	var out []int
	for _, h := range c.Hunks {
		last := max(h.OldStart, 1)
		for _, l := range h.Lines {
			switch l.Kind {
			case gitx.LineDelete:
				out = append(out, l.OldNo)
				last = l.OldNo
			case gitx.LineAdd:
				out = append(out, last)
			case gitx.LineContext:
				last = l.OldNo
			}
		}
	}
	return out
}

type Proposal struct {
	ID        string        `json:"id"`
	Repo      string        `json:"repo"`
	Title     string        `json:"title"`
	Summary   string        `json:"summary"`
	Status    Status        `json:"status"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
	Changes   []*FileChange `json:"changes"`
	Feedback  string        `json:"feedback,omitempty"`
	// Impact counts the blast radius of the change.
	Impact *ImpactSummary `json:"impact,omitempty"`
}

// ImpactSummary counts a change's blast radius.
type ImpactSummary struct {
	Changed int `json:"changed"`
	// Callers counts callers by distance: direct, two and three calls away.
	Callers  []int `json:"callers"`
	Files    int   `json:"files"`
	Packages int   `json:"packages"`
	Entries  int   `json:"entries"`
	// Untested counts direct callers no test reaches.
	Untested int `json:"untested"`
}

// Total is the number of callers in the radius.
func (s *ImpactSummary) Total() int {
	n := 0
	for _, c := range s.Callers {
		n += c
	}
	return n
}

// Additions sums added and deleted lines across all files.
func (p *Proposal) Additions() (add, del int) {
	for _, c := range p.Changes {
		add += c.Additions
		del += c.Deletions
	}
	return
}

// FS is the sandboxed filesystem a proposal is built against.
type FS interface {
	Read(path string) (content string, exists bool, err error)
	Write(path string, content []byte) error
	Delete(path string) error
	Language(path string) string
}

// hash fingerprints file content so acceptance can detect edits made after
// the proposal was created.
func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// ApplyEdits applies search/replace edits, returning agent-friendly errors.
func ApplyEdits(content string, edits []Edit) (string, error) {
	for i, e := range edits {
		if e.Old == "" {
			return "", fmt.Errorf("edit %d: old text is empty; use new_content to rewrite or create a file", i+1)
		}
		n := strings.Count(content, e.Old)
		if n == 0 {
			// Retry with normalized line endings before failing.
			if strings.Contains(content, "\r\n") {
				alt := strings.ReplaceAll(e.Old, "\n", "\r\n")
				if strings.Count(content, alt) == 1 {
					content = strings.Replace(content, alt, strings.ReplaceAll(e.New, "\n", "\r\n"), 1)
					continue
				}
			}
			return "", fmt.Errorf("edit %d: old text not found in file (it must match exactly, including whitespace): %q", i+1, preview(e.Old))
		}
		if n > 1 {
			return "", fmt.Errorf("edit %d: old text occurs %d times; include more surrounding context so it is unique: %q", i+1, n, preview(e.Old))
		}
		content = strings.Replace(content, e.Old, e.New, 1)
	}
	return content, nil
}

// preview shortens text for inclusion in error messages.
func preview(s string) string {
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}

// BuildChange materializes one ChangeInput against the filesystem.
func BuildChange(fs FS, in ChangeInput) (*FileChange, error) {
	if strings.TrimSpace(in.Path) == "" {
		return nil, errors.New("change is missing a path")
	}
	original, exists, err := fs.Read(in.Path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", in.Path, err)
	}
	action := in.Action
	if action == "" {
		action = ActionModify
		if !exists {
			action = ActionCreate
		}
	}
	fc := &FileChange{Path: in.Path, Action: action, Rationale: in.Rationale, Original: original, OriginalHash: hash(original), Language: fs.Language(in.Path), Status: StatusPending}
	switch action {
	case ActionCreate:
		if exists {
			return nil, fmt.Errorf("%s already exists; use action=modify", in.Path)
		}
		if in.NewContent == nil {
			return nil, fmt.Errorf("%s: create requires new_content", in.Path)
		}
		fc.Proposed = *in.NewContent
	case ActionDelete:
		if !exists {
			return nil, fmt.Errorf("%s does not exist", in.Path)
		}
	case ActionModify:
		if !exists {
			return nil, fmt.Errorf("%s does not exist; use action=create", in.Path)
		}
		switch {
		case in.NewContent != nil && len(in.Edits) > 0:
			return nil, fmt.Errorf("%s: give either new_content or edits, not both", in.Path)
		case in.NewContent != nil:
			fc.Proposed = *in.NewContent
		case len(in.Edits) > 0:
			fc.Proposed, err = ApplyEdits(original, in.Edits)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", in.Path, err)
			}
		default:
			return nil, fmt.Errorf("%s: modify requires new_content or edits", in.Path)
		}
		if fc.Proposed == original {
			return nil, fmt.Errorf("%s: proposed content is identical to the current file", in.Path)
		}
	default:
		return nil, fmt.Errorf("%s: unknown action %q", in.Path, action)
	}
	d := textdiff.Lines(fc.Original, fc.Proposed, 3)
	fc.Hunks, fc.Additions, fc.Deletions = d.Hunks, d.Additions, d.Deletions
	return fc, nil
}

type Store struct {
	mu    sync.Mutex
	items map[string]*Proposal
	seq   int
}

// NewStore creates an empty proposal store.
func NewStore() *Store { return &Store{items: map[string]*Proposal{}} }

// Add assigns an ID and timestamps, marks the proposal pending and stores it.
func (s *Store) Add(p *Proposal) *Proposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	p.ID = fmt.Sprintf("p%d", s.seq)
	now := time.Now()
	p.CreatedAt, p.UpdatedAt = now, now
	p.Status = StatusPending
	s.items[p.ID] = p
	return p
}

// Get returns a proposal by ID.
func (s *Store) Get(id string) (*Proposal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.items[id]
	return p, ok
}

// List returns proposals newest first, optionally filtered by repo.
func (s *Store) List(repo string) []*Proposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Proposal
	for _, p := range s.items {
		if repo == "" || p.Repo == repo {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt) || (out[i].CreatedAt.Equal(out[j].CreatedAt) && out[i].ID > out[j].ID)
	})
	return out
}

// Remove deletes a proposal (dismissed from the UI).
func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
}

// Resolve applies or rejects files of a proposal. paths==nil means all
// pending files. Conflicting files are marked and left untouched.
func (s *Store) Resolve(fs FS, id string, paths []string, accept bool, feedback string) (*Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("unknown proposal %q", id)
	}
	want := map[string]bool{}
	for _, path := range paths {
		want[path] = true
	}
	var errs []string
	for _, c := range p.Changes {
		if c.Status != StatusPending && c.Status != StatusConflict {
			continue
		}
		if len(want) > 0 && !want[c.Path] {
			continue
		}
		if !accept {
			c.Status = StatusRejected
			continue
		}
		current, exists, err := fs.Read(c.Path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			c.Status, c.Error = StatusConflict, err.Error()
			errs = append(errs, c.Path+": "+err.Error())
			continue
		}
		if (c.Action == ActionCreate && exists) || (c.Action != ActionCreate && hash(current) != c.OriginalHash) {
			c.Status, c.Error = StatusConflict, "file changed since the proposal was made"
			errs = append(errs, c.Path+": "+c.Error)
			continue
		}
		if c.Action == ActionDelete {
			err = fs.Delete(c.Path)
		} else {
			err = fs.Write(c.Path, []byte(c.Proposed))
		}
		if err != nil {
			c.Status, c.Error = StatusConflict, err.Error()
			errs = append(errs, c.Path+": "+err.Error())
			continue
		}
		c.Status, c.Error = StatusApplied, ""
	}
	if feedback != "" {
		p.Feedback = feedback
	}
	p.Status = rollup(p.Changes)
	p.UpdatedAt = time.Now()
	if len(errs) > 0 {
		return p, fmt.Errorf("some changes were not applied: %s", strings.Join(errs, "; "))
	}
	return p, nil
}

// rollup derives a proposal's overall status from its files' statuses.
func rollup(changes []*FileChange) Status {
	counts := map[Status]int{}
	for _, c := range changes {
		counts[c.Status]++
	}
	n := len(changes)
	switch {
	case counts[StatusPending] == n:
		return StatusPending
	case counts[StatusApplied] == n:
		return StatusApplied
	case counts[StatusRejected] == n:
		return StatusRejected
	case counts[StatusConflict] > 0:
		return StatusConflict
	}
	return StatusPartial
}
