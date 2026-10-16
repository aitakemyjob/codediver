// ELK layered layout. ELK only computes positions; React Flow draws.
import ELK from 'elkjs/lib/elk.bundled.js';

export type Direction = 'RIGHT' | 'DOWN' | 'LEFT' | 'UP';

export interface LayoutNode {
  id: string;
  width: number;
  height: number;
  /** Optional layer hint (e.g. call depth) to keep columns tidy. */
  layer?: number;
}

export interface LayoutEdge {
  id: string;
  source: string;
  target: string;
}

export interface Positioned {
  x: number;
  y: number;
}

let elk: InstanceType<typeof ELK> | undefined;

/** Maps LR/TB/RL/BT (or ELK names) to an ELK direction; defaults to RIGHT. */
export function toElkDirection(d?: string): Direction {
  switch ((d ?? '').toUpperCase()) {
    case 'TB':
    case 'DOWN':
      return 'DOWN';
    case 'RL':
    case 'LEFT':
      return 'LEFT';
    case 'BT':
    case 'UP':
      return 'UP';
    default:
      return 'RIGHT';
  }
}

/**
 * Computes node positions with ELK's layered algorithm. Edges to unknown
 * nodes and self-loops are ignored.
 */
export async function layout(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  direction: Direction = 'RIGHT',
  opts: { spacing?: number; layerSpacing?: number } = {},
): Promise<Map<string, Positioned>> {
  elk ??= new ELK();
  const ids = new Set(nodes.map((n) => n.id));
  const res = await elk.layout({
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': direction,
      'elk.spacing.nodeNode': String(opts.spacing ?? 28),
      'elk.layered.spacing.nodeNodeBetweenLayers': String(opts.layerSpacing ?? 70),
      'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
      'elk.layered.crossingMinimization.strategy': 'LAYER_SWEEP',
      'elk.layered.cycleBreaking.strategy': 'DEPTH_FIRST',
      'elk.separateConnectedComponents': 'true',
      'elk.spacing.componentComponent': '60',
    },
    children: nodes.map((n) => ({ id: n.id, width: n.width, height: n.height })),
    edges: edges
      .filter((e) => ids.has(e.source) && ids.has(e.target) && e.source !== e.target)
      .map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  });
  const out = new Map<string, Positioned>();
  for (const c of res.children ?? []) out.set(c.id, { x: c.x ?? 0, y: c.y ?? 0 });
  return out;
}

/** Rough text width for sizing nodes before they render. */
export function textWidth(s: string, px = 12.5, mono = false): number {
  return s.length * px * (mono ? 0.62 : 0.56);
}
