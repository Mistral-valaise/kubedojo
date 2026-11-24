import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { ArrowLeft, ChevronRight, Eye, PlayCircle, RotateCcw } from "lucide-react";
import { api, ApiError, type QuestionView, type Track } from "../api";
import { useConfig } from "../config";
import { t } from "../i18n";
import { practicable, words } from "../track";
import { Choice } from "../components/Choice";
import { TutorBox } from "../components/Help";
import { Markdown } from "../components/Markdown";
import { Rating } from "../components/Rating";
import { HeaderPager, WorkspaceHeader } from "../components/WorkspaceHeader";
import { Badge, Button, ButtonLink, Card, Difficulty, ErrorNote, Loading, MAIN_ID, Skeleton } from "../components/ui";

/**
 * Practice of a multiple-choice question: the stem, the options and, once
 * answered, the review. Nothing is provisioned, so there is no terminal and
 * no waiting screen.
 */
export function QuizPractice({ track, qid }: { track: Track; qid: string }) {
  const nav = useNavigate();
  const config = useConfig();
  const [sid, setSid] = useState<string | null>(null);
  const [view, setView] = useState<QuestionView | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<"check" | "reveal" | "reset" | null>(null);
  // Counts the rounds of this question, so that a reset mounts fresh options.
  const [round, setRound] = useState(0);
  // Counts the tries to load the question, so that a failed load can be repeated.
  const [attempt, setAttempt] = useState(0);
  const scroller = useRef<HTMLElement>(null);
  const body = useRef<HTMLDivElement>(null);
  // Where the focus goes once the next render is on the page.
  const focusNext = useRef<"options" | "review" | null>(null);

  const show = useCallback((v: QuestionView) => { setView(v); setSelected(v.choice?.answer ?? []); }, []);

  // Open (or reuse) the practice session. Reading the question prepares it.
  useEffect(() => {
    let stop = false;
    setView(null); setError(null); setSelected([]);
    scroller.current?.scrollTo({ top: 0 });
    (async () => {
      try {
        const s = await api.createSession({ mode: "practice", track: track.id });
        if (stop) return;
        setSid(s.id);
        const v = await api.question(s.id, qid);
        if (!stop) show(v);
      } catch (e) {
        if (!stop) setError((e as Error).message);
      }
    })();
    return () => { stop = true; };
  }, [track.id, qid, show, attempt]);

  useEffect(() => {
    if (!focusNext.current || !view) return;
    const target = focusNext.current === "review"
      ? body.current?.querySelector<HTMLElement>("[data-review]")
      : body.current?.querySelector<HTMLElement>('[role="radio"][tabindex="0"], [role="checkbox"][tabindex="0"]');
    // The review appears where the options were, so the page stays where it is.
    target?.focus({ preventScroll: focusNext.current === "review" });
    focusNext.current = null;
  }, [view, round]);

  const run = async (what: "check" | "reveal" | "reset", action: (sid: string) => Promise<unknown>) => {
    if (!sid) return;
    setBusy(what); setError(null);
    try {
      // A question graded elsewhere (another tab) answers 409: showing its
      // state as it is now is the right outcome, not an error.
      await action(sid).catch((e) => { if (!(e instanceof ApiError && e.status === 409)) throw e; });
      const v = await api.question(sid, qid);
      focusNext.current = what === "reset" ? "options" : "review";
      if (what === "reset") setRound((n) => n + 1);
      show(v);
    } catch (e) {
      setError((e as Error).message);
    }
    setBusy(null);
  };
  const check = () => { if (selected.length > 0 && !busy) void run("check", (s) => api.check(s, qid, selected)); };
  const reveal = () => void run("reveal", (s) => api.solution(s, qid));
  const reset = () => void run("reset", (s) => api.prepare(s, qid, true));

  const list = (track.questions ?? []).filter(practicable);
  const meta = list.find((q) => q.id === qid);
  const sameDomain = list.filter((q) => q.domain === meta?.domain);
  const pos = sameDomain.findIndex((q) => q.id === qid);
  const prev = sameDomain[pos - 1], next = sameDomain[pos + 1];
  const domain = track.domains.find((d) => d.id === meta?.domain);
  const w = words(track);
  const choice = view?.choice;
  const result = view?.state.last ?? null;
  const reviewed = !!choice?.review;


  const again = (
    <Button key="again" variant={result?.passed && next ? "secondary" : "primary"} size="lg" loading={busy === "reset"}
      icon={<RotateCcw className="size-4" aria-hidden />} onClick={reset}>{t("Try again")}</Button>
  );
  const onward = next && (
    <Button key="next" variant={result?.passed ? "primary" : "secondary"} size="lg" onClick={() => nav(`/t/${track.id}/q/${next.id}`)}>
      {w.next}<ChevronRight className="size-4" aria-hidden />
    </Button>
  );

  return (
    <div className="flex h-full flex-col">
      <WorkspaceHeader
        back={(
          <Link to={`/t/${track.id}`} className="inline-flex min-h-9 min-w-0 items-center gap-1.5 text-sm text-muted hover:text-text">
            <ArrowLeft className="size-4 shrink-0" aria-hidden /><span className="truncate">{track.short} · {t("Practice")}</span>
          </Link>
        )}
        nav={(
          <HeaderPager position={pos >= 0 ? `${pos + 1} / ${sameDomain.length}` : ""} previousLabel={w.previous} nextLabel={w.next}
            onPrevious={prev && (() => nav(`/t/${track.id}/q/${prev.id}`))} onNext={next && (() => nav(`/t/${track.id}/q/${next.id}`))} />
        )}
        docs={w.docs}
      />

      <main id={MAIN_ID} ref={scroller} className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-[46rem] px-3 py-4 sm:px-6 sm:py-10">
          <Card className="p-4 sm:p-8">
            <div ref={body} className="space-y-6">
              <div>
                <div className="mb-2.5 flex flex-wrap items-center gap-1.5">
                  {/* inline-flex: as plain inline content the badge would sit on the text baseline, below its neighbour. */}
                  {domain && <Link to={`/t/${track.id}/lesson/${domain.id}`} className="inline-flex max-w-full"><Badge tone="accent">{domain.title}</Badge></Link>}
                  {meta && <Difficulty level={meta.difficulty} />}
                </div>
                <h1 className="text-title font-semibold tracking-tight">{meta?.title ?? " "}</h1>
              </div>

              {error && <ErrorNote onRetry={view ? undefined : () => setAttempt((n) => n + 1)}>{error}</ErrorNote>}
              {!view && !error && (
                <Loading label={t("Loading the question")} className="space-y-3">
                  <Skeleton className="h-5" />
                  <Skeleton className="h-5 w-2/3" />
                  {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-12 rounded-xl" />)}
                </Loading>
              )}

              {view && choice && (
                <>
                  <Markdown className="md-lg">{view.task}</Markdown>
                  {!reviewed && (
                    <>
                      <Choice key={round} choice={choice} value={selected} onChange={setSelected} onSubmit={check}
                        disabled={busy !== null} label={meta?.title} />
                      <div className="flex flex-wrap items-center gap-2 border-t pt-5">
                        <Button variant="primary" size="lg" loading={busy === "check"} disabled={selected.length === 0 || busy !== null}
                          icon={<PlayCircle className="size-4" aria-hidden />} onClick={check}>{t("Check answer")}</Button>
                        <Button variant="ghost" size="lg" loading={busy === "reveal"} disabled={busy !== null}
                          icon={<Eye className="size-4" aria-hidden />} onClick={reveal}>{t("Show answer")}</Button>
                        <span className="ml-auto hidden text-xs text-muted sm:block">{t("Press 1-{n} to select, Enter to check", { n: Math.min(9, choice.options.length) })}</span>
                      </div>
                    </>
                  )}
                  {reviewed && (
                    <>
                      {/* Focused by script so that Tab continues from here; it is not a control and gets no ring. */}
                      <div data-review tabIndex={-1} style={{ outline: "none" }}>
                        <Choice choice={choice} value={choice.answer} passed={result?.passed} />
                      </div>
                      <div className="flex flex-wrap items-center gap-2 border-t pt-5">
                        {/* The step the learner most likely wants comes first: on after a right answer, again after a wrong one. */}
                        {result?.passed ? <>{onward}{again}</> : <>{again}{onward}</>}
                        {!next && <ButtonLink to={`/t/${track.id}`} variant="ghost" size="lg">{t("Back to {track}", { track: track.short })}</ButtonLink>}
                      </div>
                      {sid && config?.tutor && <TutorBox sid={sid} qid={qid} result={result} w={w} />}
                      <Rating track={track.id} question={qid} />
                    </>
                  )}
                </>
              )}
            </div>
          </Card>
        </div>
      </main>
    </div>
  );
}
