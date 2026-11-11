import { useEffect, useState } from "react";
import { Star } from "lucide-react";
import { api } from "../api";
import { t } from "../i18n";
import { Button, cx } from "./ui";

/** Lets the learner rate a question; feedback goes to the people who maintain the content. */
export function Rating({ track, question }: { track: string; question: string }) {
  const [stars, setStars] = useState(0);
  const [hover, setHover] = useState(0);
  const [comment, setComment] = useState("");
  const [state, setState] = useState<"idle" | "sending" | "sent" | "error">("idle");
  useEffect(() => { setStars(0); setComment(""); setState("idle"); }, [track, question]);

  if (state === "sent") {
    return <p className="rounded-xl border border-dashed px-4 py-3 text-sm text-muted">{t("Thanks — your feedback helps improve this question.")}</p>;
  }
  const send = async () => {
    setState("sending");
    try { await api.rate(track, question, stars, comment); setState("sent"); }
    catch { setState("error"); }
  };
  return (
    <div className="rounded-xl border border-dashed px-4 py-3">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm text-muted">{t("How good was this question?")}</span>
        <div className="flex" role="radiogroup" aria-label={t("Rating")} onMouseLeave={() => setHover(0)}>
          {[1, 2, 3, 4, 5].map((n) => (
            <button key={n} role="radio" aria-checked={stars === n} aria-label={t("{n} of 5 stars", { n })}
              onMouseEnter={() => setHover(n)} onClick={() => setStars(n)} className="p-0.5">
              <Star className={cx("size-5 transition", (hover || stars) >= n ? "fill-warn text-warn" : "text-line")} />
            </button>
          ))}
        </div>
      </div>
      {stars > 0 && (
        <div className="mt-3 space-y-2">
          <textarea value={comment} onChange={(e) => setComment(e.target.value)} rows={2} maxLength={2000}
            placeholder={stars <= 3 ? t("What was unclear or wrong? (optional)") : t("Anything to add? (optional)")}
            className="w-full resize-y rounded-lg border bg-panel2/50 px-3 py-2 text-sm outline-none placeholder:text-muted focus:border-accent" />
          <div className="flex items-center gap-3">
            <Button size="sm" variant="primary" loading={state === "sending"} onClick={send}>{t("Send feedback")}</Button>
            {state === "error" && <span className="text-sm text-bad">{t("Could not send. Try again.")}</span>}
          </div>
        </div>
      )}
    </div>
  );
}
