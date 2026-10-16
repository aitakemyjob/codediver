import { useState } from 'react';
import { FolderOpen, FolderPlus } from 'lucide-react';
import { api } from '../lib/api';
import { useStore } from '../lib/store';
import { Modal } from './ui';

/**
 * Dialog for registering a repository folder (with a native picker in the
 * desktop app).
 */
export function AddRepo() {
  const s = useStore();
  const [path, setPath] = useState('');
  const [name, setName] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const close = () => s.setAddRepoOpen(false);
  const browse = async () => {
    try {
      const r = await api.pickDirectory();
      if (r.path) setPath(r.path);
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  const submit = async () => {
    if (!path.trim()) return;
    setBusy(true);
    setErr('');
    try {
      const repo = await api.addRepo(path.trim(), name.trim());
      await api.setActiveRepo(repo.id);
      await s.refresh();
      s.toast({ kind: 'ok', title: `Added ${repo.name}`, body: 'Indexing in the background…' });
      close();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      title="Add a repository"
      icon={<FolderPlus size={16} style={{ color: 'var(--accent)' }} />}
      onClose={close}
      footer={
        <>
          <button className="btn" onClick={close}>Cancel</button>
          <button className="btn primary" onClick={submit} disabled={!path.trim() || busy}>
            {busy ? 'Adding…' : 'Add repository'}
          </button>
        </>
      }
    >
      <div className="muted">CodeDiver indexes the folder locally. Agents connected over MCP can only see repositories you add.</div>
      <div className="col" style={{ gap: 6 }}>
        <label className="muted">Folder</label>
        <div className="row">
          <input className="input mono" autoFocus placeholder="/path/to/your/repo" value={path} onChange={(e) => setPath(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && submit()} />
          {s.app?.canPickDir && (
            <button className="btn" onClick={browse}>
              <FolderOpen size={14} /> Browse…
            </button>
          )}
        </div>
      </div>
      <div className="col" style={{ gap: 6 }}>
        <label className="muted">Display name (optional)</label>
        <input className="input" placeholder="defaults to the folder name" value={name} onChange={(e) => setName(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && submit()} />
      </div>
      {err && <div className="error-box" style={{ margin: 0 }}>{err}</div>}
    </Modal>
  );
}
