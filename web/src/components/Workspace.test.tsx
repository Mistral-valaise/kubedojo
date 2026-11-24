import { useEffect } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { Split } from "./Split";
import { type Pane, Workspace } from "./Workspace";

/** jsdom has no ResizeObserver. This one reports the width the test sets, at once and on every resize(). */
const observed = { width: 0, by: new Set<FakeResizeObserver>() };
class FakeResizeObserver {
  callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) { this.callback = callback; observed.by.add(this); }
  observe() { this.report(); }
  unobserve() { /* one element only */ }
  disconnect() { observed.by.delete(this); }
  report() { this.callback([{ contentRect: { width: observed.width } } as ResizeObserverEntry], this as unknown as ResizeObserver); }
}
const resize = (width: number) => act(() => {
  observed.width = width;
  observed.by.forEach((o) => o.report());
});

beforeEach(() => {
  observed.by.clear();
  observed.width = 1280;
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
  localStorage.clear();
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

type Life = { mounts: number; unmounts: number };
/** Stands for a terminal: counts how often it is mounted and unmounted. */
function Probe({ life, text }: { life: Life; text: string }) {
  useEffect(() => { life.mounts++; return () => { life.unmounts++; }; }, [life]);
  return <div>{text}</div>;
}

const lives = () => ({ task: { mounts: 0, unmounts: 0 }, shell: { mounts: 0, unmounts: 0 }, chat: { mounts: 0, unmounts: 0 } });
const three = (life = lives(), open?: boolean): Pane[] => [
  { id: "task", label: "Task", content: <Probe life={life.task} text="task" /> },
  { id: "terminal", label: "Terminal", content: <Probe life={life.shell} text="shell" /> },
  { id: "assistant", label: "Assistant", content: <Probe life={life.chat} text="chat" />, open },
];
const once = { mounts: 1, unmounts: 0 };

const paneOf = (text: string) => screen.getByText(text).parentElement as HTMLElement;
const classes = (el: HTMLElement) => el.className.split(" ");
/** Out of view: hidden by visibility, which keeps the pane mounted and sized. */
const hidden = (text: string) => classes(paneOf(text)).includes("invisible");
const inView = () => ["task", "shell", "chat"].filter((x) => !hidden(x));
const layout = () => paneOf("shell").parentElement?.parentElement?.dataset.layout;
const tabNames = (list: HTMLElement) => within(list).getAllByRole("tab").map((x) => x.textContent);
const column = (text: string) => paneOf(text).style.gridColumn;

describe("Workspace with two panes", () => {
  const panes: Pane[] = [
    { id: "left", label: "Task", content: <div>task</div> },
    { id: "right", label: "Terminal", content: <div>shell</div> },
  ];

  it("renders what Split renders", () => {
    const split = render(<Split left={<div>task</div>} right={<div>shell</div>} />).container.innerHTML;
    cleanup();
    expect(render(<Workspace panes={panes} />).container.innerHTML).toBe(split);
    // The whole of it, so that a change of the two-pane markup is a decision.
    expect(split).toBe(
      '<div class="flex min-h-0 flex-1 flex-col">'
      + '<div role="tablist" aria-label="Panes" class="flex shrink-0 border-b bg-bg md:hidden">'
      + '<button role="tab" id="split-tab-left" aria-selected="true" aria-controls="split-pane-left" tabindex="0" class="relative h-10 flex-1 px-3 text-sm font-medium transition-colors text-text">Task<span aria-hidden="true" class="absolute inset-x-0 -bottom-px h-0.5 bg-accent"></span></button>'
      + '<button role="tab" id="split-tab-right" aria-selected="false" aria-controls="split-pane-right" tabindex="-1" class="relative h-10 flex-1 px-3 text-sm font-medium transition-colors text-muted hover:text-text">Terminal</button>'
      + "</div>"
      + '<div class="relative flex min-h-0 flex-1" style="--split: 40%;">'
      + '<div id="split-pane-left" class="absolute inset-0 min-w-0 overflow-hidden md:static md:w-(--split) md:shrink-0"><div>task</div></div>'
      + '<div role="separator" aria-orientation="vertical" tabindex="0" class="group relative hidden w-1.5 shrink-0 cursor-col-resize border-x bg-panel2 transition-colors hover:bg-accent/40 md:block"><span class="absolute left-1/2 top-1/2 h-8 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded bg-muted/50 group-hover:bg-accent"></span></div>'
      + '<div id="split-pane-right" class="absolute inset-0 min-w-0 overflow-hidden md:static max-md:invisible md:flex-1"><div>shell</div></div>'
      + "</div></div>",
    );
  });

  it("follows the md breakpoint and measures nothing", () => {
    render(<Workspace panes={panes} />);
    expect(observed.by.size).toBe(0);
    expect(layout()).toBeUndefined();
    expect(screen.getByRole("tablist").className).toContain("md:hidden");
    expect(screen.getByRole("separator").className).toMatch(/\bhidden\b.*\bmd:block\b/);
    fireEvent.click(screen.getByRole("tab", { name: "Terminal" }));
    expect(classes(paneOf("task"))).toContain("max-md:invisible");
    expect(classes(paneOf("shell"))).not.toContain("max-md:invisible");
  });

  it("remembers the width under its key and leaves the key of the third column alone", () => {
    localStorage.setItem("kubedojo.split.exam", "55");
    render(<Workspace panes={panes} storageKey="kubedojo.split.exam" />);
    fireEvent.keyDown(screen.getByRole("separator"), { key: "ArrowRight" });
    expect(paneOf("task").parentElement?.style.getPropertyValue("--split")).toBe("57%");
    expect(localStorage.getItem("kubedojo.split.exam")).toBe("57");
    expect(localStorage.getItem("kubedojo.split.aside")).toBeNull();
  });
});

describe("Workspace with three panes", () => {
  it("shows one tab per pane below 768 px", () => {
    observed.width = 767;
    render(<Workspace panes={three()} />);
    expect(layout()).toBe("tabs");
    expect(tabNames(screen.getByRole("tablist", { name: "Panes" }))).toEqual(["Task", "Terminal", "Assistant"]);
    // Shown by the measured width, so the tabs do not wait for a breakpoint of the viewport.
    expect(screen.getByRole("tablist").className).not.toContain("md:hidden");
    expect(screen.queryByRole("separator")).toBeNull();
    expect(inView()).toEqual(["task"]);

    fireEvent.click(screen.getByRole("tab", { name: "Assistant" }));
    expect(screen.getByRole("tab", { name: "Assistant" }).getAttribute("aria-selected")).toBe("true");
    expect(inView()).toEqual(["chat"]);
    fireEvent.click(screen.getByRole("tab", { name: "Terminal" }));
    expect(inView()).toEqual(["shell"]);
    // Each pane is the panel of its tab.
    expect(paneOf("shell").getAttribute("role")).toBe("tabpanel");
    expect(paneOf("shell").getAttribute("aria-labelledby")).toBe(screen.getByRole("tab", { name: "Terminal" }).id);
    expect(screen.getByRole("tab", { name: "Terminal" }).getAttribute("aria-controls")).toBe(paneOf("shell").id);
  });

  it("moves through the three tabs with the arrow keys", () => {
    observed.width = 500;
    render(<Workspace panes={three()} />);
    const tab = (name: string) => screen.getByRole("tab", { name });
    tab("Task").focus();
    fireEvent.keyDown(tab("Task"), { key: "ArrowRight" });
    expect(document.activeElement).toBe(tab("Terminal"));
    fireEvent.keyDown(tab("Terminal"), { key: "ArrowRight" });
    fireEvent.keyDown(tab("Assistant"), { key: "ArrowRight" });
    expect(document.activeElement).toBe(tab("Task"));
    fireEvent.keyDown(tab("Task"), { key: "ArrowLeft" });
    expect(document.activeElement).toBe(tab("Assistant"));
    expect(inView()).toEqual(["chat"]);
    expect(screen.getAllByRole("tab").map((x) => x.tabIndex)).toEqual([-1, -1, 0]);
  });

  it("shows two columns from 768 px, with a switcher above the left one", () => {
    observed.width = 768;
    render(<Workspace panes={three()} />);
    expect(layout()).toBe("two");
    expect(screen.queryByRole("tablist", { name: "Panes" })).toBeNull();
    // The terminal keeps the right column; the task and the assistant share the left one.
    expect(tabNames(screen.getByRole("tablist", { name: "Left column" }))).toEqual(["Task", "Assistant"]);
    expect(inView()).toEqual(["task", "shell"]);
    expect([column("task"), column("shell"), column("chat")]).toEqual(["1", "3", "1"]);
    expect(screen.getAllByRole("separator")).toHaveLength(1);

    fireEvent.click(screen.getByRole("tab", { name: "Assistant" }));
    expect(inView()).toEqual(["shell", "chat"]);
    expect(paneOf("chat").getAttribute("role")).toBe("tabpanel");
    expect(paneOf("shell").getAttribute("role")).toBeNull();
    fireEvent.keyDown(screen.getByRole("tab", { name: "Assistant" }), { key: "ArrowRight" });
    expect(inView()).toEqual(["task", "shell"]);
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Task" }));
  });

  it("shows three columns from 1100 px", () => {
    observed.width = 1100;
    render(<Workspace panes={three()} />);
    expect(layout()).toBe("three");
    expect(screen.queryByRole("tablist")).toBeNull();
    expect(inView()).toEqual(["task", "shell", "chat"]);
    expect([column("task"), column("shell"), column("chat")]).toEqual(["1", "3", "5"]);
    expect(paneOf("task").getAttribute("role")).toBeNull();

    const [main, aside] = screen.getAllByRole("separator");
    for (const x of [main, aside]) {
      expect(x.getAttribute("aria-orientation")).toBe("vertical");
      expect(x.tabIndex).toBe(0);
    }
    expect([main.getAttribute("aria-label"), main.getAttribute("aria-valuenow"), main.getAttribute("aria-valuemin"), main.getAttribute("aria-valuemax")])
      .toEqual(["Resize the columns", "40", "22", "70"]);
    expect([aside.getAttribute("aria-label"), aside.getAttribute("aria-valuenow"), aside.getAttribute("aria-valuemin"), aside.getAttribute("aria-valuemax")])
      .toEqual(["Resize Assistant", "380", "320", "560"]);
    // One step below, the third pane has no column of its own.
    resize(1099);
    expect(layout()).toBe("two");
  });

  it("keeps every pane mounted in every layout", () => {
    const life = lives();
    observed.width = 1280;
    render(<Workspace panes={three(life)} />);
    resize(900);
    fireEvent.click(screen.getByRole("tab", { name: "Assistant" }));
    resize(600);
    // What the left column showed is what the tabs show.
    expect(inView()).toEqual(["chat"]);
    fireEvent.click(screen.getByRole("tab", { name: "Terminal" }));
    fireEvent.click(screen.getByRole("tab", { name: "Task" }));
    resize(900);
    expect(inView()).toEqual(["task", "shell"]);
    resize(1400);
    resize(300);
    expect(life).toEqual({ task: once, shell: once, chat: once });
    // Hidden by visibility, which keeps the size of the terminal: never by display.
    for (const x of ["task", "shell", "chat"]) expect(paneOf(x).className).not.toMatch(/(^| )hidden( |$)/);
    expect(inView()).toEqual(["task"]);
  });

  it("hides a third pane that is not open, in every layout, and keeps it mounted", () => {
    const life = lives();
    observed.width = 1280;
    const view = render(<Workspace panes={three(life, false)} />);
    // Wide enough for three columns, but there are two panes to show.
    expect(layout()).toBe("two");
    expect(screen.queryByRole("tablist")).toBeNull();
    expect(screen.getAllByRole("separator")).toHaveLength(1);
    expect(inView()).toEqual(["task", "shell"]);

    resize(600);
    expect(tabNames(screen.getByRole("tablist", { name: "Panes" }))).toEqual(["Task", "Terminal"]);
    expect(inView()).toEqual(["task"]);

    // Opened on a narrow screen and selected, then closed again: the tabs fall back to the task.
    view.rerender(<Workspace panes={three(life, true)} />);
    fireEvent.click(screen.getByRole("tab", { name: "Assistant" }));
    expect(inView()).toEqual(["chat"]);
    view.rerender(<Workspace panes={three(life, false)} />);
    expect(inView()).toEqual(["task"]);

    resize(1280);
    view.rerender(<Workspace panes={three(life, true)} />);
    expect(layout()).toBe("three");
    expect(inView()).toEqual(["task", "shell", "chat"]);
    expect(life).toEqual({ task: once, shell: once, chat: once });
  });

  it("keeps the panes mounted when a third one is added to two", () => {
    const life = lives();
    const view = render(<Workspace panes={three(life).slice(0, 2)} />);
    expect(layout()).toBeUndefined();
    view.rerender(<Workspace panes={three(life)} />);
    expect(layout()).toBe("three");
    view.rerender(<Workspace panes={three(life).slice(0, 2)} />);
    expect(life).toEqual({ task: once, shell: once, chat: { mounts: 1, unmounts: 1 } });
  });

  it("resizes the third column with the keyboard and remembers its width", () => {
    render(<Workspace panes={three()} />);
    const aside = () => screen.getAllByRole("separator")[1];
    expect(localStorage.getItem("kubedojo.split.aside")).toBe("380");
    // The divider is the left edge of the column: moving it left makes the column wider.
    fireEvent.keyDown(aside(), { key: "ArrowLeft" });
    expect(aside().getAttribute("aria-valuenow")).toBe("396");
    expect(localStorage.getItem("kubedojo.split.aside")).toBe("396");
    for (let i = 0; i < 20; i++) fireEvent.keyDown(aside(), { key: "ArrowLeft" });
    expect(aside().getAttribute("aria-valuenow")).toBe("560");
    for (let i = 0; i < 40; i++) fireEvent.keyDown(aside(), { key: "ArrowRight" });
    expect(aside().getAttribute("aria-valuenow")).toBe("320");
    expect(localStorage.getItem("kubedojo.split.aside")).toBe("320");

    // The next workspace starts with the remembered width.
    cleanup();
    localStorage.setItem("kubedojo.split.aside", "500");
    render(<Workspace panes={three()} />);
    expect(aside().getAttribute("aria-valuenow")).toBe("500");
    expect(paneOf("chat").parentElement?.style.gridTemplateColumns).toContain("500px");
  });

  it("keeps a remembered width of the third column within its limits", () => {
    localStorage.setItem("kubedojo.split.aside", "9000");
    render(<Workspace panes={three()} />);
    expect(screen.getAllByRole("separator")[1].getAttribute("aria-valuenow")).toBe("560");
  });

  it("resizes both dividers by dragging", () => {
    render(<Workspace panes={three()} storageKey="kubedojo.split.exam" />);
    const [main, aside] = screen.getAllByRole("separator");
    const rect = (left: number, width: number) => () => ({ left, right: left + width, width, top: 0, bottom: 0, height: 0, x: left, y: 0, toJSON: () => ({}) });
    const body = paneOf("shell").parentElement as HTMLElement;
    body.getBoundingClientRect = rect(100, 1200);
    aside.getBoundingClientRect = rect(900, 6);

    fireEvent.pointerDown(aside);
    expect(classes(body)).toContain("select-none");
    fireEvent(window, new MouseEvent("pointermove", { clientX: 850 }));
    expect(aside.getAttribute("aria-valuenow")).toBe("450");
    fireEvent(window, new MouseEvent("pointerup"));
    expect(classes(body)).not.toContain("select-none");
    expect(localStorage.getItem("kubedojo.split.aside")).toBe("450");
    // Released: moving the pointer changes nothing.
    fireEvent(window, new MouseEvent("pointermove", { clientX: 700 }));
    expect(aside.getAttribute("aria-valuenow")).toBe("450");

    // The first column is a share of what the third one leaves: 400 of the 800 px left of its divider.
    fireEvent.pointerDown(main);
    fireEvent(window, new MouseEvent("pointermove", { clientX: 500 }));
    fireEvent(window, new MouseEvent("pointerup"));
    expect(main.getAttribute("aria-valuenow")).toBe("50");
    expect(localStorage.getItem("kubedojo.split.exam")).toBe("50");
  });

  it("falls back to the width of the viewport without a ResizeObserver", () => {
    vi.unstubAllGlobals();
    expect(typeof ResizeObserver).toBe("undefined");
    const life = lives();
    vi.stubGlobal("innerWidth", 1024);
    render(<Workspace panes={three(life)} />);
    expect(layout()).toBe("two");
    vi.stubGlobal("innerWidth", 640);
    act(() => { window.dispatchEvent(new Event("resize")); });
    expect(layout()).toBe("tabs");
    vi.stubGlobal("innerWidth", 1440);
    act(() => { window.dispatchEvent(new Event("resize")); });
    expect(layout()).toBe("three");
    expect(life).toEqual({ task: once, shell: once, chat: once });
  });
});
