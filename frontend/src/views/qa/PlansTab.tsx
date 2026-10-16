import { useEffect, useRef } from 'react';
import { Bot, Code2, FileCode2, Wrench, X } from 'lucide-react';
import { api } from '../../lib/api';
import { actions } from '../../lib/actions';
import { useStore } from '../../lib/store';
import { relTime } from '../../lib/format';
import { Empty, Markdown } from '../../components/ui';
import type { QAOverview, TestPlan } from '../../lib/types';

const kindClass: Record<string, string> = { happy: 'ok', edge: 'warn', error: 'err', regression: 'violet', property: 'accent' };

/** Test plans the agent posted with show_test_plan, newest first. */
export function PlansTab({ data, focus }: { data: QAOverview; focus: string }) {
  if (data.plans.length === 0) {
    return (
      <Empty icon={<Bot size={28} />} title="No test plans yet">
        Ask your agent something like “Using CodeDiver, find the riskiest untested code and show me a test plan for it.” Plans appear here with the behaviors to cover and any refactors the code needs first.
      </Empty>
    );
  }
  return (
    <div className="scroll" style={{ padding: 18 }}>
      {data.plans.map((p) => (
        <Plan key={p.id} plan={p} focused={p.id === focus} />
      ))}
    </div>
  );
}

/** One plan: summary, behaviors as given/when/then, and refactors. */
function Plan({ plan, focused }: { plan: TestPlan; focused: boolean }) {
  const s = useStore();
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (focused) ref.current?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  }, [focused]);
  return (
    <div ref={ref} className={`card plan ${focused ? 'focused' : ''}`} style={{ marginBottom: 14 }}>
      <div className="row" style={{ alignItems: 'flex-start', gap: 10 }}>
        <Bot size={16} style={{ color: 'var(--accent-2)', flex: 'none', marginTop: 2 }} />
        <div className="grow" style={{ minWidth: 0 }}>
          <b style={{ fontSize: 14 }}>{plan.title}</b>
          <div className="row faint" style={{ gap: 10, flexWrap: 'wrap', fontSize: 11.5, marginTop: 2 }}>
            {plan.symbolName && (
              <span className="ref" style={{ padding: 0 }} onClick={() => plan.file && actions.code(plan.file, plan.line, plan.endLine)}>
                <Code2 size={12} /> <span className="mono">{plan.symbolName}</span>
              </span>
            )}
            {plan.testFile && (
              <span className="ref" style={{ padding: 0 }} onClick={() => actions.code(plan.testFile!)}>
                <FileCode2 size={12} /> tests in <span className="mono">{plan.testFile}</span>
              </span>
            )}
            <span>{relTime(plan.created)}</span>
          </div>
        </div>
        <button className="btn ghost icon sm" title="Dismiss" onClick={() => api.qaDismissPlan(s.repoId, plan.id).then(() => s.bump())}>
          <X size={13} />
        </button>
      </div>
      {plan.summary && <Markdown src={plan.summary} className="plan-summary" />}

      {plan.behaviors.length > 0 && (
        <>
          <div className="section-title" style={{ paddingLeft: 0 }}>Behaviors to test ({plan.behaviors.length})</div>
          <div className="plan-behaviors">
            {plan.behaviors.map((b, i) => (
              <div key={i} className="plan-behavior" onClick={() => b.line && plan.file && actions.code(plan.file, b.line, b.end_line || b.line)}>
                <div className="row" style={{ gap: 8 }}>
                  <span className={`badge ${kindClass[b.kind ?? ''] ?? ''}`}>{b.kind || 'case'}</span>
                  <b className="grow">{b.name}</b>
                  {b.existing && <span className="badge ok" title={`Already covered by ${b.existing}`}>covered</span>}
                  {b.line ? <span className="loc mono">L{b.line}{b.end_line && b.end_line !== b.line ? `-${b.end_line}` : ''}</span> : null}
                </div>
                {(b.given || b.when || b.then) && (
                  <div className="gwt">
                    {b.given && <div><span>Given</span> {b.given}</div>}
                    {b.when && <div><span>When</span> {b.when}</div>}
                    {b.then && <div><span>Then</span> {b.then}</div>}
                  </div>
                )}
              </div>
            ))}
          </div>
        </>
      )}

      {(plan.refactors?.length ?? 0) > 0 && (
        <>
          <div className="section-title" style={{ paddingLeft: 0 }}><Wrench size={12} /> Changes needed first</div>
          {plan.refactors!.map((r, i) => (
            <div key={i} className="hint" onClick={() => r.file && actions.code(r.file, r.line, r.end_line)}>
              <div className="row" style={{ gap: 8 }}>
                <b className="grow">{r.title}</b>
                {r.file && <span className="loc mono">{r.file}{r.line ? `:${r.line}` : ''}</span>}
              </div>
              {r.detail && <Markdown src={r.detail} />}
            </div>
          ))}
        </>
      )}
      <div className="faint" style={{ marginTop: 10, fontSize: 11.5 }}>
        Reply to your agent to go ahead: it proposes the tests (and any refactor) as a change set you review before anything is written.
      </div>
    </div>
  );
}
