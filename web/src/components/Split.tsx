import { type CSSProperties, type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { cx } from "./ui";

type Pane = "left" | "right";

/**
 * Two panes. From the `md` breakpoint they sit side by side with a draggable
 * divider whose position is remembered. Below it there is no room for both:
 * each pane takes the full width and two tabs switch between them.
 *
 * Both panes stay mounted at every width. The pane behind the other tab is
 * hidden with `visibility`, not unmounted and not `display: none`, so a
 * terminal in it keeps its connection, its scrollback and its size.
 */
export function Split({ left, right, storageKey = "kubedojo.split", labels }: {
  left: ReactNode; right: ReactNode; storageKey?: string;
  /** Names of the two tabs on a narrow screen. */
  labels?: { left: string; right: string };
}) {
  const [pct, setPct] = useState(() => {
    try { return Number(localStorage.getItem(storageKey)) || 40; } catch { return 40; }
  });
  const [tab, setTab] = useState<Pane>("left");
  const wrap = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);
  const [active, setActive] = useState(false);
  const names = labels ?? { left: t("Task"), right: t("Terminal") };

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

  const onTabKey = (e: React.KeyboardEvent) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    e.preventDefault();
    const to: Pane = tab === "left" ? "right" : "left";
    setTab(to);
    document.getElementById(`split-tab-${to}`)?.focus();
  };
  const tabButton = (pane: Pane) => (
    <button role="tab" id={`split-tab-${pane}`} aria-selected={tab === pane} aria-controls={`split-pane-${pane}`}
      tabIndex={tab === pane ? 0 : -1} onClick={() => setTab(pane)} onKeyDown={onTabKey}
      className={cx("relative h-10 flex-1 px-3 text-sm font-medium transition-colors", tab === pane ? "text-text" : "text-muted hover:text-text")}>
      {names[pane]}
      {tab === pane && <span aria-hidden className="absolute inset-x-0 -bottom-px h-0.5 bg-accent" />}
    </button>
  );
  // Narrow: the pane fills the area, and only the selected one is visible. Wide: back in the row.
  const pane = (which: Pane) => cx("absolute inset-0 min-w-0 overflow-hidden md:static", tab !== which && "max-md:invisible");

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div role="tablist" aria-label={t("Panes")} className="flex shrink-0 border-b bg-bg md:hidden">
        {tabButton("left")}{tabButton("right")}
      </div>
      <div ref={wrap} className={cx("relative flex min-h-0 flex-1", active && "select-none")} style={{ "--split": `${pct}%` } as CSSProperties}>
        <div id="split-pane-left" className={cx(pane("left"), "md:w-(--split) md:shrink-0")}>{left}</div>
        <div
          role="separator" aria-orientation="vertical" tabIndex={0}
          onPointerDown={(e) => { dragging.current = true; setActive(true); e.preventDefault(); }}
          onKeyDown={(e) => { if (e.key === "ArrowLeft") setPct((p) => Math.max(22, p - 2)); if (e.key === "ArrowRight") setPct((p) => Math.min(70, p + 2)); }}
          className={cx("group relative hidden w-1.5 shrink-0 cursor-col-resize border-x bg-panel2 transition-colors hover:bg-accent/40 md:block", active && "bg-accent/60")}
        >
          <span className="absolute left-1/2 top-1/2 h-8 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded bg-muted/50 group-hover:bg-accent" />
        </div>
        <div id="split-pane-right" className={cx(pane("right"), "md:flex-1", active && "pointer-events-none")}>{right}</div>
      </div>
    </div>
  );
}
