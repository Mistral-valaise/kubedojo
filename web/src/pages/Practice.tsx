import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowLeft, ChevronRight, Clock, PlayCircle, RotateCcw, Server } from "lucide-react";
import { api, type QuestionView, type Result, type Session, type Track } from "../api";
import { useConfig } from "../config";
import { useElapsed, useInterval, useLoad } from "../hooks";
import { t } from "../i18n";
import { practicable, words } from "../track";
import { Help } from "../components/Help";
import { Markdown } from "../components/Markdown";
import { Provisioning } from "../components/Provisioning";
import { Rating } from "../components/Rating";
import { Results } from "../components/Results";
import { Split } from "../components/Split";
import { TerminalTabs } from "../components/TerminalTabs";
import { HeaderPager, WorkspaceHeader } from "../components/WorkspaceHeader";
import { Badge, Button, Difficulty, ErrorNote, Loading, MAIN_ID, Modal, Page, Skeleton, Spinner, cx } from "../components/ui";
import { QuizPractice } from "./QuizPractice";
import { LoadError } from "./Track";

/** After this many seconds a running check gets a note that says why it takes long. */
const SLOW_CHECK_SECONDS = 3;

/** Practice of one question: a choice question is answered on the page, a task in the terminal. */
export function PracticePage() {
  const { track: trackId = "", question: qid = "" } = useParams();
  const track = useLoad(() => api.track(trackId), [trackId]);
  if (track.error) return <Page docs={null}><LoadError message={track.error} onRetry={() => void track.reload()} /></Page>;
  if (!track.data) return <WorkspaceSkeleton />;
  const choice = track.data.questions?.find((q) => q.id === qid)?.type === "choice";
  return choice ? <QuizPractice track={track.data} qid={qid} /> : <TaskPractice track={track.data} qid={qid} />;
}

/** The workspace before its track is known: the header and the shape of a question. */
export function WorkspaceSkeleton() {
  return (
    <div className="flex h-full flex-col">
      <WorkspaceHeader />
      <main id={MAIN_ID} className="min-h-0 flex-1 overflow-hidden">
        <Loading label={t("Loading the question")} className="mx-auto w-full max-w-[46rem] space-y-4 px-4 py-6 sm:px-6 sm:py-10">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-7 w-3/4" />
          {[0, 1, 2, 3].map((i) => <Skeleton key={i} className={i === 3 ? "h-4 w-1/2" : "h-4"} />)}
          <Skeleton className="h-11 w-44 rounded-lg" />
        </Loading>
      </main>
    </div>
  );
}

function TaskPractice({ track, qid }: { track: Track; qid: string }) {
  const trackId = track.id;
  const nav = useNavigate();
  const config = useConfig();
  const [session, setSession] = useState<Session | null>(null);
  const [view, setView] = useState<QuestionView | null>(null);
  const [result, setResult] = useState<Result | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  const [termReady, setTermReady] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  // A failing check with a retry window keeps the request open for up to a minute.
  const waited = useElapsed(checking);

  const state = session?.questions?.find((x) => x.id === qid);
  const status = state?.status ?? "pending";

  // Open (or reuse) the practice session and prepare this question.
  useEffect(() => {
    let stop = false;
    setView(null); setResult(null); setError(null);
    scroller.current?.scrollTo({ top: 0 });
    (async () => {
      try {
        const s = await api.createSession({ mode: "practice", track: trackId });
        if (stop) return;
        await api.prepare(s.id, qid);
        const fresh = await api.session(s.id);
        if (!stop) setSession(fresh);
      } catch (e) {
        if (!stop) setError((e as Error).message);
      }
    })();
    return () => { stop = true; };
  }, [trackId, qid]);

  const refresh = useCallback(async () => {
    if (!session) return;
    try { setSession(await api.session(session.id)); } catch { /* transient */ }
  }, [session]);
  useInterval(() => void refresh(), 1500, !!session && status !== "ready" && status !== "error");

  // Once the question is ready, load its text with the session's host names.
  useEffect(() => {
    if (!session || status !== "ready" || view?.question.id === qid) return;
    setTermReady(true);
    api.question(session.id, qid).then((v) => { setView(v); setResult(v.state.last ?? null); }).catch((e) => setError(e.message));
  }, [session, status, qid, view]);

  const check = async () => {
    if (!session) return;
    setChecking(true); setError(null);
    try { setResult(await api.check(session.id, qid)); }
    catch (e) { setError((e as Error).message); }
    setChecking(false);
  };
  const reset = async () => {
    if (!session) return;
    setConfirmReset(false); setResult(null); setView(null);
    try { await api.prepare(session.id, qid, true); await refresh(); }
    catch (e) { setError((e as Error).message); }
  };

  const list = (track.questions ?? []).filter(practicable);
  const meta = list.find((q) => q.id === qid);
  const sameDomain = list.filter((q) => q.domain === meta?.domain);
  const pos = sameDomain.findIndex((q) => q.id === qid);
  const prev = sameDomain[pos - 1], next = sameDomain[pos + 1];
  const domain = track.domains.find((d) => d.id === meta?.domain);
  const w = words(track);

  const left = (
    <div ref={scroller} className="h-full overflow-y-auto bg-bg">
      <div className="space-y-5 px-4 py-5 sm:p-6">
        <div>
          <div className="mb-2.5 flex flex-wrap items-center gap-1.5">
            {/* inline-flex: as plain inline content the badge would sit on the text baseline, below its neighbours. */}
            {domain && <Link to={`/t/${trackId}/lesson/${domain.id}`} className="inline-flex max-w-full"><Badge tone="accent">{domain.title}</Badge></Link>}
            {meta && <Difficulty level={meta.difficulty} />}
            {meta && <Badge><Clock className="size-3" aria-hidden />{meta.minutes} min</Badge>}
            {view?.host && <Badge><Server className="size-3" aria-hidden />{view.host}</Badge>}
          </div>
          <h1 className="text-title font-semibold tracking-tight">{meta?.title ?? " "}</h1>
        </div>

        {error && <ErrorNote>{error}</ErrorNote>}
        {status === "error" && (
          <ErrorNote>
            <div className="mb-2 font-medium">{w.prepareFailed}</div>
            <div className="mb-3 font-mono text-xs break-words">{state?.error}</div>
            <Button size="sm" onClick={reset}>{t("Try again")}</Button>
          </ErrorNote>
        )}
        {!view && status !== "error" && !error && (
          <div className="flex items-center gap-3 rounded-xl border border-dashed px-4 py-6 text-sm text-muted">
            <Spinner />{w.preparing}
          </div>
        )}

        {view && (
          <>
            <Markdown>{view.task}</Markdown>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="primary" loading={checking} icon={<PlayCircle className="size-4" aria-hidden />} onClick={check}>{t("Check my solution")}</Button>
              <Button icon={<RotateCcw className="size-4" aria-hidden />} onClick={() => setConfirmReset(true)}>{w.reset}</Button>
              {/* Always present, so that a screen reader announces the note when it appears. The counter is not announced. */}
              <p role="status" aria-live="polite" className={cx("basis-full text-xs leading-relaxed text-muted", !(checking && waited >= SLOW_CHECK_SECONDS) && "hidden")}>
                {checking && waited >= SLOW_CHECK_SECONDS && (
                  <>{w.stillChecking} <span aria-hidden className="tabular-nums">{t("{n} s", { n: waited })}</span></>
                )}
              </p>
            </div>
            {result && <Results result={result} />}
            {session && meta && (
              <Help sid={session.id} q={meta} result={result} tutor={!!config?.tutor} track={track} initialHints={view.state.hintsUsed} />
            )}
            {result && <Rating track={trackId} question={qid} />}
            {result?.passed && next && (
              <Button variant="ok" size="lg" className="w-full" onClick={() => nav(`/t/${trackId}/q/${next.id}`)}>
                {w.next}<ChevronRight className="size-4" aria-hidden />
              </Button>
            )}
          </>
        )}
      </div>
    </div>
  );

  const right = session && termReady
    ? <TerminalTabs sessionId={session.id} />
    : <Provisioning session={session} title={t("Creating your sandbox")} note={w.provisioningNote} />;

  return (
    <div className="flex h-full flex-col">
      <WorkspaceHeader
        back={(
          <Link to={`/t/${trackId}`} className="inline-flex min-h-9 min-w-0 items-center gap-1.5 text-sm text-muted hover:text-text">
            <ArrowLeft className="size-4 shrink-0" aria-hidden /><span className="truncate">{track.short} · {t("Practice")}</span>
          </Link>
        )}
        nav={(
          <HeaderPager position={pos >= 0 ? `${pos + 1} / ${sameDomain.length}` : ""} previousLabel={w.previous} nextLabel={w.next}
            onPrevious={prev && (() => nav(`/t/${trackId}/q/${prev.id}`))} onNext={next && (() => nav(`/t/${trackId}/q/${next.id}`))} />
        )}
        docs={w.docs}
      />
      <main id={MAIN_ID} className="flex min-h-0 flex-1 flex-col">
        <Split left={left} right={right} labels={{ left: w.item, right: t("Terminal") }} />
      </main>
      {confirmReset && (
        <Modal title={w.resetTitle} onClose={() => setConfirmReset(false)}
          footer={<><Button onClick={() => setConfirmReset(false)}>{t("Cancel")}</Button><Button variant="danger" onClick={reset}>{t("Reset")}</Button></>}>
          {w.resetNote}
        </Modal>
      )}
    </div>
  );
}
