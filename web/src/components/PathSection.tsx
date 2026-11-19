import { Link } from "react-router";
import { CheckCircle2, ExternalLink, Route } from "lucide-react";
import type { Path, Progress, Track } from "../api";
import { t } from "../i18n";
import { assessmentPassed, words } from "../track";
import { Badge, Bar, Card, Ring, cx } from "./ui";

/**
 * The recommended order through the tracks. It only recommends: every step is
 * a plain link, whatever the learner has done so far. Renders nothing when the
 * content has no path.
 */
export function PathSection({ path, tracks, progress }: {
  path: Path | null; tracks: Track[]; progress?: Record<string, Progress> | null;
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
    <Card className="mb-8 p-6">
      <section aria-labelledby="path-title">
        <div className="mb-6 flex flex-wrap items-end justify-between gap-x-10 gap-y-4">
          <div className="min-w-0 max-w-2xl">
            <div className="mb-1.5 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted">
              <Route className="size-4 text-accent" aria-hidden />{t("Learning path")}
            </div>
            <h2 id="path-title" className="text-xl font-semibold tracking-tight">{path.title}</h2>
            {path.summary && <p className="mt-1 text-sm leading-relaxed text-muted">{path.summary}</p>}
          </div>
          {required.length > 0 && (
            <div className="w-full sm:w-60">
              {/* What counts differs by track (mock exam, exam simulation), so the visible text names none. */}
              <p className="mb-1.5 text-xs text-muted" title={t("Full mock exams and exam simulations. A quick round does not count.")}>
                {t("{n} of {m} passed", { n: passed, m: required.length })}
                <span className="sr-only"> ({t("full mock exams and exam simulations")})</span>
              </p>
              <Bar value={(passed / required.length) * 100} tone={passed === required.length ? "ok" : "accent"} />
            </div>
          )}
        </div>

        <ol className="flex flex-col lg:flex-row">
          {steps.map((s, i) => {
            const w = words(s.track), pr = progress?.[s.track.id];
            const ready = Math.round(pr?.readiness ?? 0), done = assessmentPassed(s.track, pr);
            const label = s.role === "foundation" ? t("Foundation") : w.kindLabel;
            const name = [s.track.short, s.track.title, label, t("{n}% ready", { n: ready }), done && w.assessmentPassed].filter(Boolean).join(", ");
            return (
              <li key={s.track.id} className={cx("relative lg:flex-1 lg:basis-0", i < steps.length - 1 && cx(
                "pb-6 lg:pb-0",
                // The connector to the next step: below the ring on small screens, beside it on wide ones.
                "after:absolute after:top-14 after:bottom-2 after:left-6 after:w-px after:bg-line after:content-['']",
                "lg:after:top-6 lg:after:bottom-auto lg:after:left-[calc(50%+34px)] lg:after:h-px lg:after:w-[calc(100%-68px)]",
              ))}>
                <Link to={`/t/${s.track.id}`} aria-label={name} aria-describedby={s.why ? `path-why-${s.track.id}` : undefined}
                  className="group -m-2 flex items-start gap-4 rounded-xl p-2 lg:m-0 lg:-my-2 lg:flex-col lg:items-center lg:gap-2.5 lg:px-3 lg:text-center">
                  <span className="relative shrink-0">
                    <Ring value={ready} size={48} stroke={4} tone={pr && pr.readiness >= s.track.exam.passPercent ? "ok" : "accent"}>
                      <span className="text-[11px] font-semibold tabular-nums">{pr ? `${ready}%` : "–"}</span>
                    </Ring>
                    {done && (
                      <span className="absolute -right-1 -bottom-1 flex rounded-full bg-panel p-px text-ok" data-testid="path-passed">
                        <CheckCircle2 className="size-4" aria-hidden />
                      </span>
                    )}
                  </span>
                  <span className="min-w-0 pt-0.5 lg:pt-0">
                    <span className="flex flex-wrap items-center gap-2 lg:flex-col lg:gap-1.5">
                      <span className="font-semibold tracking-tight transition-colors group-hover:text-accent">{s.track.short}</span>
                      <Badge>{label}</Badge>
                    </span>
                    {s.why && <span id={`path-why-${s.track.id}`} className="mt-1.5 block text-xs leading-relaxed text-muted">{s.why}</span>}
                  </span>
                </Link>
              </li>
            );
          })}
        </ol>

        {(path.disclaimer || path.url) && (
          <div className="mt-6 flex flex-wrap items-start justify-between gap-x-10 gap-y-2 border-t pt-4 text-xs leading-relaxed text-muted">
            {path.disclaimer && <p className="max-w-3xl">{path.disclaimer}</p>}
            {path.url && (
              <a href={path.url} target="_blank" rel="noreferrer" className="inline-flex shrink-0 items-center gap-1 font-medium text-accent hover:underline">
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
