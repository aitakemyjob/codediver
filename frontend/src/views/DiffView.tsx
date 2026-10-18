import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { Columns2, Flame, GitCommit, GitCompare, Network, Rows2, Workflow, X } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import { basename, dirname } from '../lib/format';
import type { DiffFile, DiffSpec } from '../lib/types';
import { GraphCanvas } from '../graph/GraphCanvas';
import { ChangeBar, Empty, ErrorBox, Spinner, useAsync } from '../components/ui';
import type { ViewProps } from './ViewHost';

/** Loaded on demand (pulls in Monaco). */
const DiffEditor = lazy(() => import('../components/DiffEditor'));
const letter: Record<string, string> = { modified: 'M', added: 'A', deleted: 'D', renamed: 'R', untracked: 'U' };

/** Changed-file list with status, +/− and the functions each file touches. */
export function FileList({ files, selected, onSelect }: { files: DiffFile[]; selected?: string; onSelect: (f: DiffFile) => void }) {
  return (
    <>
      {files.map((f) => (
        <div key={f.path}>
          <div className={`file-row ${selected === f.path ? 'on' : ''}`} onClick={() => onSelect(f)} title={f.path}>
            <span className={`st st-${f.status}`}>{letter[f.status]}</span>
            <span className="grow ellipsis">
              {basename(f.path)} <span className="faint" style={{ fontSize: 11 }}>{dirname(f.path)}</span>
            </span>
            <span className="mono" style={{ fontSize: 11 }}>
              <span className="add">+{f.additions}</span> <span className="del">−{f.deletions}</span>
            </span>
            <ChangeBar add={f.additions} del={f.deletions} />
          </div>
          {selected === f.path && (f.symbols?.length ?? 0) > 0 && (
            <div className="row" style={{ flexWrap: 'wrap', gap: 4, padding: '2px 12px 8px 34px' }}>
              {f.symbols!.map((cs) => (
                <span key={cs.id} className="sym-chip" title={`${cs.lines} changed lines — click for logic`} onClick={() => actions.flow({ id: cs.id, qualified: cs.name })}>
                  <Workflow size={10} /> {cs.name}
                </span>
              ))}
            </div>
          )}
        </div>
      ))}
    </>
  );
}

/**
 * Review screen: pick revisions, browse changed files side by side, and
 * see the blast radius of the change.
 */
export default function DiffView({ tab }: ViewProps) {
  const s = useStore();
  const [spec, setSpec] = useState<DiffSpec>(
    tab.params.commit
      ? { commit: String(tab.params.commit) }
      : { base: (tab.params.base as string) || undefined, head: (tab.params.head as string) || undefined, staged: !!tab.params.staged },
  );
  const [selected, setSelected] = useState<string | undefined>(tab.params.select as string | undefined);
  const [inline, setInline] = useState(false);
  const [showImpact, setShowImpact] = useState(false);
  const ov = useAsync(() => api.diff(s.repoId, spec, true), [s.repoId, spec, s.refreshKey, s.agentPulse]);
  const refs = useAsync(() => api.refs(s.repoId), [s.repoId]);
  useEffect(() => {
    if (tab.params.select) setSelected(tab.params.select as string);
  }, [tab.params.select]);
  const files = useMemo(() => ov.data?.files ?? [], [ov.data]);
  const current = files.find((f) => f.path === selected) ?? files[0];
  const versions = useAsync(
    () => (current ? api.versions(s.repoId, current.path, current.oldPath, spec) : Promise.resolve(undefined)),
    [current?.path, spec, s.repoId, s.refreshKey],
  );
  const changedIds = useMemo(() => Object.fromEntries(files.flatMap((f) => (f.symbols ?? []).map((x) => [x.id, 1]))), [files]);
  const totalSyms = Object.keys(changedIds).length;
  const firstChanged = current?.hunks?.[0]?.newStart;

  return (
    <>
      <div className="toolbar">
        <GitCompare size={16} style={{ color: 'var(--accent)' }} />
        <span className="title">Review</span>
        {spec.commit ? (
          <span className="pill" title={spec.commit}>
            <GitCommit size={13} /> Commit <span className="mono">{spec.commit.slice(0, 7)}</span>
            <button className="btn ghost icon sm" title="Compare revisions instead" onClick={() => setSpec({})}><X size={12} /></button>
          </span>
        ) : (
          <>
            <select className="input select" style={{ width: 170, height: 28 }} value={spec.staged ? ':staged' : spec.base ?? ''} onChange={(e) => {
              const v = e.target.value;
              setSpec(v === ':staged' ? { staged: true } : { base: v || undefined, head: v ? spec.head : undefined });
            }}>
              <option value="">HEAD</option>
              <option value=":staged">Staged (index)</option>
              {refs.data?.map((r) => <option key={r.kind + r.name} value={r.name}>{r.kind === 'tag' ? '🏷 ' : ''}{r.name}</option>)}
              {spec.base && !refs.data?.some((r) => r.name === spec.base) && <option value={spec.base}>{spec.base.slice(0, 12)}</option>}
            </select>
            <span className="faint">→</span>
            <select className="input select" style={{ width: 170, height: 28 }} disabled={spec.staged} value={spec.head ?? ''} onChange={(e) => setSpec({ ...spec, head: e.target.value || undefined, base: spec.base ?? 'HEAD' })}>
              <option value="">Working tree</option>
              {refs.data?.map((r) => <option key={r.kind + r.name} value={r.name}>{r.name}</option>)}
              {spec.head && !refs.data?.some((r) => r.name === spec.head) && <option value={spec.head}>{spec.head.slice(0, 12)}</option>}
            </select>
          </>
        )}
        {ov.data && (
          <span className="badge">
            {files.length} files · <span className="add">+{ov.data.additions}</span> <span className="del">−{ov.data.deletions}</span> · {totalSyms} functions
          </span>
        )}
        <div className="grow" />
        <button className="btn sm" onClick={() => actions.heatmap('changes')}><Flame size={13} /> Heatmap</button>
        <button className={`btn sm ${showImpact ? 'active' : ''}`} onClick={() => setShowImpact(!showImpact)} disabled={!ov.data?.impact}><Network size={13} /> Blast radius</button>
        <div className="seg">
          <button className={!inline ? 'on' : ''} onClick={() => setInline(false)} title="Side by side"><Columns2 size={13} /></button>
          <button className={inline ? 'on' : ''} onClick={() => setInline(true)} title="Inline"><Rows2 size={13} /></button>
        </div>
      </div>
      {ov.error ? <ErrorBox error={ov.error} /> : null}
      {ov.data && files.length === 0 && (
        <Empty icon={<GitCompare size={28} />} title="No changes">Nothing differs between the selected revisions.</Empty>
      )}
      {files.length > 0 && (
        <div className="split">
          <div className="side" style={{ paddingTop: 6 }}>
            <FileList files={files} selected={current?.path} onSelect={(f) => setSelected(f.path)} />
          </div>
          <div className="main">
            <div style={{ flex: 1, position: 'relative', minHeight: 0 }}>
              {current?.binary ? (
                <Empty icon={<GitCompare size={28} />} title="Binary file">No textual diff available.</Empty>
              ) : versions.data ? (
                <Suspense fallback={<Spinner />}>
                  <DiffEditor original={versions.data.original} modified={versions.data.modified} language={versions.data.language} inline={inline} revealLine={firstChanged} />
                </Suspense>
              ) : (
                <Spinner />
              )}
            </div>
            {showImpact && ov.data?.impact && (
              <div className="bottom-pane">
                <div className="toolbar" style={{ minHeight: 36 }}>
                  <b>Blast radius</b>
                  <span className="faint">changed functions and everything that calls them (3 levels)</span>
                </div>
                <GraphCanvas graph={ov.data.impact} direction="RL" heat={Object.fromEntries(ov.data.impact.nodes.map((n) => [n.id, 1 / ((n.depth ?? 0) + 1)]))} onSelect={(n) => n.symbol && s.inspectSymbol(n.symbol)} onOpen={(n) => n.file && actions.code(n.file, n.line)} compact />
              </div>
            )}
          </div>
        </div>
      )}
      {ov.data?.note && <div className="faint" style={{ padding: '4px 12px' }}>{ov.data.note}</div>}
      {ov.loading && !ov.data && <Spinner label="Diffing…" />}
    </>
  );
}
