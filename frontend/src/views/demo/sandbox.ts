// Builds the document an agent animation runs in. The iframe that hosts it
// uses sandbox="allow-scripts" (opaque origin: no cookies, storage, parent
// access or same-origin requests), and this document adds a CSP that blocks
// all network access, so nothing can be fetched or sent anywhere.
import type { Animation } from '../../lib/types';

export const ANIMATION_CSP =
  "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; media-src data: blob:; form-action 'none'; base-uri 'none'";

/** Theme tokens exposed to animations as CSS variables. */
export const THEME_VARS = ['--bg', '--bg-1', '--bg-2', '--bg-3', '--text', '--text-2', '--text-3', '--accent', '--accent-2', '--ok', '--warn', '--err', '--border', '--border-strong', '--font-ui', '--font-mono'];

/** Reads the theme tokens from computed styles into a :root rule. */
export function themeRule(style: Pick<CSSStyleDeclaration, 'getPropertyValue'>): string {
  const decls = THEME_VARS.map((v) => {
    const value = style.getPropertyValue(v).trim().replace(/[;{}<>]/g, '');
    return value ? `${v}:${value}` : '';
  }).filter(Boolean);
  return `:root{${decls.join(';')}}`;
}

/**
 * Lets CodeDiver zoom the animation: it accepts {codediverZoom} messages
 * from the parent only, handles Ctrl+wheel itself (the iframe swallows the
 * events), and reports the zoom back so the toolbar can show it.
 */
export const ZOOM_SCRIPT =
  "<script>(()=>{let z=1;const set=v=>{z=Math.min(4,Math.max(.25,v));document.documentElement.style.zoom=z;parent.postMessage({codediverZoom:z},'*')};" +
  "addEventListener('message',e=>{if(e.source===parent&&e.data&&typeof e.data.codediverZoom==='number')set(e.data.codediverZoom)});" +
  "addEventListener('wheel',e=>{if(e.ctrlKey||e.metaKey){e.preventDefault();set(z*Math.exp(-e.deltaY*.0015))}},{passive:false})})()</script>";

/**
 * Wraps an animation's source in a sandbox document. The CSP and theme are
 * always the very first thing parsed, before any agent markup, so even a
 * script placed ahead of the agent's own <head> runs under the policy.
 * Browsers fold the agent's duplicate <html>/<head> tags into ours.
 */
export function sandboxDocument(anim: Pick<Animation, 'format' | 'source'>, style: Pick<CSSStyleDeclaration, 'getPropertyValue'>): string {
  const head =
    `<meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${ANIMATION_CSP}">` +
    `<style>${themeRule(style)}html,body{margin:0;height:100%;background:var(--bg);color:var(--text);font-family:var(--font-ui),sans-serif}` +
    (anim.format === 'svg' ? 'body{display:grid;place-items:center}svg{max-width:100%;max-height:100%}' : '') +
    '</style>' +
    ZOOM_SCRIPT;
  const src = anim.source.replace(/^\s*<!doctype[^>]*>/i, '');
  return `<!doctype html><html><head>${head}</head>${/<(html|head|body)[\s>]/i.test(src) ? src : `<body>${src}</body>`}</html>`;
}
