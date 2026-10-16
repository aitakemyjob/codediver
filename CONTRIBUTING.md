# Contributing

## Setup

```sh
make tools deps   # Wails CLI and frontend packages
make test         # everything CI runs (Go with -race, frontend typecheck + tests)
make dev          # desktop app with hot reload
```

Without WebKitGTK headers on Linux you can still develop everything: build the
headless binary (`make headless`), run `codediver serve`, and run the Vite dev
server (`cd frontend && npm run dev`); it proxies `/api` and `/mcp` to
`127.0.0.1:47474`.

## Conventions

- **Go:** `gofmt`, `go vet -tags nogui ./...` clean. Business logic belongs in
  `internal/core`; `api` and `mcpserver` stay thin. All file access goes
  through `workspace`. Prefer table-driven tests; integration tests build a
  throwaway repo with `internal/testutil`.
- **TypeScript:** strict mode, no unused locals. Keep pure logic in `src/lib`
  with `*.test.ts` beside it; views consume the store and `api`.
- **Documentation:** every Go function and method has a doc comment that
  starts with its name (`docs_test.go` enforces this in CI), and every
  TypeScript function, component and API/store method has a JSDoc comment.
- **JSON contract:** Go structs in `core`/`analysis`/`gitx` are mirrored in
  `frontend/src/lib/types.ts`. Change both together, and return empty slices
  rather than `nil` for lists the UI iterates.
- **MCP tools:** every tool needs a description written for an LLM, a test in
  `internal/mcpserver/mcpserver_test.go` and an entry in `docs/MCP.md`.
- **Monaco:** after upgrading `monaco-editor`, run `npm install` (it
  regenerates `src/lib/monaco.generated.ts`) and commit the result.

## Branding

The source logo is `cd_logo_1.png` at the repo root. After changing it, run
`make icon` to regenerate `build/appicon.png` (desktop icons and Linux
packages), `frontend/public/logo.png`, `frontend/public/favicon.png` and
`docs/assets/logo.png` (README). The
tool removes a baked-in checkerboard background automatically.

## Releases

Run `make release VERSION=vX.Y.Z` from a clean `main`. It runs the tests,
tags and pushes; the Release workflow builds every platform with
`scripts/build.sh` / `scripts/build.ps1` and publishes the GitHub release.
See the README's *Releasing* section for signing secrets.
