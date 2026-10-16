import { Bot, Code2, Flame, FolderTree, GitBranch, Network, Route, Sparkles, Workflow } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import { compact, languageColor, plural } from '../lib/format';
import { ErrorBox, KindIcon, Markdown, Spinner, useAsync } from '../components/ui';
import type { ViewProps } from './ViewHost';

/**
 * Repository dashboard: stats, languages, quick actions, entry points,
 * hotspots, top-level folders and README.
 */
export function OverviewView({ tab }: ViewProps) {
  const s = useStore();
  const ov = useAsync(() => api.overview(s.repoId), [s.repoId, s.refreshKey, tab.id]);
  if (ov.loading && !ov.data) return <Spinner label="Analyzing repository…" />;
  if (ov.error) return <ErrorBox error={ov.error} />;
  const o = ov.data!;
  const langs = Object.entries(o.stats.languages).sort((a, b) => b[1] - a[1]);
  const totalLang = langs.reduce((n, [, v]) => n + v, 0) || 1;
  const maxDirLines = Math.max(1, ...o.topDirs.map((d) => d.lines));
  return (
    <div className="scroll" style={{ padding: 22 }}>
      <div className="row" style={{ marginBottom: 18, alignItems: 'flex-end' }}>
        <div className="grow">
          <div className="faint mono" style={{ fontSize: 11.5 }}>{o.repo.path}</div>
          <h1 style={{ margin: '4px 0 0', fontSize: 26, letterSpacing: '-0.02em' }}>{o.repo.name}</h1>
        </div>
        {o.git.isRepo && (
          <span className="pill">
            <GitBranch size={13} /> {o.git.branch}
            {o.git.dirty > 0 && <span className="badge warn">{o.git.dirty} changed</span>}
          </span>
        )}
      </div>

      <div className="stats card" style={{ marginBottom: 14 }}>
        <div className="stat"><b>{compact(o.stats.files)}</b><span>analyzed files</span></div>
        <div className="stat"><b>{compact(o.stats.functions)}</b><span>functions</span></div>
        <div className="stat"><b>{compact(o.stats.types)}</b><span>types</span></div>
        <div className="stat"><b>{compact(o.stats.packages)}</b><span>packages</span></div>
        <div className="stat"><b>{compact(o.stats.edges)}</b><span>call edges</span></div>
        <div className="stat"><b>{o.stats.durationMs}ms</b><span>index time</span></div>
      </div>

      {langs.length > 0 && (
        <div className="card" style={{ marginBottom: 14 }}>
          <h3>Languages</h3>
          <div style={{ display: 'flex', height: 10, borderRadius: 5, overflow: 'hidden', marginBottom: 10 }}>
            {langs.map(([l, n]) => (
              <div key={l} title={`${l}: ${n}`} style={{ width: `${(n / totalLang) * 100}%`, background: languageColor[l] ?? 'var(--text-3)' }} />
            ))}
          </div>
          <div className="row" style={{ flexWrap: 'wrap', gap: 14 }}>
            {langs.slice(0, 10).map(([l, n]) => (
              <span key={l} className="row" style={{ gap: 6 }}>
                <span className="dot" style={{ background: languageColor[l] ?? 'var(--text-3)' }} />
                {l} <span className="faint">{n}</span>
              </span>
            ))}
          </div>
        </div>
      )}

      <div className="cards" style={{ marginBottom: 14 }}>
        <QuickAction icon={<Network size={18} />} title="Architecture" body="Package dependency graph" onClick={() => actions.deps()} />
        <QuickAction icon={<Flame size={18} />} title="Hotspots" body="Churn heatmap of the tree" onClick={() => actions.heatmap('churn')} />
        <QuickAction icon={<Sparkles size={18} />} title="Review changes" body="Diff mapped to functions" onClick={() => s.open({ kind: 'diff', title: 'Changes', params: {} })} />
        <QuickAction icon={<Bot size={18} />} title="Ask your agent" body="Connect over MCP" onClick={() => s.setConnectOpen(true)} />
      </div>

      <div className="cards">
        <div className="card">
          <h3><Route size={13} /> Entry points</h3>
          {o.entryPoints.length === 0 && <div className="faint">No obvious entry points found.</div>}
          {o.entryPoints.map((e) => (
            <div key={e.id} className="ref" onClick={() => actions.symbol(e)}>
              <KindIcon kind={e.kind} size={18} />
              <span className="mono ellipsis">{e.qualified}</span>
              <span className="loc">{e.file}</span>
              <button className="btn ghost icon sm" title="Calls from here" onClick={(ev) => { ev.stopPropagation(); actions.callGraph(e); }}>
                <Workflow size={13} />
              </button>
            </div>
          ))}
        </div>
        <div className="card">
          <h3><Flame size={13} /> Hotspots <span className="faint" style={{ textTransform: 'none', letterSpacing: 0, fontWeight: 400 }}>churn × complexity</span></h3>
          {o.hotspots.length === 0 && <div className="faint">Needs git history and analyzable code.</div>}
          {o.hotspots.map((h, i) => (
            <div key={h.path} className="ref" onClick={() => actions.code(h.path)}>
              <span className="badge" style={{ background: `color-mix(in srgb, var(--heat-${Math.max(0, 4 - Math.floor(i / 2))}) 35%, transparent)`, color: 'var(--text)' }}>{i + 1}</span>
              <span className="mono ellipsis">{h.path}</span>
              <span className="loc">{h.churn}c · cx {h.complexity}</span>
            </div>
          ))}
        </div>
        <div className="card">
          <h3><FolderTree size={13} /> Top-level</h3>
          {o.topDirs.map((d) => (
            <div key={d.path} className="ref" onClick={() => (d.files > 1 ? actions.heatmap() : actions.code(d.path))}>
              <span className="mono ellipsis" style={{ width: 120 }}>{d.path}</span>
              <div className="grow" style={{ height: 6, background: 'var(--bg-3)', borderRadius: 3, overflow: 'hidden' }}>
                <div style={{ width: `${(d.lines / maxDirLines) * 100}%`, height: '100%', background: 'var(--accent)' }} />
              </div>
              <span className="loc">{plural(d.files, 'file')}</span>
            </div>
          ))}
        </div>
      </div>

      {o.readme && (
        <div className="card" style={{ marginTop: 14 }}>
          <h3><Code2 size={13} /> README</h3>
          <Markdown src={o.readme} />
        </div>
      )}
    </div>
  );
}

/** A large clickable card that opens a view. */
function QuickAction({ icon, title, body, onClick }: { icon: React.ReactNode; title: string; body: string; onClick: () => void }) {
  return (
    <div className="card row" style={{ cursor: 'pointer', gap: 12 }} onClick={onClick}>
      <div className="hero-icon" style={{ width: 38, height: 38, borderRadius: 11, margin: 0 }}>{icon}</div>
      <div>
        <b>{title}</b>
        <div className="faint">{body}</div>
      </div>
    </div>
  );
}
