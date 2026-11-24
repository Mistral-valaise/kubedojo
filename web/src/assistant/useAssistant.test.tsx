import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { AssistantHistory, AssistantMessage } from "../api";

// The server side: the stored conversations, by question.
const server = {
  stored: {} as Record<string, AssistantMessage[]>,
  historyCalls: [] as string[],
  cleared: [] as string[],
  failHistory: null as Error | null,
  failClear: null as Error | null,
};
const stored = (id: number, role: "user" | "assistant", content: string, partial = false): AssistantMessage => ({ id, role, content, at: "2026-11-24T10:00:00Z", partial });
const history = (q: string): AssistantHistory => ({ messages: server.stored[q] ?? [], limits: { turns: 20, messageChars: 4000 } });

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    assistantHistory: async (_sid: string, q: string) => {
      server.historyCalls.push(q);
      if (server.failHistory) throw server.failHistory;
      return history(q);
    },
    assistantClear: async (_sid: string, q: string) => {
      if (server.failClear) throw server.failClear;
      server.cleared.push(q);
      server.stored[q] = [];
    },
  },
}));
vi.mock("./stream", async (original) => ({
  ...(await original<typeof import("./stream")>()),
  streamAssistant: (await import("./fakeStream")).fakeStream.open,
}));

import { context, delta, done, failed, fakeStream, meta } from "./fakeStream";
import { AssistantError } from "./stream";
import { DEFAULT_QUESTION, useAssistant } from "./useAssistant";

const mount = (props: { sid?: string | null; qid?: string; enabled?: boolean } = {}) =>
  renderHook((p: { sid?: string | null; qid?: string; enabled?: boolean }) => useAssistant(p.sid === undefined ? "s1" : p.sid, p.qid ?? "q1", p.enabled ?? true), { initialProps: props });
/** What the panel would show: role and text of every message, with its marks. */
const shown = (a: ReturnType<typeof useAssistant>) => a.messages.map((m) => `${m.role}: ${m.content}${m.streaming ? " …" : ""}${m.partial ? " [cut]" : ""}`);
const idle = async (view: ReturnType<typeof mount>) => waitFor(() => expect(view.result.current.state).toBe("idle"));
const tick = () => act(async () => { await Promise.resolve(); });

beforeEach(() => {
  Object.assign(server, { stored: {}, historyCalls: [], cleared: [], failHistory: null, failClear: null });
  fakeStream.reset();
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers(); });

describe("useAssistant", () => {
  it("loads the stored conversation of the question", async () => {
    server.stored.q1 = [stored(1, "user", "Why?"), stored(2, "assistant", "Because."), stored(3, "user", "And?"), stored(4, "assistant", "Half an", true)];
    const view = mount();
    expect(view.result.current.state).toBe("loading");
    await idle(view);
    expect(shown(view.result.current)).toEqual(["user: Why?", "assistant: Because.", "user: And?", "assistant: Half an [cut]"]);
    expect(view.result.current.messages.map((m) => m.id)).toEqual([1, 2, 3, 4]);
    expect(view.result.current.limits).toEqual({ turns: 20, messageChars: 4000 });
    expect(server.historyCalls).toEqual(["q1"]);
  });

  it("loads nothing while it is not enabled, and once when it becomes so", async () => {
    server.stored.q1 = [stored(1, "user", "Why?")];
    const view = mount({ enabled: false });
    await tick();
    expect(server.historyCalls).toEqual([]);
    expect(view.result.current.state).toBe("idle");
    view.rerender({ enabled: true });
    await waitFor(() => expect(shown(view.result.current)).toEqual(["user: Why?"]));
    // Closing and opening the panel again keeps what is there.
    view.rerender({ enabled: false });
    view.rerender({ enabled: true });
    await tick();
    expect(server.historyCalls).toEqual(["q1"]);
  });

  it("reports a conversation that cannot be loaded, and loads it again on demand", async () => {
    server.failHistory = new Error("boom");
    const view = mount();
    await waitFor(() => expect(view.result.current.state).toBe("error"));
    expect(view.result.current.error).toMatchObject({ code: "history" });
    server.failHistory = null;
    server.stored.q1 = [stored(1, "user", "Why?")];
    await act(() => view.result.current.reload());
    expect(view.result.current.state).toBe("idle");
    expect(view.result.current.error).toBeNull();
    expect(shown(view.result.current)).toEqual(["user: Why?"]);
  });

  it("sends a message and shows the answer as it arrives", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("  Why is the node NotReady?  "); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    expect(shown(view.result.current)).toEqual(["user: Why is the node NotReady?", "assistant:  …"]);
    expect(view.result.current.announce).toBe("answering");
    expect(fakeStream.last).toMatchObject({ sid: "s1", qid: "q1", request: { message: "Why is the node NotReady?" } });

    await act(async () => { fakeStream.last.push(meta({ userMessageId: 41 }), context("checks"), context("snapshot")); });
    await waitFor(() => expect(view.result.current.messages[1].stages).toEqual(["checks", "snapshot"]));
    expect(view.result.current.messages[0].id).toBe(41);
    expect(view.result.current.meta).toEqual({ provider: "ollama", model: "llama3.2" });

    await act(async () => { fakeStream.last.push(delta("The kubelet "), delta("is stopped.")); });
    await waitFor(() => expect(view.result.current.messages[1].content).toBe("The kubelet is stopped."));
    expect(view.result.current.state).toBe("streaming");

    await act(async () => { fakeStream.last.push(delta(" Start it."), done(42)); });
    await idle(view);
    expect(view.result.current.messages[1]).toMatchObject({
      id: 42, role: "assistant", content: "The kubelet is stopped. Start it.", streaming: false,
      stats: { durationMs: 4210, outputTokens: 164, model: "llama3.2", finishReason: "stop" },
    });
    expect(view.result.current.messages[1].partial).toBeUndefined();
    expect(view.result.current.announce).toBe("ready");
    expect(view.result.current.error).toBeNull();
  });

  it("puts the text that arrived on the page once per frame, not once per piece", async () => {
    const frames: (() => void)[] = [];
    vi.stubGlobal("requestAnimationFrame", (fn: () => void) => frames.push(fn));
    vi.stubGlobal("cancelAnimationFrame", (id: number) => { frames[id - 1] = () => undefined; });
    let renders = 0;
    const view = renderHook(() => { renders++; return useAssistant("s1", "q1"); });
    await waitFor(() => expect(view.result.current.state).toBe("idle"));
    act(() => { void view.result.current.send("x"); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    const before = renders;
    for (const piece of ["The ", "kubelet ", "is ", "stopped", "."]) await act(async () => { fakeStream.last.push(delta(piece)); });
    await tick();
    // Five pieces, one frame asked for, nothing rendered for them yet.
    expect(frames).toHaveLength(1);
    expect(view.result.current.messages[1].content).toBe("");
    expect(renders).toBe(before);
    act(() => frames[0]());
    expect(view.result.current.messages[1].content).toBe("The kubelet is stopped.");
    expect(renders).toBe(before + 1);
    // The end does not wait for a frame.
    await act(async () => { fakeStream.last.push(delta(" Start it."), done()); });
    await waitFor(() => expect(view.result.current.state).toBe("idle"));
    expect(view.result.current.messages[1].content).toBe("The kubelet is stopped. Start it.");
  });

  it("asks the default question with an empty message", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send(""); });
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(fakeStream.last.request).toEqual({ message: "" });
    expect(shown(view.result.current)[0]).toBe(`user: ${DEFAULT_QUESTION}`);
  });

  it("sends a message after the conversation that is still loading", async () => {
    server.stored.q1 = [stored(1, "user", "Why?"), stored(2, "assistant", "Because.")];
    const view = mount();
    act(() => { void view.result.current.send("And then?"); });
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(shown(view.result.current)).toEqual(["user: Why?", "assistant: Because.", "user: And then?", "assistant:  …"]);
  });

  it("does not send twice while an answer is on its way", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("one"); void view.result.current.send("two"); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    act(() => { void view.result.current.send("three"); });
    await tick();
    expect(fakeStream.calls.map((c) => c.request)).toEqual([{ message: "one" }]);
    expect(shown(view.result.current)).toEqual(["user: one", "assistant:  …"]);
  });

  it("stops an answer: the request is aborted and what arrived stays, marked as cut short", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("x"); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    await act(async () => { fakeStream.last.push(meta(), delta("Half an ans")); });
    act(() => view.result.current.stop());
    expect(fakeStream.last.signal?.aborted).toBe(true);
    expect(view.result.current.state).toBe("idle");
    expect(view.result.current.announce).toBe("");
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Half an ans [cut]"]);
    expect(view.result.current.error).toBeNull();
    // What the stream still brings is not shown.
    await act(async () => { fakeStream.last.push(delta("wer"), done()); });
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Half an ans [cut]"]);
  });

  it("removes an answer that was stopped before any text arrived", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("x"); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    await act(async () => { fakeStream.last.push(meta(), context("checks")); });
    act(() => view.result.current.stop());
    expect(shown(view.result.current)).toEqual(["user: x"]);
    expect(view.result.current.state).toBe("idle");
  });

  it("asks again with retryOf and the id of the last answer", async () => {
    const view = mount();
    await idle(view);
    fakeStream.answer = [meta({ userMessageId: 1 }), delta("First answer."), done(2)];
    await act(() => view.result.current.send("x"));
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: First answer."]);

    fakeStream.answer = [meta({ userMessageId: 1 }), delta("Second answer."), done(3)];
    await act(() => view.result.current.retry());
    expect(fakeStream.calls.map((c) => c.request)).toEqual([{ message: "x" }, { retryOf: 2 }]);
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Second answer."]);
    expect(view.result.current.messages[1].id).toBe(3);
  });

  it("asks the server for the id of a stopped answer before it asks again", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("x"); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    await act(async () => { fakeStream.last.push(meta({ userMessageId: 7 }), delta("Half")); });
    act(() => view.result.current.stop());
    // The server kept the partial answer under an id this page never saw.
    server.stored.q1 = [stored(7, "user", "x"), stored(8, "assistant", "Half", true)];
    fakeStream.answer = [meta({ userMessageId: 7 }), delta("Whole."), done(9)];
    await act(() => view.result.current.retry());
    expect(fakeStream.last.request).toEqual({ retryOf: 8 });
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Whole."]);
  });

  it("sends the message again when the server kept nothing of a stopped answer", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("x"); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    await act(async () => { fakeStream.last.push(meta({ userMessageId: 7 }), delta("Half")); });
    act(() => view.result.current.stop());
    server.stored.q1 = [stored(7, "user", "x")];
    fakeStream.answer = [meta({ userMessageId: 7 }), delta("Whole."), done(9)];
    await act(() => view.result.current.retry());
    expect(fakeStream.last.request).toEqual({ message: "x" });
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Whole."]);
  });

  it("shows the error of the stream with its code, and keeps a partial answer under its id", async () => {
    const view = mount();
    await idle(view);
    fakeStream.answer = [meta({ userMessageId: 1, traceId: "4bf92f3577b34da6" }), delta("Half"), failed("upstream", "The provider failed.", 2)];
    await act(() => view.result.current.send("x"));
    expect(view.result.current.state).toBe("error");
    expect(view.result.current.error).toMatchObject({ code: "upstream", message: "The provider failed.", traceId: "4bf92f3577b34da6" });
    expect(view.result.current.announce).toBe("");
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Half [cut]"]);
    expect(view.result.current.messages[1].id).toBe(2);

    fakeStream.answer = [meta(), delta("Whole."), done(3)];
    await act(() => view.result.current.retry());
    expect(fakeStream.last.request).toEqual({ retryOf: 2 });
    expect(view.result.current.state).toBe("idle");
    expect(view.result.current.error).toBeNull();
  });

  it("sends the same message again after an error without an answer, and shows it once", async () => {
    const view = mount();
    await idle(view);
    fakeStream.answer = [meta({ userMessageId: 1 }), failed("unreachable", "Could not reach http://localhost:11434. Is Ollama running?")];
    await act(() => view.result.current.send(""));
    expect(view.result.current.error).toMatchObject({ code: "unreachable", message: "Could not reach http://localhost:11434. Is Ollama running?" });
    expect(view.result.current.error?.traceId).toBeUndefined();
    expect(shown(view.result.current)).toEqual([`user: ${DEFAULT_QUESTION}`]);

    fakeStream.answer = [meta({ userMessageId: 1 }), delta("Now it works."), done(2)];
    await act(() => view.result.current.retry());
    expect(fakeStream.last.request).toEqual({ message: DEFAULT_QUESTION });
    expect(shown(view.result.current)).toEqual([`user: ${DEFAULT_QUESTION}`, "assistant: Now it works."]);
    // Typed again by hand it is still the same message.
    fakeStream.answer = [failed("upstream")];
    await act(() => view.result.current.send("next"));
    fakeStream.answer = [delta("ok"), done(4)];
    await act(() => view.result.current.send("next"));
    expect(shown(view.result.current).filter((x) => x === "user: next")).toHaveLength(1);
  });

  it("reports a refusal before the answer with its code and the wait, and puts back what a retry replaced", async () => {
    const view = mount();
    await idle(view);
    fakeStream.answer = [meta(), delta("First answer."), done(2)];
    await act(() => view.result.current.send("x"));
    fakeStream.answer = null;
    act(() => { void view.result.current.retry(); });
    await waitFor(() => expect(fakeStream.calls).toHaveLength(2));
    expect(shown(view.result.current)).toEqual(["user: x", "assistant:  …"]);
    await act(async () => { fakeStream.last.fail(new AssistantError("rate_limited", "Too many messages.", 429, 7)); });
    await waitFor(() => expect(view.result.current.state).toBe("error"));
    expect(view.result.current.error).toMatchObject({ code: "rate_limited", message: "Too many messages.", retryAfter: 7 });
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: First answer."]);
    expect(view.result.current.messages[1].id).toBe(2);
  });

  it("reports a server that cannot be reached", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("x"); });
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    await act(async () => { fakeStream.last.fail(new AssistantError("network", "")); });
    await waitFor(() => expect(view.result.current.error).toMatchObject({ code: "network" }));
    expect(shown(view.result.current)).toEqual(["user: x"]);
  });

  it("takes a stream that ends without done as an interrupted answer", async () => {
    const view = mount();
    await idle(view);
    // With text: the answer stays, cut short, and there is nothing to report.
    fakeStream.answer = [meta(), delta("Half an ans"), { type: "interrupted" }];
    await act(() => view.result.current.send("x"));
    expect(view.result.current.state).toBe("idle");
    expect(view.result.current.error).toBeNull();
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Half an ans [cut]"]);
    // Without text there is nothing to keep: that is an error.
    fakeStream.answer = [meta(), { type: "interrupted" }];
    await act(() => view.result.current.send("y"));
    expect(view.result.current.error).toMatchObject({ code: "interrupted" });
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Half an ans [cut]", "user: y"]);
    // A stream that simply stops is the same.
    fakeStream.answer = null;
    act(() => { void view.result.current.send("z"); });
    await waitFor(() => expect(fakeStream.calls).toHaveLength(3));
    await act(async () => { fakeStream.last.push(delta("Some")); fakeStream.last.end(); });
    await idle(view);
    expect(shown(view.result.current)[4]).toBe("assistant: Some [cut]");
  });

  it("does not show the cancellation the server reports for a request this page ended", async () => {
    const view = mount();
    await idle(view);
    fakeStream.answer = [meta(), delta("Half"), failed("canceled", "The request was cancelled.", 2)];
    await act(() => view.result.current.send("x"));
    expect(view.result.current.state).toBe("idle");
    expect(view.result.current.error).toBeNull();
    expect(shown(view.result.current)).toEqual(["user: x", "assistant: Half [cut]"]);
  });

  it("clears the conversation", async () => {
    server.stored.q1 = [stored(1, "user", "Why?"), stored(2, "assistant", "Because.")];
    const view = mount();
    await idle(view);
    await act(() => view.result.current.clear());
    expect(server.cleared).toEqual(["q1"]);
    expect(view.result.current.messages).toEqual([]);
    expect(view.result.current.state).toBe("idle");
  });

  it("says so when the conversation could not be cleared", async () => {
    server.stored.q1 = [stored(1, "user", "Why?")];
    server.failClear = new Error("boom");
    const view = mount();
    await idle(view);
    await act(() => view.result.current.clear());
    expect(view.result.current.error).toMatchObject({ code: "clear" });
  });

  it("starts over when the question changes: the answer on its way is aborted and its events are dropped", async () => {
    server.stored.q2 = [stored(5, "user", "Other question")];
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("x"); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    const first = fakeStream.last;
    await act(async () => { first.push(meta(), delta("About q1")); });

    view.rerender({ qid: "q2" });
    expect(first.signal?.aborted).toBe(true);
    await waitFor(() => expect(shown(view.result.current)).toEqual(["user: Other question"]));
    expect(view.result.current.state).toBe("idle");
    expect(view.result.current.error).toBeNull();
    await act(async () => { first.push(delta(" more"), done()); });
    expect(shown(view.result.current)).toEqual(["user: Other question"]);
    expect(server.historyCalls).toEqual(["q1", "q2"]);
  });

  it("drops a conversation that arrives after the question has changed", async () => {
    server.stored.q1 = [stored(1, "user", "About q1")];
    server.stored.q2 = [stored(5, "user", "About q2")];
    const view = mount();
    view.rerender({ qid: "q2" });
    await waitFor(() => expect(shown(view.result.current)).toEqual(["user: About q2"]));
    await tick();
    expect(shown(view.result.current)).toEqual(["user: About q2"]);
  });

  it("aborts the answer on its way when it is unmounted", async () => {
    const view = mount();
    await idle(view);
    act(() => { void view.result.current.send("x"); });
    await waitFor(() => expect(view.result.current.state).toBe("streaming"));
    const call = fakeStream.last;
    view.unmount();
    expect(call.signal?.aborted).toBe(true);
  });

  it("does nothing without a session", async () => {
    const view = mount({ sid: null });
    await tick();
    await act(() => view.result.current.send("x"));
    await act(() => view.result.current.retry());
    expect(server.historyCalls).toEqual([]);
    expect(fakeStream.calls).toEqual([]);
    expect(view.result.current.messages).toEqual([]);
  });
});
