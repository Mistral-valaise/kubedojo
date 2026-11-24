import { type CSSProperties, type ReactNode, type RefObject, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { cx } from "./ui";

export interface Pane {
  /** Stable name of the pane. Its elements get the ids `split-tab-<id>` and `split-pane-<id>`. */
  id: string;
  /** Name of the pane on its tab. */
  label: string;
  content: ReactNode;
  /** Third pane only: false hides it without unmounting it. Default true. */
  open?: boolean;
}

export interface WorkspaceProps {
  /** Two or three panes, left to right. The second one is the terminal: from two columns on it is always in view. */
  panes: Pane[];
  /** localStorage key of the share of the first column, in percent. */
  storageKey?: string;
  /** localStorage key of the width of the third column, in pixels. */
  asideStorageKey?: string;
}

/** With three panes: below this width of the workspace the panes are tabs. */
export const TABS_BELOW = 768;
/** With three panes: from this width of the workspace each pane has its own column. */
export const THREE_COLUMNS_FROM = 1100;
/** Width of the third column in pixels. */
export const ASIDE_WIDTH = { min: 320, max: 560, initial: 380, step: 16 };
/** Share of the first column in percent. */
const SPLIT = { min: 22, max: 70, initial: 40, step: 2 };

type Divider = "main" | "aside";
/** "css": two panes, switched by the `md` breakpoint as Split always did. The others are measured. */
type Layout = "css" | "tabs" | "two" | "three";

const clamp = (n: number, { min, max }: { min: number; max: number }) => Math.min(max, Math.max(min, n));
const stored = (key: string) => { try { return Number(localStorage.getItem(key)) || 0; } catch { return 0; } };
const store = (key: string, value: number) => { try { localStorage.setItem(key, String(Math.round(value))); } catch { /* ignore */ } };
const viewport = () => (typeof window === "undefined" ? THREE_COLUMNS_FROM : window.innerWidth);

/**
 * Width of an element, kept current by a ResizeObserver. Where there is none
 * (jsdom, old browsers) and until the element has a width, it is the width of
 * the viewport.
 */
function useWidth(ref: RefObject<HTMLElement | null>, enabled: boolean): number {
  const [width, setWidth] = useState(viewport);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!enabled || !el) return;
    if (typeof ResizeObserver === "undefined") {
      const onResize = () => setWidth(viewport());
      onResize();
      window.addEventListener("resize", onResize);
      return () => window.removeEventListener("resize", onResize);
    }
    const take = (w: number) => { if (w > 0) setWidth(w); };
    // Measured before the first paint, so that the layout does not jump.
    take(el.getBoundingClientRect().width);
    const observer = new ResizeObserver((entries) => { const last = entries[entries.length - 1]; if (last) take(last.contentRect.width); });
    observer.observe(el);
    return () => observer.disconnect();
  }, [ref, enabled]);
  return width;
}

/**
 * The panes of a workspace view: resizable columns where there is room and
 * tabs where there is not.
 *
 * Two panes sit side by side from the `md` breakpoint, with a draggable
 * divider whose position is remembered. Below it each pane takes the full
 * width and two tabs switch between them.
 *
 * Three panes follow the measured width of the workspace, not the viewport:
 * - below 768 px: tabs, one per open pane;
 * - from 768 px: two columns. The second pane is the right column; a small
 *   switcher above the left column chooses between the first and the third;
 * - from 1100 px: three columns. The third one has a width in pixels, which
 *   is remembered, and a divider of its own.
 * A third pane with `open: false` is hidden, which leaves the layout of two.
 *
 * All panes stay mounted in every layout, and across a change from two panes
 * to three. A pane out of view is hidden with `visibility`, not unmounted and
 * not `display: none`, so a terminal in it keeps its connection, its
 * scrollback and its size. To show and hide the third pane, change its `open`.
 */
export function Workspace({ panes, storageKey = "kubedojo.split", asideStorageKey = "kubedojo.split.aside" }: WorkspaceProps) {
  const [first, second] = panes;
  const third: Pane | undefined = panes[2];
  const measured = panes.length > 2;
  // The third pane while it is open: only then it has a tab or a column.
  const openThird = third && third.open !== false ? third : null;

  const [pct, setPct] = useState(() => stored(storageKey) || SPLIT.initial);
  const [aside, setAside] = useState(() => clamp(stored(asideStorageKey) || ASIDE_WIDTH.initial, ASIDE_WIDTH));
  // What the tabs show, and what the left column shows when it holds two panes.
  const [tab, setTab] = useState(first.id);
  const [side, setSide] = useState(first.id);
  const root = useRef<HTMLDivElement>(null);
  const wrap = useRef<HTMLDivElement>(null);
  const asideDivider = useRef<HTMLDivElement>(null);
  const dragging = useRef<Divider | null>(null);
  const [active, setActive] = useState<Divider | null>(null);
  const width = useWidth(root, measured);

  const layout: Layout = !measured ? "css" : width < TABS_BELOW ? "tabs" : openThird && width >= THREE_COLUMNS_FROM ? "three" : "two";
  // A pane that is closed or gone cannot stay selected.
  const selectable = openThird ? [first, second, openThird] : [first, second];
  const shownTab = selectable.some((p) => p.id === tab) ? tab : first.id;
  const shownSide = openThird && side === openThird.id ? openThird.id : first.id;
  // In two columns the left one holds the first and the third pane.
  const leftColumn = layout === "two" && openThird ? [first, openThird] : null;

  const onMove = useCallback((e: PointerEvent) => {
    if (!dragging.current || !wrap.current) return;
    const r = wrap.current.getBoundingClientRect();
    if (dragging.current === "aside") { setAside(clamp(r.right - e.clientX, ASIDE_WIDTH)); return; }
    // The first two columns end where the third one begins, when it has a column.
    const end = asideDivider.current?.getBoundingClientRect().left ?? r.right;
    setPct(clamp(((e.clientX - r.left) / (end - r.left)) * 100, SPLIT));
  }, []);
  const stop = useCallback(() => { dragging.current = null; setActive(null); }, []);

  useEffect(() => {
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", stop);
    return () => { window.removeEventListener("pointermove", onMove); window.removeEventListener("pointerup", stop); };
  }, [onMove, stop]);
  useEffect(() => { store(storageKey, pct); }, [pct, storageKey]);
  useEffect(() => { if (measured) store(asideStorageKey, aside); }, [measured, aside, asideStorageKey]);

  const select = (id: string) => { setTab(id); if (id !== second.id) setSide(id); };
  const onTabKey = (e: React.KeyboardEvent, among: Pane[], current: string) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    e.preventDefault();
    const i = among.findIndex((p) => p.id === current) + (e.key === "ArrowRight" ? 1 : -1);
    const to = among[(i + among.length) % among.length].id;
    select(to);
    document.getElementById(`split-tab-${to}`)?.focus();
  };
  const tabButton = (pane: Pane, among: Pane[], current: string, small = false) => (
    <button key={pane.id} role="tab" id={`split-tab-${pane.id}`} aria-selected={current === pane.id} aria-controls={`split-pane-${pane.id}`}
      tabIndex={current === pane.id ? 0 : -1} onClick={() => select(pane.id)} onKeyDown={(e) => onTabKey(e, among, current)}
      className={cx(small ? "relative h-9 flex-1 px-3 text-label font-medium transition-colors" : "relative h-11 flex-1 px-3 text-sm font-medium transition-colors",
        current === pane.id ? "text-text" : "text-muted hover:text-text")}>
      {pane.label}
      {current === pane.id && <span aria-hidden className="absolute inset-x-0 -bottom-px h-0.5 bg-accent" />}
    </button>
  );

  const divider = (which: Divider) => {
    const main = which === "main";
    // The divider of the third column is its left edge: moving it left makes the column wider.
    const more = main ? "ArrowRight" : "ArrowLeft", less = main ? "ArrowLeft" : "ArrowRight";
    const range = main ? SPLIT : ASIDE_WIDTH;
    const set = main ? setPct : setAside;
    return (
      <div
        key={`divider-${which}`} ref={main ? undefined : asideDivider}
        role="separator" aria-orientation="vertical" tabIndex={0}
        // A focusable separator is a slider to a screen reader: it has a name and a value.
        aria-valuenow={Math.round(main ? clamp(pct, SPLIT) : aside)}
        aria-valuemin={range.min} aria-valuemax={range.max}
        aria-label={main ? t("Resize the columns") : t("Resize {name}", { name: third?.label ?? "" })}
        onPointerDown={(e) => { dragging.current = which; setActive(which); e.preventDefault(); }}
        onKeyDown={(e) => {
          if (e.key === less) set((v) => Math.max(range.min, v - range.step));
          if (e.key === more) set((v) => Math.min(range.max, v + range.step));
        }}
        className={measured
          ? cx("group relative w-1.5 cursor-col-resize border-x bg-panel2 transition-colors hover:bg-accent/40", active === which && "bg-accent/60")
          : cx("group relative hidden w-1.5 shrink-0 cursor-col-resize border-x bg-panel2 transition-colors hover:bg-accent/40 md:block", active && "bg-accent/60")}
        style={measured ? { gridColumn: main ? 2 : 4, gridRow: "1 / span 2" } : undefined}
      >
        <span className="absolute left-1/2 top-1/2 h-8 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded bg-muted/50 group-hover:bg-accent" />
      </div>
    );
  };

  const paneBox = (pane: Pane, i: number) => {
    if (layout === "css") {
      // Narrow: the pane fills the area, and only the selected one is visible. Wide: back in the row.
      // It is the panel of its tab; from `md` on the tabs are gone and the name stays.
      return (
        <div key={`pane-${pane.id}`} id={`split-pane-${pane.id}`} role="tabpanel" aria-labelledby={`split-tab-${pane.id}`}
          className={cx("absolute inset-0 min-w-0 overflow-hidden md:static", shownTab !== pane.id && "max-md:invisible",
            i === 0 ? "md:w-(--split) md:shrink-0" : "md:flex-1", i > 0 && active && "pointer-events-none")}>
          {pane.content}
        </div>
      );
    }
    // Panes that share a cell lie on top of each other, and one of them is visible.
    const column = layout === "tabs" ? 1 : layout === "two" ? (i === 1 ? 3 : 1) : 1 + 2 * i;
    const shown = layout === "tabs" ? pane.id === shownTab : layout === "two" ? i === 1 || pane.id === shownSide : true;
    const tabbed = layout === "tabs" ? selectable.includes(pane) : !!leftColumn && i !== 1;
    return (
      <div key={`pane-${pane.id}`} id={`split-pane-${pane.id}`}
        role={tabbed ? "tabpanel" : undefined} aria-labelledby={tabbed ? `split-tab-${pane.id}` : undefined}
        className={cx("min-h-0 min-w-0 overflow-hidden", shown ? null : "invisible", i > 0 && active && "pointer-events-none")}
        // Below the switcher in the left column; everywhere else the full height.
        style={{ gridColumn: column, gridRow: leftColumn && column === 1 ? 2 : "1 / span 2" }}>
        {pane.content}
      </div>
    );
  };

  const share = clamp(pct, SPLIT);
  const columns = layout === "tabs" ? "minmax(0, 1fr)"
    : `minmax(0, ${share}fr) auto minmax(0, ${100 - share}fr)${layout === "three" ? ` auto ${aside}px` : ""}`;

  return (
    <div ref={root} data-layout={measured ? layout : undefined} className="flex min-h-0 flex-1 flex-col">
      {(layout === "css" || layout === "tabs") && (
        <div role="tablist" aria-label={t("Panes")} className={cx("flex shrink-0 border-b bg-panel", layout === "css" && "md:hidden")}>
          {selectable.map((p) => tabButton(p, selectable, shownTab))}
        </div>
      )}
      <div ref={wrap} className={cx(measured ? "relative grid min-h-0 flex-1" : "relative flex min-h-0 flex-1", active && "select-none")}
        style={measured ? { gridTemplateColumns: columns, gridTemplateRows: "auto minmax(0, 1fr)" } : { "--split": `${pct}%` } as CSSProperties}>
        {[
          leftColumn && (
            <div key="switcher" role="tablist" aria-label={t("Left column")} className="flex border-b bg-panel" style={{ gridColumn: 1, gridRow: 1 }}>
              {leftColumn.map((p) => tabButton(p, leftColumn, shownSide, true))}
            </div>
          ),
          paneBox(first, 0),
          layout !== "tabs" && divider("main"),
          paneBox(second, 1),
          layout === "three" && divider("aside"),
          third && paneBox(third, 2),
        ]}
      </div>
    </div>
  );
}
