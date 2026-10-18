// Pure helpers behind the Demo view: what the scene looks like at a given
// step. Kept free of React so they can be unit-tested (demo.test.ts).
import type { Demo, DemoFlow } from '../../lib/types';

export interface Frame {
  /** Nodes on screen at this step. */
  visible: Set<string>;
  /** Nodes highlighted in this step. */
  focus: Set<string>;
  /** Nodes highlighted in an earlier step (the path so far). */
  visited: Set<string>;
  /** Value per node: the latest one set at or before this step. */
  values: Record<string, { text: string; fresh: boolean }>;
  /** Packets to animate in this step. */
  flows: DemoFlow[];
}

/**
 * The scene at step `index`: nodes listed in some step's `reveal` appear
 * from that step on (others are visible from the start), but never before
 * the container they sit in; focus marks the
 * current step's nodes, earlier focus becomes "visited", and values
 * persist until overwritten, like state.
 */
export function frameAt(demo: Demo, index: number): Frame {
  const revealedAt = new Map<string, number>();
  demo.steps.forEach((st, i) => st.reveal?.forEach((id) => !revealedAt.has(id) && revealedAt.set(id, i)));
  const parentOf = new Map(demo.nodes.map((n) => [n.id, n.parent]));
  const shownAt = (id: string): number => {
    let at = revealedAt.get(id) ?? -1;
    for (let p = parentOf.get(id), guard = 0; p && guard < 16; p = parentOf.get(p), guard++) at = Math.max(at, revealedAt.get(p) ?? -1);
    return at;
  };
  const visible = new Set(demo.nodes.filter((n) => shownAt(n.id) <= index).map((n) => n.id));
  const showWithContainers = (id: string) => {
    for (let p: string | undefined = id, guard = 0; p && guard < 16; p = parentOf.get(p), guard++) visible.add(p);
  };
  const step = demo.steps[index];
  const focus = new Set(step?.focus ?? []);
  const visited = new Set<string>();
  const values: Frame['values'] = {};
  for (let i = 0; i <= index && i < demo.steps.length; i++) {
    const st = demo.steps[i];
    if (i < index) st.focus?.forEach((id) => visited.add(id));
    for (const [id, text] of Object.entries(st.values ?? {})) values[id] = { text, fresh: i === index };
    // Anything a flow touches must be on screen while it animates.
    if (i === index) st.flows?.forEach((f) => (showWithContainers(f.from), showWithContainers(f.to)));
  }
  focus.forEach((id) => visited.delete(id));
  return { visible, focus, visited, values, flows: step?.flows ?? [] };
}

/** Distinct groups in first-appearance order (lanes, left to right). */
export function groupsOf(demo: Demo): string[] {
  const out: string[] = [];
  for (const n of demo.nodes) if (n.group && !out.includes(n.group)) out.push(n.group);
  return out;
}

/** How long a step stays on screen when playing, in ms at 1× speed. */
export function stepDuration(demo: Demo, index: number): number {
  const st = demo.steps[index];
  if (!st) return 0;
  const words = `${st.title} ${st.narration ?? ''}`.split(/\s+/).filter(Boolean).length;
  const flowTime = (st.flows?.length ?? 0) > 0 ? 1600 : 0;
  return Math.min(12000, Math.max(2600, flowTime + words * 260));
}

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/**
 * The camera for a frame: the bounding box of the visible boxes, padded,
 * with a minimum size so a single node isn't blown up to fill the stage,
 * and grown to the stage's aspect ratio so it fills the view.
 */
export function cameraFor(boxes: Rect[], aspect: number, pad = 60, minW = 640): Rect {
  if (boxes.length === 0) return { x: 0, y: 0, w: minW, h: minW / aspect };
  let x0 = Math.min(...boxes.map((b) => b.x)) - pad;
  let y0 = Math.min(...boxes.map((b) => b.y)) - pad;
  let x1 = Math.max(...boxes.map((b) => b.x + b.w)) + pad;
  let y1 = Math.max(...boxes.map((b) => b.y + b.h)) + pad;
  const grow = (lo: number, hi: number, size: number): [number, number] => {
    const extra = (size - (hi - lo)) / 2;
    return extra > 0 ? [lo - extra, hi + extra] : [lo, hi];
  };
  [x0, x1] = grow(x0, x1, minW);
  // Match the stage's aspect ratio by growing the shorter side.
  const w = x1 - x0;
  const h = y1 - y0;
  if (w / h < aspect) [x0, x1] = grow(x0, x1, h * aspect);
  else [y0, y1] = grow(y0, y1, w / aspect);
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}

/**
 * Zooms a camera by `factor` (< 1 zooms in) around a point given as a
 * fraction of the view (fx, fy in 0..1), so that point stays under the
 * cursor. The zoom is clamped between minW and maxW (camera widths).
 */
export function zoomAt(cam: Rect, factor: number, fx: number, fy: number, minW = 120, maxW = 20000): Rect {
  const w = Math.min(maxW, Math.max(minW, cam.w * factor));
  const k = w / cam.w;
  const h = cam.h * k;
  const px = cam.x + fx * cam.w;
  const py = cam.y + fy * cam.h;
  return { x: px - fx * w, y: py - fy * h, w, h };
}

/** Moves a camera by a drag of (dx, dy) screen pixels on a view of the given size. */
export function panBy(cam: Rect, dx: number, dy: number, viewW: number, viewH: number): Rect {
  return { ...cam, x: cam.x - (dx * cam.w) / viewW, y: cam.y - (dy * cam.h) / viewH };
}

/**
 * The nodes the camera should follow at a frame: the step's focus and
 * both ends of every packet in flight; when a step has neither, every
 * visible node.
 */
export function followTargets(frame: Frame): string[] {
  const ids = new Set(frame.focus);
  for (const f of frame.flows) {
    ids.add(f.from);
    ids.add(f.to);
  }
  return ids.size > 0 ? [...ids] : [...frame.visible];
}

/**
 * Splits text into lines of at most `max` characters, breaking at spaces
 * where it can and splitting longer words. Keeps at most `limit` lines and
 * ends the last one with … when text was cut.
 */
export function wrapText(text: string, max: number, limit = Infinity): string[] {
  max = Math.max(4, Math.floor(max));
  const lines: string[] = [];
  let cur = '';
  for (let word of text.split(/\s+/).filter(Boolean)) {
    while (word.length > max) {
      if (cur) lines.push(cur);
      cur = '';
      lines.push(word.slice(0, max));
      word = word.slice(max);
    }
    if (!cur) cur = word;
    else if (cur.length + 1 + word.length <= max) cur += ' ' + word;
    else {
      lines.push(cur);
      cur = word;
    }
  }
  if (cur) lines.push(cur);
  if (lines.length > limit) {
    const kept = lines.slice(0, limit);
    const lastLine = kept[limit - 1];
    kept[limit - 1] = (lastLine.length >= max ? lastLine.slice(0, max - 1) : lastLine) + '…';
    return kept;
  }
  return lines;
}

/**
 * Edges as the layout should see them: agents list nodes in flow order,
 * so an edge pointing back up that order (a response, a cache fill) is
 * turned around. The result has no cycles and keeps the listed order left
 * to right; arrows are still drawn in their real direction.
 */
export function forwardEdges(order: string[], edges: { from: string; to: string }[]): { source: string; target: string }[] {
  const at = new Map(order.map((id, i) => [id, i]));
  return edges.map((e) => ((at.get(e.to) ?? 0) < (at.get(e.from) ?? 0) ? { source: e.to, target: e.from } : { source: e.from, target: e.to }));
}

export interface Pt {
  x: number;
  y: number;
}

/** SVG path data through a list of points. */
export const pathD = (pts: Pt[]) => 'M' + pts.map((p) => `${p.x},${p.y}`).join(' L');

/** The point a fraction t (0..1) of the way along a route. */
export function pointAlong(pts: Pt[], t: number): Pt {
  const lens = pts.slice(1).map((p, i) => Math.hypot(p.x - pts[i].x, p.y - pts[i].y));
  let left = lens.reduce((a, b) => a + b, 0) * Math.max(0, Math.min(1, t));
  for (let i = 0; i < lens.length; i++) {
    if (left <= lens[i] || i === lens.length - 1) {
      const k = lens[i] ? Math.min(1, left / lens[i]) : 0;
      return { x: pts[i].x + (pts[i + 1].x - pts[i].x) * k, y: pts[i].y + (pts[i + 1].y - pts[i].y) * k };
    }
    left -= lens[i];
  }
  return pts[0];
}

/**
 * Where a label goes on a route: the middle of its longest straight
 * stretch, and whether that stretch runs up and down.
 */
export function labelAnchor(pts: Pt[]): { x: number; y: number; vertical: boolean } {
  let best = 0;
  let len = -1;
  for (let i = 0; i + 1 < pts.length; i++) {
    const l = Math.hypot(pts[i + 1].x - pts[i].x, pts[i + 1].y - pts[i].y);
    if (l > len) [best, len] = [i, l];
  }
  const a = pts[best];
  const b = pts[best + 1] ?? a;
  return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2, vertical: Math.abs(b.y - a.y) > Math.abs(b.x - a.x) };
}

export interface LabelItem {
  id: string;
  /** The route the label belongs to. */
  pts: Pt[];
  w: number;
  h: number;
  /** Prefer below the line (the lower of a two-way pair). */
  below?: boolean;
}

const overlap = (a: Rect, b: Rect) => Math.max(0, Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x)) * Math.max(0, Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y));

/**
 * Places labels next to their routes so they don't cover each other or
 * the boxes. Each label tries spots along its longest straight stretches
 * (above/below a horizontal one, right/left of a vertical one) and takes
 * the first that's clear, else the least covered. Earlier items win, so
 * pass the most important first. Returns each label's top-left corner.
 */
export function placeLabels(items: LabelItem[], obstacles: Rect[], gap = 8): Map<string, Pt> {
  const placed: Rect[] = [];
  const out = new Map<string, Pt>();
  for (const it of items) {
    const segs = it.pts
      .slice(1)
      .map((b, i) => ({ a: it.pts[i], b, len: Math.hypot(b.x - it.pts[i].x, b.y - it.pts[i].y) }))
      .filter((s) => s.len > 0)
      .sort((x, y) => y.len - x.len)
      .slice(0, 3);
    const spots: Pt[] = [];
    for (const s of segs) {
      const vertical = Math.abs(s.b.y - s.a.y) > Math.abs(s.b.x - s.a.x);
      for (const t of [0.5, 0.3, 0.7]) {
        const mx = s.a.x + (s.b.x - s.a.x) * t;
        const my = s.a.y + (s.b.y - s.a.y) * t;
        const sides = vertical
          ? [{ x: mx + gap, y: my - it.h / 2 }, { x: mx - it.w - gap, y: my - it.h / 2 }]
          : [{ x: mx - it.w / 2, y: my - it.h - gap }, { x: mx - it.w / 2, y: my + gap }];
        spots.push(...(it.below ? sides.reverse() : sides));
      }
    }
    if (spots.length === 0) spots.push({ x: it.pts[0]?.x ?? 0, y: it.pts[0]?.y ?? 0 });
    let best = spots[0];
    let bestScore = Infinity;
    for (const p of spots) {
      const r = { x: p.x, y: p.y, w: it.w, h: it.h };
      const score = placed.reduce((sum, q) => sum + 2 * overlap(r, q), 0) + obstacles.reduce((sum, q) => sum + overlap(r, q), 0);
      if (score < bestScore) [best, bestScore] = [p, score];
      if (score === 0) break;
    }
    placed.push({ x: best.x, y: best.y, w: it.w, h: it.h });
    out.set(it.id, best);
  }
  return out;
}

/** Linear interpolation between two rectangles (t from 0 to 1). */
export function lerpRect(a: Rect, b: Rect, t: number): Rect {
  const l = (p: number, q: number) => p + (q - p) * t;
  return { x: l(a.x, b.x), y: l(a.y, b.y), w: l(a.w, b.w), h: l(a.h, b.h) };
}

/**
 * Where a straight edge leaves a box centred at (cx, cy) with half-size
 * (hw, hh) when heading towards (tx, ty).
 */
export function boxExit(cx: number, cy: number, hw: number, hh: number, tx: number, ty: number): [number, number] {
  const dx = tx - cx;
  const dy = ty - cy;
  if (dx === 0 && dy === 0) return [cx, cy];
  const scale = Math.min(dx === 0 ? Infinity : hw / Math.abs(dx), dy === 0 ? Infinity : hh / Math.abs(dy));
  return [cx + dx * scale, cy + dy * scale];
}
