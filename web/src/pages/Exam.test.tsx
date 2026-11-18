import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import type { QState, Session } from "../api";
import { choiceView, kcnaQuiz, questionView } from "../fixtures";

// The server side of a two-question mock exam of the theory track.
const server = {
  status: "running" as Session["status"],
  answers: {} as Record<string, string[]>,
  flagged: {} as Record<string, boolean>,
  patches: [] as { q: string; body: unknown }[],
  failSave: false,
};
const ids = ["placing-pods", "listing-pods"];
const passed = (id: string) => server.answers[id]?.join() === "b";
const finished = () => server.status === "finished";

function session(): Session {
  const score = ids.filter(passed).length;
  return {
    id: "s1", mode: "exam", track: "kcna", status: server.status, createdAt: "", durationMinutes: 45, envs: {}, log: null,
    now: new Date().toISOString(), deadline: new Date(Date.now() + 45 * 60_000).toISOString(), remainingSeconds: 45 * 60,
    questions: ids.map((id): QState => ({
      id, status: "ready", flagged: !!server.flagged[id], note: "", hintsUsed: 0, solutionViewed: false, answer: server.answers[id],
      last: finished() ? { questionId: id, score: passed(id) ? 1 : 0, max: 1, passed: passed(id), checks: [], at: "" } : undefined,
    })),
    meta: Object.fromEntries(kcnaQuiz.questions!.map((q) => [q.id, { title: q.title, domain: q.domain, points: 1, host: "" }])),
    report: finished() ? {
      score, max: 2, percent: score * 50, passPercent: 75, passed: score === 2, usedSeconds: 61,
      domains: [{ id: "d1", title: "Domain", weight: 100, score, max: 2 }],
    } : undefined,
  };
}

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    config: async () => ({ version: "test", tutor: false, user: "", maxTerminals: 1 }),
    track: async () => kcnaQuiz,
    session: async () => session(),
    question: async (_s: string, q: string) =>
      questionView(q, choiceView({ answer: server.answers[q] ?? [] }, finished()), finished() ? passed(q) : undefined),
    patchQuestion: async (_s: string, q: string, body: { answer?: string[]; flagged?: boolean }) => {
      server.patches.push({ q, body });
      if (body.answer && server.failSave) throw new Error("the exam is over");
      if (body.answer) server.answers[q] = body.answer;
      if (body.flagged !== undefined) server.flagged[q] = body.flagged;
    },
    finish: async () => { server.status = "finished"; return session(); },
  },
}));
vi.mock("../components/TerminalTabs", () => ({ TerminalTabs: () => <div>terminal</div> }));
vi.mock("../components/Terminal", () => ({ Terminal: () => <div>terminal</div> }));

import { ExamPage } from "./Exam";

// jsdom has no layout, so it has no scrolling either.
Element.prototype.scrollTo = () => undefined;

const open = () => render(
  <MemoryRouter initialEntries={["/exam/s1"]}>
    <Routes><Route path="/exam/:session" element={<ExamPage />} /><Route path="/t/:track" element={<div>track page</div>} /></Routes>
  </MemoryRouter>,
);
const pill = (name: RegExp) => within(screen.getByRole("navigation", { name: "Questions" })).getByRole("button", { name });

beforeEach(() => {
  Object.assign(server, { status: "running", answers: {}, flagged: {}, patches: [], failSave: false });
  sessionStorage.clear();
});
afterEach(cleanup);

describe("mock exam of a theory track", () => {
  it("shows the question full width: no terminal and no documentation link", async () => {
    open();
    expect(await screen.findByRole("radiogroup")).toBeTruthy();
    expect(screen.getByRole("timer")).toBeTruthy();
    expect(screen.queryByText("terminal")).toBeNull();
    expect(screen.queryByRole("separator")).toBeNull();
    expect(screen.queryByText("Docs")).toBeNull();
    // Nothing is graded or explained while the exam runs.
    expect(screen.queryByRole("button", { name: "Check answer" })).toBeNull();
    expect(screen.queryByText("Correct answer")).toBeNull();
  });

  it("saves an answer with one request per change and counts it in the navigator", async () => {
    open();
    await screen.findByRole("radiogroup");
    expect(pill(/^Question 1, not answered$/)).toBeTruthy();
    expect(screen.getByText("0 of 2 answered")).toBeTruthy();
    expect(screen.getByText("Not answered yet")).toBeTruthy();

    fireEvent.click(screen.getByRole("radio", { name: "kubelet" }));
    // Shown at once, before the server has answered.
    expect(screen.getByRole("radio", { name: "kubelet" }).getAttribute("aria-checked")).toBe("true");
    expect(pill(/^Question 1, answered$/)).toBeTruthy();
    expect(await screen.findByText("Answer saved")).toBeTruthy();
    fireEvent.keyDown(document.body, { key: "2" });
    await waitFor(() => expect(server.patches).toEqual([
      { q: "placing-pods", body: { answer: ["a"] } }, { q: "placing-pods", body: { answer: ["b"] } },
    ]));
    expect(screen.getByText("1 of 2 answered")).toBeTruthy();
    expect(pill(/^Question 2, not answered$/)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Clear answer" }));
    await waitFor(() => expect(server.answers["placing-pods"]).toEqual([]));
    expect(pill(/^Question 1, not answered$/)).toBeTruthy();
  });

  it("names a flagged question in the navigator", async () => {
    open();
    await screen.findByRole("radiogroup");
    fireEvent.click(screen.getByRole("button", { name: "Flag for review" }));
    expect(pill(/^Question 1, not answered, flagged$/)).toBeTruthy();
    fireEvent.keyDown(document.body, { key: "1" });
    expect(pill(/^Question 1, answered, flagged$/)).toBeTruthy();
  });

  it("does not answer while the learner types a note", async () => {
    open();
    await screen.findByRole("radiogroup");
    const notes = screen.getByLabelText("Notes");
    notes.focus();
    fireEvent.keyDown(notes, { key: "2" });
    expect(server.patches).toEqual([]);
    expect(screen.getByText("0 of 2 answered")).toBeTruthy();
  });

  it("says so when an answer could not be saved and shows the saved answer again", async () => {
    open();
    await screen.findByRole("radiogroup");
    fireEvent.click(screen.getByRole("radio", { name: "kubelet" }));
    await screen.findByText("Answer saved");
    server.failSave = true;
    fireEvent.click(screen.getByRole("radio", { name: "kube-proxy" }));
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("The answer to question 1 was not saved.")).toBeTruthy();
    expect(within(alert).getByText("the exam is over")).toBeTruthy();
    expect(screen.getByRole("radio", { name: "kubelet" }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("radio", { name: "kube-proxy" }).getAttribute("aria-checked")).toBe("false");
    // The next successful save clears the message.
    server.failSave = false;
    fireEvent.click(screen.getByRole("radio", { name: "kube-proxy" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(server.answers["placing-pods"]).toEqual(["d"]);
  });

  it("counts the unanswered questions before finishing, then shows the report with the review", async () => {
    open();
    await screen.findByRole("radiogroup");
    fireEvent.click(screen.getByRole("radio", { name: "kubelet" }));
    fireEvent.click(screen.getByRole("button", { name: "Finish mock exam" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("1 of 2 questions are unanswered. An unanswered question scores no points.")).toBeTruthy();
    // The number keys answer questions, not dialogs.
    fireEvent.keyDown(document.body, { key: "2" });
    expect(screen.getByRole("radio", { name: "kubelet" }).getAttribute("aria-checked")).toBe("true");
    fireEvent.click(within(dialog).getByRole("button", { name: "Finish and grade" }));

    expect(await screen.findByText("Score by domain")).toBeTruthy();
    expect(screen.getByText("0%")).toBeTruthy();
    expect(screen.getByText("Pass mark 75% · 0 of 2 questions answered correctly · time used 01:01")).toBeTruthy();
    expect(screen.getByText("Question by question")).toBeTruthy();
    // No sandbox to inspect or remove.
    expect(screen.queryByText("Sandbox")).toBeNull();
    expect(screen.queryByRole("button", { name: "Open terminal" })).toBeNull();
    expect(screen.getByRole("link", { name: "Back to KCNA" }).getAttribute("href")).toBe("/t/kcna");

    fireEvent.click(screen.getByRole("button", { name: /Placing new Pods on nodes/ }));
    expect(await screen.findByText("Which component decides where a Pod runs?")).toBeTruthy();
    expect(within(screen.getByRole("status")).getByText("Incorrect")).toBeTruthy();
    expect(screen.getByText("Your answer · incorrect")).toBeTruthy();
    expect(screen.getByText("Correct answer")).toBeTruthy();
    expect(screen.getByText("The scheduler binds Pods to nodes.")).toBeTruthy();
    // Check results, hints and the separate solution belong to tasks.
    expect(screen.queryByText(/checks passed/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Show solution" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Listing Pods/ }));
    await waitFor(() => expect(screen.getAllByRole("status")).toHaveLength(2));
    expect(within(screen.getAllByRole("status")[1]).getByText("Not answered")).toBeTruthy();
  });

  it("says when every question is answered", async () => {
    server.answers = { "placing-pods": ["b"], "listing-pods": ["b"] };
    open();
    await screen.findByRole("radiogroup");
    expect(screen.getByText("2 of 2 answered")).toBeTruthy();
    expect(screen.getByRole("radio", { name: "kube-scheduler" }).getAttribute("aria-checked")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Finish mock exam" }));
    expect(within(screen.getByRole("dialog")).getByText("Every question is answered.")).toBeTruthy();
  });
});
