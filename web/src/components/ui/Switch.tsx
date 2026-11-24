import type { ButtonHTMLAttributes, Ref } from "react";
import { cx } from "./cx";
import { useField } from "./Field";

/**
 * An on/off switch that takes effect at once. Inside a Field (use
 * `layout="row"`) the label of the field names it; on its own it needs an
 * `aria-label`. Space and Enter toggle it, as on every button.
 */
export function Switch({ checked, onChange, className, ref, ...rest }: Omit<ButtonHTMLAttributes<HTMLButtonElement>, "onChange" | "role" | "aria-checked" | "children"> & {
  checked: boolean;
  onChange: (checked: boolean) => void;
  ref?: Ref<HTMLButtonElement>;
}) {
  const field = useField();
  return (
    <button type="button" {...field} {...rest} ref={ref} role="switch" aria-checked={checked}
      disabled={rest.disabled || field?.disabled}
      onClick={(e) => { rest.onClick?.(e); if (!e.defaultPrevented) onChange(!checked); }}
      className={cx("flex h-7 w-12 shrink-0 items-center rounded-full border p-0.5 transition disabled:cursor-not-allowed disabled:opacity-50",
        checked ? "justify-end border-accent bg-accent" : "justify-start bg-panel2", className)}>
      <span aria-hidden className={cx("block size-5.5 rounded-full transition", checked ? "bg-on-accent" : "bg-muted")} />
    </button>
  );
}
