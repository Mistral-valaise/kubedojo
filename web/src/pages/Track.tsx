import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { BookOpen, CheckCircle2, ChevronRight, Circle, CircleDot, Clock, Play, Sparkles, Timer, Trophy } from "lucide-react";
import { api, type Progress, type QuestionMeta, type Track } from "../api";
import { useLoad } from "../hooks";
import { t } from "../i18n";
import { Badge, Bar, Button, Card, Difficulty, ErrorNote, Modal, Page, Ring, Spinner, cx } from "../components/ui";

export function TrackPage() {
  const { track: id = "" } = useParams();
  const track = useLoad(() => api.track(id), [id]);
  const progress = useLoad(() => api.progress(id), [id]);

  if (track.error) return <Page><ErrorNote>{track.error}</ErrorNote></Page>;
  if (!track.data) return <Page><Spinner /></Page>;
  const tr = track.data, pr = progress.data;

  return (
    <Page crumbs={[{ label: tr.short }]}>
      <div className="mb-8 flex flex-wrap items-center gap-6">
        <Ring value={pr?.readiness ?? 0} tone={pr && pr.readiness >= tr.exam.passPercent ? "ok" : "accent"}>
          <span className="text-2xl font-bold tabular-nums">{Math.round(pr?.readiness ?? 0)}%</span>
          <span className="text-[10px] uppercase tracking-wider text-muted">{t("ready")}</span>
        </Ring>
        <div className="min-w-0 flex-1">
          <h1 className="text-3xl font-bold tracking-tight">{tr.title}</h1>
          <p className="mt-1.5 max-w-2xl text-muted">{tr.description}</p>
          <p className="mt-2 text-sm text-muted">
            {t("Readiness is your best result per question, weighted like the real exam. Pass mark: {p}%.", { p: tr.exam.passPercent })}
          </p>
        </div>
      </div>

      <div className="mb-10 grid gap-5 lg:grid-cols-[1fr_340px]">
        <Recommendations track={tr} progress={pr} />
        <ExamCard track={tr} progress={pr} />
      </div>

      <h2 className="mb-4 text-xs font-semibold uppercase tracking-wider text-muted">{t("Domains")}</h2>
      <div className="space-y-5">
        {tr.domains.map((d) => (
          <DomainSection key={d.id} track={tr} domainId={d.id} progress={pr} />
        ))}
      </div>
    </Page>
  );
}

function Recommendations({ track, progress }: { track: Track; progress: Progress | null }) {
  const recs = progress?.recommendations ?? [];
  return (
    <Card className="p-5">
      <div className="mb-3 flex items-center gap-2 text-sm font-semibold"><Sparkles className="size-4 text-accent" />{t("What to practise next")}</div>
      {!progress && <Spinner />}
      {progress && recs.length === 0 && (
        <p className="text-sm text-muted">{t("Every question is solved. Run an exam simulation to prove it under time pressure.")}</p>
      )}
      <ul className="space-y-2">
        {recs.map((r, i) => (
          <li key={r.domain}>
            <Link to={r.next ? `/t/${track.id}/q/${r.next}` : `/t/${track.id}/lesson/${r.domain}`}
              className="group flex items-center gap-3 rounded-xl border bg-panel2/50 px-3.5 py-3 transition hover:border-accent/60">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-accent-soft text-xs font-bold text-accent">{i + 1}</span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{r.title}</span>
                <span className="block truncate text-xs text-muted">{r.reason}</span>
              </span>
              <ChevronRight className="size-4 text-muted transition group-hover:translate-x-0.5 group-hover:text-accent" />
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

  return (
    <Card className="flex flex-col p-5">
      <div className="mb-3 flex items-center gap-2 text-sm font-semibold"><Timer className="size-4 text-accent" />{t("Exam simulation")}</div>
      <p className="mb-4 text-sm leading-relaxed text-muted">
        {t("{q} tasks in {m} minutes, weighted like the real exam. No hints. Graded when you finish.", { q: full, m: track.exam.durationMinutes })}
      </p>
      {last && (
        <div className="mb-4 flex items-center gap-2 rounded-lg bg-panel2 px-3 py-2 text-sm">
          <Trophy className={cx("size-4", last.passed ? "text-ok" : "text-muted")} />
          <span className="text-muted">{t("Last result")}</span>
          <Link to={`/exam/${last.sessionId}`} className="ml-auto font-semibold tabular-nums hover:underline">{last.percent}%</Link>
          <Badge tone={last.passed ? "ok" : "bad"}>{last.passed ? t("passed") : t("failed")}</Badge>
        </div>
      )}
      <Button variant="primary" className="mt-auto" icon={<Play className="size-4" />} disabled={available === 0} onClick={() => setOpen(true)}>
        {t("Start exam simulation")}
      </Button>
      {open && (
        <Modal title={t("Start exam simulation")} onClose={() => !busy && setOpen(false)}>
          <p className="mb-4">{t("Fresh clusters are created for the exam, which takes one to two minutes. The timer starts only when you say so.")}</p>
          {error && <div className="mb-3"><ErrorNote>{error}</ErrorNote></div>}
          <div className="space-y-2">
            <button disabled={busy} onClick={() => start(track.exam.durationMinutes, full)}
              className="flex w-full items-center gap-3 rounded-xl border bg-panel2/60 p-3.5 text-left transition hover:border-accent disabled:opacity-50">
              <Timer className="size-5 text-accent" />
              <span className="flex-1">
                <span className="block font-medium text-text">{t("Full exam")}</span>
                <span className="text-xs">{t("{q} tasks · {m} minutes", { q: full, m: track.exam.durationMinutes })}</span>
              </span>
              {busy ? <Spinner /> : <ChevronRight className="size-4" />}
            </button>
            <button disabled={busy} onClick={() => start(35, Math.min(5, available))}
              className="flex w-full items-center gap-3 rounded-xl border bg-panel2/60 p-3.5 text-left transition hover:border-accent disabled:opacity-50">
              <Clock className="size-5 text-accent" />
              <span className="flex-1">
                <span className="block font-medium text-text">{t("Quick round")}</span>
                <span className="text-xs">{t("{q} tasks · {m} minutes", { q: Math.min(5, available), m: 35 })}</span>
              </span>
              <ChevronRight className="size-4" />
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
      <div className="flex flex-wrap items-center gap-4 border-b px-5 py-4">
        <div className="flex size-11 shrink-0 flex-col items-center justify-center rounded-xl bg-accent-soft text-accent">
          <span className="text-sm font-bold leading-none tabular-nums">{d.weight}%</span>
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="font-semibold tracking-tight">{d.title}</h3>
          <p className="truncate text-sm text-muted">{d.summary}</p>
        </div>
        <div className="w-36">
          <div className="mb-1 flex justify-between text-[11px] text-muted">
            <span>{t("{a} of {b} solved", { a: dp?.solved ?? 0, b: qs.length })}</span>
          </div>
          <Bar value={dp?.mastery ?? 0} tone={dp && dp.mastery >= 100 ? "ok" : "accent"} />
        </div>
        <Link to={`/t/${track.id}/lesson/${d.id}`}>
          <Button size="sm" icon={<BookOpen className="size-3.5" />}>{t("Lesson")}</Button>
        </Link>
      </div>
      {qs.length === 0 && <div className="px-5 py-4 text-sm text-muted">{t("No questions yet.")}</div>}
      <ul className="divide-y">
        {qs.map((q) => <QuestionRow key={q.id} track={track.id} q={q} best={progress?.questions[q.id]?.best} />)}
      </ul>
    </Card>
  );
}

export function QuestionRow({ track, q, best }: { track: string; q: QuestionMeta; best?: number }) {
  const icon = best === undefined ? <Circle className="size-4 text-line" />
    : best >= 1 ? <CheckCircle2 className="size-4 text-ok" /> : <CircleDot className="size-4 text-warn" />;
  return (
    <li>
      <Link to={`/t/${track}/q/${q.id}`} className="group flex items-center gap-3 px-5 py-2.5 transition hover:bg-panel2/60">
        {icon}
        <span className="min-w-0 flex-1 truncate text-sm">{q.title}</span>
        {best !== undefined && best < 1 && <span className="text-xs tabular-nums text-warn">{Math.round(best * 100)}%</span>}
        <Difficulty level={q.difficulty} />
        <span className="hidden w-14 text-right text-xs tabular-nums text-muted sm:block">{q.minutes} min</span>
        <ChevronRight className="size-4 text-muted transition group-hover:translate-x-0.5 group-hover:text-accent" />
      </Link>
    </li>
  );
}
