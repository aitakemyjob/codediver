# Example projects

One small orders service per language, each with tests, a committed
coverage report and gaps put there on purpose. Use them to try CodeDiver's
features, to demo them, and as fixtures: `internal/core/examples_test.go`
checks what CodeDiver finds in each, so the table below stays true.

| Example | Language | Tests | Coverage report |
|---|---|---|---|
| [`go-service`](go-service) | Go | go test | `coverage.out` (coverprofile) |
| [`ts-react`](ts-react) | TypeScript + React | Vitest | `coverage/lcov.info` (LCOV) |
| [`python-api`](python-api) | Python | pytest | `coverage.lcov` (LCOV) |
| [`java-maven`](java-maven) | Java (Maven) | JUnit 5 | `target/site/jacoco/jacoco.xml` (JaCoCo) |
| [`polyglot-monorepo`](polyglot-monorepo) | Go + TypeScript + Python | go test, Vitest, pytest | one report per component |

## Try one

```sh
make example NAME=go-service      # or: scripts/example.sh go-service [dest]
```

On Windows: `scripts\example.ps1 go-service [dest]`.

This copies the example into a scratch git repository with two commits and
one uncommitted change, then prints how to open it in CodeDiver. In the
orders examples the change lowers the order limit in `validate`. In the
monorepo it changes the Go tax rate and a TypeScript label, so the review
spans two components. What to look at:

- **QA view:** coverage per file, the riskiest gaps, and testability hints
  on the notifier, which reads the clock, the environment and the network.
- **Review (diff) view:** the pending change, with **Blast radius** (the
  handler that calls `validate`) and **Tests** (which tests reach it).
- **History** on the order file: the commit that added the limit.
- An agent connected over MCP can call `find_test_gaps`, `git_diff`,
  `get_tests_for`, or `propose_changes` against it.

You can also add an example folder directly in CodeDiver. You'll get
everything except the git views.

## What every example contains

The orders examples implement the same domain in every language, so
results compare side by side:

| Piece | Go | TypeScript | Python | Java | Shows |
|---|---|---|---|---|---|
| validate | `order.Validate` | `validate` | `validate` | `Orders.validate` | partly covered: only the zero-quantity branch is tested |
| total → unit price | `Total` → `unitPrice` | `total` → `unitPrice` | `total` → `_unit_price` | `Orders.total` → `unitPrice` | reached only through a caller (indirect tests) |
| legacy total | `LegacyTotal` | `legacyTotal` | `legacy_total` | `Orders.legacyTotal` | exported, never called, never tested |
| notifier | `notify.Send` | `api.createOrder` | `notify.send` | `Notifier.send` | hard to test: clock, environment, network |
| handler | `Handler.CreateOrder` | `OrderForm` | `create_order` | `OrderHandler.createOrder` | the direct caller of validate (blast radius) |
| store | `store.Memory` | (none) | `MemoryStore` | `MemoryStore` | tested directly |

`polyglot-monorepo` is a billing system instead: a Go service
(`services/billing`), a TypeScript web app (`web`) and Python jobs
(`jobs`), each with the same kinds of gaps.

## Support matrix

What CodeDiver finds in these examples today (pinned by the tests):

| | Go | TypeScript / React | Python | Java |
|---|---|---|---|---|
| Functions and methods | ✓ | ✓ | ✓ | ✓ |
| Tests discovered | ✓ | ✓ (describe/it) | ✓ (functions and classes) | ✓ (JUnit 5, nested classes) |
| Test → code mapping | ✓ | ✓ | ✓ | ✓ (tests in the mirrored `src/test` package) |
| Call edges | ✓ type-aware | name matching (`~`) | name matching (`~`) | name matching (`~`) |
| JSX use (`<OrderSummary />`) as a call | n/a | ✓ (`component={Foo}` and `memo(Foo)` as references) | n/a | n/a |
| Calls inside components with destructured props | n/a | ✓ | n/a | n/a |
| Calls inside template literals (`` `${f(x)}` ``) | n/a | ✓ | n/a | n/a |
| Calls from module-level code (`render(<App />)` in `main.tsx`) | n/a | ✗: not inside a function, so `App` has no caller | ✗ | n/a |
| Same method name on unrelated types | ✓ type-aware | built-in names (`get`, `push`…) only link when the receiver looks like the class | same as TypeScript (`store.get` → `MemoryStore.get`, not `data.get`) | name matching |
| Coverage import | ✓ coverprofile | ✓ LCOV | ✓ LCOV | ✓ JaCoCo |
| Testability hints | ✓ | ✓ | ✓ | ✓ |
| One suite and report per component in a monorepo | ✓ | ✓ | ✓ | n/a |

## Regenerating coverage

The reports are committed so the QA view works without the toolchains.
After changing an example, regenerate its report from its folder:

| Example | Command |
|---|---|
| go-service | `go test -coverprofile=coverage.out -coverpkg=./... ./...` |
| ts-react | `npm install && npm run coverage` (don't commit `node_modules`) |
| python-api | `pip install -r requirements-dev.txt && python -m pytest --cov=orders --cov-report=lcov:coverage.lcov` |
| java-maven | `mvn -q org.jacoco:jacoco-maven-plugin:prepare-agent test org.jacoco:jacoco-maven-plugin:report` (commit only `target/site/jacoco/jacoco.xml`) |
| polyglot-monorepo | each component as above, from its own folder (see its README) |

Then run `go test ./internal/core -run Example` from the repository root.
If the demo change or the history step moved, regenerate the patches in
`<example>/.demo/`: `history.patch` is the second commit, `change.patch`
the uncommitted change, and an optional `commits` file names the two
commits.

These folders live under `testdata/`, so Go tooling ignores them. When you
open the CodeDiver repository itself, CodeDiver doesn't detect their suites
or count their tests as its own.
