# Contributing

Thanks for helping improve CodeDiver! By participating you agree to the
[Code of Conduct](CODE_OF_CONDUCT.md), and contributions are licensed under
the [MIT License](LICENSE).

## Workflow

1. For anything non-trivial, open or comment on an issue first so we can
   agree on the approach.
2. Fork, create a branch (`feat/...`, `fix/...`, `docs/...`), and keep PRs
   focused.
3. Run `make test` and `make lint` before pushing. CI must be green and a
   maintainer review is required before merging into `main`; PRs are
   squash-merged.
4. Sign off every commit (`git commit -s`). This adds a `Signed-off-by:` line
   certifying, under the [Developer Certificate of Origin](https://developercert.org),
   that you have the right to submit the change. The DCO check fails without
   it; fix existing commits with `git rebase --signoff main`.
5. Use clear commit/PR titles, e.g. `fix: trace graph misses method values`.
   Labels on the PR decide where it appears in the release notes; area labels
   are added automatically from the files you change.

## Setup

```sh
make tools deps   # Wails CLI, golangci-lint, govulncheck and frontend packages
make test         # everything CI runs (Go with -race, frontend typecheck + tests)
make dev          # desktop app with hot reload
```

Without WebKitGTK headers on Linux you can still develop everything: build the
headless binary (`make headless`), run `codediver serve`, and run the Vite dev
server (`cd frontend && npm run dev`); it proxies `/api` and `/mcp` to
`127.0.0.1:47474`.

## Conventions

- **Go:** `make lint` clean (golangci-lint, configured in `.golangci.yml`). Business logic belongs in
  `internal/core`; `api` and `mcpserver` stay thin. All file access goes
  through `workspace`. Prefer table-driven tests; integration tests build a
  throwaway repo with `internal/testutil`.
- **TypeScript:** strict mode, `npm run lint` (ESLint) clean with no warnings. Keep pure logic in `src/lib`
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

Move the *Unreleased* entries in `CHANGELOG.md` under the new version, then
run `make release VERSION=vX.Y.Z` from a clean `main`. It runs the tests,
tags and pushes; the Release workflow builds every platform with
`scripts/build.sh` / `scripts/build.ps1` and publishes the GitHub release.
See the README's *Releasing* section for signing secrets.
