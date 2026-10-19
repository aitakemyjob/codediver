package core

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aitakemyjob/codediver/internal/proposals"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

// sandboxFS adapts a workspace to proposals.FS.
type sandboxFS struct{ ws *workspace.Workspace }

// Read returns a file's content and whether it exists; binary and
// truncated files are rejected as uneditable.
func (f sandboxFS) Read(p string) (string, bool, error) {
	fc, err := f.ws.ReadFile(p, defaults)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if fc.Binary {
		return "", true, fmt.Errorf("%s is binary", p)
	}
	if fc.Truncate {
		return "", true, fmt.Errorf("%s is too large to edit", p)
	}
	return fc.Content, true, nil
}

// Write writes a file through the sandbox.
func (f sandboxFS) Write(p string, b []byte) error { return f.ws.WriteFile(p, b, defaults) }

// Delete removes a file through the sandbox.
func (f sandboxFS) Delete(p string) error { return f.ws.DeleteFile(p, defaults) }

// Language returns the editor language for a path.
func (f sandboxFS) Language(p string) string { return workspace.Language(p) }

// Propose validates and stores an agent's change set and opens it in the UI.
func (s *Service) Propose(ctx context.Context, rt *RepoHandle, title, summary string, changes []proposals.ChangeInput) (*proposals.Proposal, error) {
	if len(changes) == 0 {
		return nil, errors.New("a proposal needs at least one change")
	}
	if title == "" {
		title = "Proposed change"
	}
	fs := sandboxFS{rt.ws}
	p := &proposals.Proposal{Repo: rt.cfg.ID, Title: title, Summary: summary}
	seen := map[string]bool{}
	for _, in := range changes {
		_, rel, err := rt.ws.Resolve(in.Path, defaults)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", in.Path, err)
		}
		if seen[rel] {
			return nil, fmt.Errorf("%s appears twice; combine its edits into one change", rel)
		}
		seen[rel] = true
		in.Path = rel
		fc, err := proposals.BuildChange(fs, in)
		if err != nil {
			return nil, err
		}
		p.Changes = append(p.Changes, fc)
	}
	// Map changed lines onto existing symbols and compute the blast radius.
	if ix, err := s.Index(ctx, rt); err == nil {
		var changed []string
		for _, c := range p.Changes {
			if c.Action == proposals.ActionCreate {
				continue
			}
			for _, cs := range changedSymbols(ix, c.Path, c.ChangedOldLines()) {
				c.Symbols = append(c.Symbols, cs.ID)
				changed = append(changed, cs.ID)
			}
		}
		if len(changed) > 0 {
			p.Impact = &s.impactOf(ctx, rt, ix, changed).Summary
		}
	}
	p = s.Proposals.Add(p)
	for _, c := range p.Changes {
		s.session.touch(rt.cfg.ID, c.Path)
	}
	s.emit(EvProposal, p)
	s.Show(Focus{View: "proposal", Repo: rt.cfg.ID, Title: p.Title, Params: map[string]any{"id": p.ID}})
	return p, nil
}

// ResolveProposal is called from the UI when the developer accepts/rejects.
func (s *Service) ResolveProposal(rt *RepoHandle, id string, paths []string, accept bool, feedback string) (*proposals.Proposal, error) {
	p, ok := s.Proposals.Get(id)
	if !ok || p.Repo != rt.cfg.ID {
		return nil, fmt.Errorf("unknown proposal %q", id)
	}
	p, err := s.Proposals.Resolve(sandboxFS{rt.ws}, id, paths, accept, feedback)
	if p != nil {
		if accept {
			s.Reindex(rt)
		}
		s.emit(EvProposal, p)
	}
	return p, err
}
