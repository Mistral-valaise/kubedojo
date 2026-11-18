import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  AlertTriangle, BookOpen, CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, Flag, Flame, Server,
  SquareTerminal, Trash2, XCircle,
} from "lucide-react";
import { api, type Config, type QuestionView, type Session, type Track } from "../api";
import { fmtDuration, useInterval } from "../hooks";
import { t } from "../i18n";
import { words } from "../track";
import { Help } from "../components/Help";
import { Markdown } from "../components/Markdown";
import { Provisioning } from "../components/Provisioning";
import { Rating } from "../components/Rating";
import { Results } from "../components/Results";
import { Split } from "../components/Split";
import { Terminal } from "../components/Terminal";
import { TerminalTabs } from "../components/TerminalTabs";
import { Badge, Bar, Brand, Button, Card, ErrorNote, Modal, Page, Ring, Spinner, ThemeToggle, cx } from "../components/ui";

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
      return <Workspace session={session} track={track} onSession={setSession} refresh={refresh} />;
  }
}

function Stage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col">
      <header className="flex h-12 shrink-0 items-center gap-3 border-b bg-bg px-4"><Brand /><div className="ml-auto"><ThemeToggle /></div></header>
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

function Workspace({ session, track, onSession, refresh }: {
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
  const scroller = useRef<HTMLDivElement>(null);

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
    try { onSession(await api.finish(session.id)); } catch { await refresh(); }
  };

  const low = left < 15 * 60, critical = left < 5 * 60;
  const flagged = Object.values(flags).filter(Boolean).length;
  const meta = session.meta[cur.id];
  const domain = track.domains.find((d) => d.id === meta?.domain);

  const leftPane = (
    <div className="flex h-full flex-col bg-panel">
      <nav className="flex shrink-0 flex-wrap gap-1.5 border-b px-4 py-2.5" aria-label={t("Questions")}>
        {qs.map((q, i) => (
          <button key={q.id} onClick={() => setIdx(i)} aria-current={i === idx}
            className={cx("relative flex size-8 items-center justify-center rounded-lg border text-xs font-medium tabular-nums transition",
              i === idx ? "border-accent bg-accent text-white" : "bg-panel2 text-muted hover:border-accent/60 hover:text-text")}>
            {i + 1}
            {flags[q.id] && <span className="absolute -right-1 -top-1 size-2.5 rounded-full border-2 border-panel bg-warn" />}
          </button>
        ))}
      </nav>
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto">
        <div className="space-y-5 px-6 py-5">
          <div>
            <div className="mb-2 flex flex-wrap items-center gap-1.5">
              <Badge tone="accent">{t("Question {n} of {m}", { n: idx + 1, m: qs.length })}</Badge>
              {domain && <Badge>{domain.title}</Badge>}
              {meta && <Badge>{t("{n} points", { n: meta.points })}</Badge>}
              {meta?.host && <Badge><Server className="size-3" />{meta.host}</Badge>}
            </div>
            <h1 className="text-xl font-semibold tracking-tight">{meta?.title}</h1>
          </div>
          {view ? <Markdown>{view.task}</Markdown> : <Spinner />}
          <div>
            <label htmlFor="note" className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-muted">{t("Notes")}</label>
            <textarea id="note" value={notes[cur.id] ?? ""} onChange={(e) => setNote(e.target.value)} rows={3}
              placeholder={w.notePlaceholder}
              className="w-full resize-y rounded-xl border bg-panel2/50 px-3 py-2 font-mono text-[13px] outline-none placeholder:font-sans placeholder:text-muted focus:border-accent" />
          </div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2 border-t px-4 py-2.5">
        <Button size="sm" disabled={idx === 0} onClick={() => setIdx(idx - 1)} icon={<ChevronLeft className="size-4" />}>{t("Previous")}</Button>
        <Button size="sm" variant={flags[cur.id] ? "secondary" : "ghost"} onClick={toggleFlag}
          icon={<Flag className={cx("size-3.5", flags[cur.id] && "fill-warn text-warn")} />}>
          {flags[cur.id] ? t("Flagged") : t("Flag for review")}
        </Button>
        <Button size="sm" className="ml-auto" disabled={idx === qs.length - 1} onClick={() => setIdx(idx + 1)}>{t("Next")}<ChevronRight className="size-4" /></Button>
      </div>
    </div>
  );

  return (
    <div className="flex h-full flex-col">
      <header className="flex h-12 shrink-0 items-center gap-3 border-b bg-bg px-4">
        <Brand />
        <span className="text-line">/</span>
        <span className="text-sm text-muted">{track.short} · {w.assessment}</span>
        <div className={cx("ml-auto flex items-center gap-2 rounded-lg border px-3 py-1 font-mono text-sm font-semibold tabular-nums",
          critical ? "border-bad/50 bg-bad-soft text-bad" : low ? "border-warn/50 bg-warn-soft text-warn" : "bg-panel2")}
          role="timer" aria-label={t("Time remaining")}>
          {critical && <AlertTriangle className="size-3.5" />}{fmtDuration(left)}
        </div>
        {w.assessmentDocs && (
          <a href={w.assessmentDocs.url} target="_blank" rel="noreferrer" title={w.assessmentDocs.title} className="inline-flex items-center gap-1.5 px-1 text-sm text-muted hover:text-text">
            <BookOpen className="size-3.5" />{t("Docs")}
          </a>
        )}
        <ThemeToggle />
        <Button size="sm" variant="primary" onClick={() => setConfirm(true)}>{w.finishAssessment}</Button>
      </header>
      <Split left={leftPane} right={<TerminalTabs sessionId={session.id} />} storageKey="kubedojo.split.exam" />
      {confirm && (
        <Modal title={w.finishTitle} onClose={() => !busy && setConfirm(false)}
          footer={<><Button disabled={busy} onClick={() => setConfirm(false)}>{t("Keep working")}</Button><Button variant="primary" loading={busy} onClick={finish}>{t("Finish and grade")}</Button></>}>
          <p>{t("You still have {time} left. After finishing you cannot change your answers.", { time: fmtDuration(left) })}</p>
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
  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <Page docs={w.docs} crumbs={[{ label: track.short, to: `/t/${track.id}` }, { label: w.assessmentReport }]}>
          <Card className={cx("mb-6 flex flex-wrap items-center gap-7 p-7", r.passed ? "border-ok/40" : "border-bad/40")}>
            <Ring value={r.percent} size={140} stroke={12} tone={r.passed ? "ok" : "bad"}>
              <span className="text-3xl font-bold tabular-nums">{r.percent}%</span>
              <span className="text-[11px] text-muted">{t("{a} / {b} pts", { a: r.score, b: r.max })}</span>
            </Ring>
            <div className="min-w-0 flex-1">
              <div className={cx("mb-1 inline-flex items-center gap-1.5 text-sm font-semibold", r.passed ? "text-ok" : "text-bad")}>
                {r.passed ? <CheckCircle2 className="size-4" /> : <XCircle className="size-4" />}
                {r.passed ? t("Passed") : t("Not passed")}
              </div>
              <h1 className="text-2xl font-bold tracking-tight">
                {r.passed ? w.passedHeadline : t("{n} percentage points short of the pass mark.", { n: Math.max(0.1, Math.round((r.passPercent - r.percent) * 10) / 10) })}
              </h1>
              <p className="mt-1.5 text-muted">
                {w.reportSummary(r.passPercent, solved, qs.length, fmtDuration(r.usedSeconds))}
              </p>
            </div>
          </Card>

          <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
            <div className="space-y-6">
              <Card className="p-5">
                <h2 className="mb-4 text-xs font-semibold uppercase tracking-wider text-muted">{t("Score by domain")}</h2>
                <ul className="space-y-4">
                  {r.domains.map((d) => {
                    const pct = d.max ? Math.round((d.score / d.max) * 100) : 0;
                    return (
                      <li key={d.id}>
                        <div className="mb-1.5 flex items-baseline justify-between gap-3 text-sm">
                          <Link to={`/t/${track.id}/lesson/${d.id}`} className="truncate hover:text-accent hover:underline">{d.title}</Link>
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
          {state.note && (
            <div>
              <div className="mb-1 text-xs font-semibold uppercase tracking-wider text-muted">{t("Your notes")}</div>
              <pre className="rounded-lg border bg-code px-3 py-2 font-mono text-xs whitespace-pre-wrap">{state.note}</pre>
            </div>
          )}
          {res && <Results result={res} />}
          {full && <Help sid={session.id} q={full} result={res} tutor={tutor} track={track} confirmSolution={false} />}
          <Rating track={track.id} question={qid} />
        </div>
      )}
    </Card>
  );
}
