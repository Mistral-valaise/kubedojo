import type { ReactNode } from "react";
import { cx } from "./cx";

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cx("rounded-2xl border bg-panel shadow-card", className)}>{children}</div>;
}
