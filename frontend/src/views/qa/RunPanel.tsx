import { useEffect, useRef } from 'react';
import { CheckCircle2, CircleDashed, Loader2, Square, X, XCircle } from 'lucide-react';
import { api } from '../../lib/api';
import { useStore } from '../../lib/store';
import type { QARun, SuiteResult } from '../../lib/types';

/** Icon for a suite's state. */
function StateIcon({ state }: { state: SuiteResult['state'] }) {
  switch (state) {
    case 'running':
      return <Loader2 size={13} className="spin" style={{ color: 'var(--accent)' }} />;
    case 'passed':
      return <CheckCircle2 size={13} style={{ color: 'var(--ok)' }} />;
    case 'failed':
    case 'error':
      return <XCircle size={13} style={{ color: 'var(--err)' }} />;
    case 'cancelled':
      return <Square size={11} className="faint" />;
    default:
      return <CircleDashed size={13} className="faint" />;
  }
}

/** Live status and output of the current (or last) coverage run. */
export function RunPanel({ run, onClose }: { run: QARun; onClose: () => void }) {
  const s = useStore();
  const logRef = useRef<HTMLPreElement>(null);
  const log = run.log ?? [];

  // Fetch the full log when showing a finished run we only have a summary of.
  useEffect(() => {
    if (run.state !== 'running' && log.length === 0) {
      api.qaLastRun(s.repoId).then((r) => r && r.id === run.id && s.setQARun(r));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [run.id, run.state]);
  useEffect(() => {
    const el = logRef.current;
    if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 80) el.scrollTop = el.scrollHeight;
  }, [log.length]);

  return (
    <div className="run-panel">
      <div className="row" style={{ gap: 12, padding: '6px 12px', flexWrap: 'wrap' }}>
        <b style={{ fontSize: 12 }}>Coverage run</b>
        {run.suites.map((x) => (
          <span key={x.id} className="row" style={{ gap: 5 }} title={x.error || (x.failures?.length ? `failing: ${x.failures.join(', ')}` : x.state)}>
            <StateIcon state={x.state} />
            <span>{x.name}</span>
            {x.state === 'failed' && x.failures && x.failures.length > 0 && <span className="badge err">{x.failures.length} failing</span>}
            {x.durationMs > 0 && <span className="faint mono">{(x.durationMs / 1000).toFixed(1)}s</span>}
          </span>
        ))}
        <div className="grow" />
        {run.state !== 'running' && (
          <button className="btn ghost icon sm" onClick={onClose} title="Hide"><X size={13} /></button>
        )}
      </div>
      {run.suites.some((x) => x.error) && run.state !== 'running' && (
        <div className="run-errors">
          {run.suites.filter((x) => x.error).map((x) => (
            <div key={x.id}><b>{x.name}:</b> {x.error}</div>
          ))}
        </div>
      )}
      <pre ref={logRef} className="run-log">{log.length ? log.join('\n') : run.state === 'running' ? 'Starting…' : 'No output.'}</pre>
    </div>
  );
}
