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
- Calls are matched **by name**: `name(` or `receiver.name(`. Imports are
  used for the dependency graph, but not yet to decide which definition a
  call means.
- When several definitions share a name, CodeDiver prefers:
  1. a method of the same class, for `this.`/`self.` calls
  2. a definition in the same file
  3. one in the same directory
  4. any match in the repository

  If the closest group has one to three candidates, the **first** is linked.
  With more than three, no edge is drawn.

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
| **TypeScript, JavaScript** | lexical | name match `~`; JSX renders count as calls | — | directories, from relative imports | Vitest, Jest, Mocha, Playwright, node:test | LCOV |
| **Python** | lexical | name match `~` | — | directories, from imports | pytest, unittest | LCOV, Cobertura; per-test runs (pytest) |
| **Java, Kotlin, Scala** | lexical | name match `~` | — | directories, from imports | JUnit | JaCoCo, Cobertura |
| **C#** | lexical | name match `~` | — | directories, from `using` | xUnit, NUnit, MSTest | Cobertura |
| **Rust** | lexical | name match `~` | — | `mod` and `crate::` paths | `#[test]` | LCOV |
| **Ruby** | lexical | name match `~`, calls with parentheses only | — | `require_relative` | RSpec, minitest | LCOV, Cobertura |
| **PHP** | lexical | name match `~` | — | `require`, `include` | PHPUnit | LCOV, Cobertura |
| **C, C++** | lexical | name match `~` | — | quoted `#include` | GoogleTest | LCOV, Cobertura |
| **Swift, Dart, Lua** | lexical | name match `~` | — | Lua `require` only | XCTest, dart test | LCOV, Cobertura |

**Control flow (—)** is only built for Go. For other languages, the Logic
view and `get_function_flow` show the signature and one body block; ask your
agent to explain the function (`explain_function`) instead.

**Complexity** outside Go counts branch keywords and `&&`, `||` and `?`, so
treat it as an estimate.

**Coverage** maps reports to files by path and line in every language, so
line numbers are exact everywhere. Per-function percentages depend on where
the analyzer thinks a function ends, which some of the gaps below get
wrong.

The [example projects](../testdata/examples/README.md#support-matrix) pin
much of this matrix in tests.

## Known limitations

Each item says whether a fix is planned:
- **[#59]:** small fixes to the lexical patterns, planned for 0.2.0
- **[#18]:** needs a real parser or import-aware resolution
- **inherent:** a limit of how the analysis works

### Go

- **External interfaces:** calls through interfaces defined outside the
  repository aren't linked to implementations. A handler passed to
  `http.Handle` shows no callers for its `ServeHTTP`. *inherent: the
  standard library isn't analyzed*
- **Generic interfaces:** these get no `dispatch` edges. With
  `var r Repo[int] = intRepo{}` and `r.Get()`, the trace stops at
  `Repo.Get`. *[#18]*
- **Package initializers:** `var x = compute()` gives `compute` no caller.
  *[#18]*
- **Closures:** calls inside a closure count for the enclosing function, as
  plain calls. `go func() { f() }()` records a call to `f`, not a `go`
  edge.
- **In the control-flow view:**
  - closures are opaque blocks
  - `goto` ends at a jump node without an edge to its label
  - `t.Fatal` and similar test helpers aren't drawn as exits (`panic`,
    `os.Exit` and `log.Fatal*` are)
- **Not seen:** reflection, `go:linkname`, and files built only for other
  platforms. *inherent*

### TypeScript and JavaScript

Definitions that aren't found, so their calls and callers are lost:

```ts
class SqlStore {
  constructor(db: Db) { this.init() }    // constructor isn't a symbol [#59]
  save(o: Order) { return this.write(o) } // method with its body on the signature line [#59]
  onClick = () => { this.save(o) }        // class field holding an arrow function [#59]
}

export default {
  async fetch(req, env) { return route(req) } // object-literal method: Workers, Durable Objects [#59]
}

export default function () { setup() }   // anonymous default export [#59]
```

Edges that are wrong or missing:
- `cache.save()` links to the first class with a `save` method, which may be
  the wrong class. *[#18]*
- `obj?.helper()` links to a free function called `helper`. *[#59]*
- `super.handle()` links to the calling class's own `handle`. *[#59]*
- Functions passed as values get no edge: `items.map(format)`,
  `promise.then(render)`, `emitter.on('done', notify)`. React components
  passed as props are an exception. *[#18]*
- **Imports that rename:** aliased imports (`import { a as b }`) and
  namespace imports (`ns.fn()`) only resolve by the called name. *[#18]*
- **Module-level code:** calls outside any function, like `render(<App />)`
  in `main.tsx`, have no caller, so `App` looks uncalled. *[#18]*
- **Framework wiring:** decorators like `@Get()` read as calls to a symbol
  named `Get`. Framework entry points (route handlers, Workers, Durable
  Object methods) show no callers. *inherent*

Dependencies:
- A multi-line `import { a,\n b } from './x'` is missed. *[#59]*
- Path aliases (`@/lib/x`, tsconfig `paths`) and workspace package names
  aren't resolved. *[#18], [#7]*
- Dynamic `import('./x')` is ignored. `require()` and `export … from`
  re-exports are followed.

### Python

- **Same-named methods:** `store.save()` links to the first class with a
  `save` method; with both `Store.save` and `Cache.save`, it can pick
  `Cache.save`. *[#18]*
- **Import aliases:** `from .store import Store as S` then `S()` doesn't
  link to `Store`. *[#18]*
- **f-strings:** calls inside them are lost with the string text
  (`f"{fmt(o)}"`). *[#59]*
- **Functions passed as values** get no edge: `sorted(xs, key=keyfn)`,
  `map(fn, xs)`, callbacks. *[#18]*
- **Implicit calls:** `@property` reads and dunder methods (`__enter__`,
  operators) aren't calls. *inherent*
- **Decorated entry points:** `@app.route`, fixtures and task decorators
  show no callers. *inherent*

### Java, Kotlin, C# and Rust

- **Java:** methods with an empty or single-line body
  (`void validate(Order o) {}`) aren't symbols. *[#59]*
- **Java and C#:** interface method declarations aren't symbols. A call
  through an interface links by name to a class with that method. *[#18]*
- **Java:** method references (`this::save`) get no edge. *[#18]*
- **C#:** expression-bodied methods
  (`void Validate(Order o) => Check(o);`) aren't symbols. *[#59]*
- **Kotlin:** expression-bodied functions (`fun validate(o) = check(o)`)
  are found, but their end runs into the next function. *[#59]*
- **Rust:** `fn new()` isn't a symbol, so `Store::new()` has no target.
  *[#59]*
- **Rust:** `impl Trait for Type` isn't linked to the trait. *[#18]*
  Calls inside macros like `println!` are found.

### Ruby

- Calls without parentheses (`write o`, `save!`) aren't found, and that's
  how most Ruby is written. Expect sparse call graphs. *[#18]*

## Trace view and `trace_to`

- **How it walks:** from the target back through its callers, up to
  6 calls by default and at most 200 functions. Import and `implements`
  edges are skipped.
- **Entry points:** any caller with no callers of its own is badged as an
  entry. In Go that is accurate apart from the external interfaces above.
  For lexical languages it means *no caller was found*, so framework
  handlers, object-literal methods and decorated functions appear as
  entries.
- **Ambiguous calls:** when a call matched several same-named definitions,
  the trace follows the one that was picked (see above), and nothing marks
  that a choice was made. Check `~` edges against the code before relying
  on a path.

[#7]: https://github.com/aitakemyjob/codediver/issues/7
[#18]: https://github.com/aitakemyjob/codediver/issues/18
[#59]: https://github.com/aitakemyjob/codediver/issues/59
