import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { ArrowUp, Check, Copy, MessageSquarePlus, RotateCw, Settings, Sparkles, Square, TriangleAlert, X } from "lucide-react";
import type { AssistantStatus, Track } from "../api";
import { refreshConfig, useConfig } from "../config";
import { plural, t } from "../i18n";
import { type Words, words } from "../track";
import { Markdown, useCopied } from "../components/Markdown";
import { Badge, Button, ButtonLink, EmptyState, IconButton, Loading, Skeleton, Spinner, cx } from "../components/ui";
import type { Ask } from "./dock";
import { type Assistant, type ChatMessage, type Failure, useAssistant } from "./useAssistant";

/** Where the learner stands with the question, which decides what the panel suggests to ask. */
export type Situation = "none" | "failed" | "passed" | "reviewed";

const PROVIDERS: Record<string, string> = {
  anthropic: "Anthropic", openai: "OpenAI", ollama: "Ollama", openrouter: "OpenRouter", groq: "Groq",
  mistral_ai: "Mistral", deepseek: "DeepSeek", x_ai: "xAI", openai_compatible: "OpenAI-compatible",
};
/** The name of a provider as the server reports it in /api/config. */
export const providerLabel = (id: string) => PROVIDERS[id] ?? id;

// The newest request of a page that a panel has acted on. It outlives the
// panel, so that a panel mounted again (a sheet that becomes a column) does
// not send the same question twice.
let handledAsk = 0;

function suggestions(situation: Situation, w: Words): string[] {
  switch (situation) {
    case "failed": return [t("What went wrong, and what should I look at next?"), t("Give me a hint, not the solution")];
    case "passed": return [w.rememberPrompt];
    case "reviewed": return [t("Why is that the right answer?")];
    default: return [t("Where should I start?")];
  }
}

/** A failure in words, with what can be done about it. Never the raw answer of the server. */
function describe(f: Failure, status: AssistantStatus | undefined, limit: number): { text: string; retry?: boolean; settings?: boolean; reload?: boolean } {
  // A sentence of the server is shown as it is; anything else it sent (HTML of a proxy, JSON) is not.
  const said = /^[^<{[]/.test(f.message) && f.message.length <= 300 ? f.message : "";
  switch (f.code) {
    case "unreachable": return { text: said || t("The model could not be reached. Is it running, and is the base URL right?"), retry: true, settings: true };
    case "auth": return { text: t("The provider rejected the API key. Check the key in the settings."), settings: true };
    case "model_not_found": return {
      text: status?.model ? t("The provider does not know the model {model}. Check its name in the settings.", { model: status.model })
        : t("The provider does not know the model. Check its name in the settings."),
      settings: true,
    };
    case "rate_limited": return { text: said || t("Too many requests at once. Wait a moment and try again."), retry: true };
    case "timeout": return { text: t("The model did not answer in time. A local model may still be loading: try again."), retry: true };
    case "refused": return { text: t("The model declined to answer this.") };
    case "busy": return { text: t("The previous answer for this question is still being written. Try again in a moment."), retry: true };
    case "exam_running": return { text: t("The assistant is not available during an exam. It is there again on the report.") };
    case "blocked_url": return { text: said || t("The base URL of the assistant is not allowed."), settings: true };
    case "bad_request": return { text: t("The provider did not accept the request. Check the model name in the settings."), settings: true };
    case "upstream": return { text: t("The provider failed to answer. Try again."), retry: true };
    case "too_long": return { text: t("The message is too long. A message has at most {n} characters.", { n: limit }) };
    case "not_configured": return { text: said || t("The assistant is not set up."), settings: true };
    case "bad_retry": case "no_such_message": return { text: t("This conversation has changed elsewhere. Load it again."), reload: true };
    case "network": return { text: t("kubedojo could not be reached. Check that it is still running."), retry: true };
    case "interrupted": return { text: t("The connection was lost before an answer arrived."), retry: true };
    case "history": return { text: t("The conversation could not be loaded."), reload: true };
    case "clear": return { text: t("The conversation could not be deleted."), reload: true };
    default: return { text: said || t("The assistant could not answer."), retry: true };
  }
}

export interface AssistantPanelProps {
  /** The session; null while it is being created. */
  sid: string | null | undefined;
  qid: string;
  track: Track | null;
  situation?: Situation;
  /** False while the panel is out of view: nothing is loaded then. Default true. */
  active?: boolean;
  /** A message a button of the page wants answered. */
  ask?: Ask | null;
  /** Adds a close button. */
  onClose?: () => void;
  /** What the conversation is about, where the page shows more than one question: the title of the question. */
  subject?: string;
}

/**
 * The assistant for one question: the conversation, the input, and what to do
 * when the assistant is not set up or does not answer. It fills its parent,
 * which is a column of the workspace, a pane behind a tab, or a sheet.
 */
export function AssistantPanel({ sid, qid, track, situation = "none", active = true, ask, onClose, subject }: AssistantPanelProps) {
  const config = useConfig();
  const status = config?.assistant;
  const usable = !!status?.enabled && !!status.configured;
  const chat = useAssistant(sid, qid, active && usable);
  const w = words(track);
  const [confirm, setConfirm] = useState(false);
  const input = useRef<HTMLTextAreaElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  // Whether the list follows the answer as it grows: until the learner scrolls up.
  const follow = useRef(true);
  const streaming = chat.state === "streaming";
  const { send } = chat;

  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && follow.current) el.scrollTop = el.scrollHeight;
  }, [chat.messages, chat.error, chat.state]);

  // Opened by the learner: the next thing to do is to type.
  const wasActive = useRef(active);
  useEffect(() => {
    if (active && !wasActive.current) input.current?.focus({ preventScroll: true });
    wasActive.current = active;
  }, [active]);

  useEffect(() => {
    if (!ask || ask.n <= handledAsk || !config) return;
    // One tick later, so that an effect that is run twice (React's strict mode) sends once.
    const id = window.setTimeout(() => {
      if (ask.n <= handledAsk) return;
      handledAsk = ask.n;
      if (!usable || !sid) return;
      follow.current = true;
      void send(ask.message);
    }, 0);
    return () => window.clearTimeout(id);
  }, [ask, config, usable, sid, send]);

  // The server knows better: when it says the assistant is not set up, the page asks again what is.
  const failed = chat.error?.code;
  useEffect(() => { if (failed === "not_configured") void refreshConfig().catch(() => undefined); }, [failed]);
  useEffect(() => { setConfirm(false); }, [qid]);

  const submit = (text: string) => { follow.current = true; void send(text); };
  const last = chat.messages[chat.messages.length - 1];
  const asked = new Set(chat.messages.filter((m) => m.role === "user").map((m) => m.content));
  const chips = suggestions(situation, w).filter((s) => !asked.has(s));
  // The learner's last message has no answer and none is on its way.
  const unanswered = chat.state === "idle" && last?.role === "user";

  return (
    <section aria-label={t("Assistant")} className="flex h-full min-h-0 flex-col bg-panel">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b pr-1.5 pl-4">
        <Sparkles className="size-4 shrink-0 text-accent" aria-hidden />
        <h2 className="text-sm font-semibold">{t("Assistant")}</h2>
        {usable && status && (
          <Link to="/settings" title={t("Model and provider. Open the settings.")}
            className="ml-1 min-w-0 truncate rounded-md border bg-panel2 px-2 py-0.5 text-xs text-muted hover:border-accent/60 hover:text-text">
            {[status.model, providerLabel(status.provider)].filter(Boolean).join(" · ")}
          </Link>
        )}
        <div className="ml-auto flex shrink-0 items-center">
          {usable && chat.messages.length > 0 && (
            <IconButton aria-label={t("New conversation")} disabled={streaming} onClick={() => setConfirm(true)}><MessageSquarePlus className="size-4" aria-hidden /></IconButton>
          )}
          {onClose && <IconButton aria-label={t("Close the assistant")} onClick={onClose}><X className="size-4" aria-hidden /></IconButton>}
        </div>
      </header>

      {subject && <p className="shrink-0 truncate border-b bg-panel2 px-4 py-2 text-label text-muted" title={subject}>{t("About: {title}", { title: subject })}</p>}
      {confirm && (
        <ConfirmClear onCancel={() => setConfirm(false)} onConfirm={() => { setConfirm(false); void chat.clear(); input.current?.focus(); }} />
      )}

      {!config ? (
        <Loading label={t("Loading the assistant")} className="flex-1 space-y-3 p-4">
          <Skeleton className="h-4 w-2/3" /><Skeleton className="h-4" /><Skeleton className="h-4 w-1/2" />
        </Loading>
      ) : !usable ? (
        <div className="min-h-0 flex-1 overflow-y-auto p-4"><NotAvailable status={status} /></div>
      ) : (
        <>
          <div ref={scroller} role="log" aria-live="off" aria-label={t("Conversation")} tabIndex={chat.messages.length > 0 ? 0 : undefined}
            onScroll={(e) => { const el = e.currentTarget; follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48; }}
            className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
            {chat.state === "loading" && chat.messages.length === 0 && (
              <Loading label={t("Loading the conversation")} className="space-y-3">
                <Skeleton className="ml-auto h-9 w-2/3 rounded-2xl" /><Skeleton className="h-4" /><Skeleton className="h-4" /><Skeleton className="h-4 w-1/2" />
              </Loading>
            )}
            {chat.state !== "loading" && chat.messages.length === 0 && !chat.error && (
              <div className="m-auto flex max-w-xs flex-col items-center gap-2 py-6 text-center">
                <div aria-hidden className="flex size-11 items-center justify-center rounded-full bg-accent-soft text-accent"><Sparkles className="size-5" /></div>
                <p className="text-sm font-medium">{t("Stuck, or curious why something works?")}</p>
                <p className="text-label leading-relaxed text-muted">{w.assistantSees}</p>
              </div>
            )}
            {chat.messages.map((m, i) => (m.role === "user"
              ? <div key={m.key} className="max-w-[86%] self-end rounded-2xl rounded-br-md bg-accent-soft px-3.5 py-2.5 text-sm leading-relaxed break-words whitespace-pre-wrap">{m.content}</div>
              : <Answer key={m.key} message={m} w={w} onRetry={i === chat.messages.length - 1 && chat.state === "idle" ? () => void chat.retry() : undefined} />))}
            {unanswered && (
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-label text-muted">
                <span>{t("No answer yet.")}</span>
                <Button size="sm" variant="ghost" icon={<RotateCw className="size-3.5" aria-hidden />} onClick={() => void chat.retry()}>{t("Retry")}</Button>
              </div>
            )}
            {chat.error && <FailureNote failure={chat.error} chat={chat} status={status} />}
          </div>
          <p role="status" className="sr-only">
            {chat.announce === "answering" ? t("The assistant is answering") : chat.announce === "ready" ? t("Answer ready") : ""}
          </p>
          <Composer ref={input} chips={streaming || chat.state === "loading" ? [] : chips} placeholder={w.askPlaceholder} limit={chat.limits.messageChars}
            streaming={streaming} onSend={submit} onStop={chat.stop} />
        </>
      )}
    </section>
  );
}

/** Asked in the panel itself, not in a dialog: the panel may sit in one already. */
function ConfirmClear({ onCancel, onConfirm }: { onCancel: () => void; onConfirm: () => void }) {
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => { cancel.current?.focus(); }, []);
  return (
    <div role="alertdialog" aria-label={t("Start a new conversation?")} aria-describedby="assistant-clear-note"
      onKeyDown={(e) => { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); onCancel(); } }}
      className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b bg-panel2 px-4 py-2.5">
      <p id="assistant-clear-note" className="min-w-0 flex-1 basis-40 text-label">{t("Start a new conversation? The messages of this question are deleted.")}</p>
      <div className="flex gap-1.5">
        <button ref={cancel} type="button" onClick={onCancel} className="inline-flex h-9 items-center rounded-lg border bg-panel px-3 text-label font-medium hover:border-accent/60">{t("Cancel")}</button>
        <Button size="sm" variant="danger" onClick={onConfirm}>{t("Delete")}</Button>
      </div>
    </div>
  );
}

function NotAvailable({ status }: { status: AssistantStatus | undefined }) {
  if (!status?.writable) {
    return (
      <EmptyState level={3} icon={<Sparkles />} title={t("The assistant is not available")}
        text={t("The assistant is configured by the administrator of this kubedojo.")} />
    );
  }
  return (
    <EmptyState level={3} icon={<Sparkles />} title={status.enabled ? t("The assistant is not ready yet") : t("The assistant is switched off")}
      text={status.enabled
        ? t("Its settings are incomplete. Choose a model, and a key where the provider needs one.")
        : t("Connect a model and the assistant explains a failed check from your task and your sandbox. A local model with Ollama needs no key.")}
      action={<ButtonLink to="/settings" variant="primary" icon={<Settings className="size-4" aria-hidden />}>{t("Open settings")}</ButtonLink>} />
  );
}

/** What kubedojo is doing before the model answers, by the stage it reports. */
function stageNote(stage: string | undefined): string {
  switch (stage) {
    case "checks": return t("Reading your results");
    case "snapshot": return t("Taking a read-only look at the sandbox");
    case "diagnostics": return t("Looking at the failing checks");
    default: return t("Waiting for the model");
  }
}

function Answer({ message: m, w, onRetry }: { message: ChatMessage; w: Words; onRetry?: () => void }) {
  const [copied, copy] = useCopied();
  const looked = (m.stages ?? []).map(w.lookedAt).filter(Boolean);
  const waiting = m.streaming && m.content === "";
  const stage = m.stages?.[m.stages.length - 1];
  const seconds = m.stats ? (m.stats.durationMs / 1000).toFixed(1) : "";
  return (
    <div className="flex min-w-0 flex-col gap-2.5">
      {looked.length > 0 && !waiting && (
        <span className="self-start rounded-md border bg-panel2 px-2 py-0.5 text-xs text-muted">{t("Looked at: {list}", { list: looked.join(", ") })}</span>
      )}
      {waiting ? (
        <div className="flex items-center gap-2.5 text-label text-muted"><Spinner />{stageNote(stage)}</div>
      ) : (
        <Markdown untrusted className="md-chat">{m.content}</Markdown>
      )}
      {!m.streaming && (
        <div className="flex min-h-8 flex-wrap items-center gap-x-1 gap-y-1">
          <IconButton aria-label={t("Copy answer")} className="size-8" onClick={() => copy(m.content)}>
            {copied ? <Check className="size-4 text-ok" aria-hidden /> : <Copy className="size-4" aria-hidden />}
          </IconButton>
          {onRetry && !m.partial && (
            <IconButton aria-label={t("Ask again")} className="size-8" onClick={onRetry}><RotateCw className="size-4" aria-hidden /></IconButton>
          )}
          {m.partial && <Badge tone="warn">{t("Interrupted")}</Badge>}
          {m.partial && onRetry && <Button size="sm" variant="ghost" icon={<RotateCw className="size-3.5" aria-hidden />} onClick={onRetry}>{t("Retry")}</Button>}
          {m.stats && (
            <span className="ml-1.5 text-xs tabular-nums text-muted">
              {t("{s} s", { s: seconds })}{m.stats.outputTokens > 0 && ` · ${plural(m.stats.outputTokens, "{n} token", "{n} tokens")}`}
              {m.stats.finishReason === "length" && ` · ${t("cut at the length limit")}`}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

function FailureNote({ failure, chat, status }: { failure: Failure; chat: Assistant; status: AssistantStatus | undefined }) {
  const shown = describe(failure, status, chat.limits.messageChars);
  // A refusal with Retry-After: the button waits as long as the server asked for.
  const until = failure.at + (failure.retryAfter ?? 0) * 1000;
  const [wait, setWait] = useState(() => Math.max(0, Math.ceil((until - Date.now()) / 1000)));
  useEffect(() => {
    const tick = () => setWait(Math.max(0, Math.ceil((until - Date.now()) / 1000)));
    tick();
    if (until <= Date.now()) return;
    const id = setInterval(tick, 500);
    return () => clearInterval(id);
  }, [until]);
  const settings = shown.settings && status?.writable;
  return (
    <div role="alert" className="rounded-xl border border-bad/40 bg-bad-soft px-3.5 py-3 text-sm">
      <div className="flex gap-2.5">
        <TriangleAlert className="mt-0.5 size-4 shrink-0 text-bad" aria-hidden />
        <div className="min-w-0 flex-1">
          <p className="leading-relaxed break-words">{shown.text}</p>
          {shown.settings && !status?.writable && <p className="mt-1 text-label text-muted">{t("The assistant is configured by the administrator of this kubedojo.")}</p>}
          {wait > 0 && <p className="mt-1 text-label tabular-nums text-muted">{plural(wait, "Try again in {n} second.", "Try again in {n} seconds.")}</p>}
          {(shown.retry || shown.reload || settings) && (
            <div className="mt-2.5 flex flex-wrap gap-2">
              {shown.retry && <Button size="sm" disabled={wait > 0} icon={<RotateCw className="size-3.5" aria-hidden />} onClick={() => void chat.retry()}>{t("Retry")}</Button>}
              {shown.reload && <Button size="sm" icon={<RotateCw className="size-3.5" aria-hidden />} onClick={() => void chat.reload()}>{t("Load again")}</Button>}
              {settings && <ButtonLink to="/settings" size="sm" icon={<Settings className="size-3.5" aria-hidden />}>{t("Open settings")}</ButtonLink>}
            </div>
          )}
          {failure.traceId && <p className="mt-2 font-mono text-xs text-muted select-all">{t("Trace {id}", { id: failure.traceId })}</p>}
        </div>
      </div>
    </div>
  );
}

/** Height of the input in pixels at which it stops growing and scrolls. */
const INPUT_MAX = 132;

function Composer({ ref, chips, placeholder, limit, streaming, onSend, onStop }: {
  ref: React.RefObject<HTMLTextAreaElement | null>;
  chips: string[]; placeholder: string; limit: number; streaming: boolean;
  onSend: (text: string) => void; onStop: () => void;
}) {
  const [text, setText] = useState("");
  // Characters as the server counts them, not UTF-16 units.
  const length = [...text].length;
  const over = length > limit;
  const ready = text.trim() !== "" && !over && !streaming;

  // The input is as high as its text, up to a few lines.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, INPUT_MAX)}px`;
  }, [ref, text]);

  const submit = () => {
    if (!ready) return;
    onSend(text);
    setText("");
  };
  return (
    <div className="flex shrink-0 flex-col gap-2.5 border-t px-4 pt-3 pb-3.5">
      {chips.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {chips.map((c) => (
            <button key={c} type="button" onClick={() => onSend(c)}
              className="min-h-8 rounded-2xl border bg-panel2 px-3 py-1 text-left text-label hover:border-accent/60">{c}</button>
          ))}
        </div>
      )}
      <form onSubmit={(e) => { e.preventDefault(); submit(); }}
        className={cx("flex items-end gap-2 rounded-xl border bg-bg py-1.5 pr-1.5 pl-3.5 has-[textarea:focus-visible]:border-accent has-[textarea:focus-visible]:outline-2 has-[textarea:focus-visible]:outline-offset-2 has-[textarea:focus-visible]:outline-accent",
          over && "border-bad")}>
        <textarea ref={ref} data-autofocus value={text} rows={1} placeholder={placeholder} aria-label={t("Ask the assistant")}
          aria-invalid={over || undefined} aria-describedby={over ? "assistant-count" : undefined}
          // While an answer arrives the input keeps the focus and takes no text.
          readOnly={streaming} aria-disabled={streaming || undefined}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); submit(); }
            // Escape must not close what the panel sits in while there is something to lose.
            if (e.key === "Escape" && (text !== "" || streaming)) { e.preventDefault(); e.stopPropagation(); }
          }}
          // The frame around it shows the focus.
          style={{ outline: "none" }}
          className={cx("min-h-9 min-w-0 flex-1 resize-none bg-transparent py-2 text-sm leading-5 placeholder:text-muted", streaming && "text-muted")} />
        {streaming ? (
          <button type="button" onClick={onStop} aria-label={t("Stop")} title={t("Stop")}
            className="inline-flex h-9 w-11 shrink-0 items-center justify-center rounded-lg border bg-panel2 text-text hover:border-accent/60">
            <Square className="size-3.5 fill-current" aria-hidden />
          </button>
        ) : (
          <button type="submit" disabled={!ready} aria-label={t("Send")} title={t("Send")}
            className="inline-flex h-9 w-11 shrink-0 items-center justify-center rounded-lg bg-accent text-on-accent transition hover:brightness-110 disabled:opacity-50">
            <ArrowUp className="size-4.5" aria-hidden />
          </button>
        )}
      </form>
      {length >= limit * 0.9 && (
        <p id="assistant-count" className={cx("text-right text-xs tabular-nums", over ? "font-medium text-bad" : "text-muted")}>
          {t("{n} / {max}", { n: length, max: limit })}
        </p>
      )}
      <p className="text-xs leading-relaxed text-muted">{t("Answers come from your configured model and can be wrong. Not available during an exam.")}</p>
    </div>
  );
}
