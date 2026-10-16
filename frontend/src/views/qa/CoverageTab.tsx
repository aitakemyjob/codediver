import { useMemo, useState } from 'react';
import { Code2, Search } from 'lucide-react';
import { actions } from '../../lib/actions';
import { basename, dirname } from '../../lib/format';
import type { QAOverview } from '../../lib/types';
import { CoverageBar, coverageByDir } from './qa';

type Sort = 'pct' | 'uncovered' | 'path';

/**
 * Per-file and per-directory coverage. Clicking a file opens it with
 * covered and uncovered lines marked in the gutter.
 */
export function CoverageTab({ data }: { data: QAOverview }) {
  const [q, setQ] = useState('');
  const [sort, setSort] = useState<Sort>('pct');
  const [group, setGroup] = useState(false);
  const files = useMemo(() => {
    const query = q.toLowerCase();
    const list = data.files.filter((f) => (f.lines > 0 || f.functions > 0) && (!query || f.path.toLowerCase().includes(query)));
    const key = (f: (typeof list)[number]) => (sort === 'pct' ? (f.pct < 0 ? 101 : f.pct) : sort === 'uncovered' ? -(f.lines - f.covered) : 0);
    return list.sort((a, b) => key(a) - key(b) || a.path.localeCompare(b.path));
  }, [data.files, q, sort]);
  const dirs = useMemo(() => coverageByDir(files), [files]);

  return (
    <>
      <div className="toolbar" style={{ minHeight: 38 }}>
        <div className="row input-wrap" style={{ gap: 6 }}>
          <Search size={13} className="faint" />
          <input className="input" style={{ height: 26, width: 240 }} placeholder="Filter files…" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <div className="seg">
          <button className={!group ? 'on' : ''} onClick={() => setGroup(false)}>Files</button>
          <button className={group ? 'on' : ''} onClick={() => setGroup(true)}>Directories</button>
        </div>
        <div className="grow" />
        <span className="faint">Sort</span>
        <div className="seg">
          <button className={sort === 'pct' ? 'on' : ''} onClick={() => setSort('pct')}>Lowest %</button>
          <button className={sort === 'uncovered' ? 'on' : ''} onClick={() => setSort('uncovered')}>Most uncovered</button>
          <button className={sort === 'path' ? 'on' : ''} onClick={() => setSort('path')}>Path</button>
        </div>
      </div>
      <div className="scroll" style={{ padding: '6px 10px' }}>
        {!data.coverage && <div className="faint" style={{ padding: 12 }}>No coverage report yet: percentages appear after a coverage run. Untested counts below come from static test reach.</div>}
        <div className="qa-table">
          <div className="qa-row head">
            <span className="grow">{group ? 'Directory' : 'File'}</span>
            <span style={{ width: 90, textAlign: 'right' }}>Lines</span>
            <span style={{ width: 90, textAlign: 'right' }} title="Functions no test reaches / with zero coverage">Untested fns</span>
            <span style={{ width: 150 }}>Coverage</span>
          </div>
          {group
            ? dirs.map((d) => (
                <div key={d.path} className="qa-row" onClick={() => { setGroup(false); setQ(d.path === '.' ? '' : d.path + '/'); }}>
                  <span className="grow mono ellipsis">{d.path === '.' ? '(root)' : `${d.path}/`}</span>
                  <span className="mono faint" style={{ width: 90, textAlign: 'right' }}>{d.covered}/{d.lines}</span>
                  <span className="mono" style={{ width: 90, textAlign: 'right', color: d.untested ? 'var(--err)' : 'var(--text-3)' }}>{d.untested}</span>
                  <span style={{ width: 150 }}><CoverageBar pct={d.pct} /></span>
                </div>
              ))
            : files.map((f) => (
                <div key={f.path} className="qa-row" onClick={() => actions.code(f.path)} title="Open with coverage markers">
                  <span className="grow ellipsis">
                    <span>{basename(f.path)}</span> <span className="faint" style={{ fontSize: 11 }}>{dirname(f.path)}</span>
                    {f.stale && <span className="badge warn" style={{ marginLeft: 6 }}>changed</span>}
                  </span>
                  <span className="mono faint" style={{ width: 90, textAlign: 'right' }}>{f.lines ? `${f.covered}/${f.lines}` : '—'}</span>
                  <span className="mono" style={{ width: 90, textAlign: 'right', color: f.untested ? 'var(--err)' : 'var(--text-3)' }}>
                    {f.untested}/{f.functions}
                  </span>
                  <span style={{ width: 150 }} className="row">
                    <CoverageBar pct={f.pct} />
                    <Code2 size={12} className="faint row-hover" />
                  </span>
                </div>
              ))}
          {(group ? dirs.length : files.length) === 0 && <div className="faint" style={{ padding: 12 }}>Nothing matches.</div>}
        </div>
      </div>
    </>
  );
}
