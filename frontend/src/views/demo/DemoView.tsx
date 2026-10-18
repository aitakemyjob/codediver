import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Check, ChevronDown, Clapperboard, Code2, Crosshair, Download, FileImage, FileText, Image, Maximize2, Minus, Pause, Play, Plus, RotateCcw, SkipBack, SkipForward, Sun } from 'lucide-react';
import { api } from '../../lib/api';
import { actions } from '../../lib/actions';
import { useStore } from '../../lib/store';
import { Empty, Markdown, Spinner, useAsync, useSize } from '../../components/ui';
import { Menu } from '../../components/Menu';
import type { Demo } from '../../lib/types';
import type { ViewProps } from '../ViewHost';
import { cameraFor, followTargets, frameAt, groupsOf, lerpRect, panBy, stepDuration, zoomAt, type Frame, type Rect } from './demo';
import { buildScene, LANE_HEAD, type Scene as Laid } from './scene';
import { SceneArt } from './SceneArt';
import { fileName, renderPdf, renderStepPng, renderStepSvg, saveFile } from './export';

const speeds = [0.5, 1, 2] as const;

/**
 * Plays an agent's demo: an animated scene (architecture lanes or an
 * execution tree) with step narration, code and playback controls.
 */
export default function DemoView({ tab, active }: ViewProps) {
  const s = useStore();
  const id = String(tab.params.id ?? '');
  const demo = (s.session.demos ?? []).find((d) => d.id === id);
  if (!demo) {
    return (
      <Empty icon={<Clapperboard size={28} />} title="Demo not found">
        Demos live for the current session; ask the agent to show it again.
      </Empty>
    );
  }
  return <DemoPlayer key={demo.id} demo={demo} active={active} />;
}

/** The player for one demo. */
function DemoPlayer({ demo, active }: { demo: Demo; active: boolean }) {
  const [step, setStep] = useState(0);
  const [playing, setPlaying] = useState(true);
  const [speed, setSpeed] = useState<(typeof speeds)[number]>(1);
  const [laid, setLaid] = useState<Laid>();
  const [layoutError, setLayoutError] = useState('');
  const last = demo.steps.length - 1;
  const frame = useMemo(() => frameAt(demo, step), [demo, step]);
  const groups = useMemo(() => groupsOf(demo), [demo]);

  // Lay the scene out once; nodes keep their places as they are revealed.
  useEffect(() => {
    let alive = true;
    buildScene(demo, groups).then(
      (out) => alive && setLaid(out),
      (err) => alive && setLayoutError(String(err?.message ?? err)),
    );
    return () => {
      alive = false;
    };
  }, [demo, groups]);

  // Autoplay: advance after the step's reading time, stop at the end.
  useEffect(() => {
    if (!playing) return;
    if (step >= last) {
      const t = setTimeout(() => setPlaying(false), stepDuration(demo, step) / speed);
      return () => clearTimeout(t);
    }
    const t = setTimeout(() => setStep((i) => Math.min(last, i + 1)), stepDuration(demo, step) / speed);
    return () => clearTimeout(t);
  }, [playing, step, speed, demo, last]);

  const go = useCallback((i: number) => {
    setPlaying(false);
    setStep(Math.max(0, Math.min(last, i)));
  }, [last]);

  // Keyboard: ← → to step, space to play/pause.
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof Element && e.target.closest('input, textarea, .monaco-editor')) return;
      if (e.key === 'ArrowRight') go(step + 1);
      else if (e.key === 'ArrowLeft') go(step - 1);
      else if (e.key === ' ') {
        e.preventDefault();
        setPlaying((p) => !p);
      } else return;
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [active, go, step]);

  const current = demo.steps[step];
  const exporter = useExport(demo, step);
  return (
    <>
      <div className="toolbar">
        <Clapperboard size={16} style={{ color: 'var(--accent)' }} />
        <span className="title ellipsis" style={{ maxWidth: 360 }}>{demo.title}</span>
        <div className="row" style={{ gap: 4 }}>
          <button className="btn icon sm" title="Restart" onClick={() => { setStep(0); setPlaying(true); }}><RotateCcw size={13} /></button>
          <button className="btn icon sm" title="Previous step (←)" onClick={() => go(step - 1)} disabled={step === 0}><SkipBack size={13} /></button>
          <button className="btn sm primary" title="Play / pause (space)" onClick={() => (step >= last && !playing ? (setStep(0), setPlaying(true)) : setPlaying(!playing))} style={{ width: 74 }}>
            {playing ? <><Pause size={13} /> Pause</> : <><Play size={13} /> Play</>}
          </button>
          <button className="btn icon sm" title="Next step (→)" onClick={() => go(step + 1)} disabled={step >= last}><SkipForward size={13} /></button>
        </div>
        <div className="seg">
          {speeds.map((x) => (
            <button key={x} className={speed === x ? 'on' : ''} onClick={() => setSpeed(x)}>{x}×</button>
          ))}
        </div>
        <div className="demo-timeline grow" role="tablist" aria-label="Steps">
          {demo.steps.map((st, i) => (
            <button key={i} role="tab" aria-selected={i === step} title={`${i + 1}. ${st.title}`} className={i === step ? 'on' : i < step ? 'done' : ''} onClick={() => go(i)} />
          ))}
        </div>
        <span className="mono faint">{step + 1}/{demo.steps.length}</span>
        {exporter.menu}
      </div>
      <div className="split">
        <div className="main demo-stage">
          {laid ? <Scene demo={demo} frame={frame} boxes={laid.boxes} routes={laid.routes} groups={groups} step={step} speed={speed} active={active} /> : layoutError ? <Empty icon={<Clapperboard size={28} />} title="Couldn't lay out this demo">{layoutError}</Empty> : <Spinner label="Laying out…" />}
        </div>
        <div className="side right demo-panel" style={{ width: 380 }}>
          <div className="scroll" style={{ padding: 16 }}>
            {step === 0 && demo.summary && <Markdown src={demo.summary} className="demo-summary" />}
            <div className="faint mono" style={{ fontSize: 11 }}>STEP {step + 1} OF {demo.steps.length}</div>
            <h3 className="demo-step-title">{current?.title}</h3>
            {current?.narration && <Markdown src={current.narration} />}
            {current?.code && <CodeExcerpt file={current.code.file} line={current.code.line ?? 1} end={current.code.end_line ?? current.code.line ?? 1} />}
            {Object.keys(frame.values).length > 0 && (
              <>
                <div className="section-title" style={{ paddingLeft: 0 }}>Values</div>
                {Object.entries(frame.values).map(([nid, v]) => (
                  <div key={nid} className={`demo-value-row ${v.fresh ? 'fresh' : ''}`}>
                    <span className="ellipsis">{demo.nodes.find((n) => n.id === nid)?.label ?? nid}</span>
                    <code>{v.text}</code>
                  </div>
                ))}
              </>
            )}
          </div>
        </div>
      </div>
    </>
  );
}

/** The SVG scene: lanes, edges, animated packets, nodes and values. */
function Scene({ demo, frame, boxes, routes, groups, step, speed, active }: { demo: Demo; frame: Frame; boxes: Laid['boxes']; routes: Laid['routes']; groups: string[]; step: number; speed: number; active: boolean }) {
  const [hostRef, size] = useSize<HTMLDivElement>();
  const aspect = size.width && size.height ? size.width / size.height : 16 / 10;
  // Frame a set of nodes, leaving room for lane headers and value tags.
  const frameNodes = useCallback(
    (ids: string[], minW: number) =>
      cameraFor(
        ids.filter((id) => boxes.has(id)).map((id) => boxes.get(id)!).map((b) => ({ x: b.x, y: b.y - LANE_HEAD, w: b.w, h: b.h + LANE_HEAD + b.foot + 8 })),
        aspect,
        60,
        minW,
      ),
    [boxes, aspect],
  );
  // Follow mode frames the step's action; manual mode keeps the user's view.
  const [manual, setManual] = useState<Rect | null>(null);
  const follow = useMemo(() => frameNodes(followTargets(frame), 520), [frame, frameNodes]);
  const instant = useRef(false);
  const camera = useCamera(manual ?? follow, instant);
  const setView = (r: Rect, now = false) => {
    instant.current = now;
    setManual(r);
  };
  const fitAll = () => setView(frameNodes([...frame.visible], 400));
  const zoomBy = (factor: number) => setView(zoomAt(camera, factor, 0.5, 0.5));

  // Wheel zooms toward the cursor; it needs a non-passive listener.
  useEffect(() => {
    const el = hostRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const r = el.getBoundingClientRect();
      instant.current = true;
      setManual((m) => zoomAt(m ?? cameraRef.current, Math.exp(e.deltaY * 0.0015), (e.clientX - r.left) / r.width, (e.clientY - r.top) / r.height));
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [hostRef]);
  const cameraRef = useRef(camera);
  cameraRef.current = camera;

  // Drag to pan.
  const drag = useRef<{ x: number; y: number } | null>(null);
  const onPointerDown = (e: React.PointerEvent) => {
    if ((e.target as Element).closest('.demo-node.link, .demo-zoom')) return;
    drag.current = { x: e.clientX, y: e.clientY };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  };
  const onPointerMove = (e: React.PointerEvent) => {
    if (!drag.current || !size.width) return;
    const dx = e.clientX - drag.current.x;
    const dy = e.clientY - drag.current.y;
    drag.current = { x: e.clientX, y: e.clientY };
    setView(panBy(cameraRef.current, dx, dy, size.width, size.height), true);
  };
  const onPointerUp = () => {
    drag.current = null;
  };

  // Keys: + / - zoom, 0 fits everything, f follows the action again.
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof Element && e.target.closest('input, textarea, .monaco-editor')) return;
      if (e.key === '+' || e.key === '=') setView(zoomAt(cameraRef.current, 0.8, 0.5, 0.5));
      else if (e.key === '-' || e.key === '_') setView(zoomAt(cameraRef.current, 1.25, 0.5, 0.5));
      else if (e.key === '0') fitAll();
      else if (e.key === 'f') setManual(null);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
  return (
    <div ref={hostRef} className={`demo-viewport ${drag.current ? 'dragging' : ''}`} onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={onPointerUp}>
    <div className="demo-zoom" role="toolbar" aria-label="Zoom">
      <button className="btn icon sm" title="Zoom in (+)" onClick={() => zoomBy(0.75)}><Plus size={13} /></button>
      <button className="btn icon sm" title="Zoom out (−)" onClick={() => zoomBy(1.33)}><Minus size={13} /></button>
      <button className="btn icon sm" title="Fit everything shown so far (0)" onClick={fitAll}><Maximize2 size={13} /></button>
      <button className={`btn sm ${manual ? '' : 'active'}`} title="Follow the action: frame each step's highlighted nodes (F)" onClick={() => setManual(null)}><Crosshair size={13} /> Follow</button>
    </div>
    <SceneArt demo={demo} frame={frame} boxes={boxes} routes={routes} groups={groups} step={step} speed={speed} camera={camera} />
    </div>
  );
}

/**
 * The Export menu: this step as PNG or SVG, or the whole demo as a PDF,
 * in the light theme (for docs and print) or the current one.
 */
function useExport(demo: Demo, step: number) {
  const toast = useStore((s) => s.toast);
  const [light, setLight] = useState(true);
  const [busy, setBusy] = useState('');
  const run = async (label: string, make: () => Promise<{ blob: Blob; name: string }>) => {
    setBusy(label);
    try {
      const { blob, name } = await make();
      const path = await saveFile(name, blob);
      if (path) toast({ kind: 'ok', title: `Exported ${label}`, body: path });
    } catch (err) {
      toast({ kind: 'err', title: `Couldn't export ${label}`, body: String((err as Error)?.message ?? err) });
    } finally {
      setBusy('');
    }
  };
  const theme = light ? 'light' : 'current';
  const stepName = (ext: string) => fileName(`${demo.title} step ${step + 1}`, ext);
  const menu = (
    <Menu
      align="right"
      width={250}
      trigger={() => (
        <button className="btn sm" title="Export as an image or PDF" disabled={!!busy}>
          {busy ? <Spinner /> : <Download size={13} />} {busy ? `Exporting ${busy}…` : 'Export'} <ChevronDown size={12} className="faint" />
        </button>
      )}
      items={[
        { key: 'png', label: 'PNG image', hint: 'this step', icon: <Image size={14} />, onSelect: () => run('PNG', async () => ({ blob: (await renderStepPng(demo, step, theme)).blob, name: stepName('png') })) },
        { key: 'svg', label: 'SVG vector', hint: 'this step', icon: <FileImage size={14} />, onSelect: () => run('SVG', async () => ({ blob: new Blob([(await renderStepSvg(demo, step, theme)).svg], { type: 'image/svg+xml' }), name: stepName('svg') })) },
        { key: 'pdf', label: 'PDF document', hint: 'all steps', icon: <FileText size={14} />, onSelect: () => run('PDF', async () => ({ blob: await renderPdf(demo, theme), name: fileName(demo.title, 'pdf') })) },
        { key: 'sep', label: '', separator: true },
        { key: 'light', label: 'Light theme', hint: 'for print', icon: light ? <Check size={14} /> : <Sun size={14} />, active: light, onSelect: () => setLight(!light) },
      ]}
    />
  );
  return { menu };
}

/**
 * Eases the camera to a new target over ~0.6 s, or jumps straight there
 * when `instant` is set (wheel and drag must track the hand exactly).
 */
function useCamera(target: Rect, instant: React.MutableRefObject<boolean>): Rect {
  const [cam, setCam] = useState(target);
  const from = useRef(target);
  const current = useRef(target);
  useEffect(() => {
    if (instant.current) {
      instant.current = false;
      current.current = target;
      setCam(target);
      return;
    }
    from.current = current.current;
    const start = performance.now();
    let raf = 0;
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / 600);
      const eased = 1 - Math.pow(1 - t, 3);
      current.current = lerpRect(from.current, target, eased);
      setCam(current.current);
      if (t < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [target, instant]);
  return cam;
}

/** A few lines of code around a range, with the range highlighted. */
function CodeExcerpt({ file, line, end }: { file: string; line: number; end: number }) {
  const s = useStore();
  const fc = useAsync(() => api.read(s.repoId, file), [s.repoId, file]);
  const lines = fc.data?.content.split('\n') ?? [];
  const from = Math.max(1, line - 2);
  const to = Math.min(lines.length, end + 2, from + 24);
  return (
    <div className="demo-code">
      <div className="row demo-code-head">
        <span className="mono ellipsis grow">{file}:{line}{end > line ? `-${end}` : ''}</span>
        <button className="btn ghost sm" onClick={() => actions.code(file, line, end)}><Code2 size={12} /> Open</button>
      </div>
      {fc.loading && !fc.data ? (
        <Spinner />
      ) : (
        <pre>
          {lines.slice(from - 1, to).map((l, i) => {
            const n = from + i;
            return (
              <div key={n} className={n >= line && n <= end ? 'hl' : ''}>
                <span className="ln">{n}</span>
                {l || ' '}
              </div>
            );
          })}
        </pre>
      )}
    </div>
  );
}
