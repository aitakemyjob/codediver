# Example projects

One small orders service per language, each with tests, a committed
coverage report and gaps put there on purpose. Use them to try CodeDiver's
features, to demo them, and as fixtures: `internal/core/examples_test.go`
checks what CodeDiver finds in each, so the table below stays true.

Browsing an example needs no toolchain: its coverage report is committed.
To run its tests from CodeDiver (**Run coverage**) or regenerate the
report, install its toolchain first: see
[Installing the toolchains](TOOLCHAINS.md).

| Example | Language | Tests | Coverage report |
|---|---|---|---|
| [`go-service`](go-service) | Go | go test | `coverage.out` (coverprofile) |
| [`ts-react`](ts-react) | TypeScript + React | Vitest | `coverage/lcov.info` (LCOV) |
| [`python-api`](python-api) | Python | pytest | `coverage.lcov` (LCOV) |
| [`java-maven`](java-maven) | Java (Maven) | JUnit 5 | `target/site/jacoco/jacoco.xml` (JaCoCo) |
| [`rust-cargo`](rust-cargo) | Rust (Cargo) | `cargo test` (`#[cfg(test)]` modules) | `lcov.info` (LCOV, cargo-llvm-cov) |
| [`dotnet-xunit`](dotnet-xunit) | C# (.NET) | xUnit | `coverage/coverage.cobertura.xml` (Cobertura, coverlet) |
| [`ruby-rspec`](ruby-rspec) | Ruby | RSpec | `coverage/lcov.info` (LCOV, SimpleCov) |
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

| Piece | Rust | C# | Ruby |
|---|---|---|---|
| validate | `order::validate` | `Orders.Validate` | `Orders.validate` |
| total → unit price | `total` → `unit_price` | `Orders.Total` → `UnitPrice` | `Orders.total` → `unit_price` |
| legacy total | `legacy_total` | `Orders.LegacyTotal` | `Orders.legacy_total` |
| notifier | `notify::send` | `Notifier.SendAsync` | `Notifier.deliver` |
| handler | `Handler::create_order` | `OrderHandler.CreateOrderAsync` | `OrderHandler#create_order` |
| store | `MemoryStore` | `MemoryStore` | `MemoryStore` |

`polyglot-monorepo` is a billing system instead: a Go service
(`services/billing`), a TypeScript web app (`web`) and Python jobs
(`jobs`), each with the same kinds of gaps.

## Support matrix

What CodeDiver finds in these examples today (pinned by the tests). For
every language and the known gaps, see
[Language support](../../docs/LANGUAGES.md).

| | Go | TypeScript / React | Python | Java |
|---|---|---|---|---|
| Functions and methods | ✓ | ✓ | ✓ | ✓ |
| Tests discovered | ✓ | ✓ (describe/it) | ✓ (functions and classes) | ✓ (JUnit 5, nested classes) |
| Test → code mapping | ✓ | ✓ | ✓ | ✓ (tests in the mirrored `src/test` package) |
| Call edges | ✓ type-aware | through imports, else name matching (`~`) | through imports, else name matching (`~`) | name matching (`~`) |
| JSX use (`<OrderSummary />`) as a call | n/a | ✓ (`component={Foo}` and `memo(Foo)` as references) | n/a | n/a |
| Calls inside components with destructured props | n/a | ✓ | n/a | n/a |
| Calls inside template literals (`` `${f(x)}` ``) | n/a | ✓ | n/a | n/a |
| Constructors (`new Cart()` → `Cart.constructor`, also from tests) | n/a | ✓ | n/a | ✓ (constructors named after the class) |
| Methods on one line, class fields holding arrows (`clear = () => …`) | n/a | ✓ | n/a | ✓ (one-line methods) |
| Object-literal methods (`ordersApi.create`) and anonymous default exports (`default`) | n/a | ✓ | n/a | n/a |
| `super.subtotal()` and `cart?.subtotal()` | n/a | ✓ (`super` goes to the `extends` class) | n/a | n/a |
| Multi-line `import {…} from` as a dependency | n/a | ✓ | n/a | n/a |
| Calls through imports: two `format`s told apart (`receipt/summary.ts` uses the cart's, not the one beside it), `total as orderTotal`, `import * as receipt` through a barrel (`formatLine`, `export *`), a default import as `emit` | ✓ type-aware | ✓ (`ReceiptPage`, `cartLine`, `grandTotal`) | ✓ `orders/receipt`: `heading` uses `text.py`'s `format`, not the one beside it; `format as dollars`; `import orders.order as pricing`; `from . import receipt` with names re-exported by `__init__.py` (`line`, `from .totals import *`) | n/a |
| Calls from module-level code (`render(<App />)` in `main.tsx`) | ✓ `var ErrX = errors.New(…)`: the file's `<module>` calls it | ✓ `main.tsx <module>` calls `App` | n/a (none in the example) | n/a |
| Control flow: closures as their own flow (`sort.Slice` in `ListOrders`), `goto` (`notify.Send`'s retry); dispatch through a generic interface (`store.Repo[order.Order]` → `Memory.Get`) | ✓ | n/a (Go only) | n/a (Go only) | n/a (Go only) |
| Same method name on unrelated types | ✓ type-aware | through the receiver's written-down class (`giftSummary(cart: GiftCart)` and `Checkout`'s `private cart: GiftCart` reach `GiftCart.subtotal`, not `Cart.subtotal`); otherwise built-in names (`get`, `push`…) only link when the receiver looks like the class | through the receiver's class (`create_order`'s `store: MemoryStore` and `audit: AuditLog \| None` each reach their own `save`); otherwise as TypeScript | name matching |
| Coverage import | ✓ coverprofile | ✓ LCOV | ✓ LCOV | ✓ JaCoCo |
| Testability hints | ✓ | ✓ | ✓ | ✓ |
| One suite and report per component in a monorepo | ✓ | ✓ | ✓ | n/a |

| | Rust | C# | Ruby |
|---|---|---|---|
| Functions and methods | ✓ (`impl` methods, `fn new`) | ✓ (expression-bodied, constructors) | ✓ (`def self.x`, `initialize`) |
| Tests discovered | ✓ (`#[test]` in `#[cfg(test)]` modules) | ✓ (`[Fact]`, `[Theory]`) | ✓ (describe/it) |
| Test → code mapping | ✓ (`MemoryStore::new()` → `new`; test modules aren't code under test) | ✓ (`store.Save` → `MemoryStore.Save`: the receiver names the class) | ✓ (`Foo.new` → `initialize`; helpers inside `describe`) |
| Call edges | name matching (`~`) | name matching (`~`); `int.Parse` isn't the repo's `Parse` | name matching (`~`) |
| Calls without parentheses (`@store.save order`) | n/a | n/a | ✗ (#76) |
| Calls inside string interpolation (`"#{timestamp}"`) | n/a (none in the example) | n/a (none in the example) | ✓ (a bare name is a call when the class has that method) |
| Coverage import | ✓ LCOV | ✓ Cobertura | ✓ LCOV |
| Suite detected for Run coverage | ✓ cargo llvm-cov | ✓ dotnet test | ✓ `bundle exec rspec` |
| Testability hints | ✓ (env, clock, network) | ✓ (env, clock, network) | ✓ (env, clock, network) |

## Regenerating coverage

The reports are committed so the QA view, the demo script and
`go test ./internal/core -run Example` work without these toolchains. You
only need a toolchain to regenerate an example's report after changing it,
or to run its tests from CodeDiver. [TOOLCHAINS.md](TOOLCHAINS.md) says how
to install each one, with or without admin rights.

### Toolchains

| Example | Toolchain | Test and coverage tools | Versions the committed reports came from |
|---|---|---|---|
| go-service | [Go](https://go.dev/dl/) 1.22+ (the module's minimum) | built in: `go test -coverprofile` | Go 1.26.9 |
| ts-react | [Node.js](https://nodejs.org) 20+ with npm | Vitest and `@vitest/coverage-v8` (pinned in `package.json`; `npm install` fetches them) | Node 24.21, npm 11.19, Vitest 3.2.4 |
| python-api | Python 3.9+ (uses `dict[...]` and `tuple[...]` annotations) | pytest, pytest-cov and coverage.py (pinned in `requirements-dev.txt`) | Python 3.12.3, pytest 8.3.3, pytest-cov 5.0.0, coverage 7.16.2 |
| java-maven | JDK 17+ and [Maven](https://maven.apache.org/download.cgi) 3.9+ | JUnit 5 and the JaCoCo Maven plugin (in `pom.xml`) | JDK 25, JUnit 5.11.3, JaCoCo 0.8.12* |
| rust-cargo | Rust stable via [rustup](https://rustup.rs) (cargo, rustc) | [cargo-llvm-cov](https://github.com/taiki-e/cargo-llvm-cov) (`cargo install cargo-llvm-cov`) and `rustup component add llvm-tools-preview` | Rust 1.99.1, cargo-llvm-cov 0.9.1 |
| dotnet-xunit | [.NET SDK](https://dotnet.microsoft.com/download) 8.0+ | xUnit and coverlet (`coverlet.collector`), pinned in `tests/Orders.Tests/Orders.Tests.csproj`; `dotnet test` restores them | .NET SDK 8.0.426, xUnit 2.9.2, coverlet.collector 6.0.2 |
| ruby-rspec | Ruby 3.2+ with Bundler | RSpec, SimpleCov and simplecov-lcov (in `Gemfile`, locked in `Gemfile.lock`) | Ruby 3.4.5, RSpec 3.13, SimpleCov 0.22 |
| polyglot-monorepo | Go, Node.js and Python as above | as above, per component | as above |

\* Generated without Maven: compiled with `javac --release 17`, run with
the JUnit console launcher (`junit-platform-console-standalone` 1.11.3)
under the JaCoCo agent, reported with the JaCoCo CLI (`report … --xml`).
Running Maven gives the same report.

Use a virtual environment for Python (`python -m venv .venv`) and don't
commit it, `node_modules`, `__pycache__`, `.coverage`, or anything in
`target/` except `site/jacoco/jacoco.xml`. Build output (`target/` for
Rust, `bin/` and `obj/` for .NET, `.bundle/`) and SimpleCov's
`.resultset.json` are git-ignored under `testdata/examples`.

**Run coverage in the app** works the same way: the suite CodeDiver detects
runs the project's own tools, so install them first. For ts-react and the
monorepo's `web`, run `npm install` in that folder; without it Vitest stops
with "Cannot find package 'vitest'" and writes no report. For ruby-rspec,
run `bundle install`; Rust and .NET fetch their packages on the first run. `node_modules`
and the HTML report are git-ignored under `testdata/examples`.

### Commands

From the example's folder:

| Example | Command |
|---|---|
| go-service | `go test -coverprofile=coverage.out -coverpkg=./... ./...` |
| ts-react | `npm install && npm run coverage` (don't commit `node_modules`) |
| python-api | `pip install -r requirements-dev.txt && python -m pytest --cov=orders --cov-report=lcov:coverage.lcov` |
| java-maven | `mvn -q org.jacoco:jacoco-maven-plugin:prepare-agent test org.jacoco:jacoco-maven-plugin:report` (commit only `target/site/jacoco/jacoco.xml`) |
| rust-cargo | `cargo llvm-cov --lcov --output-path lcov.info --remap-path-prefix` (keeps the paths relative) |
| dotnet-xunit | `dotnet test --collect:"XPlat Code Coverage" --results-directory coverage`, then move `coverage/<guid>/coverage.cobertura.xml` to `coverage/coverage.cobertura.xml` and make its `<source>` relative (`src/Orders/`) |
| ruby-rspec | `bundle install && bundle exec rspec` (writes `coverage/lcov.info` with relative paths) |
| polyglot-monorepo | each component as above, from its own folder (see its README) |

Then run `go test ./internal/core -run Example` from the repository root.
If the demo change or the history step moved, regenerate the patches in
`<example>/.demo/`: `history.patch` is the second commit, `change.patch`
the uncommitted change, and an optional `commits` file names the two
commits.

These folders live under `testdata/`, so Go tooling ignores them. When you
open the CodeDiver repository itself, CodeDiver doesn't detect their suites
or count their tests as its own.
