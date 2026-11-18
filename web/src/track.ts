// Wording per track kind. Pages take every track-dependent string from
// words(track), so a new kind is one more entry here and no page decides by
// track id.
import type { Link, Progress, QuestionMeta, Track } from "./api";
import { t } from "./i18n";

export type Kind = "certification" | "skills" | "theory";

export interface Words {
  kind: Kind;
  /** False when the track needs no sandbox at all (theory). */
  sandbox: boolean;
  kindLabel: string;
  /** What the learner must know about this kind of track; null when nothing. */
  kindNote: string | null;
  /** Text of the Kubernetes version badge; null when the track has no version. */
  kubernetes: string | null;
  /** Documentation of the track; null when there is none to link. */
  docs: Link | null;
  /** Documentation allowed during the assessment; null for a closed-book exam. */
  assessmentDocs: Link | null;

  assessment: string;
  startAssessment: string;
  fullAssessment: string;
  assessmentReport: string;
  assessmentPassed: string;
  finishAssessment: string;
  finishTitle: string;
  preparingTitle: string;
  gradingTitle: string;
  readiness: string;
  readinessNote: (passPercent: number) => string;

  /** Count of practice items of the track. */
  questions: (n: number) => string;
  /** Count of items in an assessment. */
  tasks: (n: number) => string;
  duration: (minutes: number) => string;
  assessmentSummary: (n: number, minutes: number) => string;
  assessmentOption: (n: number, minutes: number) => string;
  briefingSummary: (n: number, minutes: number, passPercent: number) => string;
  briefing: string[];
  startNote: string;
  allSolved: string;
  noQuestions: string;
  practiseDomain: (n: number) => string;

  /** Notes shown while something is being set up; empty without a sandbox. */
  provisioningNote: string;
  assessmentProvisioningNote: string;
  gradingNote: string;
  environmentReady: string;
  environmentFailed: string;
  leaveAfterFailure: string;
  activeSession: (hosts: string[]) => string;

  prepareFailed: string;
  preparing: string;
  reset: string;
  resetTitle: string;
  resetNote: string;
  previous: string;
  next: string;
  notePlaceholder: string;
  flagged: (n: number) => string;
  passedHeadline: string;
  reportSummary: (passPercent: number, solved: number, n: number, time: string) => string;
  /** Text of the sandbox card in the report; empty without a sandbox. */
  reportSandbox: string;
  reportList: string;
  tutorPlaceholder: string;
  tutorNote: string;
}

const kubernetesDocs = (): Link => ({ title: t("Kubernetes docs"), url: "https://kubernetes.io/docs/" });

type Wording = Omit<Words, "kind" | "sandbox" | "kubernetes" | "docs" | "assessmentDocs">;

const wording: Record<Kind, () => Wording> = {
  certification: () => ({
    kindLabel: t("Hands-on"),
    kindNote: null,
    assessment: t("Exam simulation"),
    startAssessment: t("Start exam simulation"),
    fullAssessment: t("Full exam"),
    assessmentReport: t("Exam report"),
    assessmentPassed: t("Exam simulation passed"),
    finishAssessment: t("Finish exam"),
    finishTitle: t("Finish the exam?"),
    preparingTitle: t("Preparing your exam"),
    gradingTitle: t("Grading your exam"),
    readiness: t("Exam readiness"),
    readinessNote: (p) => t("Readiness is your best result per question, weighted like the real exam. Pass mark: {p}%.", { p }),
    questions: (n) => t("{n} questions", { n }),
    tasks: (n) => t("{n} tasks", { n }),
    duration: (n) => t("{n} min exam", { n }),
    assessmentSummary: (q, m) => t("{q} tasks in {m} minutes, weighted like the real exam. No hints. Graded when you finish.", { q, m }),
    assessmentOption: (q, m) => t("{q} tasks · {m} minutes", { q, m }),
    briefingSummary: (n, m, p) => t("{n} tasks · {m} minutes · pass mark {p}%", { n, m, p }),
    briefing: [
      t("Each task names the host to solve it on. Connect with ssh from the terminal, and return with exit before connecting to another host."),
      t("Tasks are independent. Skip what blocks you, flag it, and come back later."),
      t("There is no feedback during the exam. Everything is graded when you finish or when the time is up."),
      t("You may use the official Kubernetes documentation, like in the real exam."),
    ],
    startNote: t("Fresh clusters are created for the exam, which takes one to two minutes. The timer starts only when you say so."),
    allSolved: t("Every question is solved. Run an exam simulation to prove it under time pressure."),
    noQuestions: t("No questions yet."),
    practiseDomain: (n) => t("{n} questions on a live cluster", { n }),
    provisioningNote: t("A real Kubernetes cluster is being created for you. This takes about a minute."),
    assessmentProvisioningNote: t("Fresh clusters are created and every task is set up. The timer has not started."),
    gradingNote: t("Every task is checked against the live clusters."),
    environmentReady: t("Environment ready"),
    environmentFailed: t("The exam environment could not be created."),
    leaveAfterFailure: t("Remove sandbox and go back"),
    activeSession: (hosts) => (hosts.length ? t("Clusters: {hosts}", { hosts: hosts.join(", ") }) : t("No cluster yet")),
    prepareFailed: t("The question could not be prepared."),
    preparing: t("Preparing the environment for this question ..."),
    reset: t("Reset question"),
    resetTitle: t("Reset this question?"),
    resetNote: t("Everything this question created is removed and its starting state is restored. Other questions are not affected."),
    previous: t("Previous question"),
    next: t("Next question"),
    notePlaceholder: t("Private notes for this task ..."),
    flagged: (n) => t("{n} task(s) are still flagged for review.", { n }),
    passedHeadline: t("You would have passed this exam."),
    reportSummary: (p, s, n, time) => t("Pass mark {p}% · {s} of {n} tasks fully solved · time used {time}", { p, s, n, time }),
    reportSandbox: t("The clusters are still running, so you can inspect what you built and retry tasks using the solutions."),
    reportList: t("Task by task"),
    tutorPlaceholder: t("Ask about this task ..."),
    tutorNote: t("The tutor sees the task, your check results and a summary of the cluster state."),
  }),

  skills: () => ({
    kindLabel: t("Foundation"),
    kindNote: t("This is a kubedojo skills track, not a certification exam. No official exam stands behind it: the skills assessment is kubedojo's own measure."),
    assessment: t("Skills assessment"),
    startAssessment: t("Start skills assessment"),
    fullAssessment: t("Full assessment"),
    assessmentReport: t("Assessment report"),
    assessmentPassed: t("Skills assessment passed"),
    finishAssessment: t("Finish assessment"),
    finishTitle: t("Finish the assessment?"),
    preparingTitle: t("Preparing your assessment"),
    gradingTitle: t("Grading your assessment"),
    readiness: t("Readiness"),
    readinessNote: (p) => t("Readiness is your best result per exercise, weighted by domain. Pass mark of the skills assessment: {p}%.", { p }),
    questions: (n) => t("{n} exercises", { n }),
    tasks: (n) => t("{n} exercises", { n }),
    duration: (n) => t("{n} min assessment", { n }),
    assessmentSummary: (q, m) => t("{q} exercises in {m} minutes, weighted by domain. No hints. Graded when you finish.", { q, m }),
    assessmentOption: (q, m) => t("{q} exercises · {m} minutes", { q, m }),
    briefingSummary: (n, m, p) => t("{n} exercises · {m} minutes · pass mark {p}%", { n, m, p }),
    briefing: [
      t("Each exercise names the host to solve it on. Connect with ssh from the terminal, and return with exit before connecting to another host."),
      t("Exercises are independent. Skip what blocks you, flag it, and come back later."),
      t("There is no feedback during the assessment. Everything is graded when you finish or when the time is up."),
      t("You may use the man pages and the built-in help of every tool."),
    ],
    startNote: t("A fresh Linux sandbox is started for the assessment, which takes a few seconds. The timer starts only when you say so."),
    allSolved: t("Every exercise is solved. Run a skills assessment to prove it under time pressure."),
    noQuestions: t("No exercises yet."),
    practiseDomain: (n) => t("{n} exercises in a Linux sandbox", { n }),
    provisioningNote: t("A Linux sandbox is being started for you. This takes a few seconds."),
    assessmentProvisioningNote: t("A fresh Linux sandbox is started and every exercise is set up. The timer has not started."),
    gradingNote: t("Every exercise is checked against the live sandbox."),
    environmentReady: t("Environment ready"),
    environmentFailed: t("The assessment environment could not be created."),
    leaveAfterFailure: t("Remove sandbox and go back"),
    activeSession: (hosts) => (hosts.length ? t("Sandbox: {hosts}", { hosts: hosts.join(", ") }) : t("No sandbox yet")),
    prepareFailed: t("The exercise could not be prepared."),
    preparing: t("Preparing the environment for this exercise ..."),
    reset: t("Reset exercise"),
    resetTitle: t("Reset this exercise?"),
    resetNote: t("Everything this exercise created is removed and its starting state is restored. Other exercises are not affected."),
    previous: t("Previous exercise"),
    next: t("Next exercise"),
    notePlaceholder: t("Private notes for this exercise ..."),
    flagged: (n) => t("{n} exercise(s) are still flagged for review.", { n }),
    passedHeadline: t("You passed this skills assessment."),
    reportSummary: (p, s, n, time) => t("Pass mark {p}% · {s} of {n} exercises fully solved · time used {time}", { p, s, n, time }),
    reportSandbox: t("The sandbox is still running, so you can inspect what you built and retry exercises using the solutions."),
    reportList: t("Exercise by exercise"),
    tutorPlaceholder: t("Ask about this exercise ..."),
    tutorNote: t("The tutor sees the exercise and your check results."),
  }),

  theory: () => ({
    kindLabel: t("Theory"),
    kindNote: t("The real exam is multiple choice and closed book: no documentation is allowed, so a mock exam here links none."),
    assessment: t("Mock exam"),
    startAssessment: t("Start mock exam"),
    fullAssessment: t("Full mock exam"),
    assessmentReport: t("Mock exam report"),
    assessmentPassed: t("Mock exam passed"),
    finishAssessment: t("Finish mock exam"),
    finishTitle: t("Finish the mock exam?"),
    preparingTitle: t("Preparing your mock exam"),
    gradingTitle: t("Grading your mock exam"),
    readiness: t("Exam readiness"),
    readinessNote: (p) => t("Readiness is your latest result per question, weighted like the real exam. Pass mark: {p}%.", { p }),
    questions: (n) => t("{n} questions", { n }),
    tasks: (n) => t("{n} questions", { n }),
    duration: (n) => t("{n} min mock exam", { n }),
    assessmentSummary: (q, m) => t("{q} questions in {m} minutes, weighted like the real exam. No hints. Graded when you finish.", { q, m }),
    assessmentOption: (q, m) => t("{q} questions · {m} minutes", { q, m }),
    briefingSummary: (n, m, p) => t("{n} questions · {m} minutes · pass mark {p}%", { n, m, p }),
    briefing: [
      t("Every question is multiple choice. You can change an answer until you finish."),
      t("Questions are independent. Skip what blocks you, flag it, and come back later."),
      t("There is no feedback during the mock exam. Everything is graded when you finish or when the time is up."),
      t("The real exam is closed book: answer without looking anything up."),
    ],
    startNote: t("The timer starts only when you say so."),
    allSolved: t("Every question is answered correctly. Take a mock exam to prove it under time pressure."),
    noQuestions: t("No questions yet."),
    practiseDomain: (n) => t("{n} multiple-choice questions", { n }),
    provisioningNote: "",
    assessmentProvisioningNote: "",
    gradingNote: "",
    environmentReady: t("Ready"),
    environmentFailed: t("The mock exam could not be prepared."),
    leaveAfterFailure: t("Go back"),
    activeSession: () => t("No sandbox needed"),
    prepareFailed: t("The question could not be prepared."),
    preparing: t("Preparing this question ..."),
    reset: t("Reset question"),
    resetTitle: t("Reset this question?"),
    resetNote: t("Your answer and its result are cleared and the options are shuffled again. Other questions are not affected."),
    previous: t("Previous question"),
    next: t("Next question"),
    notePlaceholder: t("Private notes for this question ..."),
    flagged: (n) => t("{n} question(s) are still flagged for review.", { n }),
    passedHeadline: t("You would have passed this exam."),
    reportSummary: (p, s, n, time) => t("Pass mark {p}% · {s} of {n} questions answered correctly · time used {time}", { p, s, n, time }),
    reportSandbox: "",
    reportList: t("Question by question"),
    tutorPlaceholder: t("Ask about this question ..."),
    tutorNote: t("The tutor sees the question and your result."),
  }),
};

function kindOf(track: Track | null | undefined): Kind {
  return track?.kind === "skills" || track?.kind === "theory" ? track.kind : "certification";
}

/**
 * The strings for a track. Without a track (still loading) the certification
 * wording applies, which is what every page showed before kinds existed.
 */
export function words(track: Track | null | undefined): Words {
  const kind = kindOf(track);
  // A certification track without its own list is about Kubernetes; other
  // kinds link only what their track declares.
  const docs = track?.docs?.[0] ?? (kind === "certification" ? kubernetesDocs() : null);
  return {
    ...wording[kind](),
    kind,
    sandbox: kind !== "theory",
    kubernetes: track?.kubernetesVersion ? t("Kubernetes {v}", { v: track.kubernetesVersion }) : null,
    docs,
    assessmentDocs: kind === "theory" ? null : docs,
  };
}

/** The short round offered next to the full assessment. */
export function quickRound(track: Track): { questions: number; minutes: number } {
  const questions = Math.min(5, track.questions?.length ?? 0);
  // Certification tasks keep the 35 minutes the quick round always had; other
  // kinds get the pace of their full assessment.
  const minutes = kindOf(track) === "certification" || track.exam.questions === 0
    ? 35
    : Math.max(1, Math.ceil((track.exam.durationMinutes * questions) / track.exam.questions));
  return { questions, minutes };
}

/** Whether the learner passed at least one assessment of the track. */
export function assessmentPassed(progress: Progress | null | undefined): boolean {
  return !!progress?.exams?.some((e) => e.passed);
}

/** Whether a question can be practised here: choice questions need no sandbox, tasks need the local one. */
export function practicable(q: QuestionMeta): boolean {
  return q.type === "choice" || q.driver === "kind";
}

/**
 * The learner's standing on a question, 0..1, or undefined before the first
 * attempt. A choice question counts by its latest attempt (a right pick after
 * seeing the answer proves little), a task by its best.
 */
export function standing(progress: Progress | null | undefined, q: QuestionMeta): number | undefined {
  const p = progress?.questions[q.id];
  if (!p) return undefined;
  return q.type === "choice" ? p.latest : p.best;
}
