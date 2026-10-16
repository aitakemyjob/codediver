import { useMemo, useState } from 'react';
import { ArrowDownLeft, ArrowUpRight, ChevronRight, Package, Route, Workflow, X } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import { plural, shortType } from '../lib/format';
import type { FileSummary, FileSymbol, Variable } from '../lib/types';
import { ErrorBox, KindIcon, Spinner, useAsync } from './ui';

export type SortKey = 'line' | 'complexity' | 'callers';

/** Pure: filter and order functions for the summary list. */
export function arrangeFunctions(fns: FileSymbol[], query: string, sort: SortKey): FileSymbol[] {
  const q = query.trim().toLowerCase();
  const out = q ? fns.filter((f) => f.qualified.toLowerCase().includes(q) || (f.doc ?? '').toLowerCase().includes(q)) : [...fns];
  const by: Record<SortKey, (a: FileSymbol, b: FileSymbol) => number> = {
    line: (a, b) => (a.line ?? 0) - (b.line ?? 0),
    complexity: (a, b) => (b.complexity ?? 0) - (a.complexity ?? 0) || (a.line ?? 0) - (b.line ?? 0),
    callers: (a, b) => b.callers - a.callers || (a.line ?? 0) - (b.line ?? 0),
  };
  return out.sort(by[sort]);
}

/** First line of a doc comment. */
const firstLine = (s?: string) => (s ?? '').split('\n')[0];

/** Collapsible section with a count badge; hidden when empty. */
function Section({ title, count, children, defaultOpen = true }: { title: string; count: number; children: React.ReactNode; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen);
  if (count === 0) return null;
  return (
    <>
      <div className="section-title" style={{ cursor: 'pointer' }} onClick={() => setOpen(!open)}>
        <ChevronRight size={12} className={`chev ${open ? 'open' : ''}`} style={{ transition: 'transform .12s', transform: open ? 'rotate(90deg)' : undefined }} />
        {title} <span className="badge">{count}</span>
      </div>
      {open && children}
    </>
  );
}

/**
 * One function: inputs → outputs, complexity, usage counts, doc, and
 * hover actions for trace and logic.
 */
function FunctionRow({ f, file }: { f: FileSymbol; file: string }) {
  const s = useStore();
  const ins = shortType(f.params);
  const outs = shortType(f.results);
  return (
    <div className={`sum-row ${s.inspect === f.id ? 'on' : ''}`} onClick={() => { actions.code(file, f.line, f.endLine); s.inspectSymbol(f.id); }} title={f.signature}>
      <div className="row" style={{ gap: 6 }}>
        <KindIcon kind={f.kind} size={18} />
        <span className="mono ellipsis grow" style={{ fontWeight: 600 }}>{f.qualified}</span>
        {f.complexity ? <span className={`badge ${f.complexity > 10 ? 'warn' : ''}`} title="cyclomatic complexity">cx {f.complexity}</span> : null}
      </div>
      <div className="sum-io mono">
        <span style={{ color: 'var(--syn-type)' }}>({ins})</span> <span className="faint">→</span> <span style={{ color: 'var(--syn-string)' }}>{outs || '∅'}</span>
      </div>
      {f.doc && <div className="sum-doc ellipsis">{firstLine(f.doc)}</div>}
      <div className="row sum-meta">
        <span title="callers"><ArrowDownLeft size={11} /> {f.callers}</span>
        <span title="calls"><ArrowUpRight size={11} /> {f.callees}</span>
        <span className="faint mono">L{f.line}</span>
        <span className="grow" />
        <button className="btn ghost icon sm" title="Trace to here" onClick={(e) => { e.stopPropagation(); actions.trace(f); }}><Route size={12} /></button>
        <button className="btn ghost icon sm" title="Logic flow" onClick={(e) => { e.stopPropagation(); actions.flow(f); }}><Workflow size={12} /></button>
      </div>
    </div>
  );
}

/** One type with expandable fields and its method count. */
function TypeRow({ t, file }: { t: FileSymbol; file: string }) {
  const [open, setOpen] = useState(false);
  const fields = t.fields ?? [];
  return (
    <div className="sum-row" onClick={() => actions.code(file, t.line, t.endLine)}>
      <div className="row" style={{ gap: 6 }}>
        <KindIcon kind={t.kind} size={18} />
        <span className="mono ellipsis grow" style={{ fontWeight: 600 }}>{t.qualified}</span>
        <span className="badge">{t.kind}</span>
      </div>
      <div className="row sum-meta">
        {fields.length > 0 && (
          <span style={{ cursor: 'pointer' }} onClick={(e) => { e.stopPropagation(); setOpen(!open); }}>
            {plural(fields.length, 'field')} {open ? '▾' : '▸'}
          </span>
        )}
        {t.methods?.length ? <span>{plural(t.methods.length, 'method')}</span> : null}
        <span className="faint mono">L{t.line}</span>
      </div>
      {open && (
        <div className="sum-fields mono">
          {fields.map((f, i) => (
            <div key={i} className="ellipsis"><b>{f.name}</b> <span style={{ color: 'var(--syn-type)' }}>{f.type}</span></div>
          ))}
        </div>
      )}
      {t.doc && <div className="sum-doc ellipsis">{firstLine(t.doc)}</div>}
    </div>
  );
}

/** One top-level variable or constant with its type and value. */
function VariableRow({ v, file }: { v: Variable; file: string }) {
  return (
    <div className="sum-row" onClick={() => actions.code(file, v.line)} title={v.doc}>
      <div className="row mono" style={{ gap: 6, fontSize: 12 }}>
        <span className={`badge ${v.kind === 'const' ? 'accent' : ''}`}>{v.kind}</span>
        <b className="ellipsis">{v.name}</b>
        {v.type && <span className="ellipsis" style={{ color: 'var(--syn-type)' }}>{v.type}</span>}
        <span className="grow" />
        <span className="faint">L{v.line}</span>
      </div>
      {v.value && <div className="sum-io mono ellipsis"><span className="faint">=</span> {v.value}</div>}
      {v.doc && <div className="sum-doc ellipsis">{v.doc}</div>}
    </div>
  );
}

/**
 * Side panel listing everything declared in a file: functions, types,
 * variables/constants and imports, with filtering and sorting.
 */
export function FileSummaryPanel({ file, onClose }: { file: string; onClose: () => void }) {
  const s = useStore();
  const res = useAsync<FileSummary>(() => api.fileSummary(s.repoId, file), [file, s.repoId, s.refreshKey]);
  const [q, setQ] = useState('');
  const [sort, setSort] = useState<SortKey>('line');
  const d = res.data;
  const fns = useMemo(() => (d ? arrangeFunctions(d.functions, q, sort) : []), [d, q, sort]);
  const ql = q.trim().toLowerCase();
  const types = (d?.types ?? []).filter((t) => !ql || t.qualified.toLowerCase().includes(ql));
  const vars = (d?.variables ?? []).filter((v) => !ql || v.name.toLowerCase().includes(ql));
  const avgCx = d && d.functions.length ? Math.round((d.functions.reduce((n, f) => n + (f.complexity ?? 0), 0) / d.functions.length) * 10) / 10 : 0;

  return (
    <aside className="summary-panel">
      <div className="panel-head">
        File summary
        <div className="actions">
          <button className="btn ghost icon sm" onClick={onClose} aria-label="Close summary"><X size={13} /></button>
        </div>
      </div>
      {res.loading && !d && <Spinner />}
      {res.error ? <ErrorBox error={res.error} /> : null}
      {d && (
        <>
          <div style={{ padding: '0 12px 10px' }} className="col">
            <div className="stats" style={{ gridTemplateColumns: 'repeat(4, 1fr)', gap: 6 }}>
              <div className="stat"><b style={{ fontSize: 18 }}>{d.functions.length}</b><span>funcs</span></div>
              <div className="stat"><b style={{ fontSize: 18 }}>{d.types.length}</b><span>types</span></div>
              <div className="stat"><b style={{ fontSize: 18 }}>{d.variables.length}</b><span>vars</span></div>
              <div className="stat"><b style={{ fontSize: 18 }}>{avgCx}</b><span>avg cx</span></div>
            </div>
            <div className="faint" style={{ fontSize: 11.5 }}>
              {d.language} · {d.lines} lines{d.heuristic ? ' · symbols found heuristically' : ''}
            </div>
            <input className="input" placeholder="Filter…" value={q} onChange={(e) => setQ(e.target.value)} />
            <div className="seg" style={{ alignSelf: 'flex-start' }}>
              <button className={sort === 'line' ? 'on' : ''} onClick={() => setSort('line')}>Order</button>
              <button className={sort === 'complexity' ? 'on' : ''} onClick={() => setSort('complexity')}>Complexity</button>
              <button className={sort === 'callers' ? 'on' : ''} onClick={() => setSort('callers')}>Most used</button>
            </div>
          </div>
          <div className="panel-body">
            <Section title="Functions" count={fns.length}>
              {fns.map((f) => <FunctionRow key={f.id} f={f} file={d.path} />)}
            </Section>
            <Section title="Types" count={types.length}>
              {types.map((t) => <TypeRow key={t.id} t={t} file={d.path} />)}
            </Section>
            <Section title="Variables & constants" count={vars.length}>
              {vars.map((v) => <VariableRow key={v.name + v.line} v={v} file={d.path} />)}
            </Section>
            <Section title="Imports" count={d.imports.length} defaultOpen={false}>
              <div className="row" style={{ flexWrap: 'wrap', gap: 4, padding: '0 8px 8px' }}>
                {d.imports.map((im) => (
                  <span key={im} className="sym-chip" title={im}><Package size={10} /> {im}</span>
                ))}
              </div>
            </Section>
            {d.functions.length + d.types.length + d.variables.length === 0 && (
              <div className="faint" style={{ padding: 12 }}>Nothing declared here that CodeDiver recognizes.</div>
            )}
          </div>
        </>
      )}
    </aside>
  );
}
