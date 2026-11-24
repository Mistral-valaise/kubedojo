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
  return <span className={cx("inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium leading-4 whitespace-nowrap", tones[tone])}>{children}</span>;
}
