import { cx } from "./cx";

export function Bar({ value, tone = "accent", className }: { value: number; tone?: "accent" | "ok" | "bad" | "warn"; className?: string }) {
  const colors = { accent: "bg-accent", ok: "bg-ok", bad: "bg-bad", warn: "bg-warn" };
  return (
    <div className={cx("h-1.5 w-full overflow-hidden rounded-full bg-panel2", className)}>
      <div className={cx("h-full rounded-full transition-all duration-500 motion-reduce:transition-none", colors[tone])} style={{ width: `${Math.min(100, Math.max(0, value))}%` }} />
    </div>
  );
}
