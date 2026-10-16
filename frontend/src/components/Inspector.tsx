import { ArrowDownLeft, ArrowUpRight, Code2, GitFork, Layers, Route, Workflow, X } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import type { CodeSymbol, EdgeRef } from '../lib/types';
import { KindIcon, Markdown, useAsync } from './ui';

/** A clickable symbol reference with its edge kind and location. */
function RefRow({ sym, kind, onClick }: { sym: CodeSymbol; kind?: string; onClick: () => void }) {
  return (
    <div className="ref" onClick={onClick} title={sym.id}>
      <KindIcon kind={sym.kind} size={16} />
      <span className="mono ellipsis">{sym.qualified}</span>
      {kind && kind !== 'call' && <span className="badge">{kind}</span>}
      <span className="loc">{sym.kind === 'external' ? sym.package : `${sym.file?.split('/').pop()}:${sym.line}`}</span>
    </div>
  );
}

/**
 * Right panel showing the selected symbol: signature, inputs/outputs,
 * metrics, doc, agent explanation, callers, callees and related types.
 */
export function Inspector() {
  const s = useStore();
  const id = s.inspect;
  const detail = useAsync(() => (id ? api.symbol(s.repoId, id) : Promise.resolve(undefined)), [id, s.repoId, s.refreshKey]);
  const d = detail.data;
  const sym = d?.symbol;
  const go = (x: CodeSymbol) => (x.kind === 'external' ? undefined : actions.symbol(x));
  const list = (title: string, icon: React.ReactNode, refs: EdgeRef[]) =>
    refs.length > 0 && (
      <>
        <div className="section-title">
          {icon} {title} <span className="badge">{refs.length}</span>
        </div>
        {refs.slice(0, 60).map((r, i) => (
          <RefRow key={r.symbol.id + i} sym={r.symbol} kind={r.kind} onClick={() => go(r.symbol)} />
        ))}
      </>
    );
  return (
    <aside className="inspector">
      <div className="panel-head">
        Inspector
        <div className="actions">
          <button className="btn ghost icon sm" onClick={() => s.toggleInspector(false)} aria-label="Close inspector">
            <X size={13} />
          </button>
        </div>
      </div>
      <div className="panel-body" style={{ padding: '0 12px 16px' }}>
        {!id && (
          <div className="faint" style={{ padding: '8px 2px', lineHeight: 1.6 }}>
            Select a symbol — click a code lens, an outline entry or a graph node — to see its signature, inputs and outputs, callers and callees.
          </div>
        )}
        {detail.error ? <div className="error-box">{String((detail.error as Error).message ?? detail.error)}</div> : null}
        {sym && d && (
          <div className="col" style={{ gap: 10 }}>
            <div className="row" style={{ alignItems: 'flex-start' }}>
              <KindIcon kind={sym.kind} size={28} />
              <div className="grow">
                <div className="mono" style={{ fontWeight: 700, fontSize: 14, wordBreak: 'break-all' }}>{sym.qualified}</div>
                <div className="faint mono" style={{ fontSize: 11 }}>
                  {sym.file}:{sym.line}
                  {sym.heuristic && <span className="badge warn" style={{ marginLeft: 6 }}>heuristic</span>}
                </div>
              </div>
            </div>
            {sym.signature && <div className="sig">{sym.signature}</div>}
            <div className="row" style={{ flexWrap: 'wrap', gap: 6 }}>
              <button className="btn sm" onClick={() => actions.symbol(sym)}><Code2 size={13} /> Code</button>
              {(sym.kind === 'func' || sym.kind === 'method') && (
                <>
                  <button className="btn sm" onClick={() => actions.flow(sym)}><Workflow size={13} /> Logic</button>
                  <button className="btn sm" onClick={() => actions.trace(sym)}><Route size={13} /> Trace to here</button>
                  <button className="btn sm" onClick={() => actions.callGraph(sym)}><GitFork size={13} /> Calls</button>
                  <button className="btn sm" onClick={() => actions.callGraph(sym, 'callers')}><ArrowDownLeft size={13} /> Callers</button>
                </>
              )}
              {(sym.kind === 'struct' || sym.kind === 'class' || sym.kind === 'interface') && (
                <button className="btn sm" onClick={() => actions.types(sym.package || sym.file || '', sym.package?.split('/').pop())}><Layers size={13} /> Type diagram</button>
              )}
            </div>
            {(sym.params?.length || sym.results?.length) ? (
              <div className="card" style={{ padding: 10 }}>
                <dl className="kv">
                  {sym.params?.map((p, i) => (
                    <FragmentKV key={'p' + i} k={<span>in · {p.name || `#${i + 1}`}</span>} v={p.type} color="var(--syn-type)" />
                  ))}
                  {sym.results?.map((p, i) => (
                    <FragmentKV key={'r' + i} k={<span>out · {p.name || `#${i + 1}`}</span>} v={p.type} color="var(--syn-string)" />
                  ))}
                </dl>
              </div>
            ) : null}
            {sym.fields && sym.fields.length > 0 && (
              <div className="card" style={{ padding: 10 }}>
                <dl className="kv">
                  {sym.fields.map((f, i) => (
                    <FragmentKV key={i} k={f.name} v={f.type} color="var(--syn-type)" />
                  ))}
                </dl>
              </div>
            )}
            <div className="row" style={{ gap: 6, flexWrap: 'wrap' }}>
              {sym.complexity ? <span className={`badge ${sym.complexity > 10 ? 'warn' : ''}`}>complexity {sym.complexity}</span> : null}
              <span className="badge">{d.callers.length} callers</span>
              <span className="badge">{d.callees.length} callees</span>
              {sym.exported && <span className="badge accent">exported</span>}
            </div>
            {sym.doc && <Markdown src={sym.doc} className="muted" />}
            {d.explanation && (
              <div className="card" style={{ borderColor: 'color-mix(in srgb, var(--accent-2) 40%, transparent)' }}>
                <h3 style={{ color: 'var(--accent-2)' }}>Agent explanation</h3>
                <Markdown src={d.explanation.summary} />
              </div>
            )}
            {list('Called by', <ArrowDownLeft size={12} />, d.callers)}
            {list('Calls', <ArrowUpRight size={12} />, d.callees)}
            {d.implementations && d.implementations.length > 0 && (
              <>
                <div className="section-title">Implementations</div>
                {d.implementations.map((x) => <RefRow key={x.id} sym={x} onClick={() => go(x)} />)}
              </>
            )}
            {d.implements && d.implements.length > 0 && (
              <>
                <div className="section-title">Implements</div>
                {d.implements.map((x) => <RefRow key={x.id} sym={x} onClick={() => go(x)} />)}
              </>
            )}
            {d.methods && d.methods.length > 0 && (
              <>
                <div className="section-title">Methods</div>
                {d.methods.map((x) => <RefRow key={x.id} sym={x} onClick={() => go(x)} />)}
              </>
            )}
          </div>
        )}
      </div>
    </aside>
  );
}

/** A term/definition pair inside a <dl>. */
function FragmentKV({ k, v, color }: { k: React.ReactNode; v: string; color: string }) {
  return (
    <>
      <dt>{k}</dt>
      <dd style={{ color }}>{v || '—'}</dd>
    </>
  );
}
