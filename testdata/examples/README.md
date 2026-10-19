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

## Try one

```sh
make example NAME=go-service      # or: scripts/example.sh go-service [dest]
```

This copies the example into a scratch git repository with two commits and
one uncommitted change (lowering the order limit in `validate`), then
prints how to open it in CodeDiver. What to look at:

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

The same domain in every language, so results compare side by side:

| Piece | Go | TypeScript | Python | Shows |
|---|---|---|---|---|
| validate | `order.Validate` | `validate` | `validate` | partly covered: only the zero-quantity branch is tested |
| total → unit price | `Total` → `unitPrice` | `total` → `unitPrice` | `total` → `_unit_price` | reached only through a caller (indirect tests) |
| legacy total | `LegacyTotal` | `legacyTotal` | `legacy_total` | exported, never called, never tested |
| notifier | `notify.Send` | `api.createOrder` | `notify.send` | hard to test: clock, environment, network |
| handler | `Handler.CreateOrder` | `OrderForm` | `create_order` | the direct caller of validate (blast radius) |
| store | `store.Memory` | (none) | `MemoryStore` | tested directly |

## Support matrix

What CodeDiver finds in these examples today (pinned by the tests):

| | Go | TypeScript / React | Python |
|---|---|---|---|
| Functions and methods | ✓ | ✓ | ✓ |
| Tests discovered | ✓ | ✓ (describe/it) | ✓ (functions and classes) |
| Test → code mapping | ✓ | ✓ | ✓ |
| Call edges | ✓ type-aware | name matching (`~`) | name matching (`~`) |
| JSX use (`<OrderSummary />`) as a call | n/a | ✓ (`component={Foo}` and `memo(Foo)` as references) | n/a |
| Calls inside components with destructured props | n/a | ✓ | n/a |
| Calls from module-level code (`render(<App />)` in `main.tsx`) | n/a | ✗: not inside a function, so `App` has no caller | ✗ |
| Same method name on unrelated types | ✓ type-aware | name matching | built-in names (`get`, `append`…) only link when the receiver looks like the class (`store.get` → `MemoryStore.get`, not `data.get`) |
| Coverage import | ✓ | ✓ | ✓ |
| Testability hints | ✓ | ✓ | ✓ |

## Regenerating coverage

The reports are committed so the QA view works without the toolchains.
After changing an example, regenerate its report from its folder:

| Example | Command |
|---|---|
| go-service | `go test -coverprofile=coverage.out -coverpkg=./... ./...` |
| ts-react | `npm install && npm run coverage` (don't commit `node_modules`) |
| python-api | `pip install -r requirements-dev.txt && python -m pytest --cov=orders --cov-report=lcov:coverage.lcov` |

Then run `go test ./internal/core -run Example` from the repository root.
If the demo change or the history step moved, regenerate the patches in
`<example>/.demo/`. `history.patch` adds the over-1000 check and
`change.patch` lowers it to 500.

These folders live under `testdata/`, so Go tooling ignores them. When you
open the CodeDiver repository itself, CodeDiver doesn't detect their suites
or count their tests as its own.
