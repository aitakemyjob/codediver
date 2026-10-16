import { activate, closeOthers, closeTab, focusToTab, openTab, tabKey, type TabState } from './tabs';

describe('tabs', () => {
  it('keys identify logical views', () => {
    expect(tabKey('code', { file: 'a.go', line: 3 })).toBe(tabKey('code', { file: 'a.go', line: 9 }));
    expect(tabKey('graph', { kind: 'callgraph', symbol: 'x', direction: 'callees' })).not.toBe(
      tabKey('graph', { kind: 'callgraph', symbol: 'x', direction: 'callers' }),
    );
    expect(tabKey('heatmap', { metric: 'churn' })).toBe(tabKey('heatmap', { metric: 'size' }));
  });

  it('opens after the active tab and re-focuses duplicates', () => {
    let s: TabState = { tabs: [] };
    s = openTab(s, { kind: 'overview', title: 'Overview', params: {} });
    const first = s.active!;
    s = openTab(s, { kind: 'code', title: 'a.go', params: { file: 'a.go' } });
    s = activate(s, first);
    s = openTab(s, { kind: 'code', title: 'b.go', params: { file: 'b.go' } });
    expect(s.tabs.map((t) => t.title)).toEqual(['Overview', 'b.go', 'a.go']);

    s = openTab(s, { kind: 'code', title: 'a.go', params: { file: 'a.go', line: 42 } });
    expect(s.tabs).toHaveLength(3);
    expect(s.tabs.find((t) => t.id === s.active)!.params.line).toBe(42);
  });

  it('closes and picks a neighbour', () => {
    let s: TabState = { tabs: [] };
    for (const f of ['a', 'b', 'c']) s = openTab(s, { kind: 'code', title: f, params: { file: f } });
    const [a, b, c] = s.tabs.map((t) => t.id);
    s = activate(s, b);
    s = closeTab(s, b);
    expect(s.active).toBe(c);
    s = closeTab(s, c);
    expect(s.active).toBe(a);
    s = closeTab(s, a);
    expect(s.active).toBeUndefined();
    expect(closeTab(s, 'missing')).toBe(s);
  });

  it('caps the number of tabs', () => {
    let s: TabState = { tabs: [] };
    for (let i = 0; i < 20; i++) s = openTab(s, { kind: 'code', title: `${i}`, params: { file: `${i}` } });
    expect(s.tabs).toHaveLength(14);
    expect(s.tabs.some((t) => t.id === s.active)).toBe(true);
  });

  it('close others and clears agent marker on activate', () => {
    let s: TabState = { tabs: [] };
    s = openTab(s, { kind: 'code', title: 'a', params: { file: 'a' }, agent: true });
    const a = s.active!;
    s = openTab(s, { kind: 'code', title: 'b', params: { file: 'b' } });
    s = activate(s, a);
    expect(s.tabs.find((t) => t.id === a)!.agent).toBe(false);
    expect(closeOthers(s, a).tabs).toHaveLength(1);
  });

  it('maps agent focus requests to tabs', () => {
    expect(focusToTab({ view: 'code', repo: 'r', params: { file: 'pkg/x.go', line: 3 } })).toMatchObject({ kind: 'code', title: 'x.go', agent: true });
    expect(focusToTab({ view: 'explain', repo: 'r', title: 'F', params: { symbol: 's' } })!.kind).toBe('flow');
    expect(focusToTab({ view: 'proposal', repo: 'r', params: { id: 'p1' } })!.kind).toBe('proposal');
    expect(focusToTab({ view: 'bogus', repo: 'r' })).toBeNull();
  });

  it('keeps an earlier agent message when the same file is focused again', () => {
    let s: TabState = { tabs: [] };
    s = openTab(s, focusToTab({ view: 'code', repo: 'r', message: 'look here', params: { file: 'a.go', line: 3 } })!);
    s = openTab(s, focusToTab({ view: 'code', repo: 'r', params: { file: 'a.go', summary: true } })!);
    expect(s.tabs).toHaveLength(1);
    expect(s.tabs[0].params).toMatchObject({ message: 'look here', summary: true, line: 3 });
  });
});
