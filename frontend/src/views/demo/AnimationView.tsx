import { useEffect, useMemo, useRef, useState } from 'react';
import { Code2, Download, Film, Minus, Plus, RotateCcw, ShieldCheck } from 'lucide-react';
import { useStore } from '../../lib/store';
import { Empty, Markdown } from '../../components/ui';
import type { Animation } from '../../lib/types';
import type { ViewProps } from '../ViewHost';
import { sandboxDocument } from './sandbox';
import { fileName, saveFile } from './export';

/**
 * Shows an agent's free-form SVG/HTML animation inside a locked-down
 * iframe: scripts may run, but with an opaque origin and a CSP that blocks
 * every network request, so the animation can't reach CodeDiver's API,
 * the developer's files or the internet.
 */
export default function AnimationView({ tab }: ViewProps) {
  const s = useStore();
  const id = String(tab.params.id ?? '');
  const anim = (s.session.animations ?? []).find((a) => a.id === id);
  if (!anim) {
    return (
      <Empty icon={<Film size={28} />} title="Animation not found">
        Animations live for the current session; ask the agent to show it again.
      </Empty>
    );
  }
  return <Player key={anim.id} anim={anim} />;
}

/** The frame, caption and replay/source controls for one animation. */
function Player({ anim }: { anim: Animation }) {
  const theme = useStore((st) => st.theme);
  const [run, setRun] = useState(0);
  const [showSource, setShowSource] = useState(false);
  const toast = useStore((st) => st.toast);
  // The saved file is the same sandboxed document: it plays in any browser
  // and still can't reach the network.
  const saveHtml = async () => {
    try {
      const path = await saveFile(fileName(anim.title, 'html'), new Blob([doc], { type: 'text/html' }));
      if (path) toast({ kind: 'ok', title: 'Saved animation', body: path });
    } catch (err) {
      toast({ kind: 'err', title: "Couldn't save the animation", body: String((err as Error)?.message ?? err) });
    }
  };
  const frame = useRef<HTMLIFrameElement>(null);
  const [zoom, setZoom] = useState(1);
  const sendZoom = (z: number) => frame.current?.contentWindow?.postMessage({ codediverZoom: z }, '*');
  // The animation reports zoom changes (including its own Ctrl+wheel).
  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      if (e.source === frame.current?.contentWindow && typeof e.data?.codediverZoom === 'number') setZoom(e.data.codediverZoom);
    };
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, []);
  // Rebuild when the theme changes so the animation picks up its colors.
  const doc = useMemo(() => sandboxDocument(anim, getComputedStyle(document.documentElement)), [anim, theme]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <>
      <div className="toolbar">
        <Film size={16} style={{ color: 'var(--accent)' }} />
        <span className="title ellipsis" style={{ maxWidth: 420 }}>{anim.title}</span>
        <span className="badge">{anim.format}</span>
        <span className="row faint" style={{ gap: 4 }} title="Runs with no network access and no access to CodeDiver or your files">
          <ShieldCheck size={13} style={{ color: 'var(--ok)' }} /> sandboxed
        </span>
        <div className="grow" />
        <div className="row" style={{ gap: 2 }} role="toolbar" aria-label="Zoom">
          <button className="btn icon sm" title="Zoom out (Ctrl+wheel)" onClick={() => sendZoom(zoom / 1.25)}><Minus size={13} /></button>
          <button className="btn sm" title="Reset zoom" style={{ minWidth: 54 }} onClick={() => sendZoom(1)}>{Math.round(zoom * 100)}%</button>
          <button className="btn icon sm" title="Zoom in (Ctrl+wheel)" onClick={() => sendZoom(zoom * 1.25)}><Plus size={13} /></button>
        </div>
        <button className="btn sm" onClick={() => setRun((r) => r + 1)}><RotateCcw size={13} /> Replay</button>
        <button className="btn sm" title="Save as a standalone HTML file that plays in any browser" onClick={saveHtml}><Download size={13} /> Save HTML</button>
        <button className={`btn sm ${showSource ? 'active' : ''}`} onClick={() => setShowSource(!showSource)}><Code2 size={13} /> Source</button>
      </div>
      <div className="split">
        <div className="main" style={{ padding: 12, gap: 10 }}>
          <iframe ref={frame} key={run} onLoad={() => zoom !== 1 && sendZoom(zoom)} className="anim-frame" title={anim.title} sandbox="allow-scripts" srcDoc={doc} referrerPolicy="no-referrer" />
          {anim.caption && <Markdown src={anim.caption} className="anim-caption" />}
        </div>
        {showSource && (
          <div className="side right" style={{ width: 460 }}>
            <pre className="anim-source">{anim.source}</pre>
          </div>
        )}
      </div>
    </>
  );
}
