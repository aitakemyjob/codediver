import { useEffect, useRef } from 'react';
import { monaco, setMonacoTheme } from '../lib/monaco';
import { useStore } from '../lib/store';
import { monacoLang } from '../views/CodeView';
import type { LineMark } from '../views/change/change';

let seq = 0;

const markClass = { hit: 'gd-cov-hit', part: 'gd-cov-part', miss: 'gd-cov-miss' } as const;

/** Monaco decorations for coverage marks on one side of the diff. */
function decorations(marks: LineMark[] | undefined, n: number): monaco.editor.IModelDeltaDecoration[] {
  return (marks ?? [])
    .filter((m) => m.line <= n)
    .map((m) => ({ range: new monaco.Range(m.line, 1, m.line, 1), options: { isWholeLine: true, linesDecorationsClassName: markClass[m.kind], linesDecorationsTooltip: m.hover, hoverMessage: { value: m.hover } } }));
}

/**
 * Side-by-side (or inline) Monaco diff of two texts. marks put coverage
 * markers in the gutter of the original or modified side.
 */
export default function DiffEditor({ original, modified, language, inline = false, revealLine, marks }: {
  original: string;
  modified: string;
  language: string;
  inline?: boolean;
  revealLine?: number;
  marks?: { original?: LineMark[]; modified?: LineMark[] };
}) {
  const host = useRef<HTMLDivElement>(null);
  const ed = useRef<monaco.editor.IStandaloneDiffEditor>(null);
  const decos = useRef<{ o?: monaco.editor.IEditorDecorationsCollection; m?: monaco.editor.IEditorDecorationsCollection }>({});
  const theme = useStore((s) => s.theme);
  const fontSize = useStore((s) => s.app?.config.settings.fontSize ?? 13);

  useEffect(() => {
    setMonacoTheme(useStore.getState().theme);
    const d = monaco.editor.createDiffEditor(host.current!, {
      readOnly: true,
      originalEditable: false,
      automaticLayout: true,
      renderSideBySide: !inline,
      useInlineViewWhenSpaceIsLimited: true,
      fontFamily: 'JetBrains Mono Variable, JetBrains Mono, ui-monospace, monospace',
      fontSize,
      lineHeight: 1.55,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      renderOverviewRuler: true,
      hideUnchangedRegions: { enabled: true, contextLineCount: 4, minimumLineCount: 6, revealLineCount: 20 },
      diffAlgorithm: 'advanced',
      padding: { top: 8 },
      unicodeHighlight: { ambiguousCharacters: false, invisibleCharacters: false },
    });
    ed.current = d;
    return () => {
      const m = d.getModel();
      d.dispose();
      m?.original.dispose();
      m?.modified.dispose();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const d = ed.current!;
    const id = ++seq;
    const lang = monacoLang(language);
    const prev = d.getModel();
    const o = monaco.editor.createModel(original, lang, monaco.Uri.parse(`codediver-diff://o/${id}`));
    const m = monaco.editor.createModel(modified, lang, monaco.Uri.parse(`codediver-diff://m/${id}`));
    d.setModel({ original: o, modified: m });
    prev?.original.dispose();
    prev?.modified.dispose();
    if (revealLine) setTimeout(() => d.getModifiedEditor().revealLineInCenter(revealLine), 60);
  }, [original, modified, language, revealLine]);

  useEffect(() => {
    const d = ed.current!;
    const o = d.getOriginalEditor(), m = d.getModifiedEditor();
    decos.current.o ??= o.createDecorationsCollection();
    decos.current.m ??= m.createDecorationsCollection();
    decos.current.o.set(decorations(marks?.original, o.getModel()?.getLineCount() ?? 0));
    decos.current.m.set(decorations(marks?.modified, m.getModel()?.getLineCount() ?? 0));
  }, [marks, original, modified, language]);

  useEffect(() => setMonacoTheme(theme), [theme]);
  useEffect(() => ed.current?.updateOptions({ renderSideBySide: !inline }), [inline]);
  useEffect(() => ed.current?.updateOptions({ fontSize }), [fontSize]);

  return <div ref={host} style={{ position: 'absolute', inset: 0 }} />;
}
