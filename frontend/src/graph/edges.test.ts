import { describe, expect, it } from 'vitest';
import { toFlowEdge } from './edges';

describe('toFlowEdge', () => {
  it('labels and dots a guessed call', () => {
    const e = toFlowEdge({ id: 'e', from: 'a', to: 'b', kind: 'call', guessed: true });
    expect(e.label).toBe('guess?');
    expect(e.style?.strokeDasharray).toBe('1 4');
  });

  it('keeps the count beside the guess', () => {
    expect(toFlowEdge({ id: 'e', from: 'a', to: 'b', kind: 'call', count: 3, guessed: true }).label).toBe('×3 · guess?');
  });

  it('leaves resolved calls as they were', () => {
    const e = toFlowEdge({ id: 'e', from: 'a', to: 'b', kind: 'call' });
    expect(e.label).toBeUndefined();
    expect(e.style?.strokeDasharray).toBeUndefined();
  });
});
