package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ---- rendering demos to images -----------------------------------------------
//
// Demo scenes are laid out and drawn by the UI (the layout engine runs
// there), so turning a step into a PNG or SVG for an agent is a round trip:
// the core publishes a render request with the demo attached, an open
// CodeDiver window draws it and posts the result back, and the waiting
// caller gets it. With no window open the request times out.

// EvRender asks the UI to render a demo step.
const EvRender = "render"

// RenderRequest is what the UI is asked to draw.
type RenderRequest struct {
	ID     string `json:"id"`
	Demo   *Demo  `json:"demo"`
	Step   int    `json:"step"`   // 0-based
	Format string `json:"format"` // png or svg
	Theme  string `json:"theme"`  // light, dark or current
}

// RenderResult is what the UI sends back: base64 PNG bytes or SVG text.
type RenderResult struct {
	ID     string `json:"id"`
	Data   string `json:"data"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Error  string `json:"error,omitempty"`
}

// renderTimeout bounds how long a caller waits for a window to answer.
var renderTimeout = 20 * time.Second

const maxRenderBytes = 24 << 20

// RenderDemo asks an open CodeDiver window to draw step (1-based; 0 means
// the last step) of a demo as png or svg. id may be empty for the repo's
// newest demo.
func (s *Service) RenderDemo(ctx context.Context, repo, id string, step int, format, theme string) (*RenderResult, *Demo, error) {
	d := s.findDemo(repo, id)
	if d == nil {
		if id == "" {
			return nil, nil, errors.New("there are no demos yet; create one with show_demo")
		}
		return nil, nil, fmt.Errorf("unknown demo %q (demos live for the current CodeDiver session)", id)
	}
	if step < 0 || step > len(d.Steps) {
		return nil, nil, fmt.Errorf("step must be between 1 and %d", len(d.Steps))
	}
	if step == 0 {
		step = len(d.Steps)
	}
	switch format {
	case "":
		format = "png"
	case "png", "svg":
	default:
		return nil, nil, fmt.Errorf("format must be png or svg, not %q", format)
	}
	switch theme {
	case "":
		theme = "current"
	case "light", "dark", "current":
	default:
		return nil, nil, fmt.Errorf("theme must be light, dark or current, not %q", theme)
	}

	ch := make(chan RenderResult, 1)
	s.session.mu.Lock()
	rid := s.session.id("render")
	s.session.renders[rid] = ch
	s.session.mu.Unlock()
	defer func() {
		s.session.mu.Lock()
		delete(s.session.renders, rid)
		s.session.mu.Unlock()
	}()
	s.emit(EvRender, RenderRequest{ID: rid, Demo: d, Step: step - 1, Format: format, Theme: theme})

	timer := time.NewTimer(renderTimeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		if r.Error != "" {
			return nil, d, fmt.Errorf("CodeDiver couldn't render it: %s", r.Error)
		}
		return &r, d, nil
	case <-timer.C:
		return nil, d, errors.New("no CodeDiver window answered; rendering needs the app open (the desktop app, or `codediver serve` in a browser)")
	case <-ctx.Done():
		return nil, d, ctx.Err()
	}
}

// CompleteRender delivers a window's answer to a pending render. The first
// answer wins; later ones (another open window) are ignored.
func (s *Service) CompleteRender(r RenderResult) error {
	if len(r.Data) > maxRenderBytes {
		return fmt.Errorf("render is %d MB; the limit is %d MB", len(r.Data)>>20, maxRenderBytes>>20)
	}
	s.session.mu.Lock()
	ch, ok := s.session.renders[r.ID]
	s.session.mu.Unlock()
	if !ok {
		return nil
	}
	select {
	case ch <- r:
	default:
	}
	return nil
}

// findDemo returns a repository's demo by id, or its newest when id is
// empty.
func (s *Service) findDemo(repo, id string) *Demo {
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	if id != "" {
		if d := s.session.demos[id]; d != nil && d.Repo == repo {
			return d
		}
		return nil
	}
	var all []*Demo
	for _, d := range s.session.demos {
		if d.Repo == repo {
			all = append(all, d)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	if len(all) == 0 {
		return nil
	}
	return all[0]
}
