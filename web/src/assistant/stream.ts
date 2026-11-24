// The answer of the assistant, as it arrives: POST .../assistant answers with
// server-sent events (docs/assistant.md). EventSource cannot send a POST, so
// the stream is read from the body of a fetch.
import { retryAfterSeconds } from "../api";

export interface Usage { inputTokens: number; outputTokens: number; cacheReadTokens?: number; cacheWriteTokens?: number }

/** One event of an answer. `interrupted` is made here, for a stream that ended without `done` or `error`. */
export type AssistantEvent =
  | { type: "meta"; conversation: string; provider: string; model: string; turn: number; userMessageId: number; traceId?: string }
  | { type: "context"; stage: string }
  | { type: "delta"; text: string }
  | { type: "done"; messageId: number; finishReason: string; model: string; usage: Usage; durationMs: number }
  | { type: "error"; code: string; message: string; retryable: boolean; messageId?: number }
  | { type: "interrupted" };

/** What is sent: a message, or the id of the assistant's last answer to have it answered again. */
export type AssistantRequest = { message: string } | { retryOf: number };

/**
 * A request that was refused before an answer started. `code` is the one the
 * server names (`too_long`, `busy`, `rate_limited`, `exam_running`,
 * `not_configured`, ...), or one made from the status where it names none.
 * `retryAfter` is the Retry-After of a 429, in seconds.
 */
export class AssistantError extends Error {
  constructor(public code: string, message: string, public status = 0, public retryAfter?: number) { super(message); }
}

export interface SSEMessage { event: string; data: string }

/**
 * Reads server-sent events from a byte stream. An event may arrive in any
 * number of chunks and a chunk may hold any number of events; lines end with
 * LF, CRLF or CR; a line that starts with a colon is a comment; several
 * `data:` lines are one value with line breaks. An event that the end of the
 * stream cuts off is dropped, as the specification says.
 */
export async function* parseSSE(body: ReadableStream<Uint8Array>): AsyncGenerator<SSEMessage> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "", event = "", data: string[] = [];
  try {
    for (;;) {
      const { done, value } = await reader.read();
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
      for (;;) {
        const at = buffer.search(/[\r\n]/);
        if (at < 0) break;
        // A CR at the very end may be the first half of a CRLF: wait for the next chunk.
        if (buffer[at] === "\r" && at === buffer.length - 1 && !done) break;
        const line = buffer.slice(0, at);
        buffer = buffer.slice(at + (buffer[at] === "\r" && buffer[at + 1] === "\n" ? 2 : 1));
        if (line === "") {
          if (data.length > 0) yield { event: event || "message", data: data.join("\n") };
          event = ""; data = [];
          continue;
        }
        if (line.startsWith(":")) continue;
        const colon = line.indexOf(":");
        const field = colon < 0 ? line : line.slice(0, colon);
        let text = colon < 0 ? "" : line.slice(colon + 1);
        if (text.startsWith(" ")) text = text.slice(1);
        if (field === "event") event = text;
        else if (field === "data") data.push(text);
      }
      if (done) return;
    }
  } finally {
    // Leaving early (the consumer stopped, the request was aborted) must not keep the connection.
    reader.cancel().catch(() => undefined);
  }
}

const isAbort = (e: unknown) => e instanceof Error && e.name === "AbortError";
const NAMES = new Set(["meta", "context", "delta", "done", "error"]);

/** The code of a refusal that names none, from its status. */
function codeOf(status: number): string {
  switch (status) {
    case 400: case 415: return "bad_request";
    case 403: return "forbidden";
    case 404: return "not_found";
    case 409: return "conflict";
    case 429: return "rate_limited";
    default: return "server";
  }
}

/**
 * Sends a message to the assistant of a question and yields the events of the
 * answer. It ends after exactly one of `done`, `error` and `interrupted`.
 *
 * It throws an AssistantError for a request that is refused before the answer
 * starts, and for a server that cannot be reached (`network`). It throws the
 * AbortError of `signal` when that aborts, before or during the answer.
 * `fetchFn` is for tests.
 */
export async function* streamAssistant(
  sid: string, qid: string, request: AssistantRequest, signal?: AbortSignal, fetchFn: typeof fetch = fetch,
): AsyncGenerator<AssistantEvent> {
  let res: Response;
  try {
    res = await fetchFn(`/api/sessions/${encodeURIComponent(sid)}/questions/${encodeURIComponent(qid)}/assistant`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
      body: JSON.stringify(request),
      signal,
    });
  } catch (e) {
    if (isAbort(e) || signal?.aborted) throw e;
    throw new AssistantError("network", "", 0);
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null) as { error?: unknown; code?: unknown } | null;
    const text = typeof body?.error === "string" ? body.error : "";
    const code = typeof body?.code === "string" && body.code ? body.code : codeOf(res.status);
    throw new AssistantError(code, text, res.status, retryAfterSeconds(res));
  }
  if (!res.body || !(res.headers.get("Content-Type") ?? "").includes("text/event-stream")) {
    // Something in between answered in place of kubedojo.
    throw new AssistantError("server", "", res.status);
  }
  try {
    for await (const msg of parseSSE(res.body)) {
      if (!NAMES.has(msg.event)) continue;
      let payload: Record<string, unknown>;
      try { payload = JSON.parse(msg.data) as Record<string, unknown>; } catch { continue; }
      if (!payload || typeof payload !== "object") continue;
      const ev = { ...payload, type: msg.event } as AssistantEvent;
      yield ev;
      if (ev.type === "done" || ev.type === "error") return;
    }
  } catch (e) {
    if (isAbort(e) || signal?.aborted) throw e;
    // The connection broke in the middle of the answer.
  }
  if (signal?.aborted) throw new DOMException("The request was aborted.", "AbortError");
  yield { type: "interrupted" };
}
