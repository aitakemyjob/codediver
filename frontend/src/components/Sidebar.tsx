import { useEffect, useMemo, useState } from 'react';
import {
  Bot, ChevronRight, Eraser, File, Flame, FolderClosed, FolderOpen, GitCommit, GitCompare, Map as MapIcon, MessageSquareText,
  Network, RefreshCw, Route, Sparkles, Workflow,
} from 'lucide-react';
import { api } from '../lib/api';
import { useStore } from '../lib/store';
import { heatColor, heatScale } from '../lib/themes';
import { basename, dirname, plural, relTime } from '../lib/format';
import type { CodeSymbol, Commit, FileSymbol, Heatmap, SearchMatch, StatusEntry, TreeNode } from '../lib/types';
import { KindIcon, useAsync } from './ui';

/** Hosts the active sidebar panel. */
export function Sidebar() {
  const panel = useStore((s) => s.sidebar);
  return (
    <aside className="sidebar">
      {panel === 'explorer' && <Explorer />}
      {panel === 'outline' && <Outline />}
      {panel === 'search' && <SearchPanel />}
      {panel === 'changes' && <ChangesPanel />}
      {panel === 'agent' && <AgentPanel />}
    </aside>
  );
}

/** Opens a file in a code tab, optionally highlighting a range. */
export function openFile(path: string, line?: number, endLine?: number) {
  useStore.getState().open({ kind: 'code', title: basename(path), params: { file: path, line, endLine, flash: Date.now() } });
}

// ---------------------------------------------------------------- Explorer

/** Adds aggregated (summed) metric values for every directory. */
function dirValues(tree: TreeNode | undefined, values: Record<string, number>): Record<string, number> {
  const out: Record<string, number> = { ...values };
  const walk = (n: TreeNode): number => {
    if (!n.dir) return values[n.path] ?? 0;
    const v = (n.children ?? []).reduce((s, c) => s + walk(c), 0);
    out[n.path] = v;
    return v;
  };
  if (tree) walk(tree);
  return out;
}

/** File tree with an optional heat overlay per file and folder. */
function Explorer() {
  const s = useStore();
  const [metric, setMetric] = useState<string>('');
  const tree = useAsync(() => api.tree(s.repoId), [s.repoId, s.refreshKey]);
  const [heat, setHeat] = useState<Heatmap>();
  useEffect(() => {
    if (!metric) return setHeat(undefined);
    api.heatmap(s.repoId, metric).then(setHeat, () => setHeat(undefined));
  }, [metric, s.repoId, s.refreshKey, s.agentPulse]);
  const values = useMemo(() => dirValues(tree.data, heat?.values ?? {}), [tree.data, heat]);
  const fileMax = heat?.max ?? 0;
  const activeTab = s.tabs.find((t) => t.id === s.active);
  const activeFile = activeTab?.kind === 'code' ? String(activeTab.params.file) : '';
  return (
    <>
      <div className="panel-head">
        Explorer
        <div className="actions">
          <select className="input select" style={{ height: 22, fontSize: 11, padding: '0 22px 0 6px', width: 'auto', textTransform: 'none', letterSpacing: 0 }} value={metric} onChange={(e) => setMetric(e.target.value)} title="Heat overlay">
            <option value="">No heat</option>
            {Object.keys(s.app?.heatMetrics ?? {}).map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
          <button className="btn ghost icon sm" title="Refresh" onClick={() => s.bump()}>
            <RefreshCw size={13} />
          </button>
        </div>
      </div>
      <div className="panel-body">
        {tree.data?.children?.map((n) => (
          <TreeItem key={n.path} node={n} depth={0} values={values} max={fileMax} activeFile={activeFile} />
        ))}
        {tree.error ? <div className="error-box">{String(tree.error)}</div> : null}
      </div>
    </>
  );
}

/**
 * One tree row; directories expand on click and auto-expand to reveal
 * the active file until the user toggles them.
 */
function TreeItem({ node, depth, values, max, activeFile }: { node: TreeNode; depth: number; values: Record<string, number>; max: number; activeFile: string }) {
  const theme = useStore((s) => s.theme);
  // null = follow the active file (auto-expand its ancestors); boolean = user choice.
  const [open, setOpen] = useState<boolean | null>(null);
  const isOpen = open ?? (node.dir && activeFile.startsWith(node.path + '/'));
  const v = values[node.path] ?? 0;
  // Directories aggregate; normalize them against the largest top-level total.
  const t = heatScale(v, node.dir ? Math.max(max * 4, v) : max);
  return (
    <>
      <div
        className={`tree-row ${!node.dir && node.path === activeFile ? 'on' : ''}`}
        style={{ paddingLeft: 6 + depth * 14 }}
        onClick={() => (node.dir ? setOpen(!isOpen) : openFile(node.path))}
        title={node.path}
      >
        {node.dir ? <ChevronRight size={14} className={`chev ${isOpen ? 'open' : ''}`} /> : <span className="chev" />}
        {node.dir ? (
          isOpen ? <FolderOpen size={14} style={{ color: 'var(--accent)', flex: 'none' }} /> : <FolderClosed size={14} style={{ color: 'var(--accent)', flex: 'none' }} />
        ) : (
          <File size={14} className="faint" style={{ flex: 'none' }} />
        )}
        <span className="name">{node.name}</span>
        {v > 0 && (
          <span className="heat" title={`${v}`}>
            <i style={{ width: `${Math.max(12, t * 100)}%`, background: heatColor(theme, t) }} />
          </span>
        )}
      </div>
      {node.dir && isOpen && node.children?.map((c) => <TreeItem key={c.path} node={c} depth={depth + 1} values={values} max={max} activeFile={activeFile} />)}
    </>
  );
}

// ---------------------------------------------------------------- Outline

/** Symbols in the active file with caller/callee counts. */
function Outline() {
  const s = useStore();
  const tab = s.tabs.find((t) => t.id === s.active);
  const file = tab?.kind === 'code' ? String(tab.params.file) : '';
  const syms = useAsync(() => (file ? api.fileSymbols(s.repoId, file) : Promise.resolve([] as FileSymbol[])), [file, s.repoId, s.refreshKey]);
  return (
    <>
      <div className="panel-head">Outline {file && <span className="faint" style={{ textTransform: 'none', letterSpacing: 0 }}>· {basename(file)}</span>}</div>
      <div className="panel-body">
        {!file && <div className="faint" style={{ padding: 12 }}>Open a file to see its symbols.</div>}
        {syms.data?.map((sym) => (
          <div
            key={sym.id}
            className={`tree-row ${s.inspect === sym.id ? 'on' : ''}`}
            style={{ paddingLeft: sym.parent ? 26 : 8 }}
            onClick={() => {
              openFile(file, sym.line, sym.endLine);
              s.inspectSymbol(sym.id);
            }}
          >
            <KindIcon kind={sym.kind} size={18} />
            <span className="name mono">{sym.qualified}</span>
            <span className="tag faint" title="callers · callees">
              {sym.callers}↙ {sym.callees}↗
            </span>
          </div>
        ))}
      </div>
    </>
  );
}

// ---------------------------------------------------------------- Search

/** Text/regex and symbol search with results grouped by file. */
function SearchPanel() {
  const s = useStore();
  const [q, setQ] = useState('');
  const [regex, setRegex] = useState(false);
  const [mode, setMode] = useState<'code' | 'symbols'>('code');
  const [results, setResults] = useState<{ matches?: SearchMatch[]; symbols?: CodeSymbol[]; truncated?: boolean; error?: string }>({});
  useEffect(() => {
    if (!q.trim()) return setResults({});
    const t = setTimeout(() => {
      if (mode === 'code') {
        api.search(s.repoId, q, { regex, limit: 300 }).then((r) => setResults(r), (e) => setResults({ error: String(e.message ?? e) }));
      } else {
        api.searchSymbols(s.repoId, q, 80).then((symbols) => setResults({ symbols }), (e) => setResults({ error: String(e.message ?? e) }));
      }
    }, 220);
    return () => clearTimeout(t);
  }, [q, regex, mode, s.repoId]);
  const grouped = useMemo(() => {
    const m = new Map<string, SearchMatch[]>();
    for (const r of results.matches ?? []) m.set(r.path, [...(m.get(r.path) ?? []), r]);
    return [...m.entries()];
  }, [results.matches]);
  return (
    <>
      <div className="panel-head">Search</div>
      <div style={{ padding: '0 10px 8px' }} className="col">
        <input className="input" autoFocus placeholder={mode === 'code' ? 'Search code…' : 'Find a symbol…'} value={q} onChange={(e) => setQ(e.target.value)} />
        <div className="row">
          <div className="seg">
            <button className={mode === 'code' ? 'on' : ''} onClick={() => setMode('code')}>Text</button>
            <button className={mode === 'symbols' ? 'on' : ''} onClick={() => setMode('symbols')}>Symbols</button>
          </div>
          {mode === 'code' && (
            <button className={`btn sm ${regex ? 'active' : ''}`} onClick={() => setRegex(!regex)} title="Regular expression">
              .*
            </button>
          )}
        </div>
      </div>
      <div className="panel-body">
        {results.error && <div className="error-box">{results.error}</div>}
        {grouped.map(([path, ms]) => (
          <div key={path} style={{ marginBottom: 6 }}>
            <div className="tree-row" onClick={() => openFile(path, ms[0].line)}>
              <File size={13} className="faint" />
              <span className="name" style={{ fontWeight: 600, color: 'var(--text)' }}>{basename(path)}</span>
              <span className="faint ellipsis" style={{ fontSize: 11 }}>{dirname(path)}</span>
              <span className="tag faint">{ms.length}</span>
            </div>
            {ms.slice(0, 20).map((m) => (
              <div key={m.line} className="tree-row mono" style={{ paddingLeft: 28, fontSize: 11.5 }} onClick={() => openFile(path, m.line)}>
                <span className="faint" style={{ width: 30, flex: 'none', textAlign: 'right' }}>{m.line}</span>
                <span className="name">{m.text.trim()}</span>
              </div>
            ))}
          </div>
        ))}
        {results.symbols?.map((sym) => (
          <div key={sym.id} className="tree-row" onClick={() => { sym.file && openFile(sym.file, sym.line, sym.endLine); s.inspectSymbol(sym.id); }}>
            <KindIcon kind={sym.kind} size={18} />
            <span className="name mono">{sym.qualified}</span>
            <span className="tag faint ellipsis" style={{ maxWidth: 110 }}>{sym.file}</span>
          </div>
        ))}
        {results.truncated && <div className="faint" style={{ padding: 8 }}>More results omitted — refine your query.</div>}
      </div>
    </>
  );
}

// ---------------------------------------------------------------- Changes

const statusLetter: Record<string, string> = { modified: 'M', added: 'A', deleted: 'D', renamed: 'R', untracked: 'U' };

/**
 * Git panel: staged and unstaged changes plus recent commits, each
 * opening a review.
 */
function ChangesPanel() {
  const s = useStore();
  const status = useAsync(() => api.status(s.repoId), [s.repoId, s.refreshKey, s.agentPulse]);
  const log = useAsync(() => api.log(s.repoId, 40), [s.repoId, s.refreshKey]);
  const entries = status.data?.entries ?? [];
  const staged = entries.filter((e) => e.staged);
  const unstaged = entries.filter((e) => !e.staged);
  const openDiff = (path?: string, spec: Record<string, unknown> = {}) =>
    s.open({ kind: 'diff', title: spec.head ? `${String(spec.head).slice(0, 7)}` : 'Changes', params: { ...spec, select: path } });
  if (status.data && !status.data.info.isRepo) {
    return (
      <>
        <div className="panel-head">Changes</div>
        <div className="faint" style={{ padding: 12 }}>This folder is not a git repository.</div>
      </>
    );
  }
  const group = (title: string, list: StatusEntry[], spec: Record<string, unknown>) =>
    list.length > 0 && (
      <>
        <div className="section-title">{title} <span className="badge">{list.length}</span></div>
        {list.map((e) => (
          <div key={e.path + title} className="file-row" onClick={() => openDiff(e.path, spec)} title={e.path}>
            <span className={`st st-${e.status}`}>{statusLetter[e.status] ?? '?'}</span>
            <span className="ellipsis">{basename(e.path)}</span>
            <span className="faint ellipsis" style={{ fontSize: 11 }}>{dirname(e.path)}</span>
          </div>
        ))}
      </>
    );
  return (
    <>
      <div className="panel-head">
        Changes
        <div className="actions">
          <button className="btn ghost icon sm" title="Refresh" onClick={() => s.bump()}>
            <RefreshCw size={13} />
          </button>
        </div>
      </div>
      <div className="panel-body">
        <div style={{ padding: '0 6px 6px' }} className="col">
          <button className="btn primary" onClick={() => openDiff()} disabled={entries.length === 0}>
            <GitCompare size={14} /> Review working changes
          </button>
          <button className="btn" onClick={() => s.open({ kind: 'heatmap', title: 'Heatmap', params: { metric: 'changes' } })} disabled={entries.length === 0}>
            <Flame size={14} /> Change heatmap
          </button>
        </div>
        {entries.length === 0 && <div className="faint" style={{ padding: '6px 10px' }}>Working tree clean.</div>}
        {group('Staged', staged, { staged: true })}
        {group('Changes', unstaged, {})}
        <div className="section-title">History</div>
        {log.data?.map((c: Commit) => (
          <div key={c.hash} className="list-item" onClick={() => openDiff(undefined, { base: `${c.hash}~1`, head: c.hash })}>
            <GitCommit size={14} className="faint" style={{ marginTop: 2, flex: 'none' }} />
            <div className="grow">
              <div className="title ellipsis">{c.subject}</div>
              <div className="sub">
                <span className="mono">{c.short}</span> · {c.author} · {relTime(c.time)} · <span className="add">+{c.additions}</span> <span className="del">−{c.deletions}</span>
              </div>
            </div>
          </div>
        ))}
      </div>
    </>
  );
}

// ---------------------------------------------------------------- Agent

/**
 * Agent panel: proposals, walkthroughs, explanations, diagrams, notes and
 * the live activity feed.
 */
function AgentPanel() {
  const s = useStore();
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick((x) => x + 1), 15000);
    return () => clearInterval(t);
  }, []);
  const feed = [...s.activity].reverse().slice(0, 80);
  const { walkthroughs, diagrams, explanations, annotations } = s.session;
  return (
    <>
      <div className="panel-head">
        Agent
        <div className="actions">
          <button className="btn ghost sm" onClick={() => s.setConnectOpen(true)}>Connect</button>
        </div>
      </div>
      <div className="panel-body">
        {s.proposals.length > 0 && <div className="section-title"><Sparkles size={12} /> Proposals</div>}
        {s.proposals.map((p) => {
          const add = p.changes.reduce((n, c) => n + c.additions, 0);
          const del = p.changes.reduce((n, c) => n + c.deletions, 0);
          return (
            <div key={p.id} className="list-item" onClick={() => s.open({ kind: 'proposal', title: p.title, params: { id: p.id } })}>
              <Sparkles size={14} style={{ color: 'var(--accent-2)', marginTop: 2, flex: 'none' }} />
              <div className="grow">
                <div className="title ellipsis">{p.title}</div>
                <div className="sub">
                  <span className={`status-${p.status}`}>{p.status}</span> · {plural(p.changes.length, 'file')} · <span className="add">+{add}</span> <span className="del">−{del}</span>
                </div>
              </div>
            </div>
          );
        })}
        {walkthroughs.length > 0 && <div className="section-title"><Route size={12} /> Walkthroughs</div>}
        {walkthroughs.map((w) => (
          <div key={w.id} className="list-item" onClick={() => s.open({ kind: 'walkthrough', title: w.title, params: { id: w.id } })}>
            <Route size={14} style={{ color: 'var(--accent)', marginTop: 2, flex: 'none' }} />
            <div className="grow"><div className="title ellipsis">{w.title}</div><div className="sub">{plural(w.steps.length, 'step')} · {relTime(w.createdAt)}</div></div>
          </div>
        ))}
        {explanations.length > 0 && <div className="section-title"><Workflow size={12} /> Explained functions</div>}
        {explanations.map((x) => (
          <div key={x.id} className="list-item" onClick={() => s.open({ kind: 'flow', title: x.sym?.qualified ?? 'Flow', params: { symbol: x.symbol } })}>
            <Workflow size={14} style={{ color: 'var(--accent)', marginTop: 2, flex: 'none' }} />
            <div className="grow"><div className="title ellipsis mono">{x.sym?.qualified}</div><div className="sub ellipsis">{x.summary}</div></div>
          </div>
        ))}
        {diagrams.length > 0 && <div className="section-title"><Network size={12} /> Diagrams</div>}
        {diagrams.map((d) => (
          <div key={d.id} className="list-item" onClick={() => s.open({ kind: 'diagram', title: d.title, params: { id: d.id } })}>
            <MapIcon size={14} style={{ color: 'var(--accent)', marginTop: 2, flex: 'none' }} />
            <div className="grow"><div className="title ellipsis">{d.title}</div><div className="sub">{d.mermaid ? 'mermaid' : 'graph'} · {relTime(d.createdAt)}</div></div>
          </div>
        ))}
        {annotations.length > 0 && (
          <div className="section-title">
            <MessageSquareText size={12} /> Notes <span className="badge">{annotations.length}</span>
            <button className="btn ghost icon sm" style={{ marginLeft: 'auto' }} title="Clear notes" onClick={() => api.clearAnnotations(s.repoId)}>
              <Eraser size={12} />
            </button>
          </div>
        )}
        {annotations.slice(0, 30).map((a) => (
          <div key={a.id} className="list-item" onClick={() => openFile(a.file, a.line, a.end_line)}>
            <span className={`dot gd-anno-${a.kind}`} style={{ marginTop: 5 }} />
            <div className="grow"><div className="sub mono">{basename(a.file)}:{a.line}</div><div className="ellipsis">{a.text}</div></div>
          </div>
        ))}
        <div className="section-title"><Bot size={12} /> Activity</div>
        {feed.length === 0 && (
          <div className="faint" style={{ padding: '4px 10px', lineHeight: 1.6 }}>
            No agent activity yet. Connect Claude Code or another MCP client and ask it to walk you through this repo.
          </div>
        )}
        {feed.map((a) => (
          <div key={a.id} className={`feed-item ${a.error ? 'err' : ''}`} title={a.error || a.summary}>
            <span className="tool">{a.tool}</span>
            <span className="ellipsis faint">{a.error ? a.error : a.summary}</span>
            <span className="when">{relTime(a.time)}</span>
          </div>
        ))}
      </div>
    </>
  );
}
