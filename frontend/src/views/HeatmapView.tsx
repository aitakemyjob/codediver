import { useMemo, useState } from 'react';
import { ChevronRight, Code2, Flame, Home } from 'lucide-react';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import { heatColor, heatScale } from '../lib/themes';
import { layoutTreemap, maxLeaf, subtree, type Cell } from '../lib/treemap';
import { basename, compact } from '../lib/format';
import { ErrorBox, Spinner, useAsync, useSize } from '../components/ui';
import type { ViewProps } from './ViewHost';

const metricLabels: Record<string, string> = {
  churn: 'Churn', changes: 'Diff', proposals: 'Proposals', complexity: 'Complexity', agent: 'Agent focus', size: 'Size', coverage: 'Uncovered',
};

/**
 * Treemap heatmap with metric switcher, folder drill-down, tooltips, a
 * hottest-files ranking and per-function heat for the selected file.
 */
export default function HeatmapView({ tab }: ViewProps) {
  const s = useStore();
  const [metric, setMetric] = useState(String(tab.params.metric ?? 'churn'));
  const [zoom, setZoom] = useState('');
  const [hover, setHover] = useState<{ cell: Cell; x: number; y: number }>();
  const [selected, setSelected] = useState<string>();
  const [ref, size] = useSize<HTMLDivElement>();
  const hm = useAsync(() => api.heatmap(s.repoId, metric, String(tab.params.proposal ?? '')), [metric, s.repoId, s.refreshKey, metric === 'agent' ? s.agentPulse : 0]);

  const maxDepth = 4;
  const root = hm.data ? subtree(hm.data.tree, zoom) ?? hm.data.tree : undefined;
  const cells = useMemo(
    () => (root && size.width > 0 ? layoutTreemap(root, hm.data!.values, size.width, size.height, maxDepth) : []),
    [root, hm.data, size.width, size.height],
  );
  const max = useMemo(() => maxLeaf(cells), [cells]);
  const ranking = useMemo(
    () =>
      Object.entries(hm.data?.values ?? {})
        .filter(([p, v]) => v > 0 && (!zoom || p.startsWith(zoom + '/')))
        .sort((a, b) => b[1] - a[1])
        .slice(0, 40),
    [hm.data, zoom],
  );
  const selSyms = selected ? hm.data?.symbols?.[selected] ?? [] : [];
  const selMax = Math.max(1, ...selSyms.map((x) => x.value));
  const crumbs = zoom ? zoom.split('/') : [];

  return (
    <>
      <div className="toolbar">
        <Flame size={16} style={{ color: 'var(--heat-3)' }} />
        <span className="title">Heatmap</span>
        <div className="seg">
          {Object.keys(s.app?.heatMetrics ?? metricLabels).map((m) => (
            <button key={m} className={metric === m ? 'on' : ''} onClick={() => setMetric(m)} title={s.app?.heatMetrics[m]}>
              {metricLabels[m] ?? m}
            </button>
          ))}
        </div>
        <div className="grow" />
        <span className="faint">{hm.data?.label}</span>
        <span className="mono faint">0</span>
        <div className="legend-bar" />
        <span className="mono faint">{compact(max)}</span>
      </div>
      <div className="toolbar" style={{ minHeight: 34, paddingTop: 4, paddingBottom: 4 }}>
        <div className="crumbs">
          <span onClick={() => setZoom('')}><Home size={12} /> {s.app?.repos.find((r) => r.id === s.repoId)?.name}</span>
          {crumbs.map((c, i) => (
            <span key={i} onClick={() => setZoom(crumbs.slice(0, i + 1).join('/'))}>
              <ChevronRight size={11} className="faint" /> {c}
            </span>
          ))}
        </div>
        <div className="grow" />
        <span className="faint">click a folder to zoom · double-click a file to open</span>
      </div>
      <div className="split">
        <div className="main" ref={ref} style={{ margin: 8, position: 'relative' }}>
          {hm.loading && !hm.data && <Spinner label="Computing heat…" />}
          <div className="treemap">
            {cells.map((c) => {
              const w = c.x1 - c.x0;
              const h = c.y1 - c.y0;
              const collapsed = c.dir && c.depth === maxDepth;
              const t = heatScale(collapsed ? c.peak : c.value, max);
              const isFile = !c.dir || collapsed;
              return (
                <div
                  key={c.path}
                  className={`tm-cell ${isFile ? 'file' : 'dir'}`}
                  style={{ left: c.x0, top: c.y0, width: w, height: h, background: isFile ? heatColor(s.theme, t) : undefined, outlineColor: selected === c.path ? 'var(--accent)' : undefined, zIndex: c.depth }}
                  onMouseMove={(e) => setHover({ cell: c, x: e.clientX, y: e.clientY })}
                  onMouseLeave={() => setHover(undefined)}
                  onClick={(e) => {
                    e.stopPropagation();
                    if (c.dir) setZoom(c.path);
                    else setSelected(c.path);
                  }}
                  onDoubleClick={(e) => {
                    e.stopPropagation();
                    if (!c.dir) actions.code(c.path);
                  }}
                >
                  {w > 36 && h > 14 && <div className="lbl">{c.name}</div>}
                  {isFile && w > 44 && h > 34 && c.value > 0 && <div className="val">{compact(c.value)}</div>}
                </div>
              );
            })}
          </div>
        </div>
        <div className="side right" style={{ width: 300 }}>
          {selected && (
            <div style={{ padding: 12, borderBottom: '1px solid var(--border)' }} className="col">
              <div className="row">
                <b className="mono ellipsis grow">{basename(selected)}</b>
                <button className="btn sm" onClick={() => actions.code(selected)}><Code2 size={13} /> Open</button>
              </div>
              <div className="faint mono" style={{ fontSize: 11 }}>{selected}</div>
              {hm.data?.details?.[selected] && <div className="muted">{hm.data.details[selected]}</div>}
              {selSyms.length > 0 && <div className="section-title" style={{ padding: '6px 0 0' }}>Functions</div>}
              {selSyms.map((x) => (
                <div key={x.id} className="symheat" onClick={() => actions.code(selected, x.line)}>
                  <span className="mono ellipsis grow">{x.name}</span>
                  <span className="bar" style={{ width: 60 * (x.value / selMax) + 6, background: heatColor(s.theme, x.value / selMax) }} />
                  <span className="mono faint" style={{ width: 30, textAlign: 'right' }}>{compact(x.value)}</span>
                </div>
              ))}
            </div>
          )}
          <div className="section-title">Hottest files</div>
          {ranking.length === 0 && !hm.loading && <div className="faint" style={{ padding: '0 10px' }}>No data for this metric yet.</div>}
          {ranking.map(([p, v]) => (
            <div key={p} className={`symheat ${selected === p ? 'on' : ''}`} onClick={() => setSelected(p)} onDoubleClick={() => actions.code(p)} title={hm.data?.details?.[p]}>
              <span className="bar" style={{ width: 6, height: 18, background: heatColor(s.theme, heatScale(v, hm.data!.max)) }} />
              <span className="ellipsis grow">
                <span>{basename(p)}</span> <span className="faint" style={{ fontSize: 11 }}>{p.slice(0, -basename(p).length - 1)}</span>
              </span>
              <span className="mono faint">{compact(v)}</span>
            </div>
          ))}
        </div>
      </div>
      {hover && (
        <div className="tooltip" style={{ left: Math.min(hover.x + 14, window.innerWidth - 400), top: hover.y + 14 }}>
          <b>{hover.cell.path}</b>
          <div className="muted">
            {metricLabels[metric]}: {compact(hover.cell.value)} · {compact(hover.cell.size)} lines
          </div>
          {hm.data?.details?.[hover.cell.path] && <div className="faint">{hm.data.details[hover.cell.path]}</div>}
        </div>
      )}
      {hm.error ? <ErrorBox error={hm.error} /> : null}
    </>
  );
}
