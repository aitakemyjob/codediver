// Navigation helpers shared by views, the inspector, the palette and code lenses.
import { useStore } from './store';
import { basename } from './format';
import type { CodeSymbol } from './types';

/** Opens (or focuses) a tab via the store. */
const open = (...a: Parameters<ReturnType<typeof useStore.getState>['open']>) => useStore.getState().open(...a);

type SymRef = Pick<CodeSymbol, 'id' | 'qualified'> & Partial<Pick<CodeSymbol, 'file' | 'line' | 'endLine'>>;

export const actions = {
  /** Opens a file, optionally highlighting a line range. */
  code(file: string, line?: number, endLine?: number) {
    open({ kind: 'code', title: basename(file), params: { file, line, endLine, flash: Date.now() } });
  },
  /** Opens a symbol's definition and shows it in the inspector. */
  symbol(sym: SymRef) {
    if (sym.file) actions.code(sym.file, sym.line, sym.endLine);
    useStore.getState().inspectSymbol(sym.id);
  },
  /** Opens the call graph around a symbol (callees by default). */
  callGraph(sym: SymRef, direction: 'callees' | 'callers' | 'both' = 'callees') {
    open({ kind: 'graph', title: `${direction === 'callers' ? 'Callers' : 'Calls'} · ${sym.qualified}`, params: { kind: 'callgraph', symbol: sym.id, direction, depth: 2, includeRefs: true } });
  },
  /** Opens the trace of every caller path leading to a symbol. */
  trace(sym: SymRef) {
    open({ kind: 'graph', title: `Flow to · ${sym.qualified}`, params: { kind: 'trace', symbol: sym.id, depth: 6 } });
  },
  /** Opens the control-flow (logic) view of a function. */
  flow(sym: SymRef) {
    open({ kind: 'flow', title: sym.qualified, params: { symbol: sym.id } });
  },
  /** Opens the type diagram for a package, directory or file. */
  types(scope: string, title?: string) {
    open({ kind: 'graph', title: `Types · ${title ?? basename(scope)}`, params: { kind: 'types', scope } });
  },
  /** Opens the dependency graph, optionally focused on one package. */
  deps(focus = '') {
    open({ kind: 'graph', title: focus ? `Deps · ${basename(focus)}` : 'Dependencies', params: { kind: 'deps', focus } });
  },
  /** Opens the heatmap, optionally preselecting a metric. */
  heatmap(metric?: string) {
    open({ kind: 'heatmap', title: 'Heatmap', params: metric ? { metric } : {} });
  },
  /** Opens the commit history of a file. */
  history(path: string) {
    open({ kind: 'history', title: `History · ${basename(path)}`, params: { path } });
  },
};
