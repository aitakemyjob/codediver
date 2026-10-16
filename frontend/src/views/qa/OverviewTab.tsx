import { useState } from 'react';
import { AlertTriangle, FileUp, FolderTree, Play, Settings2, Target, TestTube, Trash2 } from 'lucide-react';
import { api } from '../../lib/api';
import { actions } from '../../lib/actions';
import { useStore } from '../../lib/store';
import { compact, relTime } from '../../lib/format';
import type { QAOverview } from '../../lib/types';
import { CoverageBar, coverageByDir, coverageTone, pctLabel } from './qa';

/**
 * QA dashboard: headline numbers, where coverage comes from, the least
 * covered directories and the riskiest gaps.
 */
export function QAOverviewTab({ data, onSection, onSuites }: { data: QAOverview; onSection: (s: string) => void; onSuites: () => void }) {
  const s = useStore();
  const cov = data.coverage;
  const fn = data.functions;
  const dirs = coverageByDir(data.files).slice(0, 10);
  const frameworks = Object.entries(data.tests.frameworks).sort((a, b) => b[1] - a[1]);
  return (
    <div className="scroll" style={{ padding: 22 }}>
      <div className="stats card" style={{ marginBottom: 14 }}>
        <div className="stat">
          <b style={{ color: cov ? coverageTone(cov.pct) : undefined }}>{cov ? pctLabel(cov.pct) : '—'}</b>
          <span>line coverage</span>
        </div>
        <div className="stat"><b>{compact(data.tests.tests)}</b><span>tests</span></div>
        <div className="stat"><b>{compact(data.tests.behaviors)}</b><span>behaviors</span></div>
        <div className="stat" title="Functions a test calls directly, or reaches through calls">
          <b>{fn.total ? Math.round((100 * fn.reached) / fn.total) : 0}%</b>
          <span>functions reached</span>
        </div>
        {cov && (
          <div className="stat" title="Functions with no covered lines">
            <b style={{ color: fn.uncovered ? 'var(--err)' : undefined }}>{compact(fn.uncovered)}</b>
            <span>uncovered functions</span>
          </div>
        )}
        <div className="stat"><b>{compact(data.tests.files)}</b><span>test files</span></div>
      </div>

      {!cov ? <NoCoverage data={data} onSuites={onSuites} /> : <CoverageSources data={data} />}

      <div className="cards" style={{ marginTop: 14, gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))' }}>
        <div className="card">
          <h3><FolderTree size={13} /> Least covered directories</h3>
          {!cov && <div className="faint">Appears after a coverage run.</div>}
          {dirs.map((d) => (
            <div key={d.path} className="ref" onClick={() => onSection('coverage')} title={`${d.covered}/${d.lines} lines · ${d.files} files`}>
              <span className="mono ellipsis">{d.path === '.' ? '(root)' : d.path}</span>
              {d.untested > 0 && <span className="badge err" title="untested functions">{d.untested}</span>}
              <CoverageBar pct={d.pct} width={70} />
            </div>
          ))}
        </div>
        <div className="card">
          <h3><Target size={13} /> Riskiest gaps</h3>
          {data.gaps.length === 0 && <div className="faint">No untested functions found.</div>}
          {data.gaps.slice(0, 8).map((g) => (
            <div key={g.id} className="ref" onClick={() => onSection('gaps')} title={g.reason}>
              <span className="mono ellipsis">{g.name}</span>
              <span className="loc">cx {g.complexity}</span>
              <CoverageBar pct={g.pct} width={50} />
            </div>
          ))}
        </div>
        <div className="card">
          <h3><TestTube size={13} /> Tests</h3>
          {frameworks.length === 0 && <div className="faint">No tests found. CodeDiver recognizes Go, Jest/Vitest/Mocha, pytest, JUnit, xUnit/NUnit, Rust, RSpec, XCTest, PHPUnit and GoogleTest.</div>}
          {frameworks.map(([f, n]) => (
            <div key={f} className="ref" onClick={() => onSection('behaviors')}>
              <span className="ellipsis">{f}</span>
              <span className="loc">{n} tests</span>
            </div>
          ))}
          {data.tests.skipped > 0 && <div className="faint" style={{ padding: '4px 8px' }}>{data.tests.skipped} skipped or disabled</div>}
          <div className="faint" style={{ padding: '6px 8px 0' }}>
            {fn.tested} of {fn.total} functions are called directly by a test; {fn.reached - fn.tested} more only through other code.
          </div>
        </div>
      </div>
      {data.plans.length > 0 && (
        <div className="card" style={{ marginTop: 14, cursor: 'pointer' }} onClick={() => onSection('plans')}>
          <h3>Agent test plans</h3>
          {data.plans.slice(0, 3).map((p) => (
            <div key={p.id} className="ref">
              <span className="ellipsis">{p.title}</span>
              <span className="loc">{p.behaviors.length} behaviors · {relTime(p.created)}</span>
            </div>
          ))}
        </div>
      )}
      <div className="faint" style={{ marginTop: 14, fontSize: 11.5 }}>
        Test → code mapping is static (calls in each test, resolved by name and package), so treat "reached" as an estimate; line coverage is exact.
        {s.qaRun && s.qaRun.state !== 'running' && <> Last run {relTime(s.qaRun.finished || s.qaRun.started)}: {s.qaRun.state}.</>}
      </div>
    </div>
  );
}

/** Explains how to get coverage when there is none yet. */
function NoCoverage({ data, onSuites }: { data: QAOverview; onSuites: () => void }) {
  const enabled = data.suites.filter((x) => x.enabled);
  return (
    <div className="card qa-callout">
      <div className="row" style={{ alignItems: 'flex-start', gap: 12 }}>
        <AlertTriangle size={18} style={{ color: 'var(--warn)', flex: 'none', marginTop: 2 }} />
        <div className="grow">
          <b>No line coverage yet</b>
          <div className="muted" style={{ margin: '4px 0 10px' }}>
            Behaviors and test reach above come from reading the tests. For exact covered lines, run your tests with coverage. CodeDiver runs the commands below on your machine, only when you click Run; agents can't start them.
          </div>
          {enabled.map((su) => (
            <div key={su.id} className="mono faint ellipsis" style={{ fontSize: 11.5 }}>
              {su.dir || '.'} $ {su.command}
            </div>
          ))}
          {enabled.length === 0 && <div className="faint">No test projects detected. Add a suite with your own command.</div>}
          <div className="row" style={{ marginTop: 10, gap: 8, flexWrap: 'wrap' }}>
            <button className="btn sm primary" disabled={enabled.length === 0} onClick={() => actions.runCoverage()}>
              <Play size={13} /> Run coverage
            </button>
            <button className="btn sm" onClick={onSuites}>
              <Settings2 size={13} /> Configure suites
            </button>
            <ImportReport />
          </div>
        </div>
      </div>
    </div>
  );
}

/** Lists the reports merged into the current coverage. */
function CoverageSources({ data }: { data: QAOverview }) {
  const s = useStore();
  const cov = data.coverage!;
  return (
    <div className="card">
      <h3>Coverage source</h3>
      {cov.sources.map((src) => (
        <div key={src.path} className="ref" style={{ cursor: 'default' }} title={src.path}>
          <span className="badge">{src.format}</span>
          <span className="ellipsis">{src.suite || src.path.split('/').slice(-2).join('/')}</span>
          <span className="loc">
            {src.files} files{src.skipped ? ` · ${src.skipped} outside repo` : ''} · {relTime(src.modTime)}
          </span>
        </div>
      ))}
      <div className="row" style={{ marginTop: 8, gap: 8, flexWrap: 'wrap' }}>
        {cov.stale > 0 && <span className="badge warn">{cov.stale} file(s) changed since this report: re-run for exact lines</span>}
        <div className="grow" />
        <ImportReport />
        <button className="btn sm ghost" onClick={() => api.qaClear(s.repoId)} title="Forget all coverage data">
          <Trash2 size={13} /> Clear
        </button>
      </div>
    </div>
  );
}

/** Inline form for importing an existing report file. */
function ImportReport() {
  const s = useStore();
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState('');
  const [busy, setBusy] = useState(false);
  if (!open) {
    return (
      <button className="btn sm ghost" onClick={() => setOpen(true)} title="Use a coverage file your CI or test runner already wrote">
        <FileUp size={13} /> Import report…
      </button>
    );
  }
  const submit = async () => {
    setBusy(true);
    try {
      const src = await api.qaImport(s.repoId, path.trim());
      s.toast({ kind: 'ok', title: 'Coverage imported', body: `${src.files} files from ${src.format} report` });
      setOpen(false);
      setPath('');
    } catch (e) {
      s.toast({ kind: 'err', title: 'Import failed', body: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="row" style={{ gap: 6 }} onSubmit={(e) => { e.preventDefault(); submit(); }}>
      <input className="input mono" autoFocus style={{ height: 26, width: 300, fontSize: 12 }} placeholder="coverage/lcov.info or an absolute path" value={path} onChange={(e) => setPath(e.target.value)} />
      <button className="btn sm" type="submit" disabled={!path.trim() || busy}>Import</button>
      <button className="btn sm ghost" type="button" onClick={() => setOpen(false)}>Cancel</button>
    </form>
  );
}
