import type { Param, SymbolKind } from './types';

/** Last segment of a slash-separated path. */
export const basename = (p: string) => p.split('/').pop() || p;
/** Directory part of a slash-separated path ("" for top-level files). */
export const dirname = (p: string) => (p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '');

/** Human-friendly relative time ("3m ago"), falling back to a date. */
export function relTime(iso: string | number | Date, now = Date.now()): string {
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return '';
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 5) return 'just now';
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.round(h / 24);
  if (d < 30) return `${d}d ago`;
  return new Date(t).toLocaleDateString();
}

/** Compact number formatting: 950, 1.5k, 25k, 1.2M. */
export function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${Math.round(n / 1000)}k`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(Math.round(n));
}

/** Renders params as `name type, ...`. */
export function paramList(ps?: Param[] | null): string {
  return (ps ?? []).map((p) => [p.name, p.type].filter(Boolean).join(' ')).join(', ');
}

/** Renders param types only, parenthesized when there are several. */
export function shortType(ps?: Param[] | null): string {
  const list = (ps ?? []).map((p) => p.type || p.name);
  if (list.length === 0) return '';
  const s = list.join(', ');
  return list.length > 1 ? `(${s})` : s;
}

/** Short glyph + CSS color for a symbol kind. */
export function kindStyle(kind: SymbolKind | string | undefined): { glyph: string; color: string } {
  switch (kind) {
    case 'func':
      return { glyph: 'ƒ', color: 'var(--syn-func)' };
    case 'method':
      return { glyph: 'm', color: 'var(--accent)' };
    case 'struct':
    case 'class':
      return { glyph: 'C', color: 'var(--syn-type)' };
    case 'interface':
      return { glyph: 'I', color: 'var(--accent-2)' };
    case 'type':
      return { glyph: 'T', color: 'var(--syn-type)' };
    case 'external':
      return { glyph: '↗', color: 'var(--text-3)' };
    case 'module':
      return { glyph: '◇', color: 'var(--accent-2)' };
    case 'package':
      return { glyph: '▣', color: 'var(--accent)' };
    default:
      return { glyph: '•', color: 'var(--text-3)' };
  }
}

export const languageColor: Record<string, string> = {
  go: '#00add8', typescript: '#3178c6', javascript: '#f1e05a', python: '#3572a5', rust: '#dea584',
  java: '#b07219', kotlin: '#a97bff', csharp: '#178600', c: '#555555', cpp: '#f34b7d', ruby: '#701516',
  php: '#4f5d95', swift: '#f05138', scala: '#c22d40', dart: '#00b4ab', lua: '#000080',
};

/** Pluralizes a count: plural(2, 'file') → "2 files". */
export function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}
