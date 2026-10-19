// Pure helpers for a change's blast radius and tests panels (unit-tested
// in change.test.ts).
import type { ChangeFile, ChangeReport, Graph, Impact, ImpactNode } from '../../lib/types';
import { pctLabel } from '../qa/qa';

/** Every node of a blast radius by id. */
export function nodesById(im: Impact): Map<string, ImpactNode> {
  const m = new Map<string, ImpactNode>();
  for (const n of im.changed) m.set(n.id, n);
  for (const r of im.rings) for (const n of r.nodes) m.set(n.id, n);
  for (const n of im.entries) m.set(n.id, n);
  return m;
}

/**
 * The call path from a caller back to the change, e.g. ["main",
 * "Handler.CreateOrder", "validate"]. Hops left out of capped lists show
 * by name only where the next node still knows them.
 */
export function pathToChange(n: ImpactNode, byId: Map<string, ImpactNode>): string[] {
  const out = [n.name];
  let cur: ImpactNode | undefined = n;
  for (let i = 0; cur?.via && i < 8; i++) {
    const next: ImpactNode | undefined = cur.viaId ? byId.get(cur.viaId) : undefined;
    out.push(next?.name ?? cur.via);
    cur = next;
  }
  return out;
}

/** Short test status of a node: "82% · 3 direct · 2 indirect". */
export function testStatus(n: ImpactNode): string {
  const parts: string[] = [];
  if (n.pct >= 0) parts.push(pctLabel(n.pct));
  if (n.direct) parts.push(`${n.direct} direct`);
  if (n.indirect) parts.push(`${n.indirect} indirect`);
  return parts.length ? parts.join(' · ') : 'no tests';
}

/** The blast radius as a package graph: the changed packages at the center. */
export function packageGraph(im: Impact): Graph {
  const nodes = Object.entries(im.packageDepth).map(([path, depth]) => ({
    id: path,
    label: path === '.' ? '(root)' : path,
    kind: 'package',
    depth,
    root: depth === 0,
    sublabel: depth === 0 ? 'changed' : `${depth} call${depth > 1 ? 's' : ''} away`,
  }));
  const edges = im.packageEdges.map((e) => ({ id: `${e.from}→${e.to}`, from: e.from, to: e.to, kind: 'call', count: e.count, label: e.count > 1 ? String(e.count) : undefined }));
  return { kind: 'impact', title: 'Blast radius by package', direction: 'RL', nodes, edges };
}

/** Line marks for one side of a diff editor. */
export interface LineMark {
  line: number;
  kind: 'hit' | 'part' | 'miss';
  hover: string;
}

/** Coverage marks for a file's changed lines, on the side they number. */
export function diffMarks(r: ChangeReport | undefined, path: string): { original?: LineMark[]; modified?: LineMark[] } {
  // Commit line numbers don't match current coverage well enough to mark.
  const f = r?.coverage.approximate ? undefined : r?.coverage.files.find((x) => x.path === path);
  if (!f) return {};
  const names = new Map(r!.coverage.tests.map((t) => [t.id, t.name]));
  const marks: LineMark[] = [];
  for (const l of f.lines) {
    if (l.state === 'unknown') continue;
    const by = (l.tests ?? []).map((id) => names.get(id) ?? id);
    const hover =
      l.state === 'uncovered'
        ? 'No test runs this line'
        : by.length
          ? `Run by ${by.slice(0, 5).join(', ')}${by.length > 5 ? ` and ${by.length - 5} more` : ''}`
          : `Covered${l.state === 'partial' ? ' (some branches not)' : ''}${f.measured ? '' : '; run per-test coverage to see by which tests'}`;
    marks.push({ line: l.line, kind: l.state === 'covered' ? 'hit' : l.state === 'partial' ? 'part' : 'miss', hover });
  }
  return f.side === 'original' ? { original: marks } : { modified: marks };
}

/** Uncovered changed lines of a file as compact ranges ("3-5, 9"). */
export function uncoveredRanges(f: ChangeFile): { from: number; to: number }[] {
  const lines = f.lines.filter((l) => l.state === 'uncovered').map((l) => l.line);
  const out: { from: number; to: number }[] = [];
  for (const n of lines) {
    const last = out[out.length - 1];
    if (last && n === last.to + 1) last.to = n;
    else out.push({ from: n, to: n });
  }
  return out;
}

/** A prompt asking the agent to cover the untested parts of a change. */
export function testPrompt(r: ChangeReport): string {
  const parts: string[] = [];
  for (const f of r.coverage.files) {
    const rs = uncoveredRanges(f);
    if (rs.length) parts.push(`${f.path}:${rs.map((x) => (x.from === x.to ? x.from : `${x.from}-${x.to}`)).join(',')}`);
  }
  const fns = r.coverage.untested.map((n) => n.name);
  const callers = (r.impact.rings.find((x) => x.depth === 1)?.nodes ?? []).filter((n) => n.review).map((n) => n.name);
  const lines = [`Add tests for ${r.title ? `"${r.title}"` : 'this change'}.`];
  if (parts.length) lines.push(`Changed lines no test runs: ${parts.join('; ')}.`);
  if (fns.length) lines.push(`Changed functions no test reaches: ${fns.join(', ')}.`);
  if (callers.length) lines.push(`Untested direct callers: ${callers.join(', ')}.`);
  lines.push('Use get_tests_for_change for details, then propose_changes with the tests.');
  return lines.join('\n');
}

/** Whether a report has anything a test prompt could ask for. */
export function hasGaps(r: ChangeReport): boolean {
  return r.coverage.uncovered > 0 || r.coverage.untested.length > 0 || r.impact.summary.untested > 0;
}
