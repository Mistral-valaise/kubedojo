import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api";
import { AssistantError, type AssistantRequest, streamAssistant } from "./stream";

/** What the server asks the model when a message is empty. Shown as the learner's message in that case. */
export const DEFAULT_QUESTION = "What went wrong, and what should I look at next?";

/** One message as the panel shows it. */
export interface ChatMessage {
  /** Stable for React; the stored id is not known while a message is on its way. */
  key: string;
  /** The stored id, once the server has named it. */
  id?: number;
  role: "user" | "assistant";
  content: string;
  /** An answer that was cut short: stopped, interrupted, or ended by an error. */
  partial?: boolean;
  /** The answer that is arriving right now. */
  streaming?: boolean;
  /** What kubedojo looked at for this answer: `checks`, `snapshot`, `diagnostics`. */
  stages?: string[];
  /** From `done`: how long the answer took and how long it is. */
  stats?: { durationMs: number; outputTokens: number; model: string; finishReason: string };
}

/**
 * Why there is no answer. `code` is a code of docs/assistant.md, or one of
 * the client: `network` (kubedojo cannot be reached), `interrupted` (the
 * stream ended without an answer), `history` and `clear` (those requests
 * failed). `message` is the sentence of the server, when it sent one.
 */
export interface Failure { code: string; message: string; retryAfter?: number; at: number; traceId?: string }

export type Phase = "idle" | "loading" | "streaming" | "error";

export interface Assistant {
  messages: ChatMessage[];
  state: Phase;
  error: Failure | null;
  limits: { turns: number; messageChars: number };
  /** Provider and model of the last answer, as its `meta` named them. */
  meta: { provider: string; model: string } | null;
  /** For a screen reader: an answer is on its way, or has just been completed. */
  announce: "" | "answering" | "ready";
  /** Sends a message; an empty one asks the default question. */
  send: (text: string) => Promise<void>;
  /** Ends the answer that is arriving. What arrived stays, marked as cut short. */
  stop: () => void;
  /** Asks again: for another answer in place of the last one, or for the answer that never came. */
  retry: () => Promise<void>;
  /** Deletes the conversation of this question. */
  clear: () => Promise<void>;
  /** Loads the stored conversation again. */
  reload: () => Promise<void>;
}

let keys = 0;
const key = () => `local-${++keys}`;
const placeholder = (): ChatMessage => ({ key: key(), role: "assistant", content: "", streaming: true });
// The next frame, or a moment later where there are no frames.
const later = (fn: () => void): number => (typeof requestAnimationFrame === "function" ? requestAnimationFrame(fn) : window.setTimeout(fn, 16));
const never = (id: number) => (typeof cancelAnimationFrame === "function" ? cancelAnimationFrame(id) : window.clearTimeout(id));

/** Applies a change to the answer that is arriving, the last message. */
function patchStreaming(msgs: ChatMessage[], patch: (m: ChatMessage) => ChatMessage | null): ChatMessage[] {
  const last = msgs[msgs.length - 1];
  if (!last || !last.streaming) return msgs;
  const next = patch(last);
  return next ? [...msgs.slice(0, -1), next] : msgs.slice(0, -1);
}

/**
 * The conversation with the assistant about one question of a session: its
 * stored messages, and sending, stopping, repeating and clearing.
 *
 * Nothing is loaded while `enabled` is false (a closed panel). The state
 * resets when the question changes, and a running answer is aborted then and
 * when the component goes away.
 */
export function useAssistant(sid: string | null | undefined, qid: string, enabled = true): Assistant {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [state, setState] = useState<Phase>("idle");
  const [error, setError] = useState<Failure | null>(null);
  const [limits, setLimits] = useState({ turns: 20, messageChars: 4000 });
  const [meta, setMeta] = useState<{ provider: string; model: string } | null>(null);
  const [announce, setAnnounce] = useState<"" | "answering" | "ready">("");

  // The list as of now, for the callbacks: a state alone is one render behind.
  const list = useRef<ChatMessage[]>([]);
  const change = useCallback((fn: (msgs: ChatMessage[]) => ChatMessage[]) => {
    list.current = fn(list.current);
    setMessages(list.current);
  }, []);
  // Counts the conversations this hook has shown: what a request of an earlier one brings back is dropped.
  const generation = useRef(0);
  const controller = useRef<AbortController | null>(null);
  const busy = useRef(false);
  const history = useRef<Promise<boolean> | null>(null);
  const loadedFor = useRef<string | null>(null);
  // The conversation whose stored messages have been asked for. Until then the hook reports
  // "loading": the first render of a conversation must not look like an empty one.
  const [asked, setAsked] = useState<string | null>(null);
  // Text that arrived and is not on the page yet: it is added once per frame, not once per token.
  const pending = useRef("");
  const frame = useRef(0);

  const flush = useCallback(() => {
    if (frame.current) { never(frame.current); frame.current = 0; }
    const text = pending.current;
    if (!text) return;
    pending.current = "";
    change((msgs) => patchStreaming(msgs, (m) => ({ ...m, content: m.content + text })));
  }, [change]);
  const schedule = useCallback(() => {
    if (!frame.current) frame.current = later(() => { frame.current = 0; flush(); });
  }, [flush]);

  /** Ends the arriving answer on the page: kept as cut short if it has text, removed if it has none. */
  const cutShort = useCallback((id?: number) => {
    flush();
    let kept = false;
    change((msgs) => patchStreaming(msgs, (m) => {
      if (m.content.trim() === "") return null;
      kept = true;
      return { ...m, id, streaming: false, partial: true };
    }));
    return kept;
  }, [change, flush]);

  const load = useCallback((quiet = false): Promise<boolean> => {
    if (!sid) return Promise.resolve(false);
    const gen = generation.current;
    if (!quiet) { setState("loading"); setError(null); }
    const mine: Promise<boolean> = api.assistantHistory(sid, qid).then((h) => {
      if (gen !== generation.current) return false;
      // What only this page knows about a stored message stays with it.
      const known = new Map(list.current.filter((m) => m.id !== undefined).map((m) => [m.id, m]));
      change(() => h.messages.map((m) => ({
        key: known.get(m.id)?.key ?? `stored-${m.id}`, id: m.id, role: m.role, content: m.content, partial: m.partial || undefined,
        stages: known.get(m.id)?.stages, stats: known.get(m.id)?.stats,
      })));
      setLimits(h.limits);
      if (!quiet) setState("idle");
      return true;
    }, (e: unknown) => {
      if (gen !== generation.current) return false;
      setError({ code: "history", message: e instanceof Error ? e.message : "", at: Date.now() });
      setState("error");
      return false;
    }).finally(() => { if (history.current === mine) history.current = null; });
    history.current = mine;
    return mine;
  }, [sid, qid, change]);

  // Another question is another conversation.
  useEffect(() => {
    const reset = () => {
      generation.current++;
      controller.current?.abort();
      controller.current = null;
      busy.current = false;
      if (frame.current) { never(frame.current); frame.current = 0; }
      pending.current = "";
      history.current = null;
      loadedFor.current = null;
    };
    reset();
    list.current = [];
    setMessages([]); setError(null); setState("idle"); setMeta(null); setAnnounce("");
    return reset;
  }, [sid, qid]);
  // Loaded once per conversation, when the panel is first shown.
  useEffect(() => {
    const id = `${sid}/${qid}`;
    if (!enabled || !sid || loadedFor.current === id) return;
    loadedFor.current = id;
    setAsked(id);
    void load();
  }, [sid, qid, enabled, load]);

  /** Sends the request and shows the answer in the last message, which is the arriving one. */
  const run = useCallback(async (request: AssistantRequest, restore?: ChatMessage) => {
    if (!sid) return;
    const gen = generation.current;
    const ctrl = new AbortController();
    controller.current = ctrl;
    busy.current = true;
    setState("streaming"); setError(null); setAnnounce("answering");
    // Stopped, or another question by now: the page has moved on without this answer.
    const live = () => gen === generation.current && !ctrl.signal.aborted;
    let traceId: string | undefined;
    const fail = (f: Omit<Failure, "at" | "traceId">) => {
      setError({ ...f, at: Date.now(), traceId });
      setState("error"); setAnnounce("");
    };
    let ended = false;
    try {
      for await (const ev of streamAssistant(sid, qid, request, ctrl.signal)) {
        if (!live()) return;
        ended = ev.type === "done" || ev.type === "error" || ev.type === "interrupted";
        switch (ev.type) {
          case "meta":
            traceId = typeof ev.traceId === "string" && ev.traceId ? ev.traceId : undefined;
            setMeta({ provider: ev.provider, model: ev.model });
            // The message that is being answered is the one before the arriving answer.
            change((msgs) => msgs.map((m, i) => (i === msgs.length - 2 && m.role === "user" ? { ...m, id: ev.userMessageId } : m)));
            break;
          case "context":
            change((msgs) => patchStreaming(msgs, (m) => ({ ...m, stages: [...(m.stages ?? []), ev.stage] })));
            break;
          case "delta":
            pending.current += ev.text;
            schedule();
            break;
          case "done":
            flush();
            change((msgs) => patchStreaming(msgs, (m) => ({
              ...m, id: ev.messageId, streaming: false, partial: undefined,
              stats: { durationMs: ev.durationMs, outputTokens: ev.usage?.outputTokens ?? 0, model: ev.model, finishReason: ev.finishReason },
            })));
            setState("idle"); setAnnounce("ready");
            break;
          case "error":
            cutShort(ev.messageId);
            // Cancelled is what the server says when this page left: not a failure to show.
            if (ev.code === "canceled") { setState("idle"); setAnnounce(""); } else fail({ code: ev.code, message: ev.message });
            break;
          case "interrupted":
            if (cutShort()) { setState("idle"); setAnnounce(""); } else fail({ code: "interrupted", message: "" });
            break;
        }
      }
      // A stream that just ends is an answer that was cut off.
      if (live() && !ended) { if (cutShort()) { setState("idle"); setAnnounce(""); } else fail({ code: "interrupted", message: "" }); }
    } catch (e) {
      if (!live()) return;
      // Refused before an answer started: nothing arrives, and nothing was replaced.
      change((msgs) => patchStreaming(msgs, () => restore ?? null));
      if (e instanceof AssistantError) fail({ code: e.code, message: e.message, retryAfter: e.retryAfter });
      else fail({ code: "network", message: "" });
    } finally {
      if (controller.current === ctrl) { controller.current = null; busy.current = false; }
    }
  }, [sid, qid, change, flush, schedule, cutShort]);

  const send = useCallback(async (text: string) => {
    if (!sid || busy.current) return;
    busy.current = true;
    const gen = generation.current;
    // A message sent while the conversation is still loading goes after it.
    if (history.current) await history.current;
    if (gen !== generation.current) return;
    const message = text.trim();
    const content = message || DEFAULT_QUESTION;
    const last = list.current[list.current.length - 1];
    // A message that got no answer and is sent again is the same message, here as on the server.
    const again = last?.role === "user" && last.content === content;
    change((msgs) => [...msgs, ...(again ? [] : [{ key: key(), role: "user" as const, content }]), placeholder()]);
    await run({ message });
  }, [sid, change, run]);

  const stop = useCallback(() => {
    const ctrl = controller.current;
    if (!ctrl) return;
    ctrl.abort();
    controller.current = null;
    busy.current = false;
    cutShort();
    setState("idle"); setAnnounce("");
  }, [cutShort]);

  const retry = useCallback(async () => {
    if (!sid || busy.current) return;
    busy.current = true;
    const gen = generation.current;
    const last = () => list.current[list.current.length - 1];
    // A stopped or interrupted answer has no id here: the server knows whether it kept one, and which.
    if (last()?.role === "assistant" && last().id === undefined) {
      const ok = await load(true);
      if (gen !== generation.current) return;
      if (!ok) { busy.current = false; return; }
    }
    const target = last();
    if (!target) { busy.current = false; setState("idle"); setError(null); return; }
    if (target.role === "assistant" && target.id !== undefined) {
      change((msgs) => [...msgs.slice(0, -1), placeholder()]);
      await run({ retryOf: target.id }, target);
    } else {
      change((msgs) => [...msgs, placeholder()]);
      await run({ message: target.content });
    }
  }, [sid, change, load, run]);

  const clear = useCallback(async () => {
    if (!sid) return;
    controller.current?.abort();
    controller.current = null;
    busy.current = false;
    const gen = ++generation.current;
    if (frame.current) { never(frame.current); frame.current = 0; }
    pending.current = "";
    change(() => []);
    setError(null); setState("idle"); setAnnounce("");
    try {
      await api.assistantClear(sid, qid);
    } catch (e) {
      if (gen !== generation.current) return;
      setError({ code: "clear", message: e instanceof Error ? e.message : "", at: Date.now() });
      setState("error");
    }
  }, [sid, qid, change]);

  const reload = useCallback(async () => { await load(); }, [load]);

  // The effect that loads the conversation runs after the first render that could show it.
  const phase: Phase = state === "idle" && enabled && !!sid && asked !== `${sid}/${qid}` ? "loading" : state;
  return useMemo(() => ({ messages, state: phase, error, limits, meta, announce, send, stop, retry, clear, reload }),
    [messages, phase, error, limits, meta, announce, send, stop, retry, clear, reload]);
}
