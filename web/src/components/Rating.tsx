import { type KeyboardEvent, useEffect, useRef, useState } from "react";
import { Star } from "lucide-react";
import { api } from "../api";
import { t } from "../i18n";
import { Button, cx } from "./ui";

const STARS = [1, 2, 3, 4, 5];

/** Lets the learner rate a question; feedback goes to the people who maintain the content. */
export function Rating({ track, question }: { track: string; question: string }) {
  const [stars, setStars] = useState(0);
  const [hover, setHover] = useState(0);
  const [comment, setComment] = useState("");
  const [state, setState] = useState<"idle" | "sending" | "sent" | "error">("idle");
  const radios = useRef<(HTMLButtonElement | null)[]>([]);
  useEffect(() => { setStars(0); setComment(""); setState("idle"); }, [track, question]);

  if (state === "sent") {
    return <p role="status" className="rounded-xl border border-dashed px-4 py-3 text-sm text-muted">{t("Thanks — your feedback helps improve this question.")}</p>;
  }
  const send = async () => {
    setState("sending");
    try { await api.rate(track, question, stars, comment); setState("sent"); }
    catch { setState("error"); }
  };
  // A radio group: one stop for Tab, the arrow keys move and select (WAI-ARIA radio group).
  const pick = (n: number) => { setStars(n); radios.current[n - 1]?.focus(); };
  const onKey = (e: KeyboardEvent, n: number) => {
    const to = e.key === "ArrowRight" || e.key === "ArrowUp" ? (n % 5) + 1
      : e.key === "ArrowLeft" || e.key === "ArrowDown" ? ((n + 3) % 5) + 1
        : e.key === "Home" ? 1 : e.key === "End" ? 5 : 0;
    if (!to) return;
    e.preventDefault();
    pick(to);
  };
  return (
    <div className="rounded-xl border border-dashed px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="text-sm text-muted">{t("How good was this question?")}</span>
        <div className="-ml-2 flex" role="radiogroup" aria-label={t("Rating")} onMouseLeave={() => setHover(0)}>
          {STARS.map((n) => (
            <button key={n} ref={(el) => { radios.current[n - 1] = el; }} type="button"
              role="radio" aria-checked={stars === n} aria-label={t("{n} of 5 stars", { n })}
              // Roving tabindex: the chosen star is the tab stop, the first one before a choice.
              tabIndex={stars === n || (stars === 0 && n === 1) ? 0 : -1}
              onMouseEnter={() => setHover(n)} onClick={() => setStars(n)} onKeyDown={(e) => onKey(e, n)}
              className="flex size-9 items-center justify-center rounded-lg">
              <Star aria-hidden className={cx("size-5 transition", (hover || stars) >= n ? "fill-warn text-warn" : "text-muted")} />
            </button>
          ))}
        </div>
      </div>
      {stars > 0 && (
        <div className="mt-3 space-y-2">
          <textarea value={comment} onChange={(e) => setComment(e.target.value)} rows={2} maxLength={2000}
            aria-label={t("Comment")}
            placeholder={stars <= 3 ? t("What was unclear or wrong? (optional)") : t("Anything to add? (optional)")}
            className="w-full resize-y rounded-lg border bg-bg px-3 py-2 text-sm placeholder:text-muted focus-visible:border-accent" />
          <div className="flex items-center gap-3">
            <Button size="sm" variant="primary" loading={state === "sending"} onClick={send}>{t("Send feedback")}</Button>
            {state === "error" && <span role="alert" className="text-sm text-bad">{t("Could not send. Try again.")}</span>}
          </div>
        </div>
      )}
    </div>
  );
}
