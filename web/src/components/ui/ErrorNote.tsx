import type { ReactNode } from "react";
import { t } from "../../i18n";
import { cx } from "./cx";

/** An error the learner must see. `onRetry` adds a button that tries again. */
export function ErrorNote({ children, className, onRetry, retryLabel = t("Try again") }: {
  children: ReactNode; className?: string; onRetry?: () => void; retryLabel?: string;
}) {
  return (
    <div className={cx("rounded-xl border border-bad/40 bg-bad-soft px-4 py-3 text-sm text-bad", onRetry && "flex flex-wrap items-center gap-x-4 gap-y-2", className)}>
      {onRetry ? <div className="min-w-0 flex-1 basis-48">{children}</div> : children}
      {onRetry && (
        <button type="button" onClick={onRetry}
          className="inline-flex h-9 shrink-0 items-center rounded-lg border border-bad/50 px-3 text-label font-medium hover:bg-bad hover:text-on-bad">
          {retryLabel}
        </button>
      )}
    </div>
  );
}
