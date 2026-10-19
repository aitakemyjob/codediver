import { lazy, Suspense, useState } from 'react';
import { FlaskConical, GitCommit, History, Network } from 'lucide-react';
import { api } from '../lib/api';
import { useStore } from '../lib/store';
import { relTime } from '../lib/format';
import { Empty, ErrorBox, Spinner, useAsync } from '../components/ui';
import type { ViewProps } from './ViewHost';
import { ChangePane, type PaneKind } from './change/ChangePane';

/** Loaded on demand (pulls in Monaco). */
const DiffEditor = lazy(() => import('../components/DiffEditor'));

/**
 * Commit history of one file with the diff each commit introduced, and
 * the tests and blast radius of that commit.
 */
export default function HistoryView({ tab }: ViewProps) {
  const s = useStore();
  const path = String(tab.params.path ?? '');
  const log = useAsync(() => api.log(s.repoId, 100, '', path), [path, s.repoId, s.refreshKey]);
  const [sel, setSel] = useState<string>();
  const commit = log.data?.find((c) => c.hash === sel) ?? log.data?.[0];
  const v = useAsync(
    () => (commit ? api.versions(s.repoId, path, '', { commit: commit.hash }) : Promise.resolve(undefined)),
    [commit?.hash, path, s.repoId],
  );
  const [pane, setPane] = useState<PaneKind>();
  const report = useAsync(
    () => (pane && commit ? api.changeReport(s.repoId, { spec: { commit: commit.hash } }) : Promise.resolve(undefined)),
    [pane, commit?.hash, s.repoId, s.qaVersion],
  );
  const toggle = (k: PaneKind) => setPane(pane === k ? undefined : k);
  if (log.error) return <ErrorBox error={log.error} />;
  if (log.data && log.data.length === 0) return <Empty icon={<History size={28} />} title="No history">This file has no commits yet.</Empty>;
  return (
    <>
      <div className="toolbar">
        <History size={16} style={{ color: 'var(--accent)' }} />
        <span className="title mono">{path}</span>
        {log.data && <span className="badge">{log.data.length} commits</span>}
        <div className="grow" />
        <button className={`btn sm ${pane === 'impact' ? 'active' : ''}`} disabled={!commit} onClick={() => toggle('impact')} title="What calls the code this commit changed"><Network size={13} /> Blast radius</button>
        <button className={`btn sm ${pane === 'tests' ? 'active' : ''}`} disabled={!commit} onClick={() => toggle('tests')} title="Which tests guard the lines this commit touched"><FlaskConical size={13} /> Tests</button>
      </div>
      <div className="split">
        <div className="side">
          {log.data?.map((c) => (
            <div key={c.hash} className={`list-item ${commit?.hash === c.hash ? 'on' : ''}`} onClick={() => setSel(c.hash)}>
              <GitCommit size={14} className="faint" style={{ marginTop: 2, flex: 'none' }} />
              <div className="grow" style={{ minWidth: 0 }}>
                <div className="title ellipsis">{c.subject}</div>
                <div className="sub"><span className="mono">{c.short}</span> · {c.author} · {relTime(c.time)}</div>
              </div>
            </div>
          ))}
        </div>
        <div className="main">
          <div style={{ flex: 1, position: 'relative', minHeight: 0 }}>
            {v.error ? (
              <ErrorBox error={v.error} />
            ) : v.data ? (
              <Suspense fallback={<Spinner />}>
                <DiffEditor original={v.data.original} modified={v.data.modified} language={v.data.language} />
              </Suspense>
            ) : (
              <Spinner />
            )}
          </div>
          {pane && <ChangePane kind={pane} report={report.loading ? {} : report} onClose={() => setPane(undefined)} />}
        </div>
      </div>
    </>
  );
}
