import { useStore } from './store';
import { setTransport, type Transport } from './transport';
import type { AppState, BusEvent, Proposal } from './types';

const appState: AppState = {
  version: 'test',
  platform: 'linux',
  config: {
    repos: [
      { id: 'r1', name: 'shop', path: '/x', addedAt: '', mcpEnabled: true, exclude: [] },
      { id: 'r2', name: 'firmware', path: '/y', addedAt: '', mcpEnabled: true, exclude: [] },
    ],
    activeRepo: 'r1',
    settings: { theme: 'nord', fontSize: 13, mcpPort: 47474, mcpEnabled: true, followAgent: true, churnDays: 180 },
  },
  repos: [],
  mcp: { running: true, url: 'http://127.0.0.1:47474/mcp' },
  heatMetrics: {},
  canPickDir: false,
  gitFound: true,
};

function fakeTransport() {
  let listener: ((e: BusEvent) => void) | undefined;
  const calls: string[] = [];
  const t: Transport = {
    kind: 'browser',
    async invoke<T>(method: string, args?: unknown): Promise<T> {
      calls.push(method);
      switch (method) {
        case 'repos.setActive':
          appState.config.activeRepo = (args as { id: string }).id;
          return null as T;
        case 'app.state':
          return appState as T;
        case 'activity.list':
        case 'proposals.list':
          return [] as T;
        case 'session.all':
          return { walkthroughs: [], diagrams: [], explanations: [], annotations: [] } as T;
      }
      return null as T;
    },
    subscribe(fn) {
      listener = fn;
      return () => {};
    },
  };
  return { t, calls, emit: (e: BusEvent) => listener?.(e) };
}

const ev = (type: string, data: unknown): BusEvent => ({ type, time: new Date().toISOString(), data });

describe('store', () => {
  it('initializes, applies the configured theme and opens the overview', async () => {
    const f = fakeTransport();
    setTransport(f.t);
    await useStore.getState().init();
    const s = useStore.getState();
    expect(s.ready).toBe(true);
    expect(s.repoId).toBe('r1');
    expect(s.theme.id).toBe('nord');
    expect(document.documentElement.dataset.theme).toBe('nord');
    expect(s.tabs[0].kind).toBe('overview');

    // Agent focus opens a tab when following.
    f.emit(ev('focus', { view: 'code', repo: 'r1', params: { file: 'api/api.go', line: 10 } }));
    const st = useStore.getState();
    const active = st.tabs.find((t) => t.id === st.active)!;
    expect(active.kind).toBe('code');
    expect(active.agent).toBe(true);

    // Focus for another repo is ignored.
    f.emit(ev('focus', { view: 'code', repo: 'other', params: { file: 'z.go' } }));
    expect(useStore.getState().tabs.some((t) => t.params.file === 'z.go')).toBe(false);

    // New proposals are upserted and announced.
    const p: Proposal = { id: 'p1', repo: 'r1', title: 'Fix', summary: '', status: 'pending', createdAt: '', updatedAt: '', changes: [] };
    f.emit(ev('proposal', p));
    f.emit(ev('proposal', { ...p, status: 'applied' }));
    expect(useStore.getState().proposals).toHaveLength(1);
    expect(useStore.getState().proposals[0].status).toBe('applied');
    expect(useStore.getState().toasts.some((t) => t.title === 'Agent proposed changes')).toBe(true);

    f.emit(ev('activity', { id: 1, time: '', tool: 'read_file', summary: 'x', durationMs: 1 }));
    expect(useStore.getState().activity).toHaveLength(1);

    f.emit(ev('index', { repo: 'r1', state: 'building' }));
    expect(useStore.getState().indexing).toBe(true);
    f.emit(ev('index', { repo: 'r1', state: 'ready', stats: { files: 3, functions: 4, edges: 5, durationMs: 6 } }));
    expect(useStore.getState().indexStats?.functions).toBe(4);
  });

  it('tracks coverage runs, streams their log and announces test plans', async () => {
    const f = fakeTransport();
    setTransport(f.t);
    await useStore.getState().init();
    const v0 = useStore.getState().qaVersion;
    const run = { id: 'run1', repo: 'r1', state: 'running', started: '', suites: [{ id: 'go:', name: 'Go', state: 'running', exitCode: 0, files: 0, durationMs: 0 }], log: null };
    f.emit(ev('qa', { repo: 'r1', kind: 'run', run }));
    f.emit(ev('qa', { repo: 'r1', kind: 'log', run: 'run1', lines: ['ok  pkg/a', 'ok  pkg/b'] }));
    f.emit(ev('qa', { repo: 'other', kind: 'log', run: 'x', lines: ['ignored'] }));
    expect(useStore.getState().qaRun?.log).toEqual(['ok  pkg/a', 'ok  pkg/b']);
    expect(useStore.getState().qaVersion).toBe(v0);

    // Status updates keep the streamed log; finishing bumps the version and toasts.
    f.emit(ev('qa', { repo: 'r1', kind: 'run', run: { ...run, state: 'done', suites: [{ ...run.suites[0], state: 'passed' }] } }));
    const s = useStore.getState();
    expect(s.qaRun?.state).toBe('done');
    expect(s.qaRun?.log).toHaveLength(2);
    expect(s.qaVersion).toBe(v0 + 1);
    expect(s.toasts.some((t) => t.title === 'Coverage updated')).toBe(true);

    f.emit(ev('qa', { repo: 'r1', kind: 'plan', plan: { id: 'plan1', title: 'Cover validate', behaviors: [] } }));
    expect(useStore.getState().qaVersion).toBe(v0 + 2);
    expect(useStore.getState().toasts.some((t) => t.title === 'Agent proposed a test plan')).toBe(true);
  });

  it('offers to switch repos when the agent shows a demo for another one', async () => {
    const f = fakeTransport();
    setTransport(f.t);
    appState.config.activeRepo = 'r1';
    await useStore.getState().init();
    useStore.setState({ toasts: [] });
    // Routine focus in another repo stays quiet.
    f.emit(ev('focus', { view: 'code', repo: 'r2', params: { file: 'main.c' } }));
    expect(useStore.getState().toasts).toHaveLength(0);

    f.emit(ev('focus', { view: 'demo', repo: 'r2', title: 'Byte path', params: { id: 'demo3' } }));
    const t = useStore.getState().toasts.at(-1)!;
    expect(t.title).toBe('Agent sent a demo for firmware');
    expect(t.body).toBe('Byte path');
    expect(useStore.getState().tabs.some((x) => x.kind === 'demo')).toBe(false);

    t.action!.run();
    await vi.waitFor(() => expect(useStore.getState().repoId).toBe('r2'));
    await vi.waitFor(() => expect(useStore.getState().tabs.some((x) => x.kind === 'demo' && x.params.id === 'demo3')).toBe(true));
    expect(f.calls).toContain('repos.setActive');

    f.emit(ev('focus', { view: 'animation', repo: 'r1', title: 'Bytes', params: { id: 'a1' } }));
    expect(useStore.getState().toasts.at(-1)!.title).toBe('Agent sent an animation for shop');
    appState.config.activeRepo = 'r1';
  });
});
