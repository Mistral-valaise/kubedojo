import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import type { QuestionMeta, QuestionView, Track } from "../api";
import { choiceView, cka, kcnaQuiz, questionView } from "../fixtures";

const task: QuestionMeta = {
  id: "fix-node", title: "Repair a node", domain: "d1", difficulty: "medium", type: "task",
  minutes: 7, environment: "base", driver: "kind", tags: null, track: "cka", points: 4, hints: 2,
};
// A task for a sandbox this server does not have: it must not be offered.
const remote: QuestionMeta = { ...task, id: "cloud-only", title: "Cloud only", driver: "cloud" };
const ckaTasks: Track = { ...cka, questions: [task, remote] };

// The server side of one practice question: graded once, reset on demand.
const server = { answer: [] as string[], graded: false, calls: [] as string[] };
const current = (): QuestionView =>
  questionView("placing-pods", choiceView({ answer: server.answer }, server.graded), server.graded ? server.answer.join() === "b" : undefined);

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    track: async (id: string) => (id === "kcna" ? kcnaQuiz : ckaTasks),
    config: async () => ({ version: "test", tutor: false, user: "", maxTerminals: 1 }),
    createSession: async () => ({ id: "s1", questions: [] }),
    session: async () => ({ id: "s1", questions: [{ id: "fix-node", status: "preparing" }], log: [] }),
    question: async () => current(),
    prepare: async (_s: string, _q: string, reset = false) => {
      server.calls.push(reset ? "reset" : "prepare");
      if (reset) { server.answer = []; server.graded = false; }
    },
    check: async (_s: string, _q: string, answer?: string[]) => {
      server.calls.push(`check ${answer?.join()}`);
      server.answer = answer ?? []; server.graded = true;
      return current().state.last;
    },
  },
}));
vi.mock("../components/TerminalTabs", () => ({ TerminalTabs: () => <div>terminal</div> }));

import { PracticePage } from "./Practice";

const open = (path: string) => render(
  <MemoryRouter initialEntries={[path]}>
    <Routes><Route path="/t/:track/q/:question" element={<PracticePage />} /></Routes>
  </MemoryRouter>,
);

// jsdom has no layout, so it has no scrolling either.
Element.prototype.scrollTo = () => undefined;
Element.prototype.scrollIntoView = () => undefined;

beforeEach(() => { server.answer = []; server.graded = false; server.calls = []; });
afterEach(cleanup);

describe("PracticePage", () => {
  it("shows a task with its sandbox", async () => {
    open("/t/cka/q/fix-node");
    expect(await screen.findByRole("heading", { name: "Repair a node" })).toBeTruthy();
    expect(screen.getByText("Creating your sandbox")).toBeTruthy();
    expect(screen.queryByRole("radiogroup")).toBeNull();
    // Only the task this server can run counts.
    expect(screen.getByText("1 / 1")).toBeTruthy();
    await waitFor(() => expect(server.calls).toEqual(["prepare"]));
  });

  it("shows a choice question without sandbox, terminal or hints", async () => {
    open("/t/kcna/q/placing-pods");
    expect(await screen.findByRole("radiogroup")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Placing new Pods on nodes" })).toBeTruthy();
    expect(screen.getByText("Which component decides where a Pod runs?")).toBeTruthy();
    expect(screen.queryByText("Creating your sandbox")).toBeNull();
    expect(screen.queryByText("terminal")).toBeNull();
    expect(screen.queryByText(/hint/i)).toBeNull();
    // Choice questions have no driver and are listed all the same.
    expect(screen.getByText("1 / 2")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Next question" }) as HTMLButtonElement).disabled).toBe(false);
    // Reading the question prepares it: there is no prepare call to wait for.
    expect(server.calls).toEqual([]);
  });

  it("checks an answer, reviews it and starts over", async () => {
    open("/t/kcna/q/placing-pods");
    const check = await screen.findByRole("button", { name: "Check answer" }) as HTMLButtonElement;
    expect(check.disabled).toBe(true);
    fireEvent.keyDown(document.body, { key: "1" });
    expect(check.disabled).toBe(false);
    fireEvent.keyDown(document.body, { key: "Enter" });

    const verdict = await screen.findByRole("status");
    expect(within(verdict).getByText("Incorrect")).toBeTruthy();
    expect(server.calls).toEqual(["check a"]);
    expect(screen.getByText("Your answer · incorrect")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Check answer" })).toBeNull();
    // The review takes the focus, so the next Tab reaches its actions.
    await waitFor(() => expect(document.activeElement?.contains(verdict)).toBe(true));

    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByRole("radiogroup");
    expect(server.calls).toEqual(["check a", "reset"]);
    expect(screen.getAllByRole("radio").every((o) => o.getAttribute("aria-checked") === "false")).toBe(true);
    await waitFor(() => expect(document.activeElement).toBe(screen.getAllByRole("radio")[0]));

    fireEvent.click(screen.getByRole("radio", { name: "kube-scheduler" }));
    fireEvent.click(screen.getByRole("button", { name: "Check answer" }));
    expect(within(await screen.findByRole("status")).getByText("Correct")).toBeTruthy();
    expect(server.calls).toEqual(["check a", "reset", "check b"]);
  });
});
