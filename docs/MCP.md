# MCP tool reference

Endpoint: `http://127.0.0.1:47474/mcp` (streamable HTTP), or `codediver mcp`
for stdio clients. All paths are repo-relative. `repo` is optional on every
tool and defaults to the repository open in CodeDiver. Symbols can be given as
an id (from any tool output), a qualified name (`Type.Method`), a bare name
or `file:line`.

Graph and analysis tools accept `show: true` to put the result on the
developer's screen.

## Context & exploration

| Tool | Purpose |
|---|---|
| `list_repos` | Repositories shared with agents, the active one, and git state. |
| `get_developer_context` | What the developer is looking at: view, file, cursor lines, selected text and enclosing symbol. |
| `repo_overview` | Languages, size, top-level directories, likely entry points, churn × complexity hotspots, README excerpt. |
| `list_files` | Directory tree (respects `.gitignore` and excludes). |
| `read_file` | File content with line numbers; optional range. |
| `get_file_summary` | Everything declared in a file: functions (inputs → outputs, complexity, callers/callees, doc), types, top-level variables/constants and imports. |
| `search_code` | Text or regex search, optional glob. |
| `search_symbols` | Find functions, methods, types and classes by name. |
| `get_symbol` | Signature, parameters, results, fields, doc, complexity, callers, callees, implementations and optional source. |

## Analysis

| Tool | Purpose |
|---|---|
| `get_call_graph` | Callees, callers or both to a depth; optional external calls and function references (`ref`). |
| `trace_to` | Every caller path from entry points to a function ("how is this triggered?"). |
| `get_function_flow` | Control-flow graph with inputs/outputs (precise for Go). |
| `get_dependencies` | Package/directory dependency graph, optionally focused on one package. |
| `get_types` | Structs/classes/interfaces with fields, methods and `implements` edges. |
| `git_status` | Branch and changed files. |
| `git_diff` | Files changed, +/−, functions touched and affected callers; `patch: true` for full hunks. |
| `git_log` | Recent commits with files; optionally for one path. |
| `get_heatmap` | Files ranked by `churn`, `changes`, `proposals`, `complexity`, `agent`, `size` or `coverage` (uncovered lines). |

## Testing & coverage

Coverage comes from report files your own test tooling writes (Go
coverprofile, LCOV, Cobertura XML, JaCoCo XML), either left in the repository
(`coverage.out`, `coverage/lcov.info`, …) or produced when the developer clicks
**Run coverage** in the QA view. **Agents cannot run tests or commands**; the
tools below say when coverage is missing or stale so the agent can ask.

Tests are discovered statically for Go, JS/TS (Jest, Vitest, Mocha, Playwright,
node:test), Python (pytest, unittest), Java/Kotlin (JUnit), C# (xUnit, NUnit,
MSTest), Rust, Ruby (RSpec, minitest), Swift (XCTest), PHP (PHPUnit), C++
(GoogleTest) and Dart. Each test is mapped to the code it calls by name and
package; reach through callers comes from the call graph.

| Tool | Purpose |
|---|---|
| `get_test_overview` | Line coverage and its source/age, test and behavior counts per framework, least-covered directories and the riskiest under-tested functions. |
| `get_coverage` | For a file: % covered, uncovered and partially covered line ranges, coverage per function. For a directory or the repo: per-file table, worst first. `show: true` opens the file with gutter markers. |
| `get_tests_for` | Tests that exercise a function directly or through its callers, with the behavior each describes; or, for a test file, every behavior and the code each test calls. |
| `find_test_gaps` | Functions ranked by risk (missing coverage × complexity × callers × visibility) with uncovered ranges and testability problems — hard-wired clock, randomness, env vars, network, filesystem, processes, sleeps, process exits, browser globals, high complexity — each with the refactor that fixes it. |
| `show_test_plan` | Show a plan in the QA view: behaviors to test (kind, given/when/then, the line each exercises, existing tests that already cover it) and refactors needed first. Follow with `propose_changes` for the tests. |

## Presenting to the developer

| Tool | Purpose |
|---|---|
| `show_code` | Open a file and highlight a range, with an optional message. |
| `start_walkthrough` | Guided tour: ordered steps (file/lines or symbol + markdown explanation). |
| `explain_function` | Summary, meaning of each input and output, logic steps tied to line ranges, side effects and notes — rendered beside the function's control-flow graph with steps pinned to blocks. |
| `show_diagram` | Mermaid source, or nodes/edges (nodes may link to files) rendered as an interactive graph. |
| `annotate_code` | Inline notes (`info`, `warning`, `bug`, `idea`, `question`) in the editor gutter. |

## Changing code

| Tool | Purpose |
|---|---|
| `propose_changes` | Submit a change set. Per file: `edits` (exact search/replace, each `old` must occur once) or `new_content`; `action` is `modify`, `create` or `delete`. Nothing is written until the developer accepts. Returns a proposal id, stats and blast radius. |
| `get_proposal` | `pending`, `applied`, `rejected`, `partial` or `conflict`, per file, plus the developer's feedback. |

### Recommended agent workflow

1. `get_developer_context` → understand what "this" refers to.
2. `repo_overview` / `search_symbols` / `get_symbol` → orient.
3. `trace_to` or `get_call_graph` with `show: true` → show the shape.
4. `explain_function` or `start_walkthrough` → teach.
5. `propose_changes` → let the developer review; poll `get_proposal`.

For "what isn't tested?": `get_test_overview` → `find_test_gaps` →
`get_tests_for` → `show_test_plan` → `propose_changes` with the new tests.
