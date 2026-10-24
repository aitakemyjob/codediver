import { useEffect, useMemo, useRef, useState } from 'react';
import { Background, BackgroundVariant, Controls, MarkerType, ReactFlow, ReactFlowProvider, useReactFlow, type Edge, type Node } from '@xyflow/react';
import { layout } from '../lib/layout';
import type { Flow, FlowEdge } from '../lib/types';
import { flowNodeTypes, flowSize, type FNodeData } from './nodes';

export interface FlowCanvasProps {
  flow: Flow;
  notes?: Map<string, { text: string; n: number }>;
  selected?: string;
  highlight?: Set<string>;
  onSelect?: (id: string) => void;
  onOpen?: (id: string) => void;
  /** Opens closure index of node nodeId. */
  onClosure?: (nodeId: string, index: number) => void;
}

/** Edge color by meaning: true (green), false/abort (red), loop-back. */
function edgeColor(e: FlowEdge): string {
  if (e.label === 'true') return 'var(--ok)';
  if (e.label === 'false' || e.label === 'abort') return 'var(--err)';
  if (e.back) return 'var(--accent-2)';
  return 'var(--text-3)';
}

/** Fit the whole graph if it stays legible, else zoom onto the entry node. */
export function focusReadable(
  rf: ReturnType<typeof useReactFlow>,
  pos: Map<string, { x: number; y: number }>,
  sizes: Map<string, { width: number; height: number }>,
  anchor: string | undefined,
  el: HTMLElement | null,
  minReadable = 0.55,
  top = true,
) {
  let maxX = 0, maxY = 0;
  for (const [id, p] of pos) {
    const sz = sizes.get(id);
    maxX = Math.max(maxX, p.x + (sz?.width ?? 0));
    maxY = Math.max(maxY, p.y + (sz?.height ?? 0));
  }
  const w = el?.clientWidth ?? 800;
  const h = el?.clientHeight ?? 600;
  const fitZoom = Math.min(w / (maxX + 40), h / (maxY + 40));
  const a = anchor ? pos.get(anchor) : undefined;
  if (fitZoom >= minReadable || !a || pos.size <= 12) {
    rf.fitView({ padding: 0.12, duration: 300, maxZoom: 1.1 });
    return;
  }
  const zoom = 0.85;
  const sz = sizes.get(anchor!)!;
  rf.setViewport({ x: w / 2 - (a.x + sz.width / 2) * zoom, y: top ? 40 - a.y * zoom : h / 2 - (a.y + sz.height / 2) * zoom, zoom }, { duration: 300 });
}

/** Renders a control-flow graph laid out top-down with ELK. */
function Inner({ flow, notes, selected, highlight, onSelect, onOpen, onClosure }: FlowCanvasProps) {
  const rf = useReactFlow();
  const hostRef = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<Map<string, { x: number; y: number }>>();
  const sizes = useMemo(() => new Map(flow.nodes.map((n) => [n.id, flowSize(n, notes?.get(n.id)?.text)])), [flow, notes]);

  useEffect(() => {
    let alive = true;
    layout(
      flow.nodes.map((n) => ({ id: n.id, ...sizes.get(n.id)! })),
      flow.edges.map((e, i) => ({ id: `e${i}`, source: e.from, target: e.to })),
      'DOWN',
      { spacing: 30, layerSpacing: 46 },
    ).then((p) => {
      if (!alive) return;
      setPos(p);
      setTimeout(() => focusReadable(rf, p, sizes, flow.nodes[0]?.id, hostRef.current), 80);
    });
    return () => {
      alive = false;
    };
  }, [flow, sizes, rf]);

  const nodes: Node[] = useMemo(() => {
    if (!pos) return [];
    return flow.nodes.map((n) => {
      const sz = sizes.get(n.id)!;
      const note = notes?.get(n.id);
      const data: FNodeData = {
        node: n,
        selected: n.id === selected,
        dim: highlight ? !highlight.has(n.id) : false,
        note: note?.text,
        stepNo: note?.n,
        onClosure: onClosure && n.closures?.length ? (i: number) => onClosure(n.id, i) : undefined,
      };
      return { id: n.id, type: 'flow', position: pos.get(n.id) ?? { x: 0, y: 0 }, data, width: sz.width, height: sz.height, style: { width: sz.width, height: sz.height } };
    });
  }, [pos, flow, sizes, notes, selected, highlight, onClosure]);

  const edges: Edge[] = useMemo(
    () =>
      flow.edges.map((e, i) => {
        const color = edgeColor(e);
        return {
          id: `e${i}`,
          source: e.from,
          target: e.to,
          type: e.back ? 'smoothstep' : 'default',
          label: e.label,
          labelBgPadding: [4, 2] as [number, number],
          labelBgBorderRadius: 4,
          animated: highlight ? highlight.has(e.from) && highlight.has(e.to) : false,
          style: { stroke: color, strokeWidth: 1.6, strokeDasharray: e.back ? '5 4' : undefined, opacity: highlight && !(highlight.has(e.from) && highlight.has(e.to)) ? 0.2 : 1 },
          markerEnd: { type: MarkerType.ArrowClosed, color, width: 15, height: 15 },
        };
      }),
    [flow.edges, highlight],
  );

  return (
    <div className="graph-host" ref={hostRef}>
      <div className="graph-legend">
        <span><i style={{ background: 'var(--ok)' }} /> true</span>
        <span><i style={{ background: 'var(--err)' }} /> false / abort</span>
        <span><i style={{ background: 'var(--accent-2)' }} /> loop back</span>
      </div>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={flowNodeTypes}
        proOptions={{ hideAttribution: true }}
        minZoom={0.1}
        maxZoom={2}
        nodesConnectable={false}
        onNodeClick={(_, n) => onSelect?.(n.id)}
        onNodeDoubleClick={(_, n) => onOpen?.(n.id)}
        fitView
      >
        <Background variant={BackgroundVariant.Dots} gap={22} size={1.2} color="var(--grid)" />
        <Controls showInteractive={false} position="bottom-left" />
      </ReactFlow>
    </div>
  );
}

/** Control-flow graph canvas (wraps React Flow's provider). */
export function FlowCanvas(props: FlowCanvasProps) {
  return (
    <ReactFlowProvider>
      <Inner {...props} />
    </ReactFlowProvider>
  );
}
