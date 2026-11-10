import { useEffect, useRef } from "react";
import type { Session } from "../api";
import { t } from "../i18n";
import { Spinner } from "./ui";

/** Shown in place of the terminal while the sandbox is being created. */
export function Provisioning({ session, title, note }: { session: Session | null; title: string; note?: string }) {
  const end = useRef<HTMLDivElement>(null);
  const log = session?.log ?? [];
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [log.length]);
  return (
    <div className="flex h-full w-full flex-col items-center justify-center bg-[#0a0e17] p-8 text-[#d7deea]">
      <div className="w-full max-w-xl">
        <div className="mb-1 flex items-center gap-3">
          <Spinner className="text-[#5b9dff]" />
          <h2 className="text-lg font-semibold">{title}</h2>
        </div>
        <p className="mb-5 pl-8 text-sm text-[#8d99b3]">{note ?? t("A real Kubernetes cluster is being created for you. This takes about a minute.")}</p>
        <div className="h-56 overflow-y-auto rounded-xl border border-white/10 bg-black/30 p-4 font-mono text-[12.5px] leading-6">
          {log.length === 0 && <div className="text-[#5a6784]">{t("Waiting for the sandbox ...")}</div>}
          {log.map((l, i) => (
            <div key={i} className={i === log.length - 1 ? "text-[#d7deea]" : "text-[#5a6784]"}>
              <span className="mr-2 text-[#3ddc97]">{i === log.length - 1 ? "›" : "✓"}</span>{l.msg}
            </div>
          ))}
          <div ref={end} />
        </div>
      </div>
    </div>
  );
}
