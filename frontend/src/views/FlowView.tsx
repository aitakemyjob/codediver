import { useEffect, useMemo, useState } from 'react';
import { ArrowRight, Bot, Code2, GitFork, Route, Sparkles } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import type { Explanation, Flow, Param } from '../lib/types';
import { FlowTrail } from '../lib/flowTrail';
import { FlowCanvas } from '../graph/FlowCanvas';
import { CopyButton, ErrorBox, Markdown, Spinner, useAsync } from '../components/ui';
import type { ViewProps } from './ViewHost';

/** Maps each explanation step onto the flow nodes overlapping its lines. */
export function stepNodes(flow: Flow, ex?: Explanation): { notes: Map<string, { text: string; n: number }>; byStep: Map<number, Set<string>> } {
  const notes = new Map<string, { text: string; n: number }>();
  const byStep = new Map<number, Set<string>>();
  ex?.steps?.forEach((st, i) => {
    if (!st.line) return;
    const lo = st.line;
    const hi = st.end_line || st.line;
    const ids = new Set<string>();
    for (const n of flow.nodes) {
      if (n.kind === 'start' || n.kind === 'end' || !n.line) continue;
      const nlo = n.line;
      const nhi = n.endLine || n.line;
      if (nlo <= hi && lo <= nhi) ids.add(n.id);
    }
    byStep.set(i, ids);
    // Attach the note to the first (top-most) matching node only.
    const first = flow.nodes.find((n) => ids.has(n.id) && !notes.has(n.id));
    if (first) notes.set(first.id, { text: st.text, n: i + 1 });
  });
  return { notes, byStep };
}

/** Breadcrumbs from the function into the closure being shown. */
function FlowCrumbs({ trail, onBack }: { trail: FlowTrail; onBack: (depth: number) => void }) {
  const crumbs = trail.crumbs();
  if (crumbs.length < 2) return null;
  return (
    <span className="flow-crumbs mono">
      {crumbs.map((c, i) => (
        <span key={c.depth}>
          {i > 0 && <span className="faint">›</span>}
          {i < crumbs.length - 1 ? (
            <button type="button" onClick={() => onBack(c.depth)}>{c.label}</button>
          ) : (
            <span className="here">{c.label}</span>
          )}
        </span>
      ))}
    </span>
  );
}

/** One input or output with the agent's description. */
function ParamCard({ p, desc, receiver }: { p: Param; desc?: string; receiver?: boolean }) {
  return (
    <div className="param">
      <div className="pn">
        {receiver && <span className="badge" style={{ marginRight: 6 }}>recv</span>}
        <b>{p.name || '·'}</b> <span>{p.type}</span>
      </div>
      {desc && <div className="pd">{desc}</div>}
    </div>
  );
}

/**
 * Logic view: signature card (inputs → function → outputs), control-flow
 * graph, and the agent's step-by-step explanation.
 */
export default function FlowView({ tab }: ViewProps) {
  const s = useStore();
  const symbol = String(tab.params.symbol ?? '');
  const res = useAsync(() => api.flow(s.repoId, symbol), [symbol, s.repoId, s.refreshKey]);
  const ex = s.session.explanations.find((x) => x.symbol === symbol || x.symbol === res.data?.symbol.id);
  const [selected, setSelected] = useState<string>();
  const [step, setStep] = useState<number>();
  const rootFlow = ex?.flow ?? res.data?.flow;
  const sym = res.data?.symbol ?? ex?.sym;
  const [trail, setTrail] = useState<FlowTrail>();
  // A new function (or a rebuilt flow) starts back at its top level.
  useEffect(() => setTrail(rootFlow ? new FlowTrail(rootFlow) : undefined), [rootFlow]);
  const flow = trail?.root === rootFlow ? trail?.current() : rootFlow;
  const { notes, byStep } = useMemo(() => (flow ? stepNodes(flow, ex) : { notes: new Map(), byStep: new Map() }), [flow, ex]);
  const highlight = step !== undefined ? byStep.get(step) : undefined;

  if (res.loading && !flow) return <Spinner label="Building control flow…" />;
  if (res.error && !flow) return <ErrorBox error={res.error} />;
  if (!flow || !sym) return null;
  const descIn = new Map(ex?.inputs?.map((x) => [x.name, x.description]));
  const descOut = new Map(ex?.outputs?.map((x) => [x.name, x.description]));
  const openNode = (id: string) => {
    const n = flow.nodes.find((x) => x.id === id);
    if (n?.line) actions.code(flow.file, n.line, n.endLine);
  };
  const prompt = `Using CodeDiver, explain ${sym.qualified} (${sym.file}:${sym.line}) with explain_function: describe each input and output and walk through its logic step by step.`;

  return (
    <>
      <div className="toolbar">
        <span className="title mono">{sym.qualified}</span>
        {trail && <FlowCrumbs trail={trail} onBack={(d) => setTrail(trail.back(d))} />}
        <span className="faint mono" style={{ fontSize: 11.5 }}>{flow.file}:{flow.line}</span>
        {sym.complexity ? <span className={`badge ${sym.complexity > 10 ? 'warn' : ''}`}>complexity {sym.complexity}</span> : null}
        <span className="badge">{flow.nodes.length - 2} blocks</span>
        <div className="grow" />
        <button className="btn sm" onClick={() => actions.code(flow.file, flow.line, flow.endLine)}><Code2 size={13} /> Code</button>
        <button className="btn sm" onClick={() => actions.trace(sym)}><Route size={13} /> Trace to here</button>
        <button className="btn sm" onClick={() => actions.callGraph(sym)}><GitFork size={13} /> Calls</button>
      </div>
      <div className="sigcard">
        <div className="io">
          <h4>Inputs</h4>
          {flow.receiver && <ParamCard p={flow.receiver} desc={descIn.get(flow.receiver.name)} receiver />}
          {(flow.inputs ?? []).map((p, i) => <ParamCard key={i} p={p} desc={descIn.get(p.name)} />)}
          {!flow.receiver && !(flow.inputs ?? []).length && <div className="faint">none</div>}
        </div>
        <div className="flowarrow"><ArrowRight size={18} /></div>
        <div className="fn">
          <b>{flow === rootFlow ? sym.name : flow.name}</b>
          <small className="mono" style={{ maxWidth: 320, whiteSpace: 'normal', wordBreak: 'break-word' }}>{flow.signature}</small>
        </div>
        <div className="flowarrow"><ArrowRight size={18} /></div>
        <div className="io">
          <h4>Outputs</h4>
          {(flow.outputs ?? []).map((p, i) => <ParamCard key={i} p={p} desc={descOut.get(p.name) ?? descOut.get(p.type)} />)}
          {!(flow.outputs ?? []).length && <div className="faint">none</div>}
        </div>
      </div>
      <div className="split">
        <div className="main">
          <FlowCanvas
            flow={flow}
            notes={notes}
            selected={selected}
            highlight={highlight}
            onSelect={setSelected}
            onOpen={openNode}
            onClosure={trail ? (nodeId, i) => setTrail(trail.open(nodeId, i)) : undefined}
          />
        </div>
        <div className="side right" style={{ width: 340 }}>
          {ex ? (
            <div style={{ padding: 14 }} className="col">
              <div className="row" style={{ color: 'var(--accent-2)', fontWeight: 650 }}><Sparkles size={15} /> Agent explanation</div>
              <Markdown src={ex.summary} />
              {ex.steps && ex.steps.length > 0 && (
                <div className="steps" style={{ padding: 0 }}>
                  {ex.steps.map((st, i) => (
                    <div
                      key={i}
                      className={`step ${step === i ? 'on' : ''}`}
                      onMouseEnter={() => setStep(i)}
                      onMouseLeave={() => setStep(undefined)}
                      onClick={() => st.line && actions.code(flow.file, st.line, st.end_line)}
                    >
                      <span className="num">{i + 1}</span>
                      <div className="grow">
                        <div>{st.text}</div>
                        {st.line ? <div className="st-loc">L{st.line}{st.end_line && st.end_line !== st.line ? `–${st.end_line}` : ''}</div> : null}
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {ex.sideEffects && ex.sideEffects.length > 0 && (
                <div className="card" style={{ padding: 10 }}>
                  <h3>Side effects</h3>
                  <ul style={{ margin: 0, paddingLeft: 18 }}>{ex.sideEffects.map((x, i) => <li key={i}>{x}</li>)}</ul>
                </div>
              )}
              {ex.notes && (
                <div className="card" style={{ padding: 10 }}>
                  <h3>Notes</h3>
                  <Markdown src={ex.notes} />
                </div>
              )}
            </div>
          ) : (
            <div style={{ padding: 16 }} className="col">
              <div className="row" style={{ fontWeight: 650 }}><Bot size={16} style={{ color: 'var(--accent-2)' }} /> No explanation yet</div>
              <div className="muted">
                {sym.language === 'go'
                  ? 'The control-flow graph is derived from the code. Ask your agent to narrate it — its steps will be pinned onto the matching blocks.'
                  : 'Control flow is derived precisely for Go. For other languages, ask your agent to explain this function step by step.'}
              </div>
              <div className="snippet">{prompt}<CopyButton text={prompt} /></div>
              {sym.doc && (
                <div className="card" style={{ padding: 10 }}>
                  <h3>Doc comment</h3>
                  <div className="muted" style={{ whiteSpace: 'pre-wrap' }}>{sym.doc}</div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </>
  );
}
