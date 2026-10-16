import { basename, compact, dirname, kindStyle, paramList, plural, relTime, shortType } from './format';

describe('format', () => {
  it('paths', () => {
    expect(basename('a/b/c.go')).toBe('c.go');
    expect(dirname('a/b/c.go')).toBe('a/b');
    expect(dirname('c.go')).toBe('');
  });
  it('relative time', () => {
    const now = Date.parse('2026-01-01T12:00:00Z');
    expect(relTime('2026-01-01T11:59:58Z', now)).toBe('just now');
    expect(relTime('2026-01-01T11:59:00Z', now)).toBe('1m ago');
    expect(relTime('2026-01-01T09:00:00Z', now)).toBe('3h ago');
    expect(relTime('garbage', now)).toBe('');
  });
  it('numbers and params', () => {
    expect(compact(950)).toBe('950');
    expect(compact(1500)).toBe('1.5k');
    expect(compact(25_000)).toBe('25k');
    expect(paramList([{ name: 'a', type: 'int' }, { name: '', type: 'error' }])).toBe('a int, error');
    expect(shortType([{ name: '', type: 'string' }, { name: '', type: 'bool' }])).toBe('(string, bool)');
    expect(shortType(null)).toBe('');
    expect(plural(1, 'file')).toBe('1 file');
    expect(plural(2, 'file')).toBe('2 files');
  });
  it('kind styles', () => {
    expect(kindStyle('interface').glyph).toBe('I');
    expect(kindStyle('nope').glyph).toBe('•');
  });
});
