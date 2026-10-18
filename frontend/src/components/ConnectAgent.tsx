import { useState } from 'react';
import { Bot, ShieldCheck } from 'lucide-react';
import { useStore } from '../lib/store';
import { Modal, Snippet } from './ui';

const clients = ['Claude Code', 'Cursor', 'VS Code', 'Claude Desktop', 'Codex', 'Other'] as const;

/**
 * The `claude mcp add` command for CodeDiver. `user` scope registers it once
 * for every project; `project` scope writes a shared `.mcp.json` instead.
 */
export function claudeAddCommand(url: string, scope: 'user' | 'project' = 'user'): string {
  return `claude mcp add --transport http --scope ${scope} codediver ${url}`;
}

/**
 * Dialog with MCP connection snippets for popular agents and the safety
 * model.
 */
export function ConnectAgent() {
  const s = useStore();
  const [client, setClient] = useState<(typeof clients)[number]>('Claude Code');
  const mcp = s.app?.mcp;
  const url = mcp?.url || `http://127.0.0.1:${s.app?.config.settings.mcpPort ?? 47474}/mcp`;
  const snippets: Record<(typeof clients)[number], { note: string; text: string }[]> = {
    'Claude Code': [
      { note: 'Run once in your terminal. User scope makes CodeDiver available in every project, not just the folder you run it in:', text: claudeAddCommand(url) },
      { note: 'To share it with your team instead, use project scope, which writes .mcp.json in the repository:', text: claudeAddCommand(url, 'project') },
    ],
    Cursor: [{ note: 'Add to ~/.cursor/mcp.json:', text: JSON.stringify({ mcpServers: { codediver: { url } } }, null, 2) }],
    'VS Code': [{ note: 'Add to .vscode/mcp.json:', text: JSON.stringify({ servers: { codediver: { type: 'http', url } } }, null, 2) }],
    'Claude Desktop': [
      { note: 'Claude Desktop speaks stdio; CodeDiver bridges it to this window. Add to claude_desktop_config.json:', text: JSON.stringify({ mcpServers: { codediver: { command: 'codediver', args: ['mcp'] } } }, null, 2) },
    ],
    Codex: [{ note: 'Add to ~/.codex/config.toml:', text: `[mcp_servers.codediver]\ncommand = "codediver"\nargs = ["mcp"]` }],
    Other: [
      { note: 'Streamable HTTP endpoint:', text: url },
      { note: 'Or launch over stdio (bridges to the running app):', text: 'codediver mcp' },
    ],
  };
  return (
    <Modal title="Connect an agent" icon={<Bot size={16} style={{ color: 'var(--accent-2)' }} />} onClose={() => s.setConnectOpen(false)} wide>
      <div className="row">
        <span className={`dot ${mcp?.running ? 'ok' : 'err'}`} />
        <div className="grow">
          {mcp?.running ? (
            <>MCP server is live at <span className="mono">{url}</span></>
          ) : (
            <span style={{ color: 'var(--err)' }}>{mcp?.error || 'MCP server is not running.'} Change the port in Settings and restart CodeDiver.</span>
          )}
        </div>
      </div>
      <div className="seg" style={{ alignSelf: 'flex-start', flexWrap: 'wrap' }}>
        {clients.map((c) => (
          <button key={c} className={c === client ? 'on' : ''} onClick={() => setClient(c)}>
            {c}
          </button>
        ))}
      </div>
      {snippets[client].map((sn, i) => (
        <div key={i} className="col" style={{ gap: 6 }}>
          <div className="muted">{sn.note}</div>
          <Snippet text={sn.text} />
        </div>
      ))}
      <div className="card">
        <h3>Try asking your agent</h3>
        <ul style={{ margin: 0, paddingLeft: 18, lineHeight: 1.8 }} className="muted">
          <li>“Using CodeDiver, walk me through how a request flows from main to the database.”</li>
          <li>“Explain the function I have open in CodeDiver — inputs, outputs and logic.”</li>
          <li>“Show me a heatmap of what my current diff touches and who calls it.”</li>
          <li>“What isn't tested in this repo? Show me a test plan for the riskiest gap.”</li>
          <li>“Propose a fix for this bug through CodeDiver so I can review it visually.”</li>
        </ul>
      </div>
      <div className="row muted" style={{ alignItems: 'flex-start' }}>
        <ShieldCheck size={16} style={{ color: 'var(--ok)', flex: 'none', marginTop: 2 }} />
        <span>
          Agents only see repositories you add here (and can be switched off per repo). Secrets like <span className="mono">.env</span> and keys are hidden. Agents can't write files —
          they file proposals that you accept or reject.
        </span>
      </div>
    </Modal>
  );
}
