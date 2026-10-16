import { useEffect, useMemo, useState } from 'react';
import { Bot, Code2, Lightbulb, Route, Search, Workflow, Wrench } from 'lucide-react';
import { api } from '../../lib/api';
import { actions } from '../../lib/actions';
import { useStore } from '../../lib/store';
import { CopyButton, Spinner, useAsync } from '../../components/ui';
import type { FuncQA, QAOverview } from '../../lib/types';
import { CoverageBar, gapPrompt, hintLabels } from './qa';

/** Whether a gap has hard-wired dependencies (not just many branches). */
const hardWired = (g: FuncQA) => (g.hints ?? []).some((h) => h.kind !== 'complexity' && h.kind !== 'params');

/**
 * Functions that most need tests, ranked by missing coverage × complexity
 * × callers, with uncovered lines, the tests that reach them and what
 * makes them hard to test.
 */
export function GapsTab({ data }: { data: QAOverview }) {
  const [q, setQ] = useState('');
  const [hintsOnly, setHintsOnly] = useState(false);
  const [selected, setSelected] = useState<string>(data.gaps[0]?.id ?? '');
  const gaps = useMemo(() => {
    const query = q.toLowerCase();
    return data.gaps.filter((g) => (!query || g.name.toLowerCase().includes(query) || g.file.toLowerCase().includes(query)) && (!hintsOnly || hardWired(g)));
  }, [data.gaps, q, hintsOnly]);
  const maxRisk = Math.max(1, ...data.gaps.map((g) => g.risk));
  const gap = data.gaps.find((g) => g.id === selected);

  return (
    <div className="split" style={{ minHeight: 0, flex: 1 }}>
      <div className="main" style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        <div className="toolbar" style={{ minHeight: 38 }}>
          <div className="row" style={{ gap: 6 }}>
            <Search size={13} className="faint" />
            <input className="input" style={{ height: 26, width: 240 }} placeholder="Filter functions or paths…" value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
          <label className="row faint" style={{ gap: 6, cursor: 'pointer' }}>
            <input type="checkbox" checked={hintsOnly} onChange={(e) => setHintsOnly(e.target.checked)} /> hard-wired dependencies only
          </label>
          <div className="grow" />
          <span className="faint">{data.coverage ? 'ranked by uncovered × complexity × callers' : 'ranked by static test reach (no coverage yet)'}</span>
        </div>
        <div className="scroll" style={{ padding: '6px 10px' }}>
          <div className="qa-table">
            <div className="qa-row head">
              <span style={{ width: 26 }}>#</span>
              <span className="grow">Function</span>
              <span style={{ width: 120 }}>Coverage</span>
              <span style={{ width: 44, textAlign: 'right' }} title="cyclomatic complexity">cx</span>
              <span style={{ width: 56, textAlign: 'right' }} title="callers">callers</span>
              <span style={{ width: 64, textAlign: 'right' }} title="tests calling it directly / reaching it indirectly">tests</span>
              <span style={{ width: 70 }}>Risk</span>
            </div>
            {gaps.map((g, i) => (
              <div key={g.id} className={`qa-row ${selected === g.id ? 'on' : ''}`} onClick={() => setSelected(g.id)} onDoubleClick={() => actions.code(g.file, g.line, g.endLine)}>
                <span className="mono faint" style={{ width: 26 }}>{i + 1}</span>
                <span className="grow ellipsis">
                  <span className="mono">{g.name}</span> <span className="faint" style={{ fontSize: 11 }}>{g.file}:{g.line}</span>
                  {hardWired(g) && <Wrench size={11} style={{ color: 'var(--warn)', marginLeft: 6, verticalAlign: -1 }} aria-label="hard-wired dependencies" />}
                </span>
                <span style={{ width: 120 }}><CoverageBar pct={g.pct} width={60} /></span>
                <span className="mono" style={{ width: 44, textAlign: 'right' }}>{g.complexity}</span>
                <span className="mono faint" style={{ width: 56, textAlign: 'right' }}>{g.callers}</span>
                <span className="mono faint" style={{ width: 64, textAlign: 'right' }}>{g.direct}/{g.indirect}</span>
                <span style={{ width: 70 }}>
                  <span className="risk-bar" style={{ width: Math.max(4, 60 * (g.risk / maxRisk)) }} />
                </span>
              </div>
            ))}
            {gaps.length === 0 && <div className="faint" style={{ padding: 12 }}>{data.gaps.length ? 'Nothing matches.' : 'Every function is covered. Nice.'}</div>}
          </div>
        </div>
      </div>
      {gap && <GapDetail gap={gap} />}
    </div>
  );
}

/** The selected gap: why it ranks, uncovered lines, reaching tests and fixes. */
function GapDetail({ gap }: { gap: FuncQA }) {
  const s = useStore();
  const tests = useAsync(() => api.qaTestsFor(s.repoId, gap.id), [gap.id, s.repoId, s.qaVersion]);
  const ranges = (gap.uncovered ?? '').split(', ').filter(Boolean);
  const [showAll, setShowAll] = useState(false);
  useEffect(() => setShowAll(false), [gap.id]);
  const sym = { id: gap.id, qualified: gap.name, file: gap.file, line: gap.line, endLine: gap.endLine };
  return (
    <div className="side right qa-detail" style={{ width: 380 }}>
      <div style={{ padding: 14, borderBottom: '1px solid var(--border)' }} className="col">
        <b className="mono" style={{ fontSize: 13.5, wordBreak: 'break-all' }}>{gap.name}</b>
        <div className="faint mono" style={{ fontSize: 11 }}>{gap.file}:{gap.line}-{gap.endLine}</div>
        <div className="row" style={{ gap: 10, marginTop: 6 }}>
          <CoverageBar pct={gap.pct} width={110} />
          <span className="faint">{gap.lines > 0 ? `${gap.covered}/${gap.lines} lines` : 'not instrumented'}</span>
        </div>
        {gap.reason && <div className="muted" style={{ marginTop: 6 }}>{gap.reason}</div>}
        <div className="row" style={{ gap: 6, marginTop: 10, flexWrap: 'wrap' }}>
          <button className="btn sm" onClick={() => actions.code(gap.file, gap.line, gap.endLine)}><Code2 size={13} /> Open</button>
          <button className="btn sm" onClick={() => actions.flow(sym)}><Workflow size={13} /> Logic</button>
          <button className="btn sm" onClick={() => actions.trace(sym)}><Route size={13} /> Callers</button>
        </div>
      </div>
      <div className="scroll" style={{ padding: '4px 14px 16px' }}>
        {ranges.length > 0 && (
          <>
            <div className="section-title" style={{ paddingLeft: 0 }}>Uncovered lines</div>
            <div className="row" style={{ flexWrap: 'wrap', gap: 6 }}>
              {(showAll ? ranges : ranges.slice(0, 12)).map((r) => {
                const [a, b] = r.split('-').map(Number);
                return (
                  <span key={r} className="sym-chip mono uncovered" onClick={() => actions.code(gap.file, a, b || a)}>L{r}</span>
                );
              })}
              {!showAll && ranges.length > 12 && <button className="btn sm ghost" onClick={() => setShowAll(true)}>+{ranges.length - 12}</button>}
            </div>
          </>
        )}

        {(gap.hints?.length ?? 0) > 0 && (
          <>
            <div className="section-title" style={{ paddingLeft: 0 }}><Wrench size={12} /> What makes it hard to test</div>
            {gap.hints!.map((h, i) => (
              <div key={i} className="hint" onClick={() => h.line && actions.code(gap.file, h.line)}>
                <div className="row" style={{ gap: 6 }}>
                  <span className="badge warn">{hintLabels[h.kind] ?? h.kind}</span>
                  {h.match && <span className="mono faint ellipsis" style={{ fontSize: 11 }}>L{h.line} {h.match}</span>}
                </div>
                <div style={{ marginTop: 4 }}>It {h.problem}.</div>
                <div className="row muted" style={{ marginTop: 4, gap: 6, alignItems: 'flex-start' }}>
                  <Lightbulb size={13} style={{ flex: 'none', marginTop: 2, color: 'var(--accent)' }} /> <span>{h.suggestion[0].toUpperCase() + h.suggestion.slice(1)}.</span>
                </div>
              </div>
            ))}
          </>
        )}

        <div className="section-title" style={{ paddingLeft: 0 }}>Tests that reach it</div>
        {tests.loading && !tests.data && <Spinner />}
        {tests.data && tests.data.direct.length + tests.data.indirect.length === 0 && <div className="faint">No test calls this function, directly or through its callers.</div>}
        {tests.data?.direct.map((t) => (
          <div key={t.id} className="ref" onClick={() => actions.code(t.file, t.line, t.endLine)} title={`${t.file}:${t.line}`}>
            <span className="badge ok">direct</span>
            <span className="ellipsis">{t.behavior}</span>
          </div>
        ))}
        {tests.data?.indirect.slice(0, 12).map((t) => (
          <div key={t.id} className="ref" onClick={() => actions.code(t.file, t.line, t.endLine)} title={`${t.file}:${t.line}`}>
            <span className="badge">via callers</span>
            <span className="ellipsis">{t.behavior}</span>
          </div>
        ))}
        {(tests.data?.indirect.length ?? 0) > 12 && <div className="faint" style={{ padding: '2px 8px' }}>+{tests.data!.indirect.length - 12} more</div>}

        <div className="section-title" style={{ paddingLeft: 0 }}><Bot size={12} /> Close the gap with your agent</div>
        <div className="snippet-inline">
          <span className="grow">{gapPrompt(gap.name, gap.file)}</span>
          <CopyButton text={gapPrompt(gap.name, gap.file)} />
        </div>
        <div className="faint" style={{ marginTop: 6, fontSize: 11.5 }}>The agent reads coverage and tests through MCP, puts a test plan in the Agent plans tab, then proposes the tests for you to review.</div>
      </div>
    </div>
  );
}
