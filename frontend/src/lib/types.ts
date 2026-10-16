// TypeScript mirrors of the Go JSON contract (internal/api, internal/core,
// internal/analysis, internal/gitx). Keep in sync when changing Go structs.

export interface Repo {
  id: string;
  name: string;
  path: string;
  addedAt: string;
  mcpEnabled: boolean;
  exclude: string[];
}

export interface Settings {
  theme: string;
  fontSize: number;
  mcpPort: number;
  mcpEnabled: boolean;
  followAgent: boolean;
  churnDays: number;
}

export interface Config {
  repos: Repo[];
  activeRepo: string;
  settings: Settings;
}

export interface GitInfo {
  isRepo: boolean;
  branch: string;
  head: string;
  upstream?: string;
  ahead: number;
  behind: number;
  dirty: number;
  hasHead: boolean;
}

export interface RepoSummary extends Repo {
  git: GitInfo;
  missing?: boolean;
}

export interface MCPStatus {
  running: boolean;
  url: string;
  error?: string;
}

export interface AppState {
  version: string;
  platform: string;
  config: Config;
  repos: RepoSummary[];
  mcp: MCPStatus;
  heatMetrics: Record<string, string>;
  canPickDir: boolean;
  gitFound: boolean;
}

export interface TreeNode {
  name: string;
  path: string;
  dir: boolean;
  size?: number;
  lines?: number;
  language?: string;
  files?: number;
  children?: TreeNode[];
}

export interface FileContent {
  path: string;
  content: string;
  language: string;
  lines: number;
  size: number;
  binary: boolean;
  truncated: boolean;
}

export interface SearchMatch {
  path: string;
  line: number;
  column: number;
  text: string;
}

export interface Param {
  name: string;
  type: string;
}

export type SymbolKind = 'func' | 'method' | 'class' | 'struct' | 'interface' | 'type' | 'external';

export interface CodeSymbol {
  id: string;
  name: string;
  qualified: string;
  kind: SymbolKind;
  language: string;
  file?: string;
  line?: number;
  endLine?: number;
  package?: string;
  receiver?: string;
  parent?: string;
  signature?: string;
  params?: Param[];
  results?: Param[];
  fields?: Param[];
  methods?: string[];
  implements?: string[];
  doc?: string;
  exported: boolean;
  complexity?: number;
  abstract?: boolean;
  heuristic?: boolean;
}

export interface FileSymbol extends CodeSymbol {
  callers: number;
  callees: number;
}

export interface Variable {
  name: string;
  kind: 'var' | 'const';
  type?: string;
  value?: string;
  line: number;
  doc?: string;
  exported: boolean;
  heuristic?: boolean;
}

export interface FileSummary {
  path: string;
  language: string;
  lines: number;
  functions: FileSymbol[];
  types: FileSymbol[];
  variables: Variable[];
  imports: string[];
  heuristic: boolean;
}

export interface EdgeRef {
  symbol: CodeSymbol;
  kind: string;
  lines?: number[];
}

export interface SymbolDetail {
  symbol: CodeSymbol;
  callers: EdgeRef[];
  callees: EdgeRef[];
  implementations?: CodeSymbol[];
  implements?: CodeSymbol[];
  methods?: CodeSymbol[];
  code?: string;
  explanation?: Explanation;
}

export interface GraphNode {
  id: string;
  label: string;
  kind: string;
  sublabel?: string;
  file?: string;
  line?: number;
  endLine?: number;
  group?: string;
  detail?: string;
  depth?: number;
  root?: boolean;
  external?: boolean;
  heat?: number;
  badges?: string[];
  params?: Param[];
  results?: Param[];
  fields?: Param[];
  members?: string[];
  symbol?: string;
  symKind?: SymbolKind;
  heuristic?: boolean;
}

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
  kind: string;
  label?: string;
  count?: number;
  highlight?: boolean;
  heuristic?: boolean;
}

export interface Graph {
  kind: 'callgraph' | 'trace' | 'deps' | 'types' | 'impact' | 'custom' | string;
  title: string;
  root?: string;
  direction?: string;
  nodes: GraphNode[];
  edges: GraphEdge[];
  truncated?: boolean;
  note?: string;
}

export type FlowNodeKind =
  | 'start' | 'end' | 'block' | 'cond' | 'loop' | 'switch' | 'case' | 'return' | 'defer' | 'go' | 'panic' | 'jump';

export interface FlowNode {
  id: string;
  kind: FlowNodeKind;
  label: string;
  code?: string[];
  line?: number;
  endLine?: number;
  calls?: string[];
}

export interface FlowEdge {
  from: string;
  to: string;
  label?: string;
  back?: boolean;
}

export interface Flow {
  symbol?: string;
  name: string;
  file: string;
  line: number;
  endLine: number;
  signature: string;
  inputs: Param[] | null;
  outputs: Param[] | null;
  receiver?: Param;
  nodes: FlowNode[];
  edges: FlowEdge[];
}

export type LineKind = 'context' | 'add' | 'delete';

export interface DiffLine {
  kind: LineKind;
  content: string;
  oldNo?: number;
  newNo?: number;
}

export interface Hunk {
  oldStart: number;
  oldLines: number;
  newStart: number;
  newLines: number;
  header: string;
  lines: DiffLine[];
}

export interface ChangedSymbol {
  id: string;
  name: string;
  kind: string;
  line: number;
  lines: number;
}

export interface DiffFile {
  oldPath: string;
  newPath: string;
  path: string;
  status: 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked';
  binary: boolean;
  additions: number;
  deletions: number;
  hunks: Hunk[] | null;
  language: string;
  symbols: ChangedSymbol[];
}

export interface DiffSpec {
  base?: string;
  head?: string;
  staged?: boolean;
  paths?: string[];
}

export interface DiffOverview {
  spec: DiffSpec;
  files: DiffFile[];
  additions: number;
  deletions: number;
  impact?: Graph;
  note?: string;
}

export interface FileVersions {
  path: string;
  original: string;
  modified: string;
  language: string;
}

export interface Commit {
  hash: string;
  short: string;
  author: string;
  email: string;
  time: string;
  subject: string;
  files?: { path: string; additions: number; deletions: number }[];
  additions: number;
  deletions: number;
}

export interface GitRef {
  name: string;
  kind: 'branch' | 'remote' | 'tag';
  hash: string;
}

export interface StatusEntry {
  path: string;
  origPath?: string;
  index: string;
  worktree: string;
  status: DiffFile['status'];
  staged: boolean;
}

export interface SymHeat {
  id: string;
  name: string;
  line: number;
  value: number;
}

export interface Heatmap {
  metric: string;
  label: string;
  tree: TreeNode;
  values: Record<string, number>;
  details?: Record<string, string>;
  max: number;
  total: number;
  symbols?: Record<string, SymHeat[]>;
}

export interface Overview {
  repo: Repo;
  git: GitInfo;
  stats: {
    files: number;
    symbols: number;
    functions: number;
    types: number;
    edges: number;
    packages: number;
    languages: Record<string, number>;
    errors: number;
    durationMs: number;
  };
  topDirs: { path: string; files: number; lines: number; languages: Record<string, number> }[];
  entryPoints: CodeSymbol[];
  hotspots: { path: string; churn: number; complexity: number; score: number }[];
  readme?: string;
}

export type ProposalStatus = 'pending' | 'applied' | 'rejected' | 'conflict' | 'partial';

export interface FileChange {
  path: string;
  action: 'modify' | 'create' | 'delete';
  rationale?: string;
  original: string;
  proposed: string;
  language: string;
  status: ProposalStatus;
  additions: number;
  deletions: number;
  hunks: Hunk[] | null;
  symbols?: string[];
  error?: string;
}

export interface Proposal {
  id: string;
  repo: string;
  title: string;
  summary: string;
  status: ProposalStatus;
  createdAt: string;
  updatedAt: string;
  changes: FileChange[];
  feedback?: string;
  impact?: Record<string, number>;
}

export interface WalkStep {
  title: string;
  file: string;
  line?: number;
  end_line?: number;
  symbol?: string;
  explanation: string;
}

export interface Walkthrough {
  id: string;
  repo: string;
  title: string;
  summary?: string;
  steps: WalkStep[];
  createdAt: string;
}

export interface Diagram {
  id: string;
  repo: string;
  title: string;
  description?: string;
  mermaid?: string;
  graph?: Graph;
  createdAt: string;
}

export interface Explanation {
  id: string;
  repo: string;
  symbol: string;
  summary: string;
  inputs?: { name: string; description: string }[];
  outputs?: { name: string; description: string }[];
  steps?: { line?: number; end_line?: number; text: string }[];
  sideEffects?: string[];
  notes?: string;
  flow?: Flow;
  sym?: CodeSymbol;
  createdAt: string;
}

export interface Annotation {
  id: string;
  file: string;
  line: number;
  end_line?: number;
  text: string;
  kind: 'info' | 'warning' | 'bug' | 'idea' | 'question' | string;
}

export interface Activity {
  id: number;
  time: string;
  tool: string;
  summary: string;
  repo?: string;
  error?: string;
  durationMs: number;
}

export interface Focus {
  view: string;
  repo: string;
  title?: string;
  params?: Record<string, unknown>;
  message?: string;
}

export interface BusEvent<T = unknown> {
  type: string;
  time: string;
  data: T;
}

export interface SessionData {
  walkthroughs: Walkthrough[];
  diagrams: Diagram[];
  explanations: Explanation[];
  annotations: Annotation[];
}

// ---- QA ----------------------------------------------------------------

export interface TestSuite {
  id: string;
  name: string;
  dir: string;
  command: string;
  report: string;
  tool: string;
  enabled: boolean;
}

export interface TestCase {
  id: string;
  name: string;
  behavior: string;
  kind: 'test' | 'subtest' | 'case' | 'suite' | 'benchmark' | 'fuzz' | 'example' | 'todo';
  framework: string;
  file: string;
  line: number;
  endLine: number;
  parent?: string;
  suite?: string[];
  skipped?: boolean;
  targets?: string[];
}

export interface TestabilityHint {
  kind: string;
  line?: number;
  match?: string;
  problem: string;
  suggestion: string;
}

export interface FuncQA {
  id: string;
  name: string;
  kind: string;
  file: string;
  line: number;
  endLine: number;
  complexity: number;
  callers: number;
  exported: boolean;
  covered: number;
  lines: number;
  /** -1 when there is no coverage data for the function. */
  pct: number;
  tests?: string[];
  direct: number;
  indirect: number;
  risk: number;
  reason?: string;
  uncovered?: string;
  hints?: TestabilityHint[];
  heuristic?: boolean;
}

export interface CoverageSource {
  path: string;
  format: string;
  suite?: string;
  files: number;
  skipped?: number;
  modTime: string;
}

export interface CoverageSummary {
  covered: number;
  lines: number;
  pct: number;
  generated: string;
  sources: CoverageSource[];
  stale: number;
}

export interface FileQA {
  path: string;
  covered: number;
  lines: number;
  pct: number;
  functions: number;
  untested: number;
  stale?: boolean;
}

export interface SuiteResult {
  id: string;
  name: string;
  state: 'pending' | 'running' | 'passed' | 'failed' | 'error' | 'cancelled';
  exitCode: number;
  failures?: string[];
  report?: string;
  files: number;
  error?: string;
  durationMs: number;
}

export interface QARun {
  id: string;
  repo: string;
  state: 'running' | 'done' | 'failed' | 'error' | 'cancelled';
  started: string;
  finished?: string;
  suites: SuiteResult[];
  log: string[] | null;
}

export interface PlannedBehavior {
  name: string;
  kind?: string;
  given?: string;
  when?: string;
  then?: string;
  line?: number;
  end_line?: number;
  existing?: string;
}

export interface TestRefactor {
  title: string;
  detail?: string;
  file?: string;
  line?: number;
  end_line?: number;
}

export interface TestPlan {
  id: string;
  repo: string;
  title: string;
  summary?: string;
  symbol?: string;
  symbolName?: string;
  file?: string;
  line?: number;
  endLine?: number;
  testFile?: string;
  behaviors: PlannedBehavior[];
  refactors?: TestRefactor[];
  created: string;
}

export interface QAOverview {
  coverage?: CoverageSummary;
  tests: { files: number; tests: number; behaviors: number; suites: number; skipped: number; frameworks: Record<string, number> };
  functions: { total: number; covered: number; full: number; uncovered: number; tested: number; reached: number };
  files: FileQA[];
  testFiles: { path: string; framework: string; tests: TestCase[] }[];
  gaps: FuncQA[];
  suites: TestSuite[];
  detected: boolean;
  run?: QARun;
  plans: TestPlan[];
  targets: Record<string, { name: string; file: string; line: number }>;
}

export interface FileCoverage {
  path: string;
  available: boolean;
  covered: number[];
  uncovered: number[];
  partial: number[];
  pct: number;
  stale?: boolean;
  generated?: string;
  functions: FuncQA[];
  tests?: TestCase[];
}

export interface SymbolTests {
  function: FuncQA;
  direct: TestCase[];
  indirect: TestCase[];
}
