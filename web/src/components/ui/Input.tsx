import type { InputHTMLAttributes, Ref } from "react";
import { cx } from "./cx";
import { controlClass, useField } from "./Field";

/**
 * A text input. Inside a Field it takes its id, its description and its error
 * state from there; on its own it needs an `aria-label`. `mono` sets the value
 * in the monospace font, for URLs, model names and the like.
 */
export function Input({ mono, className, ref, ...rest }: InputHTMLAttributes<HTMLInputElement> & { mono?: boolean; ref?: Ref<HTMLInputElement> }) {
  const field = useField();
  return (
    <input type="text" {...field} {...rest} ref={ref} disabled={rest.disabled || field?.disabled}
      className={cx(controlClass, mono && "font-mono text-label", className)} />
  );
}
