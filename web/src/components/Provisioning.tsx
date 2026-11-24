import { useEffect, useRef } from "react";
import type { Session } from "../api";
import { t } from "../i18n";
import { Spinner } from "./ui";

/**
 * Shown in place of the terminal while the sandbox is being created. The note
 * says what is being set up; it is empty when the track is not known yet or
 * has no sandbox. It is a terminal surface: dark in both themes.
 */
export function Provisioning({ session, title, note }: { session: Session | null; title: string; note: string }) {
  const end = useRef<HTMLDivElement>(null);
  const log = session?.log ?? [];
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [log.length]);
  return (
    <div className="term-surface flex h-full w-full flex-col items-center justify-center bg-term p-5 text-term-text sm:p-8">
      <div className="w-full max-w-xl">
        <div className="mb-1 flex items-center gap-3">
          <Spinner className="text-term-accent" />
          <h2 className="text-lg font-semibold">{title}</h2>
        </div>
        <p className="mb-5 min-h-5 pl-8 text-sm text-term-muted">{note}</p>
        <div role="log" aria-label={t("Progress")} className="h-56 overflow-y-auto rounded-xl border border-term-line bg-term-line/40 p-4 font-mono text-label leading-6">
          {log.length === 0 && <div className="text-term-muted">{t("Waiting for the sandbox ...")}</div>}
          {log.map((l, i) => (
            <div key={i} className={i === log.length - 1 ? "text-term-text" : "text-term-muted"}>
              <span aria-hidden className="mr-2 text-term-ok">{i === log.length - 1 ? "›" : "✓"}</span>{l.msg}
            </div>
          ))}
          <div ref={end} />
        </div>
      </div>
    </div>
  );
}
