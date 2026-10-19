# polyglot-monorepo

One repository, three components in three languages, each with its own
tests and coverage report:

| Component | Language | Tests | Coverage |
|---|---|---|---|
| `services/billing` | Go | go test | `services/billing/coverage.out` |
| `web` | TypeScript | Vitest | `web/coverage/lcov.info` |
| `jobs` | Python | pytest | `jobs/coverage.lcov` |

CodeDiver detects one suite per component and merges their coverage. Use it
to see how the QA view, heatmaps and the blast radius behave across
components, and for monorepo work (#7). See [../README.md](../README.md).

Regenerate coverage from each component's folder: `go test -coverprofile=coverage.out -coverpkg=./... ./...`,
`npm install && npm run coverage`, and
`python -m pytest --cov=reminders --cov-report=lcov:coverage.lcov`.
