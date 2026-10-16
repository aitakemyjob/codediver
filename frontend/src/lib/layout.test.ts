import { layout, textWidth, toElkDirection } from './layout';

describe('layout', () => {
  it('lays out a chain left to right without overlaps', async () => {
    const nodes = ['a', 'b', 'c'].map((id) => ({ id, width: 100, height: 40 }));
    const edges = [
      { id: 'ab', source: 'a', target: 'b' },
      { id: 'bc', source: 'b', target: 'c' },
      { id: 'dangling', source: 'a', target: 'missing' },
      { id: 'self', source: 'a', target: 'a' },
    ];
    const pos = await layout(nodes, edges, 'RIGHT');
    expect(pos.size).toBe(3);
    expect(pos.get('a')!.x).toBeLessThan(pos.get('b')!.x);
    expect(pos.get('b')!.x).toBeLessThan(pos.get('c')!.x);
    expect(pos.get('b')!.x - pos.get('a')!.x).toBeGreaterThanOrEqual(100);
  });

  it('lays out top to bottom and survives cycles', async () => {
    const nodes = ['a', 'b'].map((id) => ({ id, width: 80, height: 30 }));
    const pos = await layout(nodes, [{ id: '1', source: 'a', target: 'b' }, { id: '2', source: 'b', target: 'a' }], 'DOWN');
    expect(pos.get('a')!.y).not.toBe(pos.get('b')!.y);
  });

  it('maps direction aliases', () => {
    expect(toElkDirection('TB')).toBe('DOWN');
    expect(toElkDirection('RL')).toBe('LEFT');
    expect(toElkDirection(undefined)).toBe('RIGHT');
    expect(textWidth('abcd', 10)).toBeGreaterThan(0);
  });
});
