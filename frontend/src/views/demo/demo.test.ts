import { boxExit, cameraFor, followTargets, forwardEdges, labelAnchor, pathD, placeLabels, pointAlong, wrapText, frameAt, groupsOf, lerpRect, panBy, stepDuration, zoomAt } from './demo';
import type { Demo } from '../../lib/types';

const demo: Demo = {
  id: 'd1', repo: 'r', title: 't', layout: 'architecture', createdAt: '',
  nodes: [
    { id: 'user', label: 'User', group: 'client' },
    { id: 'api', label: 'API', group: 'server' },
    { id: 'db', label: 'DB', group: 'storage' },
    { id: 'cache', label: 'Cache', group: 'server' },
  ],
  edges: [{ from: 'user', to: 'api' }, { from: 'api', to: 'db' }],
  steps: [
    { title: 'Request', focus: ['user'], flows: [{ from: 'user', to: 'api', label: 'GET' }] },
    { title: 'Query', focus: ['api'], reveal: ['db'], values: { api: 'id=7' } },
    { title: 'Cached', focus: ['db'], reveal: ['cache'], values: { api: 'id=7 (cached)', db: 'row' } },
  ],
};

describe('demo frames', () => {
  it('reveals nodes at their step and keeps the rest visible', () => {
    expect([...frameAt(demo, 0).visible].sort()).toEqual(['api', 'user']);
    expect([...frameAt(demo, 1).visible].sort()).toEqual(['api', 'db', 'user']);
    expect(frameAt(demo, 2).visible.has('cache')).toBe(true);
  });

  it('tracks focus, the path so far and flows', () => {
    const f = frameAt(demo, 2);
    expect([...f.focus]).toEqual(['db']);
    expect([...f.visited].sort()).toEqual(['api', 'user']);
    expect(frameAt(demo, 0).flows).toHaveLength(1);
    expect(f.flows).toHaveLength(0);
  });

  it('keeps values like state, marking the ones set this step', () => {
    expect(frameAt(demo, 0).values).toEqual({});
    expect(frameAt(demo, 1).values.api).toEqual({ text: 'id=7', fresh: true });
    const f = frameAt(demo, 2);
    expect(f.values.api).toEqual({ text: 'id=7 (cached)', fresh: true });
    expect(f.values.db.fresh).toBe(true);
  });

  it('shows nodes a flow touches even before they are revealed', () => {
    const d: Demo = { ...demo, steps: [{ title: 'x', flows: [{ from: 'user', to: 'db' }] }, { title: 'y', reveal: ['db'] }] };
    expect(frameAt(d, 0).visible.has('db')).toBe(true);
  });

  it('orders lanes and paces steps', () => {
    expect(groupsOf(demo)).toEqual(['client', 'server', 'storage']);
    expect(stepDuration(demo, 0)).toBeGreaterThanOrEqual(2600);
    expect(stepDuration(demo, 9)).toBe(0);
  });

  it('finds where an edge leaves a box', () => {
    expect(boxExit(0, 0, 10, 5, 100, 0)).toEqual([10, 0]);
    expect(boxExit(0, 0, 10, 5, 0, -50)).toEqual([0, -5]);
    expect(boxExit(0, 0, 10, 5, 0, 0)).toEqual([0, 0]);
  });

  it('frames the visible nodes at the stage aspect ratio', () => {
    const cam = cameraFor([{ x: 0, y: 0, w: 100, h: 50 }, { x: 900, y: 0, w: 100, h: 50 }], 2, 50, 300);
    expect(cam.w / cam.h).toBeCloseTo(2);
    expect(cam.x).toBeLessThanOrEqual(-50);
    expect(cam.x + cam.w).toBeGreaterThanOrEqual(1050);
    // One small node isn't zoomed past the minimum width.
    expect(cameraFor([{ x: 0, y: 0, w: 10, h: 10 }], 1, 0, 400).w).toBe(400);
    expect(lerpRect({ x: 0, y: 0, w: 10, h: 10 }, { x: 10, y: 10, w: 20, h: 20 }, 0.5)).toEqual({ x: 5, y: 5, w: 15, h: 15 });
  });

  it('zooms around the cursor and pans by screen pixels', () => {
    const cam = { x: 0, y: 0, w: 1000, h: 500 };
    const zin = zoomAt(cam, 0.5, 0.25, 0.5);
    expect(zin).toEqual({ x: 125, y: 125, w: 500, h: 250 });
    // The point under the cursor (250, 250) stays put.
    expect(zin.x + 0.25 * zin.w).toBe(250);
    expect(zoomAt(cam, 0.01, 0.5, 0.5).w).toBe(120); // clamped
    expect(panBy(cam, 100, -50, 500, 250)).toEqual({ x: -200, y: 100, w: 1000, h: 500 });
  });

  it('follows the focus and packets in flight', () => {
    expect(followTargets(frameAt(demo, 0)).sort()).toEqual(['api', 'user']);
    expect(followTargets(frameAt(demo, 2))).toEqual(['db']);
    const d: Demo = { ...demo, steps: [{ title: 'quiet' }] };
    expect(followTargets(frameAt(d, 0)).length).toBe(4);
  });

  it('wraps long text at spaces, splits long words and cuts with …', () => {
    expect(wrapText('00 34 12 00 00 04 01', 9)).toEqual(['00 34 12', '00 00 04', '01']);
    expect(wrapText('signaliot_lora_packet_encode()', 12)).toEqual(['signaliot_lo', 'ra_packet_en', 'code()']);
    expect(wrapText('a b c d e f g h', 4, 2)).toEqual(['a b', 'c d…']);
    expect(wrapText('short', 20)).toEqual(['short']);
    expect(wrapText('', 20)).toEqual([]);
  });

  it('shows nested nodes only with their containers', () => {
    const d: Demo = {
      ...demo,
      nodes: [
        { id: 'net', label: 'Edge' },
        { id: 'dc', label: 'DC', parent: 'net' },
        { id: 'dns', label: 'DNS', parent: 'dc', kind: 'icon' },
        { id: 'early', label: 'early', parent: 'net' },
        { id: 'client', label: 'Client' },
      ],
      edges: [],
      steps: [{ title: 'a', reveal: ['early'] }, { title: 'b', reveal: ['net'] }, { title: 'c', flows: [{ from: 'client', to: 'dns' }] }],
    };
    // 'early' is revealed at step 0 but its container only at step 1.
    expect([...frameAt(d, 0).visible].sort()).toEqual(['client']);
    expect([...frameAt(d, 1).visible].sort()).toEqual(['client', 'dc', 'dns', 'early', 'net']);
    const d2: Demo = { ...d, steps: [{ title: 'x', reveal: ['net'] }, { title: 'y', flows: [{ from: 'client', to: 'dns' }] }] };
    d2.steps.unshift({ title: 'w', flows: [{ from: 'client', to: 'dns' }] });
    // A flow into a hidden container shows the container chain for that step.
    expect(frameAt(d2, 0).visible.has('net')).toBe(true);
  });

  it('turns edges that point back up the node order around for layout', () => {
    const order = ['client', 'dc1', 'dc2', 'dc3', 'origin'];
    const edges = [
      { from: 'dc1', to: 'dc3' },
      { from: 'dc3', to: 'dc1' },
      { from: 'origin', to: 'dc3' },
      { from: 'client', to: 'dc2' },
    ];
    expect(forwardEdges(order, edges)).toEqual([
      { source: 'dc1', target: 'dc3' },
      { source: 'dc1', target: 'dc3' },
      { source: 'dc3', target: 'origin' },
      { source: 'client', target: 'dc2' },
    ]);
  });

  it('puts route labels on the longest straight stretch', () => {
    const pts = [{ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 10, y: 100 }, { x: 30, y: 100 }];
    expect(labelAnchor(pts)).toEqual({ x: 10, y: 50, vertical: true });
    expect(labelAnchor([{ x: 0, y: 5 }, { x: 80, y: 5 }])).toEqual({ x: 40, y: 5, vertical: false });
    expect(pathD(pts)).toBe('M0,0 L10,0 L10,100 L30,100');
  });

  it('finds points along a route by distance', () => {
    const pts = [{ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 10, y: 30 }];
    expect(pointAlong(pts, 0)).toEqual({ x: 0, y: 0 });
    expect(pointAlong(pts, 0.5)).toEqual({ x: 10, y: 10 });
    expect(pointAlong(pts, 1)).toEqual({ x: 10, y: 30 });
  });

  it('places labels clear of each other and of boxes', () => {
    const line = [{ x: 0, y: 100 }, { x: 200, y: 100 }];
    const spots = placeLabels(
      [
        { id: 'a', pts: line, w: 60, h: 20 },
        { id: 'b', pts: line, w: 60, h: 20 },
        { id: 'c', pts: line, w: 60, h: 20, below: true },
      ],
      [],
    );
    // First takes the middle above, second moves (below), third prefers below but it's taken.
    expect(spots.get('a')).toEqual({ x: 70, y: 72 });
    expect(spots.get('b')).toEqual({ x: 70, y: 108 });
    const c = spots.get('c')!;
    expect([c.x, c.y]).not.toEqual([70, 108]);
    // A box over the middle pushes the label aside.
    const s2 = placeLabels([{ id: 'x', pts: line, w: 40, h: 20 }], [{ x: 80, y: 60, w: 40, h: 80 }]);
    const x = s2.get('x')!;
    expect(x.x + 40 <= 80 || x.x >= 120).toBe(true);
  });
});
