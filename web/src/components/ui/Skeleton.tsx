import type { ReactNode } from "react";
import { t } from "../../i18n";
import { cx } from "./cx";

/**
 * A placeholder in the shape of content that is loading. Give it the size of
 * what it stands for (`h-4 w-40`, `h-56`), so the page does not jump when the
 * content arrives. It is decoration: the announcement comes from Loading.
 * The pulse stops under prefers-reduced-motion (see base.css).
 */
export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cx("skeleton rounded-md", className)} />;
}

/** Wraps the skeletons of one loading region and names it for a screen reader. */
export function Loading({ label = t("Loading"), className, children }: { label?: string; className?: string; children: ReactNode }) {
  return <div role="status" aria-busy="true" aria-label={label} className={className}>{children}</div>;
}
