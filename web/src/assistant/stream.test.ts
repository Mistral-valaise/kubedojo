import { describe, expect, it } from "vitest";
import { type AssistantEvent, AssistantError, parseSSE, type SSEMessage, streamAssistant } from "./stream";

const encoder = new TextEncoder();
const bytes = (text: string) => encoder.encode(text);

/** A body that delivers `chunks` one read at a time and then ends, or fails with `error`. */
function body(chunks: (string | Uint8Array)[], error?: unknown): ReadableStream<Uint8Array> {
  let i = 0;
  return new ReadableStream<Uint8Array>({
    pull(controller) {
      if (i < chunks.length) { const c = chunks[i++]; controller.enqueue(typeof c === "string" ? bytes(c) : c); return; }
      if (error) controller.error(error); else controller.close();
    },
  });
}
async function all<T>(source: AsyncIterable<T>): Promise<T[]> {
  const out: T[] = [];
  for await (const x of source) out.push(x);
  return out;
}
const parse = (...chunks: (string | Uint8Array)[]) => all(parseSSE(body(chunks)));

describe("parseSSE", () => {
  it("reads an event with its name and its data", async () => {
    expect(await parse('event: delta\ndata: {"text":"Hi"}\n\n')).toEqual([{ event: "delta", data: '{"text":"Hi"}' }]);
  });

  it("reads several events from one chunk", async () => {
    expect(await parse("event: a\ndata: 1\n\nevent: b\ndata: 2\n\nevent: c\ndata: 3\n\n")).toEqual([
      { event: "a", data: "1" }, { event: "b", data: "2" }, { event: "c", data: "3" },
    ]);
  });

  it("reads the same events wherever the chunks are cut, also inside a character", async () => {
    const text = 'event: meta\ndata: {"model":"llama3.2"}\n\n: ping\n\nevent: delta\r\ndata: {"text":"Pod läuft ✓"}\r\n\r\nevent: done\ndata: {}\n\n';
    const want: SSEMessage[] = [
      { event: "meta", data: '{"model":"llama3.2"}' }, { event: "delta", data: '{"text":"Pod läuft ✓"}' }, { event: "done", data: "{}" },
    ];
    const raw = bytes(text);
    for (let cut = 1; cut < raw.length; cut++) {
      expect(await parse(raw.slice(0, cut), raw.slice(cut)), `cut at byte ${cut}`).toEqual(want);
    }
    // One byte per chunk.
    expect(await parse(...[...raw].map((b) => new Uint8Array([b])))).toEqual(want);
  });

  it("takes LF, CRLF and CR as the end of a line", async () => {
    const want = [{ event: "delta", data: "x" }, { event: "delta", data: "y" }];
    expect(await parse("event: delta\ndata: x\n\nevent: delta\ndata: y\n\n")).toEqual(want);
    expect(await parse("event: delta\r\ndata: x\r\n\r\nevent: delta\r\ndata: y\r\n\r\n")).toEqual(want);
    expect(await parse("event: delta\rdata: x\r\revent: delta\rdata: y\r\r")).toEqual(want);
  });

  it("does not take a CRLF that two chunks share for two line ends", async () => {
    // Read as CR and LF apart, the first line end would be followed by an empty line: an event too early.
    expect(await parse("event: delta\r", "\ndata: x\r", "\n\r", "\n")).toEqual([{ event: "delta", data: "x" }]);
  });

  it("skips comment lines, between events and inside one", async () => {
    expect(await parse(": ping\n\n: ping\n\nevent: delta\n: still here\ndata: x\n\n: ping\n\n")).toEqual([{ event: "delta", data: "x" }]);
  });

  it("joins several data lines with line breaks", async () => {
    expect(await parse("event: delta\ndata: one\ndata: two\ndata:\ndata: four\n\n")).toEqual([{ event: "delta", data: "one\ntwo\n\nfour" }]);
  });

  it("strips one space after the colon and no more, and takes a line without a colon as a field without a value", async () => {
    expect(await parse("event:delta\ndata:  two spaces\n\n")).toEqual([{ event: "delta", data: " two spaces" }]);
    expect(await parse("data\n\n")).toEqual([{ event: "message", data: "" }]);
    expect(await parse("data: a: b\n\n")).toEqual([{ event: "message", data: "a: b" }]);
  });

  it("ignores fields it does not know and does not report an event without data", async () => {
    expect(await parse("id: 7\nretry: 1000\nevent: delta\ndata: x\n\nevent: lonely\n\ndata: y\n\n")).toEqual([
      { event: "delta", data: "x" }, { event: "message", data: "y" },
    ]);
  });

  it("drops an event that the end of the stream cuts off", async () => {
    expect(await parse("event: delta\ndata: x\n\nevent: delta\ndata: half")).toEqual([{ event: "delta", data: "x" }]);
    expect(await parse("event: delta\ndata: x\n")).toEqual([]);
    expect(await parse()).toEqual([]);
  });

  it("releases the stream when the reader stops early", async () => {
    let cancelled = false;
    const source = new ReadableStream<Uint8Array>({
      pull(controller) { controller.enqueue(bytes("data: x\n\n")); },
      cancel() { cancelled = true; },
    });
    for await (const msg of parseSSE(source)) { expect(msg.data).toBe("x"); break; }
    await Promise.resolve();
    expect(cancelled).toBe(true);
  });
});

describe("parseSSE with a last event", () => {
  const isDone = (msg: SSEMessage) => msg.event === "done";

  it("reads nothing after it, and lets the server close the stream itself", async () => {
    let cancelled = false;
    const source = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(bytes("event: delta\ndata: x\n\nevent: done\ndata: {}\n\nevent: delta\ndata: late\n\n"));
        // As the server does: the stream ends a moment after its last event.
        setTimeout(() => controller.close(), 20);
      },
      cancel() { cancelled = true; },
    });
    const started = Date.now();
    expect(await all(parseSSE(source, isDone))).toEqual([{ event: "delta", data: "x" }, { event: "done", data: "{}" }]);
    // Ended by the server, not given up by the reader, and without waiting out the limit.
    expect(cancelled).toBe(false);
    expect(Date.now() - started).toBeLessThan(200);
  });

  it("does not wait for long for a server that keeps the stream open", async () => {
    let cancelled = false;
    const source = new ReadableStream<Uint8Array>({
      start(controller) { controller.enqueue(bytes("event: done\ndata: {}\n\n")); },
      cancel() { cancelled = true; },
    });
    const started = Date.now();
    expect(await all(parseSSE(source, isDone))).toEqual([{ event: "done", data: "{}" }]);
    expect(Date.now() - started).toBeLessThan(1000);
    await Promise.resolve();
    expect(cancelled).toBe(true);
  });

  it("ends at once when the stream ended with its last event", async () => {
    const started = Date.now();
    expect(await all(parseSSE(body(["event: done\ndata: {}\n\n"]), isDone))).toEqual([{ event: "done", data: "{}" }]);
    expect(Date.now() - started).toBeLessThan(100);
  });
});

const sse = (...events: [string, unknown][]) => events.map(([name, data]) => `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`).join("");
const stream = (chunks: (string | Uint8Array)[], error?: unknown) =>
  new Response(body(chunks, error), { status: 200, headers: { "Content-Type": "text/event-stream" } });
const refusal = (status: number, payload: unknown, headers: Record<string, string> = {}) =>
  new Response(typeof payload === "string" ? payload : JSON.stringify(payload), { status, headers: { "Content-Type": "application/json", ...headers } });
/** A fetch that answers with `res` and notes what it was asked. */
function fetcher(res: Response | (() => Promise<Response>)) {
  const calls: { url: string; init: RequestInit }[] = [];
  const fn = (async (url: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(url), init: init ?? {} });
    return typeof res === "function" ? res() : res;
  }) as typeof fetch;
  return { fn, calls };
}
const meta = { conversation: "s1/q1", provider: "ollama", model: "llama3.2", turn: 1, userMessageId: 41 };
const done = { messageId: 42, finishReason: "stop", model: "llama3.2", usage: { inputTokens: 812, outputTokens: 164 }, durationMs: 4210 };
const failure = async (promise: Promise<unknown>): Promise<AssistantError> => {
  try { await promise; } catch (e) { return e as AssistantError; }
  throw new Error("expected a refusal");
};

describe("streamAssistant", () => {
  it("posts the message as JSON and yields the events of the answer", async () => {
    const f = fetcher(stream([sse(["meta", meta], ["context", { stage: "checks" }], ["delta", { text: "The " }]), ": ping\n\n", sse(["delta", { text: "kubelet" }], ["done", done])]));
    const controller = new AbortController();
    const events = await all(streamAssistant("s 1", "q/1", { message: "why?" }, controller.signal, f.fn));
    expect(events).toEqual<AssistantEvent[]>([
      { type: "meta", ...meta }, { type: "context", stage: "checks" }, { type: "delta", text: "The " }, { type: "delta", text: "kubelet" }, { type: "done", ...done },
    ]);
    expect(f.calls).toHaveLength(1);
    expect(f.calls[0].url).toBe("/api/sessions/s%201/questions/q%2F1/assistant");
    expect(f.calls[0].init.method).toBe("POST");
    expect((f.calls[0].init.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
    expect(JSON.parse(f.calls[0].init.body as string)).toEqual({ message: "why?" });
    expect(f.calls[0].init.signal).toBe(controller.signal);
  });

  it("asks for another answer with retryOf", async () => {
    const f = fetcher(stream([sse(["done", done])]));
    await all(streamAssistant("s1", "q1", { retryOf: 42 }, undefined, f.fn));
    expect(JSON.parse(f.calls[0].init.body as string)).toEqual({ retryOf: 42 });
  });

  it("passes an optional trace id of meta on", async () => {
    const f = fetcher(stream([sse(["meta", { ...meta, traceId: "4bf92f3577b34da6" }], ["done", done])]));
    const [first] = await all(streamAssistant("s1", "q1", { message: "" }, undefined, f.fn));
    expect(first).toMatchObject({ type: "meta", traceId: "4bf92f3577b34da6" });
  });

  it("ends with the error of the stream", async () => {
    const error = { code: "unreachable", message: "Could not reach http://localhost:11434. Is Ollama running?", retryable: true, messageId: 42 };
    const f = fetcher(stream([sse(["meta", meta], ["delta", { text: "Half" }], ["error", error], ["delta", { text: "never read" }])]));
    const events = await all(streamAssistant("s1", "q1", { message: "x" }, undefined, f.fn));
    expect(events.map((e) => e.type)).toEqual(["meta", "delta", "error"]);
    expect(events[2]).toEqual({ type: "error", ...error });
  });

  it("reports a stream that ends without done as interrupted", async () => {
    const f = fetcher(stream([sse(["meta", meta], ["delta", { text: "Half an ans" }])]));
    const events = await all(streamAssistant("s1", "q1", { message: "x" }, undefined, f.fn));
    expect(events.map((e) => e.type)).toEqual(["meta", "delta", "interrupted"]);
  });

  it("reports a connection that breaks in the middle as interrupted", async () => {
    const f = fetcher(stream([sse(["meta", meta], ["delta", { text: "Half" }])], new TypeError("network error")));
    const events = await all(streamAssistant("s1", "q1", { message: "x" }, undefined, f.fn));
    expect(events.map((e) => e.type)).toEqual(["meta", "delta", "interrupted"]);
  });

  it("skips events it does not know and data that is not JSON", async () => {
    const f = fetcher(stream(["event: delta\ndata: {not json\n\n", "event: surprise\ndata: {}\n\n", "data: {}\n\n", "event: delta\ndata: 7\n\n", sse(["delta", { text: "ok" }], ["done", done])]));
    const events = await all(streamAssistant("s1", "q1", { message: "x" }, undefined, f.fn));
    expect(events).toEqual([{ type: "delta", text: "ok" }, { type: "done", ...done }]);
  });

  it.each([
    [400, { error: "The message has more than 4000 characters.", code: "too_long" }, "too_long"],
    [400, { error: "Only the assistant's last answer can be asked for again.", code: "bad_retry" }, "bad_retry"],
    [400, { error: "invalid JSON" }, "bad_request"],
    [403, { error: "forbidden" }, "forbidden"],
    [404, { error: "unknown session" }, "not_found"],
    [404, { error: "There is no such message in this conversation.", code: "no_such_message" }, "no_such_message"],
    [409, { error: "The session is an exam that is not finished.", code: "exam_running" }, "exam_running"],
    [409, { error: "The assistant is not available. Choose a model.", code: "not_configured" }, "not_configured"],
    [409, { error: "The assistant is still answering the previous message of this question.", code: "busy" }, "busy"],
    [415, { error: "Content-Type must be application/json" }, "bad_request"],
    [500, { error: "The assistant failed. The server log has the details." }, "server"],
  ])("turns a %i refusal into a typed error", async (status, payload, code) => {
    const e = await failure(all(streamAssistant("s1", "q1", { message: "x" }, undefined, fetcher(refusal(status, payload)).fn)));
    expect(e).toBeInstanceOf(AssistantError);
    expect([e.code, e.status, e.message, e.retryAfter]).toEqual([code, status, payload.error, undefined]);
  });

  it("carries the seconds of Retry-After of a 429", async () => {
    const payload = { error: "Too many messages. Try again in a few seconds.", code: "rate_limited" };
    const e = await failure(all(streamAssistant("s1", "q1", { message: "x" }, undefined, fetcher(refusal(429, payload, { "Retry-After": "7" })).fn)));
    expect([e.code, e.status, e.retryAfter]).toEqual(["rate_limited", 429, 7]);
    // Without the header, and with one that is no number, there is no wait to show.
    const plain = await failure(all(streamAssistant("s1", "q1", { message: "x" }, undefined, fetcher(refusal(429, {})).fn)));
    expect([plain.code, plain.retryAfter]).toEqual(["rate_limited", undefined]);
    const date = await failure(all(streamAssistant("s1", "q1", { message: "x" }, undefined, fetcher(refusal(429, payload, { "Retry-After": "Wed, 21 Oct 2026 07:28:00 GMT" })).fn)));
    expect(date.retryAfter).toBeUndefined();
  });

  it("has no text for a refusal that is not the JSON of kubedojo", async () => {
    const html = new Response("<html><body>502 Bad Gateway</body></html>", { status: 502, headers: { "Content-Type": "text/html" } });
    const e = await failure(all(streamAssistant("s1", "q1", { message: "x" }, undefined, fetcher(html).fn)));
    expect([e.code, e.status, e.message]).toEqual(["server", 502, ""]);
    const odd = await failure(all(streamAssistant("s1", "q1", { message: "x" }, undefined, fetcher(refusal(409, { error: { nested: true }, code: 12 })).fn)));
    expect([odd.code, odd.message]).toEqual(["conflict", ""]);
  });

  it("does not read an answer that is not an event stream", async () => {
    const page = new Response("<html>login</html>", { status: 200, headers: { "Content-Type": "text/html" } });
    const e = await failure(all(streamAssistant("s1", "q1", { message: "x" }, undefined, fetcher(page).fn)));
    expect([e.code, e.status]).toEqual(["server", 200]);
  });

  it("says so when the server cannot be reached", async () => {
    const e = await failure(all(streamAssistant("s1", "q1", { message: "x" }, undefined, fetcher(async () => { throw new TypeError("Failed to fetch"); }).fn)));
    expect(e).toBeInstanceOf(AssistantError);
    expect([e.code, e.status]).toEqual(["network", 0]);
  });

  it("throws the abort when the request is aborted before the answer", async () => {
    const controller = new AbortController();
    const f = fetcher(() => new Promise<Response>((_resolve, reject) => {
      controller.signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
    }));
    const pending = all(streamAssistant("s1", "q1", { message: "x" }, controller.signal, f.fn));
    controller.abort();
    const e = await failure(pending);
    expect(e).not.toBeInstanceOf(AssistantError);
    expect(e.name).toBe("AbortError");
  });

  it("throws the abort when the request is aborted during the answer, and yields nothing after it", async () => {
    const controller = new AbortController();
    // As fetch does it: aborting fails the read that is waiting.
    const source = new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(bytes(sse(["meta", meta], ["delta", { text: "Half" }])));
        controller.signal.addEventListener("abort", () => c.error(new DOMException("aborted", "AbortError")));
      },
    });
    const f = fetcher(new Response(source, { status: 200, headers: { "Content-Type": "text/event-stream" } }));
    const seen: string[] = [];
    const run = (async () => { for await (const ev of streamAssistant("s1", "q1", { message: "x" }, controller.signal, f.fn)) { seen.push(ev.type); if (ev.type === "delta") controller.abort(); } })();
    const e = await failure(run);
    expect(e.name).toBe("AbortError");
    expect(seen).toEqual(["meta", "delta"]);
  });

  it("throws the abort for a stream that only ends when aborted", async () => {
    const controller = new AbortController();
    const source = new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(bytes(sse(["delta", { text: "Half" }])));
        controller.signal.addEventListener("abort", () => c.close());
      },
    });
    const f = fetcher(new Response(source, { status: 200, headers: { "Content-Type": "text/event-stream" } }));
    const seen: string[] = [];
    const run = (async () => { for await (const ev of streamAssistant("s1", "q1", { message: "x" }, controller.signal, f.fn)) { seen.push(ev.type); controller.abort(); } })();
    expect((await failure(run)).name).toBe("AbortError");
    expect(seen).toEqual(["delta"]);
  });
});
