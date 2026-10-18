import { ANIMATION_CSP, sandboxDocument, themeRule, ZOOM_SCRIPT } from './sandbox';

const style = { getPropertyValue: (v: string) => ({ '--bg': '#0b1120', '--accent': '#4fc3f7', '--text': 'x;}</style><script>alert(1)</script>' } as Record<string, string>)[v] ?? '' };

describe('animation sandbox document', () => {
  it('blocks every network request', () => {
    expect(ANIMATION_CSP).toContain("default-src 'none'");
    expect(ANIMATION_CSP).not.toMatch(/connect-src|https?:/);
  });

  it('exposes theme colors and strips anything that could break out of the rule', () => {
    const rule = themeRule(style);
    expect(rule).toContain('--bg:#0b1120');
    expect(rule).toContain('--accent:#4fc3f7');
    expect(rule).not.toMatch(/[<>]/);
  });

  it('puts the CSP before any agent markup', () => {
    const svg = sandboxDocument({ format: 'svg', source: '<svg><script>1</script></svg>' }, style);
    expect(svg.indexOf('Content-Security-Policy')).toBeLessThan(svg.indexOf('<svg'));
    const full = sandboxDocument({ format: 'html', source: '<!doctype html><html><head><script>evil()</script></head><body></body></html>' }, style);
    expect(full.indexOf('Content-Security-Policy')).toBeLessThan(full.indexOf('evil()'));
    const early = sandboxDocument({ format: 'html', source: '<script>first()</script><html><head></head><body></body></html>' }, style);
    expect(early.indexOf('Content-Security-Policy')).toBeLessThan(early.indexOf('first()'));
    const fragment = sandboxDocument({ format: 'html', source: '<canvas id=c></canvas><script>1</script>' }, style);
    // The zoom helper is ours and also runs before any agent script.
    expect(fragment.indexOf(ZOOM_SCRIPT)).toBeGreaterThan(fragment.indexOf('Content-Security-Policy'));
    expect(fragment.indexOf(ZOOM_SCRIPT)).toBeLessThan(fragment.indexOf('<canvas'));
    expect(fragment.startsWith('<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy"')).toBe(true);
  });
});
