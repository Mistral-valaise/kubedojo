import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import type { QState, QuestionMeta, Session, Track } from "../api";
import { choiceView, kcnaQuiz, linux, questionView } from "../fixtures";

// A skills track with six exercises: an assessment of two of them is a quick round.
const exercises: QuestionMeta[] = ["first-edit", "find-files", "e3", "e4", "e5", "e6"].map((id, i) => ({
  id, title: i === 0 ? "A first edit" : `Exercise ${i + 1}`, domain: "d1", difficulty: "easy", type: "task",
  minutes: 5, environment: "host", driver: "kind", tags: null, track: "linux", points: 2, hints: 1,
}));
const linuxTasks: Track = { ...linux, exam: { durationMinutes: 60, passPercent: 70, questions: 4 }, questions: exercises };

// The server side of a two-question mock exam of the theory track.
const server = {
  status: "running" as Session["status"],
  answers: {} as Record<string, string[]>,
  flagged: {} as Record<string, boolean>,
  patches: [] as { q: string; body: unknown }[],
  failSave: false,
  // The track of the session and its questions; `solved` grades a track of tasks.
  track: kcnaQuiz as Track, ids: ["placing-pods", "listing-pods"], solved: [] as string[],
};
const passed = (id: string) => (server.track.kind === "theory" ? server.answers[id]?.join() === "b" : server.solved.includes(id));
const finished = () => server.status === "finished";

function session(): Session {
  const ids = server.ids;
  const score = ids.filter(passed).length;
  return {
    id: "s1", mode: "exam", track: server.track.id, status: server.status, createdAt: "", durationMinutes: 45, envs: {}, log: null,
    now: new Date().toISOString(), deadline: new Date(Date.now() + 45 * 60_000).toISOString(), remainingSeconds: 45 * 60,
    questions: ids.map((id): QState => ({
      id, status: "ready", flagged: !!server.flagged[id], note: "", hintsUsed: 0, solutionViewed: false, answer: server.answers[id],
      last: finished() ? { questionId: id, score: passed(id) ? 1 : 0, max: 1, passed: passed(id), checks: [], at: "" } : undefined,
    })),
    meta: Object.fromEntries(server.track.questions!.map((q) => [q.id, { title: q.title, domain: q.domain, points: q.points, host: server.track.kind === "theory" ? "" : "linux1" }])),
    report: finished() ? {
      score, max: ids.length, percent: (score / ids.length) * 100, passPercent: 75, passed: score === ids.length, usedSeconds: 61,
      domains: [{ id: "d1", title: "Domain", weight: 100, score, max: ids.length }],
    } : undefined,
  };
}

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    config: async () => ({ version: "test", tutor: false, user: "", maxTerminals: 1 }),
    track: async () => server.track,
    session: async () => session(),
    question: async (_s: string, q: string) => (server.track.kind === "theory"
      ? questionView(q, choiceView({ answer: server.answers[q] ?? [] }, finished()), finished() ? passed(q) : undefined)
      : { question: exercises.find((x) => x.id === q)!, task: "Solve this exercise on: `ssh linux1`", host: "linux1", state: session().questions!.find((x) => x.id === q)! }),
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
  Object.assign(server, {
    status: "running", answers: {}, flagged: {}, patches: [], failSave: false,
    track: kcnaQuiz, ids: ["placing-pods", "listing-pods"], solved: [],
  });
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
    expect(within(dialog).getByText("1 of 2 questions is unanswered. An unanswered question scores no points.")).toBeTruthy();
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

describe("quick round", () => {
  // The track has more questions than the session: the session is a quick round.
  const larger = (): Track => ({ ...kcnaQuiz, exam: { ...kcnaQuiz.exam, questions: 4 }, questions: [...kcnaQuiz.questions!, ...exercises.slice(0, 3).map((q) => ({ ...q, type: "choice" as const }))] });

  it("reports a passed quick round as such, and says what counts on the path", async () => {
    Object.assign(server, { track: larger(), status: "finished", answers: { "placing-pods": ["b"], "listing-pods": ["b"] } });
    open();
    expect(await screen.findByRole("heading", { level: 1, name: "You passed this quick round." })).toBeTruthy();
    expect(screen.getByText("Only a full mock exam counts toward readiness on the path.")).toBeTruthy();
    expect(screen.queryByText("You would have passed this exam.")).toBeNull();
    expect(screen.getByText("Passed")).toBeTruthy();
  });

  it("reports a failed quick round without the points it was short", async () => {
    Object.assign(server, { track: larger(), status: "finished", answers: { "placing-pods": ["a"] } });
    open();
    expect(await screen.findByRole("heading", { level: 1, name: "Quick round not passed." })).toBeTruthy();
    expect(screen.getByText("Only a full mock exam counts toward readiness on the path.")).toBeTruthy();
    expect(screen.queryByText(/short of the pass mark/)).toBeNull();
  });

  it("keeps the headline of a full mock exam", async () => {
    Object.assign(server, { status: "finished", answers: { "placing-pods": ["b"], "listing-pods": ["b"] } });
    open();
    expect(await screen.findByRole("heading", { level: 1, name: "You would have passed this exam." })).toBeTruthy();
    expect(screen.queryByText(/counts toward readiness on the path/)).toBeNull();
  });

  it("says how far a failed full mock exam was from the pass mark", async () => {
    Object.assign(server, { status: "finished", answers: { "placing-pods": ["b"] } });
    open();
    expect(await screen.findByRole("heading", { level: 1, name: "25 percentage points short of the pass mark." })).toBeTruthy();
  });
});

describe("skills assessment", () => {
  beforeEach(() => { Object.assign(server, { track: linuxTasks, ids: ["first-edit", "find-files"] }); });

  it("calls its items exercises and offers exercise and terminal as tabs", async () => {
    open();
    expect(await screen.findByText("Exercise 1 of 2")).toBeTruthy();
    expect(screen.queryByText(/Question \d of/)).toBeNull();
    expect(screen.getByText("2 points")).toBeTruthy();
    expect(screen.getAllByRole("tab").map((x) => x.textContent)).toEqual(["Exercise", "Terminal"]);
    expect(screen.getByText("terminal")).toBeTruthy();
    // On a phone the button shows a short name and keeps the full one.
    expect(screen.getByRole("button", { name: "Finish assessment" }).textContent).toBe("FinishFinish assessment");
  });

  it("counts the flagged exercises in singular and plural", async () => {
    open();
    await screen.findByText("Exercise 1 of 2");
    fireEvent.click(screen.getByRole("button", { name: "Flag for review" }));
    fireEvent.click(screen.getByRole("button", { name: "Finish assessment" }));
    expect(within(screen.getByRole("dialog")).getByText("1 exercise is still flagged for review.")).toBeTruthy();
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Keep working" }));

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("Exercise 2 of 2");
    fireEvent.click(screen.getByRole("button", { name: "Flag for review" }));
    fireEvent.click(screen.getByRole("button", { name: "Finish assessment" }));
    expect(within(screen.getByRole("dialog")).getByText("2 exercises are still flagged for review.")).toBeTruthy();
  });

  it("reports a quick round of a skills track in its own words", async () => {
    Object.assign(server, { status: "finished", solved: ["first-edit", "find-files"] });
    open();
    expect(await screen.findByRole("heading", { level: 1, name: "You passed this quick round." })).toBeTruthy();
    expect(screen.getByText("Only a full skills assessment counts toward readiness on the path.")).toBeTruthy();
  });
});
