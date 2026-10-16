import { useEffect } from 'react';
import { useStore } from './lib/store';
import { TitleBar } from './components/TitleBar';
import { ActivityBar } from './components/ActivityBar';
import { Sidebar } from './components/Sidebar';
import { TabsBar } from './components/TabsBar';
import { ViewHost } from './views/ViewHost';
import { StatusBar } from './components/StatusBar';
import { Inspector } from './components/Inspector';
import { CommandPalette } from './components/CommandPalette';
import { Toasts } from './components/Toasts';
import { ConnectAgent } from './components/ConnectAgent';
import { AddRepo } from './components/AddRepo';
import { Welcome } from './components/Welcome';
import { Spinner } from './components/ui';

/**
 * Root component: boots the store, binds global shortcuts and lays out
 * the title bar, activity bar, sidebar, tabs, inspector and status bar.
 */
export function App() {
  const s = useStore();

  useEffect(() => {
    useStore.getState().init();
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;
      const st = useStore.getState();
      const k = e.key.toLowerCase();
      if (k === 'k' || (k === 'p' && !e.shiftKey)) {
        e.preventDefault();
        st.setPalette(!st.palette);
      } else if (k === 'b') {
        e.preventDefault();
        st.toggleSidebar();
      } else if (k === 'i') {
        e.preventDefault();
        st.toggleInspector();
      } else if (k === 'w' && st.active) {
        e.preventDefault();
        st.close(st.active);
      } else if (e.shiftKey && k === 'h') {
        e.preventDefault();
        st.open({ kind: 'heatmap', title: 'Heatmap', params: {} });
      } else if (e.shiftKey && k === 'g') {
        e.preventDefault();
        st.open({ kind: 'diff', title: 'Changes', params: {} });
      } else if (e.shiftKey && k === 'd') {
        e.preventDefault();
        st.open({ kind: 'graph', title: 'Dependencies', params: { kind: 'deps' } });
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  if (!s.ready) {
    return (
      <div className="app">
        <Spinner label="Starting CodeDiver…" />
      </div>
    );
  }
  if (s.fatal) {
    return (
      <div className="app" style={{ display: 'grid', placeItems: 'center' }}>
        <div className="error-box" style={{ maxWidth: 520 }}>
          <b>CodeDiver could not reach its backend.</b>
          <div className="muted" style={{ marginTop: 6 }}>{s.fatal}</div>
        </div>
      </div>
    );
  }
  const hasRepo = !!s.repoId;
  return (
    <div className="app">
      <TitleBar />
      <div className="workbench">
        <ActivityBar />
        {hasRepo && s.sidebarOpen && <Sidebar />}
        <div className={`center ${s.inspectorOpen ? 'with-inspector' : ''}`}>
          {hasRepo ? (
            <>
              <TabsBar />
              <ViewHost />
            </>
          ) : (
            <Welcome />
          )}
        </div>
        {hasRepo && s.inspectorOpen && <Inspector />}
      </div>
      <StatusBar />
      <Toasts />
      {s.palette && hasRepo && <CommandPalette />}
      {s.connectOpen && <ConnectAgent />}
      {s.addRepoOpen && <AddRepo />}
    </div>
  );
}
