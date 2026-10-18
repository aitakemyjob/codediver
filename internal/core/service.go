// Package core is CodeDiver's application service. Both the desktop UI (via
// internal/api) and coding agents (via internal/mcpserver) are thin adapters
// over the methods defined here, so behavior and sandboxing stay identical.
package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/events"
	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/proposals"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

// Event types published on the bus.
const (
	EvConfig      = "config"
	EvActivity    = "activity"
	EvFocus       = "focus"
	EvProposal    = "proposal"
	EvWalkthrough = "walkthrough"
	EvDiagram     = "diagram"
	EvExplain     = "explain"
	EvAnnotations = "annotations"
	EvIndex       = "index"
	EvContext     = "context"
	EvMCP         = "mcp"
	EvQA          = "qa"
	EvDemo        = "demo"
	EvAnimation   = "animation"
)

var ErrAgentDisabled = errors.New("agent access is disabled in CodeDiver settings")

type Service struct {
	Config    *config.Store
	Bus       *events.Bus
	Proposals *proposals.Store
	Version   string
	// CacheDir holds saved coverage and coverage-run scratch output.
	CacheDir string

	mu      sync.Mutex
	repos   map[string]*RepoHandle
	session *session
	mcp     MCPStatus
}

type MCPStatus struct {
	Running  bool   `json:"running"`
	URL      string `json:"url"`
	Error    string `json:"error,omitempty"`
	Sessions int    `json:"sessions"`
}

// New creates the application service around a config store. version is
// reported to agents and shown in the UI.
func New(cfg *config.Store, version string) *Service {
	return &Service{
		Config:    cfg,
		Bus:       events.NewBus(),
		Proposals: proposals.NewStore(),
		Version:   version,
		CacheDir:  defaultCacheDir(),
		repos:     map[string]*RepoHandle{},
		session:   newSession(),
	}
}

// RepoHandle is the runtime state of one configured repository: its
// sandboxed workspace, git wrapper and analysis cache.
type RepoHandle struct {
	cfg config.Repo
	ws  *workspace.Workspace
	git *gitx.Repo
	idx indexCache
	qa  qaState
}

// runtime returns the cached handle for a repo, creating it on first use
// or when the repo's path changed, and refreshes its exclude globs.
func (s *Service) runtime(r config.Repo) *RepoHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt, ok := s.repos[r.ID]
	if !ok || rt.cfg.Path != r.Path {
		rt = &RepoHandle{cfg: r, ws: workspace.New(r.Path, r.Exclude), git: gitx.New(r.Path)}
		s.repos[r.ID] = rt
	}
	// Exclusions may have been edited; keep them current.
	rt.cfg = r
	rt.ws.Exclude = r.Exclude
	return rt
}

// Repo resolves a repo for the UI. An empty ref means the active repo.
func (s *Service) Repo(ref string) (*RepoHandle, error) {
	r, ok := s.Config.FindRepo(ref)
	if !ok {
		if ref == "" {
			return nil, errors.New("no repository configured; add one in CodeDiver")
		}
		return nil, fmt.Errorf("unknown repository %q", ref)
	}
	return s.runtime(r), nil
}

// AgentRepo resolves a repo for an agent, enforcing the MCP access switches.
func (s *Service) AgentRepo(ref string) (*RepoHandle, error) {
	cfg := s.Config.Get()
	if !cfg.Settings.MCPEnabled {
		return nil, ErrAgentDisabled
	}
	r, ok := s.Config.FindRepo(ref)
	if !ok || !r.MCPEnabled {
		var names []string
		for _, r := range cfg.Repos {
			if r.MCPEnabled {
				names = append(names, r.Name)
			}
		}
		if len(names) == 0 {
			return nil, errors.New("no repositories are shared with agents; the user must add one in CodeDiver")
		}
		if ref == "" {
			return nil, fmt.Errorf("no active repository is shared with agents; pass repo as one of: %s", strings.Join(names, ", "))
		}
		return nil, fmt.Errorf("repository %q is not available; choose one of: %s", ref, strings.Join(names, ", "))
	}
	return s.runtime(r), nil
}

// ID returns the repository's stable identifier.
func (rt *RepoHandle) ID() string { return rt.cfg.ID }

// Name returns the repository's display name.
func (rt *RepoHandle) Name() string { return rt.cfg.Name }

// WS returns the repository's sandboxed workspace.
func (rt *RepoHandle) WS() *workspace.Workspace { return rt.ws }

// Git returns the repository's git wrapper.
func (rt *RepoHandle) Git() *gitx.Repo { return rt.git }

// emit publishes an event to all connected UIs.
func (s *Service) emit(typ string, data any) { s.Bus.Publish(typ, data) }

// ---- repos & settings -------------------------------------------------

type RepoSummary struct {
	config.Repo
	Git     gitx.Info `json:"git"`
	Missing bool      `json:"missing,omitempty"`
}

// ListRepos returns every configured repository with its git state; repos
// whose folder disappeared are flagged Missing.
func (s *Service) ListRepos(ctx context.Context) []RepoSummary {
	cfg := s.Config.Get()
	out := make([]RepoSummary, 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		rs := RepoSummary{Repo: r}
		rt := s.runtime(r)
		if _, err := rt.ws.Files(config.DefaultExcludes); err != nil {
			rs.Missing = true
		}
		if gitx.Available() {
			rs.Git, _ = rt.git.Info(ctx)
		}
		out = append(out, rs)
	}
	return out
}

// AddRepo registers a directory and notifies the UI.
func (s *Service) AddRepo(path, name string) (config.Repo, error) {
	r, err := s.Config.AddRepo(path, name)
	if err == nil {
		s.emit(EvConfig, s.Config.Get())
	}
	return r, err
}

// RemoveRepo forgets a repository, drops its cached state and notifies
// the UI.
func (s *Service) RemoveRepo(id string) error {
	err := s.Config.RemoveRepo(id)
	s.mu.Lock()
	delete(s.repos, id)
	s.mu.Unlock()
	s.emit(EvConfig, s.Config.Get())
	return err
}

// UpdateRepo saves a repository's name, agent visibility and excludes.
func (s *Service) UpdateRepo(r config.Repo) error {
	err := s.Config.UpdateRepo(r)
	s.emit(EvConfig, s.Config.Get())
	return err
}

// SetActiveRepo changes the repository the UI and agents default to.
func (s *Service) SetActiveRepo(id string) error {
	err := s.Config.SetActive(id)
	s.emit(EvConfig, s.Config.Get())
	return err
}

// UpdateSettings persists application settings and notifies the UI.
func (s *Service) UpdateSettings(st config.Settings) error {
	err := s.Config.UpdateSettings(st)
	s.emit(EvConfig, s.Config.Get())
	return err
}

// SetMCPStatus records whether the MCP server is listening and notifies
// the UI.
func (s *Service) SetMCPStatus(st MCPStatus) {
	s.mu.Lock()
	s.mcp = st
	s.mu.Unlock()
	s.emit(EvMCP, st)
}

// MCPStatus returns the MCP server's current status.
func (s *Service) MCPStatus() MCPStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mcp
}

// ---- activity ------------------------------------------------------------

type Activity struct {
	ID         int       `json:"id"`
	Time       time.Time `json:"time"`
	Tool       string    `json:"tool"`
	Summary    string    `json:"summary"`
	Repo       string    `json:"repo,omitempty"`
	Error      string    `json:"error,omitempty"`
	DurationMS int64     `json:"durationMs"`
}

// RecordActivity logs an agent tool call for the activity feed.
func (s *Service) RecordActivity(a Activity) {
	a = s.session.addActivity(a)
	s.emit(EvActivity, a)
}

// Activity returns the agent activity log, oldest first.
func (s *Service) Activity() []Activity { return s.session.activities() }
