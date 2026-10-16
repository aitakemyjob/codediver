import { create } from 'zustand';
import { api } from './api';
import { getTransport } from './transport';
import { activate, closeOthers, closeTab, focusToTab, openTab, type NewTab, type Tab } from './tabs';
import { applyTheme, themeById, type Theme } from './themes';
import type {
  Activity, AppState, BusEvent, Diagram, Explanation, Focus, MCPStatus, Proposal, SessionData, Settings, Walkthrough, Annotation,
} from './types';

export type SidebarPanel = 'explorer' | 'search' | 'changes' | 'agent' | 'outline';

export interface Toast {
  id: number;
  kind: 'info' | 'ok' | 'warn' | 'err' | 'agent';
  title: string;
  body?: string;
  action?: { label: string; run: () => void };
}

export interface CursorContext {
  view: string;
  file?: string;
  line?: number;
  endLine?: number;
  selection?: string;
  symbol?: string;
  title?: string;
}

interface Store {
  ready: boolean;
  fatal?: string;
  app?: AppState;
  repoId: string;
  theme: Theme;
  tabs: Tab[];
  active?: string;
  sidebar: SidebarPanel;
  sidebarOpen: boolean;
  inspectorOpen: boolean;
  inspect?: string;
  activity: Activity[];
  proposals: Proposal[];
  session: SessionData;
  indexing: boolean;
  indexStats?: { files: number; functions: number; edges: number; durationMs: number };
  toasts: Toast[];
  palette: boolean;
  connectOpen: boolean;
  addRepoOpen: boolean;
  refreshKey: number;
  agentPulse: number;

  /** Connects to the backend, loads state and opens the overview. */
  init(): Promise<void>;
  /** Reloads app state and applies the theme; resets tabs on repo change. */
  refresh(): Promise<void>;
  /** Loads proposals and session data for the active repository. */
  loadRepoData(): Promise<void>;
  /** Switches the active repository. */
  setRepo(id: string): Promise<void>;
  /** Applies and persists a theme. */
  setTheme(id: string): Promise<void>;
  /** Merges and persists settings (optimistically updates local state). */
  saveSettings(s: Partial<Settings>): Promise<void>;
  /** Opens a tab, or focuses an equivalent existing one. */
  open(t: NewTab): void;
  /** Closes a tab. */
  close(id: string): void;
  /** Closes every tab except one. */
  closeOthers(id: string): void;
  /** Activates a tab and clears its agent marker. */
  activate(id: string): void;
  /** Merges params into a tab. */
  updateTab(id: string, params: Record<string, unknown>): void;
  /** Shows a sidebar panel, or toggles the sidebar if it is already shown. */
  setSidebar(p: SidebarPanel): void;
  /** Shows or hides the sidebar. */
  toggleSidebar(): void;
  /** Shows, hides or toggles the inspector. */
  toggleInspector(open?: boolean): void;
  /** Shows a symbol in the inspector. */
  inspectSymbol(id?: string): void;
  /** Shows a toast that dismisses itself (later when it has an action). */
  toast(t: Omit<Toast, 'id'>): void;
  /** Dismisses a toast. */
  dismissToast(id: number): void;
  /** Opens or closes the command palette. */
  setPalette(open: boolean): void;
  /** Opens or closes the "Connect an agent" dialog. */
  setConnectOpen(open: boolean): void;
  /** Opens or closes the "Add repository" dialog. */
  setAddRepoOpen(open: boolean): void;
  /** Reports the developer's cursor/selection to the backend (debounced). */
  reportContext(c: CursorContext): void;
  /** Applies a backend bus event: focus, proposals, activity, index, ... */
  handleEvent(ev: BusEvent): void;
  /** Bumps refreshKey so views refetch their data. */
  bump(): void;
}

const emptySession: SessionData = { walkthroughs: [], diagrams: [], explanations: [], annotations: [] };
let toastSeq = 0;
let ctxTimer: ReturnType<typeof setTimeout> | undefined;

/** Replaces the item with the same id, or prepends it. */
function upsert<T extends { id: string }>(list: T[], item: T): T[] {
  const i = list.findIndex((x) => x.id === item.id);
  if (i < 0) return [item, ...list];
  const next = [...list];
  next[i] = item;
  return next;
}

export const useStore = create<Store>((set, get) => ({
  ready: false,
  repoId: '',
  theme: themeById('deepdive'),
  tabs: [],
  sidebar: 'explorer',
  sidebarOpen: true,
  inspectorOpen: false,
  activity: [],
  proposals: [],
  session: emptySession,
  indexing: false,
  toasts: [],
  palette: false,
  connectOpen: false,
  addRepoOpen: false,
  refreshKey: 0,
  agentPulse: 0,

  /** See Store.init. */
  async init() {
    try {
      const t = await getTransport();
      t.subscribe((ev) => get().handleEvent(ev));
      await get().refresh();
      const activity = await api.activity();
      set({ activity: activity ?? [], ready: true });
      if (get().repoId) {
        get().open({ kind: 'overview', title: 'Overview', params: {} });
        await get().loadRepoData();
      }
    } catch (e) {
      set({ fatal: (e as Error).message, ready: true });
    }
  },

  /** See Store.refresh. */
  async refresh() {
    const app = await api.state();
    const theme = themeById(app.config.settings.theme);
    applyTheme(theme);
    document.documentElement.style.setProperty('--editor-font-size', `${app.config.settings.fontSize}px`);
    const prev = get().repoId;
    const repoId = app.config.activeRepo || app.repos[0]?.id || '';
    set({ app, theme, repoId });
    if (prev && prev !== repoId) {
      set({ tabs: [], active: undefined, inspect: undefined });
      get().open({ kind: 'overview', title: 'Overview', params: {} });
      await get().loadRepoData();
    }
  },

  /** See Store.loadRepoData. */
  async loadRepoData() {
    const repo = get().repoId;
    if (!repo) return;
    const [proposals, session] = await Promise.all([api.proposals(repo).catch(() => []), api.session(repo).catch(() => emptySession)]);
    set({ proposals: proposals ?? [], session: session ?? emptySession });
  },

  /** See Store.setRepo. */
  async setRepo(id) {
    await api.setActiveRepo(id);
    await get().refresh();
  },

  /** See Store.setTheme. */
  async setTheme(id) {
    const theme = themeById(id);
    applyTheme(theme);
    set({ theme });
    await get().saveSettings({ theme: id });
  },

  /** See Store.saveSettings. */
  async saveSettings(partial) {
    const app = get().app;
    if (!app) return;
    const next = { ...app.config.settings, ...partial };
    set({ app: { ...app, config: { ...app.config, settings: next } } });
    await api.updateSettings(next);
  },

  /** See Store.open. */
  open(t) {
    set((s) => openTab(s, t));
  },
  /** See Store.close. */
  close(id) {
    set((s) => closeTab(s, id));
  },
  /** See Store.closeOthers. */
  closeOthers(id) {
    set((s) => closeOthers(s, id));
  },
  /** See Store.activate. */
  activate(id) {
    set((s) => activate(s, id));
  },
  /** See Store.updateTab. */
  updateTab(id, params) {
    set((s) => ({ tabs: s.tabs.map((t) => (t.id === id ? { ...t, params: { ...t.params, ...params } } : t)) }));
  },
  /** See Store.setSidebar. */
  setSidebar(p) {
    set((s) => ({ sidebar: p, sidebarOpen: s.sidebar === p ? !s.sidebarOpen : true }));
  },
  /** See Store.toggleSidebar. */
  toggleSidebar() {
    set((s) => ({ sidebarOpen: !s.sidebarOpen }));
  },
  /** See Store.toggleInspector. */
  toggleInspector(open) {
    set((s) => ({ inspectorOpen: open ?? !s.inspectorOpen }));
  },
  /** See Store.inspectSymbol. */
  inspectSymbol(id) {
    set({ inspect: id, inspectorOpen: !!id || get().inspectorOpen });
  },
  /** See Store.toast. */
  toast(t) {
    const id = ++toastSeq;
    set((s) => ({ toasts: [...s.toasts.slice(-4), { ...t, id }] }));
    setTimeout(() => get().dismissToast(id), t.action ? 9000 : 4500);
  },
  /** See Store.dismissToast. */
  dismissToast(id) {
    set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) }));
  },
  /** See Store.setPalette. */
  setPalette(open) {
    set({ palette: open });
  },
  /** See Store.setConnectOpen. */
  setConnectOpen(open) {
    set({ connectOpen: open });
  },
  /** See Store.setAddRepoOpen. */
  setAddRepoOpen(open) {
    set({ addRepoOpen: open });
  },
  /** See Store.bump. */
  bump() {
    set((s) => ({ refreshKey: s.refreshKey + 1 }));
  },

  /** See Store.reportContext. */
  reportContext(c) {
    clearTimeout(ctxTimer);
    ctxTimer = setTimeout(() => {
      const repo = get().repoId;
      if (repo) api.setContext({ repo, ...c }).catch(() => {});
    }, 250);
  },

  /**
   * See Store.handleEvent. Focus requests only open tabs while following
   * the agent, and events for other repositories are ignored.
   */
  handleEvent(ev) {
    const s = get();
    const repo = s.repoId;
    const follow = s.app?.config.settings.followAgent ?? true;
    switch (ev.type) {
      case 'config':
        s.refresh();
        break;
      case 'mcp':
        if (s.app) set({ app: { ...s.app, mcp: ev.data as MCPStatus } });
        break;
      case 'activity': {
        const a = ev.data as Activity;
        set((st) => ({ activity: [...st.activity.slice(-299), a], agentPulse: st.agentPulse + 1 }));
        if (a.tool === 'connect') s.toast({ kind: 'agent', title: a.summary, body: 'Agent connected over MCP' });
        break;
      }
      case 'index': {
        const d = ev.data as { repo: string; state: string; stats?: Store['indexStats'] };
        if (d.repo !== repo) break;
        set({ indexing: d.state === 'building', ...(d.stats ? { indexStats: d.stats } : {}) });
        break;
      }
      case 'focus': {
        const f = ev.data as Focus;
        if (f.repo !== repo) break;
        const tab = focusToTab(f);
        if (tab && follow) s.open(tab);
        if (f.message) s.toast({ kind: 'agent', title: 'Agent', body: f.message });
        break;
      }
      case 'proposal': {
        const p = ev.data as Proposal;
        if (p.repo !== repo) break;
        const isNew = !s.proposals.some((x) => x.id === p.id);
        set((st) => ({ proposals: upsert(st.proposals, p) }));
        if (isNew) {
          s.toast({
            kind: 'agent', title: 'Agent proposed changes', body: `${p.title} · ${p.changes.length} file(s)`,
            action: { label: 'Review', run: () => get().open({ kind: 'proposal', title: p.title, params: { id: p.id } }) },
          });
        }
        if (p.status === 'applied' || p.status === 'partial') s.bump();
        break;
      }
      case 'walkthrough': {
        const w = ev.data as Walkthrough;
        if (w.repo === repo) set((st) => ({ session: { ...st.session, walkthroughs: upsert(st.session.walkthroughs, w) } }));
        break;
      }
      case 'diagram': {
        const d = ev.data as Diagram;
        if (d.repo === repo) set((st) => ({ session: { ...st.session, diagrams: upsert(st.session.diagrams, d) } }));
        break;
      }
      case 'explain': {
        const x = ev.data as Explanation;
        if (x.repo === repo) set((st) => ({ session: { ...st.session, explanations: upsert(st.session.explanations, x) } }));
        break;
      }
      case 'annotations': {
        const d = ev.data as { repo: string; annotations: Annotation[] };
        if (d.repo === repo) set((st) => ({ session: { ...st.session, annotations: d.annotations } }));
        break;
      }
    }
  },
}));

/** The active repository summary, if any. */
export const useRepo = () => {
  const { app, repoId } = useStore();
  return app?.repos.find((r) => r.id === repoId);
};
