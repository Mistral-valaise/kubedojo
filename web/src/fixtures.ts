// Test data: one track per kind and a path over them. The skills and theory
// tracks mirror what content/ will declare, so the presentation can be tested
// before that content exists.
import type { ChoiceView, Path, Progress, QuestionMeta, QuestionView, Track } from "./api";

function track(over: Partial<Track> & Pick<Track, "id" | "short" | "title" | "kind">): Track {
  return {
    description: "", exam: { durationMinutes: 120, passPercent: 66, questions: 17 },
    domains: [{ id: "d1", title: "Domain", weight: 100, summary: "" }],
    questions: [], environments: null, ...over,
  };
}

export const cka = track({
  id: "cka", short: "CKA", title: "Certified Kubernetes Administrator", kind: "certification", kubernetesVersion: "v1.37",
  environments: { base: { name: "base", type: "", workers: 2, description: "" } },
});
export const ckad = track({
  id: "ckad", short: "CKAD", title: "Certified Kubernetes Application Developer", kind: "certification", kubernetesVersion: "v1.37",
});
export const linux = track({
  id: "linux", short: "Linux", title: "Linux & Vim Essentials", kind: "skills",
  exam: { durationMinutes: 60, passPercent: 70, questions: 15 },
  environments: { host: { name: "host", type: "container", workers: 0, description: "" } },
});
export const kcna = track({
  id: "kcna", short: "KCNA", title: "Kubernetes and Cloud Native Associate", kind: "theory",
  exam: { durationMinutes: 45, passPercent: 75, questions: 30 },
  docs: [{ title: "KCNA curriculum", url: "https://example.org/kcna" }],
});

export const path: Path = {
  id: "kubestronaut", title: "Road to Kubestronaut", summary: "A recommended order.",
  url: "https://www.cncf.io/training/kubestronaut/",
  disclaimer: "kubedojo is an independent project. Kubestronaut is a CNCF program.",
  steps: [
    { track: "linux", role: "foundation", why: "The shell every exam runs in." },
    { track: "kcna", role: "required", why: "The concepts." },
    { track: "ckad", role: "required", why: "Applications." },
    { track: "cka", role: "required", why: "The cluster." },
  ],
};

export function progress(trackId: string, readiness: number, exams: boolean[] = []): Progress {
  return {
    track: trackId, readiness, passPercent: 66, attempts: 0, domains: [], questions: {}, recommendations: null,
    exams: exams.map((passed, i) => ({ sessionId: `s${i}`, score: 0, max: 0, percent: passed ? 90 : 10, passed, finishedAt: "" })),
  };
}

// Two choice questions of the theory track, as the API serves them.
export const quizQuestions: QuestionMeta[] = ["placing-pods", "listing-pods"].map((id, i) => ({
  id, title: i === 0 ? "Placing new Pods on nodes" : "Listing Pods", domain: "d1", difficulty: "easy", type: "choice",
  minutes: 1, environment: "", driver: "", tags: null, track: "kcna", points: 1, hints: 0,
}));
export const kcnaQuiz: Track = { ...kcna, questions: quizQuestions };

/** Options a-d; b is correct. `reviewed` adds the answer key, as the server does once the question is revealed. */
export function choiceView(over: Partial<ChoiceView> = {}, reviewed = false): ChoiceView {
  return {
    multiple: false,
    options: [
      { id: "a", text: "kubelet" }, { id: "b", text: "kube-scheduler" },
      { id: "c", text: "`kubectl get pods`" }, { id: "d", text: "kube-proxy" },
    ],
    answer: [],
    review: reviewed ? {
      correct: ["b"], explanation: "The scheduler binds Pods to nodes.",
      options: { a: "A kubelet starts containers of Pods bound to its node." },
      references: [{ title: "Kubernetes Scheduler", url: "https://kubernetes.io/docs/concepts/scheduling-eviction/kube-scheduler/" }],
    } : undefined,
    ...over,
  };
}

export function questionView(id: string, choice: ChoiceView, passed?: boolean): QuestionView {
  return {
    question: quizQuestions.find((q) => q.id === id)!, task: "Which component decides where a Pod runs?", host: "",
    state: {
      id, status: "ready", flagged: false, note: "", hintsUsed: 0, solutionViewed: false, answer: choice.answer,
      last: passed === undefined ? undefined : { questionId: id, score: passed ? 1 : 0, max: 1, passed, checks: [], at: "" },
    },
    choice,
  };
}
