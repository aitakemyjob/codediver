import { countBehaviors, coverageByDir, coverageTone, matchesQuery, pctLabel, testTree } from './qa';
import type { FileQA, TestCase } from '../../lib/types';

const tc = (id: string, behavior: string, kind: TestCase['kind'] = 'test', parent?: string): TestCase => ({
  id, name: behavior, behavior, kind, framework: 'vitest', file: 'a.test.ts', line: 1, endLine: 2, parent,
});

describe('qa helpers', () => {
  it('formats and colors coverage', () => {
    expect(pctLabel(-1)).toBe('—');
    expect(pctLabel(0)).toBe('0%');
    expect(pctLabel(76.24)).toBe('76.2%');
    expect(pctLabel(100)).toBe('100%');
    expect(coverageTone(-1)).toBe('var(--text-3)');
    expect(coverageTone(10)).toBe('var(--err)');
    expect(coverageTone(65)).toBe('var(--warn)');
    expect(coverageTone(92)).toBe('var(--ok)');
  });

  it('aggregates coverage by directory, worst first', () => {
    const files: FileQA[] = [
      { path: 'a/x.go', covered: 9, lines: 10, pct: 90, functions: 1, untested: 0 },
      { path: 'a/y.go', covered: 1, lines: 10, pct: 10, functions: 2, untested: 1 },
      { path: 'b/z.go', covered: 0, lines: 4, pct: 0, functions: 1, untested: 1 },
      { path: 'root.go', covered: 3, lines: 3, pct: 100, functions: 1, untested: 0 },
      { path: 'c/empty.go', covered: 0, lines: 0, pct: -1, functions: 0, untested: 0 },
    ];
    const dirs = coverageByDir(files);
    expect(dirs.map((d) => [d.path, d.pct, d.files, d.untested])).toEqual([
      ['b', 0, 1, 1],
      ['a', 50, 2, 1],
      ['.', 100, 1, 0],
    ]);
  });

  it('nests tests under their parents and counts behaviors', () => {
    const tests = [tc('s', 'Menu', 'suite'), tc('t1', 'opens', 'test', 's'), tc('t2', 'closes', 'test', 's'), tc('t3', 'top level'), tc('c1', 'case', 'case', 't3')];
    const tree = testTree(tests);
    expect(tree.map((n) => n.test.id)).toEqual(['s', 't3']);
    expect(tree[0].children.map((n) => n.test.id)).toEqual(['t1', 't2']);
    expect(countBehaviors(tree)).toBe(4);
    expect(matchesQuery(tree[0], 'clos')).toBe(true);
    expect(matchesQuery(tree[1], 'clos')).toBe(false);
    expect(matchesQuery(tree[1], '')).toBe(true);
  });
});
