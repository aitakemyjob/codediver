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
- Repository switcher and theme menu dropdowns were clipped by the title bar.
- The folder picker's Browse button was missing in the Linux desktop app.
- Release candidates are packaged so they sort before the final release
  (`0.1.0~rc.2` < `0.1.0`).

## [0.1.0] — 2026-10-16

First public release: the desktop app with its built-in MCP server,
walkthroughs, function logic, trace-to-here, call and dependency graphs,
heatmaps, visual review of agent proposals, ten themes, and installers for
macOS, Windows and Linux plus a headless server.

[Unreleased]: https://github.com/aitakemyjob/codediver/compare/v0.1.1-rc.1...HEAD
[0.1.1]: https://github.com/aitakemyjob/codediver/compare/v0.1.0...v0.1.1-rc.1
[0.1.0]: https://github.com/aitakemyjob/codediver/releases/tag/v0.1.0
