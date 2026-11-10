import { useEffect, useState } from "react";
import { Bot, Eye, Lightbulb, Send } from "lucide-react";
import { api, type QuestionMeta, type Result } from "../api";
import { t } from "../i18n";
import { Markdown } from "./Markdown";
import { Button, ErrorNote, Modal } from "./ui";

/** Tiered hints, the worked solution and the optional AI tutor for one question. */
export function Help({ sid, q, result, tutor, initialHints = 0, confirmSolution = true }: {
  sid: string; q: QuestionMeta; result?: Result | null; tutor: boolean; initialHints?: number; confirmSolution?: boolean;
}) {
  const [hints, setHints] = useState<string[]>([]);
  const [solution, setSolution] = useState<string | null>(null);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setHints([]); setSolution(null); setError(null);
    // Re-open hints that were already revealed earlier in this session.
    let stop = false;
    (async () => {
      const out: string[] = [];
      for (let n = 1; n <= Math.min(initialHints, q.hints); n++) {
        try { out.push((await api.hint(sid, q.id, n)).markdown); } catch { break; }
      }
      if (!stop && out.length) setHints(out);
    })();
    return () => { stop = true; };
  }, [sid, q.id, q.hints, initialHints]);

  const nextHint = async () => {
    setBusy(true);
    try { const h = await api.hint(sid, q.id, hints.length + 1); setHints((x) => [...x, h.markdown]); }
    catch (e) { setError((e as Error).message); }
    setBusy(false);
  };
  const showSolution = async () => {
    setConfirm(false);
    try { setSolution((await api.solution(sid, q.id)).markdown); }
    catch (e) { setError((e as Error).message); }
  };

  return (
    <div className="space-y-4">
      {error && <ErrorNote>{error}</ErrorNote>}
      {hints.map((h, i) => (
        <div key={i} className="rounded-xl border border-warn/30 bg-warn-soft/60 px-4 py-3">
          <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-warn">
            <Lightbulb className="size-3.5" />{t("Hint {n}", { n: i + 1 })}
          </div>
          <Markdown className="text-[0.9rem]">{h}</Markdown>
        </div>
      ))}
      {!solution && (
        <div className="flex flex-wrap gap-2">
          {hints.length < q.hints && (
            <Button size="sm" loading={busy} icon={<Lightbulb className="size-3.5" />} onClick={nextHint}>
              {hints.length === 0 ? t("Show a hint") : t("Next hint ({n} of {m})", { n: hints.length + 1, m: q.hints })}
            </Button>
          )}
          <Button size="sm" variant="ghost" icon={<Eye className="size-3.5" />} onClick={() => (confirmSolution ? setConfirm(true) : void showSolution())}>
            {t("Show solution")}
          </Button>
        </div>
      )}
      {solution && (
        <div className="rounded-xl border bg-panel2/40 px-4 py-4">
          <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-accent">{t("Solution")}</div>
          <Markdown>{solution}</Markdown>
        </div>
      )}
      {tutor && <TutorBox sid={sid} qid={q.id} result={result} />}
      {confirm && (
        <Modal title={t("Show the solution?")} onClose={() => setConfirm(false)}
          footer={<><Button onClick={() => setConfirm(false)}>{t("Keep trying")}</Button><Button variant="primary" onClick={showSolution}>{t("Show solution")}</Button></>}>
          {t("You learn most from the attempt. Try a hint first — the solution stays available either way.")}
        </Modal>
      )}
    </div>
  );
}

function TutorBox({ sid, qid, result }: { sid: string; qid: string; result?: Result | null }) {
  const [ask, setAsk] = useState("");
  const [answer, setAnswer] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => { setAnswer(null); setAsk(""); setError(null); }, [qid]);

  const send = async (question: string) => {
    setBusy(true); setError(null);
    try { setAnswer((await api.tutor(sid, qid, question)).markdown); }
    catch (e) { setError((e as Error).message); }
    setBusy(false);
  };
  const failed = result && !result.passed;

  return (
    <div className="rounded-xl border bg-panel2/40 p-4">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-accent"><Bot className="size-3.5" />{t("AI tutor")}</div>
      {answer && <Markdown className="mb-3">{answer}</Markdown>}
      {error && <div className="mb-3"><ErrorNote>{error}</ErrorNote></div>}
      {failed && !answer && (
        <Button size="sm" variant="primary" loading={busy} className="mb-3" onClick={() => send("")}>{t("Explain what went wrong")}</Button>
      )}
      <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); if (ask.trim()) void send(ask.trim()); }}>
        <input value={ask} onChange={(e) => setAsk(e.target.value)} placeholder={t("Ask about this task ...")}
          className="h-9 min-w-0 flex-1 rounded-lg border bg-panel px-3 text-sm outline-none placeholder:text-muted focus:border-accent" />
        <Button size="md" loading={busy} disabled={!ask.trim()} aria-label={t("Send")}><Send className="size-4" /></Button>
      </form>
      <p className="mt-2 text-[11px] text-muted">{t("The tutor sees the task, your check results and a summary of the cluster state.")}</p>
    </div>
  );
}
