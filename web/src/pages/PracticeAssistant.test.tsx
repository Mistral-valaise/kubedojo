import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import type { AssistantStatus, QuestionMeta, QuestionView, Result, Track } from "../api";
import { choiceView, cka, kcnaQuiz, questionView } from "../fixtures";

// The practice pages with the assistant docked: a task with its three panes
// in every layout, and a choice question with the assistant beside it.

const task: QuestionMeta = {
  id: "fix-node", title: "Repair a node", domain: "d1", difficulty: "medium", type: "task",
  minutes: 7, environment: "base", driver: "kind", tags: null, track: "cka", points: 4, hints: 2,
};
const ckaTasks: Track = { ...cka, questions: [task] };
const ready: AssistantStatus = { enabled: true, configured: true, provider: "ollama", model: "llama3.2", writable: true };
const failedResult: Result = {
  questionId: "fix-node", score: 0, max: 4, passed: false, at: "now",
  checks: [{ id: "c", title: "Node is Ready", points: 4, passed: false, expected: 'equals "Ready"', observed: "NotReady", explain: "The kubelet must run.", command: "kubectl get node" }],
};

const server = {
  status: ready as AssistantStatus | null,
  result: failedResult,
  // The choice question: the learner's answer and whether it was graded.
  answer: [] as string[], graded: false,
  historyCalls: [] as string[],
};
const quiz = (): QuestionView =>
  questionView("placing-pods", choiceView({ answer: server.answer }, server.graded), server.graded ? server.answer.join() === "b" : undefined);

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    track: async (id: string) => (id === "kcna" ? kcnaQuiz : ckaTasks),
    config: async () => ({ version: "test", tutor: false, user: "", maxTerminals: 1, ...(server.status ? { assistant: server.status } : {}) }),
    createSession: async () => ({ id: "s1", questions: [] }),
    session: async () => ({ id: "s1", questions: [{ id: "fix-node", status: "ready" }], log: [] }),
    question: async (_s: string, q: string): Promise<QuestionView> => (q === "fix-node"
      ? { question: task, task: "Repair the kubelet.", host: "cka1", state: { id: q, status: "ready", flagged: false, note: "", hintsUsed: 0, solutionViewed: false } }
      : quiz()),
    prepare: async () => { server.answer = []; server.graded = false; },
    check: async (_s: string, q: string, answer?: string[]) => {
      if (q === "fix-node") return server.result;
      server.answer = answer ?? []; server.graded = true;
      return quiz().state.last;
    },
    assistantHistory: async (_s: string, q: string) => { server.historyCalls.push(q); return { messages: [], limits: { turns: 20, messageChars: 4000 } }; },
    assistantClear: async () => undefined,
  },
}));
vi.mock("../components/TerminalTabs", () => ({ TerminalTabs: () => <div>terminal</div> }));
vi.mock("../assistant/stream", async (original) => ({
  ...(await original<typeof import("../assistant/stream")>()),
  streamAssistant: (await import("../assistant/fakeStream")).fakeStream.open,
}));

import { delta, done, fakeStream, meta } from "../assistant/fakeStream";
import { OPEN_KEY } from "../assistant/dock";
import { resetConfigCache } from "../config";
import { PracticePage } from "./Practice";

/** jsdom has no ResizeObserver: this one reports the width of the workspace that the test sets. */
const observed = { width: 1280 };
class FakeResizeObserver {
  constructor(private callback: ResizeObserverCallback) {}
  observe() { this.callback([{ contentRect: { width: observed.width } } as ResizeObserverEntry], this as unknown as ResizeObserver); }
  unobserve() { /* one element only */ }
  disconnect() { /* nothing to release */ }
}

const open = (path: string) => render(
  <MemoryRouter initialEntries={[path]}>
    <Routes>
      <Route path="/t/:track/q/:question" element={<PracticePage />} />
      <Route path="/settings" element={<div>settings page</div>} />
    </Routes>
  </MemoryRouter>,
);
const openTask = async (width: number) => {
  observed.width = width;
  open("/t/cka/q/fix-node");
  await screen.findByRole("button", { name: "Check my solution" });
  // The configuration decides whether there is an assistant to dock.
  await act(async () => { await Promise.resolve(); });
};
const toggle = () => screen.getByRole("button", { name: "Assistant" });
const layout = () => document.querySelector<HTMLElement>("[data-layout]")?.dataset.layout;
const pane = (id: string) => document.getElementById(`split-pane-${id}`) as HTMLElement;
/** The panes in view: out of view they are hidden by visibility and stay mounted. */
const inView = () => ["left", "right", "assistant"].filter((id) => pane(id) && !pane(id).className.split(" ").includes("invisible"));
const input = () => screen.getByRole("textbox", { name: "Ask the assistant" });

// jsdom has no layout, so it has no scrolling either.
Element.prototype.scrollTo = () => undefined;
Element.prototype.scrollIntoView = () => undefined;

beforeEach(() => {
  Object.assign(server, { status: ready, result: failedResult, answer: [], graded: false, historyCalls: [] });
  fakeStream.reset();
  resetConfigCache();
  localStorage.clear();
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("the assistant on a task", () => {
  it("is a third column on a wide screen, opened by the button in the header", async () => {
    await openTask(1280);
    expect(toggle().getAttribute("aria-pressed")).toBe("false");
    expect(layout()).toBe("two");
    expect(inView()).toEqual(["left", "right"]);
    // Closed, it has asked the server for nothing.
    expect(server.historyCalls).toEqual([]);

    fireEvent.click(toggle());
    expect(toggle().getAttribute("aria-pressed")).toBe("true");
    expect(layout()).toBe("three");
    expect(inView()).toEqual(["left", "right", "assistant"]);
    expect(localStorage.getItem(OPEN_KEY)).toBe("1");
    await waitFor(() => expect(server.historyCalls).toEqual(["fix-node"]));
    await waitFor(() => expect(document.activeElement).toBe(input()));
    expect(within(pane("assistant")).getByRole("link", { name: "llama3.2 · Ollama" })).toBeTruthy();

    fireEvent.click(toggle());
    expect(toggle().getAttribute("aria-pressed")).toBe("false");
    expect(layout()).toBe("two");
    expect(localStorage.getItem(OPEN_KEY)).toBe("0");
    // The terminal and the task were never unmounted for it.
    expect(screen.getByText("terminal")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Check my solution" })).toBeTruthy();
  });

  it("takes the place of the task on a medium screen, and the terminal stays in view", async () => {
    await openTask(900);
    expect(inView()).toEqual(["left", "right"]);
    fireEvent.click(toggle());
    expect(layout()).toBe("two");
    // Opening it also selects it: the left column shows the assistant now.
    expect(inView()).toEqual(["right", "assistant"]);
    const switcher = screen.getByRole("tablist", { name: "Left column" });
    expect(within(switcher).getAllByRole("tab").map((x) => `${x.textContent} ${x.getAttribute("aria-selected")}`)).toEqual(["Task false", "Assistant true"]);
    fireEvent.click(within(switcher).getByRole("tab", { name: "Task" }));
    expect(inView()).toEqual(["left", "right"]);
    // Still open: the button says so, and the way back is the switcher.
    expect(toggle().getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(within(switcher).getByRole("tab", { name: "Assistant" }));
    expect(inView()).toEqual(["right", "assistant"]);

    fireEvent.click(toggle());
    expect(inView()).toEqual(["left", "right"]);
    expect(screen.queryByRole("tablist", { name: "Left column" })).toBeNull();
  });

  it("is a tab of its own on a phone, selected when it opens", async () => {
    await openTask(390);
    expect(layout()).toBe("tabs");
    expect(screen.getAllByRole("tab").map((x) => x.textContent)).toEqual(["Task", "Terminal"]);
    fireEvent.click(toggle());
    expect(screen.getAllByRole("tab").map((x) => `${x.textContent} ${x.getAttribute("aria-selected")}`)).toEqual(["Task false", "Terminal false", "Assistant true"]);
    expect(inView()).toEqual(["assistant"]);
    fireEvent.click(screen.getByRole("tab", { name: "Terminal" }));
    expect(inView()).toEqual(["right"]);
    fireEvent.click(toggle());
    expect(screen.getAllByRole("tab").map((x) => x.textContent)).toEqual(["Task", "Terminal"]);
    // The learner was in the terminal and stays there.
    expect(inView()).toEqual(["right"]);
  });

  it("closes from its own button and hands the focus back to the header", async () => {
    await openTask(900);
    fireEvent.click(toggle());
    fireEvent.click(screen.getByRole("button", { name: "Close the assistant" }));
    expect(toggle().getAttribute("aria-pressed")).toBe("false");
    expect(inView()).toEqual(["left", "right"]);
    expect(document.activeElement).toBe(toggle());
  });

  it("is open again on the next visit, with the task in view", async () => {
    localStorage.setItem(OPEN_KEY, "1");
    await openTask(900);
    expect(toggle().getAttribute("aria-pressed")).toBe("true");
    expect(inView()).toEqual(["left", "right"]);
    expect(screen.getByRole("tablist", { name: "Left column" })).toBeTruthy();
    await waitFor(() => expect(server.historyCalls).toEqual(["fix-node"]));
  });

  it.each([[1280, ["left", "right", "assistant"]], [900, ["right", "assistant"]], [390, ["assistant"]]] as const)(
    "explains a failed check at %i px: the panel opens, comes into view and asks the default question", async (width, want) => {
      await openTask(width);
      expect(screen.queryByRole("button", { name: "Explain what went wrong" })).toBeNull();
      fireEvent.click(screen.getByRole("button", { name: "Check my solution" }));
      fireEvent.click(await screen.findByRole("button", { name: "Explain what went wrong" }));

      expect(toggle().getAttribute("aria-pressed")).toBe("true");
      expect(inView()).toEqual(want);
      await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
      // An empty message: the server asks its default question, and the panel shows it.
      expect(fakeStream.last).toMatchObject({ sid: "s1", qid: "fix-node", request: { message: "" } });
      const log = within(pane("assistant")).getByRole("log");
      expect(within(log).getByText("What went wrong, and what should I look at next?")).toBeTruthy();
      await act(async () => { fakeStream.last.push(meta(), delta("The kubelet is stopped."), done()); });
      expect(await within(log).findByText("The kubelet is stopped.")).toBeTruthy();
      // Asked once, however often the page renders.
      expect(fakeStream.calls).toHaveLength(1);
      // What is left to suggest fits a failed check.
      expect(within(pane("assistant")).getByRole("button", { name: "Give me a hint, not the solution" })).toBeTruthy();
    });

  it("suggests what fits the state of the task", async () => {
    await openTask(1280);
    fireEvent.click(toggle());
    expect(await screen.findByRole("button", { name: "Where should I start?" })).toBeTruthy();
    server.result = { ...failedResult, passed: true, score: 4, checks: [{ ...failedResult.checks[0], passed: true }] };
    fireEvent.click(screen.getByRole("button", { name: "Check my solution" }));
    expect(await screen.findByRole("button", { name: "What should I remember from this task?" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Where should I start?" })).toBeNull();
    // Nothing failed, so there is nothing to explain.
    expect(screen.queryByRole("button", { name: "Explain what went wrong" })).toBeNull();
  });

  it("shows how to set the assistant up where it is switched off, and offers no explanation it cannot give", async () => {
    server.status = { ...ready, enabled: false, configured: false };
    await openTask(1280);
    fireEvent.click(screen.getByRole("button", { name: "Check my solution" }));
    await screen.findByText("0 of 1 checks passed");
    expect(screen.queryByRole("button", { name: "Explain what went wrong" })).toBeNull();
    fireEvent.click(toggle());
    expect(await screen.findByRole("heading", { name: "The assistant is switched off" })).toBeTruthy();
    fireEvent.click(screen.getByRole("link", { name: "Open settings" }));
    expect(await screen.findByText("settings page")).toBeTruthy();
  });

  it.each([
    ["a hosted server whose assistant is off", { ...ready, enabled: false, configured: false, writable: false }],
    ["a server without an assistant", null],
  ] as const)("has no assistant on %s: two panes, as before", async (_name, status) => {
    server.status = status;
    localStorage.setItem(OPEN_KEY, "1");
    await openTask(1280);
    expect(screen.queryByRole("button", { name: "Assistant" })).toBeNull();
    expect(pane("assistant")).toBeNull();
    expect(layout()).toBeUndefined();
    expect(screen.getAllByRole("tab").map((x) => x.textContent)).toEqual(["Task", "Terminal"]);
  });

  it("shows the assistant of a hosted server that has one", async () => {
    server.status = { ...ready, writable: false };
    await openTask(1280);
    fireEvent.click(toggle());
    expect(await screen.findByRole("textbox", { name: "Ask the assistant" })).toBeTruthy();
  });
});

describe("the assistant on a choice question", () => {
  const openQuiz = async () => {
    open("/t/kcna/q/placing-pods");
    await screen.findByRole("radiogroup");
    await act(async () => { await Promise.resolve(); });
  };
  const question = () => document.getElementById("quiz-pane-question") as HTMLElement;
  const aside = () => document.getElementById("quiz-pane-assistant") as HTMLElement;
  const classes = (el: HTMLElement) => el.className.split(" ");

  it("is a column beside the question, and a tab on a narrow screen", async () => {
    await openQuiz();
    expect(toggle().getAttribute("aria-pressed")).toBe("false");
    expect(classes(aside())).toContain("hidden");
    expect(screen.queryByRole("tablist")).toBeNull();
    expect(server.historyCalls).toEqual([]);

    fireEvent.click(toggle());
    expect(toggle().getAttribute("aria-pressed")).toBe("true");
    expect(classes(aside())).not.toContain("hidden");
    // From `lg` both are in view; below it the tabs decide, and opening selected the assistant.
    expect(classes(aside())).not.toContain("max-lg:invisible");
    expect(classes(question())).toContain("max-lg:invisible");
    const tabs = screen.getByRole("tablist", { name: "Panes" });
    expect(classes(tabs)).toContain("lg:hidden");
    expect(within(tabs).getAllByRole("tab").map((x) => `${x.textContent} ${x.getAttribute("aria-selected")}`)).toEqual(["Question false", "Assistant true"]);
    await waitFor(() => expect(server.historyCalls).toEqual(["placing-pods"]));
    // The words are those of a theory track.
    expect(input().getAttribute("placeholder")).toBe("Ask about this question");

    fireEvent.click(within(tabs).getByRole("tab", { name: "Question" }));
    expect(classes(question())).not.toContain("max-lg:invisible");
    expect(classes(aside())).toContain("max-lg:invisible");
    // The question stays mounted behind the tab either way.
    expect(screen.getByRole("radiogroup")).toBeTruthy();

    fireEvent.click(toggle());
    expect(classes(aside())).toContain("hidden");
    expect(screen.queryByRole("tablist")).toBeNull();
    expect(classes(question())).not.toContain("max-lg:invisible");
  });

  it("keeps what is typed into the assistant from the number keys of the question", async () => {
    await openQuiz();
    fireEvent.click(toggle());
    await waitFor(() => expect(document.activeElement).toBe(input()));
    fireEvent.keyDown(input(), { key: "2" });
    fireEvent.keyDown(input(), { key: "Enter" });
    expect(screen.getAllByRole("radio").every((o) => o.getAttribute("aria-checked") === "false")).toBe(true);
    expect(screen.queryByText("Incorrect")).toBeNull();
    // From the page the same key picks the second option.
    fireEvent.keyDown(document.body, { key: "2" });
    expect(screen.getByRole("radio", { name: "kube-scheduler" }).getAttribute("aria-checked")).toBe("true");
  });

  it("explains a wrong answer from its review", async () => {
    await openQuiz();
    expect(screen.queryByRole("button", { name: "Explain" })).toBeNull();
    fireEvent.keyDown(document.body, { key: "1" });
    fireEvent.keyDown(document.body, { key: "Enter" });
    fireEvent.click(await screen.findByRole("button", { name: "Explain" }));

    expect(toggle().getAttribute("aria-pressed")).toBe("true");
    expect(classes(aside())).not.toContain("hidden");
    expect(screen.getByRole("tab", { name: "Assistant" }).getAttribute("aria-selected")).toBe("true");
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(fakeStream.last).toMatchObject({ sid: "s1", qid: "placing-pods", request: { message: "Why is that the right answer?" } });
    await act(async () => { fakeStream.last.push(meta(), delta("The scheduler decides."), done()); });
    expect(await within(aside()).findByText("The scheduler decides.")).toBeTruthy();
    // It was asked already, so it is not suggested again.
    expect(within(aside()).queryByRole("button", { name: "Why is that the right answer?" })).toBeNull();
  });

  it("offers no explanation for a right answer, and suggests the question to ask instead", async () => {
    await openQuiz();
    fireEvent.keyDown(document.body, { key: "2" });
    fireEvent.keyDown(document.body, { key: "Enter" });
    await screen.findByText("Correct");
    expect(screen.queryByRole("button", { name: "Explain" })).toBeNull();
    fireEvent.click(toggle());
    expect(await within(aside()).findByRole("button", { name: "Why is that the right answer?" })).toBeTruthy();
  });

  it("has no assistant where the server has none", async () => {
    server.status = null;
    await openQuiz();
    expect(screen.queryByRole("button", { name: "Assistant" })).toBeNull();
    expect(aside()).toBeNull();
    fireEvent.keyDown(document.body, { key: "1" });
    fireEvent.keyDown(document.body, { key: "Enter" });
    await screen.findByText("Incorrect");
    expect(screen.queryByRole("button", { name: "Explain" })).toBeNull();
  });
});
