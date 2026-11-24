import { type ReactNode, useEffect, useId, useRef } from "react";
import { X } from "lucide-react";
import { t } from "../../i18n";
import { cx } from "./cx";
import { IconButton } from "./IconButton";

/** An element of the content that asks for the focus when the dialog opens, in place of the first control. */
const PREFERRED = "[data-autofocus]";
const FOCUSABLE = "button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])";

/**
 * A dialog that must be answered before the page can be used again. Mount it
 * to open it, unmount it to close it; `onClose` asks for that (Escape, the
 * close button, a click on the backdrop) and may refuse, for example while a
 * request is running.
 *
 * It is the native `<dialog>` opened with showModal(): the browser keeps the
 * focus inside, makes the rest of the page inert and draws the backdrop. The
 * focus starts on the first control of the content (the cautious choice of a
 * confirmation comes first there) and returns to where it was on closing.
 *
 * `sheet` is for content that is a view of its own, such as the assistant on
 * a small screen: a taller and wider box that the content fills, without the
 * header and the padding. The content brings its own heading and its own
 * close button; `title` names the dialog for a screen reader.
 */
export function Modal({ title, children, onClose, footer, sheet }: {
  title: string; children: ReactNode; onClose: () => void; footer?: ReactNode; sheet?: boolean;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const actions = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const ask = useRef(onClose);
  ask.current = onClose;

  useEffect(() => {
    const el = dialog.current;
    if (!el) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (!el.open) el.showModal();
    const first = (within: HTMLElement | null) => within?.querySelector<HTMLElement>(FOCUSABLE);
    (content.current?.querySelector<HTMLElement>(PREFERRED) ?? first(content.current) ?? first(actions.current) ?? first(el))?.focus();
    // A browser may close the dialog on its own (a second Escape, the back
    // gesture). The owner decides: asked to close, it unmounts the dialog, and
    // a dialog that is still here afterwards opens again.
    const onClosed = () => {
      // The event arrives a task after close(). Of a dialog that is open by
      // then it is a stale one: React's strict mode runs this effect twice in
      // development, and the close of the first run would end the second.
      if (el.open) return;
      ask.current();
      queueMicrotask(() => { if (el.isConnected && !el.open) el.showModal(); });
    };
    el.addEventListener("close", onClosed);
    return () => {
      el.removeEventListener("close", onClosed);
      if (el.open) el.close();
      if (opener?.isConnected) opener.focus();
    };
  }, []);

  return (
    <dialog ref={dialog} aria-labelledby={sheet ? undefined : titleId} aria-label={sheet ? title : undefined}
      onCancel={(e) => { e.preventDefault(); onClose(); }}
      onKeyDown={(e) => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onClose(); } }}
      // The dialog element itself is the backdrop area around the box.
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}
      className={cx("m-auto w-[calc(100%-2rem)] overflow-visible bg-transparent text-text backdrop:bg-scrim backdrop:backdrop-blur-sm", sheet ? "max-w-xl" : "max-w-md")}>
      {sheet ? (
        <div ref={content} className="flex h-[min(44rem,calc(100dvh-2rem))] flex-col overflow-hidden rounded-2xl border bg-panel shadow-card">{children}</div>
      ) : (
      <div className="flex max-h-[calc(100dvh-2rem)] flex-col rounded-2xl border bg-panel shadow-card">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b py-2 pr-2 pl-5">
          <h2 id={titleId} className="text-base font-semibold">{title}</h2>
          <IconButton onClick={onClose} aria-label={t("Close")}><X className="size-4" aria-hidden /></IconButton>
        </div>
        <div ref={content} className="min-h-0 overflow-y-auto px-5 py-4 text-sm leading-relaxed text-muted">{children}</div>
        {footer && <div ref={actions} className="flex shrink-0 flex-wrap justify-end gap-2 border-t px-5 py-3.5">{footer}</div>}
      </div>
      )}
    </dialog>
  );
}
