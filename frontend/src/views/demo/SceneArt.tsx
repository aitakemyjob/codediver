import { actions } from '../../lib/actions';
import type { Demo } from '../../lib/types';
import { boxExit, labelAnchor, pathD, placeLabels, pointAlong, type Frame, type LabelItem, type Pt, type Rect } from './demo';
import { PALETTE } from './icons';
import { flowLabel, LINE, MONO_CH, valueLines, type Scene as Laid } from './scene';

const PAD = 40;
const flowColor: Record<string, string> = { request: 'var(--accent)', response: 'var(--ok)', error: 'var(--err)', event: 'var(--accent-2)', data: 'var(--text-2)' };

/**
 * Draws a scene through a fixed camera: lanes, containers, edges, nodes,
 * values and packets. `still` draws a snapshot for export: packets sit
 * where they end up and nothing fades in.
 */
export function SceneArt({ demo, frame, boxes, routes, groups, step, speed, camera, still = false, size }: {
  demo: Demo; frame: Frame; boxes: Laid['boxes']; routes: Laid['routes']; groups: string[]; step: number; speed: number; camera: Rect; still?: boolean; size?: { w: number; h: number };
}) {
  const all = [...boxes.values()];
  const maxY = Math.max(...all.map((b) => b.y + b.h + b.foot + 10), 120);
  const center = (id: string) => {
    const b = boxes.get(id)!;
    return { cx: b.x + b.w / 2, cy: b.y + b.h / 2, hw: b.w / 2, hh: b.h / 2 };
  };
  // Edges that also exist in the opposite direction are drawn side by side.
  const pairs = new Set(demo.edges.map((e) => `${e.from}>${e.to}`));
  // Nested scenes have routes around the boxes; a packet may also travel a
  // route backwards (a response). Otherwise lines run center to center.
  const routed = (from: string, to: string): Pt[] | undefined => {
    const r = routes.get(`${from}>${to}`);
    if (r) return r;
    const back = routes.get(`${to}>${from}`);
    return back && [...back].reverse();
  };
  const segment = (from: string, to: string): { pts: Pt[]; below?: boolean } | undefined => {
    if (!boxes.has(from) || !boxes.has(to)) return undefined;
    const pts = routed(from, to);
    if (pts) {
      // Of a two-way pair, the lower route takes its label below the line.
      const other = routes.has(`${from}>${to}`) && routes.get(`${to}>${from}`);
      return { pts, below: other ? labelAnchor(pts).y > labelAnchor(other).y : undefined };
    }
    const a = center(from);
    const b = center(to);
    let [x1, y1] = boxExit(a.cx, a.cy, a.hw + 2, a.hh + 2, b.cx, b.cy);
    let [x2, y2] = boxExit(b.cx, b.cy, b.hw + 6, b.hh + 6, a.cx, a.cy);
    if (pairs.has(`${to}>${from}`)) {
      const len = Math.hypot(x2 - x1, y2 - y1) || 1;
      const ox = (-(y2 - y1) / len) * 6;
      const oy = ((x2 - x1) / len) * 6;
      [x1, y1, x2, y2] = [x1 + ox, y1 + oy, x2 + ox, y2 + oy];
      // Each direction's label goes on its own side of the pair.
      return { pts: [{ x: x1, y: y1 }, { x: x2, y: y2 }], below: oy > 0 || (oy === 0 && ox > 0) };
    }
    return { pts: [{ x: x1, y: y1 }, { x: x2, y: y2 }] };
  };
  const flowPairs = new Set(frame.flows.map((f) => `${f.from}>${f.to}`));
  const nested = demo.nodes.some((n) => n.parent);
  const lanes = demo.layout === 'architecture' && groups.length > 1 && !nested
    ? groups.map((g) => {
        const mine = demo.nodes.filter((n) => n.group === g).map((n) => boxes.get(n.id)!);
        const x0 = Math.min(...mine.map((b) => b.x)) - 16;
        const x1 = Math.max(...mine.map((b) => b.x + b.w)) + 16;
        return { g, x0, x1 };
      })
    : [];
  const flowMs = Math.round(1400 / speed);
  // Containers draw outermost first, behind edges and boxes.
  const containers = demo.nodes.filter((n) => boxes.get(n.id)?.container).sort((a, b) => boxes.get(a.id)!.depth - boxes.get(b.id)!.depth);


  // Place every label up front so they stay clear of each other and of the
  // boxes: packet labels first (they matter most this step), then edges.
  const labelItems: LabelItem[] = [];
  frame.flows.forEach((f, i) => {
    const seg = segment(f.from, f.to);
    const l = f.label || f.detail ? flowLabel(f.label, f.detail) : undefined;
    if (seg && l) labelItems.push({ id: `f${i}`, pts: seg.pts, w: l.w, h: l.h, below: seg.below });
  });
  demo.edges.forEach((e, i) => {
    if (!(e.label || e.detail) || !frame.visible.has(e.from) || !frame.visible.has(e.to)) return;
    const seg = segment(e.from, e.to);
    const l = flowLabel(e.label, e.detail);
    if (seg) labelItems.push({ id: `e${i}`, pts: seg.pts, w: l.w, h: l.h, below: seg.below });
  });
  const obstacles = demo.nodes
    .filter((n) => frame.visible.has(n.id) && !boxes.get(n.id)?.container)
    .map((n) => boxes.get(n.id)!)
    .map((b) => ({ x: b.x, y: b.y, w: b.w, h: b.h + b.foot }));
  const spots = placeLabels(labelItems, obstacles);

  return (
  <svg xmlns="http://www.w3.org/2000/svg" className={`demo-svg ${still ? 'still' : ''}`} viewBox={`${camera.x} ${camera.y} ${camera.w} ${camera.h}`} preserveAspectRatio="xMidYMid meet" width={size?.w} height={size?.h}>
    <defs>
      <marker id="demo-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
        <path d="M0,0 L10,5 L0,10 z" fill="var(--border-strong)" />
      </marker>
      <marker id="demo-arrow-on" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
        <path d="M0,0 L10,5 L0,10 z" fill="var(--accent)" />
      </marker>
      {Object.entries(PALETTE).map(([name, c]) => (
        <marker key={name} id={`demo-arrow-${name}`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill={c} />
        </marker>
      ))}
    </defs>

    {lanes.map((l) => (
      <g key={l.g} className="demo-lane">
        <rect x={l.x0} y={-PAD / 2 + 6} width={l.x1 - l.x0} height={maxY + PAD - 12} rx={12} />
        <text x={l.x0 + 12} y={-PAD / 2 + 26}>{l.g}</text>
      </g>
    ))}

    {containers.map((n) => {
      const b = boxes.get(n.id)!;
      const state = frame.focus.has(n.id) ? 'focus' : frame.visited.has(n.id) ? 'visited' : '';
      const Icon = b.icon;
      const tx = Icon ? 40 : 14;
      return (
        <g key={n.id} className={`demo-box kind-${n.kind ?? 'group'} depth-${Math.min(b.depth, 3)} ${state} ${frame.visible.has(n.id) ? '' : 'hidden'} ${n.file ? 'link' : ''}`} transform={`translate(${b.x},${b.y})`} onClick={() => n.file && actions.code(n.file, n.line)}>
          <title>{n.detail ? `${n.label}: ${n.detail}` : n.label}</title>
          <rect width={b.w} height={b.h} rx={12} />
          {Icon && <Icon className="icon" x={14} y={13} width={18} height={18} />}
          {b.label.map((l, i) => <text key={`l${i}`} className="title" x={tx} y={27 + i * 17}>{l}</text>)}
          {b.detail.map((l, i) => <text key={`d${i}`} className="subtitle" x={tx} y={25 + b.label.length * 17 + i * LINE}>{l}</text>)}
          {frame.values[n.id] && (() => {
            // A container's value is a badge on its top border, clear of the title.
            const v = frame.values[n.id];
            const text = valueLines(v.text, b.w - 24)[0];
            const w = text.length * MONO_CH + 16;
            return (
              <g className={`demo-value ${v.fresh ? 'fresh' : ''}`} transform={`translate(${b.w - w - 12},-10)`}>
                {/* Solid backing so the container's border doesn't show through the tint. */}
                <rect className="backing" width={w} height={20} rx={6} />
                <rect width={w} height={20} rx={6} />
                <text x={8} y={14}>{text}</text>
              </g>
            );
          })()}
        </g>
      );
    })}

    {demo.edges.map((e, i) => {
      const seg = segment(e.from, e.to);
      if (!seg) return null;
      const on = flowPairs.has(`${e.from}>${e.to}`) || flowPairs.has(`${e.to}>${e.from}`);
      const shown = frame.visible.has(e.from) && frame.visible.has(e.to);
      const color = e.color ? PALETTE[e.color] : undefined;
      // A packet's own label takes the edge label's place while it runs.
      const lbl = e.label || e.detail ? flowLabel(e.label, e.detail) : undefined;
      const covered = frame.flows.some((f) => (f.label || f.detail) && ((f.from === e.from && f.to === e.to) || (f.from === e.to && f.to === e.from)));
      const at = spots.get(`e${i}`);
      return (
        <g key={i} className={`demo-edge ${on ? 'on' : ''} ${color ? 'colored' : ''} ${shown ? '' : 'hidden'} ${e.kind === 'async' || e.kind === 'event' ? 'dashed' : ''}`} style={color ? { color } : undefined}>
          <path d={pathD(seg.pts)} markerEnd={`url(#${on ? 'demo-arrow-on' : e.color ? `demo-arrow-${e.color}` : 'demo-arrow'})`} />
          {lbl && !covered && at && (
            <g className="edge-label" transform={`translate(${at.x + lbl.w / 2},${at.y + 12})`}>
              {lbl.lines.map((l, k) => <text key={k} className="label" y={k * LINE}>{l}</text>)}
              {lbl.details.map((l, k) => <text key={`d${k}`} className="detail" y={lbl.lines.length * LINE + k * 13}>{l}</text>)}
            </g>
          )}
        </g>
      );
    })}

    {demo.nodes.map((n) => {
      const b = boxes.get(n.id)!;
      if (b.container) return null;
      const state = frame.focus.has(n.id) ? 'focus' : frame.visited.has(n.id) ? 'visited' : '';
      const value = frame.values[n.id];
      return (
        <g
          key={n.id}
          className={`demo-node kind-${n.kind ?? 'module'} ${state} ${frame.visible.has(n.id) ? '' : 'hidden'} ${n.file ? 'link' : ''}`}
          transform={`translate(${b.x},${b.y})`}
          onClick={() => n.file && actions.code(n.file, n.line)}
        >
          <title>{n.file ? `${n.label}: ${n.file}${n.line ? `:${n.line}` : ''}` : n.label}</title>
          <rect width={b.w} height={b.h} rx={b.tile ? 10 : n.kind === 'actor' ? b.h / 2 : n.kind === 'datastore' ? 14 : 9} />
          {b.tile && b.icon && <b.icon className="icon" x={b.w / 2 - 14} y={12} width={28} height={28} />}
          {b.tile && b.label.map((l, i) => <text key={`l${i}`} className="tile-label" x={b.w / 2} y={60 + i * LINE}>{l}</text>)}
          {!b.tile && (() => {
            const tx = b.icon ? 44 : 12;
            return (
              <>
                {b.icon && <b.icon className="icon" x={13} y={b.h / 2 - 11} width={22} height={22} />}
                {n.kind === 'datastore' && <path className="deco" d={`M8,12 Q${b.w / 2},24 ${b.w - 8},12`} />}
                {n.kind === 'queue' && <path className="deco" d={`M${b.w - 12},6 V${b.h - 6} M${b.w - 18},6 V${b.h - 6}`} />}
                <text className="kind" x={tx} y={16}>{n.kind ?? ''}</text>
                {b.label.map((l, i) => <text key={`l${i}`} className="label" x={tx} y={35 + i * 17}>{l}</text>)}
                {b.detail.map((l, i) => <text key={`d${i}`} className="detail" x={tx} y={33 + b.label.length * 17 + i * LINE}>{l}</text>)}
              </>
            );
          })()}
          {value && (() => {
            const lines = valueLines(value.text, b.w);
            return (
              <g className={`demo-value ${value.fresh ? 'fresh' : ''}`} transform={`translate(0,${b.h + 6})`}>
                <rect width={Math.max(...lines.map((l) => l.length)) * MONO_CH + 16} height={lines.length * LINE + 6} rx={6} />
                {lines.map((l, i) => <text key={i} x={8} y={14 + i * LINE}>{l}</text>)}
              </g>
            );
          })()}
        </g>
      );
    })}

    {frame.flows.map((f, i) => {
      const seg = segment(f.from, f.to);
      if (!seg) return null;
      const d = pathD(seg.pts);
      const color = (f.color && PALETTE[f.color]) || flowColor[f.kind ?? 'request'] || 'var(--accent)';
      const delay = i * 250;
      // The label stays put beside the route (see spots); only the packet moves.
      const lbl = f.label || f.detail ? flowLabel(f.label, f.detail) : undefined;
      const at = spots.get(`f${i}`);
      return (
        <g key={`${step}-${i}`} className="demo-flow">
          <path d={d} className="track" style={{ stroke: color }} />
          {lbl && at && (
            <g className="demo-flow-label" transform={`translate(${at.x},${at.y})`} style={{ animationDelay: `${delay}ms` }}>
              <rect width={lbl.w} height={lbl.h} rx={5} style={{ stroke: color }} />
              {lbl.lines.map((l, k) => <text key={k} x={6} y={14 + k * LINE}>{l}</text>)}
              {lbl.details.map((l, k) => <text key={`d${k}`} className="detail" x={6} y={13 + lbl.lines.length * LINE + k * 13}>{l}</text>)}
            </g>
          )}
          {still ? (
            // A snapshot shows the packet where it ends up.
            <circle r={6} fill={color} cx={pointAlong(seg.pts, 0.94).x} cy={pointAlong(seg.pts, 0.94).y} />
          ) : (
            <circle r={6} fill={color}>
              <animateMotion path={d} dur={`${flowMs}ms`} begin={`${delay}ms`} fill="freeze" calcMode="spline" keyPoints="0;0.94" keyTimes="0;1" keySplines="0.4 0 0.2 1" />
            </circle>
          )}
        </g>
      );
    })}
  </svg>
  );
}
