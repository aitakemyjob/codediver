package config

import (
	"path/filepath"
	"testing"

	"github.com/aitakemyjob/codediver/internal/qa"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Settings.MCPPort; got != DefaultMCPPort {
		t.Fatalf("default port = %d", got)
	}
	dir := t.TempDir()
	r, err := s.AddRepo(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := s.AddRepo(dir, "other"); again.ID != r.ID {
		t.Fatal("adding the same path twice should be idempotent")
	}
	if s.Get().ActiveRepo != r.ID {
		t.Fatal("first repo should become active")
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if found, ok := reopened.FindRepo(r.Name); !ok || found.Path != r.Path || !found.MCPEnabled {
		t.Fatalf("FindRepo by name = %+v %v", found, ok)
	}
	if _, ok := reopened.FindRepo(""); !ok {
		t.Fatal("empty ref should resolve to the active repo")
	}
	if err := reopened.SetSuites(r.ID, []qa.Suite{{ID: "go:", Command: "go test ./..."}}); err != nil {
		t.Fatal(err)
	}
	if again, _ := Open(path); len(again.Get().Repos[0].Suites) != 1 {
		t.Fatal("suites were not persisted")
	}
	if err := reopened.SetSuites("nope", nil); err == nil {
		t.Fatal("expected error for unknown repo")
	}
	if err := reopened.RemoveRepo(r.ID); err != nil {
		t.Fatal(err)
	}
	if c := reopened.Get(); len(c.Repos) != 0 || c.ActiveRepo != "" {
		t.Fatalf("after remove: %+v", c)
	}
}

func TestAddRepoRejectsFiles(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "c.json"))
	if _, err := s.AddRepo(filepath.Join(t.TempDir(), "missing"), ""); err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestUpdateSettingsClamps(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "c.json"))
	if err := s.UpdateSettings(Settings{MCPPort: -1, FontSize: 99, Theme: "nord"}); err != nil {
		t.Fatal(err)
	}
	st := s.Get().Settings
	if st.MCPPort != DefaultMCPPort || st.FontSize != 13 || st.ChurnDays != 180 || st.Theme != "nord" {
		t.Fatalf("settings = %+v", st)
	}
}
