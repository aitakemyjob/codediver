import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, Search } from 'lucide-react';
import { actions } from '../../lib/actions';
import { basename, dirname } from '../../lib/format';
import type { QAOverview, TestCase } from '../../lib/types';
import { countBehaviors, matchesQuery, testTree, type TestNode } from './qa';

const kindBadge: Partial<Record<TestCase['kind'], { label: string; cls: string }>> = {
  subtest: { label: 'subtest', cls: '' },
  case: { label: 'case', cls: '' },
  benchmark: { label: 'bench', cls: 'violet' },
  fuzz: { label: 'fuzz', cls: 'violet' },
  example: { label: 'example', cls: '' },
  todo: { label: 'todo', cls: 'warn' },
};

/**
 * Every test file as a readable spec: describe blocks, tests, subtests and
 * table cases as plain-language behaviors, each with the code it calls.
 */
export function BehaviorsTab({ data }: { data: QAOverview }) {
  const [q, setQ] = useState('');
  const [framework, setFramework] = useState('');
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const [showTargets, setShowTargets] = useState(true);
  const query = q.trim().toLowerCase();
  const frameworks = Object.keys(data.tests.frameworks).sort();
  const files = useMemo(
    () =>
      data.testFiles
        .filter((f) => !framework || f.framework === framework)
        .map((f) => ({ ...f, tree: testTree(f.tests) }))
        .map((f) => ({ ...f, tree: f.tree.filter((n) => matchesQuery(n, query) || f.path.toLowerCase().includes(query)) }))
        .filter((f) => f.tree.length > 0),
    [data.testFiles, framework, query],
  );
  const allCollapsed = files.length > 0 && files.every((f) => collapsed[f.path]);

  return (
    <>
      <div className="toolbar" style={{ minHeight: 38 }}>
        <div className="row" style={{ gap: 6 }}>
          <Search size={13} className="faint" />
          <input className="input" style={{ height: 26, width: 260 }} placeholder="Filter behaviors…" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        {frameworks.length > 1 && (
          <div className="seg">
            <button className={!framework ? 'on' : ''} onClick={() => setFramework('')}>All</button>
            {frameworks.map((f) => (
              <button key={f} className={framework === f ? 'on' : ''} onClick={() => setFramework(f)}>{f}</button>
            ))}
          </div>
        )}
        <div className="grow" />
        <label className="row faint" style={{ gap: 6, cursor: 'pointer' }}>
          <input type="checkbox" checked={showTargets} onChange={(e) => setShowTargets(e.target.checked)} /> code exercised
        </label>
        <button className="btn sm ghost" onClick={() => setCollapsed(allCollapsed ? {} : Object.fromEntries(files.map((f) => [f.path, true])))}>
          {allCollapsed ? 'Expand all' : 'Collapse all'}
        </button>
      </div>
      <div className="scroll" style={{ padding: '8px 14px 24px' }}>
        {files.length === 0 && <div className="faint" style={{ padding: 12 }}>{data.testFiles.length ? 'Nothing matches.' : 'No tests found in this repository.'}</div>}
        {files.map((f) => {
          const isCollapsed = collapsed[f.path];
          return (
            <div key={f.path} className="spec-file">
              <div className="spec-head" onClick={() => setCollapsed({ ...collapsed, [f.path]: !isCollapsed })}>
                {isCollapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
                <b>{basename(f.path)}</b>
                <span className="faint mono" style={{ fontSize: 11 }}>{dirname(f.path)}</span>
                <span className="badge">{f.framework}</span>
                <div className="grow" />
                <span className="faint">{countBehaviors(f.tree)} behaviors</span>
              </div>
              {!isCollapsed && (
                <div className="spec-body">
                  {f.tree.map((n) => (
                    <Behavior key={n.test.id} node={n} depth={0} query={query} data={data} showTargets={showTargets} />
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </>
  );
}

/** One behavior row and its nested children. */
function Behavior({ node, depth, query, data, showTargets }: { node: TestNode; depth: number; query: string; data: QAOverview; showTargets: boolean }) {
  const t = node.test;
  if (query && !matchesQuery(node, query)) return null;
  const suite = t.kind === 'suite';
  const badge = kindBadge[t.kind];
  const targets = (t.targets ?? [])
    .map((id) => ({ id, ...data.targets[id] }))
    .filter((x, i, all) => x.name && all.findIndex((y) => y.name === x.name) === i);
  return (
    <>
      <div className={`spec-row ${suite ? 'suite' : ''}`} style={{ paddingLeft: 10 + depth * 18 }} onClick={() => actions.code(t.file, t.line, t.endLine)} title={`${t.name} · ${t.file}:${t.line}`}>
        {!suite && <span className={`spec-dot ${t.skipped ? 'skipped' : ''}`} />}
        <span className={suite ? 'spec-suite' : 'spec-text'}>{t.behavior}</span>
        {badge && <span className={`badge ${badge.cls}`}>{badge.label}</span>}
        {t.skipped && <span className="badge warn">skipped</span>}
        {!suite && t.name.toLowerCase() !== t.behavior.toLowerCase() && t.kind === 'test' && <span className="mono faint ellipsis" style={{ fontSize: 11 }}>{t.name}</span>}
        <div className="grow" />
        {showTargets && targets.length > 0 && (
          <span className="spec-targets">
            {targets.slice(0, 4).map((x) => (
              <span key={x.id} className="sym-chip mono" onClick={(e) => { e.stopPropagation(); actions.code(x.file, x.line); }} title={`${x.file}:${x.line}`}>
                {x.name}
              </span>
            ))}
            {targets.length > 4 && <span className="faint">+{targets.length - 4}</span>}
          </span>
        )}
        <span className="loc mono">:{t.line}</span>
      </div>
      {node.children.map((c) => (
        <Behavior key={c.test.id} node={c} depth={depth + 1} query={query} data={data} showTargets={showTargets} />
      ))}
    </>
  );
}
