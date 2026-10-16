import { Bot, Files, Flame, GitCompare, LayoutDashboard, ListTree, Network, Search, Settings } from 'lucide-react';
import { useStore, type SidebarPanel } from '../lib/store';

const panels: { id: SidebarPanel; icon: typeof Files; label: string }[] = [
  { id: 'explorer', icon: Files, label: 'Explorer' },
  { id: 'outline', icon: ListTree, label: 'Outline' },
  { id: 'search', icon: Search, label: 'Search' },
  { id: 'changes', icon: GitCompare, label: 'Changes' },
  { id: 'agent', icon: Bot, label: 'Agent' },
];

/**
 * Left icon rail: sidebar panels, quick views and settings. The Agent icon
 * shows a badge with pending proposals.
 */
export function ActivityBar() {
  const s = useStore();
  const pending = s.proposals.filter((p) => p.status === 'pending').length;
  const hasRepo = !!s.repoId;
  return (
    <nav className="activitybar">
      {hasRepo &&
        panels.map((p) => (
          <button
            key={p.id}
            className={`ab-btn ${s.sidebarOpen && s.sidebar === p.id ? 'on' : ''}`}
            title={p.label}
            onClick={() => s.setSidebar(p.id)}
          >
            <p.icon size={19} strokeWidth={1.8} />
            {p.id === 'agent' && pending > 0 && <span className="count">{pending}</span>}
          </button>
        ))}
      {hasRepo && (
        <>
          <div style={{ height: 1, width: 24, background: 'var(--border)', margin: '6px 0' }} />
          <button className="ab-btn" title="Overview" onClick={() => s.open({ kind: 'overview', title: 'Overview', params: {} })}>
            <LayoutDashboard size={19} strokeWidth={1.8} />
          </button>
          <button className="ab-btn" title="Heatmap (Ctrl+Shift+H)" onClick={() => s.open({ kind: 'heatmap', title: 'Heatmap', params: {} })}>
            <Flame size={19} strokeWidth={1.8} />
          </button>
          <button className="ab-btn" title="Dependencies (Ctrl+Shift+D)" onClick={() => s.open({ kind: 'graph', title: 'Dependencies', params: { kind: 'deps' } })}>
            <Network size={19} strokeWidth={1.8} />
          </button>
        </>
      )}
      <div className="bottom">
        <button className="ab-btn" title="Settings" onClick={() => s.open({ kind: 'settings', title: 'Settings', params: {} })}>
          <Settings size={19} strokeWidth={1.8} />
        </button>
      </div>
    </nav>
  );
}
