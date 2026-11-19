import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Split } from "./Split";

afterEach(cleanup);

/** Stands for a terminal: counts how often it is mounted and unmounted. */
function Probe({ life }: { life: { mounts: number; unmounts: number } }) {
  useEffect(() => { life.mounts++; return () => { life.unmounts++; }; }, [life]);
  return <div>shell</div>;
}

const paneOf = (text: string) => screen.getByText(text).parentElement as HTMLElement;
// Below md a pane behind the other tab is invisible; from md both are in the row.
const hiddenWhenNarrow = (el: HTMLElement) => el.className.split(" ").includes("max-md:invisible");

describe("Split", () => {
  it("offers the two panes as tabs for a narrow screen, the task first", () => {
    render(<Split left={<div>task</div>} right={<div>shell</div>} />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((x) => x.textContent)).toEqual(["Task", "Terminal"]);
    expect(tabs.map((x) => x.getAttribute("aria-selected"))).toEqual(["true", "false"]);
    // The tabs exist for narrow screens only; the divider for wide ones only.
    expect(screen.getByRole("tablist").className).toContain("md:hidden");
    expect(screen.getByRole("separator").className).toMatch(/\bhidden\b.*\bmd:block\b/);
    expect(hiddenWhenNarrow(paneOf("task"))).toBe(false);
    expect(hiddenWhenNarrow(paneOf("shell"))).toBe(true);
  });

  it("switches tabs without unmounting the terminal", () => {
    const life = { mounts: 0, unmounts: 0 };
    render(<Split left={<div>task</div>} right={<Probe life={life} />} />);
    fireEvent.click(screen.getByRole("tab", { name: "Terminal" }));
    expect(screen.getByRole("tab", { name: "Terminal" }).getAttribute("aria-selected")).toBe("true");
    expect(hiddenWhenNarrow(paneOf("shell"))).toBe(false);
    expect(hiddenWhenNarrow(paneOf("task"))).toBe(true);
    fireEvent.click(screen.getByRole("tab", { name: "Task" }));
    fireEvent.click(screen.getByRole("tab", { name: "Terminal" }));
    expect(life).toEqual({ mounts: 1, unmounts: 0 });
    // Hidden by visibility, which keeps the size of the terminal: never by display.
    expect(paneOf("shell").className).not.toMatch(/(^| )hidden( |$)/);
    expect(paneOf("task").className).not.toMatch(/(^| )hidden( |$)/);
  });

  it("moves between the tabs with the arrow keys", () => {
    render(<Split left={<div>task</div>} right={<div>shell</div>} labels={{ left: "Exercise", right: "Terminal" }} />);
    const first = screen.getByRole("tab", { name: "Exercise" });
    first.focus();
    fireEvent.keyDown(first, { key: "ArrowRight" });
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Terminal" }));
    expect(screen.getByRole("tab", { name: "Terminal" }).getAttribute("aria-selected")).toBe("true");
  });

  it("keeps the remembered width for the wide layout", () => {
    vi.spyOn(Storage.prototype, "getItem").mockReturnValue("55");
    render(<Split left={<div>task</div>} right={<div>shell</div>} />);
    expect(paneOf("task").parentElement?.style.getPropertyValue("--split")).toBe("55%");
    fireEvent.keyDown(screen.getByRole("separator"), { key: "ArrowLeft" });
    expect(paneOf("task").parentElement?.style.getPropertyValue("--split")).toBe("53%");
  });
});
