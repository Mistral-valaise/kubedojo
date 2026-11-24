import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  AlertTriangle, ArrowLeft, Check, CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, Flag, Flame, Server,
  SquareTerminal, Trash2, XCircle,
} from "lucide-react";
import { api, type Config, type QuestionView, type Session, type Track } from "../api";
import { fmtDuration, useInterval } from "../hooks";
import { plural, t } from "../i18n";
import { isFullAssessment, words } from "../track";
import { Choice } from "../components/Choice";
import { Help, TutorBox } from "../components/Help";
import { Markdown } from "../components/Markdown";
import { Provisioning } from "../components/Provisioning";
import { Rating } from "../components/Rating";
import { Results } from "../components/Results";
import { Split } from "../components/Split";
import { Terminal } from "../components/Terminal";
import { TerminalTabs } from "../components/TerminalTabs";
import { WorkspaceHeader } from "../components/WorkspaceHeader";
import { Badge, Bar, Button, Card, ErrorNote, Modal, Page, Ring, Spinner, cx } from "../components/ui";

export function ExamPage() {
  const { session: sid = "" } = useParams();
  const [session, setSession] = useState<Session | null>(null);
  const [track, setTrack] = useState<Track | null>(null);
  const [config, setConfig] = useState<Config | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try { setSession(await api.session(sid)); setError(null); }
    catch (e) { setError((e as Error).message); }
  }, [sid]);
  useEffect(() => { void refresh(); api.config().then(setConfig).catch(() => undefined); }, [refresh]);
  useEffect(() => { if (session && !track) api.track(session.track).then(setTrack).catch(() => undefined); }, [session, track]);
  const fast = session?.status === "provisioning" || session?.status === "grading";
  useInterval(() => void refresh(), fast ? 1500 : 10000, !!session && session.status !== "finished" && session.status !== "error");

  if (error && !session) return <Page><ErrorNote>{t("This session no longer exists.")} <Link className="underline" to="/">{t("Back to start")}</Link></ErrorNote></Page>;
  if (!session || !track) return <Page docs={null}><Spinner /></Page>;
  const w = words(track);
  // Without a sandbox nothing is provisioned and grading is immediate, so
  // these states pass in an instant and have no log to show.
  if (!w.sandbox && (session.status === "provisioning" || session.status === "grading")) return <Page docs={null}><Spinner /></Page>;

  switch (session.status) {
    case "provisioning":
      return <Stage><div className="h-full"><Provisioning session={session} title={w.preparingTitle}
        note={w.assessmentProvisioningNote} /></div></Stage>;
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
      return <Stage><div className="h-full"><Provisioning session={session} title={w.gradingTitle}
        note={w.gradingNote} /></div></Stage>;
    case "finished":
      return <ReportView session={session} track={track} tutor={!!config?.tutor} />;
    default:
      return <ExamWorkspace session={session} track={track} onSession={setSession} refresh={refresh} />;
  }
}

function Stage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col">
      <WorkspaceHeader />
      <div className="min-h-0 flex-1">{children}</div>
    </div>
  );
}

function EndButton({ sid, to, label }: { sid: string; to: string; label: string }) {
  const nav = useNavigate();
  const [busy, setBusy] = useState(false);
  return (
    <Button size="sm" loading={busy} icon={<Trash2 className="size-3.5" />}
      onClick={async () => { setBusy(true); try { await api.destroy(sid); } catch { /* already gone */ } nav(to); }}>
      {label}
    </Button>
  );
}

function Briefing({ session, track, onStart }: { session: Session; track: Track; onStart: () => Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const n = session.questions?.length ?? 0;
  const hosts = Object.values(session.envs ?? {});
  const w = words(track);
  return (
    <Page docs={w.assessmentDocs} crumbs={[{ label: track.short, to: `/t/${track.id}` }, { label: w.assessment }]}>
      <Card className="mx-auto max-w-2xl p-8">
        <Badge tone="ok"><CheckCircle2 className="size-3" />{w.environmentReady}</Badge>
        <h1 className="mt-3 text-2xl font-bold tracking-tight">{track.title}</h1>
        <p className="mt-1 text-muted">{w.briefingSummary(n, session.durationMinutes, track.exam.passPercent)}</p>
        <ul className="mt-6 space-y-3 text-sm leading-relaxed">
          {w.briefing.map((x, i) => (
            <li key={i} className="flex gap-3"><span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[11px] font-bold text-accent">{i + 1}</span><span className="text-muted">{x}</span></li>
          ))}
        </ul>
        {hosts.length > 0 && (
          <div className="mt-6 flex flex-wrap items-center gap-2 rounded-xl bg-panel2 px-4 py-3 text-sm">
            <Server className="size-4 text-muted" /><span className="text-muted">{t("Your hosts")}</span>
            {hosts.map((h) => <code key={h} className="rounded-md border bg-panel px-1.5 py-0.5 font-mono text-xs">{h}</code>)}
          </div>
        )}
        <div className="mt-7 flex items-center gap-3">
          <Button variant="primary" size="lg" loading={busy} icon={<Flame className="size-4" />}
            onClick={async () => { setBusy(true); try { await onStart(); } finally { setBusy(false); } }}>
            {t("Start the timer")}
          </Button>
          <EndButton sid={session.id} to={`/t/${track.id}`} label={t("Cancel")} />
        </div>
      </Card>
    </Page>
  );
}

function ExamWorkspace({ session, track, onSession, refresh }: {
  session: Session; track: Track; onSession: (s: Session) => void; refresh: () => Promise<void>;
}) {
  const qs = session.questions ?? [];
  const [idx, setIdx] = useState(() => {
    try { return Math.min(Number(sessionStorage.getItem(`kubedojo.exam.${session.id}`)) || 0, qs.length - 1); } catch { return 0; }
  });
  const [view, setView] = useState<QuestionView | null>(null);
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
    setView(null);
    scroller.current?.scrollTo({ top: 0 });
    try { sessionStorage.setItem(`kubedojo.exam.${session.id}`, String(idx)); } catch { /* ignore */ }
    api.question(session.id, cur.id).then(setView).catch(() => undefined);
  }, [session.id, cur.id, idx]);

  const saveTimer = useRef<number>(0);
  const setNote = (text: string) => {
    setNotes((n) => ({ ...n, [cur.id]: text }));
    window.clearTimeout(saveTimer.current);
    const id = cur.id;
    saveTimer.current = window.setTimeout(() => void api.patchQuestion(session.id, id, { note: text }), 600);
  };
  const toggleFlag = () => {
    const v = !flags[cur.id];
    setFlags((f) => ({ ...f, [cur.id]: v }));
    void api.patchQuestion(session.id, cur.id, { flagged: v });
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
    <div className="flex h-full flex-col bg-panel">
      <div className={cx("shrink-0 border-b py-2.5", bar)}>
        <div className={cx("flex flex-wrap items-center gap-x-4 gap-y-2", column)}>
          <nav className="flex flex-wrap gap-1.5" aria-label={t("Questions")}>
            {qs.map((q, i) => (
              <button key={q.id} onClick={() => setIdx(i)} aria-current={i === idx}
                // The state is in the name and in the border, not in colour alone.
                aria-label={quiz ? [t("Question {n}", { n: i + 1 }), answered(q.id) ? t("answered") : t("not answered"), flags[q.id] && t("flagged")].filter(Boolean).join(", ") : undefined}
                className={cx("relative flex size-8 items-center justify-center rounded-lg border text-xs font-medium tabular-nums transition",
                  i === idx ? "border-accent bg-accent text-white"
                    : !quiz ? "bg-panel2 text-muted hover:border-accent/60 hover:text-text"
                      : answered(q.id) ? "border-accent/50 bg-accent-soft font-semibold text-accent hover:border-accent"
                        : "border-dashed border-muted/50 text-muted hover:border-accent/60 hover:text-text")}>
                {i + 1}
                {flags[q.id] && <span className="absolute -right-1 -top-1 size-2.5 rounded-full border-2 border-panel bg-warn" />}
              </button>
            ))}
          </nav>
          {quiz && <span className="ml-auto text-xs tabular-nums text-muted">{t("{a} of {n} answered", { a: qs.length - unanswered, n: qs.length })}</span>}
        </div>
      </div>
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto">
        <div className={cx("space-y-5 py-5", quiz ? "sm:py-8" : "px-4 sm:px-6", column)}>
          <div>
            <div className="mb-2 flex flex-wrap items-center gap-1.5">
              <Badge tone="accent">{w.position(idx + 1, qs.length)}</Badge>
              {domain && <Badge>{domain.title}</Badge>}
              {/* Every question of a quiz counts the same, so the points say nothing. */}
              {meta && !quiz && <Badge>{plural(meta.points, "{n} point", "{n} points")}</Badge>}
              {meta?.host && <Badge><Server className="size-3" />{meta.host}</Badge>}
            </div>
            <h1 className="text-xl font-semibold tracking-tight">{meta?.title}</h1>
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
          {view?.question.id === cur.id ? <Markdown className={quiz ? "md-lg" : undefined}>{view.task}</Markdown> : <Spinner />}
          {view?.choice && view.question.id === cur.id && (
            <div>
              <Choice choice={view.choice} value={answers[cur.id] ?? []} onChange={setAnswer} hotkeys={!confirm} label={meta?.title} />
              <div className="mt-2 flex min-h-8 items-center gap-2 text-xs text-muted" aria-live="polite">
                {saving > 0 ? <span>{t("Saving ...")}</span>
                  : answered(cur.id) ? <span className="inline-flex items-center gap-1"><Check aria-hidden className="size-3.5 text-ok" />{t("Answer saved")}</span>
                    : <span>{t("Not answered yet")}</span>}
                {answered(cur.id) && <Button size="sm" variant="ghost" className="ml-auto" onClick={() => setAnswer([])}>{t("Clear answer")}</Button>}
              </div>
            </div>
          )}
          <div>
            <label htmlFor="note" className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-muted">{t("Notes")}</label>
            <textarea id="note" value={notes[cur.id] ?? ""} onChange={(e) => setNote(e.target.value)} rows={3}
              placeholder={w.notePlaceholder}
              className="w-full resize-y rounded-xl border bg-panel2/50 px-3 py-2 font-mono text-[13px] outline-none placeholder:font-sans placeholder:text-muted focus:border-accent" />
          </div>
        </div>
      </div>
      <div className={cx("shrink-0 border-t py-2.5", bar)}>
        <div className={cx("flex items-center gap-2", column)}>
          <Button size="sm" disabled={idx === 0} onClick={() => setIdx(idx - 1)} icon={<ChevronLeft className="size-4" />}>{t("Previous")}</Button>
          <Button size="sm" variant={flags[cur.id] ? "secondary" : "ghost"} onClick={toggleFlag}
            icon={<Flag className={cx("size-3.5", flags[cur.id] && "fill-warn text-warn")} />}>
            {flags[cur.id] ? t("Flagged") : t("Flag for review")}
          </Button>
          <Button size="sm" className="ml-auto" disabled={idx === qs.length - 1} onClick={() => setIdx(idx + 1)}>{t("Next")}<ChevronRight className="size-4" /></Button>
        </div>
      </div>
    </div>
  );

  return (
    <div className="flex h-full flex-col">
      {/* On a phone the header has room for the timer and the button only. */}
      <WorkspaceHeader
        title={<span className="min-w-0 truncate text-sm text-muted">{track.short}<span className="hidden sm:inline"> · {w.assessment}</span></span>}
        center={(
          <div className={cx("ml-auto shrink-0 flex items-center gap-2 rounded-lg border px-3 py-1 font-mono text-sm font-semibold tabular-nums",
            critical ? "border-bad/50 bg-bad-soft text-bad" : low ? "border-warn/50 bg-warn-soft text-warn" : "bg-panel2")}
            role="timer" aria-label={t("Time remaining")}>
            {critical && <AlertTriangle className="size-3.5" />}{fmtDuration(left)}
          </div>
        )}
        docs={w.assessmentDocs}
        end={(
          // Next to a terminal the phone header is full: the button keeps its name and shows a short one.
          <Button size="sm" variant="primary" onClick={() => setConfirm(true)} aria-label={w.finishAssessment}>
            {quiz ? w.finishAssessment : <><span className="sm:hidden">{t("Finish")}</span><span className="hidden sm:inline">{w.finishAssessment}</span></>}
          </Button>
        )}
      />
      {quiz
        ? <div className="min-h-0 flex-1">{leftPane}</div>
        : <Split left={leftPane} right={<TerminalTabs sessionId={session.id} />} storageKey="kubedojo.split.exam" labels={{ left: w.item, right: t("Terminal") }} />}
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

function ReportView({ session, track, tutor }: { session: Session; track: Track; tutor: boolean }) {
  const r = session.report!;
  const [term, setTerm] = useState(false);
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
      <div className="min-h-0 flex-1 overflow-y-auto">
        <Page docs={w.docs} crumbs={[{ label: track.short, to: `/t/${track.id}` }, { label: w.assessmentReport }]}>
          <Card className={cx("mb-6 flex flex-wrap items-center gap-x-7 gap-y-5 p-5 sm:p-7", r.passed ? "border-ok/40" : "border-bad/40")}>
            <Ring value={r.percent} size={140} stroke={12} tone={r.passed ? "ok" : "bad"}>
              <span className="text-3xl font-bold tabular-nums">{r.percent}%</span>
              <span className="text-[11px] text-muted">{t("{a} / {b} pts", { a: r.score, b: r.max })}</span>
            </Ring>
            {/* The basis lets the text wrap below the ring on a narrow screen. */}
            <div className="min-w-0 flex-1 basis-64">
              <div className={cx("mb-1 inline-flex items-center gap-1.5 text-sm font-semibold", r.passed ? "text-ok" : "text-bad")}>
                {r.passed ? <CheckCircle2 className="size-4" /> : <XCircle className="size-4" />}
                {r.passed ? t("Passed") : t("Not passed")}
              </div>
              <h1 className="text-2xl font-bold tracking-tight">{headline}</h1>
              <p className="mt-1.5 text-muted">
                {w.reportSummary(r.passPercent, solved, qs.length, fmtDuration(r.usedSeconds))}
              </p>
              {quick && <p className="mt-1.5 text-sm text-muted">{w.quickRoundNote}</p>}
              {/* With a sandbox the way back is its "Remove sandbox" button. */}
              {!w.sandbox && (
                <Link to={`/t/${track.id}`} className="mt-4 inline-block">
                  <Button icon={<ArrowLeft className="size-4" />}>{t("Back to {track}", { track: track.short })}</Button>
                </Link>
              )}
            </div>
          </Card>

          <div className="grid grid-cols-[minmax(0,1fr)] gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
            <div className="space-y-6">
              <Card className="p-5">
                <h2 className="mb-4 text-xs font-semibold uppercase tracking-wider text-muted">{t("Score by domain")}</h2>
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
                <Card className="space-y-3 p-5 text-sm">
                  <h2 className="text-xs font-semibold uppercase tracking-wider text-muted">{t("Sandbox")}</h2>
                  <p className="leading-relaxed text-muted">{w.reportSandbox}</p>
                  <div className="flex flex-wrap gap-2">
                    <Button size="sm" icon={<SquareTerminal className="size-3.5" />} onClick={() => setTerm(!term)}>{term ? t("Hide terminal") : t("Open terminal")}</Button>
                    <EndButton sid={session.id} to={`/t/${track.id}`} label={t("Remove sandbox")} />
                  </div>
                </Card>
              )}
            </div>
            <div>
              <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-muted">{w.reportList}</h2>
              <div className="space-y-3">
                {qs.map((q, i) => <ReportQuestion key={q.id} n={i + 1} session={session} track={track} qid={q.id} tutor={tutor} />)}
              </div>
            </div>
          </div>
        </Page>
      </div>
      {term && <div className="h-[38vh] shrink-0 border-t"><Terminal sessionId={session.id} /></div>}
    </div>
  );
}

function ReportQuestion({ n, session, track, qid, tutor }: { n: number; session: Session; track: Track; qid: string; tutor: boolean }) {
  const state = session.questions!.find((q) => q.id === qid)!;
  const meta = session.meta[qid];
  const res = state.last;
  const [open, setOpen] = useState(false);
  const [view, setView] = useState<QuestionView | null>(null);
  useEffect(() => { if (open && !view) api.question(session.id, qid).then(setView).catch(() => undefined); }, [open, view, session.id, qid]);
  const full = track.questions?.find((q) => q.id === qid);
  const choice = full?.type === "choice";
  const tone = !res ? "muted" : res.passed ? "ok" : res.score > 0 ? "warn" : "bad";
  return (
    <Card className="overflow-hidden">
      <button onClick={() => setOpen(!open)} aria-expanded={open} className="flex w-full items-center gap-3 px-5 py-3.5 text-left hover:bg-panel2/50">
        <span className={cx("flex size-7 shrink-0 items-center justify-center rounded-lg text-xs font-bold tabular-nums",
          tone === "ok" ? "bg-ok-soft text-ok" : tone === "warn" ? "bg-warn-soft text-warn" : tone === "bad" ? "bg-bad-soft text-bad" : "bg-panel2 text-muted")}>{n}</span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{meta?.title}</span>
          <span className="block truncate text-xs text-muted">{track.domains.find((d) => d.id === meta?.domain)?.title}</span>
        </span>
        {state.flagged && <Flag className="size-3.5 fill-warn text-warn" />}
        <span className="shrink-0 text-sm tabular-nums text-muted">{res?.score ?? 0}/{res?.max ?? meta?.points}</span>
        <ChevronDown className={cx("size-4 shrink-0 text-muted transition-transform", open && "rotate-180")} />
      </button>
      {open && (
        <div className="space-y-5 border-t px-5 py-5">
          {view ? <Markdown>{view.task}</Markdown> : <Spinner />}
          {/* A choice question has no checks: its review is the feedback. */}
          {view?.choice && <Choice choice={view.choice} value={view.choice.answer} passed={res?.passed} />}
          {state.note && (
            <div>
              <div className="mb-1 text-xs font-semibold uppercase tracking-wider text-muted">{t("Your notes")}</div>
              <pre className="rounded-lg border bg-code px-3 py-2 font-mono text-xs whitespace-pre-wrap">{state.note}</pre>
            </div>
          )}
          {res && !choice && <Results result={res} />}
          {full && !choice && <Help sid={session.id} q={full} result={res} tutor={tutor} track={track} confirmSolution={false} />}
          {choice && tutor && <TutorBox sid={session.id} qid={qid} result={res} w={words(track)} />}
          <Rating track={track.id} question={qid} />
        </div>
      )}
    </Card>
  );
}
