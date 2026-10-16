import { MarkerType, type Edge } from '@xyflow/react';
import type { GraphEdge } from '../lib/types';

interface EdgeStyle {
  color: string;
  dash?: string;
  animated?: boolean;
  width?: number;
  label?: string;
}

export const edgeStyles: Record<string, EdgeStyle> = {
  call: { color: 'var(--text-3)' },
  go: { color: 'var(--warn)', dash: '6 4', animated: true, label: 'go' },
  defer: { color: 'var(--warn)', dash: '3 3', label: 'defer' },
  ref: { color: 'var(--accent-2)', dash: '2 4', label: 'ref' },
  dispatch: { color: 'var(--accent-2)', dash: '6 3', label: 'dispatch' },
  import: { color: 'var(--text-3)' },
  implements: { color: 'var(--accent-2)', dash: '6 3' },
  field: { color: 'var(--text-3)', dash: '2 3', width: 1 },
  data: { color: 'var(--syn-string)' },
  async: { color: 'var(--warn)', dash: '6 4', animated: true },
  dependency: { color: 'var(--text-3)', dash: '4 3' },
  change: { color: 'var(--err)', width: 2 },
};

export const edgeLegend = ['call', 'go', 'defer', 'ref', 'dispatch'] as const;

/**
 * Converts a graph edge to a styled React Flow edge (dash, color, width
 * by call count, dimming and highlight).
 */
export function toFlowEdge(e: GraphEdge, opts: { highlight?: boolean; dim?: boolean } = {}): Edge {
  const st = edgeStyles[e.kind] ?? edgeStyles.call;
  const hot = opts.highlight || e.highlight;
  const color = hot ? 'var(--accent)' : st.color;
  const label = e.label ?? (e.count && e.count > 1 ? `×${e.count}` : st.label);
  return {
    id: e.id,
    source: e.from,
    target: e.to,
    type: 'default',
    animated: !!(st.animated || (hot && opts.highlight)),
    label,
    labelBgPadding: [4, 2],
    labelBgBorderRadius: 4,
    style: {
      stroke: color,
      strokeWidth: (st.width ?? 1.5) + (hot ? 0.6 : 0) + Math.min(2, (e.count ?? 1) - 1) * 0.4,
      strokeDasharray: st.dash,
      opacity: opts.dim ? 0.12 : e.heuristic ? 0.6 : 1,
    },
    markerEnd: { type: MarkerType.ArrowClosed, color, width: 16, height: 16 },
  };
}
