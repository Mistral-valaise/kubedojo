import { describe, expect, it } from "vitest";
import type { QuestionMeta } from "./api";
import { cka, kcna, linux, progress } from "./fixtures";
import { plural } from "./i18n";
import { assessmentPassed, isFullAssessment, quickRound, words, type Words } from "./track";

/** Every string a track of this kind can show, the parametrised ones with sample numbers. */
function everything(w: Words): string {
  return Object.values(w).flatMap((v) => {
    if (typeof v !== "function") return [v];
    return [(v as (...a: unknown[]) => string)(v === w.activeSession ? ["box1"] : 3, 45, 70, "1:00"), v === w.activeSession ? w.activeSession([]) : ""];
  }).flat().filter((v) => typeof v === "string").join("\n");
}

describe("words", () => {
  it("keeps the wording the certification tracks always had", () => {
    const w = words(cka);
    expect(w.kind).toBe("certification");
    expect(w.kindLabel).toBe("Hands-on");
    expect(w.kindNote).toBeNull();
    expect(w.kubernetes).toBe("Kubernetes v1.37");
    expect(w.assessment).toBe("Exam simulation");
    expect(w.startAssessment).toBe("Start exam simulation");
    expect(w.readiness).toBe("Exam readiness");
    expect(w.questions(42)).toBe("42 questions");
    expect(w.tasks(17)).toBe("17 tasks");
    expect(w.duration(120)).toBe("120 min exam");
    expect(w.assessmentSummary(17, 120)).toBe("17 tasks in 120 minutes, weighted like the real exam. No hints. Graded when you finish.");
    expect(w.provisioningNote).toBe("A real Kubernetes cluster is being created for you. This takes about a minute.");
    expect(w.activeSession(["cka1", "cka2"])).toBe("Clusters: cka1, cka2");
    expect(w.activeSession([])).toBe("No cluster yet");
    expect(w.briefing).toHaveLength(4);
    expect(w.briefing[3]).toBe("You may use the official Kubernetes documentation, like in the real exam.");
    expect(w.sandbox).toBe(true);
    // No docs in track.yaml: a certification track is about Kubernetes.
    expect(w.docs).toEqual({ title: "Kubernetes docs", url: "https://kubernetes.io/docs/" });
    expect(w.assessmentDocs).toEqual(w.docs);
  });

  it("words a skills track without clusters, exams or Kubernetes", () => {
    const w = words(linux);
    expect(w.kind).toBe("skills");
    expect(w.kindLabel).toBe("Foundation");
    expect(w.kubernetes).toBeNull();
    expect(w.assessment).toBe("Skills assessment");
    expect(w.questions(40)).toBe("40 exercises");
    expect(w.tasks(15)).toBe("15 exercises");
    expect(w.provisioningNote).toBe("A Linux sandbox is being started for you. This takes a few seconds.");
    expect(w.kindNote).toMatch(/kubedojo skills track, not a certification exam/);
    expect(w.sandbox).toBe(true);
    expect(w.docs).toBeNull();
    expect(everything(w)).toContain("Sandbox: box1");
    expect(everything(w)).not.toMatch(/cluster|Kubernetes|real exam/i);
  });

  it("words a theory track without any sandbox and as closed book", () => {
    const w = words(kcna);
    expect(w.kind).toBe("theory");
    expect(w.kindLabel).toBe("Theory");
    expect(w.assessment).toBe("Mock exam");
    expect(w.questions(120)).toBe("120 questions");
    expect(w.tasks(30)).toBe("30 questions");
    expect(w.sandbox).toBe(false);
    expect(w.provisioningNote).toBe("");
    expect(w.assessmentProvisioningNote).toBe("");
    expect(w.gradingNote).toBe("");
    expect(w.reportSandbox).toBe("");
    expect(w.kindNote).toMatch(/closed book/);
    expect(w.briefing.join(" ")).toMatch(/closed book/);
    // The track may link study material, but never during a mock exam.
    expect(w.docs).toEqual({ title: "KCNA curriculum", url: "https://example.org/kcna" });
    expect(w.assessmentDocs).toBeNull();
    expect(everything(w)).toContain("3 questions in 45 minutes");
    expect(everything(w)).not.toMatch(/cluster|ssh|host|terminal|sandbox is|Linux/i);
  });

  it("falls back to the certification wording while the track is loading or of an unknown kind", () => {
    expect(words(null).assessment).toBe("Exam simulation");
    expect(words(null).kubernetes).toBeNull();
    expect(words({ ...cka, kind: "other" as never }).kind).toBe("certification");
  });

  it("prefers the documentation a track declares", () => {
    const w = words({ ...linux, docs: [{ title: "Debian manual pages", url: "https://manpages.debian.org/" }] });
    expect(w.docs?.title).toBe("Debian manual pages");
    expect(w.assessmentDocs).toEqual(w.docs);
  });
});

describe("quickRound", () => {
  const q = (n: number) => Array.from({ length: n }, () => ({}) as never);

  it("keeps five tasks in 35 minutes for certification tracks", () => {
    expect(quickRound({ ...cka, questions: q(40) })).toEqual({ questions: 5, minutes: 35 });
    expect(quickRound({ ...cka, questions: q(3) })).toEqual({ questions: 3, minutes: 35 });
  });

  it("paces other kinds like their full assessment", () => {
    expect(quickRound({ ...linux, questions: q(40) })).toEqual({ questions: 5, minutes: 20 });
    expect(quickRound({ ...kcna, questions: q(120) })).toEqual({ questions: 5, minutes: 8 });
  });
});

describe("assessmentPassed", () => {
  it("needs one passed assessment, however many failed", () => {
    expect(assessmentPassed(cka, undefined)).toBe(false);
    expect(assessmentPassed(cka, progress("cka", 90))).toBe(false);
    expect(assessmentPassed(cka, progress("cka", 10, [false, false]))).toBe(false);
    expect(assessmentPassed(cka, progress("cka", 10, [false, true, false]))).toBe(true);
  });

  it("does not count a passed quick round", () => {
    expect(assessmentPassed(cka, progress("cka", 10, ["quick"]))).toBe(false);
    expect(assessmentPassed(cka, progress("cka", 10, ["quick", false]))).toBe(false);
    expect(assessmentPassed(cka, progress("cka", 10, ["quick", true]))).toBe(true);
  });
});

describe("isFullAssessment", () => {
  const q = (n: number) => Array.from({ length: n }, () => ({}) as QuestionMeta);

  it("tells a quick round from the assessment of the track", () => {
    const track = { ...linux, questions: q(40) }; // an assessment has 15 exercises
    expect(isFullAssessment(track, 5)).toBe(false);
    expect(isFullAssessment(track, 14)).toBe(false);
    expect(isFullAssessment(track, 15)).toBe(true);
  });

  it("takes every question for full when the track has fewer than an assessment asks for", () => {
    expect(isFullAssessment({ ...linux, questions: q(4) }, 4)).toBe(true);
    expect(isFullAssessment({ ...linux, questions: q(4) }, 3)).toBe(false);
  });

  it("counts an assessment of unknown size as full, as before sizes were recorded", () => {
    expect(isFullAssessment(linux, undefined)).toBe(true);
    expect(isFullAssessment(linux, 0)).toBe(true);
  });
});

describe("counts", () => {
  it("uses the singular for exactly one", () => {
    expect(plural(1, "{n} item", "{n} items")).toBe("1 item");
    expect([0, 2, 11].map((n) => plural(n, "{n} item", "{n} items"))).toEqual(["0 items", "2 items", "11 items"]);
    expect(plural(1, "{n} of {m} is open", "{n} of {m} are open", { m: 3 })).toBe("1 of 3 is open");
  });

  it("words every count of a track in singular and plural", () => {
    const c = words(cka), l = words(linux), k = words(kcna);
    expect([c.questions(1), c.tasks(1), l.questions(1), l.tasks(2), k.tasks(1)]).toEqual(["1 question", "1 task", "1 exercise", "2 exercises", "1 question"]);
    expect(l.assessmentOption(1, 45)).toBe("1 exercise · 45 minutes");
    expect(k.assessmentOption(5, 1)).toBe("5 questions · 1 minute");
    expect(c.briefingSummary(1, 1, 66)).toBe("1 task · 1 minute · pass mark 66%");
    expect(l.practiseDomain(1)).toBe("1 exercise in a Linux sandbox");
  });

  it("says how many items are still flagged, without a bracketed plural", () => {
    expect(words(linux).flagged(1)).toBe("1 exercise is still flagged for review.");
    expect(words(linux).flagged(2)).toBe("2 exercises are still flagged for review.");
    expect(words(kcna).flagged(2)).toBe("2 questions are still flagged for review.");
    expect(words(cka).flagged(1)).toBe("1 task is still flagged for review.");
  });

  it("names the position in an assessment by kind", () => {
    expect(words(linux).position(1, 5)).toBe("Exercise 1 of 5");
    expect(words(cka).position(2, 17)).toBe("Question 2 of 17");
    expect(words(kcna).position(3, 30)).toBe("Question 3 of 30");
  });

  it("explains a slow check with the sandbox of the track", () => {
    expect(words(linux).stillChecking).toMatch(/for the sandbox to settle/);
    expect(words(cka).stillChecking).toMatch(/for the cluster to settle/);
  });
});
