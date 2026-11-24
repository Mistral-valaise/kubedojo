import type { ReactNode } from "react";
import { cx } from "./cx";

/** What a view shows when it has nothing to list: what is missing, why, and what to do about it. */
export function EmptyState({ icon, title, text, action, level = 2, className }: {
  /** A lucide icon element; it is sized and coloured here. */
  icon?: ReactNode;
  title: string;
  text?: ReactNode;
  /** The one thing to do next: a Button or a ButtonLink. */
  action?: ReactNode;
  /** Heading level of the title. */
  level?: 1 | 2 | 3;
  className?: string;
}) {
  const Title = `h${level}` as "h1" | "h2" | "h3";
  return (
    <div className={cx("flex flex-col items-center rounded-2xl border border-dashed px-6 py-12 text-center", className)}>
      {icon && <div aria-hidden className="mb-4 flex size-12 items-center justify-center rounded-full bg-panel2 text-muted [&>svg]:size-6">{icon}</div>}
      <Title className="text-lg font-semibold tracking-tight">{title}</Title>
      {text && <p className="mt-1.5 max-w-md text-sm leading-relaxed text-muted">{text}</p>}
      {action && <div className="mt-5 flex flex-wrap justify-center gap-2">{action}</div>}
    </div>
  );
}
