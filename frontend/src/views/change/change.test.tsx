// Blast radius and change-tests panels, and the helpers behind them.
import { act, fireEvent, render, screen } from '@testing-library/react';
import type { ChangeReport, Impact, ImpactNode } from '../../lib/types';
import { useStore } from '../../lib/store';

const api = vi.hoisted(() => ({ qaRunTests: vi.fn(), qaLastRun: vi.fn() }));
vi.mock('../../lib/api', () => ({ api }));
const actions = vi.hoisted(() => ({ code: vi.fn(), qa: vi.fn() }));
vi.mock('../../lib/actions', () => ({ actions }));

import { diffMarks, hasGaps, nodesById, packageGraph, pathToChange, testPrompt, testStatus, uncoveredRanges } from './change';
import { ImpactPanel } from './ImpactPanel';
import { TestsPanel } from './TestsPanel';

const node = (o: Partial<ImpactNode>): ImpactNode => ({ id: o.name!, name: '', kind: 'func', file: 'a.go', line: 1, package: 'api', depth: 0, pct: -1, direct: 0, indirect: 0, tested: false, ...o });

const validate = node({ id: 'v', name: 'validate', file: 'api/api.go', line: 36, tested: true, indirect: 1, pct: 0 });
const create = node({ id: 'c', name: 'Handler.CreateOrder', file: 'api/api.go', line: 18, depth: 1, via: 'validate', viaId: 'v', tested: true, direct: 2, pct: 82 });
const save = node({ id: 's', name: 'Batch.Save', file: 'api/batch.go', line: 5, depth: 1, via: 'validate', viaId: 'v', review: true });
const main = node({ id: 'm', name: 'main', file: 'cmd/shop/main.go', line: 10, package: 'cmd/shop', depth: 2, via: 'Handler.CreateOrder', viaId: 'c', entry: 'main' });

const impact: Impact = {
  changed: [validate],
  rings: [
    { depth: 1, total: 3, tested: 1, avgPct: 41, packages: [{ path: 'api', count: 3, tested: 1 }], nodes: [save, create], more: 1 },
    { depth: 2, total: 1, tested: 0, avgPct: -1, packages: [{ path: 'cmd/shop', count: 1, tested: 0 }], nodes: [main] },
  ],
  entries: [main],
  beyond: 2,
  tests: true,
  coverage: true,
  packageEdges: [{ from: 'cmd/shop', to: 'api', count: 1 }],
  packageDepth: { api: 0, 'cmd/shop': 2 },
  summary: { changed: 1, callers: [3, 1, 0], files: 3, packages: 2, entries: 1, untested: 1 },
};

const report: ChangeReport = {
  title: 'Accept qty 1',
  impact,
  coverage: {
    available: true, measured: true, lines: 3, covered: 1, uncovered: 2, unknown: 0, added: 1, untested: [],
    files: [{ path: 'api/api.go', side: 'original', covered: 1, uncovered: 2, unknown: 0, measured: true, lines: [
      { line: 36, state: 'covered', tests: ['t1'] }, { line: 37, state: 'uncovered' }, { line: 38, state: 'uncovered' },
    ] }],
    tests: [
      { id: 't1', name: 'TestCreateOrder', behavior: 'Create order', file: 'api/api_test.go', line: 9, framework: 'go test', lines: 1, direct: false, measured: true, runnable: true, functions: ['validate'] },
      { id: 't2', name: 'posts the quantity', behavior: 'Posts the quantity', file: 'web/app.test.ts', line: 4, framework: 'vitest', lines: 0, direct: true, measured: false, runnable: false },
    ],
  },
};

describe('change helpers', () => {
  it('walks a caller back to the change', () => {
    const byId = nodesById(impact);
    expect(pathToChange(main, byId)).toEqual(['main', 'Handler.CreateOrder', 'validate']);
    // A hop left out of a capped list still shows by name.
    expect(pathToChange(node({ name: 'x', via: 'gone', viaId: 'zz' }), byId)).toEqual(['x', 'gone']);
  });

  it('summarizes test status and builds the package graph', () => {
    expect(testStatus(create)).toBe('82.0% · 2 direct');
    expect(testStatus(save)).toBe('no tests');
    const g = packageGraph(impact);
    expect(g.nodes.find((n) => n.id === 'api')).toMatchObject({ root: true, sublabel: 'changed' });
    expect(g.nodes.find((n) => n.id === 'cmd/shop')?.sublabel).toBe('2 calls away');
    expect(g.edges).toEqual([expect.objectContaining({ from: 'cmd/shop', to: 'api', count: 1 })]);
  });

  it('marks changed lines on the side they number, with the tests that run them', () => {
    const m = diffMarks(report, 'api/api.go');
    expect(m.modified).toBeUndefined();
    expect(m.original).toEqual([
      { line: 36, kind: 'hit', hover: 'Run by TestCreateOrder' },
      { line: 37, kind: 'miss', hover: 'No test runs this line' },
      { line: 38, kind: 'miss', hover: 'No test runs this line' },
    ]);
    expect(diffMarks(report, 'other.go')).toEqual({});
    expect(diffMarks({ ...report, coverage: { ...report.coverage, approximate: true } }, 'api/api.go')).toEqual({});
  });

  it('writes a prompt for the missing tests', () => {
    expect(uncoveredRanges(report.coverage.files[0])).toEqual([{ from: 37, to: 38 }]);
    expect(hasGaps(report)).toBe(true);
    const p = testPrompt(report);
    expect(p).toContain('"Accept qty 1"');
    expect(p).toContain('api/api.go:37-38');
    expect(p).toContain('Untested direct callers: Batch.Save');
  });
});

describe('ImpactPanel', () => {
  it('shows direct callers first and the rest by package', () => {
    render(<ImpactPanel impact={impact} />);
    expect(screen.getByText('Direct callers')).toBeInTheDocument();
    expect(screen.getByText('Batch.Save')).toBeInTheDocument(); // open by default
    expect(screen.getByText('untested')).toBeInTheDocument();
    expect(screen.getByText('+1 more not listed')).toBeInTheDocument();
    // The second ring starts collapsed with package counts; it expands on click.
    expect(screen.queryByText('main', { selector: '.impact-row .mono' })).toBeNull();
    fireEvent.click(screen.getByText('Two calls away'));
    expect(screen.getByText('via Handler.CreateOrder → validate')).toBeInTheDocument();
    expect(screen.getByText('Where it surfaces')).toBeInTheDocument();
    expect(screen.getByText('2 call paths continue past three calls.')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Batch.Save'));
    expect(actions.code).toHaveBeenCalledWith('api/batch.go', 5);
  });

  it('says when the change only adds code', () => {
    render(<ImpactPanel impact={{ ...impact, changed: [] }} />);
    expect(screen.getByText('No existing code changes')).toBeInTheDocument();
  });
});

describe('TestsPanel', () => {
  beforeEach(() => useStore.setState({ repoId: 'r', qaRun: undefined }));

  it('summarizes the changed lines and runs the runnable tests', async () => {
    api.qaRunTests.mockResolvedValue({ id: 'run1', repo: 'r', state: 'running', started: '', suites: [], log: [] });
    api.qaLastRun.mockResolvedValue(null);
    render(<TestsPanel report={report} />);
    expect(screen.getByText('measured')).toBeInTheDocument();
    expect(screen.getByText('2 not run by any test')).toBeInTheDocument();
    expect(screen.getByText('runs 1 changed line')).toBeInTheDocument();
    expect(screen.getByText('calls the changed code')).toBeInTheDocument();
    fireEvent.click(screen.getByText('37-38'));
    expect(actions.code).toHaveBeenCalledWith('api/api.go', 37, 38);
    await act(async () => fireEvent.click(screen.getByText('Run these tests')));
    // Only the Go test can be run per test.
    expect(api.qaRunTests).toHaveBeenCalledWith('r', ['t1']);
    expect(screen.getByText('Coverage run')).toBeInTheDocument();
  });

  it('asks for a coverage run when there is no report', () => {
    render(<TestsPanel report={{ ...report, coverage: { ...report.coverage, available: false, measured: false, tests: [] } }} />);
    expect(screen.getByText('estimated')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Run coverage in QA'));
    expect(actions.qa).toHaveBeenCalled();
    expect(screen.getByText('No tests exercise this change')).toBeInTheDocument();
    expect(screen.getByText('Run these tests').closest('button')).toBeDisabled();
  });
});
