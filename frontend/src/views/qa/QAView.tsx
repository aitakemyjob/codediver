import { useEffect, useState } from 'react';
import { FlaskConical, Play, Settings2, Square } from 'lucide-react';
import { api } from '../../lib/api';
import { actions } from '../../lib/actions';
import { useStore } from '../../lib/store';
import { ErrorBox, Spinner, useAsync } from '../../components/ui';
import type { ViewProps } from '../ViewHost';
import { QAOverviewTab } from './OverviewTab';
import { CoverageTab } from './CoverageTab';
import { BehaviorsTab } from './BehaviorsTab';
import { GapsTab } from './GapsTab';
import { PlansTab } from './PlansTab';
import { SuitesPanel } from './SuitesPanel';
import { RunPanel } from './RunPanel';
import { pctLabel } from './qa';

const tabs = [
  { id: 'overview', label: 'Overview' },
  { id: 'coverage', label: 'Coverage' },
  { id: 'behaviors', label: 'Behaviors' },
  { id: 'gaps', label: 'Gaps' },
  { id: 'plans', label: 'Agent plans' },
] as const;

/**
 * QA view: what the tests cover and what they don't. Shows line coverage
 * (from report files or a coverage run the developer starts), the
 * behaviors tests describe, ranked untested functions with testability
 * hints, and test plans posted by the agent.
 */
export default function QAView({ tab }: ViewProps) {
  const s = useStore();
  const [section, setSection] = useState(String(tab.params.tab ?? 'overview'));
  const [suitesOpen, setSuitesOpen] = useState(false);
  const [logOpen, setLogOpen] = useState(false);
  const ov = useAsync(() => api.qaOverview(s.repoId), [s.repoId, s.refreshKey, s.qaVersion]);
  const running = s.qaRun?.state === 'running';

  // Agent focus (e.g. a new test plan) switches the section.
  useEffect(() => {
    if (tab.params.tab) setSection(String(tab.params.tab));
  }, [tab.params.tab, tab.params.flash]);
  useEffect(() => {
    if (running) setLogOpen(true);
  }, [running]);

  const data = ov.data;
  const cov = data?.coverage;
  return (
    <>
      <div className="toolbar">
        <FlaskConical size={16} style={{ color: 'var(--accent)' }} />
        <span className="title">QA</span>
        <div className="seg">
          {tabs.map((t) => (
            <button key={t.id} className={section === t.id ? 'on' : ''} onClick={() => setSection(t.id)}>
              {t.label}
              {t.id === 'plans' && (data?.plans.length ?? 0) > 0 && <span className="badge violet">{data!.plans.length}</span>}
              {t.id === 'gaps' && (data?.gaps.length ?? 0) > 0 && <span className="badge">{data!.gaps.length >= 150 ? '150+' : data!.gaps.length}</span>}
            </button>
          ))}
        </div>
        <div className="grow" />
        {cov && (
          <span className="faint" title={cov.sources.map((x) => `${x.suite || x.path} (${x.format})`).join('\n')}>
            {pctLabel(cov.pct)} lines covered{cov.stale > 0 && <span className="badge warn" style={{ marginLeft: 6 }}>{cov.stale} changed since</span>}
          </span>
        )}
        <button className={`btn sm ${suitesOpen ? 'active' : ''}`} onClick={() => setSuitesOpen(!suitesOpen)} title="Configure the commands that produce coverage">
          <Settings2 size={13} /> Suites
        </button>
        {running ? (
          <button className="btn sm" onClick={() => api.qaCancel(s.repoId)}>
            <Square size={12} /> Stop
          </button>
        ) : (
          <button className="btn sm primary" onClick={() => actions.runCoverage()} disabled={!data || data.suites.filter((x) => x.enabled).length === 0} title="Run every enabled suite with coverage">
            <Play size={13} /> Run coverage
          </button>
        )}
      </div>
      {(logOpen || running) && s.qaRun && <RunPanel run={s.qaRun} onClose={() => setLogOpen(false)} />}
      <div className="split">
        <div className="main" style={{ minWidth: 0, display: 'flex', flexDirection: 'column' }}>
          {ov.loading && !data && <Spinner label="Discovering tests…" />}
          {ov.error ? <ErrorBox error={ov.error} /> : null}
          {data && section === 'overview' && <QAOverviewTab data={data} onSection={setSection} onSuites={() => setSuitesOpen(true)} />}
          {data && section === 'coverage' && <CoverageTab data={data} />}
          {data && section === 'behaviors' && <BehaviorsTab data={data} />}
          {data && section === 'gaps' && <GapsTab data={data} />}
          {data && section === 'plans' && <PlansTab data={data} focus={String(tab.params.plan ?? '')} />}
        </div>
        {suitesOpen && data && <SuitesPanel suites={data.suites} detected={data.detected} onClose={() => setSuitesOpen(false)} onShowLog={() => setLogOpen(true)} />}
      </div>
    </>
  );
}
