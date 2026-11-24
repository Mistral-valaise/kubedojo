import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { BookOpen, CheckCircle2, ChevronRight, Circle, CircleDot, Clock, Info, Play, Sparkles, Timer, Trophy } from "lucide-react";
import { api, type Progress, type QuestionMeta, type Track } from "../api";
import { useLoad } from "../hooks";
import { t } from "../i18n";
import { quickRound, standing, words } from "../track";
import { Badge, Bar, Button, ButtonLink, Card, Difficulty, ErrorNote, Loading, Modal, Page, Ring, SectionLabel, Skeleton, Spinner, cx } from "../components/ui";

export function TrackPage() {
  const { track: id = "" } = useParams();
  const track = useLoad(() => api.track(id), [id]);
  const progress = useLoad(() => api.progress(id), [id]);

  if (track.error) return <Page docs={null}><LoadError message={track.error} onRetry={() => void track.reload()} /></Page>;
  if (!track.data) return <Page docs={null}><TrackSkeleton /></Page>;
  const tr = track.data, pr = progress.data, w = words(tr);

  return (
    <Page docs={w.docs} crumbs={[{ label: tr.short }]}>
      <div className="mb-8 flex flex-wrap items-center gap-x-6 gap-y-5">
        <Ring value={pr?.readiness ?? 0} tone={pr && pr.readiness >= tr.exam.passPercent ? "ok" : "accent"}>
          <span className="text-2xl font-bold tabular-nums">{pr ? `${Math.round(pr.readiness)}%` : "–"}</span>
          <span className="text-xs tracking-wider text-muted uppercase">{t("ready")}</span>
        </Ring>
        <div className="min-w-0 flex-1 basis-64">
          <h1 className="text-3xl font-bold tracking-tight">{tr.title}</h1>
          <p className="mt-1.5 max-w-2xl text-base text-muted">{tr.description}</p>
          <p className="mt-2 max-w-2xl text-sm text-muted">{w.readinessNote(tr.exam.passPercent)}</p>
          {w.kindNote && (
            <p className="mt-3 flex max-w-2xl items-start gap-2 rounded-xl border bg-panel2 px-3.5 py-2.5 text-sm text-muted">
              <Info className="mt-0.5 size-4 shrink-0 text-accent" aria-hidden /><span>{w.kindNote}</span>
            </p>
          )}
        </div>
      </div>

      {progress.error && (
        <ErrorNote className="mb-5" onRetry={() => void progress.reload()}>
          {t("Your progress could not be loaded, so nothing here shows what you have solved.")} <span className="font-mono text-xs break-words">{progress.error}</span>
        </ErrorNote>
      )}

      <div className="mb-10 grid grid-cols-[minmax(0,1fr)] gap-5 lg:grid-cols-[minmax(0,1fr)_340px]">
        <Recommendations track={tr} progress={pr} failed={!!progress.error} />
        <ExamCard track={tr} progress={pr} />
      </div>

      <SectionLabel as="h2" className="mb-4">{t("Domains")}</SectionLabel>
      <div className="space-y-5">
        {tr.domains.map((d) => (
          <DomainSection key={d.id} track={tr} domainId={d.id} progress={pr} />
        ))}
      </div>
    </Page>
  );
}

/** A page that could not load what it is about: the reason, a retry and the way home. */
export function LoadError({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <ErrorNote onRetry={onRetry}>
      <span className="font-medium">{t("This page could not be loaded.")}</span>{" "}
      <span className="font-mono text-xs break-words">{message}</span>{" "}
      <Link to="/" className="underline underline-offset-2">{t("Back to start")}</Link>
    </ErrorNote>
  );
}

function TrackSkeleton() {
  return (
    <Loading label={t("Loading the track")}>
      <div className="mb-8 flex flex-wrap items-center gap-6">
        <Skeleton className="size-[120px] rounded-full" />
        <div className="min-w-0 flex-1 basis-64 space-y-3">
          <Skeleton className="h-9 w-4/5 max-w-md" />
          <Skeleton className="h-5 max-w-xl" />
          <Skeleton className="h-4 w-3/5 max-w-sm" />
        </div>
      </div>
      <div className="mb-10 grid grid-cols-[minmax(0,1fr)] gap-5 lg:grid-cols-[minmax(0,1fr)_340px]">
        <Skeleton className="h-48 rounded-2xl" />
        <Skeleton className="h-48 rounded-2xl" />
      </div>
      <div className="space-y-5">
        <Skeleton className="h-40 rounded-2xl" />
        <Skeleton className="h-40 rounded-2xl" />
      </div>
    </Loading>
  );
}

function Recommendations({ track, progress, failed }: { track: Track; progress: Progress | null; failed: boolean }) {
  const recs = progress?.recommendations ?? [];
  return (
    <Card padded>
      <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold"><Sparkles className="size-4 text-accent" aria-hidden />{t("What to practise next")}</h2>
      {!progress && !failed && (
        <Loading label={t("Loading your progress")} className="space-y-2">
          {[0, 1, 2].map((i) => <Skeleton key={i} className="h-14 rounded-xl" />)}
        </Loading>
      )}
      {!progress && failed && <p className="text-sm text-muted">{t("No recommendation without your progress.")}</p>}
      {progress && recs.length === 0 && (
        <p className="text-sm text-muted">{words(track).allSolved}</p>
      )}
      <ul className="space-y-2">
        {recs.map((r, i) => (
          <li key={r.domain}>
            <Link to={r.next ? `/t/${track.id}/q/${r.next}` : `/t/${track.id}/lesson/${r.domain}`}
              className="group flex min-h-14 items-center gap-3 rounded-xl border bg-panel2 px-3.5 py-2.5 transition-colors hover:border-accent/60">
              <span aria-hidden className="flex size-6 shrink-0 items-center justify-center rounded-full bg-accent-soft text-xs font-bold text-accent">{i + 1}</span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium sm:truncate">{r.title}</span>
                <span className="block text-label text-muted sm:truncate">{r.reason}</span>
              </span>
              <ChevronRight aria-hidden className="size-4 shrink-0 text-muted transition group-hover:translate-x-0.5 group-hover:text-accent" />
            </Link>
          </li>
        ))}
      </ul>
    </Card>
  );
}

function ExamCard({ track, progress }: { track: Track; progress: Progress | null }) {
  const nav = useNavigate();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const available = track.questions?.length ?? 0;
  const full = Math.min(track.exam.questions, available);
  const last = progress?.exams.at(-1);
  const w = words(track), quick = quickRound(track);

  const start = async (minutes: number, questions: number) => {
    setBusy(true);
    setError(null);
    try {
      const s = await api.createSession({ mode: "exam", track: track.id, minutes, questions });
      nav(`/exam/${s.id}`);
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };
  const option = "flex min-h-16 w-full items-center gap-3 rounded-xl border bg-panel2 p-3.5 text-left transition-colors hover:border-accent disabled:opacity-50";

  return (
    <Card padded className="flex flex-col">
      <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold"><Timer className="size-4 text-accent" aria-hidden />{w.assessment}</h2>
      <p className="mb-4 text-sm leading-relaxed text-muted">
        {w.assessmentSummary(full, track.exam.durationMinutes)}
      </p>
      {last && (
        <div className="mb-4 flex flex-wrap items-center gap-2 rounded-lg bg-panel2 px-3 py-2 text-sm">
          <Trophy className={cx("size-4", last.passed ? "text-ok" : "text-muted")} aria-hidden />
          <span className="text-muted">{t("Last result")}</span>
          <Link to={`/exam/${last.sessionId}`} className="ml-auto font-semibold tabular-nums underline-offset-2 hover:underline">{last.percent}%</Link>
          <Badge tone={last.passed ? "ok" : "bad"}>{last.passed ? t("passed") : t("failed")}</Badge>
        </div>
      )}
      <Button variant="primary" size="lg" className="mt-auto" icon={<Play className="size-4" aria-hidden />} disabled={available === 0} onClick={() => setOpen(true)}>
        {w.startAssessment}
      </Button>
      {open && (
        <Modal title={w.startAssessment} onClose={() => !busy && setOpen(false)}>
          <p className="mb-4">{w.startNote}</p>
          {error && <div className="mb-3"><ErrorNote>{error}</ErrorNote></div>}
          <div className="space-y-2">
            <button disabled={busy} onClick={() => start(track.exam.durationMinutes, full)} className={option}>
              <Timer className="size-5 shrink-0 text-accent" aria-hidden />
              <span className="min-w-0 flex-1">
                <span className="block font-medium text-text">{w.fullAssessment}</span>
                <span className="text-label">{w.assessmentOption(full, track.exam.durationMinutes)}</span>
              </span>
              {busy ? <Spinner /> : <ChevronRight className="size-4 shrink-0" aria-hidden />}
            </button>
            <button disabled={busy} onClick={() => start(quick.minutes, quick.questions)} className={option}>
              <Clock className="size-5 shrink-0 text-accent" aria-hidden />
              <span className="min-w-0 flex-1">
                <span className="block font-medium text-text">{t("Quick round")}</span>
                <span className="text-label">{w.assessmentOption(quick.questions, quick.minutes)}</span>
              </span>
              <ChevronRight className="size-4 shrink-0" aria-hidden />
            </button>
          </div>
        </Modal>
      )}
    </Card>
  );
}

function DomainSection({ track, domainId, progress }: { track: Track; domainId: string; progress: Progress | null }) {
  const d = track.domains.find((x) => x.id === domainId)!;
  const qs = (track.questions ?? []).filter((q) => q.domain === domainId);
  const dp = progress?.domains.find((x) => x.id === domainId);
  return (
    <Card className="overflow-hidden">
      {/* On a phone the title takes the first row and the progress with the lesson button the second. */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-3 border-b px-5 py-4">
        <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-accent-soft text-accent" title={t("Weight in the assessment")}>
          <span className="text-sm font-bold leading-none tabular-nums">{d.weight}%</span>
        </div>
        <div className="min-w-0 flex-1 basis-[calc(100%-4rem)] sm:basis-0">
          <h3 className="font-semibold tracking-tight">{d.title}</h3>
          {d.summary && <p className="text-sm text-muted sm:truncate">{d.summary}</p>}
        </div>
        <div className="flex w-full items-center gap-4 sm:w-auto">
          <div className="min-w-0 flex-1 sm:w-40 sm:flex-none">
            <div className="mb-1.5 text-xs text-muted">{t("{a} of {b} solved", { a: dp?.solved ?? 0, b: qs.length })}</div>
            <Bar value={dp?.mastery ?? 0} tone={dp && dp.mastery >= 100 ? "ok" : "accent"} />
          </div>
          <ButtonLink to={`/t/${track.id}/lesson/${d.id}`} size="sm" icon={<BookOpen className="size-3.5" aria-hidden />}
            aria-label={t("Lesson: {title}", { title: d.title })}>{t("Lesson")}</ButtonLink>
        </div>
      </div>
      {qs.length === 0 && <div className="px-5 py-4 text-sm text-muted">{words(track).noQuestions}</div>}
      <ul className="divide-y">
        {qs.map((q) => <QuestionRow key={q.id} track={track.id} q={q} best={standing(progress, q)} />)}
      </ul>
    </Card>
  );
}

export function QuestionRow({ track, q, best, compact }: { track: string; q: QuestionMeta; best?: number; compact?: boolean }) {
  // The state is in the shape of the icon and in its name, not in its colour alone.
  const icon = best === undefined ? <Circle role="img" aria-label={t("Not attempted")} className="size-4 shrink-0 text-muted" />
    : best >= 1 ? <CheckCircle2 role="img" aria-label={t("Solved")} className="size-4 shrink-0 text-ok" />
      : <CircleDot role="img" aria-label={t("Attempted")} className="size-4 shrink-0 text-warn" />;
  return (
    <li>
      <Link to={`/t/${track}/q/${q.id}`} className={cx("group flex min-h-11 items-center gap-3 py-2.5 transition-colors hover:bg-panel2", compact ? "px-4" : "px-5")}>
        {icon}
        <span className={cx("min-w-0 flex-1 text-sm", !compact && "sm:truncate")}>{q.title}</span>
        {best !== undefined && best < 1 && <span className="text-xs tabular-nums text-warn">{Math.round(best * 100)}%</span>}
        <Difficulty level={q.difficulty} />
        {/* A choice question has no time budget of its own; the cell stays so the rows align. */}
        {!compact && <span className="hidden w-14 text-right text-xs tabular-nums text-muted sm:block">{q.type === "choice" ? "" : `${q.minutes} min`}</span>}
        <ChevronRight aria-hidden className="size-4 shrink-0 text-muted transition group-hover:translate-x-0.5 group-hover:text-accent" />
      </Link>
    </li>
  );
}
