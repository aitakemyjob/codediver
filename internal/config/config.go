// Package config persists CodeDiver's user settings: the repositories the
// agent is allowed to see, the UI theme, and MCP server options.
package config

import (
	"crypto/sha1" //nolint:gosec // repo IDs, not security; changing the hash would orphan saved repos
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aitakemyjob/codediver/internal/qa"
)

const DefaultMCPPort = 47474

// DefaultExcludes are glob patterns hidden from the agent in every repo.
var DefaultExcludes = []string{".env", ".env.*", "*.pem", "*.key", "*.p12", "id_rsa*", "*.keystore"}

type Repo struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
	// MCPEnabled controls whether agents can see this repo at all.
	MCPEnabled bool `json:"mcpEnabled"`
	// Exclude lists extra glob patterns (matched against the base name and the
	// repo-relative path) that are invisible to the agent.
	Exclude []string `json:"exclude"`
	// Suites are the developer's coverage commands. Nil means "use the
	// detected defaults"; only the UI can change them.
	Suites []qa.Suite `json:"suites,omitempty"`
}

type Settings struct {
	Theme      string `json:"theme"`
	FontSize   int    `json:"fontSize"`
	MCPPort    int    `json:"mcpPort"`
	MCPEnabled bool   `json:"mcpEnabled"`
	// FollowAgent makes the UI jump to whatever the agent shows.
	FollowAgent bool `json:"followAgent"`
	// ChurnDays is the git history window used for churn heatmaps.
	ChurnDays int `json:"churnDays"`
}

type Config struct {
	Repos      []Repo   `json:"repos"`
	ActiveRepo string   `json:"activeRepo"`
	Settings   Settings `json:"settings"`
}

// defaults returns the configuration used when no config file exists yet.
func defaults() Config {
	return Config{
		Repos: []Repo{},
		Settings: Settings{
			Theme:       "deepdive",
			FontSize:    13,
			MCPPort:     DefaultMCPPort,
			MCPEnabled:  true,
			FollowAgent: true,
			ChurnDays:   180,
		},
	}
}

// Store is a concurrency-safe, file-backed Config.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

// DefaultPath returns the platform config location, e.g.
// ~/.config/codediver/config.json on Linux.
func DefaultPath() (string, error) {
	if p := os.Getenv("CODEDIVER_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codediver", "config.json"), nil
}

// Open loads the config at path, filling in defaults for missing settings.
// A missing file is not an error; it is created on the first save.
func Open(path string) (*Store, error) {
	s := &Store{path: path, cfg: defaults()}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(data, &s.cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	d := defaults().Settings
	if s.cfg.Settings.MCPPort == 0 {
		s.cfg.Settings.MCPPort = d.MCPPort
	}
	if s.cfg.Settings.FontSize == 0 {
		s.cfg.Settings.FontSize = d.FontSize
	}
	if s.cfg.Settings.ChurnDays == 0 {
		s.cfg.Settings.ChurnDays = d.ChurnDays
	}
	if s.cfg.Settings.Theme == "" {
		s.cfg.Settings.Theme = d.Theme
	}
	if s.cfg.Repos == nil {
		s.cfg.Repos = []Repo{}
	}
	return s, nil
}

// Get returns a snapshot copy of the current configuration.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.cfg
	c.Repos = append([]Repo(nil), s.cfg.Repos...)
	return c
}

// saveLocked writes the config atomically (temp file + rename). Callers
// must hold s.mu.
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// repoID derives a short, stable identifier from a repository path.
func repoID(path string) string {
	h := sha1.Sum([]byte(path)) //nolint:gosec // stable short ID, not a security boundary
	return hex.EncodeToString(h[:])[:10]
}

// AddRepo registers a directory. The path is cleaned and symlinks resolved so
// sandbox checks compare like with like.
func (s *Store) AddRepo(path, name string) (Repo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Repo{}, err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	st, err := os.Stat(abs)
	if err != nil {
		return Repo{}, err
	}
	if !st.IsDir() {
		return Repo{}, fmt.Errorf("%s is not a directory", abs)
	}
	if name == "" {
		name = filepath.Base(abs)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.cfg.Repos {
		if r.Path == abs {
			return r, nil
		}
	}
	r := Repo{
		ID:         repoID(abs),
		Name:       name,
		Path:       abs,
		AddedAt:    time.Now(),
		MCPEnabled: true,
		Exclude:    []string{},
	}
	s.cfg.Repos = append(s.cfg.Repos, r)
	if s.cfg.ActiveRepo == "" {
		s.cfg.ActiveRepo = r.ID
	}
	return r, s.saveLocked()
}

// RemoveRepo forgets a repository (files on disk are untouched). If it was
// active, the first remaining repository becomes active.
func (s *Store) RemoveRepo(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.cfg.Repos[:0]
	for _, r := range s.cfg.Repos {
		if r.ID != id {
			out = append(out, r)
		}
	}
	s.cfg.Repos = out
	if s.cfg.ActiveRepo == id {
		s.cfg.ActiveRepo = ""
		if len(out) > 0 {
			s.cfg.ActiveRepo = out[0].ID
		}
	}
	return s.saveLocked()
}

// UpdateRepo saves the user-editable fields of a repository: name, agent
// visibility and exclude globs.
func (s *Store) UpdateRepo(r Repo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Repos {
		if s.cfg.Repos[i].ID == r.ID {
			s.cfg.Repos[i].Name = r.Name
			s.cfg.Repos[i].MCPEnabled = r.MCPEnabled
			s.cfg.Repos[i].Exclude = r.Exclude
			return s.saveLocked()
		}
	}
	return fmt.Errorf("unknown repo %q", r.ID)
}

// SetSuites saves a repository's coverage suites. Commands are run by the
// developer from the UI, so this is never reachable from agents.
func (s *Store) SetSuites(id string, suites []qa.Suite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Repos {
		if s.cfg.Repos[i].ID == id {
			s.cfg.Repos[i].Suites = suites
			return s.saveLocked()
		}
	}
	return fmt.Errorf("unknown repo %q", id)
}

// SetActive marks the repository the UI and agents default to.
func (s *Store) SetActive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.cfg.Repos {
		if r.ID == id {
			s.cfg.ActiveRepo = id
			return s.saveLocked()
		}
	}
	return fmt.Errorf("unknown repo %q", id)
}

// UpdateSettings validates, clamps and persists application settings.
func (s *Store) UpdateSettings(st Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.MCPPort <= 0 || st.MCPPort > 65535 {
		st.MCPPort = DefaultMCPPort
	}
	if st.FontSize < 9 || st.FontSize > 28 {
		st.FontSize = 13
	}
	if st.ChurnDays <= 0 {
		st.ChurnDays = 180
	}
	s.cfg.Settings = st
	return s.saveLocked()
}

// FindRepo looks a repo up by ID, name (case-insensitive) or path.
func (s *Store) FindRepo(ref string) (Repo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ref == "" {
		ref = s.cfg.ActiveRepo
	}
	for _, r := range s.cfg.Repos {
		if r.ID == ref || strings.EqualFold(r.Name, ref) || r.Path == ref {
			return r, true
		}
	}
	return Repo{}, false
}
