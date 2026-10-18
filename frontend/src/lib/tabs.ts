// Tab model: pure functions so they are trivially testable.
import type { Focus } from './types';

export type TabKind =
  | 'overview' | 'code' | 'graph' | 'flow' | 'heatmap' | 'diff' | 'proposal'
  | 'walkthrough' | 'diagram' | 'settings' | 'history' | 'qa';

export interface Tab {
  id: string;
  kind: TabKind;
  title: string;
  params: Record<string, unknown>;
  /** Opened by the agent (shows a pulse until the user views it). */
  agent?: boolean;
}

export type NewTab = Omit<Tab, 'id'>;

/** Stringifies a param, treating null/undefined as "". */
const str = (v: unknown) => (v === undefined || v === null ? '' : String(v));

/** Identity of a tab: opening a tab with an existing key re-focuses it. */
export function tabKey(kind: TabKind, p: Record<string, unknown>): string {
  switch (kind) {
    case 'code':
      return `code:${str(p.file)}`;
    case 'graph':
      return `graph:${str(p.kind)}:${str(p.symbol ?? p.focus ?? p.scope)}:${str(p.direction)}`;
    case 'flow':
      return `flow:${str(p.symbol)}`;
    case 'diff':
      return p.commit ? `diff:commit:${str(p.commit)}` : `diff:${str(p.base)}..${str(p.head)}:${p.staged ? 'staged' : ''}`;
    case 'proposal':
    case 'walkthrough':
    case 'diagram':
      return `${kind}:${str(p.id)}`;
    case 'history':
      return `history:${str(p.path)}`;
    default:
      return kind;
  }
}

let seq = 0;
/** Generates a unique tab id. */
export const newTabId = () => `t${++seq}`;

export interface TabState {
  tabs: Tab[];
  active?: string;
}

/**
 * Opens a tab after the active one, or focuses and updates an equivalent
 * tab. Keeps at most 14 tabs by dropping the oldest inactive ones.
 */
export function openTab(state: TabState, t: NewTab): TabState {
  const key = tabKey(t.kind, t.params);
  const existing = state.tabs.find((x) => tabKey(x.kind, x.params) === key);
  if (existing) {
    const updated = { ...existing, title: t.title || existing.title, params: { ...existing.params, ...t.params }, agent: t.agent };
    return { tabs: state.tabs.map((x) => (x.id === existing.id ? updated : x)), active: existing.id };
  }
  const tab: Tab = { ...t, id: newTabId() };
  const idx = state.tabs.findIndex((x) => x.id === state.active);
  const tabs = [...state.tabs];
  tabs.splice(idx < 0 ? tabs.length : idx + 1, 0, tab);
  // Keep the strip manageable: drop the oldest non-active tabs past 14.
  while (tabs.length > 14) {
    const victim = tabs.findIndex((x) => x.id !== tab.id);
    tabs.splice(victim, 1);
  }
  return { tabs, active: tab.id };
}

/** Closes a tab; if it was active, its right neighbour (or left) activates. */
export function closeTab(state: TabState, id: string): TabState {
  const idx = state.tabs.findIndex((x) => x.id === id);
  if (idx < 0) return state;
  const tabs = state.tabs.filter((x) => x.id !== id);
  let active = state.active;
  if (active === id) {
    active = tabs[Math.min(idx, tabs.length - 1)]?.id;
  }
  return { tabs, active };
}

/** Keeps only the given tab. */
export function closeOthers(state: TabState, id: string): TabState {
  return { tabs: state.tabs.filter((t) => t.id === id), active: id };
}

/** Activates a tab and clears its "opened by agent" marker. */
export function activate(state: TabState, id: string): TabState {
  return {
    tabs: state.tabs.map((t) => (t.id === id && t.agent ? { ...t, agent: false } : t)),
    active: id,
  };
}

/** Last path segment of a param value. */
const basename = (p: unknown) => str(p).split('/').pop() || str(p);

/** Converts an agent "focus" request into a tab to open. */
export function focusToTab(f: Focus): NewTab | null {
  const p = f.params ?? {};
  const agent = true;
  switch (f.view) {
    case 'code':
      // Only set message when present so a later focus on the same file
      // doesn't wipe an earlier agent message.
      return { kind: 'code', title: basename(p.file), params: { ...p, flash: Date.now(), ...(f.message ? { message: f.message } : {}) }, agent };
    case 'graph':
      return { kind: 'graph', title: f.title || 'Graph', params: p, agent };
    case 'flow':
    case 'explain':
      return { kind: 'flow', title: f.title || 'Flow', params: p, agent };
    case 'heatmap':
      return { kind: 'heatmap', title: 'Heatmap', params: p, agent };
    case 'diff':
      return { kind: 'diff', title: 'Changes', params: p, agent };
    case 'proposal':
      return { kind: 'proposal', title: f.title || 'Proposal', params: p, agent };
    case 'walkthrough':
      return { kind: 'walkthrough', title: f.title || 'Walkthrough', params: p, agent };
    case 'diagram':
      return { kind: 'diagram', title: f.title || 'Diagram', params: p, agent };
    case 'overview':
      return { kind: 'overview', title: 'Overview', params: {}, agent };
    case 'qa':
      return { kind: 'qa', title: 'QA', params: { ...p, flash: Date.now() }, agent };
  }
  return null;
}
