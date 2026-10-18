import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { ChevronLeft, ChevronRight, Map as MapIcon, Route } from 'lucide-react';
import { useStore } from '../lib/store';
import { basename } from '../lib/format';
import type { Graph } from '../lib/types';
import { GraphCanvas } from '../graph/GraphCanvas';
import { Empty, Markdown, Spinner } from '../components/ui';
import type { ViewProps } from './ViewHost';

/** Loaded on demand (pulls in Monaco). */
const CodeView = lazy(() => import('./CodeView'));

/**
 * Guided tour: steps list, narration, the highlighted code, and a map of
 * the path. Arrow keys move between steps.
 */
/**
 * Whether a key event comes from somewhere the developer is typing (the
 * editor or a text field), where arrow keys must not change steps. Key
 * events can target the window or document, which aren't elements.
 */
export function isTypingTarget(target: EventTarget | null): boolean {
  return target instanceof Element && !!target.closest('.monaco-editor, input, textarea');
}

export default function WalkthroughView({ tab, active }: ViewProps) {
  const s = useStore();
  const w = s.session.walkthroughs.find((x) => x.id === tab.params.id);
  const [i, setI] = useState(0);
  const [showMap, setShowMap] = useState(true);
  const n = w?.steps.length ?? 0;

  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if (isTypingTarget(e.target)) return;
      if (e.key === 'ArrowRight' || e.key === 'j') setI((x) => Math.min(n - 1, x + 1));
      if (e.key === 'ArrowLeft' || e.key === 'k') setI((x) => Math.max(0, x - 1));
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [active, n]);

  const map: Graph | undefined = useMemo(() => {
    if (!w) return undefined;
    return {
      kind: 'custom',
      title: w.title,
      direction: 'LR',
      nodes: w.steps.map((st, k) => ({ id: `s${k}`, label: `${k + 1}. ${st.title}`, kind: k === i ? 'service' : 'step', detail: `${basename(st.file)}${st.line ? `:${st.line}` : ''}`, heat: k === i ? 0.5 : k < i ? 0.2 : 0 })),
      edges: w.steps.slice(1).map((_, k) => ({ id: `e${k}`, from: `s${k}`, to: `s${k + 1}`, kind: 'call', highlight: k === i - 1 })),
    };
  }, [w, i]);

  if (!w) return <Empty icon={<Route size={28} />} title="Walkthrough not found">Walkthroughs live for this session only.</Empty>;
  const st = w.steps[i];
  const codeTab = { id: `${tab.id}-code`, kind: 'code' as const, title: basename(st.file), params: { file: st.file, line: st.line, endLine: st.end_line, flash: i } };

  return (
    <>
      <div className="progress"><i style={{ width: `${((i + 1) / n) * 100}%` }} /></div>
      <div className="split">
        <div className="side" style={{ width: 290 }}>
          <div style={{ padding: '14px 14px 4px' }}>
            <div className="row" style={{ color: 'var(--accent)', fontWeight: 650 }}><Route size={15} /> Walkthrough</div>
            <h3 style={{ margin: '6px 0 4px', fontSize: 15 }}>{w.title}</h3>
            {w.summary && <Markdown src={w.summary} className="muted" />}
          </div>
          <div className="steps">
            {w.steps.map((x, k) => (
              <div key={k} className={`step ${k === i ? 'on' : ''} ${k < i ? 'done' : ''}`} onClick={() => setI(k)}>
                <span className="num">{k + 1}</span>
                <div className="grow" style={{ minWidth: 0 }}>
                  <div className="st-title">{x.title}</div>
                  <div className="st-loc ellipsis">{x.file}{x.line ? `:${x.line}` : ''}</div>
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="main">
          <div className="narration">
            <div className="row" style={{ marginBottom: 8 }}>
              <span className="badge violet">Step {i + 1} of {n}</span>
              <b style={{ fontSize: 14 }}>{st.title}</b>
              <div className="grow" />
              <button className={`btn sm ${showMap ? 'active' : ''}`} onClick={() => setShowMap(!showMap)}><MapIcon size={13} /> Map</button>
              <button className="btn sm" disabled={i === 0} onClick={() => setI(i - 1)}><ChevronLeft size={14} /> Prev</button>
              <button className="btn sm primary" disabled={i === n - 1} onClick={() => setI(i + 1)}>Next <ChevronRight size={14} /></button>
            </div>
            <Markdown src={st.explanation} />
          </div>
          <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', position: 'relative' }}>
            <Suspense fallback={<Spinner />}>
              <CodeView tab={codeTab} active={active} />
            </Suspense>
          </div>
          {showMap && map && (
            <div style={{ height: 150, borderTop: '1px solid var(--border)', display: 'flex' }}>
              <GraphCanvas graph={map} direction="LR" legend={false} compact onSelect={(node) => setI(Number(node.id.slice(1)))} />
            </div>
          )}
        </div>
      </div>
    </>
  );
}
