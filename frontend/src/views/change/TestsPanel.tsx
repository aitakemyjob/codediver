import { useState } from 'react';
import { Check, CircleAlert, FlaskConical, Gauge, MessageSquare, Play } from 'lucide-react';
import { api } from '../../lib/api';
import { actions } from '../../lib/actions';
import { useStore } from '../../lib/store';
import { basename, dirname, relTime } from '../../lib/format';
import type { ChangeReport } from '../../lib/types';
import { Empty, ErrorBox } from '../../components/ui';
import { RunPanel } from '../qa/RunPanel';
import { hasGaps, testPrompt, uncoveredRanges } from './change';

/**
 * Which tests cover a change: changed lines run or not run by tests, the
 * tests to run (measured by per-test runs, or estimated from the static
 * mapping), and changed functions no test reaches.
 */
export function TestsPanel({ report }: { report: ChangeReport }) {
  const s = useStore();
  const [err, setErr] = useState<unknown>();
  const cc = report.coverage;
  const runnable = cc.tests.filter((t) => t.runnable);
  const running = s.qaRun?.state === 'running';
  const [showRun, setShowRun] = useState(false);

  const run = async () => {
    setErr(undefined);
    try {
      s.setQARun(await api.qaRunTests(s.repoId, runnable.map((t) => t.id)));
      setShowRun(true);
    } catch (e) {
      setErr(e);
    }
  };

  return (
    <div className="scroll change-tests">
      <div className="change-sum">
        <FlaskConical size={15} style={{ color: 'var(--accent)', flex: 'none' }} />
        <span className="grow">
          {!cc.available ? (
            <>No coverage report yet, so changed lines can't be checked. <button className="linkish" onClick={() => actions.qa()}>Run coverage in QA</button>.</>
          ) : cc.lines === 0 ? (
            cc.added > 0 ? <>The change only adds code: {cc.added} new lines no test has run yet.</> : <>No executable lines changed.</>
          ) : (
            <>
              <b>{cc.covered} of {cc.lines}</b> changed lines are covered by tests
              {cc.uncovered > 0 && <>, <b className="del">{cc.uncovered} not run by any test</b></>}
              {cc.unknown > 0 && <>, {cc.unknown} in files without coverage data</>}
              {cc.added > 0 && <> · {cc.added} new lines</>}
            </>
          )}
        </span>
        <span className={`badge ${cc.measured ? 'ok' : ''}`} title={cc.measured ? 'Per-test runs recorded which test runs each line' : 'From the static test-to-code mapping: which tests call or reach the changed functions'}>
          {cc.measured ? 'measured' : 'estimated'}
        </span>
        <button className="btn sm" disabled={!runnable.length || running} onClick={run}
          title={runnable.length ? `Run ${runnable.length} tests one by one with per-test coverage, to measure which test runs which changed line` : 'Per-test coverage runs Go and pytest tests; run the full coverage in QA for others'}>
          <Play size={13} /> Run these tests
        </button>
        {hasGaps(report) && <AskButton text={testPrompt(report)} />}
      </div>
      {cc.stale && <div className="change-note warn">Files changed after the last coverage run, so some lines may be off. Run coverage again for exact results.</div>}
      {cc.approximate && <div className="change-note">Line numbers come from a commit and are matched against current coverage: treat them as approximate.</div>}
      {err ? <ErrorBox error={err} /> : null}
      {showRun && s.qaRun && <RunPanel run={s.qaRun} onClose={() => setShowRun(false)} />}

      {cc.untested.length > 0 && (
        <section>
          <div className="impact-head"><CircleAlert size={13} className="del" /> <b>Changed functions no test reaches</b></div>
          {cc.untested.map((n) => (
            <div key={n.id} className="qa-row" onClick={() => actions.code(n.file, n.line)}>
              <span className="mono grow ellipsis">{n.name}</span>
              <span className="faint">{basename(n.file)}:{n.line}</span>
            </div>
          ))}
        </section>
      )}

      {cc.files.some((f) => f.uncovered > 0) && (
        <section>
          <div className="impact-head"><b>Changed lines no test runs</b></div>
          {cc.files.filter((f) => f.uncovered > 0).map((f) => (
            <div key={f.path} className="qa-row" style={{ cursor: 'default' }}>
              <span className="grow ellipsis">{basename(f.path)} <span className="faint" style={{ fontSize: 11 }}>{dirname(f.path)}</span></span>
              <span className="row" style={{ gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
                {uncoveredRanges(f).map((r) => (
                  <button key={r.from} className="linkish mono" onClick={() => actions.code(f.path, r.from, r.to)} title={f.side === 'original' ? 'Lines of the current code this change replaces' : 'Changed lines'}>
                    {r.from === r.to ? r.from : `${r.from}-${r.to}`}
                  </button>
                ))}
              </span>
            </div>
          ))}
        </section>
      )}

      <section>
        <div className="impact-head">
          <Gauge size={13} className="faint" /> <b>Tests to run</b> <span className="mono">{cc.tests.length + (cc.moreTests ?? 0)}</span>
          <span className="faint">{cc.measured ? 'ranked by how many changed lines each runs' : 'tests that call or reach the changed functions'}</span>
        </div>
        {cc.tests.length === 0 ? (
          <Empty icon={<FlaskConical size={24} />} title="No tests exercise this change">
            {hasGaps(report) ? 'Use Ask agent for tests above to copy a prompt for your agent.' : 'Nothing found that calls the changed code.'}
          </Empty>
        ) : (
          cc.tests.map((t) => (
            <div key={t.id} className="qa-row" onClick={() => actions.code(t.file, t.line)} title={t.behavior}>
              <span className="grow ellipsis">
                <span className="mono">{t.name}</span> <span className="faint" style={{ fontSize: 11 }}>{basename(t.file)}:{t.line}</span>
              </span>
              {t.failed && <span className="badge err">failed last run</span>}
              <span className="faint ellipsis" style={{ maxWidth: 260 }}>
                {t.measured ? (t.lines ? `runs ${t.lines} changed line${t.lines > 1 ? 's' : ''}` : 'runs none of the changed lines') : t.direct ? 'calls the changed code' : t.functions?.length ? `reaches ${t.functions.join(', ')}` : ''}
              </span>
              {t.ran && <span className="faint" style={{ width: 70, textAlign: 'right' }}>{relTime(t.ran)}</span>}
            </div>
          ))
        )}
        {(cc.moreTests ?? 0) > 0 && <div className="faint" style={{ padding: '4px 12px' }}>+{cc.moreTests} more</div>}
      </section>
    </div>
  );
}

/** Copies a prompt asking the agent for the missing tests. */
function AskButton({ text }: { text: string }) {
  const [done, setDone] = useState(false);
  return (
    <button className="btn sm" title={`Copies a prompt for your agent:\n\n${text}`} onClick={() => {
      navigator.clipboard?.writeText(text).catch(() => {});
      setDone(true);
      setTimeout(() => setDone(false), 1500);
    }}>
      {done ? <Check size={13} /> : <MessageSquare size={13} />} {done ? 'Copied: paste it to your agent' : 'Ask agent for tests'}
    </button>
  );
}
