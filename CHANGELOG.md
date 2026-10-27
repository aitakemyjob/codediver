# Changelog

Notable changes to CodeDiver. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org). Each
[GitHub release](https://github.com/aitakemyjob/codediver/releases) also lists
every merged pull request.

## [Unreleased]

### Added
- **Example projects for Rust, C# and Ruby** (#51): `rust-cargo` (cargo
  test, cargo-llvm-cov), `dotnet-xunit` (xUnit, coverlet Cobertura) and
  `ruby-rspec` (RSpec, SimpleCov LCOV), with committed coverage and demo
  patches, pinned in the examples tests. RSpec projects get a coverage
  suite (`bundle exec rspec`), and `coverage/coverage.cobertura.xml` is
  imported automatically.
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
- **Tests for a change:** proposals, the working tree, branches and
  commits get a Tests panel: how many changed lines tests run, which ones
  no test runs, the tests to run, and changed functions no test reaches,
  with coverage markers on the changed lines in the diff. **Run these
  tests** runs Go and pytest tests one by one with per-test coverage, so
  the panel says which test runs which line (measured) instead of
  estimating from the test-to-code mapping. Agents get the same with
  `get_tests_for_change`, and `propose_changes` / `get_proposal` now
  report the coverage of the change.

### Changed
- **Callbacks are references** (#68): a function passed as a value
  (`items.map(format)`, `promise.then(render)`, `{ handler: create }`,
  `sorted(xs, key=keyfn)`, `Thread(target=run)`, Java's `this::save` and
  `Orders::total`) gets a `ref` edge from the function passing it, when the
  name resolves to a function, so callbacks no longer look uncalled and
  Trace to here reaches through them.
- **Calls on typed receivers** (#67): in TypeScript, JavaScript and
  Python, `s.save()` links to the `save` of `s`'s class where the code
  writes it down (`new SqlStore()`, typed parameters and fields,
  constructor parameter properties, `s = Store()`, annotated parameters
  and attributes, `self.store = …` in `__init__`), following `extends`,
  instead of the first same-named method. Calls still matched by name
  among several definitions are marked *guessed*: a dotted *guess?* edge
  in the app, and *(guessed …)* with a note in agent tools.
- **Go control flow and generics** (#74): calls through generic interfaces
  (`Repo[int]`) dispatch to the types implementing that instantiation,
  generic ones included. In the Logic view, a block holding a closure
  opens the closure's own control flow, with a breadcrumb back; `goto`
  connects to its label; and `t.Fatal`, `FailNow` and `Skip*` end a test
  like `panic` does. `get_function_flow` shows closures nested under
  their block.
- **Module-level code is a caller** (#69): each file's top-level code is
  a `<module>` symbol that calls what it calls. That covers TypeScript,
  JavaScript and Python module statements (`render(<App />)`,
  `if __name__ == "__main__": main()`) and Go package-level `var`
  initializers. `App` in the `ts-react` example now has `main.tsx
  <module>` as its caller. Modules are entry points in traces and the
  blast radius, and have no test status. Test files' top level isn't
  counted.
- **TypeScript and JavaScript calls resolve through imports** (#65): a
  call links to the function its file imports instead of any same-named
  function, through named, aliased, default and namespace imports,
  re-exports and barrels, and CommonJS `require`. Calls that don't go
  through a resolvable import still match by name.
- **Python calls resolve through imports** (#66): aliases (`import … as`,
  `from … import … as`), module-qualified calls (`u.fmt()`,
  `util.fmt()`), relative imports, `from m import *` and names re-exported
  by a package's `__init__.py` link to the definition imported, not a
  same-named one elsewhere. The `python-api` example's receipt package
  shows each case.
- **Blast radius:** the full caller graph is replaced by a view that
  goes by distance. Direct callers are listed first with their test
  status and coverage, and untested ones are flagged. Callers further out
  are grouped by package, and the far end shows the entry points the
  change reaches. Each caller shows its path back to the change, and a
  package graph is available on demand. The radius is capped and grouped
  on the server, so changing a widely used helper no longer produces a
  graph of hundreds of nodes.
- **Language support docs** (#8): [docs/LANGUAGES.md](docs/LANGUAGES.md)
  lists, per language, how symbols, call graphs, traces, control flow,
  dependencies, tests and coverage are analyzed. It also covers what the
  `~` marker does and doesn't guarantee, and the known gaps with
  examples. Agents read the same page, as shipped in the build, with the
  new `get_language_support` tool (one language or all) or the
  `codediver://docs/languages` resource. Graphs with name-matched edges
  end with a precision note pointing to it, and the tool descriptions no
  longer suggest control flow or dependency resolution exists where it
  doesn't.

### Fixed
- **Calls inside string interpolation** (#79) in Ruby, Kotlin, C#, Swift,
  Dart, Scala and PHP are now code, like JavaScript template literals and
  Python f-strings already were, so they get call edges and test-to-code
  mapping. In Ruby, a bare `"#{name}"` calls the caller's class's method
  of that name: `Notifier.timestamp` in the `ruby-rspec` example now has
  its caller.
- **Call graphs outside Go** (#59): definitions and calls the lexical
  analyzer missed or got wrong.
  - **TypeScript/JavaScript** (#61, #62):
    - constructors, methods written on one line, class fields holding
      arrow functions, object-literal methods (Workers' `export default {
      fetch() }`) and anonymous default exports are found
    - `new Foo()` reaches Foo's constructor
    - `obj?.m()` keeps its receiver
    - `super.m()` goes to the parent class
    - multi-line imports count as dependencies
  - **Java, C#, Kotlin, Rust** (#63):
    - one-line Java methods and constructors are found
    - C# `=>` methods are found
    - Kotlin `= expr` functions end where they should
    - Rust `fn new()` and `Store::new()` calls are found
  - **Python** (#64): calls inside f-strings are found, in call graphs
    and in the test-to-code mapping.
  - **Tests that construct a class** (`new Cart()`, `Store()`) now count
    as running its constructor (`constructor`, `__init__` or a method
    named after the class), and through it what the constructor calls.
- A coverage run that fails because the project's JavaScript dependencies
  aren't installed ("Cannot find package 'vitest'") now says to run
  `npm install` in the suite's folder instead of only reporting that no
  coverage report was written.
- Found through the new examples (#51):
  - **Test mapping:**
    - Rust unit tests in `#[cfg(test)]` modules are test code, not code
      under test
    - `MemoryStore::new()`, `Foo.new` (Ruby) and `store.Save(…)` (a
      receiver named like its class) map tests to the code they call
    - a helper defined inside `describe` no longer absorbs the tests
      after it
  - **Call graph:** calls on built-in types (`int.Parse`, `String::new`)
    no longer link to same-named repository functions.
  - **Testability hints** recognize Rust's `TcpStream`, .NET's
    `HttpClient` calls, and Ruby's `Time.now`, `ENV` and `Net::HTTP`.
- **Blast radius test status** (#57): callers showed as tested when a
  test merely could reach them on paper, or when any of their lines ran.
  Each caller now says whether tests run its call toward the change
  (*runs the path*), run it but not that call (*call not run*), never run
  it (*not run*), or, without coverage, whether a test calls it directly
  (*likely tested*, estimated). Measured coverage always decides over the
  static estimate, per-test runs name the tests, coverage older than the
  file is marked stale, and rings count callers by status. Direct callers
  whose call into the change no test runs are flagged for review, in the
  app and in the agent's `get_tests_for_change` text.
- The test-to-code mapping ignored nothing inside strings and comments,
  so a test with source code as a fixture "called" what that code called
  (#19).
- Jest manual mocks (files under `__mocks__/`) count as test files, not as
  untested code.
- Coverage below 0.05% shows as `<0.1%` instead of a misleading `0.0%`.
- Agent text for tests of a change: once per-test runs exist, tests that
  run the changed lines are listed apart from tests that reach the changed
  functions without running those lines, and estimated tests are labelled
  as such. Counts read "1 caller", "1 new line", and testability hints
  read `(line 7: fetch)`.
- **Java / Kotlin:** tests in `src/test/java/…` are now mapped to the code
  they call in the mirrored `src/main/java/…` package, so methods called
  on local variables (`store.save(…)`) count as tested.
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
