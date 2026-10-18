# Changelog

Notable changes to CodeDiver. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org). Each
[GitHub release](https://github.com/aitakemyjob/codediver/releases) also lists
every merged pull request.

## [Unreleased]

### Added
- Linting in CI (golangci-lint, ESLint), dependency vulnerability checks
  (govulncheck, npm audit), CodeQL and OpenSSF Scorecard.
- Release builds ship an SPDX SBOM and signed build provenance; verify a
  download with `gh attestation verify <file> --repo aitakemyjob/codediver`.
- Roadmap, release-note categories, automatic PR labels and DCO sign-off.

## [0.1.1] — unreleased (0.1.1-rc.1 available)

### Added
- **QA view:** line coverage from Go, LCOV, Cobertura and JaCoCo reports or
  from coverage suites you run in the app; tests shown as readable behaviors;
  functions ranked by test risk with testability hints; coverage in the editor
  gutter; an "uncovered" heatmap.
- MCP tools `get_test_overview`, `get_coverage`, `get_tests_for`,
  `find_test_gaps` and `show_test_plan`.

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

[Unreleased]: https://github.com/aitakemyjob/codediver/compare/v0.1.1-rc.1...HEAD
[0.1.1]: https://github.com/aitakemyjob/codediver/compare/v0.1.0...v0.1.1-rc.1
[0.1.0]: https://github.com/aitakemyjob/codediver/releases/tag/v0.1.0
