import { describe, expect, it } from 'vitest';
import { FlowTrail } from './flowTrail';
import type { Flow, FlowNode } from './types';

/** Builds a flow with the given nodes. */
function flow(name: string, line: number, nodes: FlowNode[]): Flow {
  return { name, file: 'a.go', line, endLine: line + 5, signature: 'func()', inputs: null, outputs: null, nodes, edges: [] };
}

const inner = flow('func literal (L4)', 4, [{ id: 'n0', kind: 'start', label: 'func()' }]);
const lit = flow('func literal (L3)', 3, [
  { id: 'n0', kind: 'start', label: 'func()' },
  { id: 'n2', kind: 'block', label: '', closures: [inner] },
]);
const root = flow('F', 1, [
  { id: 'n0', kind: 'start', label: 'F()' },
  { id: 'n2', kind: 'block', label: '', closures: [lit] },
]);

describe('FlowTrail', () => {
  it('starts at the root', () => {
    const t = new FlowTrail(root);
    expect(t.current()).toBe(root);
    expect(t.depth()).toBe(0);
    expect(t.crumbs()).toEqual([{ label: 'F', depth: 0 }]);
  });

  it('opens nested closures and goes back', () => {
    const t = new FlowTrail(root).open('n2', 0).open('n2', 0);
    expect(t.current()).toBe(inner);
    expect(t.crumbs().map((c) => c.label)).toEqual(['F', 'closure L3', 'closure L4']);
    expect(t.back(1).current()).toBe(lit);
    expect(t.back(0).current()).toBe(root);
  });

  it('ignores closures that do not exist', () => {
    const t = new FlowTrail(root);
    expect(t.open('n9', 0)).toBe(t);
    expect(t.open('n2', 3)).toBe(t);
  });

  it('stops at a step that no longer resolves', () => {
    const t = new FlowTrail(root).open('n2', 0);
    const rebuilt = new FlowTrail(flow('F', 1, [{ id: 'n0', kind: 'start', label: 'F()' }]), [{ nodeId: 'n2', index: 0 }]);
    expect(t.depth()).toBe(1);
    expect(rebuilt.depth()).toBe(0);
  });
});
