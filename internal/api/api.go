// Package api is the JSON method table the CodeDiver frontend calls. The same
// table is reached through Wails bindings in the desktop app and through
// POST /api/invoke in browser ("serve") mode.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"

	"github.com/aitakemyjob/codediver/internal/analysis"
	"github.com/aitakemyjob/codediver/internal/config"
	"github.com/aitakemyjob/codediver/internal/core"
	"github.com/aitakemyjob/codediver/internal/gitx"
	"github.com/aitakemyjob/codediver/internal/workspace"
)

type handler func(ctx context.Context, raw json.RawMessage) (any, error)

type API struct {
	core    *core.Service
	methods map[string]handler
	// PickDirectory is provided by the desktop shell; nil in browser mode.
	PickDirectory func() (string, error)
}

// method adapts a typed function into a handler.
func method[T any](fn func(context.Context, T) (any, error)) handler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in T
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, fmt.Errorf("bad arguments: %w", err)
			}
		}
		return fn(ctx, in)
	}
}

type repoReq struct {
	Repo string `json:"repo"`
}

// New builds the method table over a core service.
func New(c *core.Service) *API {
	a := &API{core: c}
	a.methods = map[string]handler{}
	a.registerApp()
	a.registerCode()
	a.registerGit()
	a.registerSession()
	return a
}

// Methods lists available method names (used by tests and docs).
func (a *API) Methods() []string {
	out := make([]string, 0, len(a.methods))
	for k := range a.methods {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Invoke runs a method with JSON arguments and returns JSON.
func (a *API) Invoke(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	h, ok := a.methods[name]
	if !ok {
		return nil, fmt.Errorf("unknown method %q", name)
	}
	out, err := h(ctx, args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

type AppState struct {
	Version     string             `json:"version"`
	Platform    string             `json:"platform"`
	Config      config.Config      `json:"config"`
	Repos       []core.RepoSummary `json:"repos"`
	MCP         core.MCPStatus     `json:"mcp"`
	HeatMetrics map[string]string  `json:"heatMetrics"`
	CanPickDir  bool               `json:"canPickDir"`
	GitFound    bool               `json:"gitFound"`
}

// registerApp adds app state, repository and settings methods.
func (a *API) registerApp() {
	c := a.core
	a.methods["app.state"] = method(func(ctx context.Context, _ struct{}) (any, error) {
		return AppState{
			Version: c.Version, Platform: runtime.GOOS, Config: c.Config.Get(), Repos: c.ListRepos(ctx),
			MCP: c.MCPStatus(), HeatMetrics: core.HeatMetrics, CanPickDir: a.PickDirectory != nil, GitFound: gitx.Available(),
		}, nil
	})
	a.methods["repos.list"] = method(func(ctx context.Context, _ struct{}) (any, error) { return c.ListRepos(ctx), nil })
	a.methods["repos.pick"] = method(func(ctx context.Context, _ struct{}) (any, error) {
		if a.PickDirectory == nil {
			return nil, fmt.Errorf("directory picker unavailable in browser mode")
		}
		p, err := a.PickDirectory()
		return map[string]string{"path": p}, err
	})
	a.methods["repos.add"] = method(func(ctx context.Context, in struct{ Path, Name string }) (any, error) {
		return c.AddRepo(in.Path, in.Name)
	})
	a.methods["repos.remove"] = method(func(ctx context.Context, in struct{ ID string }) (any, error) {
		return nil, c.RemoveRepo(in.ID)
	})
	a.methods["repos.update"] = method(func(ctx context.Context, in config.Repo) (any, error) {
		return nil, c.UpdateRepo(in)
	})
	a.methods["repos.setActive"] = method(func(ctx context.Context, in struct{ ID string }) (any, error) {
		return nil, c.SetActiveRepo(in.ID)
	})
	a.methods["settings.update"] = method(func(ctx context.Context, in config.Settings) (any, error) {
		return nil, c.UpdateSettings(in)
	})
	a.methods["activity.list"] = method(func(ctx context.Context, _ struct{}) (any, error) { return c.Activity(), nil })
	a.methods["ui.context"] = method(func(ctx context.Context, in core.UIContext) (any, error) {
		c.SetUIContext(in)
		return nil, nil
	})
}

// registerCode adds file, symbol, graph and heatmap methods.
func (a *API) registerCode() {
	c := a.core
	withRepo := func(ref string, fn func(rt *core.RepoHandle) (any, error)) (any, error) {
		rt, err := c.Repo(ref)
		if err != nil {
			return nil, err
		}
		return fn(rt)
	}
	a.methods["overview"] = method(func(ctx context.Context, in repoReq) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.Overview(ctx, rt) })
	})
	a.methods["index.rebuild"] = method(func(ctx context.Context, in repoReq) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			c.Reindex(rt)
			ix, err := c.Index(ctx, rt)
			if err != nil {
				return nil, err
			}
			return ix.Stats(), nil
		})
	})
	a.methods["fs.tree"] = method(func(ctx context.Context, in repoReq) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.Tree(rt) })
	})
	a.methods["fs.read"] = method(func(ctx context.Context, in struct{ Repo, Path string }) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.ReadFile(rt, in.Path) })
	})
	a.methods["fs.search"] = method(func(ctx context.Context, in struct {
		Repo string `json:"repo"`
		workspace.SearchOptions
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			m, more, err := c.Search(rt, in.SearchOptions)
			if m == nil {
				m = []workspace.SearchMatch{}
			}
			return map[string]any{"matches": m, "truncated": more}, err
		})
	})
	a.methods["sym.file"] = method(func(ctx context.Context, in struct{ Repo, File string }) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			syms, err := c.FileSymbols(ctx, rt, in.File)
			if syms == nil {
				syms = []core.FileSymbol{}
			}
			return syms, err
		})
	})
	a.methods["sym.summary"] = method(func(ctx context.Context, in struct{ Repo, File string }) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.FileSummary(ctx, rt, in.File) })
	})
	a.methods["sym.detail"] = method(func(ctx context.Context, in struct {
		Repo, Symbol string
		Code         bool
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.SymbolDetail(ctx, rt, in.Symbol, in.Code) })
	})
	a.methods["sym.search"] = method(func(ctx context.Context, in struct {
		Repo, Q string
		Limit   int
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			res, err := c.SearchSymbols(ctx, rt, in.Q, in.Limit)
			if res == nil {
				res = []*analysis.Symbol{}
			}
			return res, err
		})
	})
	a.methods["graph.call"] = method(func(ctx context.Context, in struct {
		Repo, Symbol string
		analysis.CallGraphOptions
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.CallGraph(ctx, rt, in.Symbol, in.CallGraphOptions) })
	})
	a.methods["graph.trace"] = method(func(ctx context.Context, in struct {
		Repo, Symbol string
		Depth        int
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.TraceTo(ctx, rt, in.Symbol, in.Depth) })
	})
	a.methods["graph.deps"] = method(func(ctx context.Context, in struct {
		Repo, Focus     string
		IncludeExternal bool
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.PackageGraph(ctx, rt, in.IncludeExternal, in.Focus) })
	})
	a.methods["graph.types"] = method(func(ctx context.Context, in struct{ Repo, Scope string }) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.TypeGraph(ctx, rt, in.Scope) })
	})
	a.methods["graph.flow"] = method(func(ctx context.Context, in struct{ Repo, Symbol string }) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			fl, sym, err := c.FunctionFlow(ctx, rt, in.Symbol)
			if err != nil {
				return nil, err
			}
			return map[string]any{"flow": fl, "symbol": sym}, nil
		})
	})
	a.methods["graph.impact"] = method(func(ctx context.Context, in struct{ Repo, Proposal string }) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.ProposalImpactGraph(ctx, rt, in.Proposal) })
	})
	a.methods["heatmap"] = method(func(ctx context.Context, in struct{ Repo, Metric, Proposal string }) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.Heatmap(ctx, rt, in.Metric, in.Proposal) })
	})
}

// registerGit adds diff, file-version, log, ref and status methods.
func (a *API) registerGit() {
	c := a.core
	withRepo := func(ref string, fn func(rt *core.RepoHandle) (any, error)) (any, error) {
		rt, err := c.Repo(ref)
		if err != nil {
			return nil, err
		}
		return fn(rt)
	}
	a.methods["git.diff"] = method(func(ctx context.Context, in struct {
		Repo   string        `json:"repo"`
		Spec   gitx.DiffSpec `json:"spec"`
		Impact bool          `json:"impact"`
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.Diff(ctx, rt, in.Spec, in.Impact) })
	})
	a.methods["git.versions"] = method(func(ctx context.Context, in struct {
		Repo    string        `json:"repo"`
		Path    string        `json:"path"`
		OldPath string        `json:"oldPath"`
		Spec    gitx.DiffSpec `json:"spec"`
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) { return c.FileVersions(ctx, rt, in.Path, in.OldPath, in.Spec) })
	})
	a.methods["git.log"] = method(func(ctx context.Context, in struct {
		Repo, Ref, Path string
		Limit           int
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			commits, err := c.GitLog(ctx, rt, in.Limit, in.Ref, in.Path)
			if commits == nil {
				commits = []gitx.Commit{}
			}
			return commits, err
		})
	})
	a.methods["git.refs"] = method(func(ctx context.Context, in repoReq) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			refs, err := rt.Git().Refs(ctx)
			if refs == nil {
				refs = []gitx.Ref{}
			}
			return refs, err
		})
	})
	a.methods["git.status"] = method(func(ctx context.Context, in repoReq) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			info, err := rt.Git().Info(ctx)
			if err != nil || !info.IsRepo {
				return map[string]any{"info": info, "entries": []gitx.StatusEntry{}}, err
			}
			entries, err := rt.Git().Status(ctx)
			out := []gitx.StatusEntry{}
			for _, e := range entries {
				if !rt.WS().Excluded(e.Path, config.DefaultExcludes) {
					out = append(out, e)
				}
			}
			return map[string]any{"info": info, "entries": out}, err
		})
	})
}

// registerSession adds proposal, walkthrough, diagram, explanation and
// annotation methods.
func (a *API) registerSession() {
	c := a.core
	withRepo := func(ref string, fn func(rt *core.RepoHandle) (any, error)) (any, error) {
		rt, err := c.Repo(ref)
		if err != nil {
			return nil, err
		}
		return fn(rt)
	}
	a.methods["proposals.list"] = method(func(ctx context.Context, in repoReq) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			list := c.Proposals.List(rt.ID())
			if list == nil {
				return []any{}, nil
			}
			return list, nil
		})
	})
	a.methods["proposals.resolve"] = method(func(ctx context.Context, in struct {
		Repo     string   `json:"repo"`
		ID       string   `json:"id"`
		Paths    []string `json:"paths"`
		Accept   bool     `json:"accept"`
		Feedback string   `json:"feedback"`
	}) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			return c.ResolveProposal(rt, in.ID, in.Paths, in.Accept, in.Feedback)
		})
	})
	a.methods["proposals.dismiss"] = method(func(ctx context.Context, in struct{ ID string }) (any, error) {
		c.Proposals.Remove(in.ID)
		return nil, nil
	})
	a.methods["session.all"] = method(func(ctx context.Context, in repoReq) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			return map[string]any{
				"walkthroughs": nonNil(c.Walkthroughs(rt.ID())),
				"diagrams":     nonNil(c.Diagrams(rt.ID())),
				"explanations": nonNil(c.Explanations(rt.ID())),
				"annotations":  c.Annotations(rt.ID()),
			}, nil
		})
	})
	a.methods["annotations.clear"] = method(func(ctx context.Context, in repoReq) (any, error) {
		return withRepo(in.Repo, func(rt *core.RepoHandle) (any, error) {
			c.ClearAnnotations(rt.ID())
			return nil, nil
		})
	})
}

// nonNil returns s, or an empty slice instead of nil so it encodes as [].
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
