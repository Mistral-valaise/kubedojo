import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import type { QuestionMeta, QuestionView, Result, Track } from "../api";
import { choiceView, cka, kcnaQuiz, questionView } from "../fixtures";

const task: QuestionMeta = {
  id: "fix-node", title: "Repair a node", domain: "d1", difficulty: "medium", type: "task",
  minutes: 7, environment: "base", driver: "kind", tags: null, track: "cka", points: 4, hints: 2,
};
// A task for a sandbox this server does not have: it must not be offered.
const remote: QuestionMeta = { ...task, id: "cloud-only", title: "Cloud only", driver: "cloud" };
const ckaTasks: Track = { ...cka, questions: [task, remote] };

// The server side of one practice question: graded once, reset on demand.
const server = {
  answer: [] as string[], graded: false, calls: [] as string[],
  // The task: whether its sandbox is ready, and the pending check request, which the test answers.
  ready: false, grade: null as null | ((r: Result) => void),
};
const taskResult = (passed: boolean): Result => ({
  questionId: "fix-node", score: passed ? 4 : 0, max: 4, passed, at: "now",
  checks: [{ id: "c", title: "Node is Ready", points: 4, passed, expected: 'equals "Ready"', observed: "NotReady", explain: "The kubelet must run.", command: "kubectl get node" }],
});
const current = (): QuestionView =>
  questionView("placing-pods", choiceView({ answer: server.answer }, server.graded), server.graded ? server.answer.join() === "b" : undefined);

vi.mock("../api", async (original) => ({
  ...(await original<typeof import("../api")>()),
  api: {
    track: async (id: string) => (id === "kcna" ? kcnaQuiz : ckaTasks),
    config: async () => ({ version: "test", tutor: false, user: "", maxTerminals: 1 }),
    createSession: async () => ({ id: "s1", questions: [] }),
    session: async () => ({ id: "s1", questions: [{ id: "fix-node", status: server.ready ? "ready" : "preparing" }], log: [] }),
    question: async (_s: string, q: string): Promise<QuestionView> => (q === "fix-node"
      ? { question: task, task: "Repair the kubelet.", host: "cka1", state: { id: q, status: "ready", flagged: false, note: "", hintsUsed: 0, solutionViewed: false } }
      : current()),
    solution: async () => ({ markdown: "Restart the kubelet." }),
    prepare: async (_s: string, _q: string, reset = false) => {
      server.calls.push(reset ? "reset" : "prepare");
      if (reset) { server.answer = []; server.graded = false; }
    },
    check: async (_s: string, q: string, answer?: string[]) => {
      if (q === "fix-node") return new Promise<Result>((resolve) => { server.grade = resolve; });
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

beforeEach(() => { server.answer = []; server.graded = false; server.calls = []; server.ready = false; server.grade = null; });
afterEach(() => { vi.useRealTimers(); cleanup(); });

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

  it("says why a check takes long, with the seconds it has run, and stays quiet for a fast one", async () => {
    server.ready = true;
    open("/t/cka/q/fix-node");
    const check = await screen.findByRole("button", { name: "Check my solution" }) as HTMLButtonElement;
    const note = () => screen.getByRole("status");
    expect(note().getAttribute("aria-live")).toBe("polite");
    expect(note().textContent).toBe("");

    vi.useFakeTimers();
    fireEvent.click(check);
    expect(check.disabled).toBe(true);
    act(() => { vi.advanceTimersByTime(2900); });
    expect(note().textContent).toBe("");
    act(() => { vi.advanceTimersByTime(200); });
    expect(note().textContent).toBe("Still checking. Some checks wait a little for the cluster to settle before they give up. 3 s");
    // The counter is for the eye: read out every second it would be noise.
    expect(within(note()).getByText("3 s").getAttribute("aria-hidden")).toBe("true");
    act(() => { vi.advanceTimersByTime(39_000); });
    expect(within(note()).getByText("42 s")).toBeTruthy();
    expect(check.disabled).toBe(true);

    await act(async () => { server.grade?.(taskResult(false)); });
    expect(note().textContent).toBe("");
    expect(check.disabled).toBe(false);
    expect(screen.getByText("0 of 1 checks passed")).toBeTruthy();

    // A check that answers at once never shows the note.
    fireEvent.click(check);
    act(() => { vi.advanceTimersByTime(900); });
    await act(async () => { server.grade?.(taskResult(true)); });
    expect(note().textContent).toBe("");
    expect(screen.getByText("All checks passed")).toBeTruthy();
  });

  it("asks before showing the solution of an unsolved task, and no more once every check passed", async () => {
    server.ready = true;
    open("/t/cka/q/fix-node");
    fireEvent.click(await screen.findByRole("button", { name: "Show solution" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/You learn most from the attempt/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Keep trying" }));
    expect(screen.queryByText("Restart the kubelet.")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Check my solution" }));
    await act(async () => { server.grade?.(taskResult(true)); });
    fireEvent.click(screen.getByRole("button", { name: "Show solution" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(await screen.findByText("Restart the kubelet.")).toBeTruthy();
  });

  it("offers task and terminal as tabs for a narrow screen", async () => {
    server.ready = true;
    open("/t/cka/q/fix-node");
    await screen.findByRole("button", { name: "Check my solution" });
    expect(screen.getAllByRole("tab").map((x) => x.textContent)).toEqual(["Task", "Terminal"]);
    expect(screen.getByText("terminal")).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Terminal" }));
    // The task stays mounted behind the tab, like the terminal does.
    expect(screen.getByRole("button", { name: "Check my solution" })).toBeTruthy();
  });
});
