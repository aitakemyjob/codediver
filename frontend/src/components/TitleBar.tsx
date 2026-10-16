import { Bot, ChevronDown, FolderGit2, GitBranch, Palette, PanelRight, Plus, Search, Settings2 } from 'lucide-react';
import { useRepo, useStore } from '../lib/store';
import { themes } from '../lib/themes';
import { Menu } from './Menu';

/**
 * Top bar: logo, repository switcher, branch, search, follow-agent toggle,
 * MCP status, theme menu and inspector toggle.
 */
export function TitleBar() {
  const s = useStore();
  const repo = useRepo();
  const isMac = s.app?.platform === 'darwin';
  const mod = isMac ? '⌘' : 'Ctrl';
  const mcp = s.app?.mcp;
  const follow = s.app?.config.settings.followAgent ?? true;

  return (
    <header className={`titlebar ${isMac ? 'mac' : ''}`}>
      <div className="brand">
        <img className="brand-mark" src="./logo.png" alt="" draggable={false} />
        CodeDiver
      </div>

      <Menu
        width={320}
        trigger={() => (
          <button className="btn ghost" title="Switch repository">
            <FolderGit2 size={15} className="faint" />
            <span style={{ fontWeight: 600 }}>{repo?.name ?? 'No repository'}</span>
            <ChevronDown size={14} className="faint" />
          </button>
        )}
        items={[
          ...(s.app?.repos ?? []).map((r) => ({
            key: r.id,
            label: r.name,
            hint: r.git.isRepo ? r.git.branch : r.path,
            active: r.id === s.repoId,
            icon: <FolderGit2 size={14} />,
            onSelect: () => s.setRepo(r.id),
          })),
          { key: 'sep', label: '', separator: true },
          { key: 'add', label: 'Add repository…', icon: <Plus size={14} />, onSelect: () => s.setAddRepoOpen(true) },
          { key: 'manage', label: 'Manage repositories', icon: <Settings2 size={14} />, onSelect: () => s.open({ kind: 'settings', title: 'Settings', params: { section: 'repos' } }) },
        ]}
      />

      {repo?.git.isRepo && (
        <span className="pill" title="Current branch" onClick={() => s.open({ kind: 'diff', title: 'Changes', params: {} })}>
          <GitBranch size={13} />
          {repo.git.branch}
          {repo.git.dirty > 0 && <span className="badge warn">{repo.git.dirty}</span>}
        </span>
      )}

      <div className="spacer" />

      {s.repoId && (
        <div className="omnibox" onClick={() => s.setPalette(true)}>
          <Search size={14} />
          <span>Search symbols, files, commands…</span>
          <span className="kbd">{mod} K</span>
        </div>
      )}

      <div className="spacer" />

      <button
        className={`pill ${follow ? '' : 'faint'}`}
        title={follow ? 'Following the agent: views it shows open automatically' : 'Not following the agent'}
        onClick={() => s.saveSettings({ followAgent: !follow })}
      >
        <Bot size={14} style={{ color: follow ? 'var(--accent-2)' : undefined }} />
        {follow ? 'Following agent' : 'Follow agent'}
      </button>

      <span className="pill" onClick={() => s.setConnectOpen(true)} title={mcp?.error || mcp?.url || 'MCP server'}>
        <span className={`dot ${mcp?.running ? 'ok' : mcp?.error ? 'err' : 'idle'}`} />
        MCP {mcp?.running ? 'live' : mcp?.error ? 'error' : 'off'}
      </span>

      <Menu
        align="right"
        width={220}
        trigger={() => (
          <button className="btn ghost icon" title="Theme">
            <Palette size={16} />
          </button>
        )}
        items={themes.map((t) => ({
          key: t.id,
          label: t.name,
          active: t.id === s.theme.id,
          icon: <span style={{ width: 14, height: 14, borderRadius: 4, background: t.accent, display: 'block' }} />,
          hint: t.mode,
          onSelect: () => s.setTheme(t.id),
        }))}
      />
      <button className={`btn ghost icon ${s.inspectorOpen ? 'active' : ''}`} title={`Inspector (${mod}+I)`} onClick={() => s.toggleInspector()}>
        <PanelRight size={16} />
      </button>
    </header>
  );
}
