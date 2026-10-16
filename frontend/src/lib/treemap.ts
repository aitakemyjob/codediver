// Squarified treemap layout for the heatmap.
import { hierarchy, treemap, treemapSquarify, type HierarchyRectangularNode } from 'd3-hierarchy';
import type { TreeNode } from './types';

export interface Cell {
  path: string;
  name: string;
  dir: boolean;
  depth: number;
  x0: number;
  y0: number;
  x1: number;
  y1: number;
  /** Area weight (lines, min 1). */
  size: number;
  /** Metric value (sum for directories). */
  value: number;
  /** Hottest single file inside (equals value for files); used to color collapsed folders. */
  peak: number;
}

/** Find a subtree by path ("" for root). */
export function subtree(root: TreeNode, path: string): TreeNode | undefined {
  if (!path) return root;
  let cur: TreeNode | undefined = root;
  const parts = path.split('/');
  for (let i = 0; i < parts.length && cur; i++) {
    const prefix = parts.slice(0, i + 1).join('/');
    cur = cur.children?.find((c) => c.path === prefix);
  }
  return cur;
}

/**
 * Lays out a squarified treemap: area by line count, value by metric.
 * Directories deeper than maxDepth are not emitted (they show as blocks).
 */
export function layoutTreemap(root: TreeNode, values: Record<string, number>, width: number, height: number, maxDepth = 3): Cell[] {
  const h = hierarchy<TreeNode>(root, (d) => (d.dir ? d.children : undefined))
    .sum((d) => (d.dir ? 0 : Math.max(d.lines ?? 0, 1)))
    .sort((a, b) => (b.value ?? 0) - (a.value ?? 0));
  const laid = treemap<TreeNode>()
    .tile(treemapSquarify.ratio(1.3))
    .size([width, height])
    .paddingOuter(3)
    .paddingTop((d) => (d.depth === 0 ? 0 : d.data.dir ? 20 : 0))
    .paddingInner(2)
    .round(true)(h);

  const metric = (n: HierarchyRectangularNode<TreeNode>): number => {
    if (!n.data.dir) return values[n.data.path] ?? 0;
    return (n.children ?? []).reduce((s, c) => s + metric(c), 0);
  };
  const peak = (n: HierarchyRectangularNode<TreeNode>): number => {
    if (!n.data.dir) return values[n.data.path] ?? 0;
    return (n.children ?? []).reduce((m, c) => Math.max(m, peak(c)), 0);
  };
  const cells: Cell[] = [];
  laid.each((n) => {
    if (n.depth === 0 || n.depth > maxDepth) return;
    // Beyond maxDepth, directories render as solid blocks.
    cells.push({
      path: n.data.path,
      name: n.data.name,
      dir: n.data.dir,
      depth: n.depth,
      x0: n.x0, y0: n.y0, x1: n.x1, y1: n.y1,
      size: n.value ?? 0,
      value: metric(n),
      peak: peak(n),
    });
  });
  return cells;
}

/** Hottest single file across the layout — the color scale's maximum. */
export function maxLeaf(cells: Cell[]): number {
  let m = 0;
  for (const c of cells) m = Math.max(m, c.peak);
  return m;
}
