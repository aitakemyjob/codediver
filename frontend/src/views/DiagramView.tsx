import { useEffect, useRef, useState } from 'react';
import { Map as MapIcon } from 'lucide-react';
import { actions } from '../lib/actions';
import { useStore } from '../lib/store';
import { mermaidVars } from '../lib/themes';
import { GraphCanvas } from '../graph/GraphCanvas';
import { Empty, ErrorBox, Markdown, Spinner } from '../components/ui';
import type { ViewProps } from './ViewHost';

let mermaidSeq = 0;

/**
 * Renders Mermaid source with the current theme; shows the source if it
 * fails to parse.
 */
function Mermaid({ source }: { source: string }) {
  const theme = useStore((s) => s.theme);
  const ref = useRef<HTMLDivElement>(null);
  const [err, setErr] = useState<string>();
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    let alive = true;
    setLoading(true);
    import('mermaid').then(async ({ default: mermaid }) => {
      mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: 'base', themeVariables: mermaidVars(theme), flowchart: { curve: 'basis', htmlLabels: true }, sequence: { mirrorActors: false } });
      try {
        const { svg } = await mermaid.render(`gd-mermaid-${++mermaidSeq}`, source);
        if (alive && ref.current) {
          ref.current.innerHTML = svg;
          setErr(undefined);
        }
      } catch (e) {
        if (alive) setErr((e as Error).message);
      } finally {
        if (alive) setLoading(false);
      }
    });
    return () => {
      alive = false;
    };
  }, [source, theme]);
  return (
    <>
      {err && <ErrorBox error={`Mermaid could not render this diagram: ${err}`} />}
      <div className="mermaid-host" ref={ref} />
      {loading && <Spinner />}
      {err && <pre className="snippet" style={{ margin: 16 }}>{source}</pre>}
    </>
  );
}

/** Shows an agent diagram: Mermaid or an interactive node/edge graph. */
export default function DiagramView({ tab }: ViewProps) {
  const s = useStore();
  const d = s.session.diagrams.find((x) => x.id === tab.params.id);
  if (!d) return <Empty icon={<MapIcon size={28} />} title="Diagram not found">Diagrams live for this session only.</Empty>;
  return (
    <>
      <div className="toolbar">
        <MapIcon size={16} style={{ color: 'var(--accent)' }} />
        <span className="title">{d.title}</span>
        <span className="badge">{d.mermaid ? 'mermaid' : `${d.graph?.nodes.length} nodes`}</span>
      </div>
      {d.description && (
        <div className="narration">
          <Markdown src={d.description} />
        </div>
      )}
      {d.mermaid ? (
        <Mermaid source={d.mermaid} />
      ) : d.graph ? (
        <GraphCanvas graph={d.graph} direction={d.graph.direction || 'LR'} onOpen={(n) => n.file && actions.code(n.file, n.line)} onSelect={(n) => n.file && actions.code(n.file, n.line)} />
      ) : null}
    </>
  );
}
