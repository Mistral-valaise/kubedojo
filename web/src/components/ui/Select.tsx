import type { Ref, SelectHTMLAttributes } from "react";
import { ChevronDown } from "lucide-react";
import { cx } from "./cx";
import { controlClass, useField } from "./Field";

/**
 * The native select, styled like Input: the system draws the list, which is
 * what works best on a phone and with a screen reader. Pass `<option>`
 * elements as children. Inside a Field it is labelled by it.
 */
export function Select({ className, children, ref, ...rest }: SelectHTMLAttributes<HTMLSelectElement> & { ref?: Ref<HTMLSelectElement> }) {
  const field = useField();
  return (
    <span className="relative block">
      <select {...field} {...rest} ref={ref} disabled={rest.disabled || field?.disabled}
        className={cx(controlClass, "appearance-none pr-10", className)}>
        {children}
      </select>
      <ChevronDown aria-hidden className="pointer-events-none absolute right-3.5 top-1/2 size-4 -translate-y-1/2 text-muted" />
    </span>
  );
}
