import { vi } from 'vitest';

// Views import Monaco/React Flow; stub the heavy modules so the pure helpers
// can be tested in jsdom.
vi.mock('../lib/monaco', () => ({ monaco: {}, setMonacoTheme: () => {} }));

import { stepNodes } from './FlowView';
import { isTypingTarget } from './WalkthroughView';
import { fuzzyScore } from '../components/CommandPalette';
import type { Flow } from '../lib/types';

const flow: Flow = {
  name: 'F', file: 'f.go', line: 1, endLine: 20, signature: 'func F()', inputs: [], outputs: [],
  nodes: [
    { id: 'n0', kind: 'start', label: 'F()', line: 1 },
    { id: 'n1', kind: 'block', label: '', line: 2, endLine: 4 },
    { id: 'n2', kind: 'cond', label: 'if x', line: 5 },
    { id: 'n3', kind: 'return', label: 'return', line: 9 },
    { id: 'n4', kind: 'end', label: 'end', line: 20 },
  ],
  edges: [],
};

describe('stepNodes', () => {
  it('pins each step to the first overlapping block', () => {
    const { notes, byStep } = stepNodes(flow, {
      id: 'x', repo: 'r', symbol: 's', summary: '', createdAt: '',
      steps: [{ line: 3, end_line: 5, text: 'setup' }, { line: 9, text: 'done' }, { text: 'no lines' }],
    });
    expect([...byStep.get(0)!]).toEqual(['n1', 'n2']);
    expect(notes.get('n1')).toEqual({ text: 'setup', n: 1 });
    expect(notes.get('n3')).toEqual({ text: 'done', n: 2 });
    expect(byStep.has(2)).toBe(false);
    expect(notes.has('n0')).toBe(false); // start/end never get notes
  });
});

describe('fuzzyScore', () => {
  it('ranks substrings above subsequences and drops scattered matches', () => {
    expect(fuzzyScore('Index.TraceTo', 'trace')).toBeGreaterThan(fuzzyScore('Index.TraceTo', 'itt'));
    expect(fuzzyScore('internal/mcpserver/tools.go', 'TraceTo')).toBe(0);
    expect(fuzzyScore('anything', '')).toBe(1);
  });
});

describe('isTypingTarget', () => {
  it('ignores keys typed into the editor or fields, and handles non-element targets', () => {
    document.body.innerHTML = '<div class="monaco-editor"><span id="code"></span></div><input id="field"><button id="btn"></button>';
    expect(isTypingTarget(document.getElementById('code'))).toBe(true);
    expect(isTypingTarget(document.getElementById('field'))).toBe(true);
    expect(isTypingTarget(document.getElementById('btn'))).toBe(false);
    // Events dispatched on window/document used to crash the handler.
    expect(isTypingTarget(window)).toBe(false);
    expect(isTypingTarget(document)).toBe(false);
    expect(isTypingTarget(null)).toBe(false);
  });
});
