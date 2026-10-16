package core

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

// indexCache rebuilds a repo's analysis index only when code files change.
type indexCache struct {
	mu        sync.Mutex
	ix        *analysis.Index
	print     string
	checkedAt time.Time
}

// fingerprint hashes the names, sizes and mtimes of analyzable files.
func fingerprint(root string, files []string) string {
	h := sha1.New()
	var buf [16]byte
	for _, f := range files {
		if !workspace.IsCode(workspace.Language(f)) {
			continue
		}
		st, err := os.Stat(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		h.Write([]byte(f))
		binary.LittleEndian.PutUint64(buf[:8], uint64(st.Size()))
		binary.LittleEndian.PutUint64(buf[8:], uint64(st.ModTime().UnixNano()))
		h.Write(buf[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Index returns the analysis index for a repo, rebuilding if stale.
func (s *Service) Index(_ context.Context, rt *RepoHandle) (*analysis.Index, error) {
	c := &rt.idx
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ix != nil && time.Since(c.checkedAt) < 2*time.Second {
		return c.ix, nil
	}
	files, err := rt.ws.Files(config.DefaultExcludes)
	if err != nil {
		return nil, err
	}
	fp := fingerprint(rt.cfg.Path, files)
	c.checkedAt = time.Now()
	if c.ix != nil && fp == c.print {
		return c.ix, nil
	}
	s.emit(EvIndex, map[string]any{"repo": rt.cfg.ID, "state": "building"})
	ix := analysis.Build(rt.cfg.Path, files, workspace.Language)
	c.ix, c.print = ix, fp
	s.emit(EvIndex, map[string]any{"repo": rt.cfg.ID, "state": "ready", "stats": ix.Stats()})
	return ix, nil
}

// Reindex forces a rebuild on next access.
func (s *Service) Reindex(rt *RepoHandle) {
	rt.idx.mu.Lock()
	rt.idx.ix = nil
	rt.idx.mu.Unlock()
	rt.ws.Invalidate()
}
