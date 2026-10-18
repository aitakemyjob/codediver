import { render } from '@testing-library/react';
import type { Demo } from '../../lib/types';
import { useStore } from '../../lib/store';
import { groupsOf, frameAt } from './demo';
import { buildScene } from './scene';
import { SceneArt } from './SceneArt';

const api = vi.hoisted(() => ({ renderResult: vi.fn(), saveFile: vi.fn() }));
vi.mock('../../lib/api', () => ({ api }));

import { answerRenderRequest, bakeStyles, blobBase64, fileName, plainText, renderStepSvg, saveFile } from './export';

const demo: Demo = {
  id: 'd1', repo: 'r', title: 'Order flow', layout: 'architecture', createdAt: '',
  nodes: [
    { id: 'net', label: 'Edge', kind: 'region', icon: 'globe' },
    { id: 'api', label: 'API Worker', parent: 'net', icon: 'api', kind: 'service' },
    { id: 'db', label: 'Postgres', kind: 'icon', icon: 'database' },
    { id: 'late', label: 'Revealed later' },
  ],
  edges: [{ from: 'api', to: 'db', label: 'INSERT', detail: 'order #1007', color: 'green' }],
  steps: [
    { title: 'Write', focus: ['api'], flows: [{ from: 'api', to: 'db', label: 'INSERT', kind: 'data' }], values: { net: 'HIT', db: 'order pending' } },
    { title: 'Later', reveal: ['late'] },
  ],
};

beforeEach(() => {
  api.renderResult.mockReset().mockResolvedValue(undefined);
  api.saveFile.mockReset();
});

describe('export helpers', () => {
  it('reduces markdown to plain text for print', () => {
    expect(plainText('**Bold** and `code`, a [link](http://x) and _em_.\n\n\n\n## Head')).toBe('Bold and code, a link and em.\n\nHead');
    expect(plainText('before\n```go\nx := 1\n```\nafter')).toBe('before\n\nafter');
  });

  it('makes file names from titles', () => {
    expect(fileName('A production app on Cloudflare', 'pdf')).toBe('a-production-app-on-cloudflare.pdf');
    expect(fileName('  ***  ', 'png')).toBe('demo.png');
    expect(fileName('x'.repeat(100), 'svg')).toBe('x'.repeat(60) + '.svg');
  });

  it('bakes styles in, drops hidden parts and app classes', () => {
    const host = document.createElement('div');
    host.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" class="demo-svg">
      <g class="demo-node"><rect style="fill: rgb(1, 2, 3)"/><text style="text-transform: uppercase">service</text></g>
      <g class="demo-node hidden" style="opacity: 0"><rect/></g>
    </svg>`;
    document.body.appendChild(host);
    const out = bakeStyles(host.querySelector('svg')!);
    host.remove();
    expect(out.querySelectorAll('[class]').length).toBe(0);
    expect(out.querySelectorAll('g').length).toBe(1);
    expect(out.querySelector('rect')!.getAttribute('style')).toContain('fill:rgb(1, 2, 3)');
    expect(out.querySelector('text')!.textContent).toBe('SERVICE');
  });

  it('encodes blobs as base64', async () => {
    expect(await blobBase64(new Blob(['hi']))).toBe('aGk=');
  });
});

describe('renderStepSvg', () => {
  it('renders a standalone SVG of the whole scene at a step', async () => {
    const r = await renderStepSvg(demo, 0, 'light');
    expect(r.svg.startsWith('<?xml')).toBe(true);
    expect(r.svg).toContain('xmlns="http://www.w3.org/2000/svg"');
    expect(r.svg).not.toContain('class=');
    expect(r.width).toBeGreaterThan(100);
    expect(r.svg).toContain(`width="${r.width}"`);
    // Labels, values and the packet label are all in it; packets are frozen.
    for (const t of ['API Worker', 'Postgres', 'pending', 'INSERT', 'HIT']) expect(r.svg).toContain(t);
    expect(r.svg).not.toContain('animateMotion');
    // Nothing is left mounted.
    expect(document.querySelectorAll('svg').length).toBe(0);
  });

  it('frames only what is visible at that step', async () => {
    const before = await renderStepSvg(demo, 0, 'current');
    const after = await renderStepSvg(demo, 1, 'current');
    expect(after.width * after.height).toBeGreaterThanOrEqual(before.width * before.height);
    expect(after.svg).toContain('Revealed later');
  });
});

describe('answering agent render requests', () => {
  it('posts an SVG back', async () => {
    await answerRenderRequest({ id: 'render1', demo, step: 0, format: 'svg', theme: 'light' });
    const r = api.renderResult.mock.calls[0][0];
    expect(r.id).toBe('render1');
    expect(r.data).toContain('<svg');
    expect(r.error).toBeUndefined();
  });

  it('posts the error back when rendering fails', async () => {
    // jsdom has no image decoding, so rasterizing a PNG fails here.
    await answerRenderRequest({ id: 'render2', demo, step: 0, format: 'png', theme: 'light' });
    const r = api.renderResult.mock.calls[0][0];
    expect(r.id).toBe('render2');
    expect(r.data).toBe('');
    expect(r.error).toBeTruthy();
  });
});

describe('saveFile', () => {
  const app = (canSaveFile: boolean) => useStore.setState({ app: { canSaveFile } as never });

  it('downloads in the browser', async () => {
    app(false);
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    URL.createObjectURL = vi.fn(() => 'blob:x');
    URL.revokeObjectURL = vi.fn();
    expect(await saveFile('a.svg', new Blob(['<svg/>']))).toBe('a.svg');
    expect(click).toHaveBeenCalledTimes(1);
    expect(api.saveFile).not.toHaveBeenCalled();
    click.mockRestore();
  });

  it('uses the native dialog in the desktop app', async () => {
    app(true);
    api.saveFile.mockResolvedValue({ path: '/home/me/a.png' });
    expect(await saveFile('a.png', new Blob(['hi']))).toBe('/home/me/a.png');
    expect(api.saveFile).toHaveBeenCalledWith('a.png', 'aGk=');
  });
});

describe('SceneArt', () => {
  it('draws a still snapshot with packets at their destination', async () => {
    const scene = await buildScene(demo, groupsOf(demo));
    const frame = frameAt(demo, 0);
    const cam = { x: -50, y: -50, w: 1200, h: 600 };
    const props = { demo, frame, boxes: scene.boxes, routes: scene.routes, groups: groupsOf(demo), step: 0, speed: 1, camera: cam };
    const still = render(<SceneArt {...props} still size={{ w: 1200, h: 600 }} />);
    const svg = still.container.querySelector('svg')!;
    expect(svg.getAttribute('viewBox')).toBe('-50 -50 1200 600');
    expect(svg.querySelector('animateMotion')).toBeNull();
    const dot = svg.querySelector('.demo-flow circle')!;
    expect(Number(dot.getAttribute('cx'))).toBeGreaterThan(0);
    // Containers, leaves and tiles each draw once; the late node is hidden.
    expect(svg.querySelectorAll('.demo-box').length).toBe(1);
    expect(svg.querySelectorAll('.demo-node').length).toBe(3);
    expect(svg.querySelector('.demo-node.hidden')!.textContent).toContain('Revealed later');
    // The edge's own label gives way to the packet's while it runs.
    expect(svg.querySelectorAll('.edge-label').length).toBe(0);
    expect(svg.querySelector('.demo-flow-label')!.textContent).toBe('INSERT');
    // The container's value is a badge with a solid backing.
    expect(svg.querySelector('.demo-box .demo-value rect.backing')).not.toBeNull();
    still.unmount();

    const live = render(<SceneArt {...props} />);
    expect(live.container.querySelector('animateMotion')).not.toBeNull();
    live.unmount();
  });

  it('shows edge labels when no packet is on the edge', async () => {
    const quiet: Demo = { ...demo, steps: [{ title: 'quiet' }] };
    const scene = await buildScene(quiet, groupsOf(quiet));
    const { container } = render(
      <SceneArt demo={quiet} frame={frameAt(quiet, 0)} boxes={scene.boxes} routes={scene.routes} groups={[]} step={0} speed={1} camera={{ x: 0, y: 0, w: 900, h: 500 }} still />,
    );
    const label = container.querySelector('.edge-label')!;
    expect(label.textContent).toBe('INSERTorder #1007');
  });
});
