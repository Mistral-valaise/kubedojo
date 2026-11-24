import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Link, type LinkProps } from "react-router";
import { Loader2 } from "lucide-react";
import { cx } from "./cx";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger" | "ok";
/** sm is 36 px high, for dense toolbars; md 40 px; lg 44 px, for the main action of a page. */
export type ButtonSize = "sm" | "md" | "lg";

const variants: Record<ButtonVariant, string> = {
  primary: "bg-accent font-semibold text-on-accent hover:brightness-110",
  secondary: "border bg-panel2 text-text hover:border-accent/60",
  ghost: "text-muted hover:bg-panel2 hover:text-text",
  danger: "bg-bad font-semibold text-on-bad hover:brightness-110",
  ok: "bg-ok font-semibold text-on-ok hover:brightness-110",
};
const sizes: Record<ButtonSize, string> = {
  sm: "h-9 gap-1.5 rounded-lg px-3 text-label",
  md: "h-10 gap-2 rounded-lg px-4 text-sm",
  lg: "h-11 gap-2 rounded-[0.625rem] px-5 text-body",
};

/** The classes of a button, for an element that looks like one and is none. */
export function buttonClass({ variant = "secondary", size = "md", className }: { variant?: ButtonVariant; size?: ButtonSize; className?: string } = {}) {
  return cx(
    "inline-flex shrink-0 items-center justify-center font-medium whitespace-nowrap transition disabled:opacity-50 aria-disabled:opacity-50",
    sizes[size], variants[variant], className,
  );
}

export function Button({
  variant = "secondary", size = "md", loading, icon, children, className, ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; size?: ButtonSize; loading?: boolean; icon?: ReactNode }) {
  return (
    <button {...rest} disabled={rest.disabled || loading} className={buttonClass({ variant, size, className })}>
      {loading ? <Loader2 className="spin size-4" aria-hidden /> : icon}
      {children}
    </button>
  );
}

/** A link of the router that looks like a button: for a step to another page. */
export function ButtonLink({ variant = "secondary", size = "md", icon, children, className, ...rest }: LinkProps & {
  variant?: ButtonVariant; size?: ButtonSize; icon?: ReactNode;
}) {
  return (
    <Link {...rest} className={buttonClass({ variant, size, className })}>
      {icon}
      {children}
    </Link>
  );
}
