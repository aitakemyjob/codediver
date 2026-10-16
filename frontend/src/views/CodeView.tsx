import { useEffect, useRef, useState } from 'react';
import { Bot, FlaskConical, GitFork, History, ListTree, Route, Workflow, X } from 'lucide-react';
import { FileSummaryPanel } from '../components/FileSummaryPanel';
import { monaco, setMonacoTheme } from '../lib/monaco';
import { api } from '../lib/api';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import type { Annotation, FileCoverage, FileSymbol, FuncQA } from '../lib/types';
import { ErrorBox, Spinner } from '../components/ui';
import type { ViewProps } from './ViewHost';

const langMap: Record<string, string> = {
  json: 'javascript', protobuf: 'proto', makefile: 'shell', groovy: 'java', 'go.mod': 'go', zig: 'rust', erlang: 'plaintext', cmake: 'plaintext',
};
/** Maps a CodeDiver language id to a Monaco language id. */
export const monacoLang = (l: string) => langMap[l] ?? l;

// One code-lens provider and command set for all editors; per-model symbols
// live in this registry.
const symbolsByModel = new Map<string, FileSymbol[]>();
// Per-model function test status (by symbol id), shown as a coverage lens.
const coverageByModel = new Map<string, Map<string, FuncQA>>();
let lensesRegistered = false;
const lensEmitter = new monaco.Emitter<monaco.languages.CodeLensProvider>();
let lensProvider: monaco.languages.CodeLensProvider | undefined;
/** Asks Monaco to re-request code lenses. */
const refreshLenses = () => lensProvider && lensEmitter.fire(lensProvider);

/** Registers the shared code-lens provider and its commands (once). */
function registerLenses() {
  if (lensesRegistered) return;
  lensesRegistered = true;
  monaco.editor.registerCommand('gd.inspect', (_a: unknown, id: string) => useStore.getState().inspectSymbol(id));
  monaco.editor.registerCommand('gd.callers', (_a: unknown, s: FileSymbol) => actions.callGraph(s, 'callers'));
  monaco.editor.registerCommand('gd.callees', (_a: unknown, s: FileSymbol) => actions.callGraph(s, 'callees'));
  monaco.editor.registerCommand('gd.trace', (_a: unknown, s: FileSymbol) => actions.trace(s));
  monaco.editor.registerCommand('gd.flow', (_a: unknown, s: FileSymbol) => actions.flow(s));
  monaco.editor.registerCommand('gd.qa', () => actions.qa('gaps'));
  lensProvider = {
    onDidChange: lensEmitter.event,
    provideCodeLenses(model) {
      const syms = symbolsByModel.get(model.uri.toString()) ?? [];
      const cov = coverageByModel.get(model.uri.toString());
      const lenses: { range: monaco.IRange; command: { id: string; title: string; arguments?: unknown[] } }[] = [];
      for (const s of syms) {
        if (!s.line) continue;
        const range = { startLineNumber: s.line, startColumn: 1, endLineNumber: s.line, endColumn: 1 };
        const callable = s.kind === 'func' || s.kind === 'method';
        if (callable && !s.abstract) {
          lenses.push({ range, command: { id: 'gd.callers', title: `↙ ${s.callers} caller${s.callers === 1 ? '' : 's'}`, arguments: [s] } });
          lenses.push({ range, command: { id: 'gd.callees', title: `↗ ${s.callees} call${s.callees === 1 ? '' : 's'}`, arguments: [s] } });
          lenses.push({ range, command: { id: 'gd.trace', title: '⇢ trace to here', arguments: [s] } });
          lenses.push({ range, command: { id: 'gd.flow', title: '◇ logic', arguments: [s] } });
          const q = cov?.get(s.id);
          if (q && (q.pct >= 0 || q.direct + q.indirect > 0 || q.risk > 0)) {
            const tests = q.direct ? ` · ${q.direct} test${q.direct === 1 ? '' : 's'}` : q.indirect ? ` · ${q.indirect} indirect` : ' · no tests';
            const title = q.pct >= 0 ? `${q.pct >= 80 ? '✓' : q.pct > 0 ? '◐' : '✗'} ${q.pct}% covered${tests}` : `${q.direct + q.indirect ? '◐' : '✗'}${tests.slice(2)}`;
            lenses.push({ range, command: { id: 'gd.qa', title, arguments: [] } });
          }
        } else if (s.kind !== 'external') {
          lenses.push({ range, command: { id: 'gd.inspect', title: `${s.kind}${s.methods?.length ? ` · ${s.methods.length} methods` : ''}`, arguments: [s.id] } });
        }
        if (s.complexity && s.complexity > 10) lenses.push({ range, command: { id: 'gd.flow', title: `⚠ complexity ${s.complexity}`, arguments: [s] } });
      }
      return { lenses, dispose() {} };
    },
  };
  monaco.languages.registerCodeLensProvider('*', lensProvider);
}

const annoColor: Record<string, string> = { info: 'info', warning: 'warning', bug: 'bug', idea: 'idea', question: 'question' };

/**
 * Read-only Monaco editor with code lenses, highlights, agent notes, git
 * change markers, Ctrl+click navigation and the file summary panel.
 */
export default function CodeView({ tab, active }: ViewProps) {
  const s = useStore();
  const file = String(tab.params.file ?? '');
  const line = Number(tab.params.line ?? 0) || 0;
  const endLine = Number(tab.params.endLine ?? 0) || line;
  const message = tab.params.message ? String(tab.params.message) : '';
  const hostRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<monaco.editor.IStandaloneCodeEditor>(null);
  const symbolsRef = useRef<FileSymbol[]>([]);
  const hlRef = useRef<monaco.editor.IEditorDecorationsCollection>(null);
  const annoRef = useRef<monaco.editor.IEditorDecorationsCollection>(null);
  const gitRef = useRef<monaco.editor.IEditorDecorationsCollection>(null);
  const covRef = useRef<monaco.editor.IEditorDecorationsCollection>(null);
  const [coverage, setCoverage] = useState<FileCoverage>();
  const [state, setState] = useState<{ loading: boolean; error?: unknown; binary?: boolean }>({ loading: true });
  const [banner, setBanner] = useState(message);
  // Opened by the button, or by an agent via get_file_summary(show=true).
  const [summaryOpen, setSummaryOpen] = useState(!!tab.params.summary);
  useEffect(() => {
    if (tab.params.summary) setSummaryOpen(true);
  }, [tab.params.summary, tab.params.flash]);
  useEffect(() => setBanner(message), [message, tab.params.flash]);

  // Create editor once.
  useEffect(() => {
    registerLenses();
    setMonacoTheme(useStore.getState().theme);
    const ed = monaco.editor.create(hostRef.current!, {
      readOnly: true,
      domReadOnly: true,
      automaticLayout: true,
      fontFamily: 'JetBrains Mono Variable, JetBrains Mono, ui-monospace, monospace',
      fontSize: useStore.getState().app?.config.settings.fontSize ?? 13,
      fontLigatures: true,
      lineHeight: 1.6,
      minimap: { enabled: true, renderCharacters: false, scale: 1, maxColumn: 90 },
      scrollBeyondLastLine: false,
      smoothScrolling: true,
      cursorBlinking: 'smooth',
      renderLineHighlight: 'all',
      glyphMargin: true,
      codeLens: true,
      codeLensFontFamily: 'Inter Variable, ui-sans-serif',
      codeLensFontSize: 11,
      padding: { top: 10, bottom: 10 },
      stickyScroll: { enabled: true },
      bracketPairColorization: { enabled: true },
      guides: { indentation: true, bracketPairs: true },
      contextmenu: true,
      links: false,
      // A reader, not an editor: don't box legitimate unicode like − or →.
      unicodeHighlight: { ambiguousCharacters: false, invisibleCharacters: false },
    });
    editorRef.current = ed;
    hlRef.current = ed.createDecorationsCollection();
    annoRef.current = ed.createDecorationsCollection();
    gitRef.current = ed.createDecorationsCollection();
    covRef.current = ed.createDecorationsCollection();

    const symbolAt = (ln: number) => {
      let best: FileSymbol | undefined;
      for (const sym of symbolsRef.current) {
        if (sym.line && sym.line <= ln && ln <= (sym.endLine ?? sym.line)) {
          if (!best || (sym.endLine! - sym.line) <= (best.endLine! - best.line!)) best = sym;
        }
      }
      return best;
    };
    const atCursor = () => symbolAt(ed.getPosition()?.lineNumber ?? 0);
    const addAction = (id: string, label: string, run: (s: FileSymbol) => void, key?: number) =>
      ed.addAction({
        id, label, contextMenuGroupId: '0_codediver', keybindings: key ? [key] : undefined,
        run: () => {
          const sym = atCursor();
          if (sym) run(sym);
        },
      });
    addAction('gd.trace', 'CodeDiver: Trace execution to here', (x) => actions.trace(x), monaco.KeyMod.Alt | monaco.KeyCode.KeyT);
    addAction('gd.flow', 'CodeDiver: Show logic flow', (x) => actions.flow(x), monaco.KeyMod.Alt | monaco.KeyCode.KeyL);
    addAction('gd.graph', 'CodeDiver: Call graph', (x) => actions.callGraph(x), monaco.KeyMod.Alt | monaco.KeyCode.KeyG);
    ed.addAction({ id: 'gd.summary', label: 'CodeDiver: Toggle file summary', contextMenuGroupId: '0_codediver', keybindings: [monaco.KeyMod.Alt | monaco.KeyCode.KeyS], run: () => setSummaryOpen((o) => !o) });
    addAction('gd.inspect', 'CodeDiver: Inspect symbol', (x) => useStore.getState().inspectSymbol(x.id), monaco.KeyMod.Alt | monaco.KeyCode.KeyI);

    // Ctrl/Cmd+click jumps to the definition of the identifier.
    ed.onMouseDown((e) => {
      if (!(e.event.ctrlKey || e.event.metaKey) || !e.target.position) return;
      const word = ed.getModel()?.getWordAtPosition(e.target.position)?.word;
      if (!word) return;
      const st = useStore.getState();
      api.searchSymbols(st.repoId, word, 10).then((res) => {
        const hit = res.find((r) => r.name === word && r.kind !== 'external');
        if (hit) actions.symbol(hit);
      });
    });

    const report = () => {
      const pos = ed.getPosition();
      const sel = ed.getSelection();
      const model = ed.getModel();
      const st = useStore.getState();
      const sym = pos ? symbolAt(pos.lineNumber) : undefined;
      st.reportContext({
        view: 'code', file: model?.uri.path.replace(/^\//, '') ?? '', line: sel?.startLineNumber, endLine: sel?.endLineNumber,
        selection: sel && model && !sel.isEmpty() ? model.getValueInRange(sel) : undefined, symbol: sym?.id,
      });
    };
    ed.onDidChangeCursorSelection(report);
    ed.onDidFocusEditorText(report);
    return () => {
      const uri = ed.getModel()?.uri.toString();
      if (uri) {
        symbolsByModel.delete(uri);
        coverageByModel.delete(uri);
      }
      ed.getModel()?.dispose();
      ed.dispose();
    };
  }, []);

  // Theme & font changes.
  useEffect(() => {
    setMonacoTheme(s.theme);
  }, [s.theme]);
  useEffect(() => {
    editorRef.current?.updateOptions({ fontSize: s.app?.config.settings.fontSize ?? 13 });
  }, [s.app?.config.settings.fontSize]);

  // Load file + symbols + git markers.
  useEffect(() => {
    const ed = editorRef.current!;
    let alive = true;
    setState({ loading: true });
    const uri = monaco.Uri.parse(`codediver://${s.repoId}/${file}`);
    Promise.all([api.read(s.repoId, file), api.fileSymbols(s.repoId, file).catch(() => [] as FileSymbol[])])
      .then(([fc, syms]) => {
        if (!alive) return;
        if (fc.binary) return setState({ loading: false, binary: true });
        let model = monaco.editor.getModel(uri);
        if (model) model.setValue(fc.content);
        else model = monaco.editor.createModel(fc.content, monacoLang(fc.language), uri);
        if (ed.getModel() !== model) {
          const old = ed.getModel();
          ed.setModel(model);
          if (old && old !== model) old.dispose();
        }
        symbolsRef.current = syms;
        symbolsByModel.set(uri.toString(), syms);
        refreshLenses();
        setState({ loading: false });
        api.diff(s.repoId, { paths: [file] }, false).then((ov) => {
          if (!alive) return;
          const f = ov.files[0];
          const added = new Set<number>();
          f?.hunks?.forEach((h) => h.lines.forEach((l) => l.kind === 'add' && l.newNo && added.add(l.newNo)));
          gitRef.current?.set([...added].map((n) => ({ range: new monaco.Range(n, 1, n, 1), options: { isWholeLine: true, linesDecorationsClassName: 'gd-changed-glyph' } })));
        }).catch(() => {});
      })
      .catch((error) => alive && setState({ loading: false, error }));
    return () => {
      alive = false;
    };
  }, [file, s.repoId, s.refreshKey]);

  // Test coverage: gutter markers and per-function lenses.
  const showCoverage = s.showCoverage || !!tab.params.coverage;
  useEffect(() => {
    let alive = true;
    api.qaFile(s.repoId, file).then(
      (c) => alive && setCoverage(c),
      () => alive && setCoverage(undefined),
    );
    return () => {
      alive = false;
    };
  }, [file, s.repoId, s.qaVersion, s.refreshKey]);
  useEffect(() => {
    const model = editorRef.current?.getModel();
    if (!model || state.loading) return;
    const uri = model.uri.toString();
    coverageByModel.set(uri, new Map((coverage?.functions ?? []).map((f) => [f.id, f])));
    refreshLenses();
    if (!coverage?.available || !showCoverage) {
      covRef.current?.clear();
      return;
    }
    const n = model.getLineCount();
    const partial = new Set(coverage.partial);
    const deco: monaco.editor.IModelDeltaDecoration[] = [];
    for (const ln of coverage.covered) {
      if (ln <= n) deco.push({ range: new monaco.Range(ln, 1, ln, 1), options: { isWholeLine: true, linesDecorationsClassName: partial.has(ln) ? 'gd-cov-part' : 'gd-cov-hit' } });
    }
    for (const ln of coverage.uncovered) {
      if (ln <= n) deco.push({ range: new monaco.Range(ln, 1, ln, 1), options: { isWholeLine: true, className: 'gd-cov-miss-line', linesDecorationsClassName: 'gd-cov-miss' } });
    }
    covRef.current?.set(deco);
  }, [coverage, showCoverage, state.loading]);

  // Highlight & reveal requested range.
  useEffect(() => {
    const ed = editorRef.current;
    if (!ed || state.loading || !line) {
      hlRef.current?.clear();
      return;
    }
    const range = new monaco.Range(line, 1, Math.max(line, endLine), 1);
    hlRef.current?.set([
      { range, options: { isWholeLine: true, className: 'gd-hl-line', linesDecorationsClassName: 'gd-hl-gutter' } },
      { range, options: { isWholeLine: true, className: 'gd-flash' } },
    ]);
    ed.setPosition({ lineNumber: line, column: 1 });
    // Reveal after layout settles: nested editors (walkthroughs) may not have
    // their final size on the first pass.
    const reveal = setTimeout(() => {
      ed.layout();
      ed.revealRangeInCenterIfOutsideViewport(range, monaco.editor.ScrollType.Smooth);
    }, 60);
    const t = setTimeout(() => hlRef.current?.set([{ range, options: { isWholeLine: true, className: 'gd-hl-line', linesDecorationsClassName: 'gd-hl-gutter' } }]), 1700);
    return () => {
      clearTimeout(t);
      clearTimeout(reveal);
    };
  }, [line, endLine, state.loading, tab.params.flash]);

  // Agent annotations.
  useEffect(() => {
    if (state.loading) return;
    const notes = s.session.annotations.filter((a: Annotation) => a.file === file);
    const model = editorRef.current?.getModel();
    annoRef.current?.set(
      notes.map((a) => {
        const end = Math.min(a.end_line || a.line, model?.getLineCount() ?? a.line);
        return {
          // End the range at the end of its last line so the inline note
          // (`after` text) renders after the code instead of before it.
          range: new monaco.Range(a.line, 1, end, model?.getLineMaxColumn(end) ?? 1),
          options: {
            isWholeLine: true,
            glyphMarginClassName: `gd-anno-glyph gd-anno-${annoColor[a.kind] ?? 'info'}`,
            glyphMarginHoverMessage: { value: `**${a.kind}** — ${a.text}` },
            hoverMessage: { value: `🤖 **Agent ${a.kind}:** ${a.text}` },
            after: { content: `   ◆ ${a.text.length > 90 ? a.text.slice(0, 87) + '…' : a.text}`, inlineClassName: 'gd-anno-inline' },
          },
        };
      }),
    );
  }, [s.session.annotations, file, state.loading]);

  useEffect(() => {
    if (active) editorRef.current?.layout();
  }, [active]);

  const cursorSymbol = () => {
    const ln = editorRef.current?.getPosition()?.lineNumber ?? line;
    return symbolsRef.current.filter((x) => x.line && x.line <= ln && ln <= (x.endLine ?? x.line)).sort((a, b) => a.endLine! - a.line! - (b.endLine! - b.line!))[0];
  };
  const withSym = (fn: (x: FileSymbol) => void) => () => {
    const x = cursorSymbol();
    if (x) fn(x);
    else s.toast({ kind: 'info', title: 'Place the cursor inside a function first' });
  };

  const parts = file.split('/');
  return (
    <>
      <div className="toolbar">
        <div className="crumbs mono">
          {parts.map((p, i) => (
            <span key={i}>
              {i > 0 && <span className="faint"> / </span>}
              {p}
            </span>
          ))}
        </div>
        <div className="grow" />
        {coverage?.available && (
          <button
            className={`btn sm ${showCoverage ? 'active' : ''}`}
            onClick={() => s.setShowCoverage(!showCoverage)}
            title={`${showCoverage ? 'Hide' : 'Show'} covered (green), partial (amber) and uncovered (red) lines${coverage.stale ? '. The file changed since the coverage report, so markers may be off' : ''}`}
          >
            <FlaskConical size={13} /> {coverage.pct}%{coverage.stale ? ' · stale' : ''}
          </button>
        )}
        <button className="btn sm" title="Alt+T" onClick={withSym(actions.trace)}><Route size={13} /> Trace to here</button>
        <button className={`btn sm ${summaryOpen ? 'active' : ''}`} title="Functions, types, variables and imports in this file (Alt+S)" onClick={() => setSummaryOpen(!summaryOpen)}>
          <ListTree size={13} /> Summary
        </button>
        <button className="btn sm" title="Alt+L" onClick={withSym(actions.flow)}><Workflow size={13} /> Logic</button>
        <button className="btn sm" title="Alt+G" onClick={withSym((x) => actions.callGraph(x))}><GitFork size={13} /> Calls</button>
        <button className="btn sm ghost" onClick={() => actions.history(file)}><History size={13} /> History</button>
      </div>
      {banner && (
        <div className="agent-banner">
          <Bot size={15} style={{ color: 'var(--accent-2)' }} />
          <span className="grow">{banner}</span>
          <button className="btn ghost icon sm" onClick={() => setBanner('')}><X size={13} /></button>
        </div>
      )}
      <div className="split">
        <div className="code-host">
          <div ref={hostRef} style={{ position: 'absolute', inset: 0 }} />
          {state.loading && <Spinner />}
          {state.binary && <div className="empty"><div>Binary file — nothing to show.</div></div>}
        </div>
        {summaryOpen && <FileSummaryPanel file={file} onClose={() => setSummaryOpen(false)} />}
      </div>
      {state.error ? <ErrorBox error={state.error} /> : null}
    </>
  );
}
