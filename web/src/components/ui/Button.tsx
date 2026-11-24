import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Loader2 } from "lucide-react";
import { cx } from "./cx";

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
