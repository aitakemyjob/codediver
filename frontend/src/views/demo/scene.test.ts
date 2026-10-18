import type { Demo, DemoNode } from '../../lib/types';
import { groupsOf } from './demo';
import { buildScene, flowLabel, LANE_HEAD, valueLines, type Box } from './scene';

// The CDN reference architecture in miniature: clients, an edge region with
// two lower-tier data centers and an upper tier (each a grid of tiles), and
// an origin. Two-way edges make cycles the layout has to cope with.
const tiles = (dc: string): DemoNode[] =>
  ['DoS', 'DNS', 'CDN', 'Cache'].map((t) => ({ id: `${dc}-${t}`, label: t, kind: 'icon', icon: 'shield', parent: dc }));
const cdn: Demo = {
  id: 'd1', repo: 'r', title: 'CDN', layout: 'architecture', createdAt: '',
  nodes: [
    { id: 'clients', label: 'Clients', icon: 'users' },
    { id: 'c1', label: 'Client 1', kind: 'icon', icon: 'laptop', parent: 'clients' },
    { id: 'c2', label: 'Client 2', kind: 'icon', icon: 'tablet', parent: 'clients' },
    { id: 'edge', label: 'Global Anycast Edge Network', kind: 'region', detail: 'Anycast IPs' },
    { id: 'dc1', label: 'Data Center 1', detail: 'Lower tier', parent: 'edge' }, ...tiles('dc1'),
    { id: 'dc2', label: 'Data Center 2', detail: 'Lower tier', parent: 'edge' }, ...tiles('dc2'),
    { id: 'dc3', label: 'Data Center 3', detail: 'Upper tier', parent: 'edge' }, ...tiles('dc3'),
    { id: 'origin', label: 'Origin' },
    { id: 'servers', label: 'Data Centers', kind: 'icon', parent: 'origin' },
  ],
  edges: [
    { from: 'c1', to: 'dc1', label: 'Request', detail: 'example.com/index.html', color: 'green' },
    { from: 'c2', to: 'dc2', label: 'Request', color: 'purple' },
    { from: 'dc1', to: 'dc3' },
    { from: 'dc3', to: 'dc1', label: 'Cache' },
    { from: 'dc2', to: 'dc3' },
    { from: 'dc3', to: 'dc2', label: 'Cache' },
    { from: 'dc3', to: 'origin', label: 'Request' },
    { from: 'origin', to: 'dc3', label: 'Response' },
  ],
  steps: [{ title: 'one', values: { c1: 'a fairly long value that has to wrap onto more lines', dc3: 'HIT' } }],
};

const inside = (a: Box, b: Box) => a.x >= b.x - 0.5 && a.y >= b.y - 0.5 && a.x + a.w <= b.x + b.w + 0.5 && a.y + a.h <= b.y + b.h + 0.5;
const onBorder = (p: { x: number; y: number }, b: Box) =>
  p.x >= b.x - 1 && p.x <= b.x + b.w + 1 && p.y >= b.y - 1 && p.y <= b.y + b.h + 1 &&
  (Math.abs(p.x - b.x) < 1 || Math.abs(p.x - b.x - b.w) < 1 || Math.abs(p.y - b.y) < 1 || Math.abs(p.y - b.y - b.h) < 1);

describe('nested scene layout', () => {
  let boxes: Map<string, Box>;
  let routes: Map<string, { x: number; y: number }[]>;
  beforeAll(async () => {
    ({ boxes, routes } = await buildScene(cdn, groupsOf(cdn)));
  });
  const box = (id: string) => boxes.get(id)!;

  it('lays out every node, containers around their children', () => {
    expect(boxes.size).toBe(cdn.nodes.length);
    for (const n of cdn.nodes) if (n.parent) expect(inside(box(n.id), box(n.parent)), `${n.id} in ${n.parent}`).toBe(true);
    expect(box('edge').container && box('dc1').container && !box('c1').container).toBe(true);
    expect(box('dc1').depth).toBe(1);
    expect(box('dc1-DNS').depth).toBe(2);
    // Children sit below the container's header.
    expect(box('dc1-DoS').y).toBeGreaterThanOrEqual(box('dc1').y + box('dc1').head);
  });

  it('puts a container of four edge-less tiles in a 2×2 grid', () => {
    const t = tiles('dc1').map((n) => box(n.id));
    expect(new Set(t.map((b) => Math.round(b.x))).size).toBe(2);
    expect(new Set(t.map((b) => Math.round(b.y))).size).toBe(2);
    expect(t.every((b) => b.tile)).toBe(true);
  });

  it('keeps the listed order and flows left to right despite two-way edges', () => {
    // Lower tiers share a layer, DC1 above DC2; the upper tier comes after.
    expect(Math.round(box('dc1').x)).toBe(Math.round(box('dc2').x));
    expect(box('dc1').y).toBeLessThan(box('dc2').y);
    expect(box('c1').y).toBeLessThan(box('c2').y);
    expect(box('dc3').x).toBeGreaterThan(box('dc1').x + box('dc1').w);
    expect(box('origin').x).toBeGreaterThan(box('dc3').x + box('dc3').w);
    expect(box('clients').x + box('clients').w).toBeLessThan(box('edge').x);
  });

  it('routes every edge from its own source to its own target', () => {
    for (const e of cdn.edges) {
      const r = routes.get(`${e.from}>${e.to}`);
      expect(r, `${e.from}>${e.to}`).toBeDefined();
      expect(onBorder(r![0], box(e.from)), `${e.from}>${e.to} starts on ${e.from}`).toBe(true);
      expect(onBorder(r![r!.length - 1], box(e.to)), `${e.from}>${e.to} ends on ${e.to}`).toBe(true);
      // Orthogonal: every stretch is horizontal or vertical.
      r!.slice(1).forEach((p, i) => expect(Math.abs(p.x - r![i].x) < 0.5 || Math.abs(p.y - r![i].y) < 0.5).toBe(true));
    }
    // The two directions of a pair are routed separately.
    expect(routes.get('dc1>dc3')).not.toEqual([...routes.get('dc3>dc1')!].reverse());
  });

  it('reserves room for value tags under leaf boxes only', () => {
    expect(box('c1').foot).toBe(valueLines(cdn.steps[0].values!.c1, box('c1').w).length * 14 + 14);
    expect(box('dc3').foot).toBe(0); // a container's value is a badge on its border
    expect(box('c2').foot).toBe(0);
  });

  it('gives containers a header with a title and subtitle', () => {
    expect(box('dc1').label).toEqual(['Data Center 1']);
    expect(box('dc1').detail).toEqual(['Lower tier']);
    expect(box('dc1').head).toBeGreaterThan(30);
  });
});

describe('flat scene layout', () => {
  it('lays architecture lanes out in group order with straight edges', async () => {
    const d: Demo = {
      id: 'f', repo: 'r', title: 'lanes', layout: 'architecture', createdAt: '',
      nodes: [
        { id: 'ui', label: 'UI', group: 'frontend' },
        { id: 'api', label: 'API', group: 'backend', detail: 'a detail long enough to wrap onto a second line of text' },
        { id: 'db', label: 'DB', kind: 'datastore', group: 'storage' },
      ],
      edges: [{ from: 'ui', to: 'api' }, { from: 'api', to: 'db' }, { from: 'db', to: 'api' }],
      steps: [{ title: 's' }],
    };
    const { boxes, routes } = await buildScene(d, groupsOf(d));
    expect(routes.size).toBe(0);
    const [ui, api, db] = ['ui', 'api', 'db'].map((id) => boxes.get(id)!);
    expect(ui.x).toBeLessThan(api.x);
    expect(api.x).toBeLessThan(db.x);
    // Lanes leave room for their headers.
    expect(Math.min(ui.y, api.y, db.y)).toBeGreaterThanOrEqual(LANE_HEAD);
    expect(api.detail.length).toBe(2);
    expect(api.h).toBeGreaterThan(ui.h);
  });

  it('lays execution trees top-down', async () => {
    const d: Demo = {
      id: 't', repo: 'r', title: 'tree', layout: 'tree', createdAt: '',
      nodes: [{ id: 'main', label: 'main' }, { id: 'a', label: 'a' }, { id: 'b', label: 'b' }],
      edges: [{ from: 'main', to: 'a' }, { from: 'main', to: 'b' }],
      steps: [{ title: 's' }],
    };
    const { boxes } = await buildScene(d, groupsOf(d));
    expect(boxes.get('a')!.y).toBeGreaterThan(boxes.get('main')!.y);
    expect(boxes.get('a')!.y).toBe(boxes.get('b')!.y);
  });
});

describe('flow labels', () => {
  it('wrap long text and measure label plus detail', () => {
    const l = flowLabel('00 34 12 00 00 04 01 00 CB F8 01 00 02 57 extra words', 'example.com/index.html');
    expect(l.lines.length).toBe(2);
    expect(l.details).toEqual(['example.com/index.html']);
    expect(l.h).toBe(2 * 14 + 13 + 6);
    expect(flowLabel('', '').h).toBe(6);
  });
});
