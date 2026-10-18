import { Component, lazy, Suspense, type ErrorInfo, type ReactNode } from 'react';
import { AlertTriangle, RotateCcw } from 'lucide-react';
import { useStore } from '../lib/store';
import type { Tab } from '../lib/tabs';
import { Spinner } from '../components/ui';
import { OverviewView } from './OverviewView';

// Heavy views (Monaco, React Flow, Mermaid) load on demand.
const CodeView = lazy(() => import('./CodeView'));
const GraphView = lazy(() => import('./GraphView'));
const FlowView = lazy(() => import('./FlowView'));
const HeatmapView = lazy(() => import('./HeatmapView'));
const DiffView = lazy(() => import('./DiffView'));
const ProposalView = lazy(() => import('./ProposalView'));
const WalkthroughView = lazy(() => import('./WalkthroughView'));
const DiagramView = lazy(() => import('./DiagramView'));
const SettingsView = lazy(() => import('./SettingsView'));
const HistoryView = lazy(() => import('./HistoryView'));
const QAView = lazy(() => import('./qa/QAView'));
const DemoView = lazy(() => import('./demo/DemoView'));
const AnimationView = lazy(() => import('./demo/AnimationView'));

/** Renders the view component for a tab. */
function render(tab: Tab, active: boolean) {
  const p = { tab, active };
  switch (tab.kind) {
    case 'overview':
      return <OverviewView {...p} />;
    case 'code':
      return <CodeView {...p} />;
    case 'graph':
      return <GraphView {...p} />;
    case 'flow':
      return <FlowView {...p} />;
    case 'heatmap':
      return <HeatmapView {...p} />;
    case 'diff':
      return <DiffView {...p} />;
    case 'proposal':
      return <ProposalView {...p} />;
    case 'walkthrough':
      return <WalkthroughView {...p} />;
    case 'diagram':
      return <DiagramView {...p} />;
    case 'settings':
      return <SettingsView {...p} />;
    case 'history':
      return <HistoryView {...p} />;
    case 'qa':
      return <QAView {...p} />;
    case 'demo':
      return <DemoView {...p} />;
    case 'animation':
      return <AnimationView {...p} />;
  }
}

/** Contains a crashing view to its own tab instead of blanking the app. */
export class ViewErrorBoundary extends Component<{ children: ReactNode; title: string }, { error?: Error }> {
  state: { error?: Error } = {};
  /** Captures a render error so the boundary can show it. */
  static getDerivedStateFromError(error: Error) {
    return { error };
  }
  /** Logs the crash with the component stack for debugging. */
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`[CodeDiver] view "${this.props.title}" crashed`, error, info.componentStack);
  }
  /** Shows the view, or the error with a retry button. */
  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="empty">
        <div>
          <div className="hero-icon" style={{ color: 'var(--err)', background: 'var(--err-soft)' }}>
            <AlertTriangle size={28} />
          </div>
          <h2>This view hit an error</h2>
          <div className="mono faint" style={{ marginBottom: 14, whiteSpace: 'pre-wrap' }}>{this.state.error.message}</div>
          <button className="btn" onClick={() => this.setState({ error: undefined })}>
            <RotateCcw size={14} /> Try again
          </button>
        </div>
      </div>
    );
  }
}

export interface ViewProps {
  tab: Tab;
  active: boolean;
}

/**
 * Renders every open tab, keeping inactive ones mounted (hidden) so they
 * preserve their state.
 */
export function ViewHost() {
  const { tabs, active } = useStore();
  if (tabs.length === 0) {
    return (
      <div className="empty">
        <div>
          <h2>Nothing open</h2>
          <div>Press <span className="kbd">Ctrl K</span> to jump to a symbol or file.</div>
        </div>
      </div>
    );
  }
  return (
    <>
      {tabs.map((t) => (
        <div key={t.id} className="view" style={{ display: t.id === active ? 'flex' : 'none' }}>
          <ViewErrorBoundary title={t.title}>
            <Suspense fallback={<Spinner />}>{render(t, t.id === active)}</Suspense>
          </ViewErrorBoundary>
        </div>
      ))}
    </>
  );
}
