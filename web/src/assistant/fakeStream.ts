// For tests: stands in for streamAssistant, so that a test decides what the
// assistant answers and when. Use it with
//
//   vi.mock("../assistant/stream", async (original) => ({
//     ...(await original<typeof import("../assistant/stream")>()),
//     streamAssistant: (await import("../assistant/fakeStream")).fakeStream.open,
//   }));
import type { AssistantEvent, AssistantRequest } from "./stream";

type Item = { event: AssistantEvent } | { end: true } | { error: unknown };

/** One request to the assistant, and the handles to answer it. */
export interface FakeCall {
  sid: string; qid: string; request: AssistantRequest; signal?: AbortSignal;
  /** Sends events. The stream ends by itself after `done` and `error`. */
  push: (...events: AssistantEvent[]) => void;
  /** Ends the stream without a last event, as a lost connection does. */
  end: () => void;
  /** Makes the stream throw: an AssistantError is a refusal before the answer. */
  fail: (error: unknown) => void;
}

export const meta = (over: Partial<Extract<AssistantEvent, { type: "meta" }>> = {}): AssistantEvent =>
  ({ type: "meta", conversation: "s1/q1", provider: "ollama", model: "llama3.2", turn: 1, userMessageId: 1, ...over });
export const delta = (text: string): AssistantEvent => ({ type: "delta", text });
export const context = (stage: string): AssistantEvent => ({ type: "context", stage });
export const done = (messageId = 2, over: Partial<Extract<AssistantEvent, { type: "done" }>> = {}): AssistantEvent =>
  ({ type: "done", messageId, finishReason: "stop", model: "llama3.2", usage: { inputTokens: 812, outputTokens: 164 }, durationMs: 4210, ...over });
export const failed = (code: string, message = "", messageId?: number): AssistantEvent => ({ type: "error", code, message, retryable: true, messageId });

export const fakeStream = {
  /** Every request so far, oldest first. */
  calls: [] as FakeCall[],
  /** When set, every new request is answered with these events at once. */
  answer: null as AssistantEvent[] | null,
  reset() { this.calls = []; this.answer = null; },
  /** The newest request. */
  get last(): FakeCall {
    const call = this.calls[this.calls.length - 1];
    if (!call) throw new Error("the assistant was not asked");
    return call;
  },
  open(sid: string, qid: string, request: AssistantRequest, signal?: AbortSignal): AsyncGenerator<AssistantEvent> {
    const queue: Item[] = [];
    let wake: (() => void) | null = null;
    const add = (...items: Item[]) => { queue.push(...items); wake?.(); };
    const call: FakeCall = {
      sid, qid, request, signal,
      push: (...events) => add(...events.map((event) => ({ event }))),
      end: () => add({ end: true }),
      fail: (error) => add({ error }),
    };
    fakeStream.calls.push(call);
    if (fakeStream.answer) call.push(...fakeStream.answer);
    return (async function* () {
      for (;;) {
        while (queue.length === 0) {
          // As fetch does it: an aborted request fails the read that is waiting.
          if (signal?.aborted) throw new DOMException("aborted", "AbortError");
          await new Promise<void>((resolve) => { wake = resolve; signal?.addEventListener("abort", () => resolve(), { once: true }); });
        }
        const item = queue.shift() as Item;
        if ("end" in item) return;
        if ("error" in item) throw item.error;
        yield item.event;
        if (item.event.type === "done" || item.event.type === "error" || item.event.type === "interrupted") return;
      }
    })();
  },
};
