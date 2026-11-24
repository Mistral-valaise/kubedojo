import type { ButtonHTMLAttributes, ReactNode, Ref } from "react";
import { cx } from "./cx";

export type IconButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-label" | "children"> & {
  /** The name of the button: an icon alone says nothing to a screen reader. */
  "aria-label": string;
  /** The icon. */
  children: ReactNode;
  /** md is 36 px square, for toolbars; lg 40 px, for page headers. */
  size?: "md" | "lg";
  /** `outline` draws a border, for a button that stands alone among text. */
  variant?: "ghost" | "outline";
  ref?: Ref<HTMLButtonElement>;
};

/** A square button that shows an icon only. Its name is required and is also its tooltip. */
export function IconButton({ size = "md", variant = "ghost", className, children, title, ref, type = "button", ...rest }: IconButtonProps) {
  return (
    <button {...rest} ref={ref} type={type} title={title ?? rest["aria-label"]}
      className={cx(
        "inline-flex shrink-0 items-center justify-center rounded-lg text-muted transition hover:bg-panel2 hover:text-text disabled:opacity-50 disabled:hover:bg-transparent disabled:hover:text-muted",
        size === "lg" ? "size-10" : "size-9", variant === "outline" && "border", className,
      )}>
      {children}
    </button>
  );
}
