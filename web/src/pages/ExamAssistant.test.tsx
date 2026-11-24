import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import type { AssistantStatus, QState, QuestionMeta, Result, Session, Track } from "../api";
import { choiceView, kcnaQuiz, linux, questionView } from "../fixtures";

// The assistant and an exam: none while the exam runs, and one per question on the report.

const exercises: QuestionMeta[] = ["first-edit", "find-files"].map((id, i) => ({
  id, title: i === 0 ? "A first edit" : "Find files", domain: "d1", difficulty: "easy", type: "task",
  minutes: 5, environment: "host", driver: "kind", tags: null, track: "linux", points: 2, hints: 1,
}));
const linuxTasks: Track = { ...linux, exam: { durationMinutes: 60, passPercent: 70, questions: 2 }, questions: exercises };
const ready: AssistantStatus = { enabled: true, configured: true, provider: "ollama", model: "llama3.2", writable: true };

const server = {
  status: ready as AssistantStatus | null,
  finished: true,
  track: linuxTasks as Track,
  // Choice questions: the learner's answers. Tasks: none is solved.
  answers: { "placing-pods": ["a"], "listing-pods": ["b"] } as Record<string, string[]>,
  historyCalls: [] as string[],
};
const theory = () => server.track.kind === "theory";
const passed = (id: string) => theory() && server.answers[id]?.join() === "b";
const resultOf = (id: string): Result => ({
  questionId: id, score: passed(id) ? 1 : 0, max: theory() ? 1 : 2, passed: passed(id), at: "",
  checks: theory() ? [] : [{ id: "c", title: "The file is saved", points: 2, passed: false, expected: 'equals "yes"', observed: "no", explain: "Save with `:w`.", command: "cat file" }],
});
function session(): Session {
  const ids = server.track.questions!.map((q) => q.id);
  return {
    id: "s1", mode: "exam", track: server.track.id, status: server.finished ? "finished" : "running", createdAt: "", durationMinutes: 45, envs: {}, log: null,
    now: new Date().toISOString(), deadline: new Date(Date.now() + 45 * 60_000).toISOString(), remainingSeconds: 45 * 60,
    questions: ids.map((id): QState => ({
      id, status: "ready", flagged: false, note: "", hintsUsed: 0, solutionViewed: false, answer: server.answers[id],
      last: server.finished ? resultOf(id) : undefined,
    })),
    meta: Object.fromEntries(server.track.questions!.map((q) => [q.id, { title: q.title, domain: q.domain, points: q.points, host: theory() ? "" : "linux1" }])),
    report: server.finished ? { score: 1, max: 2, percent: 50, passPercent: 70, passed: false, usedSeconds: 61, domains: [{ id: "d1", title: "Domain", weight: 100, score: 1, max: 2 }] } : undefined,
  };
}

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    config: async () => ({ version: "test", tutor: false, user: "", maxTerminals: 1, ...(server.status ? { assistant: server.status } : {}) }),
    track: async () => server.track,
    session: async () => session(),
    question: async (_s: string, q: string) => (theory()
      ? questionView(q, choiceView({ answer: server.answers[q] ?? [] }, server.finished), server.finished ? passed(q) : undefined)
      : { question: exercises.find((x) => x.id === q)!, task: "Edit the file on `linux1`.", host: "linux1", state: session().questions!.find((x) => x.id === q)! }),
    patchQuestion: async () => undefined,
    assistantHistory: async (_s: string, q: string) => { server.historyCalls.push(q); return { messages: [], limits: { turns: 20, messageChars: 4000 } }; },
    assistantClear: async () => undefined,
  },
}));
vi.mock("../components/TerminalTabs", () => ({ TerminalTabs: () => <div>terminal</div> }));
vi.mock("../components/Terminal", () => ({ Terminal: () => <div>terminal</div> }));
vi.mock("../assistant/stream", async (original) => ({
  ...(await original<typeof import("../assistant/stream")>()),
  streamAssistant: (await import("../assistant/fakeStream")).fakeStream.open,
}));

import { delta, done, fakeStream, meta } from "../assistant/fakeStream";
import { resetConfigCache } from "../config";
import { ExamPage } from "./Exam";

Element.prototype.scrollTo = () => undefined;
Element.prototype.scrollIntoView = () => undefined;

/** Makes the window as wide as the report needs for a docked assistant, or not. */
const wide = (matches: boolean) => vi.stubGlobal("matchMedia", (query: string) => ({
  matches: matches && query === "(min-width: 1100px)", media: query, addEventListener: () => undefined, removeEventListener: () => undefined,
}));
const openExam = async () => {
  render(<MemoryRouter initialEntries={["/exam/s1"]}><Routes><Route path="/exam/:session" element={<ExamPage />} /></Routes></MemoryRouter>);
  await act(async () => { await Promise.resolve(); });
};
const expand = async (title: RegExp) => {
  fireEvent.click(await screen.findByRole("button", { name: title }));
  return (await screen.findByRole("button", { name: title })).closest("div.rounded-2xl") as HTMLElement;
};
const input = () => screen.getByRole("textbox", { name: "Ask the assistant" });

beforeEach(() => {
  Object.assign(server, { status: ready, finished: true, track: linuxTasks, answers: { "placing-pods": ["a"], "listing-pods": ["b"] }, historyCalls: [] });
  fakeStream.reset();
  resetConfigCache();
  localStorage.clear();
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("the assistant during an exam", () => {
  it.each([["a task exam", linuxTasks], ["a mock exam", kcnaQuiz]] as const)("is not there at all in %s", async (_name, track) => {
    server.finished = false;
    server.track = track;
    // Even for a learner who had it open in practice.
    localStorage.setItem("kubedojo.assistant.open", "1");
    await openExam();
    await screen.findByRole("timer");
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByRole("button", { name: "Assistant" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Ask the assistant|Explain/ })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Ask the assistant" })).toBeNull();
    expect(screen.queryByText("Assistant")).toBeNull();
    expect(server.historyCalls).toEqual([]);
    expect(fakeStream.calls).toEqual([]);
  });
});

describe("the assistant on the report of an exam", () => {
  it("opens for a question as a sheet over the report on a small screen", async () => {
    await openExam();
    const card = await expand(/A first edit/);
    const askButton = await within(card).findByRole("button", { name: "Ask the assistant" });
    expect(askButton.getAttribute("aria-pressed")).toBe("false");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(server.historyCalls).toEqual([]);

    fireEvent.click(askButton);
    const sheet = screen.getByRole("dialog", { name: "Assistant" });
    expect(within(sheet).getByRole("heading", { name: "Assistant" })).toBeTruthy();
    expect(within(sheet).getByText("About: A first edit")).toBeTruthy();
    await waitFor(() => expect(server.historyCalls).toEqual(["first-edit"]));
    // The keyboard starts in the input, not on the first link of the panel.
    await waitFor(() => expect(document.activeElement).toBe(input()));
    // Nothing is asked until the learner asks.
    expect(fakeStream.calls).toEqual([]);
    expect(within(sheet).getByRole("button", { name: "What went wrong, and what should I look at next?" })).toBeTruthy();
    expect(input().getAttribute("placeholder")).toBe("Ask about this exercise");

    fireEvent.click(within(sheet).getByRole("button", { name: "Close the assistant" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(askButton.getAttribute("aria-pressed")).toBe("false");
  });

  it("explains a failed check: the sheet opens and asks the default question", async () => {
    await openExam();
    const card = await expand(/Find files/);
    fireEvent.click(await within(card).findByRole("button", { name: "Explain what went wrong" }));
    const sheet = screen.getByRole("dialog", { name: "Assistant" });
    expect(within(sheet).getByText("About: Find files")).toBeTruthy();
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(fakeStream.last).toMatchObject({ sid: "s1", qid: "find-files", request: { message: "" } });
    await act(async () => { fakeStream.last.push(meta(), delta("The file was never written."), done()); });
    expect(await within(sheet).findByText("The file was never written.")).toBeTruthy();
    expect(fakeStream.calls).toHaveLength(1);
  });

  it("is a column beside the report from 1100 px, and follows the question that is asked about", async () => {
    wide(true);
    await openExam();
    const first = await expand(/A first edit/);
    fireEvent.click(await within(first).findByRole("button", { name: "Ask the assistant" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    const column = screen.getByRole("complementary");
    expect(within(column).getByText("About: A first edit")).toBeTruthy();
    expect(within(first).getByRole("button", { name: "Ask the assistant" }).getAttribute("aria-pressed")).toBe("true");
    await waitFor(() => expect(server.historyCalls).toEqual(["first-edit"]));
    // The report is still there, next to it.
    expect(screen.getByRole("heading", { level: 1 })).toBeTruthy();

    const second = await expand(/Find files/);
    fireEvent.click(await within(second).findByRole("button", { name: "Explain what went wrong" }));
    expect(within(column).getByText("About: Find files")).toBeTruthy();
    expect(within(first).getByRole("button", { name: "Ask the assistant" }).getAttribute("aria-pressed")).toBe("false");
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(fakeStream.last).toMatchObject({ qid: "find-files", request: { message: "" } });
    expect(server.historyCalls).toEqual(["first-edit", "find-files"]);

    fireEvent.click(within(column).getByRole("button", { name: "Close the assistant" }));
    expect(screen.queryByRole("complementary")).toBeNull();
  });

  it("explains a wrong answer of a mock exam, and only a wrong one", async () => {
    server.track = kcnaQuiz;
    await openExam();
    const right = await expand(/Listing Pods/);
    await within(right).findByText("Correct");
    expect(within(right).queryByRole("button", { name: "Explain" })).toBeNull();
    expect(within(right).getByRole("button", { name: "Ask the assistant" })).toBeTruthy();

    const wrong = await expand(/Placing new Pods on nodes/);
    fireEvent.click(await within(wrong).findByRole("button", { name: "Explain" }));
    const sheet = screen.getByRole("dialog", { name: "Assistant" });
    await waitFor(() => expect(fakeStream.calls).toHaveLength(1));
    expect(fakeStream.last).toMatchObject({ qid: "placing-pods", request: { message: "Why is that the right answer?" } });
    expect(within(sheet).getByText("About: Placing new Pods on nodes")).toBeTruthy();
  });

  it("offers the way to the settings where the assistant is switched off, and nothing it cannot do", async () => {
    server.status = { ...ready, enabled: false, configured: false };
    await openExam();
    const card = await expand(/A first edit/);
    await within(card).findByText("0 of 1 checks passed");
    expect(within(card).queryByRole("button", { name: "Explain what went wrong" })).toBeNull();
    fireEvent.click(within(card).getByRole("button", { name: "Ask the assistant" }));
    expect(within(screen.getByRole("dialog")).getByRole("heading", { name: "The assistant is switched off" })).toBeTruthy();
  });

  it("has no assistant where the server has none", async () => {
    server.status = null;
    await openExam();
    const card = await expand(/A first edit/);
    await within(card).findByText("0 of 1 checks passed");
    expect(within(card).queryByRole("button", { name: /Ask the assistant|Explain/ })).toBeNull();
    // Hints and the solution are there as before.
    expect(within(card).getByRole("button", { name: "Show solution" })).toBeTruthy();
  });
});
