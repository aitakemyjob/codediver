// ELK layered layout. ELK only computes positions; React Flow draws.
import ELK from 'elkjs/lib/elk.bundled.js';

export type Direction = 'RIGHT' | 'DOWN' | 'LEFT' | 'UP';

export interface LayoutNode {
  id: string;
  width: number;
  height: number;
  /** Optional layer hint (e.g. call depth) to keep columns tidy. */
  layer?: number;
  /** Optional partition: nodes are placed in increasing partition order along the layout direction (lanes). */
  partition?: number;
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
  const partitioned = nodes.some((n) => n.partition !== undefined);
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
      ...(partitioned ? { 'elk.partitioning.activate': 'true' } : {}),
    },
    children: nodes.map((n) => ({
      id: n.id,
      width: n.width,
      height: n.height,
      ...(n.partition !== undefined ? { layoutOptions: { 'elk.partitioning.partition': String(n.partition) } } : {}),
    })),
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

export interface NestedNode {
  id: string;
  /** Size of a leaf; containers are sized by their children. */
  width: number;
  height: number;
  /** Container this node sits in. */
  parent?: string;
  /** Room a container keeps around its children (its header goes on top). */
  padding?: { top: number; left: number; bottom: number; right: number };
}

export interface Placed {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** An edge's route: its start, bend and end points. */
export type Route = Positioned[];

/**
 * Lays out a nested graph (containers inside containers) with ELK's
 * layered algorithm across the whole hierarchy, so edges may connect
 * nodes in different containers or containers themselves. Siblings keep
 * the order they are listed in, and edges must not form cycles. Returns
 * absolute positions and sizes for every node, containers included, and
 * each edge's orthogonal route around the nodes, by edge id.
 */
export async function layoutNested(
  nodes: NestedNode[],
  edges: LayoutEdge[],
  direction: Direction = 'RIGHT',
  opts: { spacing?: number; layerSpacing?: number } = {},
): Promise<{ nodes: Map<string, Placed>; routes: Map<string, Route> }> {
  elk ??= new ELK();
  const layered = {
    'elk.edgeRouting': 'ORTHOGONAL',
    'elk.spacing.edgeEdge': '14',
    'elk.spacing.edgeNode': '18',
    'elk.layered.spacing.edgeNodeBetweenLayers': '18',
    'elk.layered.spacing.edgeEdgeBetweenLayers': '12',
    'elk.algorithm': 'layered',
    'elk.direction': direction,
    'elk.spacing.nodeNode': String(opts.spacing ?? 28),
    'elk.layered.spacing.nodeNodeBetweenLayers': String(opts.layerSpacing ?? 70),
    'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
    // MODEL_ORDER strategies crash elkjs with nested graphs, so callers
    // break cycles themselves (see forwardEdges in the demo scene) and the
    // listed order is kept by seeding positions for interactive crossing
    // minimization: siblings in a layer stay in the order they were given.
    'elk.layered.cycleBreaking.strategy': 'DEPTH_FIRST',
    'elk.layered.crossingMinimization.strategy': 'INTERACTIVE',
    'elk.spacing.componentComponent': '40',
  };
  type ElkNode = { id: string; x?: number; y?: number; width?: number; height?: number; children: ElkNode[]; layoutOptions?: Record<string, string> };
  const ids = new Set(nodes.map((n) => n.id));
  const across = direction === 'RIGHT' || direction === 'LEFT' ? 'y' : 'x';
  // Interactive ordering compares node centers, so seeds sit far enough
  // apart that a tall node can't overtake the one listed after it.
  const elkNodes = new Map<string, ElkNode>(nodes.map((n, i) => [n.id, { id: n.id, children: [], x: 0, y: 0, [across]: i * 10_000 }]));
  const root: ElkNode = { id: '__root', children: [], layoutOptions: { ...layered, 'elk.hierarchyHandling': 'INCLUDE_CHILDREN', 'elk.json.edgeCoords': 'ROOT' } };
  for (const n of nodes) (elkNodes.get(n.parent ?? '') ?? root).children.push(elkNodes.get(n.id)!);
  for (const n of nodes) {
    const e = elkNodes.get(n.id)!;
    if (e.children.length === 0) {
      e.width = n.width;
      e.height = n.height;
      continue;
    }
    const p = n.padding ?? { top: 40, left: 16, bottom: 16, right: 16 };
    e.layoutOptions = {
      ...layered,
      'elk.padding': `[top=${p.top},left=${p.left},bottom=${p.bottom},right=${p.right}]`,
      // Wide enough for the container's own header.
      'elk.nodeSize.constraints': 'MINIMUM_SIZE',
      'elk.nodeSize.minimum': `(${n.width}, ${n.height})`,
    };
  }
  const res = await elk.layout({
    ...root,
    edges: edges
      .filter((e) => ids.has(e.source) && ids.has(e.target) && e.source !== e.target)
      .map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  } as Parameters<typeof elk.layout>[0]);
  const out = new Map<string, Placed>();
  const walk = (list: typeof res.children, ox: number, oy: number) => {
    for (const c of list ?? []) {
      const x = ox + (c.x ?? 0);
      const y = oy + (c.y ?? 0);
      out.set(c.id, { x, y, w: c.width ?? 0, h: c.height ?? 0 });
      walk(c.children, x, y);
    }
  };
  walk(res.children, 0, 0);
  // ELK may file edges under the container they run in; coordinates are
  // root-relative either way (edgeCoords ROOT).
  type ElkOut = { edges?: { id: string; sections?: { startPoint: Positioned; endPoint: Positioned; bendPoints?: Positioned[] }[] }[]; children?: ElkOut[] };
  const routes = new Map<string, Route>();
  const collect = (n: ElkOut) => {
    for (const e of n.edges ?? []) {
      const s = e.sections?.[0];
      if (s) routes.set(e.id, [s.startPoint, ...(s.bendPoints ?? []), s.endPoint]);
    }
    n.children?.forEach(collect);
  };
  collect(res as ElkOut);
  return { nodes: out, routes };
}
