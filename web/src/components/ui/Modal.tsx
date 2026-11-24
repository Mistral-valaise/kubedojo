import { type ReactNode, useEffect } from "react";
import { X } from "lucide-react";
import { t } from "../../i18n";

export function Modal({ title, children, onClose, footer }: { title: string; children: ReactNode; onClose: () => void; footer?: ReactNode }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4 backdrop-blur-sm" onMouseDown={onClose}>
      <div role="dialog" aria-modal className="w-full max-w-md rounded-2xl border bg-panel shadow-card" onMouseDown={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b px-5 py-3.5">
          <h2 className="font-semibold">{title}</h2>
          <button onClick={onClose} className="text-muted hover:text-text" aria-label={t("Close")}><X className="size-4" /></button>
        </div>
        <div className="px-5 py-4 text-sm leading-relaxed text-muted">{children}</div>
        {footer && <div className="flex justify-end gap-2 border-t px-5 py-3.5">{footer}</div>}
      </div>
    </div>
  );
}
