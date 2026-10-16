import { Activity, Bot, Cpu, GitBranch, Plug } from 'lucide-react';
import { useRepo, useStore } from '../lib/store';
import { relTime } from '../lib/format';

/**
 * Bottom bar: repo path, branch, index status, last agent action and the
 * MCP endpoint.
 */
export function StatusBar() {
  const s = useStore();
  const repo = useRepo();
  const last = s.activity[s.activity.length - 1];
  const mcp = s.app?.mcp;
  return (
    <footer className="statusbar">
      {repo && (
        <span className="item" title={repo.path}>
          <span className="mono">{repo.path}</span>
        </span>
      )}
      {repo?.git.isRepo && (
        <span className="item">
          <GitBranch size={12} />
          {repo.git.branch}
          {repo.git.ahead > 0 && ` ↑${repo.git.ahead}`}
          {repo.git.behind > 0 && ` ↓${repo.git.behind}`}
        </span>
      )}
      <span className="item">
        <Cpu size={12} />
        {s.indexing ? (
          <>indexing…</>
        ) : s.indexStats ? (
          <>
            {s.indexStats.functions} functions · {s.indexStats.edges} edges · {s.indexStats.durationMs}ms
          </>
        ) : (
          'ready'
        )}
      </span>
      <div className="right">
        {last && (
          <span className="item click" onClick={() => s.setSidebar('agent')} title={last.summary}>
            <Bot size={12} style={{ color: 'var(--accent-2)' }} />
            <span className="mono">{last.tool}</span> {relTime(last.time)}
          </span>
        )}
        <span className="item click" onClick={() => s.setConnectOpen(true)} title={mcp?.error ?? 'Agent endpoint'}>
          <Plug size={12} />
          {mcp?.running ? <span className="mono">{mcp.url}</span> : mcp?.error ? <span style={{ color: 'var(--err)' }}>MCP error</span> : 'MCP off'}
        </span>
        <span className="item">
          <Activity size={12} /> {s.app?.version?.startsWith('v') ? '' : 'v'}{s.app?.version}
        </span>
      </div>
    </footer>
  );
}
