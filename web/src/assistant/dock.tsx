import { useCallback, useState } from "react";
import { Sparkles } from "lucide-react";
import { useConfig } from "../config";
import { t } from "../i18n";
import { Button, cx } from "../components/ui";

/** localStorage key: "1" while the learner keeps the assistant open. */
export const OPEN_KEY = "kubedojo.assistant.open";
/** Id of the header button that opens and closes the assistant: the focus returns to it when the panel closes itself. */
export const TOGGLE_ID = "assistant-toggle";

/**
 * A message a page asks the panel to send, such as the default question
 * behind "Explain what went wrong". `n` makes every request a new one, so
 * that the same question can be asked twice and none is sent twice.
 */
export interface Ask { n: number; message: string }
let asks = 0;
/** A request to send `message`; an empty one asks the server's default question. */
export const newAsk = (message = ""): Ask => ({ n: ++asks, message });

const remembered = () => { try { return localStorage.getItem(OPEN_KEY) === "1"; } catch { return false; } };

export interface Dock {
  /** The assistant can be shown: it is switched on, or it can be set up here. */
  available: boolean;
  /** The assistant can answer: switched on and configured. */
  usable: boolean;
  /** The panel is open. Never true while the assistant is not available. */
  open: boolean;
  setOpen: (open: boolean) => void;
  /** The message the panel is to send next, if any. */
  ask: Ask | null;
  /** Opens the panel and has it send `message` (the default question when empty). */
  explain: (message?: string) => void;
}

/**
 * What a page needs to dock the assistant: whether to offer it at all, whether
 * its panel is open (remembered across pages and visits), and the message a
 * button of the page wants answered.
 */
export function useAssistantDock(): Dock {
  const status = useConfig()?.assistant;
  const available = !!status && (status.enabled || status.writable);
  const usable = !!status?.enabled && !!status.configured;
  const [wanted, setWanted] = useState(remembered);
  const [ask, setAsk] = useState<Ask | null>(null);
  const setOpen = useCallback((open: boolean) => {
    setWanted(open);
    try { localStorage.setItem(OPEN_KEY, open ? "1" : "0"); } catch { /* ignore */ }
  }, []);
  const explain = useCallback((message = "") => { setOpen(true); setAsk(newAsk(message)); }, [setOpen]);
  return { available, usable, open: available && wanted, setOpen, ask, explain };
}

/** The header button that opens and closes the assistant. On a phone it shows its icon only. */
export function AssistantToggle({ pressed, onClick }: { pressed: boolean; onClick: () => void }) {
  return (
    <button type="button" id={TOGGLE_ID} aria-pressed={pressed} aria-label={t("Assistant")} title={pressed ? t("Close the assistant") : t("Open the assistant")} onClick={onClick}
      className={cx("inline-flex h-9 shrink-0 items-center gap-2 rounded-lg border px-2.5 text-sm font-medium transition sm:px-3",
        pressed ? "border-accent bg-accent-soft text-accent" : "text-muted hover:border-accent/60 hover:text-text")}>
      <Sparkles className="size-4" aria-hidden /><span className="hidden sm:inline">{t("Assistant")}</span>
    </button>
  );
}

/** The button that hands a result to the assistant: "Explain what went wrong" and its like. */
export function ExplainButton({ onClick, children, size = "sm" }: { onClick: () => void; children: string; size?: "sm" | "lg" }) {
  return (
    <Button size={size} onClick={onClick} icon={<Sparkles className="size-4 text-accent" aria-hidden />}>{children}</Button>
  );
}
