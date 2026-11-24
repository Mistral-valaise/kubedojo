import { Link } from "react-router";
import { CheckCircle2, ExternalLink, Route } from "lucide-react";
import type { Path, Progress, Track } from "../api";
import { t } from "../i18n";
import { assessmentPassed, words } from "../track";
import { Badge, Bar, Card, SectionLabel, cx } from "./ui";

/**
 * The recommended order through the tracks, as a card: one row per step with
 * the readiness of its track. It only recommends: every step is a plain link,
 * whatever the learner has done so far. Renders nothing when the content has
 * no path.
 */
export function PathSection({ path, tracks, progress, className }: {
  path: Path | null; tracks: Track[]; progress?: Record<string, Progress> | null; className?: string;
}) {
  const steps = (path?.steps ?? []).flatMap((s) => {
    const track = tracks.find((x) => x.id === s.track);
    return track ? [{ ...s, track }] : [];
  });
  if (!path || steps.length === 0) return null;

  // Only required steps count: a foundation prepares for the goal, it is not part of it.
  const required = steps.filter((s) => s.role === "required");
  const passed = required.filter((s) => assessmentPassed(s.track, progress?.[s.track.id])).length;

  return (
    <Card padded className={className}>
      <section aria-labelledby="path-title" className="flex h-full flex-col">
        <SectionLabel icon={<Route className="text-accent" aria-hidden />} className="mb-1.5">{t("Learning path")}</SectionLabel>
        <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <h2 id="path-title" className="text-lg font-semibold tracking-tight">{path.title}</h2>
          {required.length > 0 && (
            // What counts differs by track (mock exam, exam simulation), so the visible text names none.
            <p className="text-label text-muted" title={t("Full mock exams and exam simulations. A quick round does not count.")}>
              {t("{n} of {m} passed", { n: passed, m: required.length })}
              <span className="sr-only"> ({t("full mock exams and exam simulations")})</span>
            </p>
          )}
        </div>
        {path.summary && <p className="mt-1 text-label leading-relaxed text-muted">{path.summary}</p>}

        <ol className="mt-4 flex flex-col">
          {steps.map((s) => {
            const w = words(s.track), pr = progress?.[s.track.id];
            const ready = Math.round(pr?.readiness ?? 0), done = assessmentPassed(s.track, pr);
            const reached = !!pr && pr.readiness >= s.track.exam.passPercent;
            const label = s.role === "foundation" ? t("Foundation") : w.kindLabel;
            const name = [s.track.short, s.track.title, label, t("{n}% ready", { n: ready }), done && w.assessmentPassed].filter(Boolean).join(", ");
            return (
              <li key={s.track.id}>
                {/* One line per step, so the card stays as short as the hero beside it. Why the step
                    is on the path is its tooltip and its description for a screen reader. */}
                <Link to={`/t/${s.track.id}`} aria-label={name} aria-describedby={s.why ? `path-why-${s.track.id}` : undefined} title={s.why || undefined}
                  className="group -mx-2 grid min-h-11 grid-cols-[3.5rem_5.5rem_minmax(0,1fr)_3.75rem] items-center gap-x-3 rounded-lg px-2 transition-colors hover:bg-panel2">
                  <span className="truncate text-label font-bold tracking-wide uppercase transition-colors group-hover:text-accent">{s.track.short}</span>
                  <span className="flex"><Badge>{label}</Badge></span>
                  <Bar value={ready} tone={reached ? "ok" : "accent"} />
                  <span className={cx("flex items-center justify-end gap-1 text-label tabular-nums", !pr && "text-muted")}>
                    {pr ? `${ready}%` : "–"}
                    {done && <CheckCircle2 className="size-4 shrink-0 text-ok" aria-hidden data-testid="path-passed" />}
                  </span>
                  {s.why && <span id={`path-why-${s.track.id}`} className="sr-only">{s.why}</span>}
                </Link>
              </li>
            );
          })}
        </ol>

        {(path.disclaimer || path.url) && (
          <div className="mt-auto flex flex-col items-start gap-1 pt-4 text-xs leading-relaxed text-muted">
            <div className="mb-3 w-full border-t" />
            {path.disclaimer && <p>{path.disclaimer}</p>}
            {path.url && (
              <a href={path.url} target="_blank" rel="noopener noreferrer" className="inline-flex min-h-6 items-center gap-1 font-medium text-accent hover:underline">
                {programLink(path.url)}<ExternalLink className="size-3" aria-hidden />
              </a>
            )}
          </div>
        )}
      </section>
    </Card>
  );
}

/** Names the site the link leaves for, so the link text says where it goes. */
function programLink(url: string): string {
  try {
    return t("Official program page on {site}", { site: new URL(url).hostname.replace(/^www\./, "") });
  } catch {
    return t("Official program page");
  }
}
