import { useLayoutEffect, useRef, useState } from "react";
import { Check, CheckCircle2, Circle, ExternalLink, Eye, XCircle } from "lucide-react";
import type { ChoiceReview, ChoiceView } from "../api";
import { t } from "../i18n";
import { Markdown } from "./Markdown";
import { Badge, cx } from "./ui";

/** Keys typed into a text field belong to that field, never to the options. */
function isTextField(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable;
}

/**
 * The options of a multiple-choice question. Until the server reveals the
 * answer key the options can be picked; with a key they are shown as a review.
 *
 * Keys: 1-9 pick the nth option from anywhere on the page, the arrow keys
 * move between options, Space picks the focused one, Enter submits. `passed`
 * is the graded outcome shown above the review; leave it out when the answer
 * was revealed without grading.
 */
export function Choice({ choice, value, onChange, onSubmit, hotkeys = true, disabled, passed, label }: {
  choice: ChoiceView; value: string[]; onChange?: (ids: string[]) => void; onSubmit?: () => void;
  hotkeys?: boolean; disabled?: boolean; passed?: boolean; label?: string;
}) {
  if (choice.review) return <Review choice={choice} review={choice.review} passed={passed} />;
  return <Options choice={choice} value={value} onChange={onChange} onSubmit={onSubmit} hotkeys={hotkeys} disabled={disabled} label={label} />;
}

function Options({ choice, value, onChange, onSubmit, hotkeys, disabled, label }: {
  choice: ChoiceView; value: string[]; onChange?: (ids: string[]) => void; onSubmit?: () => void;
  hotkeys: boolean; disabled?: boolean; label?: string;
}) {
  const { options, multiple } = choice;
  const group = useRef<HTMLDivElement>(null);
  const rows = useRef<(HTMLDivElement | null)[]>([]);
  // Roving tabindex: one option is in the tab order, the arrow keys move it.
  const [focus, setFocus] = useState(() => Math.max(0, options.findIndex((o) => value.includes(o.id))));

  const pick = (i: number) => {
    const id = options[i]?.id;
    if (!id || disabled) return;
    if (multiple) onChange?.(value.includes(id) ? value.filter((x) => x !== id) : [...value, id]);
    else if (!value.includes(id)) onChange?.([id]);
  };
  const go = (i: number) => {
    const n = (i + options.length) % options.length;
    setFocus(n);
    rows.current[n]?.focus();
  };

  // The listener is on the window so that the number keys work without first
  // tabbing into the group. It always sees the latest props through the ref.
  const onKey = useRef<(e: KeyboardEvent) => void>(() => undefined);
  onKey.current = (e) => {
    // Modified keys are browser and system shortcuts (Ctrl+1 switches tabs).
    if (!hotkeys || disabled || e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey || e.isComposing) return;
    if (isTextField(e.target)) return;
    const el = e.target as Node | null;
    const inside = !!el && !!group.current?.contains(el);
    if (/^[1-9]$/.test(e.key)) {
      const i = Number(e.key) - 1;
      if (i >= options.length) return;
      e.preventDefault();
      pick(i);
      go(i);
      return;
    }
    if (e.key === "Enter") {
      // Enter on a button or link elsewhere on the page keeps its own meaning.
      if (!onSubmit || !(inside || el === document.body)) return;
      e.preventDefault();
      if (value.length > 0) onSubmit();
      return;
    }
    if (!inside) return;
    const at = rows.current.findIndex((r) => r === el);
    switch (e.key) {
      case "ArrowDown": case "ArrowRight": e.preventDefault(); go(at + 1); break;
      case "ArrowUp": case "ArrowLeft": e.preventDefault(); go(at - 1); break;
      case "Home": e.preventDefault(); go(0); break;
      case "End": e.preventDefault(); go(options.length - 1); break;
      case " ": e.preventDefault(); if (at >= 0) pick(at); break;
    }
  };
  // A layout effect: the keys work from the moment the options are on the
  // page, not one task later. A learner who types ahead, and a test that
  // presses a key as soon as it sees the options, both rely on that.
  useLayoutEffect(() => {
    const handler = (e: KeyboardEvent) => onKey.current(e);
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  return (
    <div>
      <p id="choice-hint" className="mb-2.5 text-xs text-muted">
        {multiple ? t("Select every answer that applies.") : t("Select one answer.")}
      </p>
      <div ref={group} role={multiple ? "group" : "radiogroup"} aria-label={label ?? t("Answers")} aria-describedby="choice-hint"
        aria-disabled={disabled || undefined} className="space-y-2">
        {options.map((o, i) => {
          const on = value.includes(o.id);
          return (
            <div key={o.id} ref={(el) => { rows.current[i] = el; }}
              role={multiple ? "checkbox" : "radio"} aria-checked={on} tabIndex={i === focus ? 0 : -1}
              onClick={() => { setFocus(i); pick(i); }} onFocus={() => setFocus(i)}
              className={cx("flex min-h-11 items-start gap-3 rounded-xl border px-4 py-3 transition select-none",
                disabled ? "cursor-default opacity-70" : "cursor-pointer",
                on ? "border-accent bg-accent-soft" : "bg-panel2/40 hover:border-accent/60")}>
              <span aria-hidden className={cx("mt-0.5 flex size-5 shrink-0 items-center justify-center border-2 transition",
                multiple ? "rounded-md" : "rounded-full", on ? "border-accent bg-accent text-white" : "border-muted/60 bg-panel")}>
                {on && (multiple ? <Check className="size-3.5" strokeWidth={3} /> : <span className="size-2 rounded-full bg-white" />)}
              </span>
              <Markdown plain className="min-w-0 flex-1">{o.text}</Markdown>
              {i < 9 && (
                <kbd aria-hidden className="mt-0.5 hidden h-5 min-w-5 shrink-0 items-center justify-center rounded-md border bg-panel px-1 font-mono text-[11px] text-muted sm:flex">{i + 1}</kbd>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function Review({ choice, review, passed }: { choice: ChoiceView; review: ChoiceReview; passed?: boolean }) {
  const answered = choice.answer.length > 0;
  const verdict = passed === undefined
    ? { tone: "muted", icon: <Eye className="size-5 text-muted" />, title: t("Answer shown"), note: t("The answer was revealed before you answered.") }
    : passed
      ? { tone: "ok", icon: <CheckCircle2 className="size-5 text-ok" />, title: t("Correct"), note: t("You picked the right answer.") }
      : answered
        ? { tone: "bad", icon: <XCircle className="size-5 text-bad" />, title: t("Incorrect"), note: t("The correct answer is marked below.") }
        : { tone: "bad", icon: <XCircle className="size-5 text-bad" />, title: t("Not answered"), note: t("The correct answer is marked below.") };
  const refs = review.references ?? [];
  return (
    <div className="space-y-4">
      <div role="status" className={cx("flex items-center gap-3 rounded-xl border px-4 py-3",
        verdict.tone === "ok" ? "border-ok/40 bg-ok-soft" : verdict.tone === "bad" ? "border-bad/40 bg-bad-soft" : "bg-panel2")}>
        {verdict.icon}
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold">{verdict.title}</div>
          <div className="text-xs text-muted">{verdict.note}</div>
        </div>
      </div>

      <ul aria-label={t("Answers")} className="space-y-2">
        {choice.options.map((o) => {
          const correct = review.correct.includes(o.id), picked = choice.answer.includes(o.id);
          const why = review.options[o.id];
          return (
            <li key={o.id} className={cx("flex items-start gap-3 rounded-xl border px-4 py-3",
              correct ? "border-ok/50 bg-ok-soft/60" : picked ? "border-bad/50 bg-bad-soft/60" : "bg-panel2/40")}>
              {correct ? <CheckCircle2 aria-hidden className="mt-0.5 size-5 shrink-0 text-ok" />
                : picked ? <XCircle aria-hidden className="mt-0.5 size-5 shrink-0 text-bad" />
                  : <Circle aria-hidden className="mt-0.5 size-5 shrink-0 text-muted/50" />}
              <div className="min-w-0 flex-1">
                <Markdown>{o.text}</Markdown>
                {(correct || picked) && (
                  <div className="mt-1.5">
                    {correct && picked && <Badge tone="ok">{t("Your answer · correct")}</Badge>}
                    {correct && !picked && <Badge tone="ok">{t("Correct answer")}</Badge>}
                    {!correct && picked && <Badge tone="bad">{t("Your answer · incorrect")}</Badge>}
                  </div>
                )}
                {why && <Markdown className="md-sm mt-1.5 text-muted">{why}</Markdown>}
              </div>
            </li>
          );
        })}
      </ul>

      {review.explanation && (
        <div className="rounded-xl border bg-panel2/40 px-4 py-4">
          <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-accent">{t("Explanation")}</div>
          <Markdown>{review.explanation}</Markdown>
        </div>
      )}
      {refs.length > 0 && (
        <div>
          <div className="mb-1.5 text-xs font-semibold uppercase tracking-wider text-muted">{t("References")}</div>
          <ul className="space-y-1">
            {refs.map((r) => (
              <li key={r.url}>
                <a href={r.url} target="_blank" rel="noopener noreferrer"
                  className="inline-flex min-h-6 items-center gap-1.5 text-sm text-accent underline underline-offset-2 hover:brightness-110">
                  <ExternalLink aria-hidden className="size-3.5 shrink-0" />{r.title}
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
