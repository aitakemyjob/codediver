// Shared pieces of the QA view: formatting, coverage bars and the pure
// helpers that shape backend data for display (unit-tested in qa.test.ts).
import type { FileQA, TestCase } from '../../lib/types';

/** Formats a coverage percentage; negative means unknown. */
export function pctLabel(p: number): string {
  if (p < 0) return '—';
  if (p > 0 && p < 0.05) return '<0.1%';
  return `${p >= 99.95 || p === 0 ? p.toFixed(0) : p.toFixed(1)}%`;
}

/** Color token for a coverage level: red below 50%, amber below 80%, green above. */
export function coverageTone(p: number): string {
  if (p < 0) return 'var(--text-3)';
  if (p < 50) return 'var(--err)';
  if (p < 80) return 'var(--warn)';
  return 'var(--ok)';
}

/** Thin horizontal coverage bar with the percentage beside it. */
export function CoverageBar({ pct, width = 90, label = true }: { pct: number; width?: number; label?: boolean }) {
  return (
    <span className="covbar" title={pct < 0 ? 'no coverage data' : `${pctLabel(pct)} of instrumented lines ran`}>
      <span className="covbar-track" style={{ width }}>
        {pct >= 0 && <span className="covbar-fill" style={{ width: `${Math.max(pct, 1.5)}%`, background: coverageTone(pct) }} />}
      </span>
      {label && <span className="mono covbar-pct" style={{ color: coverageTone(pct) }}>{pctLabel(pct)}</span>}
    </span>
  );
}

export interface DirCoverage {
  path: string;
  covered: number;
  lines: number;
  pct: number;
  files: number;
  untested: number;
}

/** Aggregates per-file coverage into directories, worst first. */
export function coverageByDir(files: FileQA[]): DirCoverage[] {
  const dirs = new Map<string, DirCoverage>();
  for (const f of files) {
    if (f.lines === 0) continue;
    const dir = f.path.includes('/') ? f.path.slice(0, f.path.lastIndexOf('/')) : '.';
    const d = dirs.get(dir) ?? { path: dir, covered: 0, lines: 0, pct: 0, files: 0, untested: 0 };
    d.covered += f.covered;
    d.lines += f.lines;
    d.files++;
    d.untested += f.untested;
    dirs.set(dir, d);
  }
  return [...dirs.values()]
    .map((d) => ({ ...d, pct: Math.round((1000 * d.covered) / d.lines) / 10 }))
    .sort((a, b) => a.pct - b.pct || b.lines - a.lines);
}

export interface TestNode {
  test: TestCase;
  children: TestNode[];
}

/** Nests a file's tests under their parents (describe blocks, Test funcs). */
export function testTree(tests: TestCase[]): TestNode[] {
  const nodes = new Map<string, TestNode>();
  for (const t of tests) nodes.set(t.id, { test: t, children: [] });
  const roots: TestNode[] = [];
  for (const t of tests) {
    const n = nodes.get(t.id)!;
    const parent = t.parent ? nodes.get(t.parent) : undefined;
    if (parent && parent !== n) parent.children.push(n);
    else roots.push(n);
  }
  return roots;
}

/** Counts runnable behaviors (everything except suites) in a subtree. */
export function countBehaviors(nodes: TestNode[]): number {
  let n = 0;
  for (const x of nodes) {
    if (x.test.kind !== 'suite') n++;
    n += countBehaviors(x.children);
  }
  return n;
}

/** Whether a test or any descendant matches a lower-case query. */
export function matchesQuery(node: TestNode, q: string): boolean {
  if (!q) return true;
  const t = node.test;
  if (t.behavior.toLowerCase().includes(q) || t.name.toLowerCase().includes(q) || t.file.toLowerCase().includes(q)) return true;
  return node.children.some((c) => matchesQuery(c, q));
}

/** Human labels for testability hint kinds. */
export const hintLabels: Record<string, string> = {
  clock: 'Clock', random: 'Randomness', env: 'Env vars', network: 'Network', filesystem: 'Filesystem', process: 'Processes',
  sleep: 'Real time', exit: 'Exits process', global: 'Global state', browser: 'Browser globals', complexity: 'Many branches', params: 'Many params',
};

/** A prompt the developer can paste into their agent to close a gap. */
export function gapPrompt(name: string, file: string): string {
  return `Using CodeDiver, look at the test gap for ${name} in ${file}: check its existing tests and uncovered lines, then show me a test plan (show_test_plan) and propose the tests.`;
}
