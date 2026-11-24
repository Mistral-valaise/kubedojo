import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  AlertTriangle, ArrowLeft, Check, CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, Flag, Flame, Server, Sparkles,
  SquareTerminal, Trash2, XCircle,
} from "lucide-react";
import { api, type QuestionView, type Session, type Track } from "../api";
import { AssistantPanel } from "../assistant/AssistantPanel";
import { type Ask, ExplainButton, newAsk, useAssistantDock } from "../assistant/dock";
import { fmtDuration, useInterval, useMediaQuery } from "../hooks";
import { plural, t } from "../i18n";
import { isFullAssessment, words } from "../track";
import { Choice } from "../components/Choice";
import { Help } from "../components/Help";
import { Markdown } from "../components/Markdown";
import { Provisioning } from "../components/Provisioning";
import { Rating } from "../components/Rating";
import { Results } from "../components/Results";
import { Split } from "../components/Split";
import { Terminal } from "../components/Terminal";
import { TerminalTabs } from "../components/TerminalTabs";
import { WorkspaceHeader } from "../components/WorkspaceHeader";
import { Badge, Bar, Button, ButtonLink, Card, ErrorNote, Loading, MAIN_ID, Modal, Page, Ring, SectionLabel, Skeleton, cx } from "../components/ui";
import { WorkspaceSkeleton } from "./Practice";
import { LoadError } from "./Track";

export function ExamPage() {
  const { session: sid = "" } = useParams();
  const [session, setSession] = useState<Session | null>(null);
  const [track, setTrack] = useState<Track | null>(null);
  const [trackError, setTrackError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try { setSession(await api.session(sid)); setError(null); }
    catch (e) { setError((e as Error).message); }
  }, [sid]);
  useEffect(() => { void refresh(); }, [refresh]);
  const trackId = session?.track;
  const loadTrack = useCallback(() => {
    if (!trackId) return;
    setTrackError(null);
    api.track(trackId).then(setTrack, (e) => setTrackError((e as Error).message));
  }, [trackId]);
  useEffect(() => { if (!track) loadTrack(); }, [track, loadTrack]);
  const fast = session?.status === "provisioning" || session?.status === "grading";
  useInterval(() => void refresh(), fast ? 1500 : 10000, !!session && session.status !== "finished" && session.status !== "error");

  if (error && !session) return <Page><ErrorNote>{t("This session no longer exists.")} <Link className="underline underline-offset-2" to="/">{t("Back to start")}</Link></ErrorNote></Page>;
  // Without its track the session cannot be shown: say so instead of waiting forever.
  if (session && !track && trackError) return <Page docs={null}><LoadError message={trackError} onRetry={loadTrack} /></Page>;
  if (!session || !track) return <WorkspaceSkeleton />;
  const w = words(track);
  // Without a sandbox nothing is provisioned and grading is immediate, so
  // these states pass in an instant and have no log to show.
  if (!w.sandbox && (session.status === "provisioning" || session.status === "grading")) return <WorkspaceSkeleton />;

  switch (session.status) {
    case "provisioning":
      return <Stage><Provisioning session={session} title={w.preparingTitle} note={w.assessmentProvisioningNote} /></Stage>;
    case "error":
      return <Page docs={w.docs} crumbs={[{ label: track.short, to: `/t/${track.id}` }, { label: w.assessment }]}>
        <ErrorNote>
          <div className="mb-1 font-semibold">{w.environmentFailed}</div>
          <div className="mb-3 font-mono text-xs break-words">{session.error}</div>
          <EndButton sid={session.id} to={`/t/${track.id}`} label={w.leaveAfterFailure} />
        </ErrorNote>
      </Page>;
    case "ready":
      return <Briefing session={session} track={track} onStart={async () => setSession(await api.start(session.id))} />;
    case "grading":
      return <Stage><Provisioning session={session} title={w.gradingTitle} note={w.gradingNote} /></Stage>;
    case "finished":
      return <ReportView session={session} track={track} />;
    default:
      return <ExamWorkspace session={session} track={track} onSession={setSession} refresh={refresh} />;
  }
}

function Stage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col">
      <WorkspaceHeader />
      <main id={MAIN_ID} className="min-h-0 flex-1">{children}</main>
    </div>
  );
}

function EndButton({ sid, to, label }: { sid: string; to: string; label: string }) {
  const nav = useNavigate();
  const [busy, setBusy] = useState(false);
  return (
    <Button size="sm" loading={busy} icon={<Trash2 className="size-3.5" aria-hidden />}
      onClick={async () => { setBusy(true); try { await api.destroy(sid); } catch { /* already gone */ } nav(to); }}>
      {label}
    </Button>
  );
}

function Briefing({ session, track, onStart }: { session: Session; track: Track; onStart: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const n = session.questions?.length ?? 0;
  const hosts = Object.values(session.envs ?? {});
  const w = words(track);
  const start = async () => {
    setBusy(true); setError(null);
    try { await onStart(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };
  return (
    <Page docs={w.assessmentDocs} crumbs={[{ label: track.short, to: `/t/${track.id}` }, { label: w.assessment }]}>
      <Card className="mx-auto max-w-2xl p-5 sm:p-8">
        <Badge tone="ok"><CheckCircle2 className="size-3" aria-hidden />{w.environmentReady}</Badge>
        <h1 className="mt-3 text-2xl font-bold tracking-tight">{track.title}</h1>
        <p className="mt-1 text-muted">{w.briefingSummary(n, session.durationMinutes, track.exam.passPercent)}</p>
        <ol className="mt-6 space-y-3 text-sm leading-relaxed">
          {w.briefing.map((x, i) => (
            <li key={i} className="flex gap-3"><span aria-hidden className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-xs font-bold text-accent">{i + 1}</span><span className="text-muted">{x}</span></li>
          ))}
        </ol>
        {hosts.length > 0 && (
          <div className="mt-6 flex flex-wrap items-center gap-2 rounded-xl bg-panel2 px-4 py-3 text-sm">
            <Server className="size-4 text-muted" aria-hidden /><span className="text-muted">{t("Your hosts")}</span>
            {hosts.map((h) => <code key={h} className="rounded-md border bg-panel px-1.5 py-0.5 font-mono text-xs">{h}</code>)}
          </div>
        )}
        {error && <ErrorNote className="mt-5">{t("The timer could not be started.")} <span className="font-mono text-xs break-words">{error}</span></ErrorNote>}
        <div className="mt-7 flex flex-wrap items-center gap-3">
          <Button variant="primary" size="lg" loading={busy} icon={<Flame className="size-4" aria-hidden />} onClick={start}>
            {t("Start the timer")}
          </Button>
          <EndButton sid={session.id} to={`/t/${track.id}`} label={t("Cancel")} />
        </div>
      </Card>
    </Page>
  );
}

/** What of a question's private data did not reach the server. */
type Unsaved = { note?: boolean; flag?: boolean };

function ExamWorkspace({ session, track, onSession, refresh }: {
  session: Session; track: Track; onSession: (s: Session) => void; refresh: () => Promise<void>;
}) {
  const qs = session.questions ?? [];
  const [idx, setIdx] = useState(() => {
    try { return Math.min(Number(sessionStorage.getItem(`kubedojo.exam.${session.id}`)) || 0, qs.length - 1); } catch { return 0; }
  });
  const [view, setView] = useState<QuestionView | null>(null);
  const [viewError, setViewError] = useState<string | null>(null);
  // Counts the tries to load the question, so that a failed load can be repeated.
  const [attempt, setAttempt] = useState(0);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notes, setNotes] = useState<Record<string, string>>(() => Object.fromEntries(qs.map((q) => [q.id, q.note])));
  const [flags, setFlags] = useState<Record<string, boolean>>(() => Object.fromEntries(qs.map((q) => [q.id, q.flagged])));
  const cur = qs[idx];
  const w = words(track);
  // A track without a sandbox is answered on the page: no terminal, and the
  // navigator and the finish dialog know which questions have an answer.
  const quiz = !w.sandbox;
  const scroller = useRef<HTMLDivElement>(null);

  // Answers are shown at once and saved one request per change, in order.
  const [answers, setAnswers] = useState<Record<string, string[]>>(() => Object.fromEntries(qs.map((q) => [q.id, q.answer ?? []])));
  const saved = useRef(answers); // what the server confirmed
  const latest = useRef<Record<string, number>>({}); // number of the newest change per question
  const queue = useRef<Promise<void>>(Promise.resolve());
  const [saving, setSaving] = useState(0);
  const [saveError, setSaveError] = useState<{ id: string; message: string } | null>(null);
  const setAnswer = (ids: string[]) => {
    const id = cur.id, seq = (latest.current[id] ?? 0) + 1;
    latest.current[id] = seq;
    setAnswers((a) => ({ ...a, [id]: ids }));
    setSaveError(null);
    setSaving((n) => n + 1);
    queue.current = queue.current.then(async () => {
      try {
        await api.patchQuestion(session.id, id, { answer: ids });
        saved.current = { ...saved.current, [id]: ids };
      } catch (e) {
        // A newer change of the same question is still on its way and decides
        // what is shown; only the newest failure falls back to the saved answer.
        if (latest.current[id] === seq) {
          setAnswers((a) => ({ ...a, [id]: saved.current[id] ?? [] }));
          setSaveError({ id, message: (e as Error).message });
        }
      } finally {
        setSaving((n) => n - 1);
      }
    });
  };

  // Local countdown, anchored to the server clock.
  const skew = useMemo(() => Date.parse(session.now) - Date.now(), [session.now]);
  const [left, setLeft] = useState(session.remainingSeconds ?? 0);
  useInterval(() => {
    if (!session.deadline) return;
    const rem = Math.max(0, Math.round((Date.parse(session.deadline) - (Date.now() + skew)) / 1000));
    setLeft(rem);
    if (rem === 0) void refresh();
  }, 1000);

  useEffect(() => {
    let stop = false;
    setView(null); setViewError(null);
    scroller.current?.scrollTo({ top: 0 });
    try { sessionStorage.setItem(`kubedojo.exam.${session.id}`, String(idx)); } catch { /* ignore */ }
    api.question(session.id, cur.id).then(
      (v) => { if (!stop) setView(v); },
      (e) => { if (!stop) setViewError((e as Error).message); },
    );
    return () => { stop = true; };
  }, [session.id, cur.id, idx, attempt]);

  // Notes and flags are private to the learner and saved in the background.
  // What the screen shows stays as typed; a save that failed is named, with a
  // way to send it again, because an exam note that is silently lost is gone.
  const [unsaved, setUnsaved] = useState<Record<string, Unsaved>>({});
  const mark = (id: string, what: keyof Unsaved, failed: boolean) =>
    setUnsaved((u) => ({ ...u, [id]: { ...u[id], [what]: failed } }));
  const saveNote = (id: string, text: string) =>
    api.patchQuestion(session.id, id, { note: text }).then(() => mark(id, "note", false), () => mark(id, "note", true));
  const saveFlag = (id: string, flagged: boolean) =>
    api.patchQuestion(session.id, id, { flagged }).then(() => mark(id, "flag", false), () => mark(id, "flag", true));
  const saveTimer = useRef<number>(0);
  const setNote = (text: string) => {
    setNotes((n) => ({ ...n, [cur.id]: text }));
    window.clearTimeout(saveTimer.current);
    const id = cur.id;
    saveTimer.current = window.setTimeout(() => void saveNote(id, text), 600);
  };
  const toggleFlag = () => {
    const v = !flags[cur.id];
    setFlags((f) => ({ ...f, [cur.id]: v }));
    void saveFlag(cur.id, v);
  };
  const failed = unsaved[cur.id] ?? {};
  const retrySave = () => {
    if (failed.note) void saveNote(cur.id, notes[cur.id] ?? "");
    if (failed.flag) void saveFlag(cur.id, !!flags[cur.id]);
  };
  const finish = async () => {
    setBusy(true);
    await queue.current; // an answer still being saved must count
    try { onSession(await api.finish(session.id)); } catch { await refresh(); }
  };

  const low = left < 15 * 60, critical = left < 5 * 60;
  const flagged = Object.values(flags).filter(Boolean).length;
  const meta = session.meta[cur.id];
  const domain = track.domains.find((d) => d.id === meta?.domain);
  const answered = (id: string) => (answers[id]?.length ?? 0) > 0;
  const unanswered = qs.filter((q) => !answered(q.id)).length;
  // The content of a quiz is one readable column in the middle of the page.
  const column = quiz ? "mx-auto w-full max-w-[46rem] px-4 sm:px-6" : "";
  const bar = quiz ? "" : "px-4"; // the column brings its own gutter

  const leftPane = (
    <div className="flex h-full flex-col bg-bg">
      <div className={cx("shrink-0 border-b bg-panel py-2.5", bar)}>
        <div className={cx("flex flex-wrap items-center gap-x-4 gap-y-2", column)}>
          <nav className="flex flex-wrap gap-1.5" aria-label={t("Questions")}>
            {qs.map((q, i) => (
              <button key={q.id} onClick={() => setIdx(i)} aria-current={i === idx}
                // The state is in the name, in the border and in the flag mark: never in colour alone.
                // Only a quiz knows whether a question is answered: a task is solved in the terminal.
                aria-label={[t("{item} {n}", { item: w.item, n: i + 1 }), quiz && (answered(q.id) ? t("answered") : t("not answered")), flags[q.id] && t("flagged")].filter(Boolean).join(", ")}
                className={cx("relative flex size-9 items-center justify-center rounded-lg border text-label font-medium tabular-nums transition-colors",
                  i === idx ? "border-accent bg-accent font-semibold text-on-accent"
                    : !quiz ? "bg-panel2 text-muted hover:border-accent/60 hover:text-text"
                      : answered(q.id) ? "border-accent/50 bg-accent-soft font-semibold text-accent hover:border-accent"
                        : "border-dashed border-muted text-muted hover:border-accent/60 hover:text-text")}>
                {i + 1}
                {flags[q.id] && (
                  <span aria-hidden className="absolute -top-1.5 -right-1.5 flex size-4 items-center justify-center rounded-full border border-panel bg-warn-soft text-warn">
                    <Flag className="size-2.5 fill-current" />
                  </span>
                )}
              </button>
            ))}
          </nav>
          {quiz && <span className="ml-auto text-label tabular-nums text-muted">{t("{a} of {n} answered", { a: qs.length - unanswered, n: qs.length })}</span>}
        </div>
      </div>
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto">
        <div className={cx("space-y-5 py-5", quiz ? "sm:py-8" : "px-4 sm:p-6", column)}>
          <div>
            <div className="mb-2.5 flex flex-wrap items-center gap-1.5">
              <Badge tone="accent">{w.position(idx + 1, qs.length)}</Badge>
              {domain && <Badge>{domain.title}</Badge>}
              {/* Every question of a quiz counts the same, so the points say nothing. */}
              {meta && !quiz && <Badge>{plural(meta.points, "{n} point", "{n} points")}</Badge>}
              {meta?.host && <Badge><Server className="size-3" aria-hidden />{meta.host}</Badge>}
            </div>
            <h1 className="text-title font-semibold tracking-tight">{meta?.title}</h1>
          </div>
          {saveError && (
            <div role="alert">
              <ErrorNote>
                <span className="font-medium">{t("The answer to question {n} was not saved.", { n: qs.findIndex((q) => q.id === saveError.id) + 1 })}</span>{" "}
                {t("It shows the last saved answer again. Select your answer once more.")}
                <div className="mt-1 font-mono text-xs break-words">{saveError.message}</div>
              </ErrorNote>
            </div>
          )}
          {view?.question.id === cur.id ? <Markdown className={quiz ? "md-lg" : undefined}>{view.task}</Markdown>
            : viewError ? (
              <div role="alert">
                <ErrorNote onRetry={() => setAttempt((n) => n + 1)}>
                  <span className="font-medium">{t("This question could not be loaded.")}</span>{" "}
                  {t("The timer keeps running. Try again, or move on and come back.")}
                  <div className="mt-1 font-mono text-xs break-words">{viewError}</div>
                </ErrorNote>
              </div>
            ) : (
              <Loading label={t("Loading the question")} className="space-y-3">
                {[0, 1, 2, 3].map((i) => <Skeleton key={i} className={i === 3 ? "h-4 w-1/2" : "h-4"} />)}
              </Loading>
            )}
          {view?.choice && view.question.id === cur.id && (
            <div>
              <Choice choice={view.choice} value={answers[cur.id] ?? []} onChange={setAnswer} hotkeys={!confirm} label={meta?.title} />
              <div className="mt-2 flex min-h-9 items-center gap-2 text-label text-muted" aria-live="polite">
                {saving > 0 ? <span>{t("Saving ...")}</span>
                  : answered(cur.id) ? <span className="inline-flex items-center gap-1"><Check aria-hidden className="size-3.5 text-ok" />{t("Answer saved")}</span>
                    : <span>{t("Not answered yet")}</span>}
                {answered(cur.id) && <Button size="sm" variant="ghost" className="ml-auto" onClick={() => setAnswer([])}>{t("Clear answer")}</Button>}
              </div>
            </div>
          )}
          <div>
            <SectionLabel as="label" htmlFor="note" className="mb-1.5">{t("Notes")}</SectionLabel>
            <textarea id="note" value={notes[cur.id] ?? ""} onChange={(e) => setNote(e.target.value)} rows={3}
              placeholder={w.notePlaceholder}
              className="w-full resize-y rounded-xl border bg-panel px-3 py-2 font-mono text-label placeholder:font-sans placeholder:text-muted focus-visible:border-accent" />
            {(failed.note || failed.flag) && (
              <p role="alert" className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-label text-bad">
                <AlertTriangle className="size-3.5 shrink-0" aria-hidden />
                <span>{failed.note && failed.flag ? t("Could not save your note and the flag.") : failed.note ? t("Could not save your note.") : t("Could not save the flag.")}</span>
                <button type="button" onClick={retrySave} className="inline-flex min-h-9 items-center font-semibold underline underline-offset-2">{t("Retry")}</button>
              </p>
            )}
          </div>
        </div>
      </div>
      <div className={cx("shrink-0 border-t bg-panel py-2.5", bar)}>
        <div className={cx("flex items-center gap-2", column)}>
          <Button size="sm" disabled={idx === 0} onClick={() => setIdx(idx - 1)} icon={<ChevronLeft className="size-4" aria-hidden />}>{t("Previous")}</Button>
          <Button size="sm" variant={flags[cur.id] ? "secondary" : "ghost"} onClick={toggleFlag} aria-pressed={!!flags[cur.id]}
            icon={<Flag className={cx("size-3.5", flags[cur.id] && "fill-warn text-warn")} aria-hidden />}>
            {flags[cur.id] ? t("Flagged") : t("Flag for review")}
          </Button>
          <Button size="sm" className="ml-auto" disabled={idx === qs.length - 1} onClick={() => setIdx(idx + 1)}>{t("Next")}<ChevronRight className="size-4" aria-hidden /></Button>
        </div>
      </div>
    </div>
  );

  return (
    <div className="flex h-full flex-col">
      {/* On a phone the header has room for the timer and the button only: the About button waits for a wider screen. */}
      <WorkspaceHeader
        title={<span className="min-w-0 truncate text-sm text-muted">{track.short}<span className="hidden sm:inline"> · {w.assessment}</span></span>}
        center={(
          <div className={cx("ml-auto flex h-9 shrink-0 items-center gap-2 rounded-lg border px-3 font-mono text-sm font-semibold tabular-nums",
            critical ? "border-bad/50 bg-bad-soft text-bad" : low ? "border-warn/50 bg-warn-soft text-warn" : "bg-panel2")}
            role="timer" aria-label={t("Time remaining")}>
            {critical && <AlertTriangle className="size-3.5" aria-hidden />}{fmtDuration(left)}
          </div>
        )}
        docs={w.assessmentDocs}
        about="sm"
        end={(
          // Next to a terminal the phone header is full: the button keeps its name and shows a short one.
          <Button size="sm" variant="primary" onClick={() => setConfirm(true)} aria-label={w.finishAssessment}>
            {quiz ? w.finishAssessment : <><span className="sm:hidden">{t("Finish")}</span><span className="hidden sm:inline">{w.finishAssessment}</span></>}
          </Button>
        )}
      />
      <main id={MAIN_ID} className="flex min-h-0 flex-1 flex-col">
        {quiz
          ? <div className="min-h-0 flex-1">{leftPane}</div>
          : <Split left={leftPane} right={<TerminalTabs sessionId={session.id} />} storageKey="kubedojo.split.exam" labels={{ left: w.item, right: t("Terminal") }} />}
      </main>
      {confirm && (
        <Modal title={w.finishTitle} onClose={() => !busy && setConfirm(false)}
          footer={<><Button disabled={busy} onClick={() => setConfirm(false)}>{t("Keep working")}</Button><Button variant="primary" loading={busy} onClick={finish}>{t("Finish and grade")}</Button></>}>
          <p>{t("You still have {time} left. After finishing you cannot change your answers.", { time: fmtDuration(left) })}</p>
          {quiz && (unanswered > 0
            ? <p className="mt-2 font-medium text-warn">{plural(unanswered, "{n} of {m} questions is unanswered. An unanswered question scores no points.", "{n} of {m} questions are unanswered. An unanswered question scores no points.", { m: qs.length })}</p>
            : <p className="mt-2">{t("Every question is answered.")}</p>)}
          {flagged > 0 && <p className="mt-2 text-warn">{w.flagged(flagged)}</p>}
        </Modal>
      )}
    </div>
  );
}

/** From this width of the window the assistant of a report is a column beside it; below, a sheet over it. */
const DOCK_FROM = "(min-width: 1100px)";

/** What a question of the report needs to hand itself to the assistant. */
interface ReportAssistant {
  /** The assistant can be shown, and it can answer: see Dock. */
  available: boolean; usable: boolean;
  /** The question the open assistant is about. */
  qid: string | null;
  /** Opens the assistant for a question, with a message to send if there is one. */
  open: (qid: string, ask?: Ask) => void;
}

function ReportView({ session, track }: { session: Session; track: Track }) {
  const r = session.report!;
  const [term, setTerm] = useState(false);
  // The exam is over, so the assistant is there again: one question at a time.
  const dock = useAssistantDock();
  const [about, setAbout] = useState<{ qid: string; ask: Ask | null } | null>(null);
  const docked = useMediaQuery(DOCK_FROM);
  const assistant: ReportAssistant = {
    available: dock.available, usable: dock.usable, qid: about?.qid ?? null,
    open: (qid, ask) => setAbout((now) => ({ qid, ask: ask ?? (now?.qid === qid ? now.ask : null) })),
  };
  const aboutResult = about ? session.questions?.find((q) => q.id === about.qid)?.last : undefined;
  const panel = about && dock.available && (
    <AssistantPanel sid={session.id} qid={about.qid} track={track} ask={about.ask} subject={session.meta[about.qid]?.title}
      situation={track.questions?.find((q) => q.id === about.qid)?.type === "choice" ? "reviewed" : !aboutResult ? "none" : aboutResult.passed ? "passed" : "failed"}
      onClose={() => setAbout(null)} />
  );
  const qs = session.questions ?? [];
  const solved = qs.filter((q) => q.last?.passed).length;
  const w = words(track);
  // A quick round is graded like an assessment and proves less: it says so and does not count on the path.
  const quick = !isFullAssessment(track, qs.length);
  const short = Math.max(0.1, Math.round((r.passPercent - r.percent) * 10) / 10);
  const headline = quick ? (r.passed ? t("You passed this quick round.") : t("Quick round not passed."))
    : r.passed ? w.passedHeadline
      : plural(short, "{n} percentage point short of the pass mark.", "{n} percentage points short of the pass mark.");
  return (
    <div className="flex h-full flex-col">
      <div className="flex min-h-0 flex-1">
        {/* A container: the columns of the report follow its width, which a docked assistant makes narrower than the window. */}
        <div className="@container min-h-0 min-w-0 flex-1 overflow-y-auto">
          <Page docs={w.docs} crumbs={[{ label: track.short, to: `/t/${track.id}` }, { label: w.assessmentReport }]}>
            <Card className={cx("mb-6 flex flex-wrap items-center gap-x-7 gap-y-5 p-5 sm:p-7", r.passed ? "border-ok/40" : "border-bad/40")}>
              <Ring value={r.percent} size={140} stroke={12} tone={r.passed ? "ok" : "bad"}>
                <span className="text-3xl font-bold tabular-nums">{r.percent}%</span>
                <span className="text-xs text-muted">{t("{a} / {b} pts", { a: r.score, b: r.max })}</span>
              </Ring>
              {/* The basis lets the text wrap below the ring on a narrow screen. */}
              <div className="min-w-0 flex-1 basis-64">
                <div className={cx("mb-1 inline-flex items-center gap-1.5 text-sm font-semibold", r.passed ? "text-ok" : "text-bad")}>
                  {r.passed ? <CheckCircle2 className="size-4" aria-hidden /> : <XCircle className="size-4" aria-hidden />}
                  {r.passed ? t("Passed") : t("Not passed")}
                </div>
                <h1 className="text-2xl font-bold tracking-tight">{headline}</h1>
                <p className="mt-1.5 text-muted">
                  {w.reportSummary(r.passPercent, solved, qs.length, fmtDuration(r.usedSeconds))}
                </p>
                {quick && <p className="mt-1.5 text-sm text-muted">{w.quickRoundNote}</p>}
                {/* With a sandbox the way back is its "Remove sandbox" button. */}
                {!w.sandbox && (
                  <ButtonLink to={`/t/${track.id}`} className="mt-4" icon={<ArrowLeft className="size-4" aria-hidden />}>{t("Back to {track}", { track: track.short })}</ButtonLink>
                )}
              </div>
            </Card>

            <div className="grid grid-cols-[minmax(0,1fr)] gap-6 @5xl:grid-cols-[320px_minmax(0,1fr)]">
              <div className="space-y-6">
                <Card padded>
                  <SectionLabel as="h2" className="mb-4">{t("Score by domain")}</SectionLabel>
                  <ul className="space-y-4">
                    {r.domains.map((d) => {
                      const pct = d.max ? Math.round((d.score / d.max) * 100) : 0;
                      return (
                        <li key={d.id}>
                          <div className="mb-1.5 flex items-baseline justify-between gap-3 text-sm">
                            {/* Long titles wrap: cut off, two domains can read the same. */}
                            <Link to={`/t/${track.id}/lesson/${d.id}`} className="min-w-0 break-words hover:text-accent hover:underline">{d.title}</Link>
                            <span className="shrink-0 tabular-nums text-muted">{d.score}/{d.max}</span>
                          </div>
                          <Bar value={pct} tone={pct >= r.passPercent ? "ok" : pct >= 40 ? "warn" : "bad"} />
                        </li>
                      );
                    })}
                  </ul>
                </Card>
                {w.sandbox && (
                  <Card padded className="space-y-3 text-sm">
                    <SectionLabel as="h2">{t("Sandbox")}</SectionLabel>
                    <p className="leading-relaxed text-muted">{w.reportSandbox}</p>
                    <div className="flex flex-wrap gap-2">
                      <Button size="sm" aria-expanded={term} icon={<SquareTerminal className="size-3.5" aria-hidden />} onClick={() => setTerm(!term)}>{term ? t("Hide terminal") : t("Open terminal")}</Button>
                      <EndButton sid={session.id} to={`/t/${track.id}`} label={t("Remove sandbox")} />
                    </div>
                  </Card>
                )}
              </div>
              <div>
                <SectionLabel as="h2" className="mb-3">{w.reportList}</SectionLabel>
                <div className="space-y-3">
                  {qs.map((q, i) => <ReportQuestion key={q.id} n={i + 1} session={session} track={track} qid={q.id} assistant={assistant} />)}
                </div>
              </div>
            </div>
          </Page>
        </div>
        {panel && docked && <aside className="w-[clamp(20rem,30vw,26rem)] shrink-0 border-l">{panel}</aside>}
      </div>
      {panel && !docked && <Modal sheet title={t("Assistant")} onClose={() => setAbout(null)}>{panel}</Modal>}
      {/* A share of the window, within limits: usable on a phone in landscape, and not half of a tall screen. */}
      {term && <div className="h-[clamp(12rem,40dvh,28rem)] shrink-0 border-t border-term-line"><Terminal sessionId={session.id} /></div>}
    </div>
  );
}

function ReportQuestion({ n, session, track, qid, assistant }: { n: number; session: Session; track: Track; qid: string; assistant: ReportAssistant }) {
  const state = session.questions!.find((q) => q.id === qid)!;
  const meta = session.meta[qid];
  const res = state.last;
  const [open, setOpen] = useState(false);
  const [view, setView] = useState<QuestionView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(() => {
    setError(null);
    api.question(session.id, qid).then(setView, (e) => setError((e as Error).message));
  }, [session.id, qid]);
  useEffect(() => { if (open && !view) load(); }, [open, view, load]);
  const full = track.questions?.find((q) => q.id === qid);
  const choice = full?.type === "choice";
  const tone = !res ? "muted" : res.passed ? "ok" : res.score > 0 ? "warn" : "bad";
  const outcome = !res ? t("not graded") : res.passed ? t("solved") : res.score > 0 ? t("partly solved") : t("not solved");
  return (
    <Card className="overflow-hidden">
      <button onClick={() => setOpen(!open)} aria-expanded={open} className="flex min-h-14 w-full items-center gap-3 px-5 py-3 text-left hover:bg-panel2">
        <span className={cx("flex size-7 shrink-0 items-center justify-center rounded-lg text-xs font-bold tabular-nums",
          tone === "ok" ? "bg-ok-soft text-ok" : tone === "warn" ? "bg-warn-soft text-warn" : tone === "bad" ? "bg-bad-soft text-bad" : "bg-panel2 text-muted")}>{n}</span>
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium sm:truncate">{meta?.title}</span>
          <span className="block text-label text-muted sm:truncate">{track.domains.find((d) => d.id === meta?.domain)?.title}</span>
        </span>
        {state.flagged && <Flag role="img" aria-label={t("flagged")} className="size-3.5 shrink-0 fill-warn text-warn" />}
        {/* The number says how it went; its colour only repeats that. */}
        <span className="shrink-0 text-sm tabular-nums text-muted"><span className="sr-only">{outcome}, </span>{res?.score ?? 0}/{res?.max ?? meta?.points}</span>
        <ChevronDown aria-hidden className={cx("size-4 shrink-0 text-muted transition-transform", open && "rotate-180")} />
      </button>
      {open && (
        <div className="space-y-5 border-t px-5 py-5">
          {view ? <Markdown>{view.task}</Markdown>
            : error ? (
              <ErrorNote onRetry={load}>
                <span className="font-medium">{t("This question could not be loaded.")}</span> <span className="font-mono text-xs break-words">{error}</span>
              </ErrorNote>
            ) : (
              <Loading label={t("Loading the question")} className="space-y-3">
                {[0, 1, 2].map((i) => <Skeleton key={i} className={i === 2 ? "h-4 w-1/2" : "h-4"} />)}
              </Loading>
            )}
          {/* A choice question has no checks: its review is the feedback. */}
          {view?.choice && <Choice choice={view.choice} value={view.choice.answer} passed={res?.passed} />}
          {state.note && (
            <div>
              <SectionLabel className="mb-1">{t("Your notes")}</SectionLabel>
              <pre className="rounded-lg border bg-code px-3 py-2 font-mono text-xs whitespace-pre-wrap">{state.note}</pre>
            </div>
          )}
          {res && !choice && <Results result={res} onExplain={assistant.usable ? () => assistant.open(qid, newAsk()) : undefined} />}
          {full && !choice && <Help sid={session.id} q={full} result={res} confirmSolution={false} />}
          {assistant.available && (
            <div className="flex flex-wrap gap-2">
              {choice && assistant.usable && res && !res.passed && (
                <ExplainButton onClick={() => assistant.open(qid, newAsk(t("Why is that the right answer?")))}>{t("Explain")}</ExplainButton>
              )}
              <Button size="sm" aria-pressed={assistant.qid === qid} className={assistant.qid === qid ? "border-accent" : undefined} icon={<Sparkles className="size-4 text-accent" aria-hidden />} onClick={() => assistant.open(qid)}>
                {t("Ask the assistant")}
              </Button>
            </div>
          )}
          <Rating track={track.id} question={qid} />
        </div>
      )}
    </Card>
  );
}
