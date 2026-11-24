import { useState } from "react";
import { ArrowRight, BookOpen, FolderOpen, Sparkles, Terminal as TerminalIcon, Timer, Trash2 } from "lucide-react";
import { api, type Progress, type Session, type Track } from "../api";
import { useConfig } from "../config";
import { useInterval, useLoad } from "../hooks";
import { plural, t } from "../i18n";
import { words } from "../track";
import { PathSection } from "../components/PathSection";
import { Badge, Bar, Button, ButtonLink, Card, EmptyState, ErrorNote, Loading, Page, SectionLabel, Skeleton, buttonClass } from "../components/ui";

/** Id of the track grid: the target of "Browse all tracks". */
const TRACKS_ID = "tracks";

export function Home() {
  const config = useConfig();
  const tracks = useLoad(() => api.tracks(), []);
  // Wrapped, because "this content has no path" is an answer too (null), not the same as still loading.
  const path = useLoad(async () => ({ path: await api.path() }), []);
  const sessions = useLoad(() => api.sessions(), []);
  // The progress of every track, once the tracks are known.
  const progress = useLoad(async () => {
    if (!tracks.data) return null;
    const entries = await Promise.all(tracks.data.map(async (tr) => [tr.id, await api.progress(tr.id)] as const));
    return Object.fromEntries(entries) as Record<string, Progress>;
  }, [tracks.data]);
  useInterval(() => void sessions.reload(), 4000);

  const list = tracks.data;
  const hasPath = !!list && !!path.data?.path && (path.data.path.steps ?? []).some((s) => list.some((tr) => tr.id === s.track));
  // Until both are known the hero keeps the room of the path card, so that it does not jump.
  const pathPending = !tracks.error && !path.error && (!list || !path.data);
  // What to continue is known once everything it is derived from has answered, or has failed.
  const settled = (!!list || !!tracks.error) && (!!sessions.data || !!sessions.error) && (!!progress.data || !!progress.error || !!tracks.error);
  const next = list ? continueWith(list, sessions.data, progress.data) : null;

  const tiles = [
    { icon: BookOpen, title: t("Learn"), text: t("One short lesson per domain: the 20% of knowledge that solves 80% of the tasks.") },
    { icon: TerminalIcon, title: t("Practise"), text: t("Solve single questions in a live sandbox. Checks tell you what is wrong and why, hints and solutions are one click away.") },
    // Only where the server has an assistant to offer.
    ...(config?.tutor ? [{ icon: Sparkles, title: t("Ask"), text: t("Stuck on a failed check? The assistant explains it from your task and your cluster, with the model you choose.") }] : []),
    { icon: Timer, title: t("Simulate"), text: t("Timed and weighted like the real exam, no hints. Get a score per domain and see exactly where points were lost.") },
  ];

  return (
    <Page>
      <div className="flex flex-col gap-10 sm:gap-14">
        <section className={(hasPath || pathPending) ? "grid items-center gap-8 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)] lg:gap-10" : undefined}>
          <div className="flex min-w-0 flex-col gap-5 sm:py-4">
            <h1 className="text-display font-bold tracking-tight">
              {t("Get good at Linux and Kubernetes by doing,")} <span className="text-accent">{t("not by reading.")}</span>
            </h1>
            <p className="max-w-[56ch] text-base text-muted sm:text-lg sm:leading-[1.65]">
              {t("Real terminals, real clusters and tasks graded on the spot. Learn a topic in minutes, practise it, then prove it against the clock.")}
            </p>
            <div className="flex flex-wrap gap-2.5 pt-1">
              {!settled ? <Loading label={t("Loading your progress")}><Skeleton className="h-11 w-52 rounded-[0.625rem]" /></Loading> : next ? (
                <>
                  <ButtonLink to={next.to} variant="primary" size="lg">
                    {t("Continue with {track}", { track: next.track.short })}<ArrowRight className="size-4" aria-hidden />
                  </ButtonLink>
                  <a href={`#${TRACKS_ID}`} className={buttonClass({ size: "lg" })}>{t("Browse all tracks")}</a>
                </>
              ) : (
                <a href={`#${TRACKS_ID}`} className={buttonClass({ variant: "primary", size: "lg" })}>
                  {t("Browse all tracks")}<ArrowRight className="size-4" aria-hidden />
                </a>
              )}
            </div>
          </div>
          {hasPath && list && <PathSection path={path.data!.path} tracks={list} progress={progress.data} />}
          {pathPending && (
            <Loading label={t("Loading the learning path")}>
              <Card padded className="space-y-4">
                <Skeleton className="h-4 w-28" />
                <Skeleton className="h-6 w-56 max-w-full" />
                <Skeleton className="h-12" />
                {[0, 1, 2, 3, 4, 5].map((i) => <Skeleton key={i} className="h-7" />)}
                <Skeleton className="h-24" />
              </Card>
            </Loading>
          )}
        </section>

        {(path.error || sessions.error || progress.error) && (
          <div className="space-y-3">
            {path.error && <ErrorNote onRetry={() => void path.reload()}>{t("The learning path could not be loaded.")} <Detail>{path.error}</Detail></ErrorNote>}
            {progress.error && <ErrorNote onRetry={() => void progress.reload()}>{t("Your progress could not be loaded, so the readiness shown here is missing.")} <Detail>{progress.error}</Detail></ErrorNote>}
            {sessions.error && <ErrorNote onRetry={() => void sessions.reload()}>{t("Your active sessions could not be loaded.")} <Detail>{sessions.error}</Detail></ErrorNote>}
          </div>
        )}

        {sessions.data && sessions.data.length > 0 && list && (
          <ActiveSessions sessions={sessions.data} tracks={list} onChange={() => sessions.reload()} />
        )}

        <section id={TRACKS_ID} aria-labelledby="tracks-title" className="flex scroll-mt-20 flex-col gap-5">
          <div className="flex flex-wrap items-baseline gap-x-3">
            <h2 id="tracks-title" className="text-title font-semibold tracking-tight">{t("Tracks")}</h2>
            {list && list.length > 0 && <span className="text-sm text-muted">{plural(list.length, "{n} track", "{n} tracks")}</span>}
          </div>
          {tracks.error && (
            <ErrorNote onRetry={() => void tracks.reload()}>{t("The tracks could not be loaded.")} <Detail>{tracks.error}</Detail></ErrorNote>
          )}
          {!list && !tracks.error && (
            <Loading label={t("Loading the tracks")} className="grid gap-5 md:grid-cols-2 lg:grid-cols-3">
              {[0, 1, 2].map((i) => <TrackCardSkeleton key={i} />)}
            </Loading>
          )}
          {list && list.length === 0 && (
            <EmptyState level={3} icon={<FolderOpen />} title={t("No tracks yet")}
              text={t("This server has no content to practise. Tracks are loaded from its content directory when it starts.")}
              action={<Button onClick={() => void tracks.reload()}>{t("Check again")}</Button>} />
          )}
          {list && list.length > 0 && (
            <div className="grid gap-5 md:grid-cols-2 lg:grid-cols-3">
              {list.map((tr) => <TrackCard key={tr.id} track={tr} progress={progress.data?.[tr.id]} />)}
            </div>
          )}
        </section>

        <section aria-label={t("How it works")} className="grid gap-x-6 gap-y-8 sm:grid-cols-2 lg:grid-flow-col lg:auto-cols-fr lg:grid-cols-none">
          {tiles.map((f) => (
            <div key={f.title} className="flex flex-col gap-2">
              <f.icon className="size-5.5 text-accent" aria-hidden />
              <h3 className="text-base font-semibold">{f.title}</h3>
              <p className="text-sm leading-relaxed text-muted">{f.text}</p>
            </div>
          ))}
        </section>
      </div>
    </Page>
  );
}

/** The technical message of an error, after the sentence that says what is missing. */
function Detail({ children }: { children: string }) {
  return <span className="font-mono text-xs break-words opacity-90">{children}</span>;
}

/**
 * Where the learner left off, for the main action of the page: a session that
 * is still open, else the track they have come furthest in without finishing
 * it. Null when there is nothing to continue.
 */
export function continueWith(tracks: Track[], sessions: Session[] | null, progress: Record<string, Progress> | null): { track: Track; to: string } | null {
  for (const s of sessions ?? []) {
    const track = tracks.find((tr) => tr.id === s.track);
    if (!track || s.status === "error" || s.status === "destroyed") continue;
    return { track, to: s.mode === "exam" ? `/exam/${s.id}` : `/t/${track.id}` };
  }
  let best: Track | null = null;
  for (const track of tracks) {
    const p = progress?.[track.id];
    const started = !!p && (p.attempts > 0 || p.readiness > 0 || Object.keys(p.questions ?? {}).length > 0);
    if (!started || p.readiness >= 100) continue;
    if (!best || p.readiness > (progress?.[best.id]?.readiness ?? 0)) best = track;
  }
  return best ? { track: best, to: `/t/${best.id}` } : null;
}

export function TrackCard({ track, progress }: { track: Track; progress?: Progress }) {
  const count = track.questions?.length ?? 0;
  const w = words(track);
  return (
    <Card padded className="flex flex-col gap-4 transition-colors hover:border-accent/50">
      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded-lg bg-accent-soft px-2.5 py-1 text-xs font-bold tracking-wide text-accent uppercase">{track.short}</span>
        <Badge>{w.kindLabel}</Badge>
        {w.kubernetes && <Badge>{w.kubernetes}</Badge>}
      </div>
      <div className="flex flex-col gap-1.5">
        <h3 className="text-lg font-semibold tracking-tight">{track.title}</h3>
        {track.description && <p className="text-sm leading-relaxed text-muted">{track.description}</p>}
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-label text-muted">
        <span>{plural(track.domains.length, "{n} domain", "{n} domains")}</span>
        <span>{w.questions(count)}</span>
        <span>{w.duration(track.exam.durationMinutes)}</span>
      </div>
      <div className="mt-auto flex flex-col gap-2 pt-1">
        <div className="flex justify-between text-label">
          <span className="text-muted">{w.readiness}</span>
          <span className="font-medium tabular-nums">{progress ? `${Math.round(progress.readiness)}%` : "–"}</span>
        </div>
        <Bar value={progress?.readiness ?? 0} tone={progress && progress.readiness >= track.exam.passPercent ? "ok" : "accent"} />
      </div>
      <ButtonLink to={`/t/${track.id}`} size="lg" className="w-full" aria-label={t("Open track {track}", { track: track.short })}>
        {t("Open track")}<ArrowRight className="size-4" aria-hidden />
      </ButtonLink>
    </Card>
  );
}

function TrackCardSkeleton() {
  return (
    <Card padded className="flex flex-col gap-4">
      <Skeleton className="h-6 w-32" />
      <Skeleton className="h-6 w-4/5" />
      <Skeleton className="h-10" />
      <Skeleton className="h-4 w-3/5" />
      <Skeleton className="h-1.5 rounded-full" />
      <Skeleton className="h-11 rounded-[0.625rem]" />
    </Card>
  );
}

function ActiveSessions({ sessions, tracks, onChange }: { sessions: Session[]; tracks: Track[]; onChange: () => Promise<void> }) {
  const find = (id: string) => tracks.find((x) => x.id === id);
  // A session of a theory track has no sandbox, so the heading cannot promise one.
  const sandboxes = sessions.every((s) => words(find(s.track)).sandbox);
  const title = sandboxes ? t("Active sandboxes") : t("Active sessions");
  return (
    <Card>
      <section aria-label={title} className="divide-y">
        <SectionLabel as="h2" className="px-5 py-3">{title}</SectionLabel>
        {sessions.map((s) => <SessionRow key={s.id} session={s} track={find(s.track)} onChange={onChange} />)}
      </section>
    </Card>
  );
}

function SessionRow({ session: s, track, onChange }: { session: Session; track?: Track; onChange: () => Promise<void> }) {
  const [ending, setEnding] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const hosts = Object.values(s.envs ?? {});
  const w = words(track);
  // The list is read again once the server has confirmed the end: no guessing how long it takes.
  const end = async () => {
    setEnding(true); setError(null);
    try { await api.destroy(s.id); await onChange(); }
    catch (e) { setError((e as Error).message); }
    setEnding(false);
  };
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2 px-5 py-3">
      <span aria-hidden className={s.status === "error" ? "size-2 rounded-full bg-bad" : "pulse size-2 rounded-full bg-ok"} />
      <div className="min-w-0 flex-1 basis-56">
        <div className="text-sm font-medium">
          {track?.short ?? s.track} · {s.mode === "exam" ? w.assessment : t("Practice")}
          <span className="ml-2 align-middle"><Badge tone={s.status === "error" ? "bad" : s.status === "running" ? "warn" : "muted"}>{t(s.status)}</Badge></span>
        </div>
        <div className="truncate pt-0.5 text-label text-muted">{w.activeSession(hosts)}</div>
        {error && <div role="alert" className="pt-1 text-label text-bad">{t("The session could not be ended.")} <Detail>{error}</Detail></div>}
      </div>
      {s.mode === "exam" && <ButtonLink to={`/exam/${s.id}`}>{s.status === "finished" ? t("View report") : t("Resume")}</ButtonLink>}
      {s.mode === "practice" && <ButtonLink to={`/t/${s.track}`}>{t("Continue")}</ButtonLink>}
      <Button variant="ghost" loading={ending} icon={<Trash2 className="size-4" aria-hidden />} onClick={end}>{t("End")}</Button>
    </div>
  );
}
