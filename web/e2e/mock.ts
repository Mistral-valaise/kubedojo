import type { Page, Route } from "@playwright/test";
import type { AssistantMessage, AssistantStatus, QuestionMeta, QuestionView, Result, Session, Settings, SettingsPatch, Track } from "../src/api";
import { choiceView, cka, kcnaQuiz, questionView } from "../src/fixtures";

// A kubedojo server for the browser tests: the API is answered here, in the
// test, and the real UI runs against it. Nothing is provisioned and no model
// is asked.

export const task: QuestionMeta = {
  id: "fix-rollout", title: "Fix the broken rollout", domain: "workloads", difficulty: "medium", type: "task",
  minutes: 8, environment: "base", driver: "kind", tags: null, track: "cka", points: 7, hints: 2,
};
const more: QuestionMeta[] = [
  { ...task, id: "scale-deployment", title: "Scale a Deployment", difficulty: "easy", minutes: 4, points: 4 },
  { ...task, id: "drain-node", title: "Drain a node for maintenance", difficulty: "hard", minutes: 10, points: 8 },
];
export const ckaTrack: Track = {
  ...cka,
  domains: [{ id: "workloads", title: "Workloads & Scheduling", weight: 15, summary: "" }],
  questions: [more[0], task, more[1]],
};

const taskText = [
  "The Deployment `web` in namespace `shop` is stuck in a rollout. Find out why and fix it so that all 3 replicas become available.",
  "",
  "Do not delete the Deployment.",
].join("\n");

export const failed: Result = {
  questionId: task.id, score: 4, max: 7, passed: false, at: "2026-11-24T10:02:11Z",
  checks: [
    { id: "exists", title: "Deployment web still exists", points: 2, passed: true, expected: "", observed: "", explain: "The Deployment must be repaired, not replaced.", command: "kubectl -n shop get deployment web" },
    { id: "strategy", title: "Rollout strategy is unchanged", points: 2, passed: true, expected: "", observed: "", explain: "The strategy stays `RollingUpdate`.", command: "kubectl -n shop get deployment web -o jsonpath='{.spec.strategy.type}'" },
    { id: "available", title: "3 replicas are available", points: 3, passed: false, expected: "equals \"3\"", observed: "0", explain: "A rollout is done when the new ReplicaSet has all its Pods available.", command: "kubectl -n shop get deployment web -o jsonpath='{.status.availableReplicas}'" },
  ],
};

export const answer = [
  "The check **3 replicas are available** failed because none of the new Pods is Ready. They are in `ImagePullBackOff`: the kubelet cannot pull the image the Deployment asks for, so the rollout waits.",
  "",
  "A rollout only finishes when the new ReplicaSet becomes available. Look at the events of one Pod to see the exact image reference that fails:",
  "",
  "```bash",
  "kubectl -n shop describe pod -l app=web | tail -n 12",
  "```",
  "",
  "Once you know which tag is wrong, correct it on the Deployment. Ask me if you want a hint for that step.",
].join("\n");

export const quizAnswer = [
  "**kube-scheduler** is right because placing a Pod is a decision, and that decision has one owner. The scheduler watches for Pods without a node, filters the nodes that can run them, scores the rest and writes the chosen node into `spec.nodeName`.",
  "",
  "The **kubelet** you picked only comes in afterwards: it sees a Pod that is already bound to its node and starts the containers. It never chooses a node.",
].join("\n");

/** A body of server-sent events, as POST .../assistant answers. */
export function sse(events: [name: string, data: unknown][]): string {
  return events.map(([name, data]) => `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`).join("");
}

/** The events of one complete answer, in a few pieces. */
export function answerEvents(text: string, stages = ["checks", "snapshot", "diagnostics"], id = 2): [string, unknown][] {
  const pieces = text.match(/[\s\S]{1,40}/g) ?? [];
  return [
    ["meta", { conversation: "s1/q", provider: "ollama", model: "llama3.2", turn: 1, userMessageId: id - 1 }],
    ...stages.map((stage): [string, unknown] => ["context", { stage }]),
    ...pieces.map((piece): [string, unknown] => ["delta", { text: piece }]),
    ["done", { messageId: id, finishReason: "stop", model: "llama3.2", usage: { inputTokens: 812, outputTokens: 164 }, durationMs: 4210 }],
  ];
}

const field = <T,>(value: T, lockedBy?: string) => ({ value, source: lockedBy ? "flag" as const : "file" as const, locked: !!lockedBy, ...(lockedBy ? { lockedBy } : {}) });
const providers: Settings["providers"] = [
  { id: "anthropic", label: "Anthropic", needsKey: true, defaultModel: "claude-opus-5-5" },
  { id: "openai", label: "OpenAI-compatible", needsKey: false, presets: [
    { label: "Ollama", baseURL: "http://localhost:11434/v1" }, { label: "LM Studio", baseURL: "http://localhost:1234/v1" },
    { label: "OpenAI", baseURL: "https://api.openai.com/v1" }, { label: "OpenRouter", baseURL: "https://openrouter.ai/api/v1" },
  ] },
];
/** GET /api/settings of a local server with Ollama. `over` replaces parts of `assistant`. */
export function settings(over: Partial<Settings["assistant"]> = {}, top: Partial<Settings> = {}): Settings {
  return {
    writable: true, hosted: false, providers,
    assistant: {
      enabled: field(true), provider: field("openai"), model: field("llama3.2"), baseURL: field("http://localhost:11434/v1"),
      apiKey: { set: false, source: "default", locked: false }, configured: true, problem: "", ...over,
    },
    traceContent: { value: false, reason: "no local observability stack" },
    ...top,
  };
}

export interface MockServer {
  /** GET /api/config → assistant. Null is a server before 0.5, which has none. */
  status: AssistantStatus | null;
  settings: Settings;
  /** The stored conversation of the question. */
  history: AssistantMessage[];
  /** What POST .../assistant answers: a body of events, or a refusal. */
  reply: { events: [string, unknown][] } | { status: number; body: unknown; headers?: Record<string, string> };
  /** What POST .../check answers for the task. */
  result: Result;
  /** The exam session GET /api/sessions/exam1 answers with; null without one. */
  exam: Session | null;
  /** Bodies of the requests the UI sent, in order. */
  asked: unknown[];
  saved: SettingsPatch[];
  tested: SettingsPatch[];
  /** How often the configuration was asked for. */
  configRequests: number;
  /** Requests that the mock does not know: a test asserts that there are none. */
  unknown: string[];
}

const quizView = (answered: string[], reviewed: boolean): QuestionView =>
  questionView("placing-pods", choiceView({ answer: answered }, reviewed), reviewed ? answered.join() === "b" : undefined);

/** Answers every request under /api. Returns the state of the server, which a test may change. */
export async function mockApi(page: Page, init: Partial<MockServer> = {}): Promise<MockServer> {
  const server: MockServer = {
    status: { enabled: true, configured: true, provider: "ollama", model: "llama3.2", writable: true },
    settings: settings(), history: [], reply: { events: answerEvents(answer) }, result: failed, exam: null,
    asked: [], saved: [], tested: [], configRequests: 0, unknown: [],
    ...init,
  };
  const quiz = { answer: [] as string[], graded: false };
  const json = (route: Route, body: unknown, status = 200, headers: Record<string, string> = {}) =>
    route.fulfill({ status, contentType: "application/json", headers, body: JSON.stringify(body) });
  const practice = (track: string): Partial<Session> => ({
    id: "s1", mode: "practice", track, status: "running", envs: { base: "cka1" }, log: [],
    questions: [{ id: task.id, status: "ready", flagged: false, note: "", hintsUsed: 0, solutionViewed: false }], meta: {},
  });

  await page.route("**/api/**", async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname.replace(/^\/api/, "");
    const what = `${req.method()} ${path}`;
    const body = () => { try { return req.postDataJSON() as Record<string, unknown>; } catch { return {}; } };
    const assistant = /^\/sessions\/([^/]+)\/questions\/([^/]+)\/assistant$/.exec(path);
    if (assistant) {
      if (req.method() === "GET") return json(route, { messages: server.history, limits: { turns: 20, messageChars: 4000 } });
      if (req.method() === "DELETE") { server.history = []; return route.fulfill({ status: 204 }); }
      server.asked.push(body());
      if ("events" in server.reply) return route.fulfill({ status: 200, contentType: "text/event-stream", headers: { "Cache-Control": "no-store" }, body: sse(server.reply.events) });
      return json(route, server.reply.body, server.reply.status, server.reply.headers);
    }
    switch (what) {
      case "GET /config": server.configRequests++; return json(route, { version: "0.5.0", tutor: !!server.status?.enabled && server.status.configured, user: "", maxTerminals: 4, ...(server.status ? { assistant: server.status } : {}) });
      case "GET /tracks": return json(route, [ckaTrack, kcnaQuiz]);
      case "GET /tracks/cka": return json(route, ckaTrack);
      case "GET /tracks/kcna": return json(route, kcnaQuiz);
      case "GET /path": return json(route, { error: "no path" }, 404);
      case "GET /sessions": return json(route, []);
      case "POST /sessions": return json(route, practice(String(body().track)));
      case "GET /sessions/s1": return json(route, practice("cka"));
      case `POST /sessions/s1/questions/${task.id}/prepare`: return json(route, {});
      case `GET /sessions/s1/questions/${task.id}`:
        return json(route, { question: task, task: taskText, host: "cka1", state: { id: task.id, status: "ready", flagged: false, note: "", hintsUsed: 0, solutionViewed: false } });
      case `POST /sessions/s1/questions/${task.id}/check`: return json(route, server.result);
      case "GET /sessions/s1/questions/placing-pods": return json(route, quizView(quiz.answer, quiz.graded));
      case "POST /sessions/s1/questions/placing-pods/check":
        quiz.answer = (body().answer as string[]) ?? []; quiz.graded = true;
        return json(route, quizView(quiz.answer, true).state.last);
      case "POST /sessions/s1/questions/placing-pods/prepare": quiz.answer = []; quiz.graded = false; return json(route, {});
      case "GET /settings": return json(route, server.settings);
      case "PUT /settings": {
        const patch = (body().assistant ?? {}) as SettingsPatch;
        server.saved.push(patch);
        const a = server.settings.assistant;
        // What /api/config says follows the settings, as on the server.
        if (server.status) server.status = { ...server.status, enabled: patch.enabled ?? server.status.enabled, model: patch.model ?? server.status.model };
        server.settings = { ...server.settings, assistant: {
          ...a,
          enabled: patch.enabled === undefined ? a.enabled : { ...a.enabled, value: patch.enabled },
          provider: patch.provider === undefined ? a.provider : { ...a.provider, value: patch.provider },
          model: patch.model === undefined ? a.model : { ...a.model, value: patch.model },
          baseURL: patch.baseURL === undefined ? a.baseURL : { ...a.baseURL, value: patch.baseURL },
          apiKey: patch.apiKey === undefined ? a.apiKey : patch.apiKey === "" ? { set: false, source: "default", locked: false } : { set: true, last4: patch.apiKey.slice(-4), source: "file", locked: false },
        } };
        return json(route, server.settings);
      }
      case "POST /settings/test": {
        const patch = (body().assistant ?? {}) as SettingsPatch;
        server.tested.push(patch);
        return json(route, { ok: true, provider: "ollama", model: patch.model || "llama3.2", latencyMs: 412 });
      }
    }
    if (server.exam) {
      if (what === "GET /sessions/exam1") return json(route, server.exam);
      const q = /^GET \/sessions\/exam1\/questions\/([^/]+)$/.exec(what);
      if (q) {
        if (q[1] === task.id) return json(route, { question: task, task: taskText, host: "cka1", state: server.exam.questions!.find((x) => x.id === task.id) });
        return json(route, { question: more.find((m) => m.id === q[1]), task: "Scale the Deployment `api` in namespace `shop` to 5 replicas.", host: "cka1", state: server.exam.questions!.find((x) => x.id === q[1]) });
      }
    }
    server.unknown.push(what);
    return json(route, { error: `not mocked: ${what}` }, 404);
  });
  return server;
}

/**
 * A terminal that connects and shows `lines`, for a page with the task
 * workspace. Returns what the page typed into it.
 */
export async function mockTerminal(page: Page, lines: string[] = []): Promise<string[]> {
  const typed: string[] = [];
  await page.routeWebSocket(/\/api\/sessions\/[^/]+\/terminal/, (ws) => {
    // The shell speaks first: its output is there as soon as the terminal is.
    if (lines.length > 0) ws.send(lines.join("\r\n"));
    ws.onMessage((m) => { if (typeof m !== "string") typed.push(m.toString("utf8")); }); // text frames are resizes
  });
  return typed;
}
