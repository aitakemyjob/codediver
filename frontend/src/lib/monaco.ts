// Monaco setup: lean bundle (see scripts/gen-monaco.mjs), a single editor
// worker, and editor themes generated from CodeDiver palettes.
import * as monaco from './monaco.generated';
import EditorWorker from '@monaco/editor/editor.worker.js?worker';
import type { Theme } from './themes';
import { hexA as alpha, mix } from './themes';

(self as unknown as { MonacoEnvironment: unknown }).MonacoEnvironment = {
  /** Every language uses the single generic editor worker. */
  getWorker: () => new EditorWorker(),
};

/** Strips the leading # for Monaco token colors. */
const strip = (c: string) => c.replace('#', '');

/**
 * Registers a Monaco theme derived from a CodeDiver palette and returns
 * its name.
 */
export function defineMonacoTheme(t: Theme): string {
  const name = `gd-${t.id}`;
  const s = t.syntax;
  monaco.editor.defineTheme(name, {
    base: t.mode === 'dark' ? 'vs-dark' : 'vs',
    inherit: true,
    rules: [
      { token: '', foreground: strip(s.variable) },
      { token: 'comment', foreground: strip(s.comment), fontStyle: 'italic' },
      { token: 'keyword', foreground: strip(s.keyword) },
      { token: 'string', foreground: strip(s.string) },
      { token: 'string.escape', foreground: strip(s.operator) },
      { token: 'number', foreground: strip(s.number) },
      { token: 'type', foreground: strip(s.type) },
      { token: 'type.identifier', foreground: strip(s.type) },
      { token: 'identifier', foreground: strip(s.variable) },
      { token: 'delimiter', foreground: strip(t.text2) },
      { token: 'operator', foreground: strip(s.operator) },
      { token: 'predefined', foreground: strip(s.func) },
      { token: 'attribute.name', foreground: strip(s.func) },
      { token: 'tag', foreground: strip(s.keyword) },
      { token: 'metatag', foreground: strip(s.type) },
      { token: 'regexp', foreground: strip(s.string) },
    ],
    colors: {
      'editor.background': t.bg1,
      'editor.foreground': t.text,
      'editorLineNumber.foreground': t.text3,
      'editorLineNumber.activeForeground': t.text,
      'editor.lineHighlightBackground': alpha(t.text, t.mode === 'dark' ? 0.035 : 0.04),
      'editor.lineHighlightBorder': '#00000000',
      'editor.selectionBackground': alpha(t.accent, 0.28),
      'editor.inactiveSelectionBackground': alpha(t.accent, 0.14),
      'editor.findMatchBackground': alpha(t.warn, 0.4),
      'editor.findMatchHighlightBackground': alpha(t.warn, 0.18),
      'editorCursor.foreground': t.accent,
      'editorWhitespace.foreground': alpha(t.text3, 0.3),
      'editorIndentGuide.background1': alpha(t.text3, 0.18),
      'editorIndentGuide.activeBackground1': alpha(t.text3, 0.45),
      'editorBracketMatch.background': alpha(t.accent, 0.15),
      'editorBracketMatch.border': alpha(t.accent, 0.5),
      'editorGutter.background': t.bg1,
      'editorCodeLens.foreground': t.text3,
      'editorWidget.background': t.bg2,
      'editorWidget.border': t.borderStrong,
      'editorHoverWidget.background': t.bg2,
      'editorHoverWidget.border': t.borderStrong,
      'editorSuggestWidget.background': t.bg2,
      'scrollbarSlider.background': alpha(t.text3, 0.18),
      'scrollbarSlider.hoverBackground': alpha(t.text3, 0.3),
      'scrollbarSlider.activeBackground': alpha(t.text3, 0.4),
      'minimap.background': t.bg1,
      'diffEditor.insertedTextBackground': alpha(t.ok, 0.16),
      'diffEditor.removedTextBackground': alpha(t.err, 0.16),
      'diffEditor.insertedLineBackground': alpha(t.ok, 0.08),
      'diffEditor.removedLineBackground': alpha(t.err, 0.08),
      'diffEditorGutter.insertedLineBackground': alpha(t.ok, 0.18),
      'diffEditorGutter.removedLineBackground': alpha(t.err, 0.18),
      'diffEditor.diagonalFill': alpha(t.text3, 0.12),
      'editorOverviewRuler.border': '#00000000',
      'focusBorder': alpha(t.accent, 0.5),
      'input.background': t.bg2,
      'peekViewEditor.background': mix(t.bg1, t.bg2, 0.5),
    },
  });
  return name;
}

/** Applies a CodeDiver palette to every Monaco editor. */
export function setMonacoTheme(t: Theme) {
  monaco.editor.setTheme(defineMonacoTheme(t));
}

export { monaco };
