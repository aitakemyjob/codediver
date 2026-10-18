import { useEffect, useState } from 'react';
import { Play, Plus, RotateCcw, ScrollText, ShieldCheck, Trash2, X } from 'lucide-react';
import { api } from '../../lib/api';
import { useStore } from '../../lib/store';
import { Switch } from '../../components/ui';
import type { TestSuite } from '../../lib/types';

/**
 * Edits the commands that produce coverage. Suites are detected from the
 * project files until the developer saves their own.
 */
export function SuitesPanel({ suites, detected, onClose, onShowLog }: { suites: TestSuite[]; detected: boolean; onClose: () => void; onShowLog: () => void }) {
  const s = useStore();
  const [draft, setDraft] = useState<TestSuite[]>(suites);
  const [busy, setBusy] = useState(false);
  useEffect(() => setDraft(suites), [suites]);
  const dirty = JSON.stringify(draft) !== JSON.stringify(suites);
  const running = s.qaRun?.state === 'running';

  const update = (i: number, patch: Partial<TestSuite>) => setDraft(draft.map((x, j) => (j === i ? { ...x, ...patch } : x)));
  const save = async () => {
    setBusy(true);
    try {
      await api.qaSaveSuites(s.repoId, draft);
      s.toast({ kind: 'ok', title: 'Suites saved' });
      s.bump();
    } catch (e) {
      s.toast({ kind: 'err', title: 'Could not save suites', body: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  };
  const runOne = async (id: string) => {
    if (dirty) await save();
    try {
      const run = await api.qaRun(s.repoId, [id]);
      s.setQARun({ ...run, log: [] });
      onShowLog();
    } catch (e) {
      s.toast({ kind: 'err', title: 'Could not start', body: e instanceof Error ? e.message : String(e) });
    }
  };

  return (
    <div className="side right" style={{ width: 420, display: 'flex', flexDirection: 'column' }}>
      <div className="panel-head">
        <span className="grow">Coverage suites</span>
        <button className="btn ghost icon sm" onClick={onClose} title="Close"><X size={14} /></button>
      </div>
      <div className="scroll" style={{ padding: 12 }}>
        <div className="row muted" style={{ gap: 8, alignItems: 'flex-start', marginBottom: 12, fontSize: 12 }}>
          <ShieldCheck size={15} style={{ flex: 'none', color: 'var(--ok)', marginTop: 1 }} />
          <span>
            Commands run on your machine, in the suite's folder, only when you click Run. Agents can read the results but can't run or edit suites.
            <code>{' {out} '}</code> is a scratch folder outside your repo.
          </span>
        </div>
        {detected && draft.length > 0 && !dirty && <div className="faint" style={{ marginBottom: 10 }}>Detected from your project files. Edit and save to customize.</div>}
        {draft.map((su, i) => (
          <div key={su.id} className="card suite-card">
            <div className="row" style={{ gap: 8 }}>
              <Switch on={su.enabled} onChange={(v) => update(i, { enabled: v })} />
              <input className="input grow" style={{ height: 26, fontWeight: 600 }} value={su.name} onChange={(e) => update(i, { name: e.target.value })} aria-label="Suite name" />
              <button className="btn sm" title="Run this suite" disabled={running || !su.command.trim()} onClick={() => runOne(su.id)}><Play size={12} /></button>
              <button className="btn ghost icon sm" title="Remove" onClick={() => setDraft(draft.filter((_, j) => j !== i))}><Trash2 size={13} /></button>
            </div>
            <label className="suite-field">
              <span>Folder</span>
              <input className="input mono" placeholder="repository root, or a subfolder such as frontend" title="A folder inside the repository: relative (frontend) or absolute" value={su.dir} onChange={(e) => update(i, { dir: e.target.value })} />
            </label>
            <label className="suite-field">
              <span>Command</span>
              <textarea className="input mono" rows={2} value={su.command} onChange={(e) => update(i, { command: e.target.value })} spellCheck={false} />
            </label>
            <label className="suite-field">
              <span>Report</span>
              <input className="input mono" placeholder="{out}/lcov.info" value={su.report} onChange={(e) => update(i, { report: e.target.value })} spellCheck={false} />
            </label>
          </div>
        ))}
        <button
          className="btn sm"
          onClick={() => setDraft([...draft, { id: `custom:${Date.now()}`, name: 'Custom', dir: '', command: '', report: '{out}/lcov.info', tool: 'custom', enabled: true }])}
        >
          <Plus size={13} /> Add suite
        </button>
        <div className="faint" style={{ marginTop: 12, fontSize: 11.5 }}>
          Reports can be Go coverprofile, LCOV, Cobertura XML or JaCoCo XML; the format is detected automatically.
        </div>
      </div>
      <div className="row" style={{ padding: 10, gap: 8, borderTop: '1px solid var(--border)' }}>
        {!detected && (
          <button className="btn sm ghost" onClick={() => api.qaResetSuites(s.repoId).then(() => s.bump())} title="Discard your suites and use the detected ones">
            <RotateCcw size={13} /> Reset to detected
          </button>
        )}
        <button className="btn sm ghost" onClick={onShowLog}><ScrollText size={13} /> Last run</button>
        <div className="grow" />
        <button className="btn sm primary" disabled={!dirty || busy} onClick={save}>Save</button>
      </div>
    </div>
  );
}
