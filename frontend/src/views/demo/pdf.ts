// A minimal PDF writer: each page is one full-page JPEG. That's all demo
// exports need (pages are composed on a canvas), and it avoids pulling a
// PDF library into the app.

export interface PdfPage {
  /** JPEG bytes. */
  jpeg: Uint8Array;
  /** Image size in pixels. */
  width: number;
  height: number;
}

/**
 * Builds a PDF with one page per image, each stretched over a page of
 * pageW × pageH points (default A4 landscape).
 */
export function buildPdf(pages: PdfPage[], pageW = 842, pageH = 595, title = ''): Uint8Array {
  const enc = new TextEncoder();
  const chunks: Uint8Array[] = [];
  const offsets: number[] = [];
  let size = 0;
  const push = (b: Uint8Array) => {
    chunks.push(b);
    size += b.length;
  };
  const text = (s: string) => push(enc.encode(s));
  const obj = (n: number, body: () => void) => {
    offsets[n] = size;
    text(`${n} 0 obj\n`);
    body();
    text('\nendobj\n');
  };

  // Header; the second line's high bytes mark the file as binary.
  text('%PDF-1.4\n');
  push(new Uint8Array([0x25, 0xe2, 0xe3, 0xcf, 0xd3, 0x0a]));

  // Objects: 1 catalog, 2 page tree, 3 info, then 3 per page (page, content, image).
  const first = 4;
  const kids = pages.map((_, i) => `${first + i * 3} 0 R`).join(' ');
  obj(1, () => text('<< /Type /Catalog /Pages 2 0 R >>'));
  obj(2, () => text(`<< /Type /Pages /Kids [${kids}] /Count ${pages.length} >>`));
  obj(3, () => text(`<< /Title (${pdfString(title)}) /Producer (CodeDiver) >>`));
  pages.forEach((p, i) => {
    const [page, content, image] = [first + i * 3, first + i * 3 + 1, first + i * 3 + 2];
    obj(page, () =>
      text(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${pageW} ${pageH}] /Resources << /XObject << /Im0 ${image} 0 R >> >> /Contents ${content} 0 R >>`),
    );
    const draw = enc.encode(`q ${pageW} 0 0 ${pageH} 0 0 cm /Im0 Do Q`);
    obj(content, () => {
      text(`<< /Length ${draw.length} >>\nstream\n`);
      push(draw);
      text('\nendstream');
    });
    obj(image, () => {
      text(`<< /Type /XObject /Subtype /Image /Width ${p.width} /Height ${p.height} /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length ${p.jpeg.length} >>\nstream\n`);
      push(p.jpeg);
      text('\nendstream');
    });
  });

  const count = first + pages.length * 3;
  const xref = size;
  text(`xref\n0 ${count}\n0000000000 65535 f \n`);
  for (let n = 1; n < count; n++) text(`${String(offsets[n]).padStart(10, '0')} 00000 n \n`);
  text(`trailer\n<< /Size ${count} /Root 1 0 R /Info 3 0 R >>\nstartxref\n${xref}\n%%EOF\n`);

  const out = new Uint8Array(size);
  let at = 0;
  for (const c of chunks) {
    out.set(c, at);
    at += c.length;
  }
  return out;
}

/** Escapes text for a PDF literal string, keeping it ASCII. */
function pdfString(s: string): string {
  return s.replace(/[^\x20-\x7e]/g, '?').replace(/([\\()])/g, '\\$1');
}
