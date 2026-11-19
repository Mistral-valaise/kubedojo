import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { CheckResult, Result } from "../api";
import { Results } from "./Results";

afterEach(cleanup);

const check = (over: Partial<CheckResult>): CheckResult => ({
  id: "c", title: "The file is saved", points: 2, passed: false, expected: 'equals "yes"', observed: "no",
  explain: "Plain words.", command: "cat file", ...over,
});
const show = (...checks: CheckResult[]) => {
  const max = checks.reduce((n, c) => n + c.points, 0);
  const result: Result = { questionId: "q", score: 0, max, passed: checks.every((c) => c.passed), checks, at: "now" };
  return render(<Results result={result} />);
};

describe("Results", () => {
  it("renders the explanation of a check as Markdown, without click-to-copy controls", () => {
    const { container } = show(check({ explain: "`:w` saves, `:wq` saves and quits. See **:help write**." }));
    const codes = [...container.querySelectorAll(".md code")];
    expect(codes.map((c) => c.textContent)).toEqual([":w", ":wq"]);
    expect(container.querySelector(".md strong")?.textContent).toBe(":help write");
    expect(container.querySelector(".md")?.textContent).not.toContain("`");
    // The row is a button already: code inside the explanation is no second control.
    expect(codes.every((c) => c.getAttribute("role") === null && c.getAttribute("tabindex") === null)).toBe(true);
  });

  it("shows expected and observed values as preformatted text with real line breaks", () => {
    const { container } = show(check({ expected: "equals:\nalpha 1\nbeta 2", observed: "alpha 1\n  beta 3\n" }));
    const blocks = [...container.querySelectorAll("dl pre")];
    // Line breaks and indentation are kept; the trailing one adds no empty line.
    expect(blocks.map((b) => b.textContent)).toEqual(["equals:\nalpha 1\nbeta 2", "alpha 1\n  beta 3"]);
    // pre-wrap: the breaks are real, and a long line wraps instead of widening the row.
    expect(blocks.every((b) => b.className.split(" ").includes("whitespace-pre-wrap"))).toBe(true);
    expect(container.textContent).not.toContain("\\n");
  });

  it("shows a value of one line the same way", () => {
    show(check({}));
    expect(screen.getByText('equals "yes"').tagName).toBe("PRE");
    expect(screen.getByText("no").tagName).toBe("PRE");
  });

  it("opens failed checks and keeps passed ones closed", () => {
    show(check({ id: "a", title: "Failed one" }), check({ id: "b", title: "Passed one", passed: true, explain: "Hidden until opened." }));
    expect(screen.getByText("Plain words.")).toBeTruthy();
    expect(screen.queryByText("Hidden until opened.")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /Passed one/ }));
    expect(screen.getByText("Hidden until opened.")).toBeTruthy();
    expect(screen.getByText("0 of 4 points · 0%", { exact: false })).toBeTruthy();
  });
});
