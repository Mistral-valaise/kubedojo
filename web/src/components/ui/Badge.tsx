import type { ReactNode } from "react";
import { cx } from "./cx";

export function Badge({ tone = "muted", children }: { tone?: "muted" | "accent" | "ok" | "bad" | "warn"; children: ReactNode }) {
  const tones = {
    muted: "bg-panel2 text-muted border",
    accent: "bg-accent-soft text-accent",
    ok: "bg-ok-soft text-ok",
    bad: "bg-bad-soft text-bad",
    warn: "bg-warn-soft text-warn",
  };
  return <span className={cx("inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap", tones[tone])}>{children}</span>;
}
