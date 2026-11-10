import { type ButtonHTMLAttributes, type ReactNode, useEffect } from "react";
import { Link } from "react-router";
import { Loader2, Moon, Sun, X } from "lucide-react";
import { useTheme } from "../hooks";
import { t } from "../i18n";

export function cx(...parts: (string | false | null | undefined)[]) {
  return parts.filter(Boolean).join(" ");
}

type Variant = "primary" | "secondary" | "ghost" | "danger" | "ok";
const variants: Record<Variant, string> = {
  primary: "bg-accent text-white hover:brightness-110 shadow-sm",
  secondary: "bg-panel2 text-text border hover:border-accent/60",
  ghost: "text-muted hover:text-text hover:bg-panel2",
  danger: "bg-bad text-white hover:brightness-110",
  ok: "bg-ok text-black/90 hover:brightness-110",
};

export function Button({
  variant = "secondary", size = "md", loading, icon, children, className, ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: "sm" | "md" | "lg"; loading?: boolean; icon?: ReactNode }) {
  const sizes = { sm: "h-8 px-2.5 text-[13px] gap-1.5", md: "h-9 px-3.5 text-sm gap-2", lg: "h-11 px-5 text-[15px] gap-2" };
  return (
    <button
      {...rest}
      disabled={rest.disabled || loading}
      className={cx(
        "inline-flex shrink-0 items-center justify-center rounded-lg font-medium whitespace-nowrap transition disabled:opacity-50",
        sizes[size], variants[variant], className,
      )}
    >
      {loading ? <Loader2 className="spin size-4" /> : icon}
      {children}
    </button>
  );
}

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cx("spin size-5 text-muted", className)} />;
}

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

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cx("rounded-2xl border bg-panel shadow-card", className)}>{children}</div>;
}

export function Bar({ value, tone = "accent", className }: { value: number; tone?: "accent" | "ok" | "bad" | "warn"; className?: string }) {
  const colors = { accent: "bg-accent", ok: "bg-ok", bad: "bg-bad", warn: "bg-warn" };
  return (
    <div className={cx("h-1.5 w-full overflow-hidden rounded-full bg-panel2", className)}>
      <div className={cx("h-full rounded-full transition-all duration-500", colors[tone])} style={{ width: `${Math.min(100, Math.max(0, value))}%` }} />
    </div>
  );
}

export function Ring({ value, size = 120, stroke = 10, tone = "accent", children }: {
  value: number; size?: number; stroke?: number; tone?: "accent" | "ok" | "bad"; children?: ReactNode;
}) {
  const r = (size - stroke) / 2, c = 2 * Math.PI * r;
  const color = { accent: "var(--accent)", ok: "var(--ok)", bad: "var(--bad)" }[tone];
  return (
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--panel-2)" strokeWidth={stroke} />
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={color} strokeWidth={stroke} strokeLinecap="round"
          strokeDasharray={c} strokeDashoffset={c * (1 - Math.min(100, Math.max(0, value)) / 100)} style={{ transition: "stroke-dashoffset .6s ease" }} />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">{children}</div>
    </div>
  );
}

export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden>
      <path d="M16 4.5 26 10.3v11.4L16 27.5 6 21.7V10.3z" fill="none" stroke="var(--accent)" strokeWidth="2.2" strokeLinejoin="round" />
      <path d="m11.3 16 3.3 3.3 6.1-6.7" fill="none" stroke="var(--ok)" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function ThemeToggle() {
  const [theme, toggle] = useTheme();
  return (
    <Button variant="ghost" size="sm" onClick={toggle} aria-label={t("Toggle theme")} title={t("Toggle theme")}>
      {theme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </Button>
  );
}

export function Brand() {
  return (
    <Link to="/" className="flex items-center gap-2 font-semibold tracking-tight">
      <Logo className="size-7" />
      <span className="text-[15px]">kubedojo</span>
    </Link>
  );
}

/** Standard page chrome for non-workspace pages. */
export function Page({ children, crumbs }: { children: ReactNode; crumbs?: { label: string; to?: string }[] }) {
  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-20 border-b bg-bg/85 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-5">
          <Brand />
          {crumbs?.map((c, i) => (
            <span key={i} className="flex min-w-0 items-center gap-3 text-sm text-muted">
              <span className="text-line">/</span>
              {c.to ? <Link to={c.to} className="truncate hover:text-text">{c.label}</Link> : <span className="truncate text-text">{c.label}</span>}
            </span>
          ))}
          <div className="ml-auto flex items-center gap-1">
            <a href="https://kubernetes.io/docs/" target="_blank" rel="noreferrer" className="hidden px-2 text-sm text-muted hover:text-text sm:block">
              {t("Kubernetes docs")}
            </a>
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-5 py-8">{children}</main>
    </div>
  );
}

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

export function ErrorNote({ children }: { children: ReactNode }) {
  return <div className="rounded-xl border border-bad/40 bg-bad-soft px-4 py-3 text-sm text-bad">{children}</div>;
}

export function Difficulty({ level }: { level: string }) {
  const tone = level === "easy" ? "ok" : level === "hard" ? "bad" : "warn";
  return <Badge tone={tone}>{t(level)}</Badge>;
}
