# Roadmap

Where CodeDiver is heading. Each line has a
[milestone](https://github.com/aitakemyjob/codediver/milestones), and issues
labeled [`ready`](https://github.com/aitakemyjob/codediver/issues?q=is%3Aopen+label%3Aready)
are open for contributors. Bug fixes land in every release.

Plans change; if something here matters to you, say so in its issue or in
[Discussions](https://github.com/aitakemyjob/codediver/discussions).

## 0.2 — Quality and deeper code flow

- **QA view:** coverage, test behaviors, gaps and agent test plans (shipped in
  0.1.1), plus follow-ups such as more accurate test-to-code mapping.
- **Code flow for more languages:** control-flow graphs and precise call
  tracing for TypeScript/JavaScript and Python first, then Java/Kotlin, C# and
  Rust ([#18](https://github.com/aitakemyjob/codediver/issues/18)), with
  documented precision per language ([#8](https://github.com/aitakemyjob/codediver/issues/8)).

## 0.3 — Monorepos, and agents that configure CodeDiver

- **Monorepo components** with independent coverage, targets and dependency
  views ([#7](https://github.com/aitakemyjob/codediver/issues/7)).
- **Agent-driven configuration over MCP:** components, coverage settings and
  targets ([#12](https://github.com/aitakemyjob/codediver/issues/12)), where
  anything that runs a command or loosens a check needs the developer's approval.
- **Watch areas:** flag changes to high-risk code for review
  ([#13](https://github.com/aitakemyjob/codediver/issues/13)).

## 0.4 — Where developers work

- **VS Code integration:** editor context sync, in-editor views and proposal
  review ([#10](https://github.com/aitakemyjob/codediver/issues/10)).
- **Signed and notarized macOS app** ([#9](https://github.com/aitakemyjob/codediver/issues/9)).

## 1.0 — Teams

- **Teams support through the cloud**, still to be defined
  ([#21](https://github.com/aitakemyjob/codediver/issues/21)). CodeDiver stays
  local-first: anything that leaves your machine will be opt-in and explicit.
