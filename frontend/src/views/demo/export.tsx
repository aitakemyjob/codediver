// Exports demo steps as SVG, PNG and PDF, for the Export menu and for
// agents (export_demo asks an open window to render through here).
//
// A step is drawn offscreen with the same SceneArt the player uses, in
// `still` mode and framed to the whole scene, then every element's computed
// styles are baked into the SVG so it looks the same outside CodeDiver.
import { flushSync } from 'react-dom';
import { createRoot } from 'react-dom/client';
import { api } from '../../lib/api';
import { applyTheme, themeById } from '../../lib/themes';
import { useStore } from '../../lib/store';
import type { Demo } from '../../lib/types';
import { frameAt, groupsOf, type Rect } from './demo';
import { buildPdf, type PdfPage } from './pdf';
import { buildScene, LANE_HEAD, type Scene } from './scene';
import { SceneArt } from './SceneArt';

export type ExportTheme = 'light' | 'dark' | 'current';

export interface Rendered {
  svg: string;
  width: number;
  height: number;
}

/** Computed styles worth keeping when an SVG leaves the app. */
const STYLE_PROPS = [
  'fill', 'fill-opacity', 'stroke', 'stroke-width', 'stroke-dasharray', 'stroke-linejoin', 'stroke-linecap', 'stroke-opacity',
  'opacity', 'color', 'font-family', 'font-size', 'font-weight', 'font-style', 'letter-spacing', 'text-anchor', 'dominant-baseline',
];

/** The theme an export draws with: light uses Paper, dark uses Deep Dive. */
function themeFor(t: ExportTheme) {
  return t === 'light' ? themeById('paper') : t === 'dark' ? themeById('deepdive') : undefined;
}

/** The whole scene at a step, padded: every visible node and its value tags. */
function frameAll(scene: Scene, visible: Set<string>, lanes: boolean): Rect {
  const shown = [...visible].map((id) => scene.boxes.get(id)).filter((b) => !!b);
  if (shown.length === 0) return { x: 0, y: 0, w: 400, h: 300 };
  const top = lanes ? LANE_HEAD + 8 : 12;
  const x0 = Math.min(...shown.map((b) => b.x)) - 32;
  const y0 = Math.min(...shown.map((b) => b.y)) - top - 20;
  const x1 = Math.max(...shown.map((b) => b.x + b.w)) + 32;
  const y1 = Math.max(...shown.map((b) => b.y + b.h + b.foot)) + 28;
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}

/**
 * Renders one step of a demo to a standalone SVG. Pass a scene to reuse a
 * layout (the PDF renders every step of one layout).
 */
export async function renderStepSvg(demo: Demo, step: number, theme: ExportTheme, scene?: Scene): Promise<Rendered> {
  const groups = groupsOf(demo);
  scene ??= await buildScene(demo, groups);
  const frame = frameAt(demo, step);
  const lanes = demo.layout === 'architecture' && groups.length > 1 && !demo.nodes.some((n) => n.parent);
  const camera = frameAll(scene, frame.visible, lanes);
  const size = { w: Math.round(camera.w), h: Math.round(camera.h) };

  // Offscreen, but in the document so styles (and the theme) apply.
  const host = document.createElement('div');
  host.style.cssText = 'position:fixed;left:-100000px;top:0;pointer-events:none';
  const t = themeFor(theme);
  if (t) applyTheme(t, host);
  document.body.appendChild(host);
  const root = createRoot(host);
  try {
    flushSync(() =>
      root.render(<SceneArt demo={demo} frame={frame} boxes={scene.boxes} routes={scene.routes} groups={groups} step={step} speed={1} camera={camera} still size={size} />),
    );
    const svg = host.querySelector('svg');
    if (!svg) throw new Error('nothing was drawn');
    const out = bakeStyles(svg);
    // A solid background so the image reads the same anywhere.
    const bg = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
    Object.entries({ x: camera.x, y: camera.y, width: camera.w, height: camera.h }).forEach(([k, v]) => bg.setAttribute(k, String(v)));
    bg.setAttribute('fill', getComputedStyle(host).getPropertyValue('--bg').trim() || '#ffffff');
    out.insertBefore(bg, out.firstChild);
    out.setAttribute('font-family', getComputedStyle(svg).fontFamily);
    return { svg: '<?xml version="1.0" encoding="UTF-8"?>\n' + new XMLSerializer().serializeToString(out), width: size.w, height: size.h };
  } finally {
    root.unmount();
    host.remove();
  }
}

/**
 * Copies an SVG with each element's computed styles inlined, dropping what
 * isn't visible and the app's classes (they mean nothing outside it).
 */
export function bakeStyles(src: SVGSVGElement): SVGSVGElement {
  const out = src.cloneNode(true) as SVGSVGElement;
  const walk = (a: Element, b: Element) => {
    const cs = getComputedStyle(a);
    if (b !== out && (cs.display === 'none' || cs.opacity === '0' || cs.visibility === 'hidden')) {
      b.remove();
      return;
    }
    const style = STYLE_PROPS.map((p) => [p, cs.getPropertyValue(p)] as const).filter(([, v]) => v && v !== 'normal' && v !== 'auto');
    b.setAttribute('style', style.map(([p, v]) => `${p}:${v}`).join(';'));
    b.removeAttribute('class');
    // text-transform isn't reliable in other SVG renderers; apply it.
    if (a.tagName === 'text' && cs.textTransform === 'uppercase') b.textContent = (a.textContent ?? '').toUpperCase();
    const ak = [...a.children];
    const bk = [...b.children];
    ak.forEach((c, i) => bk[i] && walk(c, bk[i]));
  };
  walk(src, out);
  return out;
}

/** Rasterizes an SVG at `scale` (capped so canvases stay within limits). */
export async function svgToCanvas(r: Rendered, scale = 2): Promise<HTMLCanvasElement> {
  const k = Math.min(scale, 8000 / Math.max(r.width, r.height));
  const img = new Image();
  img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(r.svg);
  await img.decode();
  const c = document.createElement('canvas');
  c.width = Math.round(r.width * k);
  c.height = Math.round(r.height * k);
  c.getContext('2d')!.drawImage(img, 0, 0, c.width, c.height);
  return c;
}

/** A canvas as encoded image bytes. */
function canvasBytes(c: HTMLCanvasElement, type: 'image/png' | 'image/jpeg', quality?: number): Promise<Blob> {
  return new Promise((resolve, reject) => c.toBlob((b) => (b ? resolve(b) : reject(new Error('could not encode the image'))), type, quality));
}

/** One step as a PNG. */
export async function renderStepPng(demo: Demo, step: number, theme: ExportTheme, scale = 2): Promise<{ blob: Blob; width: number; height: number }> {
  const c = await svgToCanvas(await renderStepSvg(demo, step, theme), scale);
  return { blob: await canvasBytes(c, 'image/png'), width: c.width, height: c.height };
}

// ---- PDF -----------------------------------------------------------------------

const PAGE = { w: 842, h: 595 }; // A4 landscape, points
const PX = 2.5; // canvas pixels per point

/** Markdown reduced to plain text for print. */
export function plainText(md: string): string {
  return md
    .replace(/```[\s\S]*?```/g, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/(\*\*|__|`|\*|_)(.+?)\1/g, '$2')
    .replace(/^#+\s*/gm, '')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

/** Draws wrapped text; returns the y below it. */
function paragraph(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, maxW: number, lineH: number, maxY = Infinity): number {
  for (const para of text.split(/\n+/)) {
    let line = '';
    for (const word of para.split(/\s+/).filter(Boolean)) {
      const next = line ? `${line} ${word}` : word;
      if (ctx.measureText(next).width > maxW && line) {
        if (y + lineH > maxY) return y;
        ctx.fillText(line, x, y);
        y += lineH;
        line = word;
      } else line = next;
    }
    if (line && y + lineH <= maxY) {
      ctx.fillText(line, x, y);
      y += lineH;
    }
    y += lineH * 0.4;
  }
  return y;
}

/**
 * The whole demo as a PDF: a cover page with the summary and the list of
 * steps, then one page per step with its diagram and narration.
 */
export async function renderPdf(demo: Demo, theme: ExportTheme, onProgress?: (done: number, total: number) => void): Promise<Blob> {
  const scene = await buildScene(demo, groupsOf(demo));
  const t = themeFor(theme);
  const css = getComputedStyle(document.documentElement);
  const color = (v: keyof NonNullable<typeof t>, varName: string) => (t ? (t[v] as string) : css.getPropertyValue(varName).trim());
  const ink = { bg: color('bg', '--bg'), text: color('text', '--text'), text2: color('text2', '--text-2'), text3: color('text3', '--text-3'), accent: color('accent', '--accent') };
  const font = css.getPropertyValue('--font-ui').trim() || 'sans-serif';
  const pages: PdfPage[] = [];
  const M = 40; // margin, points

  const page = async (draw: (ctx: CanvasRenderingContext2D) => Promise<void> | void) => {
    const c = document.createElement('canvas');
    c.width = PAGE.w * PX;
    c.height = PAGE.h * PX;
    const ctx = c.getContext('2d')!;
    ctx.scale(PX, PX);
    ctx.fillStyle = ink.bg;
    ctx.fillRect(0, 0, PAGE.w, PAGE.h);
    ctx.textBaseline = 'top';
    await draw(ctx);
    // Footer on every page.
    ctx.font = `10px ${font}`;
    ctx.fillStyle = ink.text3;
    ctx.fillText(`${demo.title} · CodeDiver`, M, PAGE.h - 24);
    const n = `${pages.length + 1}`;
    ctx.fillText(n, PAGE.w - M - ctx.measureText(n).width, PAGE.h - 24);
    const blob = await canvasBytes(c, 'image/jpeg', 0.92);
    pages.push({ jpeg: new Uint8Array(await blob.arrayBuffer()), width: c.width, height: c.height });
  };

  const total = demo.steps.length + 1;
  await page((ctx) => {
    ctx.fillStyle = ink.accent;
    ctx.font = `600 11px ${font}`;
    ctx.fillText('WALKTHROUGH', M, M + 30);
    ctx.fillStyle = ink.text;
    ctx.font = `700 28px ${font}`;
    let y = paragraph(ctx, demo.title, M, M + 50, PAGE.w - 2 * M, 34);
    ctx.font = `13px ${font}`;
    ctx.fillStyle = ink.text2;
    if (demo.summary) y = paragraph(ctx, plainText(demo.summary), M, y + 8, PAGE.w - 2 * M, 19) + 10;
    ctx.font = `600 11px ${font}`;
    ctx.fillStyle = ink.text3;
    ctx.fillText(`${demo.steps.length} STEPS`, M, y + 6);
    ctx.font = `12.5px ${font}`;
    ctx.fillStyle = ink.text;
    // Two columns when the list is long.
    const perCol = Math.max(1, Math.floor((PAGE.h - y - 80) / 19));
    demo.steps.forEach((st, i) => {
      const col = Math.floor(i / perCol);
      ctx.fillText(`${i + 1}.  ${st.title}`, M + col * ((PAGE.w - 2 * M) / 2), y + 28 + (i % perCol) * 19);
    });
  });
  onProgress?.(1, total);

  for (let i = 0; i < demo.steps.length; i++) {
    const st = demo.steps[i];
    const art = await svgToCanvas(await renderStepSvg(demo, i, theme, scene), PX * 1.5);
    await page((ctx) => {
      ctx.font = `600 10px ${font}`;
      ctx.fillStyle = ink.text3;
      ctx.fillText(`STEP ${i + 1} OF ${demo.steps.length}`, M, M - 8);
      ctx.fillStyle = ink.text;
      ctx.font = `700 18px ${font}`;
      ctx.fillText(st.title, M, M + 8);
      // Narration (and code reference) get the bottom of the page; the
      // diagram fills what's left.
      ctx.font = `12px ${font}`;
      const narration = plainText(st.narration ?? '') + (st.code ? `\n${st.code.file}${st.code.line ? `:${st.code.line}` : ''}` : '');
      const textTop = PAGE.h - 46 - Math.min(150, measureHeight(ctx, narration, PAGE.w - 2 * M, 17));
      const box = { x: M, y: M + 38, w: PAGE.w - 2 * M, h: textTop - (M + 38) - 14 };
      const k = Math.min(box.w / art.width, box.h / art.height);
      const w = art.width * k;
      const h = art.height * k;
      ctx.drawImage(art, box.x + (box.w - w) / 2, box.y + (box.h - h) / 2, w, h);
      ctx.fillStyle = ink.text2;
      paragraph(ctx, narration, M, textTop, PAGE.w - 2 * M, 17, PAGE.h - 40);
    });
    onProgress?.(i + 2, total);
  }
  const bytes = buildPdf(pages, PAGE.w, PAGE.h, demo.title);
  return new Blob([bytes.buffer as ArrayBuffer], { type: 'application/pdf' });
}

/** Height a paragraph would take, without drawing it. */
function measureHeight(ctx: CanvasRenderingContext2D, text: string, maxW: number, lineH: number): number {
  if (!text) return 0;
  let lines = 0;
  for (const para of text.split(/\n+/)) {
    let line = '';
    lines++;
    for (const word of para.split(/\s+/).filter(Boolean)) {
      const next = line ? `${line} ${word}` : word;
      if (ctx.measureText(next).width > maxW && line) {
        lines++;
        line = word;
      } else line = next;
    }
  }
  return lines * lineH * 1.1;
}

// ---- saving ----------------------------------------------------------------------

/** A blob as base64 (no data: prefix). */
export function blobBase64(b: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result).replace(/^data:[^,]*,/, ''));
    r.onerror = () => reject(r.error);
    r.readAsDataURL(b);
  });
}

/**
 * Saves a file: through the native save dialog in the desktop app, or as
 * a browser download. Returns where it went ('' if cancelled).
 */
export async function saveFile(name: string, blob: Blob): Promise<string> {
  if (useStore.getState().app?.canSaveFile) {
    const { path } = await api.saveFile(name, await blobBase64(blob));
    return path;
  }
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
  return name;
}

/** A file name from a title: lowercase words joined by dashes. */
export function fileName(title: string, ext: string): string {
  const base = title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 60) || 'demo';
  return `${base}.${ext}`;
}

// ---- agent render requests --------------------------------------------------------

export interface RenderRequest {
  id: string;
  demo: Demo;
  step: number;
  format: 'png' | 'svg';
  theme: ExportTheme;
}

/** Renders what an agent asked for (export_demo) and posts it back. */
export async function answerRenderRequest(req: RenderRequest): Promise<void> {
  try {
    if (req.format === 'svg') {
      const r = await renderStepSvg(req.demo, req.step, req.theme);
      await api.renderResult({ id: req.id, data: r.svg, width: r.width, height: r.height });
    } else {
      const r = await renderStepPng(req.demo, req.step, req.theme);
      await api.renderResult({ id: req.id, data: await blobBase64(r.blob), width: r.width, height: r.height });
    }
  } catch (err) {
    await api.renderResult({ id: req.id, data: '', error: String((err as Error)?.message ?? err) }).catch(() => {});
  }
}
