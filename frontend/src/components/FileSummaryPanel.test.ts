import { arrangeFunctions } from './FileSummaryPanel';
import type { FileSymbol } from '../lib/types';

const fn = (name: string, line: number, complexity: number, callers: number, doc = ''): FileSymbol => ({
  id: name, name, qualified: name, kind: 'func', language: 'go', line, exported: true, complexity, callers, callees: 0, doc,
});

describe('arrangeFunctions', () => {
  const list = [fn('b', 20, 3, 9), fn('a', 10, 12, 1, 'parses input'), fn('c', 30, 12, 4)];
  it('sorts by line, complexity or usage', () => {
    expect(arrangeFunctions(list, '', 'line').map((f) => f.name)).toEqual(['a', 'b', 'c']);
    expect(arrangeFunctions(list, '', 'complexity').map((f) => f.name)).toEqual(['a', 'c', 'b']);
    expect(arrangeFunctions(list, '', 'callers').map((f) => f.name)).toEqual(['b', 'c', 'a']);
  });
  it('filters by name or doc without mutating input', () => {
    expect(arrangeFunctions(list, 'PARSES', 'line').map((f) => f.name)).toEqual(['a']);
    expect(list[0].name).toBe('b');
  });
});
