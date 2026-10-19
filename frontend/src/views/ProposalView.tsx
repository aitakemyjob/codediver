import { lazy, Suspense, useMemo, useState } from 'react';
import { Check, Flame, FlaskConical, MessageSquare, Network, Sparkles, X, FileCode2 } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import { basename, dirname, relTime } from '../lib/format';
import type { FileChange } from '../lib/types';
import { ChangeBar, Empty, ErrorBox, Markdown, Spinner, useAsync } from '../components/ui';
import { ImpactPanel } from './change/ImpactPanel';
import { TestsPanel } from './change/TestsPanel';
import { diffMarks } from './change/change';
import type { ViewProps } from './ViewHost';

/** Loaded on demand (pulls in Monaco). */
const DiffEditor = lazy(() => import('../components/DiffEditor'));
const actionLetter = { modify: 'M', create: 'A', delete: 'D' } as const;

/**
 * Review of an agent proposal: summary, per-file diffs with rationale and
 * coverage markers, blast radius, the tests covering the change, and
 * accept/reject (all or per file) with feedback.
 */
export default function ProposalView({ tab }: ViewProps) {
  const s = useStore();
  const id = String(tab.params.id);
  const p = s.proposals.find((x) => x.id === id);
  const [selected, setSelected] = useState<string>();
  const [pane, setPane] = useState<'diff' | 'impact' | 'tests'>(tab.params.pane === 'tests' || tab.params.pane === 'impact' ? tab.params.pane : 'diff');
  const [feedback, setFeedback] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<unknown>();
  const report = useAsync(() => (p ? api.changeReport(s.repoId, { proposal: id }) : Promise.resolve(undefined)), [id, s.repoId, p?.updatedAt, s.qaVersion]);
  const current0 = p?.changes.find((c) => c.path === selected) ?? p?.changes[0];
  const marks = useMemo(() => diffMarks(report.data, current0?.path ?? ''), [report.data, current0?.path]);

  if (!p) return <Empty icon={<Sparkles size={28} />} title="Proposal not found">It may have been dismissed, or CodeDiver was restarted.</Empty>;
  const current: FileChange | undefined = p.changes.find((c) => c.path === selected) ?? p.changes[0];
  const add = p.changes.reduce((n, c) => n + c.additions, 0);
  const del = p.changes.reduce((n, c) => n + c.deletions, 0);
  const pending = p.changes.filter((c) => c.status === 'pending' || c.status === 'conflict');
  const im = p.impact;
  const cov = report.data?.coverage;

  const resolve = async (accept: boolean, paths?: string[]) => {
    setBusy(true);
    setErr(undefined);
    try {
      await api.resolveProposal(s.repoId, id, accept, paths, feedback);
      s.toast({ kind: accept ? 'ok' : 'info', title: accept ? 'Changes applied' : 'Changes rejected', body: paths?.length === 1 ? paths[0] : p.title });
      if (accept) s.bump();
    } catch (e) {
      setErr(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="proposal-head">
        <h2>
          <Sparkles size={18} style={{ color: 'var(--accent-2)' }} /> {p.title}
          <span className={`badge status-${p.status}`} style={{ background: 'var(--bg-3)' }}>{p.status}</span>
        </h2>
        <div className="row faint" style={{ marginBottom: 10, flexWrap: 'wrap' }}>
          proposed by your agent · {relTime(p.createdAt)} · {p.changes.length} files · <span className="add">+{add}</span> <span className="del">−{del}</span>
          {im && im.callers.some(Boolean) && (
            <> · <button className="linkish" onClick={() => setPane('impact')}>blast radius: {im.callers[0]} direct callers, {im.callers[1] + im.callers[2]} further{im.untested ? `, ${im.untested} untested` : ''}</button></>
          )}
          {cov?.available && cov.lines > 0 && (
            <> · <button className={`linkish ${cov.uncovered ? 'del' : ''}`} onClick={() => setPane('tests')}>{cov.covered} of {cov.lines} changed lines tested</button></>
          )}
        </div>
        {p.summary && <Markdown src={p.summary} />}
        {p.feedback && (
          <div className="row muted" style={{ marginTop: 8 }}><MessageSquare size={13} /> Your feedback: {p.feedback}</div>
        )}
        {pending.length > 0 && (
          <div className="row" style={{ marginTop: 12, alignItems: 'flex-end' }}>
            <input className="input" placeholder="Optional feedback for the agent (sent with your decision)…" value={feedback} onChange={(e) => setFeedback(e.target.value)} />
            <button className="btn ok" disabled={busy} onClick={() => resolve(true)}><Check size={14} /> Accept {pending.length === p.changes.length ? 'all' : `${pending.length} remaining`}</button>
            <button className="btn danger" disabled={busy} onClick={() => resolve(false)}><X size={14} /> Reject</button>
          </div>
        )}
        {err ? <ErrorBox error={err} /> : null}
      </div>
      <div className="split">
        <div className="side" style={{ paddingTop: 6 }}>
          {p.changes.map((c) => (
            <div key={c.path} className={`file-row ${current?.path === c.path ? 'on' : ''}`} onClick={() => setSelected(c.path)} title={c.rationale || c.path} style={{ alignItems: 'flex-start' }}>
              <span className={`st st-${c.action}`}>{actionLetter[c.action]}</span>
              <div className="grow" style={{ minWidth: 0 }}>
                <div className="ellipsis">{basename(c.path)} <span className="faint" style={{ fontSize: 11 }}>{dirname(c.path)}</span></div>
                <div className="row" style={{ gap: 6, fontSize: 11 }}>
                  <span className={`status-${c.status}`}>{c.status}</span>
                  <span className="add">+{c.additions}</span>
                  <span className="del">−{c.deletions}</span>
                  <ChangeBar add={c.additions} del={c.deletions} />
                  {c.symbols?.length ? <span className="faint">{c.symbols.length} fn</span> : null}
                </div>
                {c.error && <div className="del" style={{ fontSize: 11 }}>{c.error}</div>}
              </div>
            </div>
          ))}
        </div>
        <div className="main">
          <div className="toolbar" style={{ minHeight: 40 }}>
            <div className="seg">
              <button className={pane === 'diff' ? 'on' : ''} onClick={() => setPane('diff')}><FileCode2 size={13} /> Diff</button>
              <button className={pane === 'impact' ? 'on' : ''} onClick={() => setPane('impact')}><Network size={13} /> Blast radius</button>
              <button className={pane === 'tests' ? 'on' : ''} onClick={() => setPane('tests')}><FlaskConical size={13} /> Tests</button>
            </div>
            <button className="btn sm" onClick={() => actions.heatmap('proposals')}><Flame size={13} /> Heatmap</button>
            <div className="grow" />
            {pane === 'diff' && current && (
              <>
                <span className="mono faint ellipsis" style={{ maxWidth: 320 }}>{current.path}</span>
                {(current.status === 'pending' || current.status === 'conflict') && (
                  <>
                    <button className="btn sm ok" disabled={busy} onClick={() => resolve(true, [current.path])}><Check size={13} /> Accept file</button>
                    <button className="btn sm danger" disabled={busy} onClick={() => resolve(false, [current.path])}><X size={13} /> Reject file</button>
                  </>
                )}
              </>
            )}
          </div>
          {pane === 'diff' && current?.rationale && (
            <div className="agent-banner"><Sparkles size={14} style={{ color: 'var(--accent-2)' }} /> <span>{current.rationale}</span></div>
          )}
          <div style={{ flex: 1, position: 'relative', minHeight: 0, display: 'flex', flexDirection: 'column' }}>
            {pane === 'diff' && current && (
              <Suspense fallback={<Spinner />}>
                <DiffEditor original={current.original} modified={current.action === 'delete' ? '' : current.proposed} language={current.language} revealLine={current.hunks?.[0]?.newStart} marks={marks} />
              </Suspense>
            )}
            {pane !== 'diff' && (report.error ? <ErrorBox error={report.error} /> : !report.data ? <Spinner /> : pane === 'impact' ? <ImpactPanel impact={report.data.impact} /> : <TestsPanel report={report.data} />)}
          </div>
        </div>
      </div>
    </>
  );
}
