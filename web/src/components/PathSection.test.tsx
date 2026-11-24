import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Path, Progress, Track } from "../api";
import { cka, ckad, kcna, linux, path, progress } from "../fixtures";
import { PathSection } from "./PathSection";

const tracks = [linux, kcna, ckad, cka];

function show(p: Path | null, pr?: Record<string, Progress>, list: Track[] = tracks) {
  return render(<MemoryRouter><PathSection path={p} tracks={list} progress={pr} /></MemoryRouter>);
}
const steps = () => within(screen.getByRole("list")).getAllByRole("listitem");

afterEach(cleanup);

describe("PathSection", () => {
  it("renders nothing without a path", () => {
    expect(show(null).container.innerHTML).toBe("");
  });

  it("renders nothing when no step names a known track", () => {
    expect(show(path, undefined, []).container.innerHTML).toBe("");
  });

  it("lists the steps in path order, whatever the order of the tracks", () => {
    show(path, undefined, [cka, ckad, kcna, linux]);
    expect(steps().map((li) => within(li).getByRole("link").getAttribute("href"))).toEqual(["/t/linux", "/t/kcna", "/t/ckad", "/t/cka"]);
    expect(screen.getByRole("heading", { name: "Road to Kubestronaut" })).toBeTruthy();
  });

  it("badges a step by its role and the kind of its track", () => {
    show(path);
    const badge = (i: number) => steps()[i].querySelector("a span span")?.textContent;
    expect([0, 1, 2, 3].map(badge)).toEqual(["Foundation", "Theory", "Hands-on", "Hands-on"]);
  });

  it("links every step without locking any, however little was done", () => {
    show(path, { linux: progress("linux", 0), kcna: progress("kcna", 0), ckad: progress("ckad", 0), cka: progress("cka", 0) });
    const links = within(screen.getByRole("list")).getAllByRole("link");
    expect(links).toHaveLength(4);
    for (const a of links) {
      expect(a.getAttribute("href")).toMatch(/^\/t\//);
      expect(a.getAttribute("aria-disabled")).toBeNull();
      expect(a.getAttribute("tabindex")).toBeNull();
    }
  });

  it("names each link by track, kind, readiness and result", () => {
    show(path, { kcna: progress("kcna", 81.4, [true]), cka: progress("cka", 40) });
    expect(screen.getByRole("link", { name: "KCNA, Kubernetes and Cloud Native Associate, Theory, 81% ready, Mock exam passed" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "CKA, Certified Kubernetes Administrator, Hands-on, 40% ready" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Linux, Linux & Vim Essentials, Foundation, 0% ready" })).toBeTruthy();
  });

  it("ticks only the steps with a passed assessment", () => {
    show(path, { linux: progress("linux", 100, [true]), kcna: progress("kcna", 100), ckad: progress("ckad", 20, [false, true]), cka: progress("cka", 90, [false]) });
    expect(steps().map((li) => within(li).queryAllByTestId("path-passed").length)).toEqual([1, 0, 1, 0]);
  });

  it("counts passed mock exams over the required steps only", () => {
    // The foundation step is passed too, and must change neither number.
    show(path, { linux: progress("linux", 100, [true]), kcna: progress("kcna", 80, [true]), ckad: progress("ckad", 20, [false]) });
    expect(screen.getByText("1 of 3 passed")).toBeTruthy();
  });

  it("names what counts without calling an exam simulation a mock exam", () => {
    show(path);
    const counter = screen.getByText("0 of 3 passed");
    expect(counter.textContent).toBe("0 of 3 passed (full mock exams and exam simulations)");
    expect(counter.getAttribute("title")).toMatch(/^Full mock exams and exam simulations/);
    expect(counter.querySelector(".sr-only")).toBeTruthy();
  });

  it("neither counts nor ticks a passed quick round", () => {
    show(path, { linux: progress("linux", 100, ["quick"]), kcna: progress("kcna", 80, ["quick"]), ckad: progress("ckad", 80, ["quick", true]) });
    expect(screen.getByText("1 of 3 passed")).toBeTruthy();
    expect(steps().map((li) => within(li).queryAllByTestId("path-passed").length)).toEqual([0, 0, 1, 0]);
    expect(screen.getByRole("link", { name: "KCNA, Kubernetes and Cloud Native Associate, Theory, 80% ready" })).toBeTruthy();
  });

  it("counts nothing before progress is loaded", () => {
    show(path);
    expect(screen.getByText("0 of 3 passed")).toBeTruthy();
  });

  it("skips steps whose track the server does not offer", () => {
    show(path, undefined, [linux, cka]);
    expect(steps()).toHaveLength(2);
    expect(screen.getByText("0 of 1 passed")).toBeTruthy();
  });

  it("shows the disclaimer and links the program page, and claims no title", () => {
    const { container } = show(path, { kcna: progress("kcna", 100, [true]), ckad: progress("ckad", 100, [true]), cka: progress("cka", 100, [true]) });
    expect(screen.getByText(path.disclaimer)).toBeTruthy();
    const link = screen.getByRole("link", { name: "Official program page on cncf.io" });
    expect(link.getAttribute("href")).toBe(path.url);
    expect(screen.getByText("3 of 3 passed")).toBeTruthy();
    expect(container.textContent).not.toMatch(/you are|you're|you have become|congratulations/i);
  });

  it("says why a step is on the path: as its description and as its tooltip", () => {
    show(path);
    const link = screen.getByRole("link", { name: /^Linux, / });
    const why = document.getElementById(link.getAttribute("aria-describedby") ?? "");
    expect(why?.textContent).toBe("The shell every exam runs in.");
    // One line per step keeps the card short; the reason is not lost for who cannot hover.
    expect(why?.className).toBe("sr-only");
    expect(link.getAttribute("title")).toBe("The shell every exam runs in.");
  });

  it("shows the readiness of each step as a number and a bar, and a dash before it is known", () => {
    show(path, { kcna: progress("kcna", 81.4) });
    const [linuxStep, kcnaStep] = steps();
    expect(within(linuxStep).getByText("–")).toBeTruthy();
    expect(within(kcnaStep).getByText("81%")).toBeTruthy();
    // Above the pass mark of the track (75%) the bar is the colour of success.
    expect((kcnaStep.querySelector("div.rounded-full > div") as HTMLElement).className).toContain("bg-ok");
    expect((kcnaStep.querySelector("div.rounded-full > div") as HTMLElement).style.width).toBe("81%");
  });
});
