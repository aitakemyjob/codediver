import { useMemo, useState } from 'react';
import { ArrowDownLeft, ArrowLeftRight, ArrowUpRight, Layers, RefreshCw, Route, Rows3, Columns3 } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import type { Graph, GraphNode } from '../lib/types';
import { GraphCanvas } from '../graph/GraphCanvas';
import { Empty, ErrorBox, Spinner, Switch, useAsync } from '../components/ui';
import type { ViewProps } from './ViewHost';

type Dir = 'callees' | 'callers' | 'both';

/**
 * Graph screen for call graphs, traces, dependencies, types and impact,
 * with direction, depth, externals, refs and layout controls.
 */
export default function GraphView({ tab }: ViewProps) {
  const s = useStore();
  const p = tab.params;
  const kind = String(p.kind ?? 'callgraph');
  const symbol = String(p.symbol ?? '');
  const [direction, setDirection] = useState<Dir>((p.direction as Dir) ?? 'callees');
  const [depth, setDepth] = useState(Number(p.depth ?? (kind === 'trace' ? 6 : 2)));
  const [external, setExternal] = useState(!!p.includeExternal);
  const [refs, setRefs] = useState(p.includeRefs !== false);
  const [focus, setFocus] = useState(String(p.focus ?? ''));
  const [layoutDir, setLayoutDir] = useState<string | undefined>(undefined);
  const [selected, setSelected] = useState<string>();
  const [nonce, setNonce] = useState(0);

  const g = useAsync<Graph>(() => {
    switch (kind) {
      case 'trace':
        return api.trace(s.repoId, symbol, depth);
      case 'deps':
        return api.deps(s.repoId, external, focus);
      case 'types':
        return api.types(s.repoId, String(p.scope ?? ''));
      default:
        return api.callGraph(s.repoId, symbol, { direction, depth, includeExternal: external, includeRefs: refs });
    }
  }, [kind, symbol, direction, depth, external, refs, focus, p.scope, s.repoId, s.refreshKey, nonce]);

  const heat = useMemo(() => {
    if (!g.data || (kind !== 'impact' && kind !== 'trace')) return undefined;
    const maxD = Math.max(1, ...g.data.nodes.map((n) => n.depth ?? 0));
    return Object.fromEntries(g.data.nodes.map((n) => [n.id, kind === 'impact' ? 1 - (n.depth ?? 0) / (maxD + 1) : n.root ? 1 : 0]));
  }, [g.data, kind]);

  const onSelect = (n: GraphNode) => {
    setSelected(n.id);
    if (n.symbol && !n.external) s.inspectSymbol(n.symbol);
  };
  const onOpen = (n: GraphNode) => {
    if (kind === 'deps') {
      if (n.external) return;
      return actions.types(n.id, n.label);
    }
    if (n.file) actions.code(n.file, n.line, n.endLine);
  };

  const defaultDir = kind === 'deps' || kind === 'types' ? 'TB' : kind === 'impact' ? 'RL' : 'LR';
  const dir = layoutDir ?? g.data?.direction ?? defaultDir;
  const graph = g.data;

  return (
    <>
      <div className="toolbar">
        <span className="title ellipsis" style={{ maxWidth: 420 }}>{graph?.title ?? tab.title}</span>
        {graph && <span className="badge">{graph.nodes.length} nodes · {graph.edges.length} edges</span>}
        {graph?.truncated && <span className="badge warn">truncated</span>}
        <div className="grow" />
        {kind === 'callgraph' && (
          <div className="seg">
            <button className={direction === 'callers' ? 'on' : ''} onClick={() => setDirection('callers')}><ArrowDownLeft size={13} /> Callers</button>
            <button className={direction === 'both' ? 'on' : ''} onClick={() => setDirection('both')}><ArrowLeftRight size={13} /> Both</button>
            <button className={direction === 'callees' ? 'on' : ''} onClick={() => setDirection('callees')}><ArrowUpRight size={13} /> Callees</button>
          </div>
        )}
        {(kind === 'callgraph' || kind === 'trace') && (
          <label className="row faint" style={{ gap: 6 }}>
            depth
            <input type="range" min={1} max={kind === 'trace' ? 10 : 6} value={depth} onChange={(e) => setDepth(Number(e.target.value))} style={{ width: 90, accentColor: 'var(--accent)' }} />
            <span className="mono" style={{ color: 'var(--text)' }}>{depth}</span>
          </label>
        )}
        {(kind === 'callgraph' || kind === 'deps') && (
          <label className="row faint" style={{ gap: 6 }}>
            <Switch on={external} onChange={setExternal} label="External" /> external
          </label>
        )}
        {kind === 'callgraph' && (
          <label className="row faint" style={{ gap: 6 }} title="Functions passed as values, e.g. HTTP handlers">
            <Switch on={refs} onChange={setRefs} label="Refs" /> refs
          </label>
        )}
        {kind === 'deps' && focus && (
          <button className="btn sm" onClick={() => setFocus('')}>Show all packages</button>
        )}
        <div className="seg">
          <button className={dir === 'LR' || dir === 'RL' ? 'on' : ''} onClick={() => setLayoutDir('LR')} title="Left to right"><Columns3 size={13} /></button>
          <button className={dir === 'TB' ? 'on' : ''} onClick={() => setLayoutDir('TB')} title="Top to bottom"><Rows3 size={13} /></button>
        </div>
        {symbol && kind === 'callgraph' && (
          <button className="btn sm" onClick={() => actions.trace({ id: symbol, qualified: graph?.nodes.find((n) => n.root)?.label ?? symbol })}><Route size={13} /> Trace</button>
        )}
        <button className="btn ghost icon sm" title="Refresh" onClick={() => setNonce((n) => n + 1)}><RefreshCw size={13} /></button>
      </div>
      {g.error ? <ErrorBox error={g.error} /> : null}
      {graph && graph.nodes.length === 0 && (
        <Empty icon={<Layers size={28} />} title="Nothing to draw">
          {kind === 'trace' ? 'No callers found — this may be an entry point, a callback registered dynamically, or only called from tests.' : 'No matching relationships were found.'}
        </Empty>
      )}
      {graph && graph.nodes.length > 0 && (
        <GraphCanvas
          graph={graph}
          direction={dir}
          selected={selected}
          heat={heat}
          onSelect={(n) => {
            onSelect(n);
            if (kind === 'deps' && !n.external) setFocus(n.id);
          }}
          onOpen={onOpen}
        />
      )}
      {g.loading && <Spinner />}
      {graph?.note && <div className="faint" style={{ padding: '6px 12px', borderTop: '1px solid var(--border)' }}>{graph.note}</div>}
    </>
  );
}
