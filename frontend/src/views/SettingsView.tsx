import { useState } from 'react';
import { Bot, FolderGit2, FolderPlus, Palette, ShieldCheck, Trash2 } from 'lucide-react';
import { api } from '../lib/api';
import { useStore } from '../lib/store';
import { themes } from '../lib/themes';
import type { Repo } from '../lib/types';
import { Switch } from '../components/ui';
import type { ViewProps } from './ViewHost';

/**
 * Settings card for one repository: agent visibility, hidden paths and
 * removal.
 */
function RepoRow({ repo }: { repo: Repo }) {
  const s = useStore();
  const [exclude, setExclude] = useState(repo.exclude.join('\n'));
  const [confirm, setConfirm] = useState(false);
  const save = (patch: Partial<Repo>) => api.updateRepo({ ...repo, ...patch }).then(() => s.refresh());
  return (
    <div className="card col" style={{ gap: 10 }}>
      <div className="row">
        <FolderGit2 size={16} style={{ color: 'var(--accent)' }} />
        <b>{repo.name}</b>
        {repo.id === s.repoId && <span className="badge accent">active</span>}
        <span className="faint mono ellipsis grow" style={{ fontSize: 11.5 }}>{repo.path}</span>
        {repo.id !== s.repoId && <button className="btn sm" onClick={() => s.setRepo(repo.id)}>Open</button>}
        {confirm ? (
          <>
            <button className="btn sm danger" onClick={() => api.removeRepo(repo.id).then(() => s.refresh())}>Remove</button>
            <button className="btn sm" onClick={() => setConfirm(false)}>Cancel</button>
          </>
        ) : (
          <button className="btn sm ghost icon" title="Remove from CodeDiver (files are untouched)" onClick={() => setConfirm(true)}><Trash2 size={13} /></button>
        )}
      </div>
      <div className="form-row">
        <label>Visible to agents</label>
        <div className="row"><Switch on={repo.mcpEnabled} onChange={(v) => save({ mcpEnabled: v })} label="Visible to agents" /> <span className="faint">{repo.mcpEnabled ? 'Agents can explore this repo and propose changes' : 'Hidden from MCP clients'}</span></div>
      </div>
      <div className="form-row" style={{ alignItems: 'flex-start' }}>
        <label>Hidden paths</label>
        <div className="col" style={{ gap: 6 }}>
          <textarea className="input mono" rows={3} placeholder={'one glob per line, e.g.\ninternal/secrets/\n*.sqlite'} value={exclude} onChange={(e) => setExclude(e.target.value)} />
          <div className="row">
            <span className="faint grow">Always hidden: .git, .env*, *.pem, *.key, id_rsa*, *.p12, *.keystore</span>
            <button className="btn sm" onClick={() => save({ exclude: exclude.split('\n').map((x) => x.trim()).filter(Boolean) })}>Save</button>
          </div>
        </div>
      </div>
    </div>
  );
}

/**
 * Settings: themes, editor font, churn window, agent access and
 * repositories.
 */
export default function SettingsView(_: ViewProps) {
  const s = useStore();
  const st = s.app?.config.settings;
  const [port, setPort] = useState(String(st?.mcpPort ?? 47474));
  if (!st) return null;
  return (
    <div className="scroll" style={{ padding: 24, maxWidth: 980 }}>
      <h1 style={{ margin: '0 0 18px', fontSize: 22 }}>Settings</h1>

      <div className="section-title" style={{ paddingLeft: 0 }}><Palette size={13} /> Appearance</div>
      <div className="theme-swatches" style={{ marginBottom: 14 }}>
        {themes.map((t) => (
          <div key={t.id} className={`swatch ${t.id === s.theme.id ? 'on' : ''}`} onClick={() => s.setTheme(t.id)}>
            <div className="preview" style={{ background: t.bg }}>
              {t.heat.map((c) => <i key={c} style={{ background: c, height: 10 }} />)}
              <i style={{ background: t.accent, height: 34 }} />
              <i style={{ background: t.accent2, height: 26 }} />
            </div>
            <div className="name" style={{ background: t.bg1, color: t.text }}>{t.name} <span style={{ color: t.text3, fontSize: 11 }}>{t.mode}</span></div>
          </div>
        ))}
      </div>
      <div className="card col" style={{ marginBottom: 22 }}>
        <div className="form-row">
          <label>Editor font size</label>
          <div className="row">
            <input type="range" min={10} max={20} value={st.fontSize} onChange={(e) => s.saveSettings({ fontSize: Number(e.target.value) })} style={{ width: 200, accentColor: 'var(--accent)' }} />
            <span className="mono">{st.fontSize}px</span>
          </div>
        </div>
        <div className="form-row">
          <label>Churn window</label>
          <div className="row">
            <select className="input select" style={{ width: 160 }} value={st.churnDays} onChange={(e) => s.saveSettings({ churnDays: Number(e.target.value) })}>
              {[30, 90, 180, 365, 730, 3650].map((d) => <option key={d} value={d}>{d >= 365 ? `${d / 365} year${d > 365 ? 's' : ''}` : `${d} days`}</option>)}
            </select>
            <span className="faint">history used by the churn heatmap and hotspots</span>
          </div>
        </div>
      </div>

      <div className="section-title" style={{ paddingLeft: 0 }}><Bot size={13} /> Agent access (MCP)</div>
      <div className="card col" style={{ marginBottom: 22 }}>
        <div className="form-row">
          <label>Allow agents</label>
          <div className="row"><Switch on={st.mcpEnabled} onChange={(v) => s.saveSettings({ mcpEnabled: v })} label="Allow agents" /> <span className="faint">Master switch for every MCP tool</span></div>
        </div>
        <div className="form-row">
          <label>Follow the agent</label>
          <div className="row"><Switch on={st.followAgent} onChange={(v) => s.saveSettings({ followAgent: v })} label="Follow agent" /> <span className="faint">Open whatever the agent shows you automatically</span></div>
        </div>
        <div className="form-row">
          <label>Port</label>
          <div className="row">
            <input className="input mono" style={{ width: 120 }} value={port} onChange={(e) => setPort(e.target.value.replace(/\D/g, ''))} />
            <button className="btn sm" onClick={() => s.saveSettings({ mcpPort: Number(port) || 47474 }).then(() => s.toast({ kind: 'info', title: 'Port saved', body: 'Restart CodeDiver to apply.' }))}>Save</button>
            <button className="btn sm" onClick={() => s.setConnectOpen(true)}>Connection snippets</button>
            <span className="faint">{s.app?.mcp.running ? s.app.mcp.url : s.app?.mcp.error}</span>
          </div>
        </div>
        <div className="row muted" style={{ alignItems: 'flex-start' }}>
          <ShieldCheck size={16} style={{ color: 'var(--ok)', flex: 'none', marginTop: 2 }} />
          The server binds to 127.0.0.1 only and rejects non-local hosts. Agents read through CodeDiver's sandbox and can only change files via proposals you accept.
        </div>
      </div>

      <div className="section-title" style={{ paddingLeft: 0 }}>
        <FolderGit2 size={13} /> Repositories
        <button className="btn sm" style={{ marginLeft: 'auto' }} onClick={() => s.setAddRepoOpen(true)}><FolderPlus size={13} /> Add</button>
      </div>
      <div className="col" style={{ gap: 12, marginBottom: 22 }}>
        {s.app?.config.repos.map((r) => <RepoRow key={r.id + r.exclude.join()} repo={r} />)}
      </div>
      <div className="faint">CodeDiver {s.app?.version} · {s.app?.platform} · git {s.app?.gitFound ? 'found' : 'not found — history features disabled'}</div>
    </div>
  );
}
