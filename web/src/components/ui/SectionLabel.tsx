import type { ReactNode } from "react";
import { cx } from "./cx";

/**
 * The small uppercase label above a group of content. It is a `div` unless
 * `as` says otherwise: a heading where it titles a section of the page, a
 * `label` (with `htmlFor`) where it names a form control.
 */
export function SectionLabel({ as: Tag = "div", tone = "muted", icon, htmlFor, id, className, children }: {
  as?: "div" | "span" | "h2" | "h3" | "label";
  tone?: "muted" | "accent" | "warn";
  /** A lucide icon element shown before the text. */
  icon?: ReactNode;
  /** With `as="label"`: the id of the control it names. */
  htmlFor?: string;
  id?: string;
  className?: string;
  children: ReactNode;
}) {
  const tones = { muted: "text-muted", accent: "text-accent", warn: "text-warn" };
  return (
    <Tag id={id} {...(Tag === "label" ? { htmlFor } : {})}
      className={cx("text-xs font-semibold uppercase tracking-wider", icon ? "flex items-center gap-1.5 [&>svg]:size-3.5" : Tag === "span" ? "inline-block" : "block", tones[tone], className)}>
      {icon}{children}
    </Tag>
  );
}
