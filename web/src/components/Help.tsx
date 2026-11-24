import { useEffect, useState } from "react";
import { Eye, Lightbulb } from "lucide-react";
import { api, type QuestionMeta, type Result } from "../api";
import { t } from "../i18n";
import { Markdown } from "./Markdown";
import { Button, ErrorNote, Modal, SectionLabel } from "./ui";

/** Tiered hints and the worked solution for one question. */
export function Help({ sid, q, result, initialHints = 0, confirmSolution = true }: {
  sid: string; q: QuestionMeta; result?: Result | null; initialHints?: number; confirmSolution?: boolean;
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
      {/* A live region, so that a hint is read out when it appears. It is always here: one added together with its content is not announced. */}
      <div aria-live="polite" className="space-y-4 empty:hidden">
        {hints.map((h, i) => (
          <div key={i} className="rounded-xl border border-warn/30 bg-warn-soft px-4 py-3">
            <SectionLabel tone="warn" icon={<Lightbulb aria-hidden />} className="mb-1">{t("Hint {n}", { n: i + 1 })}</SectionLabel>
            <Markdown>{h}</Markdown>
          </div>
        ))}
      </div>
      {!solution && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border bg-panel py-2 pr-2 pl-4">
          {q.hints > 0 && (
            <>
              <span className="text-sm font-medium">{t("Hints")}</span>
              <span className="text-label text-muted">{t("{n} of {m} used", { n: hints.length, m: q.hints })}</span>
            </>
          )}
          <div className="ml-auto flex flex-wrap items-center gap-1">
            {hints.length < q.hints && (
              <Button size="sm" loading={busy} icon={<Lightbulb className="size-3.5" aria-hidden />} onClick={nextHint}>
                {hints.length === 0 ? t("Show a hint") : t("Next hint ({n} of {m})", { n: hints.length + 1, m: q.hints })}
              </Button>
            )}
            {/* Once every check has passed there is no attempt left to protect: the solution opens without asking. */}
            <Button size="sm" variant="ghost" icon={<Eye className="size-3.5" aria-hidden />} onClick={() => (confirmSolution && !result?.passed ? setConfirm(true) : void showSolution())}>
              {t("Show solution")}
            </Button>
          </div>
        </div>
      )}
      {solution && (
        <div className="rounded-xl border bg-panel px-4 py-4">
          <SectionLabel tone="accent" className="mb-2">{t("Solution")}</SectionLabel>
          <Markdown>{solution}</Markdown>
        </div>
      )}
      {confirm && (
        <Modal title={t("Show the solution?")} onClose={() => setConfirm(false)}
          footer={<><Button onClick={() => setConfirm(false)}>{t("Keep trying")}</Button><Button variant="primary" onClick={showSolution}>{t("Show solution")}</Button></>}>
          {t("You learn most from the attempt. Try a hint first — the solution stays available either way.")}
        </Modal>
      )}
    </div>
  );
}
