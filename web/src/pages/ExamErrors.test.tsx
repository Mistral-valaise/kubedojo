import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import type { QState, QuestionMeta, Session, Track } from "../api";
import { linux } from "../fixtures";

// What the exam views do when a request fails: nothing may fail silently, and
// nothing may wait forever.

const exercises: QuestionMeta[] = ["first-edit", "find-files"].map((id, i) => ({
  id, title: i === 0 ? "A first edit" : "Find files", domain: "d1", difficulty: "easy", type: "task",
  minutes: 5, environment: "host", driver: "kind", tags: null, track: "linux", points: 2, hints: 0,
}));
const linuxTasks: Track = { ...linux, exam: { durationMinutes: 60, passPercent: 70, questions: 2 }, questions: exercises };

const server = {
  status: "running" as Session["status"],
  fail: { track: false, question: false, note: false, flag: false },
  patches: [] as unknown[],
  trackRequests: 0,
};

function session(): Session {
  const done = server.status === "finished";
  return {
    id: "s1", mode: "exam", track: "linux", status: server.status, createdAt: "", durationMinutes: 60, envs: { host: "linux1" }, log: null,
    now: new Date().toISOString(), deadline: new Date(Date.now() + 60 * 60_000).toISOString(), remainingSeconds: 60 * 60,
    questions: exercises.map((q): QState => ({
      id: q.id, status: "ready", flagged: false, note: "", hintsUsed: 0, solutionViewed: false,
      last: done ? { questionId: q.id, score: 0, max: 2, passed: false, checks: [], at: "" } : undefined,
    })),
    meta: Object.fromEntries(exercises.map((q) => [q.id, { title: q.title, domain: q.domain, points: q.points, host: "linux1" }])),
    report: done ? { score: 0, max: 4, percent: 0, passPercent: 70, passed: false, usedSeconds: 60, domains: [] } : undefined,
  };
}

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    config: async () => ({ version: "test", tutor: false, user: "", maxTerminals: 1 }),
    track: async () => { server.trackRequests++; if (server.fail.track) throw new Error("track store down"); return linuxTasks; },
    session: async () => session(),
    question: async (_s: string, q: string) => {
      if (server.fail.question) throw new Error("question store down");
      return { question: exercises.find((x) => x.id === q)!, task: `Text of ${q}.`, host: "linux1", state: session().questions!.find((x) => x.id === q)! };
    },
    patchQuestion: async (_s: string, q: string, body: { note?: string; flagged?: boolean }) => {
      if (body.note !== undefined && server.fail.note) throw new Error("no space left");
      if (body.flagged !== undefined && server.fail.flag) throw new Error("no space left");
      server.patches.push({ q, ...body });
    },
    rate: async () => ({}),
  },
}));
vi.mock("../components/TerminalTabs", () => ({ TerminalTabs: () => <div>terminal</div> }));
vi.mock("../components/Terminal", () => ({ Terminal: () => <div>terminal</div> }));

import { ExamPage } from "./Exam";

Element.prototype.scrollTo = () => undefined;

const open = () => render(
  <MemoryRouter initialEntries={["/exam/s1"]}>
    <Routes><Route path="/exam/:session" element={<ExamPage />} /></Routes>
  </MemoryRouter>,
);
const pills = () => within(screen.getByRole("navigation", { name: "Questions" })).getAllByRole("button");

beforeEach(() => {
  Object.assign(server, { status: "running", fail: { track: false, question: false, note: false, flag: false }, patches: [], trackRequests: 0 });
  sessionStorage.clear();
});
afterEach(() => { vi.useRealTimers(); cleanup(); });

describe("an exam whose track cannot be loaded", () => {
  it("says so instead of waiting forever, and loads it again on request", async () => {
    server.fail.track = true;
    open();
    expect(await screen.findByText("This page could not be loaded.")).toBeTruthy();
    expect(screen.getByText("track store down")).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
    server.fail.track = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Exercise 1 of 2")).toBeTruthy();
    expect(server.trackRequests).toBe(2);
  });
});

describe("a question of a running exam that cannot be loaded", () => {
  it("shows an error with a retry in place of the question text", async () => {
    server.fail.question = true;
    open();
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("This question could not be loaded.")).toBeTruthy();
    expect(within(alert).getByText("question store down")).toBeTruthy();
    // Not an endless placeholder.
    expect(screen.queryByRole("status", { name: "Loading the question" })).toBeNull();
    // The rest of the exam stays usable: the title, the notes, the way to the next question.
    expect(screen.getByRole("heading", { level: 1, name: "A first edit" })).toBeTruthy();
    expect(screen.getByLabelText("Notes")).toBeTruthy();

    server.fail.question = false;
    fireEvent.click(within(alert).getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Text of first-edit.")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("clears the error when the learner moves on to a question that loads", async () => {
    server.fail.question = true;
    open();
    await screen.findByRole("alert");
    server.fail.question = false;
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByText("Text of find-files.")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("notes and flags that cannot be saved", () => {
  it("names a note that was not saved, keeps the text and sends it again on request", async () => {
    open();
    await screen.findByText("Text of first-edit.");
    vi.useFakeTimers();
    server.fail.note = true;
    const notes = screen.getByLabelText("Notes") as HTMLTextAreaElement;
    fireEvent.change(notes, { target: { value: "check /etc/fstab" } });
    expect(screen.queryByRole("alert")).toBeNull();
    // The note is saved a moment after the last key.
    await act(async () => { await vi.advanceTimersByTimeAsync(700); });
    vi.useRealTimers();

    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Could not save your note.")).toBeTruthy();
    expect(notes.value).toBe("check /etc/fstab");
    expect(server.patches).toEqual([]);

    server.fail.note = false;
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(server.patches).toEqual([{ q: "first-edit", note: "check /etc/fstab" }]);
  });

  it("names a flag that was not saved and sends it again on request", async () => {
    open();
    await screen.findByText("Text of first-edit.");
    server.fail.flag = true;
    fireEvent.click(screen.getByRole("button", { name: "Flag for review" }));
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Could not save the flag.")).toBeTruthy();
    // The screen keeps what the learner chose.
    expect(screen.getByRole("button", { name: "Flagged" }).getAttribute("aria-pressed")).toBe("true");

    server.fail.flag = false;
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(server.patches).toEqual([{ q: "first-edit", flagged: true }]);
  });

  it("shows the note only at the question it belongs to", async () => {
    open();
    await screen.findByText("Text of first-edit.");
    server.fail.flag = true;
    fireEvent.click(screen.getByRole("button", { name: "Flag for review" }));
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await screen.findByText("Text of find-files.");
    expect(screen.queryByRole("alert")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    expect(await screen.findByRole("alert")).toBeTruthy();
  });
});

describe("the navigator of an exam of tasks", () => {
  it("names every button by its item and says when it is flagged, not by colour alone", async () => {
    open();
    await screen.findByText("Text of first-edit.");
    // A task is solved in the terminal: there is no answered state to name.
    expect(pills().map((b) => b.getAttribute("aria-label"))).toEqual(["Exercise 1", "Exercise 2"]);
    expect(pills()[0].getAttribute("aria-current")).toBe("true");
    expect(pills()[0].querySelector("svg")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Flag for review" }));
    expect(pills().map((b) => b.getAttribute("aria-label"))).toEqual(["Exercise 1, flagged", "Exercise 2"]);
    // The mark is a flag icon: a shape, in addition to its colour.
    const mark = pills()[0].querySelector("svg");
    expect(mark?.getAttribute("class")).toContain("lucide-flag");
    expect(mark?.parentElement?.getAttribute("aria-hidden")).toBe("true");
    expect(pills()[1].querySelector("svg")).toBeNull();
  });
});

describe("a question of the report that cannot be loaded", () => {
  it("shows an error with a retry where the question text would be", async () => {
    server.status = "finished";
    server.fail.question = true;
    open();
    fireEvent.click(await screen.findByRole("button", { name: /A first edit/ }));
    expect(await screen.findByText("This question could not be loaded.")).toBeTruthy();
    expect(screen.getByText("question store down")).toBeTruthy();
    expect(screen.queryByRole("status", { name: "Loading the question" })).toBeNull();

    server.fail.question = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Text of first-edit.")).toBeTruthy();
    expect(screen.queryByText("This question could not be loaded.")).toBeNull();
  });

  it("names the outcome of each question in words", async () => {
    server.status = "finished";
    open();
    const row = await screen.findByRole("button", { name: /A first edit/ });
    expect(row.textContent).toContain("not solved, 0/2");
  });
});
