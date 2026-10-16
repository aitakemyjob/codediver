# Architecture

CodeDiver is one Go binary. The desktop shell (Wails), the browser mode and the
MCP server are thin adapters over a single application service, so the UI and
agents see exactly the same data through the same sandbox.

```
            ┌──────────────── frontend (React + TS) ────────────────┐
            │ views: code · graph · flow · heatmap · diff · qa · …   │
            │ lib/transport.ts ── Wails bindings │ HTTP + SSE        │
            └───────────────┬────────────────────┴─────────┬─────────┘
                            │ Invoke(method, json)          │
   gui.go (Wails) ──────────┤                               │
   internal/server ─────────┴─ /api/invoke · /api/events ───┘
                            │
                     internal/api  (JSON method table)        internal/mcpserver (MCP tools)
                            └──────────────┬──────────────────────────┘
                                     internal/core  (Service)
         ┌──────────────┬──────────────┬───┴──────────┬──────────────┬─────────────┐
   workspace (sandbox)  gitx (git CLI)  analysis       proposals      events (bus)  config  qa
```

## Packages

| Package | Responsibility |
|---|---|
| `internal/config` | Persisted settings and the repository list (`os.UserConfigDir()/codediver/config.json`, override with `CODEDIVER_CONFIG`). |
| `internal/workspace` | **The sandbox.** Resolves repo-relative paths, rejects `..`/absolute/symlink escapes and excluded globs, lists files via `git ls-files` (honoring `.gitignore`), reads, searches, writes. Every file access goes through it. |
| `internal/gitx` | Wrapper over the `git` CLI: status, structured diffs (pure `ParseUnifiedDiff`), log with numstat, churn, refs, `show`. Refs are validated so they can't be smuggled in as options. |
| `internal/analysis` | Builds an immutable `Index` of symbols and edges. `golang.go` type-checks packages with `go/types`, stubbing external imports so intra-repo resolution stays exact without a Go toolchain; it records calls, `go`/`defer`, function values (`ref`), interface `dispatch` and `implements`. `generic.go` handles other languages lexically over comment/string-masked source. `cfg.go` builds per-function control-flow graphs. `index.go` holds graph queries: call graph, trace-to, impact, packages, types. |
| `internal/qa` | Test intelligence with no execution of its own: coverage parsers (Go coverprofile, LCOV, Cobertura, JaCoCo) with a `Resolver` that maps report paths onto visible repo files, test discovery and behavior naming for Go, JS/TS, Python, JVM, .NET, Rust, Ruby, Swift, PHP, C++ and Dart, call extraction from test bodies, testability hints (clock, network, filesystem, env, processes, sleeps, exits…), suite detection, and the cancellable `Run` used for developer-started coverage runs. |
| `internal/textdiff` | Myers line diff producing `gitx.Hunk`s, used for proposals. |
| `internal/proposals` | Proposal model, search/replace edit application with agent-friendly errors, and conflict-safe accept/reject. |
| `internal/events` | Non-blocking fan-out bus that carries focus requests, activity, proposals, etc. to UIs. |
| `internal/core` | The `Service`: repo handles, analysis cache (rebuilt when code files' size/mtime fingerprint changes), heatmaps, diff → symbol mapping, sessions (walkthroughs, diagrams, explanations, annotations, developer context), QA (coverage per suite, saved under `os.UserCacheDir()/codediver`, test → code mapping over the index, risk-ranked gaps, coverage runs, agent test plans) and the agent access guard `AgentRepo`. |
| `internal/api` | The frontend's JSON method table; one entry point `Invoke(method, args)`. |
| `internal/mcpserver` | MCP tools over `core`, rendered as compact text for agents; every call is logged to the activity feed. |
| `internal/server` | Loopback HTTP: `/mcp` (streamable HTTP), `/healthz`, and in browser mode the UI, `/api/invoke` and `/api/events` (SSE). |
| `main.go`, `gui.go`, `bridge.go` | CLI, Wails desktop shell (`!nogui`), stdio↔HTTP MCP bridge. |

## Design principles

- **One service, many adapters.** Business logic lives in `core`; the UI API
  and MCP tools only translate. Behavior and sandboxing can't drift apart.
- **The sandbox is the only door.** No package below `core` reads repository
  files except through `workspace`, and agent entry points go through
  `AgentRepo`, which enforces the global and per-repo switches.
- **Humans own writes.** Agents can only create proposals; applying one is a
  UI action that re-hashes the file first.
- **Humans own execution.** Nothing reachable from MCP runs a command. Coverage
  suites are configured and started only through the UI API (`qa.run`); their
  output goes to a scratch directory outside the repository.
- **Pure where possible.** Diff parsing, Myers diff, CFG building, graph
  queries, treemap layout and tab state are pure functions with unit tests.
- **Headless-testable.** `-tags nogui` builds everything except the webview,
  so CI and `codediver serve` run anywhere.

## Frontend

React 19 + TypeScript + Vite. State lives in a zustand store (`lib/store.ts`);
tabs are pure functions (`lib/tabs.ts`). Monaco is loaded through a generated
lean entry (`scripts/gen-monaco.mjs`) with only Monarch grammars and editor
contributions. Graphs use React Flow with ELK layered layout; Mermaid renders
agent diagrams; the heatmap is a d3-hierarchy squarified treemap. Themes are
palettes that drive CSS variables, the Monaco theme, Mermaid variables and the
heat scale together (`lib/themes.ts`).

Agent "focus" events arrive over the bus; when **Follow agent** is on, the
store converts them into tabs (`focusToTab`). The UI reports the developer's
cursor/selection back through `ui.context` so `get_developer_context` works.

## Adding things

- **An MCP tool:** add an input struct and `addTool` call in
  `internal/mcpserver/tools.go` (testing tools live in `qa_tools.go`), implement logic in `core`, list it in
  `docs/MCP.md` (a test enforces this) and cover it in `mcpserver_test.go`.
- **A language:** add a `langSpec` in `internal/analysis/generic.go` and its
  extension in `workspace.langByExt`, with a fixture test.
- **A heat metric:** add it to `core.HeatMetrics` and a case in
  `Service.Heatmap`.
- **A coverage format:** detect it in `qa.Detect` and add a parser that
  returns a `qa.Raw` (raw paths are resolved for you), with a test in
  `internal/qa/qa_test.go`.
- **A test framework:** extend `qa.IsTestFile` and `qa.Discover` (and
  `qa.DetectSuites` if it has a standard coverage command), with a
  `TestDiscover…` case.
- **A theme:** append to `themes` in `frontend/src/lib/themes.ts`; the theme
  test validates the palette.
