import { useEffect, useMemo, useRef, useState } from 'react';
import { focusReadable } from './FlowCanvas';
import { Background, BackgroundVariant, Controls, MiniMap, ReactFlow, ReactFlowProvider, useReactFlow, type Edge, type Node } from '@xyflow/react';
import { layout, toElkDirection } from '../lib/layout';
import { useStore } from '../lib/store';
import { heatColor } from '../lib/themes';
import type { Graph, GraphNode } from '../lib/types';
import { graphNodeTypes, nodeTypeFor, sizeOf, type GNodeData } from './nodes';
import { edgeLegend, edgeStyles, toFlowEdge } from './edges';

export interface GraphCanvasProps {
  graph: Graph;
  direction?: string;
  selected?: string;
  /** Node id → 0..1 heat used for the node's heat bar. */
  heat?: Record<string, number>;
  onSelect?: (n: GraphNode) => void;
  onOpen?: (n: GraphNode) => void;
  legend?: boolean;
  /** Layer spacing tweak for dense graphs. */
  compact?: boolean;
}

/**
 * Renders a graph with ELK layout, hover highlighting of neighbours,
 * heat bars, a legend and a minimap for larger graphs.
 */
function Inner({ graph, direction, selected, heat, onSelect, onOpen, legend = true, compact }: GraphCanvasProps) {
  const theme = useStore((s) => s.theme);
  const rf = useReactFlow();
  const hostRef = useRef<HTMLDivElement>(null);
  const [positions, setPositions] = useState<Map<string, { x: number; y: number }>>();
  const [hover, setHover] = useState<string>();
  const dir = toElkDirection(direction ?? graph.direction);
  const horizontal = dir === 'RIGHT' || dir === 'LEFT';

  const sizes = useMemo(() => new Map(graph.nodes.map((n) => [n.id, sizeOf(n)])), [graph]);

  useEffect(() => {
    let alive = true;
    setPositions(undefined);
    layout(
      graph.nodes.map((n) => ({ id: n.id, ...sizes.get(n.id)! })),
      graph.edges.map((e) => ({ id: e.id, source: e.from, target: e.to })),
      dir,
      compact ? { spacing: 18, layerSpacing: 50 } : { spacing: 26, layerSpacing: graph.kind === 'deps' ? 60 : 80 },
    ).then((p) => {
      if (!alive) return;
      setPositions(p);
      setTimeout(() => focusReadable(rf, p, sizes, graph.root ?? graph.nodes.find((n) => n.root)?.id, hostRef.current, 0.4, false), 80);
    });
    return () => {
      alive = false;
    };
  }, [graph, dir, sizes, rf, compact]);

  const neighbours = useMemo(() => {
    const focus = hover ?? selected;
    if (!focus) return undefined;
    const set = new Set([focus]);
    for (const e of graph.edges) {
      if (e.from === focus) set.add(e.to);
      if (e.to === focus) set.add(e.from);
    }
    return { focus, set };
  }, [hover, selected, graph.edges]);

  const nodes: Node[] = useMemo(() => {
    if (!positions) return [];
    return graph.nodes.map((n) => {
      const sz = sizes.get(n.id)!;
      const h = heat?.[n.id] ?? n.heat;
      const data: GNodeData = {
        node: n,
        horizontal,
        dim: neighbours ? !neighbours.set.has(n.id) : false,
        selected: n.id === selected,
        heatColor: h && h > 0 ? heatColor(theme, h) : undefined,
      };
      return {
        id: n.id,
        type: nodeTypeFor(n),
        position: positions.get(n.id) ?? { x: 0, y: 0 },
        data,
        width: sz.width,
        height: sz.height,
        style: { width: sz.width, height: sz.height },
        draggable: true,
        selectable: true,
      };
    });
  }, [positions, graph.nodes, sizes, heat, horizontal, neighbours, selected, theme]);

  const edges: Edge[] = useMemo(
    () =>
      graph.edges.map((e) =>
        toFlowEdge(e, {
          highlight: neighbours ? e.from === neighbours.focus || e.to === neighbours.focus : false,
          dim: neighbours ? !(e.from === neighbours.focus || e.to === neighbours.focus) : false,
        }),
      ),
    [graph.edges, neighbours],
  );

  const kinds = useMemo(() => new Set(graph.edges.map((e) => e.kind)), [graph.edges]);
  const byId = useMemo(() => new Map(graph.nodes.map((n) => [n.id, n])), [graph.nodes]);

  return (
    <div className="graph-host" ref={hostRef}>
      {legend && kinds.size > 1 && (
        <div className="graph-legend">
          {[...kinds].filter((k) => (edgeLegend as readonly string[]).includes(k) || edgeStyles[k]).map((k) => (
            <span key={k}>
              <i style={{ background: edgeStyles[k]?.color ?? 'var(--text-3)', height: 2, ...(edgeStyles[k]?.dash ? { backgroundImage: 'none', borderTop: `2px dashed ${edgeStyles[k].color}`, background: 'transparent' } : {}) }} />
              {k}
            </span>
          ))}
        </div>
      )}
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={graphNodeTypes}
        minZoom={0.08}
        maxZoom={2.2}
        proOptions={{ hideAttribution: true }}
        onNodeMouseEnter={(_, n) => setHover(n.id)}
        onNodeMouseLeave={() => setHover(undefined)}
        onNodeClick={(_, n) => {
          const g = byId.get(n.id);
          if (g) onSelect?.(g);
        }}
        onNodeDoubleClick={(_, n) => {
          const g = byId.get(n.id);
          if (g) onOpen?.(g);
        }}
        nodesConnectable={false}
        elevateEdgesOnSelect
        fitView
      >
        <Background variant={BackgroundVariant.Dots} gap={22} size={1.2} color="var(--grid)" />
        <Controls showInteractive={false} position="bottom-left" />
        {graph.nodes.length > 12 && (
          <MiniMap
            pannable
            zoomable
            position="bottom-right"
            nodeColor={(n) => ((n.data as GNodeData).node.root ? theme.accent : (n.data as GNodeData).heatColor ?? theme.bg3)}
            maskColor="rgba(0,0,0,0.35)"
            style={{ width: 170, height: 110 }}
          />
        )}
      </ReactFlow>
    </div>
  );
}

/** Interactive graph canvas (wraps React Flow's provider). */
export function GraphCanvas(props: GraphCanvasProps) {
  return (
    <ReactFlowProvider>
      <Inner {...props} />
    </ReactFlowProvider>
  );
}
