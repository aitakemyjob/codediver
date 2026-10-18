// The demo and animation views end to end in jsdom, plus PDF composition
// with a stand-in 2D canvas (jsdom has no canvas or image decoding).
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { Animation, Demo } from '../../lib/types';
import { useStore } from '../../lib/store';

const api = vi.hoisted(() => ({ renderResult: vi.fn(), saveFile: vi.fn(), read: vi.fn(), setContext: vi.fn() }));
vi.mock('../../lib/api', () => ({ api }));

import DemoView from './DemoView';
import AnimationView from './AnimationView';
import { renderPdf, renderStepPng } from './export';

const demo: Demo = {
  id: 'demo1', repo: 'r', title: 'Checkout', summary: 'One **order**, end to end.', layout: 'architecture', createdAt: '',
  nodes: [
    { id: 'net', label: 'Edge', kind: 'region' },
    { id: 'api', label: 'API Worker', parent: 'net' },
    { id: 'db', label: 'Postgres', kind: 'datastore' },
  ],
  edges: [{ from: 'api', to: 'db', label: 'INSERT' }],
  steps: [
    { title: 'Request arrives', narration: 'The **API Worker** validates it.', focus: ['api'] },
    { title: 'Write the order', flows: [{ from: 'api', to: 'db', label: 'INSERT' }], values: { db: 'order #1007' } },
  ],
};
const anim: Animation = { id: 'anim1', repo: 'r', title: 'Bytes', format: 'html', source: '<p>hi</p>', caption: 'A **caption**', createdAt: '' };

beforeEach(() => {
  useStore.setState((s) => ({
    app: { canSaveFile: true } as never,
    session: { ...s.session, demos: [demo], animations: [anim] },
  }));
  api.saveFile.mockReset().mockResolvedValue({ path: '/home/me/file' });
});

// ---- a stand-in canvas ----------------------------------------------------------

const drawn: string[] = [];
beforeAll(() => {
  // jsdom has no layout, so nothing ever resizes.
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as never;
  const ctx = {
    scale: () => {},
    fillRect: () => {},
    drawImage: () => {},
    fillText: (t: string) => drawn.push(t),
    measureText: (t: string) => ({ width: t.length * 6 }),
    set fillStyle(_: string) {},
    set font(_: string) {},
    set textBaseline(_: string) {},
  };
  HTMLCanvasElement.prototype.getContext = (() => ctx) as never;
  HTMLCanvasElement.prototype.toBlob = function (cb: BlobCallback, type?: string) {
    cb(new Blob([type === 'image/jpeg' ? '\xff\xd8jpeg\xff\xd9' : 'png'], { type }));
  };
  HTMLImageElement.prototype.decode = () => Promise.resolve();
});

describe('PNG and PDF export', () => {
  it('rasterizes a step at 2×', async () => {
    const r = await renderStepPng(demo, 1, 'light');
    expect(r.blob.type).toBe('image/png');
    expect(r.width).toBeGreaterThan(200);
  });

  it('builds a cover page plus one page per step', async () => {
    drawn.length = 0;
    const progress: number[] = [];
    const pdf = await renderPdf(demo, 'light', (done) => progress.push(done));
    const text = new TextDecoder('latin1').decode(await pdf.arrayBuffer());
    expect(pdf.type).toBe('application/pdf');
    expect(text).toContain('/Count 3');
    expect(text).toContain('/Title (Checkout)');
    expect(progress).toEqual([1, 2, 3]);
    // Cover: title, plain-text summary and the step list; then each step.
    for (const t of ['Checkout', 'One order, end to end.', '1.  Request arrives', 'STEP 2 OF 2', 'Write the order', 'The API Worker validates it.']) {
      expect(drawn.some((d) => d.includes(t)), t).toBe(true);
    }
    expect(drawn.filter((d) => d === 'Checkout · CodeDiver').length).toBe(3);
  });
});

// ---- the demo view ---------------------------------------------------------------

describe('DemoView', () => {
  const view = () => render(<DemoView tab={{ id: 't', kind: 'demo', title: 'Checkout', params: { id: 'demo1' } }} active />);

  it('plays steps with narration, values and controls', async () => {
    view();
    expect(await screen.findByText('Request arrives')).toBeInTheDocument();
    expect(screen.getByText('order', { exact: false })).toBeInTheDocument(); // summary on step 1
    await waitFor(() => expect(document.querySelector('.demo-svg')).not.toBeNull());
    fireEvent.click(screen.getByTitle('Next step (→)'));
    expect(screen.getByText('Write the order')).toBeInTheDocument();
    expect(screen.getAllByText('order #1007').length).toBe(2); // on the diagram and in the panel
    fireEvent.keyDown(window, { key: 'ArrowLeft' });
    expect(screen.getByText('Request arrives')).toBeInTheDocument();
    fireEvent.click(screen.getByText('2×'));
    expect(screen.getByText('2×').className).toContain('on');
  });

  it('switches the camera between follow and manual', async () => {
    view();
    await waitFor(() => expect(document.querySelector('.demo-svg')).not.toBeNull());
    const follow = screen.getByTitle(/Follow the action/);
    expect(follow.className).toContain('active');
    fireEvent.click(screen.getByTitle('Zoom in (+)'));
    expect(follow.className).not.toContain('active');
    fireEvent.click(follow);
    expect(follow.className).toContain('active');
    fireEvent.keyDown(window, { key: '0' });
    expect(follow.className).not.toContain('active');
    fireEvent.keyDown(window, { key: 'f' });
    expect(follow.className).toContain('active');
  });

  it('exports through the menu and reports where it went', async () => {
    view();
    await waitFor(() => expect(document.querySelector('.demo-svg')).not.toBeNull());
    fireEvent.click(screen.getByTitle('Export as an image or PDF'));
    expect(screen.getAllByRole('menuitem').map((m) => m.textContent)).toEqual([
      expect.stringContaining('PNG image'), expect.stringContaining('SVG vector'), expect.stringContaining('PDF document'), expect.stringContaining('Light theme'),
    ]);
    await act(async () => fireEvent.click(screen.getByText('SVG vector')));
    await waitFor(() => expect(api.saveFile).toHaveBeenCalled());
    const [name, data] = api.saveFile.mock.calls[0];
    expect(name).toBe('checkout-step-1.svg');
    expect(atob(data)).toContain('<svg');
    await waitFor(() => expect(useStore.getState().toasts.some((t) => t.title === 'Exported SVG' && t.body === '/home/me/file')).toBe(true));
  });

  it('says when a demo is gone', () => {
    render(<DemoView tab={{ id: 't', kind: 'demo', title: 'x', params: { id: 'nope' } }} active />);
    expect(screen.getByText('Demo not found')).toBeInTheDocument();
  });
});

// ---- the animation view --------------------------------------------------------

describe('AnimationView', () => {
  const view = (id = 'anim1') => render(<AnimationView tab={{ id: 't', kind: 'animation', title: 'Bytes', params: { id } }} active />);

  it('runs the animation sandboxed and saves it as HTML', async () => {
    view();
    const frame = document.querySelector('iframe')!;
    expect(frame.getAttribute('sandbox')).toBe('allow-scripts');
    expect(frame.getAttribute('srcdoc')).toContain('Content-Security-Policy');
    expect(screen.getByText('caption')).toBeInTheDocument();
    await act(async () => fireEvent.click(screen.getByText('Save HTML')));
    await waitFor(() => expect(api.saveFile).toHaveBeenCalled());
    const [name, data] = api.saveFile.mock.calls[0];
    expect(name).toBe('bytes.html');
    const html = atob(data);
    expect(html).toContain("default-src 'none'");
    expect(html).toContain('<p>hi</p>');
  });

  it('zooms through the toolbar and shows the source', () => {
    view();
    const frame = document.querySelector('iframe')!;
    const post = vi.spyOn(frame.contentWindow!, 'postMessage');
    fireEvent.click(screen.getByTitle('Zoom in (Ctrl+wheel)'));
    expect(post).toHaveBeenCalledWith({ codediverZoom: 1.25 }, '*');
    // The animation reports its zoom back; only its own frame is trusted.
    act(() => window.dispatchEvent(new MessageEvent('message', { data: { codediverZoom: 1.5 }, source: frame.contentWindow })));
    expect(screen.getByTitle('Reset zoom').textContent).toBe('150%');
    act(() => window.dispatchEvent(new MessageEvent('message', { data: { codediverZoom: 3 }, source: window })));
    expect(screen.getByTitle('Reset zoom').textContent).toBe('150%');
    fireEvent.click(screen.getByText('Source'));
    expect(document.querySelector('.anim-source')!.textContent).toBe('<p>hi</p>');
  });

  it('says when an animation is gone', () => {
    view('nope');
    expect(screen.getByText('Animation not found')).toBeInTheDocument();
  });
});
