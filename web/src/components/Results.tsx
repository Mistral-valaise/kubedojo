import { useState } from "react";
import { CheckCircle2, ChevronDown, XCircle } from "lucide-react";
import type { Result } from "../api";
import { plural, t } from "../i18n";
import { Markdown } from "./Markdown";
import { cx } from "./ui";

/** Per-check grading feedback: what was expected, what was found, and why it matters. */
export function Results({ result, defaultOpen = true }: { result: Result; defaultOpen?: boolean }) {
  const pct = result.max ? Math.round((result.score / result.max) * 100) : 0;
  return (
    <div className={cx("overflow-hidden rounded-xl border", result.passed ? "border-ok/40" : "border-line")}>
      <div className={cx("flex items-center gap-3 px-4 py-3", result.passed ? "bg-ok-soft" : "bg-panel2")}>
        {result.passed ? <CheckCircle2 className="size-5 text-ok" /> : <XCircle className="size-5 text-bad" />}
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold">
            {result.passed ? t("All checks passed") : t("{n} of {m} checks passed", { n: result.checks.filter((c) => c.passed).length, m: result.checks.length })}
          </div>
          <div className="text-xs text-muted">{plural(result.max, "{score} of {n} point", "{score} of {n} points", { score: result.score })} · {pct}%</div>
        </div>
      </div>
      <ul className="divide-y">
        {result.checks.map((c) => <CheckRow key={c.id + result.at} check={c} defaultOpen={defaultOpen && !c.passed} />)}
      </ul>
    </div>
  );
}

function CheckRow({ check, defaultOpen }: { check: Result["checks"][number]; defaultOpen: boolean }) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <li>
      <button onClick={() => setOpen(!open)} className="flex w-full items-start gap-2.5 px-4 py-2.5 text-left hover:bg-panel2/60" aria-expanded={open}>
        {check.passed ? <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-ok" /> : <XCircle className="mt-0.5 size-4 shrink-0 text-bad" />}
        <span className="min-w-0 flex-1 text-sm">{check.title}</span>
        <span className="shrink-0 text-xs tabular-nums text-muted">{check.passed ? check.points : 0}/{check.points}</span>
        <ChevronDown className={cx("mt-0.5 size-4 shrink-0 text-muted transition-transform", open && "rotate-180")} />
      </button>
      {open && (
        <div className="space-y-2.5 px-4 pb-3.5 pl-[42px] text-[13px]">
          {!check.passed && (
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
              <dt className="text-muted">{t("Expected")}</dt>
              <dd className="min-w-0"><Value text={check.expected} /></dd>
              <dt className="text-muted">{t("Found")}</dt>
              <dd className="min-w-0 text-bad"><Value text={check.observed} /></dd>
            </dl>
          )}
          {/* Explanations are Markdown: commands and file names in them are code. */}
          <Markdown plain className="md-sm text-muted">{check.explain}</Markdown>
          <details className="text-xs text-muted">
            <summary className="cursor-pointer select-none hover:text-text">{t("How this was checked")}</summary>
            <pre className="mt-1.5 overflow-x-auto rounded-lg border bg-code px-3 py-2 font-mono text-[11.5px] whitespace-pre-wrap">{check.command}</pre>
            {check.attempts !== undefined && (
              <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                <dt>{t("Ran on")}</dt>
                <dd className="font-mono">{check.on}</dd>
                <dt>{t("Exit code")}</dt>
                <dd className="font-mono">{check.timedOut ? t("none (timed out)") : check.exitCode}</dd>
                <dt>{t("Duration")}</dt>
                <dd className="font-mono tabular-nums">
                  {((check.durationMs ?? 0) / 1000).toFixed(1)}s
                  {(check.attempts ?? 1) > 1 && ` · ${plural(check.attempts ?? 1, "{n} attempt", "{n} attempts")}`}
                </dd>
                {check.stderr && (
                  <>
                    <dt>{t("Error output")}</dt>
                    <dd className="font-mono break-words whitespace-pre-wrap">{check.stderr}</dd>
                  </>
                )}
              </dl>
            )}
          </details>
        </div>
      )}
    </li>
  );
}

/**
 * An expected or observed value, as preformatted text: a value of several
 * lines keeps its line breaks and its indentation, and a long line wraps
 * instead of widening the row.
 */
function Value({ text }: { text: string }) {
  return <pre className="font-mono text-[12px] leading-5 break-words whitespace-pre-wrap">{text.replace(/\n+$/, "")}</pre>;
}
