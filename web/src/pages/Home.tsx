import { Link, useNavigate } from "react-router";
import { ArrowRight, BookOpen, CheckCircle2, GraduationCap, Layers, Terminal as TerminalIcon, Timer, Trash2 } from "lucide-react";
import { api, type Progress, type Session, type Track } from "../api";
import { useInterval, useLoad } from "../hooks";
import { t } from "../i18n";
import { Badge, Bar, Button, Card, ErrorNote, Page, Spinner } from "../components/ui";

export function Home() {
  const tracks = useLoad(() => api.tracks(), []);
  const sessions = useLoad(() => api.sessions(), []);
  const progress = useLoad(async () => {
    const list = await api.tracks();
    const entries = await Promise.all(list.map(async (tr) => [tr.id, await api.progress(tr.id)] as const));
    return Object.fromEntries(entries) as Record<string, Progress>;
  }, []);
  useInterval(() => void sessions.reload(), 4000);

  return (
    <Page>
      <section className="mb-10 max-w-3xl">
        <h1 className="text-4xl font-bold tracking-tight sm:text-[2.75rem] sm:leading-[1.1]">
          {t("Pass your Kubernetes exam by doing,")} <span className="text-accent">{t("not by reading.")}</span>
        </h1>
        <p className="mt-4 text-lg leading-relaxed text-muted">
          {t("Real clusters, a real terminal and tasks in exam format. Learn each topic in minutes, practise single questions with instant grading, then prove it in a timed exam simulation.")}
        </p>
      </section>

      {tracks.error && <ErrorNote>{tracks.error}</ErrorNote>}
      {!tracks.data && !tracks.error && <Spinner />}

      {sessions.data && sessions.data.length > 0 && tracks.data && (
        <ActiveSessions sessions={sessions.data} tracks={tracks.data} onChange={() => void sessions.reload()} />
      )}

      <div className="grid gap-5 md:grid-cols-2">
        {tracks.data?.map((tr) => <TrackCard key={tr.id} track={tr} progress={progress.data?.[tr.id]} />)}
      </div>

      <section className="mt-14 grid gap-5 sm:grid-cols-3">
        {[
          { icon: BookOpen, title: t("Learn"), text: t("One short lesson per exam domain: the 20% of knowledge that solves 80% of the tasks.") },
          { icon: TerminalIcon, title: t("Practise"), text: t("Solve single questions on a live cluster. Checks tell you what is wrong and why, hints and solutions are one click away.") },
          { icon: Timer, title: t("Simulate"), text: t("Two hours, 17 weighted tasks, no hints. Get a score per domain and see exactly where points were lost.") },
        ].map((f) => (
          <div key={f.title} className="rounded-2xl border border-dashed p-5">
            <f.icon className="mb-3 size-5 text-accent" />
            <h3 className="mb-1 font-semibold">{f.title}</h3>
            <p className="text-sm leading-relaxed text-muted">{f.text}</p>
          </div>
        ))}
      </section>
    </Page>
  );
}

function TrackCard({ track, progress }: { track: Track; progress?: Progress }) {
  const count = track.questions?.length ?? 0;
  return (
    <Card className="flex flex-col p-6 transition hover:border-accent/50">
      <div className="mb-4 flex items-start justify-between gap-4">
        <div>
          <div className="mb-1.5 flex items-center gap-2">
            <span className="rounded-lg bg-accent-soft px-2 py-1 text-xs font-bold tracking-wide text-accent">{track.short}</span>
            <Badge>Kubernetes {track.kubernetesVersion}</Badge>
          </div>
          <h2 className="text-xl font-semibold tracking-tight">{track.title}</h2>
        </div>
        <GraduationCap className="size-6 shrink-0 text-muted" />
      </div>
      <p className="mb-5 text-sm leading-relaxed text-muted">{track.description}</p>
      <div className="mb-5 flex flex-wrap gap-x-5 gap-y-1.5 text-sm text-muted">
        <span className="inline-flex items-center gap-1.5"><Layers className="size-4" />{t("{n} domains", { n: track.domains.length })}</span>
        <span className="inline-flex items-center gap-1.5"><CheckCircle2 className="size-4" />{t("{n} questions", { n: count })}</span>
        <span className="inline-flex items-center gap-1.5"><Timer className="size-4" />{t("{n} min exam", { n: track.exam.durationMinutes })}</span>
      </div>
      <div className="mb-6">
        <div className="mb-1.5 flex justify-between text-xs">
          <span className="text-muted">{t("Exam readiness")}</span>
          <span className="font-medium tabular-nums">{progress ? `${Math.round(progress.readiness)}%` : "–"}</span>
        </div>
        <Bar value={progress?.readiness ?? 0} tone={progress && progress.readiness >= track.exam.passPercent ? "ok" : "accent"} />
      </div>
      <div className="mt-auto flex gap-2">
        <Link to={`/t/${track.id}`} className="flex-1">
          <Button variant="primary" className="w-full" icon={<ArrowRight className="size-4" />}>{t("Open track")}</Button>
        </Link>
      </div>
    </Card>
  );
}

function ActiveSessions({ sessions, tracks, onChange }: { sessions: Session[]; tracks: Track[]; onChange: () => void }) {
  const nav = useNavigate();
  const short = (id: string) => tracks.find((x) => x.id === id)?.short ?? id;
  return (
    <Card className="mb-8 divide-y">
      <div className="px-5 py-3 text-xs font-semibold uppercase tracking-wider text-muted">{t("Active sandboxes")}</div>
      {sessions.map((s) => {
        const hosts = Object.values(s.envs ?? {});
        return (
          <div key={s.id} className="flex flex-wrap items-center gap-3 px-5 py-3">
            <span className="pulse size-2 rounded-full bg-ok" />
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium">
                {short(s.track)} · {s.mode === "exam" ? t("Exam simulation") : t("Practice")}
                <span className="ml-2 align-middle"><Badge tone={s.status === "error" ? "bad" : s.status === "running" ? "warn" : "muted"}>{t(s.status)}</Badge></span>
              </div>
              <div className="truncate text-xs text-muted">
                {hosts.length ? t("Clusters: {hosts}", { hosts: hosts.join(", ") }) : t("No cluster yet")}
              </div>
            </div>
            {s.mode === "exam" && <Button size="sm" onClick={() => nav(`/exam/${s.id}`)}>{s.status === "finished" ? t("View report") : t("Resume")}</Button>}
            {s.mode === "practice" && <Button size="sm" onClick={() => nav(`/t/${s.track}`)}>{t("Continue")}</Button>}
            <Button size="sm" variant="ghost" icon={<Trash2 className="size-3.5" />}
              onClick={async () => { await api.destroy(s.id); setTimeout(onChange, 400); }}>
              {t("End")}
            </Button>
          </div>
        );
      })}
    </Card>
  );
}
