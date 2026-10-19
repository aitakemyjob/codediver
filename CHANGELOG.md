# Changelog

Notable changes to CodeDiver. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org). Each
[GitHub release](https://github.com/aitakemyjob/codediver/releases) also lists
every merged pull request.

## [Unreleased]

### Added
- **Demos:** agents can play animated, step-by-step demos with
  `show_demo` — architecture diagrams with nested containers, icons,
  regions and color-coded paths, or execution trees seeded from the call
  graph — with packets moving along routed edges, values on each box,
  narration and code. Zoom, pan and a camera that follows the action.
- **Animations:** `show_animation` plays an agent-written SVG or HTML
  animation in a sandbox with no network access; save it as HTML.
- **Export:** any demo step as PNG or SVG, the whole demo as a PDF.
  Agents can render steps themselves with `export_demo` (PNG returned as
  an image, or SVG text).
- When an agent shows a demo, animation, walkthrough, diagram or
  explanation for a repository you aren't viewing, a toast offers to
  switch to it.

### Fixed
- **TypeScript / JavaScript:** a function with destructured parameters
  (`function Card({ title }: Props)`) or an object return type was taken
  to end on its first line, so calls in its body were missed. Arrow
  functions that return JSX are measured to their closing parenthesis.
  On CodeDiver's own React front end this finds almost three times as
  many call edges.
- **React:** rendering a component (`<OrderSummary />`, `<ui.Row>`) now
  counts as calling it, and passing one (`component={Page}`,
  `memo(Page)`) as a reference, so components are no longer reported as
  unused. Calls in a nested function count for it, not also for the
  function around it.
- **TypeScript / JavaScript:** calls inside template literals
  (`` `${dollars(cents)}` ``) were ignored along with the string text.
- Testability hints now flag `smtplib`, `net/smtp`, sockets, `httpx`,
  `aiohttp`, WebSockets and `XMLHttpRequest` as network access.
- **Python / JavaScript:** calls like `data.get(...)` or `items.push(...)`
  on a variable of unknown type no longer link to an unrelated class's
  method of the same name; `store.get(...)` still links to
  `MemoryStore.get`.
- A repository with a top-level `testdata/` folder no longer gets coverage
  suites or tests detected from the fixtures inside it.
- Opening a file with no covered lines (from the Uncovered heatmap, say)
  showed a view error ("null is not an object") instead of the code.
- The install scripts now print the Claude Code connection command with
  `--scope user`, matching the app, so CodeDiver is available in every
  project.
- **Sandbox:** on macOS and Windows, agents could read files hidden from
  them by using another spelling of the name: different case (`.ENV`,
  `.GIT/config`), a trailing dot or space, or a Windows data stream
  (`.env::$DATA`). Exclude rules now match all of those on every OS, and
  a rule like `.env` also hides everything inside a `.env` directory.
- Release builds and scanners now agree on Go 1.26.9 as the minimum, and
  vulnerable indirect dependencies were updated (none were reachable).
- Arrow keys in a walkthrough could throw an error when the key event
  didn't come from a page element.

## [0.1.1] — 2026-10-17

### Added
- **QA view:** line coverage from Go, LCOV, Cobertura and JaCoCo reports or
  from coverage suites you run in the app; tests shown as readable behaviors;
  functions ranked by test risk with testability hints; coverage in the editor
  gutter; an "uncovered" heatmap.
- MCP tools `get_test_overview`, `get_coverage`, `get_tests_for`,
  `find_test_gaps` and `show_test_plan`.
- Linting in CI (golangci-lint, ESLint), dependency vulnerability checks
  (govulncheck, npm audit), CodeQL and OpenSSF Scorecard.
- Release builds ship an SPDX SBOM and signed build provenance; verify a
  download with `gh attestation verify <file> --repo aitakemyjob/codediver`.
- Roadmap, release-note categories, automatic PR labels and DCO sign-off.

### Fixed
- Clicking the first commit of a repository in History failed with "unknown
  revision"; it now shows everything that commit added. The per-file History
  view shows errors instead of spinning forever. Agents can request a single
  commit's changes with `git_diff`'s new `commit` option.
- Repository switcher and theme menu dropdowns were clipped by the title bar.
- The folder picker's Browse button was missing in the Linux desktop app.
- Release candidates are packaged so they sort before the final release
  (`0.1.0~rc.2` < `0.1.0`).
- The Claude Code connection command now uses `--scope user`, so CodeDiver is
  available in every project instead of only the folder where it was added.
- Quitting the desktop app could hang while an agent was connected over MCP
  (or the browser UI was open), because the server waited for streaming
  connections that never finish. Shutdown now ends them immediately, and
  coverage runs in progress are stopped so no test processes are left behind.
- A coverage suite's Folder entered as an absolute path was saved as a
  nonexistent relative folder, and the run failed with a misleading
  "fork/exec /usr/bin/sh: no such file or directory". Folders are now
  normalized (absolute paths inside the repo, `./`, trailing slashes),
  checked when saved, and checked again before each run.
- Test names with mixed-case words read correctly in the QA view's Behaviors
  tab: `TestParseJaCoCo` shows as "Parse JaCoCo" instead of "Parse ja co co"
  (also GitHub, OAuth, TypeScript, iOS and others).

## [0.1.0] — 2026-10-16

First public release: the desktop app with its built-in MCP server,
walkthroughs, function logic, trace-to-here, call and dependency graphs,
heatmaps, visual review of agent proposals, ten themes, and installers for
macOS, Windows and Linux plus a headless server.

[Unreleased]: https://github.com/aitakemyjob/codediver/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/aitakemyjob/codediver/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/aitakemyjob/codediver/releases/tag/v0.1.0
