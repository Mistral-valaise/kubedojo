import type { ReactNode } from "react";
import { cx } from "./cx";

/**
 * A surface that groups content. `padded` gives it the one padding cards
 * have; leave it out for a card made of rows that bring their own.
 */
export function Card({ children, className, padded }: { children: ReactNode; className?: string; padded?: boolean }) {
  return <div className={cx("rounded-2xl border bg-panel shadow-card", padded && "p-5 sm:p-6", className)}>{children}</div>;
}
