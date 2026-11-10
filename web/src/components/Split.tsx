import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

/** Two resizable panes side by side; the split position is remembered. */
export function Split({ left, right, storageKey = "kubedojo.split" }: { left: ReactNode; right: ReactNode; storageKey?: string }) {
  const [pct, setPct] = useState(() => {
    try { return Number(localStorage.getItem(storageKey)) || 40; } catch { return 40; }
  });
  const wrap = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);
  const [active, setActive] = useState(false);

  const onMove = useCallback((e: PointerEvent) => {
    if (!dragging.current || !wrap.current) return;
    const r = wrap.current.getBoundingClientRect();
    setPct(Math.min(70, Math.max(22, ((e.clientX - r.left) / r.width) * 100)));
  }, []);
  const stop = useCallback(() => { dragging.current = false; setActive(false); }, []);

  useEffect(() => {
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", stop);
    return () => { window.removeEventListener("pointermove", onMove); window.removeEventListener("pointerup", stop); };
  }, [onMove, stop]);
  useEffect(() => { try { localStorage.setItem(storageKey, String(Math.round(pct))); } catch { /* ignore */ } }, [pct, storageKey]);

  return (
    <div ref={wrap} className={"flex min-h-0 flex-1 " + (active ? "select-none" : "")}>
      <div className="min-w-0 overflow-hidden" style={{ width: `${pct}%` }}>{left}</div>
      <div
        role="separator" aria-orientation="vertical" tabIndex={0}
        onPointerDown={(e) => { dragging.current = true; setActive(true); e.preventDefault(); }}
        onKeyDown={(e) => { if (e.key === "ArrowLeft") setPct((p) => Math.max(22, p - 2)); if (e.key === "ArrowRight") setPct((p) => Math.min(70, p + 2)); }}
        className={"group relative w-1.5 shrink-0 cursor-col-resize border-x bg-panel2 transition-colors hover:bg-accent/40 " + (active ? "bg-accent/60" : "")}
      >
        <span className="absolute left-1/2 top-1/2 h-8 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded bg-muted/50 group-hover:bg-accent" />
      </div>
      <div className={"min-w-0 flex-1 overflow-hidden " + (active ? "pointer-events-none" : "")}>{right}</div>
    </div>
  );
}
