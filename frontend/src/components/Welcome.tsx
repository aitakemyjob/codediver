import { Bot, Flame, FolderPlus, GitCompare, Route, Sparkles, Workflow } from 'lucide-react';
import { useStore } from '../lib/store';

const features = [
  { icon: Route, title: 'Trace to here', body: 'Click any function and see every path from entry points to it.' },
  { icon: Workflow, title: 'Function logic', body: 'Control-flow graphs with inputs and outputs, narrated by your agent.' },
  { icon: Flame, title: 'Heatmaps', body: 'Churn, complexity, diffs and agent proposals across directories and files.' },
  { icon: GitCompare, title: 'Visual review', body: 'Diffs mapped to the functions they touch, plus their blast radius.' },
  { icon: Sparkles, title: 'Agent proposals', body: 'Agents propose; you review side-by-side and accept file by file.' },
  { icon: Bot, title: 'Built-in MCP', body: 'Claude Code, Cursor, VS Code and others drive the UI as they explain.' },
];

/** First-run screen shown until a repository is added. */
export function Welcome() {
  const s = useStore();
  return (
    <div className="empty" style={{ overflow: 'auto' }}>
      <div style={{ maxWidth: 860 }}>
        <img src="./logo.png" alt="CodeDiver" width={168} height={168} draggable={false} style={{ display: 'block', margin: '0 auto' }} />
        <h2 style={{ fontSize: 28, marginTop: 18 }}>Dive into any codebase</h2>
        <p className="muted" style={{ fontSize: 14, maxWidth: 520, margin: '0 auto 22px' }}>
          Add a repository to map its structure, trace how code flows, and let your coding agent walk you through it — visually.
        </p>
        <div className="row" style={{ justifyContent: 'center', gap: 10 }}>
          <button className="btn primary" style={{ height: 36, padding: '0 16px' }} onClick={() => s.setAddRepoOpen(true)}>
            <FolderPlus size={16} /> Add a repository
          </button>
          <button className="btn" style={{ height: 36, padding: '0 16px' }} onClick={() => s.setConnectOpen(true)}>
            <Bot size={16} /> Connect an agent
          </button>
        </div>
        <div className="cards" style={{ marginTop: 36, textAlign: 'left' }}>
          {features.map((f) => (
            <div key={f.title} className="card">
              <div className="row" style={{ marginBottom: 6 }}>
                <f.icon size={16} style={{ color: 'var(--accent)' }} />
                <b style={{ color: 'var(--text)' }}>{f.title}</b>
              </div>
              <div className="muted">{f.body}</div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
