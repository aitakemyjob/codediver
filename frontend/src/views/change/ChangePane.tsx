import { FlaskConical, Network, X } from 'lucide-react';
import type { ChangeReport } from '../../lib/types';
import { ErrorBox, Spinner } from '../../components/ui';
import { ImpactPanel } from './ImpactPanel';
import { TestsPanel } from './TestsPanel';

export type PaneKind = 'impact' | 'tests';

/** Bottom pane of the diff and history views: blast radius or tests. */
export function ChangePane({ kind, report, onClose }: { kind: PaneKind; report: { data?: ChangeReport; error?: unknown }; onClose: () => void }) {
  return (
    <div className="bottom-pane">
      <div className="toolbar" style={{ minHeight: 36 }}>
        {kind === 'impact' ? <Network size={14} className="faint" /> : <FlaskConical size={14} className="faint" />}
        <b>{kind === 'impact' ? 'Blast radius' : 'Tests'}</b>
        <span className="faint">{kind === 'impact' ? 'what calls the changed code, closest first' : 'which tests cover the changed lines'}</span>
        <div className="grow" />
        <button className="btn ghost icon sm" title="Close" onClick={onClose}><X size={13} /></button>
      </div>
      {report.error ? <ErrorBox error={report.error} /> : !report.data ? <Spinner /> : kind === 'impact' ? <ImpactPanel impact={report.data.impact} /> : <TestsPanel report={report.data} />}
    </div>
  );
}
