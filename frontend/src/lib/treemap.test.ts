import { layoutTreemap, maxLeaf, subtree } from './treemap';
import type { TreeNode } from './types';

const tree: TreeNode = {
  name: 'root', path: '', dir: true,
  children: [
    { name: 'api', path: 'api', dir: true, children: [
      { name: 'a.go', path: 'api/a.go', dir: false, lines: 300 },
      { name: 'b.go', path: 'api/b.go', dir: false, lines: 100 },
    ] },
    { name: 'main.go', path: 'main.go', dir: false, lines: 100 },
  ],
};

describe('treemap', () => {
  it('areas follow line counts and dirs aggregate metric values', () => {
    const cells = layoutTreemap(tree, { 'api/a.go': 5, 'main.go': 1 }, 800, 400);
    const by = Object.fromEntries(cells.map((c) => [c.path, c]));
    const area = (p: string) => (by[p].x1 - by[p].x0) * (by[p].y1 - by[p].y0);
    expect(area('api/a.go')).toBeGreaterThan(area('api/b.go') * 2);
    expect(by['api'].value).toBe(5);
    expect(by['api/b.go'].value).toBe(0);
    for (const c of cells) {
      expect(c.x0).toBeGreaterThanOrEqual(0);
      expect(c.x1).toBeLessThanOrEqual(800);
    }
    expect(maxLeaf(cells)).toBe(5);
    expect(by['api'].peak).toBe(5);
  });

  it('respects max depth', () => {
    const cells = layoutTreemap(tree, {}, 400, 200, 1);
    expect(cells.every((c) => c.depth <= 1)).toBe(true);
  });

  it('finds subtrees', () => {
    expect(subtree(tree, 'api')?.children).toHaveLength(2);
    expect(subtree(tree, '')).toBe(tree);
    expect(subtree(tree, 'nope')).toBeUndefined();
  });
});
