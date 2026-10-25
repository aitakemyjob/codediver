# Language support

CodeDiver analyzes Go precisely and every other code language lexically.
This page explains what each feature can and can't do per language, so you
and your agents know how much to trust a call graph, a trace or a blast
radius.

## Two analyzers

**Go is type-checked.**
- Each package is checked with `go/types`. No Go toolchain or downloaded
  dependencies are needed: imports from outside the repository become
  empty stand-in packages.
- Calls between the repository's packages are exact. Calls into the
  standard library and dependencies are recorded by name only, as
  `external` nodes.
- **Edges:** calls, `go` and `defer`, functions passed as values (`ref`),
  interface `dispatch` (an interface method to each implementation) and
  `implements`.
- **Build constraints** are evaluated for the platform CodeDiver runs on.
- **Excluded:** `_test.go` files are left out of the call graph; the QA view
  maps tests separately.

**Every other language is lexical.**
- Per-language patterns run over the source after comments and string
  contents are blanked out. They find definitions, where each one ends
  (braces, indentation or `end`), imports, and calls.
- Calls are found as `name(` or `receiver.name(`. In TypeScript,
  JavaScript and Python, a call is first resolved through the calling
  file's imports (see below). Otherwise, and in every other language, it
  is matched **by name**.
- When several definitions share a name, CodeDiver prefers:
  1. a method of the same class, for `this.`/`self.` calls
  2. a definition in the same file
  3. one in the same directory
  4. any match in the repository

  If the closest group has one to three candidates, the **first** is linked.
  With more than three, no edge is drawn.
- Calls on built-in types and namespaces (`int.Parse`, `Math.max`,
  `String::new`) never link to the repository's code.

## The `~` marker

Everything the lexical analyzer produces is marked as heuristic:
- **App:** `~` on graph nodes, faded edges, and a *heuristic* badge in the
  inspector.
- **Agent tools:** `~heuristic` on symbols, `~` on edges, and a precision
  note under graphs that contain them.

A `~` edge **does** mean that a function or method with that name is called
on that line, in code (not in a comment or string).

It **does not** guarantee that:
- **The target is right.** The edge points at the definition that was
  picked, which may be a same-named function elsewhere.
- **The callers are complete.** Calls through variables, aliases,
  callbacks and framework wiring are missed.
- **The edge is real.** Decorators and some call shapes match by accident
  (see the known limitations below).

## Support matrix

| | Symbols | Call graph, trace, blast radius | Control flow | Dependencies | Tests found | Coverage |
|---|---|---|---|---|---|---|
| **Go** | type-checked | type-checked, with interface dispatch | full graph | packages and external modules | `go test`, subtests, table cases | coverprofile; per-test runs |
| **TypeScript, JavaScript** | lexical | through imports, else name match `~`; JSX renders count as calls | — | directories, from relative imports | Vitest, Jest, Mocha, Playwright, node:test | LCOV |
| **Python** | lexical | through imports, else name match `~` | — | directories, from imports | pytest, unittest | LCOV, Cobertura; per-test runs (pytest) |
| **Java, Kotlin, Scala** | lexical | name match `~` | — | directories, from imports | JUnit | JaCoCo, Cobertura |
| **C#** | lexical | name match `~` | — | directories, from `using` | xUnit, NUnit, MSTest | Cobertura |
| **Rust** | lexical | name match `~` | — | `mod` and `crate::` paths | `#[test]` | LCOV |
| **Ruby** | lexical | name match `~`, calls with parentheses only | — | `require_relative` | RSpec, minitest | LCOV, Cobertura |
| **PHP** | lexical | name match `~` | — | `require`, `include` | PHPUnit | LCOV, Cobertura |
| **C, C++** | lexical | name match `~` | — | quoted `#include` | GoogleTest | LCOV, Cobertura |
| **Swift, Dart, Lua** | lexical | name match `~` | — | Lua `require` only | XCTest, dart test | LCOV, Cobertura |

**Control flow (—)** is only built for Go. For other languages, the Logic
view and `get_function_flow` show the signature and one body block; ask your
agent to explain the function (`explain_function`) instead. TypeScript and
Python control flow are planned ([#71], [#72], [#73]).

**Complexity** outside Go counts branch keywords and `&&`, `||` and `?`, so
treat it as an estimate.

**Coverage** maps reports to files by path and line in every language, so
line numbers are exact everywhere. Per-function percentages depend on where
the analyzer thinks a function ends, which some of the gaps below get
wrong.

The [example projects](../testdata/examples/README.md#support-matrix) pin
much of this matrix in tests.

## Known limitations

Each item links the issue that plans its fix, or says *inherent* when it's
a limit of how the analysis works. Deeper analysis is tracked in [#18].

### Go

- **External interfaces:** calls through interfaces defined outside the
  repository aren't linked to implementations. A handler passed to
  `http.Handle` shows no callers for its `ServeHTTP`. *inherent: the
  standard library isn't analyzed*
- **Generic interfaces:** `dispatch` edges come from the instantiations
  the code uses. With `var r Repo[int] = intRepo{}`, `Repo.Get` dispatches
  to `intRepo.Get` and to generic types instantiated to satisfy
  `Repo[int]`, but not to a type that only implements `Repo[string]`
  when nothing uses `Repo[string]`.
- **Package initializers:** calls in package-level `var` initializers
  (`var x = compute()`) are made by the file's `<module>` (see *Trace
  view* below). `init` functions are functions of their own.
- **Closures:** calls inside a closure count for the enclosing function, as
  plain calls. `go func() { f() }()` records a call to `f`, not a `go`
  edge.
- **In the control-flow view:**
  - a block holding a closure has a *closure* chip that opens the
    closure's own control flow, and agents get it nested under that block
  - `goto` jumps connect to their label's statement
  - `t.Fatal`, `FailNow` and `Skip*` on the function's own `*testing.T`,
    `B`, `F` or `testing.TB` parameter are exits, like `panic`, `os.Exit`
    and `log.Fatal*`. Through a helper or another variable, they're plain
    calls.
- **Not seen:** reflection, `go:linkname`, and files built only for other
  platforms. *inherent*

### TypeScript and JavaScript

How calls resolve through imports:
- A call to an imported name links to the definition it imports, even when
  other files define the same name: named (`import { format }`), aliased
  (`import { format as fmt }`), default (`import save from`) and
  namespace (`import * as api`, then `api.list()`) imports, and CommonJS
  `const { load } = require('./x')` and `const x = require('./x')`.
- Re-exports and barrels are followed to the defining file (`export { x }
  from`, `export * from`, `export { default as Button } from`), up to
  8 hops, as are local export lists (`export { a as b }`) and
  `module.exports = { … }`.
- Static methods of an imported class (`Store.create()`) and methods of an
  imported object literal (`api.remove()`) link to that file's member.
- When the import doesn't lead to a definition in the repository (a
  package, a path alias, an export CodeDiver can't read), the call falls
  back to name matching.
- These edges are still marked `~`: a local variable that shadows an
  imported name isn't noticed.

How definitions are named:
- **Object literals:** methods and function properties of an object
  literal assigned to a name or default-exported (Workers'
  `export default { fetch() {…} }`, `const api = { list() {…} }`) are
  qualified by the object's name: `default.fetch`, `api.list`. The object
  itself isn't a symbol.
- **Default exports:** an anonymous default export is called `default`.
- **Constructors:** `new Foo()` links to `Foo`'s `constructor` when it has
  one.

Edges that are wrong or missing:
- `cache.save()` links to the first class with a `save` method, which may be
  the wrong class. *[#67]*
- `super.handle()` links to `handle` on the class named by `extends`.
  When that class isn't in the repository, no edge is drawn.
- Functions passed as values get no edge: `items.map(format)`,
  `promise.then(render)`, `emitter.on('done', notify)`. React components
  passed as props are an exception. *[#68]*
- **Module-level code:** calls outside any function and class, like
  `render(<App />)` in `main.tsx`, are made by the file's `<module>`. Test
  files are left out: their top level is `describe`/`it` blocks.
- **Framework wiring:** decorators like `@Get()` read as calls to a symbol
  named `Get`. Framework entry points (route handlers, Workers, Durable
  Object methods) show no callers. *[#75]*

Dependencies:
- Path aliases (`@/lib/x`, tsconfig `paths`) and workspace package names
  aren't resolved. *[#70]*
- Dynamic `import('./x')` is ignored. *[#70]* `require()` and
  `export … from` re-exports are followed.

### Python

How calls resolve through imports:
- A call to an imported name links to the definition it imports, even when
  other modules define the same name: `from .util import fmt as f` then
  `f()`, `from .store import Store as S` then `S()` (its `__init__`),
  `import orders.util as u` then `u.fmt()`, `from orders import util`
  then `util.fmt()`, and `from .util import *`.
- Relative (`.`, `..`) and absolute imports both resolve, and names a
  package's `__init__.py` imports are followed to where they're defined.
- Class methods called on an imported class (`Store.create()`) link to
  that class's method.
- When the import doesn't lead to a definition in the repository (the
  standard library, an installed package), the call falls back to name
  matching. These edges are still marked `~`.

- **Same-named methods:** `store.save()` links to the first class with a
  `save` method; with both `Store.save` and `Cache.save`, it can pick
  `Cache.save`. *[#67]*
- **Functions passed as values** get no edge: `sorted(xs, key=keyfn)`,
  `map(fn, xs)`, callbacks. *[#68]*
- **Implicit calls:** `@property` reads and dunder methods (`__enter__`,
  operators) aren't calls. *inherent*
- **Decorated entry points:** `@app.route`, fixtures and task decorators
  show no callers. *[#75]*
- **Module-level code** (including `if __name__ == "__main__": main()`)
  is made by the file's `<module>`. Test files are left out.

### Java, Kotlin, C# and Rust

- **Java and C#:** interface method declarations aren't symbols. A call
  through an interface links by name to a class with that method. *[#76]*
- **Java:** method references (`this::save`) get no edge. *[#68]*
- **Rust:** `impl Trait for Type` isn't linked to the trait. *[#76]*
  Calls inside macros like `println!` are found. Functions in
  `#[cfg(test)]` modules are test code, not code under test.
- **Kotlin and C#:** calls inside string interpolation (`"${total(o)}"`,
  `$"{Total(o)}"`) are lost with the string text. *[#79]*

### Ruby

- Calls without parentheses (`write o`, `save!`) aren't found, and that's
  how most Ruby is written. Expect sparse call graphs. *[#76]*
- Calls inside string interpolation (`"sent at #{timestamp}"`) are lost
  with the string text. *[#79]*
- `Foo.new` reaches `Foo#initialize` in the test-to-code mapping, but not
  yet in the call graph.

## Trace view and `trace_to`

- **How it walks:** from the target back through its callers, up to
  6 calls by default and at most 200 functions. Import and `implements`
  edges are skipped.
- **Entry points:** any caller with no callers of its own is badged as an
  entry. A file's top-level code appears as `<module>` (`main.tsx
  <module>`): in TypeScript, JavaScript and Python it's the module's
  statements, in Go its package-level var initializers. It is never called
  and isn't a function, so it has no test status. In Go that is accurate apart from the external interfaces above.
  For lexical languages it means *no caller was found*, so framework
  handlers, object-literal methods and decorated functions appear as
  entries.
- **Ambiguous calls:** when a call matched several same-named definitions,
  the trace follows the one that was picked (see above), and nothing marks
  that a choice was made. Check `~` edges against the code before relying
  on a path.

[#18]: https://github.com/aitakemyjob/codediver/issues/18
[#67]: https://github.com/aitakemyjob/codediver/issues/67
[#68]: https://github.com/aitakemyjob/codediver/issues/68
[#70]: https://github.com/aitakemyjob/codediver/issues/70
[#71]: https://github.com/aitakemyjob/codediver/issues/71
[#72]: https://github.com/aitakemyjob/codediver/issues/72
[#73]: https://github.com/aitakemyjob/codediver/issues/73
[#75]: https://github.com/aitakemyjob/codediver/issues/75
[#76]: https://github.com/aitakemyjob/codediver/issues/76
[#79]: https://github.com/aitakemyjob/codediver/issues/79
