import { describe, expect, it } from "vitest";
import { cka, kcna, linux, progress } from "./fixtures";
import { assessmentPassed, quickRound, words, type Words } from "./track";

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
    expect(assessmentPassed(undefined)).toBe(false);
    expect(assessmentPassed(progress("cka", 90))).toBe(false);
    expect(assessmentPassed(progress("cka", 10, [false, false]))).toBe(false);
    expect(assessmentPassed(progress("cka", 10, [false, true, false]))).toBe(true);
  });
});
