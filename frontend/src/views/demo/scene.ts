// Sizes and positions of everything in a demo scene: plain boxes, icon
// tiles and (nested) containers, laid out once with ELK. Text is wrapped
// here, before layout, so the layout leaves room for all of it.
import type { LucideIcon } from 'lucide-react';
import { layout, layoutNested, textWidth } from '../../lib/layout';
import type { Demo, DemoNode } from '../../lib/types';
import { forwardEdges, wrapText, type Pt } from './demo';
import { iconFor } from './icons';

// Character widths of the scene's fonts, for wrapping text before render.
export const LABEL_CH = 7.3; // 13px UI, semibold
export const MONO_CH = 6.82; // 11px mono (details, values, packet labels)
const TILE_CH = 6.5; // 11.5px UI, tile labels
const FLOW_CHARS = 30; // packet labels wrap at ~200px
export const LINE = 14;
export const LANE_HEAD = 28;
const ICON_GAP = 32; // room an icon takes left of a box's text
const GRID_GAP = 8;

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
  /** Space reserved under the node for its value tags. */
  foot: number;
  label: string[];
  detail: string[];
  /** Nesting depth (0 = top level). */
  depth: number;
  /** Has children: drawn as a frame with a header. */
  container: boolean;
  /** Compact icon tile (kind icon). */
  tile: boolean;
  /** Height of a container's header. */
  head: number;
  icon?: LucideIcon;
}

/** A laid-out scene: node boxes, and edge routes keyed "from>to" (empty when edges are drawn straight). */
export interface Scene {
  boxes: Map<string, Box>;
  routes: Map<string, Pt[]>;
}

/** Value tag lines for a node of width w. */
export const valueLines = (text: string, w: number) => wrapText(text, (w - 16) / MONO_CH, 4);

/** Packet or edge label lines (bold label, then detail) and the box they take up. */
export function flowLabel(text = '', detail = ''): { lines: string[]; details: string[]; w: number; h: number } {
  const lines = text ? wrapText(text, FLOW_CHARS, 3) : [];
  const details = detail ? wrapText(detail, FLOW_CHARS + 2, 2) : [];
  const widest = Math.max(0, ...lines.map((l) => l.length * MONO_CH), ...details.map((l) => l.length * MONO_CH * 0.92));
  return { lines, details, w: widest + 12, h: lines.length * LINE + details.length * 13 + 6 };
}

type Sized = Omit<Box, 'x' | 'y' | 'foot' | 'depth'>;

/** A plain box: kind, label and detail, with an icon on the left if it has one. */
function boxSize(n: DemoNode): Sized {
  const icon = iconFor(n.icon, n.kind);
  const pad = icon ? ICON_GAP : 0;
  const w = Math.max(130, Math.min(260, Math.max(textWidth(n.label, 13), textWidth(n.detail ?? '', 11, true)) + 36)) + pad;
  const label = wrapText(n.label, (w - 24 - pad) / LABEL_CH, 2);
  const detail = n.detail ? wrapText(n.detail, (w - 24 - pad) / MONO_CH, 2) : [];
  return { w, h: 26 + label.length * 17 + detail.length * LINE + 9, label, detail, container: false, tile: false, head: 0, icon };
}

/** An icon tile: a big icon with the label underneath. */
function tileSize(n: DemoNode): Sized {
  const w = Math.max(76, Math.min(124, n.label.length * TILE_CH + 16));
  const label = wrapText(n.label, (w - 10) / TILE_CH, 2);
  return { w, h: 52 + label.length * LINE + 4, label, detail: [], container: false, tile: true, head: 0, icon: iconFor(n.icon, n.kind, true) };
}

/** A container's header (icon, title, subtitle); its size comes from layout. */
function containerSize(n: DemoNode): Sized {
  const icon = iconFor(n.icon, undefined);
  const pad = icon ? 26 : 0;
  const w = Math.min(320, Math.max(textWidth(n.label, 13.5), textWidth(n.detail ?? '', 11)) + 32 + pad);
  const label = wrapText(n.label, (w - 28 - pad) / 7.6, 2);
  const detail = n.detail ? wrapText(n.detail, (w - 28 - pad) / 6.2, 2) : [];
  const head = 14 + label.length * 17 + detail.length * LINE + 6;
  return { w, h: head + 30, label, detail, container: true, tile: false, head, icon };
}

/**
 * Lays out a demo scene. Without nesting, architecture demos get lanes
 * by group; with nesting (any node has a parent), ELK lays out the whole
 * hierarchy and containers of edge-less icon tiles become grids.
 */
export async function buildScene(demo: Demo, groups: string[]): Promise<Scene> {
  const kids = new Map<string, DemoNode[]>();
  for (const n of demo.nodes) if (n.parent) kids.set(n.parent, [...(kids.get(n.parent) ?? []), n]);
  const parentOf = new Map(demo.nodes.map((n) => [n.id, n.parent]));
  const depthOf = (id: string) => {
    let d = 0;
    for (let p = parentOf.get(id); p; p = parentOf.get(p)) d++;
    return d;
  };
  // Anything an edge or flow touches has to be laid out on its own.
  const linked = new Set(demo.edges.flatMap((e) => [e.from, e.to]));
  demo.steps.forEach((st) => st.flows?.forEach((f) => (linked.add(f.from), linked.add(f.to))));

  const sizes = new Map<string, Sized>(
    demo.nodes.map((n) => [n.id, kids.has(n.id) ? containerSize(n) : n.kind === 'icon' ? tileSize(n) : boxSize(n)]),
  );
  // Containers holding only unlinked tiles: we place the tiles in a grid.
  const grids = new Map<string, { cols: number; cw: number; ch: number }>();
  for (const [id, list] of kids) {
    if (!list.every((c) => c.kind === 'icon' && !kids.has(c.id) && !linked.has(c.id))) continue;
    const cols = list.length <= 2 ? 1 : list.length <= 4 ? 2 : 3;
    const cw = Math.max(...list.map((c) => sizes.get(c.id)!.w));
    const ch = Math.max(...list.map((c) => sizes.get(c.id)!.h));
    const rows = Math.ceil(list.length / cols);
    const box = sizes.get(id)!;
    box.w = Math.max(box.w, cols * cw + (cols - 1) * GRID_GAP + 24);
    box.h = box.head + rows * ch + (rows - 1) * GRID_GAP + 12;
    grids.set(id, { cols, cw, ch });
  }
  // Reserve room under each node for its tallest value tag, so tags never
  // run into the node below.
  const foot = new Map(
    demo.nodes.map((n) => {
      const most = Math.max(0, ...demo.steps.map((st) => (st.values?.[n.id] ? valueLines(st.values[n.id], sizes.get(n.id)!.w).length : 0)));
      return [n.id, most && !kids.has(n.id) ? most * LINE + 14 : 0];
    }),
  );
  // Labels sit in the gap between layers, so make the gap fit the widest
  // (side by side) or tallest (top-down) one.
  const labels = [
    ...demo.edges.map((e) => flowLabel(e.label, e.detail)),
    ...demo.steps.flatMap((st) => (st.flows ?? []).map((f) => flowLabel(f.label, f.detail))),
  ].filter((l) => l.h > 6);
  const tree = demo.layout === 'tree';
  const gap = tree ? Math.max(56, ...labels.map((l) => l.h + 28)) : Math.min(260, Math.max(90, ...labels.map((l) => l.w + 36)));
  const edges = forwardEdges(demo.nodes.map((n) => n.id), demo.edges).map((e, i) => ({ id: `e${i}`, ...e }));
  const out = new Map<string, Box>();

  if (kids.size === 0) {
    const lanes = demo.layout === 'architecture' && groups.length > 1;
    const pos = await layout(
      demo.nodes.map((n) => ({
        id: n.id,
        width: sizes.get(n.id)!.w,
        height: sizes.get(n.id)!.h + foot.get(n.id)!,
        partition: lanes && n.group ? groups.indexOf(n.group) : undefined,
      })),
      edges,
      tree ? 'DOWN' : 'RIGHT',
      { spacing: 34, layerSpacing: gap },
    );
    for (const n of demo.nodes) {
      const p = pos.get(n.id) ?? { x: 0, y: 0 };
      out.set(n.id, { ...sizes.get(n.id)!, x: p.x, y: p.y + (groups.length > 1 ? LANE_HEAD : 0), foot: foot.get(n.id)!, depth: 0 });
    }
    return { boxes: out, routes: new Map() };
  }

  const inGrid = (n: DemoNode) => !!n.parent && grids.has(n.parent);
  const { nodes: placed, routes: raw } = await layoutNested(
    demo.nodes.filter((n) => !inGrid(n)).map((n) => {
      const sz = sizes.get(n.id)!;
      const isBox = !kids.has(n.id) || grids.has(n.id);
      return {
        id: n.id,
        parent: n.parent,
        width: sz.w,
        height: isBox ? sz.h + foot.get(n.id)! : sz.h,
        padding: { top: sz.head + 10, left: 16, bottom: 16, right: 16 },
      };
    }),
    edges,
    tree ? 'DOWN' : 'RIGHT',
    { spacing: 30, layerSpacing: gap },
  );
  for (const n of demo.nodes) {
    if (inGrid(n)) continue;
    const p = placed.get(n.id) ?? { x: 0, y: 0, w: 0, h: 0 };
    const sz = sizes.get(n.id)!;
    // ELK sizes containers; boxes keep their own size (the rest was value room).
    const h = kids.has(n.id) && !grids.has(n.id) ? p.h : sz.h;
    out.set(n.id, { ...sz, x: p.x, y: p.y, w: kids.has(n.id) ? p.w : sz.w, h, foot: foot.get(n.id)!, depth: depthOf(n.id) });
  }
  for (const [id, g] of grids) {
    const c = out.get(id)!;
    const list = kids.get(id)!;
    const used = Math.min(g.cols, list.length) * g.cw + (Math.min(g.cols, list.length) - 1) * GRID_GAP;
    const x0 = c.x + (c.w - used) / 2;
    list.forEach((n, i) => {
      const sz = sizes.get(n.id)!;
      const col = i % g.cols;
      const row = Math.floor(i / g.cols);
      out.set(n.id, {
        ...sz,
        x: x0 + col * (g.cw + GRID_GAP) + (g.cw - sz.w) / 2,
        y: c.y + c.head + row * (g.ch + GRID_GAP),
        foot: 0,
        depth: depthOf(n.id),
      });
    });
  }
  // Routes in each edge's real direction (layout may have turned it around).
  const routes = new Map<string, Pt[]>();
  demo.edges.forEach((e, i) => {
    const r = raw.get(`e${i}`);
    const key = `${e.from}>${e.to}`;
    if (r && !routes.has(key)) routes.set(key, edges[i].source === e.from ? r : [...r].reverse());
  });
  return { boxes: out, routes };
}
