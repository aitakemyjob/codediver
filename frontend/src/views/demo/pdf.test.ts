import { buildPdf } from './pdf';

const latin1 = (b: Uint8Array) => Array.from(b, (c) => String.fromCharCode(c)).join('');

describe('pdf writer', () => {
  const jpeg = new Uint8Array([0xff, 0xd8, 0xff, 0xd9]);
  const pdf = buildPdf([{ jpeg, width: 4, height: 2 }, { jpeg, width: 8, height: 6 }], 842, 595, 'Demo (draft) → final');
  const s = latin1(pdf);

  it('has a header, two pages and a trailer', () => {
    expect(s.startsWith('%PDF-1.4\n')).toBe(true);
    expect(s).toContain('/Count 2');
    expect(s).toContain('/Width 8 /Height 6');
    expect(s.trimEnd().endsWith('%%EOF')).toBe(true);
    // Title is escaped and kept ASCII.
    expect(s).toContain('/Title (Demo \\(draft\\) ? final)');
  });

  it('points every xref entry and startxref at the right bytes', () => {
    const startxref = Number(/startxref\n(\d+)/.exec(s)![1]);
    expect(s.slice(startxref, startxref + 4)).toBe('xref');
    const entries = [...s.slice(startxref).matchAll(/^(\d{10}) 00000 n $/gm)].map((m) => Number(m[1]));
    expect(entries.length).toBe(3 + 2 * 3);
    entries.forEach((off, i) => expect(s.slice(off).startsWith(`${i + 1} 0 obj`)).toBe(true));
  });

  it('embeds the JPEG bytes unchanged', () => {
    expect(s).toContain('stream\n' + latin1(jpeg) + '\nendstream');
  });
});
