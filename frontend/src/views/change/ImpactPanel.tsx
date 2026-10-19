import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, CircleAlert, List, LogIn, Network } from 'lucide-react';
import { actions } from '../../lib/actions';
import { basename } from '../../lib/format';
import type { Impact, ImpactNode, ImpactRing } from '../../lib/types';
import { GraphCanvas } from '../../graph/GraphCanvas';
import { Empty } from '../../components/ui';
import { CoverageBar } from '../qa/qa';
import { nodesById, packageGraph, pathToChange, testStatus } from './change';

const ringTitle: Record<number, string> = { 1: 'Direct callers', 2: 'Two calls away', 3: 'Three calls away' };
const ringNote: Record<number, string> = {
  1: 'call the changed code: the likeliest to break',
  2: 'reach it through a direct caller',
  3: 'further out',
};
const entryLabel = { main: 'main', exported: 'exported', uncalled: 'no callers' } as const;

/**
 * The blast radius of a change, closest first: direct callers in full with
 * their test status, further rings grouped by package, then the entry
 * points where the change surfaces. The package graph is on demand.
 */
export function ImpactPanel({ impact }: { impact: Impact }) {
  const [view, setView] = useState<'list' | 'graph'>('list');
  const byId = useMemo(() => nodesById(impact), [impact]);
  const graph = useMemo(() => (view === 'graph' ? packageGraph(impact) : undefined), [impact, view]);
  const sum = impact.summary;
  const total = sum.callers.reduce((a, b) => a + b, 0);

  if (impact.changed.length === 0) {
    return <Empty icon={<Network size={28} />} title="No existing code changes">The change only adds new code.</Empty>;
  }
  return (
    <div className="impact">
      <div className="impact-strip">
        <Tile n={sum.changed} label={`changed function${sum.changed === 1 ? '' : 's'}`} />
        <Tile n={sum.callers[0]} label="direct callers" sub={impact.tests && sum.callers[0] ? (sum.untested ? `${sum.untested} untested` : 'all tested') : undefined} warn={sum.untested > 0} />
        <Tile n={sum.callers[1] + sum.callers[2]} label="further out" sub={`${sum.callers[1]} · ${sum.callers[2]} by hop`} />
        <Tile n={sum.files} label="files" sub={`${sum.packages} packages`} />
        <Tile n={sum.entries} label="entry points reached" />
        <div className="grow" />
        <div className="seg">
          <button className={view === 'list' ? 'on' : ''} onClick={() => setView('list')} title="Callers by distance"><List size={13} /> List</button>
          <button className={view === 'graph' ? 'on' : ''} onClick={() => setView('graph')} title="Packages and how they reach the change"><Network size={13} /> Packages</button>
        </div>
      </div>
      {view === 'graph' && graph ? (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 200 }}>
          <GraphCanvas graph={graph} direction="RL" heat={Object.fromEntries(graph.nodes.map((n) => [n.id, 1 / ((n.depth ?? 0) + 1)]))} compact />
        </div>
      ) : (
        <div className="scroll impact-list">
          <section>
            <div className="impact-head"><b>Changed</b></div>
            <div className="row" style={{ flexWrap: 'wrap', gap: 6, padding: '0 8px 6px' }}>
              {impact.changed.map((n) => (
                <span key={n.id} className="sym-chip" onClick={() => actions.code(n.file, n.line)} title={`${n.file}:${n.line} · ${testStatus(n)}`}>
                  {n.name} {impact.tests && !n.tested && <span className="del">· untested</span>}
                </span>
              ))}
            </div>
          </section>
          {total === 0 && <div className="faint" style={{ padding: '4px 12px' }}>Nothing calls the changed code (it may be an entry point, a callback registered dynamically, or only called from tests).</div>}
          {impact.rings.map((r) => (
            <Ring key={r.depth} ring={r} impact={impact} byId={byId} />
          ))}
          {impact.entries.length > 0 && (
            <section>
              <div className="impact-head">
                <LogIn size={13} className="faint" /> <b>Where it surfaces</b>
                <span className="faint">callers nothing else calls</span>
              </div>
              {impact.entries.map((n) => (
                <div key={n.id} className="qa-row" onClick={() => actions.code(n.file, n.line)} title={pathToChange(n, byId).join(' → ')}>
                  <span className="mono grow ellipsis">{n.name}</span>
                  <span className="badge">{entryLabel[n.entry ?? 'uncalled']}</span>
                  <span className="faint" style={{ width: 120, textAlign: 'right' }}>{n.depth} call{n.depth > 1 ? 's' : ''} away</span>
                </div>
              ))}
            </section>
          )}
          {(impact.beyond ?? 0) > 0 && <div className="faint" style={{ padding: '6px 12px' }}>{impact.beyond} call paths continue past three calls.</div>}
          {(impact.heuristic ?? 0) > 0 && <div className="faint" style={{ padding: '0 12px 8px' }}>{impact.heuristic} symbols are matched by name (~), so some callers may be missing or extra.</div>}
        </div>
      )}
    </div>
  );
}

function Tile({ n, label, sub, warn }: { n: number; label: string; sub?: string; warn?: boolean }) {
  return (
    <div className="impact-tile">
      <span className="impact-n">{n}</span>
      <span className="impact-l">{label}</span>
      {sub && <span className={`impact-s ${warn ? 'del' : 'faint'}`}>{sub}</span>}
    </div>
  );
}

/** One distance ring: direct callers open, the others collapsed by package. */
function Ring({ ring, impact, byId }: { ring: ImpactRing; impact: Impact; byId: Map<string, ImpactNode> }) {
  const [open, setOpen] = useState(ring.depth === 1);
  return (
    <section className={`impact-ring d${ring.depth}`}>
      <div className="impact-head clickable" onClick={() => setOpen(!open)}>
        {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        <b>{ringTitle[ring.depth]}</b> <span className="mono">{ring.total}</span>
        <span className="faint">{ringNote[ring.depth]}</span>
        <div className="grow" />
        {impact.tests && <span className="faint">{ring.tested} of {ring.total} tested</span>}
        {ring.avgPct >= 0 && <CoverageBar pct={ring.avgPct} width={60} />}
      </div>
      {!open && (
        <div className="row faint" style={{ flexWrap: 'wrap', gap: 10, padding: '0 12px 6px 30px', fontSize: 11.5 }}>
          {ring.packages.slice(0, 8).map((p) => (
            <span key={p.path}><span className="mono">{p.path}</span> {p.count}</span>
          ))}
          {ring.packages.length > 8 && <span>+{ring.packages.length - 8} more</span>}
        </div>
      )}
      {open && ring.nodes.map((n) => <CallerRow key={n.id} n={n} impact={impact} byId={byId} />)}
      {open && (ring.more ?? 0) > 0 && <div className="faint" style={{ padding: '4px 30px' }}>+{ring.more} more not listed</div>}
    </section>
  );
}

function CallerRow({ n, impact, byId }: { n: ImpactNode; impact: Impact; byId: Map<string, ImpactNode> }) {
  const path = pathToChange(n, byId);
  return (
    <div className={`qa-row impact-row ${n.review ? 'review' : ''}`} onClick={() => actions.code(n.file, n.line)} title={path.join(' → ')}>
      <span className="grow ellipsis">
        <span className="mono">{n.name}</span>
        {n.heuristic && <span className="faint" title="matched by name"> ~</span>}
        <span className="faint"> via {path.slice(1).join(' → ')}</span>
      </span>
      <span className="faint ellipsis" style={{ width: 160, textAlign: 'right' }}>{basename(n.file)}:{n.line}</span>
      {impact.tests || impact.coverage ? (
        <>
          <span style={{ width: 130, textAlign: 'right' }} className={n.tested ? 'faint' : 'del'}>
            {n.review && <CircleAlert size={12} style={{ verticalAlign: -2, marginRight: 4 }} />}
            {n.tested ? `${n.direct ? `${n.direct} direct` : ''}${n.direct && n.indirect ? ' · ' : ''}${n.indirect ? `${n.indirect} indirect` : ''}` || 'covered' : 'untested'}
          </span>
          <span style={{ width: 110 }}><CoverageBar pct={n.pct} width={50} /></span>
        </>
      ) : null}
    </div>
  );
}
