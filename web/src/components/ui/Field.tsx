import { type ReactNode, createContext, useContext, useId } from "react";
import { Lock } from "lucide-react";
import { cx } from "./cx";

/** What a Field tells the control inside it. */
export interface FieldControl {
  /** The id the label points at. */
  id: string;
  /** Ids of the description and the error text, when there are any. */
  "aria-describedby"?: string;
  "aria-invalid"?: true;
  /** True when the value is fixed elsewhere: the control shows it and cannot change it. */
  disabled?: true;
}

const FieldContext = createContext<FieldControl | null>(null);

/**
 * The attributes a control takes from the Field around it, or null outside a
 * Field. Input, Select and Switch use it themselves; a control of your own
 * spreads the result on its element.
 */
export function useField(): FieldControl | null {
  return useContext(FieldContext);
}

/**
 * One labelled form control: label, control, description and error text,
 * with the label and the texts tied to the control for a screen reader.
 *
 *     <Field label="Base URL" description="Where the model is served." error={error}>
 *       <Input value={url} onChange={(e) => setUrl(e.target.value)} />
 *     </Field>
 *
 * `locked` says that the value is set elsewhere (a flag, an environment
 * variable): it is shown as a note next to the label and disables the control.
 */
export function Field({ label, description, error, locked, layout = "stack", className, children }: {
  label: string;
  description?: ReactNode;
  /** Shown in place of nothing, below the description; marks the control as invalid. */
  error?: string | null;
  /** Why the value cannot be changed here, such as "Set by --ai-model". */
  locked?: ReactNode;
  /** `row` puts the control to the right of the label and its texts, for a Switch. */
  layout?: "stack" | "row";
  className?: string;
  children: ReactNode;
}) {
  const id = useId();
  const descriptionId = description ? `${id}-description` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  const control: FieldControl = {
    id,
    "aria-describedby": [descriptionId, errorId].filter(Boolean).join(" ") || undefined,
    "aria-invalid": error ? true : undefined,
    disabled: locked ? true : undefined,
  };
  const head = (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
      <label htmlFor={id} className="text-sm font-medium">{label}</label>
      {locked && (
        <span className="inline-flex items-center gap-1 rounded-md border bg-panel2 px-2 py-0.5 text-xs text-warn">
          <Lock className="size-3" aria-hidden />{locked}
        </span>
      )}
    </div>
  );
  const texts = (
    <>
      {description && <p id={descriptionId} className="text-label leading-relaxed text-muted">{description}</p>}
      {error && <p id={errorId} role="alert" className="text-label font-medium text-bad">{error}</p>}
    </>
  );
  return (
    <FieldContext.Provider value={control}>
      {layout === "row" ? (
        <div className={cx("flex items-start gap-4", className)}>
          <div className="flex min-w-0 flex-1 flex-col gap-1">{head}{texts}</div>
          {children}
        </div>
      ) : (
        <div className={cx("flex flex-col gap-2", className)}>{head}{children}{texts}</div>
      )}
    </FieldContext.Provider>
  );
}

/**
 * The look shared by Input and Select: 44 px high, with the states of a form
 * control. The focus ring is the one every element gets (base.css).
 */
export const controlClass =
  "h-11 w-full rounded-[0.625rem] border bg-bg px-3.5 text-sm text-text transition placeholder:text-muted "
  + "focus-visible:border-accent aria-invalid:border-bad "
  + "disabled:cursor-not-allowed disabled:bg-panel2 disabled:text-muted";
