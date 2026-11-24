import { type ReactNode, type Ref, useCallback, useEffect, useId, useRef, useState } from "react";
import { cx } from "./cx";

/** What the trigger of a Popover must carry: spread it on a button. */
export interface PopoverTriggerProps {
  ref: Ref<HTMLButtonElement>;
  "aria-expanded": boolean;
  "aria-controls": string;
  "aria-haspopup": "dialog";
  onClick: () => void;
}

/**
 * A small panel that opens below its trigger on a click.
 *
 *     <Popover label="About" trigger={(props) => <IconButton {...props} aria-label="About"><Info /></IconButton>}>
 *       ...
 *     </Popover>
 *
 * Escape and a click outside close it. The focus stays on the trigger when it
 * opens, so Tab walks into the panel, and returns to the trigger when the
 * panel closes under it. Tabbing out of the panel closes it too. It is not
 * modal: for something that must be answered use Modal.
 */
export function Popover({ trigger, label, align = "end", className, panelClassName, children }: {
  /** Renders the trigger from the props it must carry; `open` is for its look. */
  trigger: (props: PopoverTriggerProps, open: boolean) => ReactNode;
  /** The name of the panel for a screen reader. */
  label: string;
  /** Which edge of the trigger the panel lines up with. */
  align?: "start" | "end";
  /** Classes of the element around the trigger and the panel, such as `hidden sm:inline-flex`. */
  className?: string;
  /** Classes of the panel, such as a width. */
  panelClassName?: string;
  /** The content, or a function that gets `close` for content that closes the panel itself. */
  children: ReactNode | ((close: () => void) => ReactNode);
}) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const root = useRef<HTMLSpanElement>(null);
  const button = useRef<HTMLButtonElement>(null);

  // Closing must not drop the focus on the page: it goes back to the trigger,
  // unless the click that closed the panel has already given it to something else.
  const close = useCallback(() => {
    setOpen(false);
    const at = document.activeElement;
    if (!at || at === document.body || root.current?.contains(at)) button.current?.focus();
  }, []);

  useEffect(() => {
    if (!open) return;
    const onPointer = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) close();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      close();
    };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, close]);

  return (
    <span ref={root} className={cx("relative inline-flex", className)}
      onBlur={(e) => { if (open && e.relatedTarget instanceof Node && !e.currentTarget.contains(e.relatedTarget)) setOpen(false); }}>
      {trigger({ ref: button, "aria-expanded": open, "aria-controls": id, "aria-haspopup": "dialog", onClick: () => (open ? close() : setOpen(true)) }, open)}
      {open && (
        <div id={id} role="dialog" aria-label={label}
          className={cx("absolute top-full z-40 mt-2 w-max max-w-[min(20rem,calc(100vw-2rem))] rounded-xl border bg-panel p-3 text-sm shadow-card",
            align === "end" ? "right-0" : "left-0", panelClassName)}>
          {typeof children === "function" ? children(close) : children}
        </div>
      )}
    </span>
  );
}
