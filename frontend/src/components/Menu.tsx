import { useEffect, useRef, useState, type ReactNode } from 'react';

export interface MenuItem {
  key: string;
  label: ReactNode;
  hint?: ReactNode;
  icon?: ReactNode;
  active?: boolean;
  onSelect?: () => void;
  separator?: boolean;
}

/** Minimal dropdown anchored under its trigger. */
export function Menu({ trigger, items, align = 'left', width = 260 }: { trigger: (open: boolean) => ReactNode; items: MenuItem[]; align?: 'left' | 'right'; width?: number }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    window.addEventListener('mousedown', close);
    window.addEventListener('keydown', esc);
    return () => {
      window.removeEventListener('mousedown', close);
      window.removeEventListener('keydown', esc);
    };
  }, [open]);
  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <div onClick={() => setOpen((o) => !o)}>{trigger(open)}</div>
      {open && (
        <div
          className="modal"
          style={{ position: 'absolute', top: 'calc(100% + 6px)', [align]: 0, width, maxHeight: 420, padding: 6, zIndex: 40, animationDuration: '.12s' }}
        >
          <div style={{ overflow: 'auto' }}>
            {items.map((it) =>
              it.separator ? (
                <div key={it.key} style={{ height: 1, background: 'var(--border)', margin: '6px 4px' }} />
              ) : (
                <div
                  key={it.key}
                  className={`palette-item ${it.active ? 'on' : ''}`}
                  onClick={() => {
                    setOpen(false);
                    it.onSelect?.();
                  }}
                >
                  {it.icon && <span className="pi-icon">{it.icon}</span>}
                  <span className="ellipsis">{it.label}</span>
                  {it.hint && <span className="pi-sub ellipsis">{it.hint}</span>}
                </div>
              ),
            )}
          </div>
        </div>
      )}
    </div>
  );
}
