// Typed client for the kubedojo API.

export interface Domain { id: string; title: string; weight: number; summary: string }
export interface QuestionMeta {
  id: string; title: string; domain: string; difficulty: "easy" | "medium" | "hard";
  // A task is solved in the terminal, a choice question by picking options.
  type: "task" | "choice";
  minutes: number; environment: string; driver: string; tags: string[] | null;
  track: string; points: number; hints: number;
}
export interface Link { title: string; url: string }
export interface Track {
  id: string; title: string; short: string; description: string;
  // How the track is presented; see words() in track.ts.
  kind: "certification" | "skills" | "theory";
  docs?: Link[]; kubernetesVersion?: string;
  exam: { durationMinutes: number; passPercent: number; questions: number };
  domains: Domain[]; questions: QuestionMeta[] | null;
  // Null for a track without a sandbox. An empty type means cluster.
  environments: Record<string, { name: string; type: "" | "cluster" | "container"; workers: number; description: string }> | null;
}
export interface PathStep { track: string; role: "foundation" | "required"; why: string }
/** The recommended order through the tracks. It never locks a track. */
export interface Path { id: string; title: string; summary: string; url: string; disclaimer: string; steps: PathStep[] | null }
export interface CheckResult {
  id: string; title: string; points: number; passed: boolean;
  expected: string; observed: string; explain: string; command: string;
  // Debug data of the last run. Optional: results stored by older versions lack it.
  on?: string; exitCode?: number; stderr?: string; durationMs?: number; attempts?: number; timedOut?: boolean;
}
export interface Result { questionId: string; score: number; max: number; passed: boolean; checks: CheckResult[]; at: string }
export interface QState {
  id: string; status: "pending" | "preparing" | "ready" | "error" | ""; error?: string;
  flagged: boolean; note: string; hintsUsed: number; solutionViewed: boolean; last?: Result;
  // Choice questions only: the order the options are shown in and the learner's picks.
  order?: string[]; answer?: string[];
}
export interface DomainScore { id: string; title: string; weight: number; score: number; max: number }
export interface Report {
  score: number; max: number; percent: number; passPercent: number; passed: boolean;
  domains: DomainScore[]; usedSeconds: number;
}
export type SessionStatus = "provisioning" | "ready" | "running" | "grading" | "finished" | "error" | "destroyed";
export interface Session {
  id: string; mode: "practice" | "exam"; track: string; status: SessionStatus; error?: string;
  createdAt: string; startedAt?: string; deadline?: string; finishedAt?: string; durationMinutes: number;
  envs: Record<string, string>; questions: QState[] | null; log: { at: string; msg: string }[] | null;
  report?: Report; remainingSeconds?: number; now: string;
  meta: Record<string, { title: string; domain: string; points: number; host: string }>;
}
/** The answer key of a choice question. The server sends it only once the question is revealed. */
export interface ChoiceReview {
  correct: string[]; explanation: string;
  // Option id -> why it is right or wrong. Options without a text are missing.
  options: Record<string, string>;
  references: Link[] | null;
}
export interface ChoiceView {
  multiple: boolean;
  // In the order drawn for this session.
  options: { id: string; text: string }[];
  answer: string[];
  review?: ChoiceReview;
}
/** For a choice question `task` is the stem and `choice` is set. */
export interface QuestionView { question: QuestionMeta; task: string; host: string; state: QState; choice?: ChoiceView }
export interface DomainProgress {
  id: string; title: string; weight: number; questions: number; attempted: number; solved: number; mastery: number;
}
export interface Progress {
  track: string; readiness: number; passPercent: number; attempts: number;
  domains: DomainProgress[];
  // `latest` is what counts for a choice question, `best` for a task.
  questions: Record<string, { best: number; latest: number; attempts: number; last: string }>;
  exams: { sessionId: string; score: number; max: number; percent: number; passed: boolean; finishedAt: string }[];
  recommendations: { domain: string; title: string; reason: string; next?: string }[] | null;
}
export interface Config { version: string; tutor: boolean; user: string; maxTerminals: number }

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch("/api" + path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, (data as { error?: string }).error || res.statusText);
  return data as T;
}

export const api = {
  config: () => call<Config>("GET", "/config"),
  tracks: () => call<Track[]>("GET", "/tracks"),
  track: (id: string) => call<Track>("GET", `/tracks/${id}`),
  // Content without a path answers 404, which is not an error for the UI.
  path: () => call<Path>("GET", "/path").catch((e) => {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }),
  lesson: (track: string, domain: string) => call<{ domain: Domain; markdown: string }>("GET", `/tracks/${track}/lessons/${domain}`),
  progress: (track: string) => call<Progress>("GET", `/tracks/${track}/progress`),
  sessions: () => call<Session[]>("GET", "/sessions"),
  createSession: (body: { mode: "practice" | "exam"; track: string; minutes?: number; questions?: number }) =>
    call<Session>("POST", "/sessions", body),
  session: (id: string) => call<Session>("GET", `/sessions/${id}`),
  destroy: (id: string) => call<unknown>("DELETE", `/sessions/${id}`),
  start: (id: string) => call<Session>("POST", `/sessions/${id}/start`),
  finish: (id: string) => call<Session>("POST", `/sessions/${id}/finish`),
  question: (sid: string, q: string) => call<QuestionView>("GET", `/sessions/${sid}/questions/${q}`),
  patchQuestion: (sid: string, q: string, body: { flagged?: boolean; note?: string; answer?: string[] }) =>
    call<unknown>("PATCH", `/sessions/${sid}/questions/${q}`, body),
  prepare: (sid: string, q: string, reset = false) =>
    call<unknown>("POST", `/sessions/${sid}/questions/${q}/prepare${reset ? "?reset=1" : ""}`),
  // A choice question is submitted and graded in one call by passing the answer.
  check: (sid: string, q: string, answer?: string[]) =>
    call<Result>("POST", `/sessions/${sid}/questions/${q}/check`, answer && { answer }),
  hint: (sid: string, q: string, n: number) => call<{ n: number; markdown: string }>("GET", `/sessions/${sid}/questions/${q}/hints/${n}`),
  solution: (sid: string, q: string) => call<{ markdown: string }>("GET", `/sessions/${sid}/questions/${q}/solution`),
  rate: (track: string, q: string, stars: number, comment: string) =>
    call<unknown>("POST", `/tracks/${track}/questions/${q}/rating`, { stars, comment }),
  tutor: (sid: string, q: string, ask: string) => call<{ markdown: string }>("POST", `/sessions/${sid}/questions/${q}/tutor`, { ask }),
};
