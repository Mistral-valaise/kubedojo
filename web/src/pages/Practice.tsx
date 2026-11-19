import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowLeft, ChevronLeft, ChevronRight, Clock, PlayCircle, RotateCcw, Server } from "lucide-react";
import { api, type Config, type QuestionView, type Result, type Session, type Track } from "../api";
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
import { Badge, Brand, Button, Difficulty, ErrorNote, Modal, Page, Spinner, ThemeToggle, cx } from "../components/ui";
import { QuizPractice } from "./QuizPractice";

/** After this many seconds a running check gets a note that says why it takes long. */
const SLOW_CHECK_SECONDS = 3;

/** Practice of one question: a choice question is answered on the page, a task in the terminal. */
export function PracticePage() {
  const { track: trackId = "", question: qid = "" } = useParams();
  const track = useLoad(() => api.track(trackId), [trackId]);
  if (track.error) return <Page docs={null}><ErrorNote>{track.error}</ErrorNote></Page>;
  if (!track.data) return <Page docs={null}><Spinner /></Page>;
  const choice = track.data.questions?.find((q) => q.id === qid)?.type === "choice";
  return choice ? <QuizPractice track={track.data} qid={qid} /> : <TaskPractice track={track.data} qid={qid} />;
}

function TaskPractice({ track, qid }: { track: Track; qid: string }) {
  const trackId = track.id;
  const nav = useNavigate();
  const [config, setConfig] = useState<Config | null>(null);
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

  useEffect(() => {
    api.config().then(setConfig).catch(() => undefined);
  }, []);

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
    <div ref={scroller} className="h-full overflow-y-auto bg-panel">
      <div className="space-y-5 px-4 py-5 sm:px-6">
        <div>
          <div className="mb-2 flex flex-wrap items-center gap-1.5">
            {/* inline-flex: as plain inline content the badge would sit on the text baseline, below its neighbours. */}
            {domain && <Link to={`/t/${trackId}/lesson/${domain.id}`} className="inline-flex max-w-full"><Badge tone="accent">{domain.title}</Badge></Link>}
            {meta && <Difficulty level={meta.difficulty} />}
            {meta && <Badge><Clock className="size-3" />{meta.minutes} min</Badge>}
            {view?.host && <Badge><Server className="size-3" />{view.host}</Badge>}
          </div>
          <h1 className="text-xl font-semibold tracking-tight">{meta?.title ?? " "}</h1>
        </div>

        {error && <ErrorNote>{error}</ErrorNote>}
        {status === "error" && (
          <ErrorNote>
            <div className="mb-2 font-medium">{w.prepareFailed}</div>
            <div className="mb-3 font-mono text-xs break-words">{state?.error}</div>
            <Button size="sm" onClick={reset}>{t("Try again")}</Button>
          </ErrorNote>
        )}
        {!view && status !== "error" && (
          <div className="flex items-center gap-3 rounded-xl border border-dashed px-4 py-6 text-sm text-muted">
            <Spinner />{w.preparing}
          </div>
        )}

        {view && (
          <>
            <Markdown>{view.task}</Markdown>
            <div className="flex flex-wrap items-center gap-2 border-t pt-4">
              <Button variant="primary" loading={checking} icon={<PlayCircle className="size-4" />} onClick={check}>{t("Check my solution")}</Button>
              <Button variant="ghost" icon={<RotateCcw className="size-4" />} onClick={() => setConfirmReset(true)}>{w.reset}</Button>
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
              <Button variant="ok" className="w-full" onClick={() => nav(`/t/${trackId}/q/${next.id}`)}>
                {w.next}<ChevronRight className="size-4" />
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
      <header className="flex h-12 shrink-0 items-center gap-3 border-b bg-bg px-4">
        {/* On a phone the header has room for the way back and the navigation only. */}
        <span className="hidden items-center gap-3 sm:flex"><Brand /><span className="text-line">/</span></span>
        <Link to={`/t/${trackId}`} className="inline-flex min-w-0 items-center gap-1.5 text-sm text-muted hover:text-text">
          <ArrowLeft className="size-3.5 shrink-0" /><span className="truncate">{track.short} · {t("Practice")}</span>
        </Link>
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <Button size="sm" variant="ghost" disabled={!prev} onClick={() => prev && nav(`/t/${trackId}/q/${prev.id}`)} aria-label={w.previous}><ChevronLeft className="size-4" /></Button>
          <span className="px-1 text-xs whitespace-nowrap tabular-nums text-muted">{pos >= 0 ? `${pos + 1} / ${sameDomain.length}` : ""}</span>
          <Button size="sm" variant="ghost" disabled={!next} onClick={() => next && nav(`/t/${trackId}/q/${next.id}`)} aria-label={w.next}><ChevronRight className="size-4" /></Button>
          {w.docs && <a href={w.docs.url} target="_blank" rel="noreferrer" title={w.docs.title} className="hidden px-2 text-sm text-muted hover:text-text sm:block">{t("Docs")}</a>}
          <ThemeToggle />
        </div>
      </header>
      <Split left={left} right={right} labels={{ left: w.item, right: t("Terminal") }} />
      {confirmReset && (
        <Modal title={w.resetTitle} onClose={() => setConfirmReset(false)}
          footer={<><Button onClick={() => setConfirmReset(false)}>{t("Cancel")}</Button><Button variant="danger" onClick={reset}>{t("Reset")}</Button></>}>
          {w.resetNote}
        </Modal>
      )}
    </div>
  );
}
