// Typed client for the Go method table in internal/api.
import { getTransport } from './transport';
import type {
  AppState, Commit, Config, FileSummary, DiffOverview, DiffSpec, FileContent, FileSymbol, FileVersions, Flow, Graph,
  GitRef, Heatmap, Overview, Proposal, Repo, RepoSummary, SearchMatch, SessionData, Settings, StatusEntry,
  SymbolDetail, CodeSymbol, TreeNode, Activity, GitInfo, QAOverview, FileCoverage, SymbolTests, TestSuite, QARun, CoverageSource,
} from './types';

/** Calls a backend method through the active transport. */
async function call<T>(method: string, args?: unknown): Promise<T> {
  const t = await getTransport();
  return t.invoke<T>(method, args);
}

export interface CallGraphOptions {
  direction?: 'callees' | 'callers' | 'both';
  depth?: number;
  includeExternal?: boolean;
  includeRefs?: boolean;
  maxNodes?: number;
}

export const api = {
  /** Full application state: config, repos, MCP status and capabilities. */
  state: () => call<AppState>('app.state'),
  /** Configured repositories with their git state. */
  repos: () => call<RepoSummary[]>('repos.list'),
  /** Asks where to save a file and writes it (desktop app only); returns the path, '' if cancelled. */
  saveFile: (name: string, data: string) => call<{ path: string }>('file.save', { name, data }),
  /** Answers an agent's render request (export_demo). */
  renderResult: (r: { id: string; data: string; width?: number; height?: number; error?: string }) => call<void>('render.result', r),
  /** Opens the native folder picker (desktop app only). */
  pickDirectory: () => call<{ path: string }>('repos.pick'),
  /** Registers a repository folder. */
  addRepo: (path: string, name = '') => call<Repo>('repos.add', { path, name }),
  /** Forgets a repository (files on disk are untouched). */
  removeRepo: (id: string) => call<void>('repos.remove', { id }),
  /** Saves a repository's name, agent visibility and hidden paths. */
  updateRepo: (repo: Repo) => call<void>('repos.update', repo),
  /** Switches the active repository. */
  setActiveRepo: (id: string) => call<void>('repos.setActive', { id }),
  /** Persists application settings. */
  updateSettings: (s: Settings) => call<void>('settings.update', s),
  /** Agent activity log, oldest first. */
  activity: () => call<Activity[]>('activity.list'),
  /** Reports what the developer is looking at, for get_developer_context. */
  setContext: (ctx: Record<string, unknown>) => call<void>('ui.context', ctx),

  /** Repository dashboard: stats, entry points, hotspots, README. */
  overview: (repo: string) => call<Overview>('overview', { repo }),
  /** Forces the analysis index to rebuild. */
  reindex: (repo: string) => call<unknown>('index.rebuild', { repo }),
  /** Visible file tree. */
  tree: (repo: string) => call<TreeNode>('fs.tree', { repo }),
  /** Reads a file through the sandbox. */
  read: (repo: string, path: string) => call<FileContent>('fs.read', { repo, path }),
  /** Text or regex search across the repository. */
  search: (repo: string, query: string, opts: { regex?: boolean; caseSensitive?: boolean; glob?: string; limit?: number } = {}) =>
    call<{ matches: SearchMatch[]; truncated: boolean }>('fs.search', { repo, query, ...opts }),

  /** Symbols in a file with caller/callee counts (code lenses, outline). */
  fileSymbols: (repo: string, file: string) => call<FileSymbol[]>('sym.file', { repo, file }),
  /** Everything declared in a file: functions, types, variables, imports. */
  fileSummary: (repo: string, file: string) => call<FileSummary>('sym.summary', { repo, file }),
  /** Inspector details for a symbol, optionally with its source. */
  symbol: (repo: string, symbol: string, code = false) => call<SymbolDetail>('sym.detail', { repo, symbol, code }),
  /** Fuzzy symbol search for the palette. */
  searchSymbols: (repo: string, q: string, limit = 40) => call<CodeSymbol[]>('sym.search', { repo, q, limit }),

  /** Call graph around a symbol. */
  callGraph: (repo: string, symbol: string, o: CallGraphOptions = {}) => call<Graph>('graph.call', { repo, symbol, ...o }),
  /** Every caller path leading to a symbol. */
  trace: (repo: string, symbol: string, depth = 6) => call<Graph>('graph.trace', { repo, symbol, depth }),
  /** Package/directory dependency graph. */
  deps: (repo: string, includeExternal = false, focus = '') => call<Graph>('graph.deps', { repo, includeExternal, focus }),
  /** Type diagram for a package, directory or file. */
  types: (repo: string, scope: string) => call<Graph>('graph.types', { repo, scope }),
  /** Control-flow graph of a function. */
  flow: (repo: string, symbol: string) => call<{ flow: Flow; symbol: CodeSymbol }>('graph.flow', { repo, symbol }),
  /** Blast-radius graph of a proposal. */
  proposalImpact: (repo: string, proposal: string) => call<Graph>('graph.impact', { repo, proposal }),
  /** Heatmap values for a metric (optionally one proposal). */
  heatmap: (repo: string, metric: string, proposal = '') => call<Heatmap>('heatmap', { repo, metric, proposal }),

  /** Git diff mapped to changed functions, with impact graph. */
  diff: (repo: string, spec: DiffSpec = {}, impact = true) => call<DiffOverview>('git.diff', { repo, spec, impact }),
  /** Both sides of a file for the side-by-side diff viewer. */
  versions: (repo: string, path: string, oldPath: string, spec: DiffSpec) => call<FileVersions>('git.versions', { repo, path, oldPath, spec }),
  /** Recent commits, optionally for one path. */
  log: (repo: string, limit = 50, ref = '', path = '') => call<Commit[]>('git.log', { repo, limit, ref, path }),
  /** Branches, remote branches and tags. */
  refs: (repo: string) => call<GitRef[]>('git.refs', { repo }),
  /** Branch info and changed files. */
  status: (repo: string) => call<{ info: GitInfo; entries: StatusEntry[] }>('git.status', { repo }),

  /** Agent proposals for a repository. */
  proposals: (repo: string) => call<Proposal[]>('proposals.list', { repo }),
  /** Accepts or rejects proposal files, with optional feedback for the agent. */
  resolveProposal: (repo: string, id: string, accept: boolean, paths?: string[], feedback = '') =>
    call<Proposal>('proposals.resolve', { repo, id, accept, paths, feedback }),
  /** Removes a proposal from the list. */
  dismissProposal: (id: string) => call<void>('proposals.dismiss', { id }),
  /** Walkthroughs, diagrams, explanations and annotations. */
  session: (repo: string) => call<SessionData>('session.all', { repo }),
  /** Removes all agent annotations. */
  clearAnnotations: (repo: string) => call<void>('annotations.clear', { repo }),

  /** QA view: coverage, tests, behaviors, gaps, suites, last run and plans. */
  qaOverview: (repo: string) => call<QAOverview>('qa.overview', { repo }),
  /** Line coverage and per-function test status for one file. */
  qaFile: (repo: string, path: string) => call<FileCoverage>('qa.file', { repo, path }),
  /** Tests that exercise a function directly or through its callers. */
  qaTestsFor: (repo: string, symbol: string) => call<SymbolTests>('qa.testsFor', { repo, symbol }),
  /** Saves the repository's coverage suites. */
  qaSaveSuites: (repo: string, suites: TestSuite[]) => call<void>('qa.suites.save', { repo, suites }),
  /** Reverts to the detected coverage suites. */
  qaResetSuites: (repo: string) => call<void>('qa.suites.reset', { repo }),
  /** Starts a coverage run (all enabled suites, or the given ones). */
  qaRun: (repo: string, suites: string[] = []) => call<QARun>('qa.run', { repo, suites }),
  /** Stops the running coverage run. */
  qaCancel: (repo: string) => call<void>('qa.cancel', { repo }),
  /** The last coverage run with its log. */
  qaLastRun: (repo: string) => call<QARun | null>('qa.lastRun', { repo }),
  /** Imports an existing coverage report file. */
  qaImport: (repo: string, path: string) => call<CoverageSource>('qa.import', { repo, path }),
  /** Forgets all coverage data. */
  qaClear: (repo: string) => call<void>('qa.clear', { repo }),
  /** Removes an agent test plan. */
  qaDismissPlan: (repo: string, id: string) => call<void>('qa.plans.dismiss', { repo, id }),
};

export type { Config };
