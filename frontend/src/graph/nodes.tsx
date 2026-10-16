import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { kindStyle, shortType } from '../lib/format';
import { textWidth } from '../lib/layout';
import type { FlowNode, GraphNode } from '../lib/types';

export interface GNodeData extends Record<string, unknown> {
  node: GraphNode;
  horizontal: boolean;
  dim?: boolean;
  selected?: boolean;
  heatColor?: string;
}

export interface FNodeData extends Record<string, unknown> {
  node: FlowNode;
  dim?: boolean;
  selected?: boolean;
  note?: string;
  stepNo?: number;
}

/** Invisible connection points, left/right or top/bottom by layout. */
function Handles({ horizontal }: { horizontal: boolean }) {
  return (
    <>
      <Handle type="target" position={horizontal ? Position.Left : Position.Top} isConnectable={false} />
      <Handle type="source" position={horizontal ? Position.Right : Position.Bottom} isConnectable={false} />
    </>
  );
}

/** Glyph and tinted background for a symbol kind. */
const glyphStyle = (kind?: string) => {
  const k = kindStyle(kind);
  return { glyph: k.glyph, style: { color: k.color, background: `color-mix(in srgb, ${k.color} 16%, transparent)` } };
};

// ---- sizing (ELK needs dimensions before render) ----

/** Estimates a node's rendered size so ELK can lay it out before render. */
export function sizeOf(n: GraphNode): { width: number; height: number } {
  switch (n.kind) {
    case 'package': {
      const w = Math.max(textWidth(n.label, 12.5, true), textWidth(n.sublabel ?? '', 10.5, true)) + 60;
      return { width: clamp(w, 170, 340), height: 82 };
    }
    case 'struct':
    case 'class':
    case 'interface':
    case 'type': {
      const rows = Math.min(14, (n.fields?.length ?? 0) + (n.members?.length ?? 0));
      const longest = Math.max(
        textWidth(n.label, 12.5, true) + 50,
        ...(n.fields ?? []).map((f) => textWidth(`${f.name} ${f.type}`, 10.5, true) + 28),
        ...(n.members ?? []).map((m) => textWidth(m, 10.5, true) + 28),
      );
      return { width: clamp(longest, 200, 400), height: 58 + (rows ? rows * 15.5 + 14 : 0) };
    }
    case 'external':
      return { width: clamp(textWidth(n.label, 12, true) + 56, 150, 300), height: 54 };
    default: {
      if (!n.symKind && !n.symbol) {
        // Custom diagram node.
        const w = Math.max(textWidth(n.label, 12.5), textWidth(n.detail ?? '', 11)) + 56;
        return { width: clamp(w, 140, 320), height: n.detail ? 64 : 46 };
      }
      const io = (n.params?.length ?? 0) + (n.results?.length ?? 0) > 0;
      const badges = (n.badges ?? []).filter((b) => b !== 'exported').length > 0;
      return { width: clamp(textWidth(n.label, 12, true) + 70, 190, 360), height: 56 + (io ? 30 : 0) + (badges ? 24 : 0) + 3 };
    }
  }
}

/**
 * Estimates a control-flow node's size from its code lines, calls and
 * attached step note.
 */
export function flowSize(n: FlowNode, note?: string): { width: number; height: number } {
  const lines = n.code?.length ? n.code.slice(0, 6) : [n.label];
  const longest = Math.max(...lines.map((l) => textWidth(l, 11.5, true)), 60);
  const width = clamp(longest + 44, n.kind === 'start' || n.kind === 'end' ? 120 : 150, 470);
  let height = 18 + 16 * lines.length + (n.kind === 'start' || n.kind === 'end' ? 0 : 16);
  if (n.code && n.code.length > 6) height += 16;
  if (n.calls?.length) height += 24 * Math.ceil(Math.min(n.calls.length, 6) / 3);
  if (note) height += 14 + Math.ceil(textWidth(note, 11) / (width - 24)) * 15;
  return { width, height };
}

/** Clamps v into [lo, hi]. */
const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));

// ---- components ----

/** Function/method card: name, location, inputs → outputs and badges. */
export const FunctionNode = memo(({ data }: NodeProps) => {
  const { node: n, horizontal, dim, selected, heatColor } = data as GNodeData;
  const g = glyphStyle(n.symKind ?? n.kind);
  const ins = shortType(n.params);
  const outs = shortType(n.results);
  const badges = (n.badges ?? []).filter((b) => b !== 'exported');
  return (
    <div className={`gnode ${n.root ? 'root' : ''} ${dim ? 'dim' : ''} ${selected ? 'sel' : ''} ${n.external ? 'external' : ''}`} style={{ width: '100%', height: '100%' }}>
      <Handles horizontal={horizontal} />
      <div className="gn-head">
        <span className="gn-icon" style={g.style}>{g.glyph}</span>
        <span className="gn-title" title={n.label}>{n.label}</span>
      </div>
      <div className="gn-sub" title={n.sublabel}>{n.sublabel}</div>
      {(ins || outs) && !n.external && (
        <div className="gn-io" title={`(${ins}) → ${outs}`}>
          <span className="in">{ins || '∅'}</span>
          <span className="arrow">→</span>
          <span className="out">{outs || '∅'}</span>
        </div>
      )}
      {badges.length > 0 && (
        <div className="gn-badges">
          {badges.map((b) => (
            <span key={b} className={`badge ${b === 'entry' ? 'ok' : b === 'complex' ? 'warn' : b === 'changed' ? 'err' : b === 'interface' ? 'violet' : ''}`}>{b}</span>
          ))}
          {n.heuristic && <span className="badge">~</span>}
        </div>
      )}
      {heatColor && <div className="heatbar" style={{ background: heatColor, position: 'absolute', left: 0, right: 0, bottom: 0 }} />}
    </div>
  );
});

/** Package card with file and dependent counts. */
export const PackageNode = memo(({ data }: NodeProps) => {
  const { node: n, horizontal, dim, selected, heatColor } = data as GNodeData;
  return (
    <div className={`gnode ${n.root ? 'root' : ''} ${dim ? 'dim' : ''} ${selected ? 'sel' : ''} ${n.external ? 'external' : ''}`} style={{ width: '100%', height: '100%', position: 'relative' }}>
      <Handles horizontal={horizontal} />
      <div className="gn-head">
        <span className="gn-icon" style={glyphStyle('package').style}>▣</span>
        <span className="gn-title">{n.label}</span>
      </div>
      <div className="gn-sub">{n.sublabel || n.detail}</div>
      <div className="gn-badges">
        {(n.badges ?? []).map((b) => (
          <span key={b} className={`badge ${b === 'entry' ? 'ok' : ''}`}>{b}</span>
        ))}
      </div>
      {heatColor && <div className="heatbar" style={{ background: heatColor, position: 'absolute', left: 0, right: 0, bottom: 0 }} />}
    </div>
  );
});

/** Class/struct/interface card listing fields and methods. */
export const TypeNode = memo(({ data }: NodeProps) => {
  const { node: n, horizontal, dim, selected } = data as GNodeData;
  const g = glyphStyle(n.symKind ?? n.kind);
  const fields = n.fields ?? [];
  const members = n.members ?? [];
  const max = 14;
  return (
    <div className={`gnode ${dim ? 'dim' : ''} ${selected ? 'sel' : ''}`} style={{ width: '100%', height: '100%' }}>
      <Handles horizontal={horizontal} />
      <div className="gn-head">
        <span className="gn-icon" style={g.style}>{g.glyph}</span>
        <span className="gn-title">{n.label}</span>
        <span className="badge" style={{ marginLeft: 'auto' }}>{n.symKind ?? n.kind}</span>
      </div>
      <div className="gn-sub">{n.sublabel}</div>
      {fields.length + members.length > 0 && (
        <div className="members">
          {fields.slice(0, max).map((f, i) => (
            <div key={'f' + i} className="f ellipsis"><b>{f.name}</b> {f.type}</div>
          ))}
          {members.slice(0, Math.max(0, max - fields.length)).map((m, i) => (
            <div key={'m' + i} className="m ellipsis">{m}</div>
          ))}
        </div>
      )}
    </div>
  );
});

const customColors: Record<string, string> = {
  function: 'var(--syn-func)', class: 'var(--syn-type)', service: 'var(--accent)', data: 'var(--syn-string)',
  decision: 'var(--accent-2)', start: 'var(--ok)', end: 'var(--ok)', external: 'var(--text-3)', note: 'var(--warn)', step: 'var(--accent)',
};

/** Agent diagram node, styled by its kind (service, data, decision...). */
export const CustomNode = memo(({ data }: NodeProps) => {
  const { node: n, horizontal, dim, selected, heatColor } = data as GNodeData;
  const color = customColors[n.kind] ?? 'var(--accent)';
  return (
    <div
      className={`gnode ${dim ? 'dim' : ''} ${selected ? 'sel' : ''}`}
      style={{ width: '100%', height: '100%', borderColor: `color-mix(in srgb, ${color} 55%, transparent)`, borderRadius: n.kind === 'start' || n.kind === 'end' ? 999 : n.kind === 'decision' ? 4 : 10, borderStyle: n.kind === 'external' || n.kind === 'note' ? 'dashed' : 'solid', position: 'relative' }}
    >
      <Handles horizontal={horizontal} />
      <div className="gn-head">
        <span className="dot" style={{ background: color }} />
        <span className="gn-title" style={{ fontFamily: 'var(--font-ui)' }}>{n.label}</span>
      </div>
      {n.detail && <div className="gn-sub" style={{ fontFamily: 'var(--font-ui)' }}>{n.detail}</div>}
      {heatColor && <div className="heatbar" style={{ background: heatColor, position: 'absolute', left: 0, right: 0, bottom: 0 }} />}
    </div>
  );
});

const flowKindLabel: Partial<Record<FlowNode['kind'], string>> = {
  cond: 'if', loop: 'loop', switch: 'switch', case: 'case', return: 'return', defer: 'defer', go: 'goroutine', panic: 'abort', jump: 'jump', block: '',
};

/**
 * Control-flow block: statement kind, code lines, calls and an optional
 * narrated step.
 */
export const FlowNodeView = memo(({ data }: NodeProps) => {
  const { node: n, dim, selected, note, stepNo } = data as FNodeData;
  const lines = n.code?.length ? n.code : [n.label];
  const terminal = n.kind === 'start' || n.kind === 'end';
  return (
    <div className={`fnode k-${n.kind} ${dim ? 'dim' : ''} ${selected ? 'sel' : ''}`} style={{ width: '100%', height: '100%', overflow: 'hidden' }}>
      <Handle type="target" position={Position.Top} isConnectable={false} />
      <Handle type="source" position={Position.Bottom} isConnectable={false} />
      {!terminal && (
        <div className="kind">
          {stepNo ? <span className="badge violet" style={{ marginRight: 6 }}>{stepNo}</span> : null}
          {flowKindLabel[n.kind] || 'step'}
          {n.line ? <span className="faint" style={{ float: 'right', fontFamily: 'var(--font-mono)', fontWeight: 500 }}>L{n.line}{n.endLine && n.endLine !== n.line ? `–${n.endLine}` : ''}</span> : null}
        </div>
      )}
      {lines.slice(0, 6).map((l, i) => (
        <div key={i} className="code-line">{l}</div>
      ))}
      {lines.length > 6 && <div className="faint">… {lines.length - 6} more</div>}
      {n.calls && n.calls.length > 0 && (
        <div className="calls">
          {n.calls.slice(0, 6).map((c) => (
            <span key={c}>{c}()</span>
          ))}
        </div>
      )}
      {note && <div className="step-note">{note}</div>}
    </div>
  );
});

export const graphNodeTypes = { fn: FunctionNode, pkg: PackageNode, type: TypeNode, custom: CustomNode };
export const flowNodeTypes = { flow: FlowNodeView };

/** Chooses the React Flow node type for a graph node. */
export function nodeTypeFor(n: GraphNode): keyof typeof graphNodeTypes {
  if (n.kind === 'package' || (n.kind === 'external' && !n.symbol && n.id.startsWith('extpkg:'))) return 'pkg';
  if (['struct', 'class', 'interface', 'type'].includes(n.kind)) return 'type';
  if (n.symbol || n.symKind) return 'fn';
  return 'custom';
}
