// Typed client for the kubedojo API.

export interface Domain { id: string; title: string; weight: number; summary: string }
export interface QuestionMeta {
  id: string; title: string; domain: string; difficulty: "easy" | "medium" | "hard";
  minutes: number; environment: string; driver: string; tags: string[] | null;
  track: string; points: number; hints: number;
}
export interface Track {
  id: string; title: string; short: string; description: string; kubernetesVersion: string;
  exam: { durationMinutes: number; passPercent: number; questions: number };
  domains: Domain[]; questions: QuestionMeta[] | null;
  environments: Record<string, { name: string; workers: number; description: string }>;
}
export interface CheckResult {
  id: string; title: string; points: number; passed: boolean;
  expected: string; observed: string; explain: string; command: string;
}
export interface Result { questionId: string; score: number; max: number; passed: boolean; checks: CheckResult[]; at: string }
export interface QState {
  id: string; status: "pending" | "preparing" | "ready" | "error" | ""; error?: string;
  flagged: boolean; note: string; hintsUsed: number; solutionViewed: boolean; last?: Result;
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
export interface QuestionView { question: QuestionMeta; task: string; host: string; state: QState }
export interface DomainProgress {
  id: string; title: string; weight: number; questions: number; attempted: number; solved: number; mastery: number;
}
export interface Progress {
  track: string; readiness: number; passPercent: number; attempts: number;
  domains: DomainProgress[];
  questions: Record<string, { best: number; attempts: number; last: string }>;
  exams: { sessionId: string; score: number; max: number; percent: number; passed: boolean; finishedAt: string }[];
  recommendations: { domain: string; title: string; reason: string; next?: string }[] | null;
}
export interface Config { version: string; tutor: boolean; user: string }

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
  patchQuestion: (sid: string, q: string, body: { flagged?: boolean; note?: string }) =>
    call<unknown>("PATCH", `/sessions/${sid}/questions/${q}`, body),
  prepare: (sid: string, q: string, reset = false) =>
    call<unknown>("POST", `/sessions/${sid}/questions/${q}/prepare${reset ? "?reset=1" : ""}`),
  check: (sid: string, q: string) => call<Result>("POST", `/sessions/${sid}/questions/${q}/check`),
  hint: (sid: string, q: string, n: number) => call<{ n: number; markdown: string }>("GET", `/sessions/${sid}/questions/${q}/hints/${n}`),
  solution: (sid: string, q: string) => call<{ markdown: string }>("GET", `/sessions/${sid}/questions/${q}/solution`),
  tutor: (sid: string, q: string, ask: string) => call<{ markdown: string }>("POST", `/sessions/${sid}/questions/${q}/tutor`, { ask }),
};
