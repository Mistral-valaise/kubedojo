import type { ReactNode } from "react";

export function Ring({ value, size = 120, stroke = 10, tone = "accent", children }: {
  value: number; size?: number; stroke?: number; tone?: "accent" | "ok" | "bad"; children?: ReactNode;
}) {
  const r = (size - stroke) / 2, c = 2 * Math.PI * r;
  const color = { accent: "var(--accent)", ok: "var(--ok)", bad: "var(--bad)" }[tone];
  return (
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90" aria-hidden>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--panel-2)" strokeWidth={stroke} />
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={color} strokeWidth={stroke} strokeLinecap="round"
          strokeDasharray={c} strokeDashoffset={c * (1 - Math.min(100, Math.max(0, value)) / 100)} className="ring-arc" />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">{children}</div>
    </div>
  );
}
