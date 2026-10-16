import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Bot, File, Flame, FolderPlus, GitCompare, LayoutDashboard, Network, Palette, PanelRight, RefreshCw, Settings } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import { themes } from '../lib/themes';
import type { CodeSymbol, TreeNode } from '../lib/types';
import { KindIcon } from './ui';

interface Item {
  key: string;
  group: string;
  icon: ReactNode;
  label: string;
  sub?: string;
  /** Runs the item's action. */
  run: () => void;
}

/**
 * Scores how well q matches text: substring hits rank highest, mostly
 * adjacent subsequences next, scattered matches score 0.
 */
export function fuzzyScore(text: string, q: string): number {
  if (!q) return 1;
  const t = text.toLowerCase();
  const query = q.toLowerCase();
  const idx = t.indexOf(query);
  if (idx >= 0) return 100 - idx - t.length / 50;
  let ti = 0;
  let score = 0;
  for (const ch of query) {
    const found = t.indexOf(ch, ti);
    if (found < 0) return 0;
    score += found === ti ? 3 : 1;
    ti = found + 1;
  }
  // Scattered subsequence matches are noise; require mostly-adjacent hits.
  return score >= query.length * 2 ? score : 0;
}

/** Collects every file path in a tree. */
function flatten(n: TreeNode | undefined, out: string[] = []): string[] {
  if (!n) return out;
  if (!n.dir) out.push(n.path);
  n.children?.forEach((c) => flatten(c, out));
  return out;
}

let fileCache: { repo: string; files: string[] } | undefined;

/**
 * Ctrl/⌘ K palette: jump to symbols and files, or run commands (views,
 * heatmap metrics, themes, re-index...).
 */
export function CommandPalette() {
  const s = useStore();
  const [q, setQ] = useState('');
  const [sel, setSel] = useState(0);
  const [symbols, setSymbols] = useState<CodeSymbol[]>([]);
  const [files, setFiles] = useState<string[]>(fileCache?.repo === s.repoId ? fileCache.files : []);
  const listRef = useRef<HTMLDivElement>(null);
  const close = () => s.setPalette(false);

  useEffect(() => {
    api.tree(s.repoId).then((t) => {
      const f = flatten(t);
      fileCache = { repo: s.repoId, files: f };
      setFiles(f);
    });
  }, [s.repoId]);

  useEffect(() => {
    const t = setTimeout(() => {
      if (q.trim().length < 1) return setSymbols([]);
      api.searchSymbols(s.repoId, q.trim(), 30).then(setSymbols, () => setSymbols([]));
    }, 120);
    return () => clearTimeout(t);
  }, [q, s.repoId]);

  const commands: Item[] = useMemo(
    () => [
      { key: 'c-overview', group: 'Commands', icon: <LayoutDashboard size={14} />, label: 'Open overview', run: () => s.open({ kind: 'overview', title: 'Overview', params: {} }) },
      ...Object.entries(s.app?.heatMetrics ?? {}).map(([m, label]) => ({
        key: `c-heat-${m}`, group: 'Commands', icon: <Flame size={14} />, label: `Heatmap: ${m}`, sub: label, run: () => actions.heatmap(m),
      })),
      { key: 'c-deps', group: 'Commands', icon: <Network size={14} />, label: 'Show package dependencies', run: () => actions.deps() },
      { key: 'c-diff', group: 'Commands', icon: <GitCompare size={14} />, label: 'Review working changes', run: () => s.open({ kind: 'diff', title: 'Changes', params: {} }) },
      { key: 'c-connect', group: 'Commands', icon: <Bot size={14} />, label: 'Connect an agent (MCP)', run: () => s.setConnectOpen(true) },
      { key: 'c-add', group: 'Commands', icon: <FolderPlus size={14} />, label: 'Add repository…', run: () => s.setAddRepoOpen(true) },
      { key: 'c-reindex', group: 'Commands', icon: <RefreshCw size={14} />, label: 'Re-index repository', run: () => api.reindex(s.repoId).then(() => s.bump()) },
      { key: 'c-inspector', group: 'Commands', icon: <PanelRight size={14} />, label: 'Toggle inspector', run: () => s.toggleInspector() },
      { key: 'c-settings', group: 'Commands', icon: <Settings size={14} />, label: 'Settings', run: () => s.open({ kind: 'settings', title: 'Settings', params: {} }) },
      ...themes.map((t) => ({ key: `c-theme-${t.id}`, group: 'Commands', icon: <Palette size={14} />, label: `Theme: ${t.name}`, run: () => s.setTheme(t.id) })),
    ],
    [s],
  );

  const items: Item[] = useMemo(() => {
    const query = q.trim();
    const cmd = commands
      .map((c) => ({ c, score: fuzzyScore(c.label, query) }))
      .filter((x) => x.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, query ? 6 : 8)
      .map((x) => x.c);
    const syms: Item[] = symbols.slice(0, 14).map((sym) => ({
      key: `s-${sym.id}`, group: 'Symbols', icon: <KindIcon kind={sym.kind} size={20} />, label: sym.qualified,
      sub: `${sym.file}:${sym.line}`, run: () => actions.symbol(sym),
    }));
    const fs: Item[] = query
      ? files
          .map((f) => ({ f, score: fuzzyScore(f, query) + fuzzyScore(f.split('/').pop() ?? f, query) * 2 }))
          .filter((x) => x.score > 0)
          .sort((a, b) => b.score - a.score)
          .slice(0, 10)
          .map(({ f }) => ({ key: `f-${f}`, group: 'Files', icon: <File size={14} />, label: f.split('/').pop() ?? f, sub: f, run: () => actions.code(f) }))
      : [];
    return query ? [...syms, ...fs, ...cmd] : cmd;
  }, [q, commands, symbols, files]);

  useEffect(() => setSel(0), [q]);
  useEffect(() => {
    listRef.current?.querySelector('.palette-item.on')?.scrollIntoView({ block: 'nearest' });
  }, [sel]);

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSel((x) => Math.min(items.length - 1, x + 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSel((x) => Math.max(0, x - 1));
    } else if (e.key === 'Enter' && items[sel]) {
      items[sel].run();
      close();
    } else if (e.key === 'Escape') {
      close();
    }
  };

  let lastGroup = '';
  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div className="modal" style={{ width: 'min(680px, 92vw)' }}>
        <input className="palette-input" autoFocus placeholder="Jump to a symbol, file or command…" value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onKey} />
        <div className="palette-list" ref={listRef}>
          {items.length === 0 && <div className="faint" style={{ padding: 14 }}>No matches.</div>}
          {items.map((it, i) => {
            const header = it.group !== lastGroup ? <div className="palette-group">{it.group}</div> : null;
            lastGroup = it.group;
            return (
              <div key={it.key}>
                {header}
                <div
                  className={`palette-item ${i === sel ? 'on' : ''}`}
                  onMouseEnter={() => setSel(i)}
                  onClick={() => {
                    it.run();
                    close();
                  }}
                >
                  <span className="pi-icon">{it.icon}</span>
                  <span className={`ellipsis ${it.group === 'Symbols' ? 'mono' : ''}`}>{it.label}</span>
                  {it.sub && <span className="pi-sub ellipsis">{it.sub}</span>}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
