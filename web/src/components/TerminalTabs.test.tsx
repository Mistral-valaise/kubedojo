import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { TerminalShortcut } from "./Terminal";

// A stand-in for the xterm terminal: it records how often each shell was
// started and ended, which is what tab handling must get right.
const shells = { started: 0, ended: 0, shortcut: null as null | ((s: TerminalShortcut) => void) };

vi.mock("./Terminal", async () => {
  const { useEffect, useState } = await import("react");
  return {
    Terminal: ({ sessionId, active, onShortcut }: { sessionId: string; active: boolean; onShortcut: (s: TerminalShortcut) => void }) => {
      const [id] = useState(() => ++shells.started);
      useEffect(() => () => { shells.ended++; }, []);
      if (active) shells.shortcut = onShortcut;
      return <div data-testid="shell" data-shell={id} data-session={sessionId} data-active={String(active)} />;
    },
    shortcutModifiers: () => "Alt+Shift",
  };
});

const config = vi.fn();
vi.mock("../api", () => ({ api: { config: () => config() } }));

import { TerminalTabs } from "./TerminalTabs";

const tabs = () => screen.getAllByRole("tab");
const names = () => tabs().map((x) => x.textContent);
const selected = () => tabs().find((x) => x.getAttribute("aria-selected") === "true")?.textContent;
const add = () => screen.getByRole("button", { name: "New terminal" });
const activeShell = () => screen.getAllByTestId("shell").find((x) => x.dataset.active === "true")?.dataset.shell;
const shortcut = (s: TerminalShortcut) => act(() => shells.shortcut?.(s));

beforeEach(() => {
  shells.started = 0;
  shells.ended = 0;
  shells.shortcut = null;
  config.mockReset().mockResolvedValue({ maxTerminals: 4 });
});
afterEach(cleanup);

describe("TerminalTabs", () => {
  it("starts with a single terminal that cannot be closed", () => {
    render(<TerminalTabs sessionId="s1" max={4} />);
    expect(names()).toEqual(["Terminal 1"]);
    expect(selected()).toBe("Terminal 1");
    expect(screen.queryByRole("button", { name: /^Close/ })).toBeNull();
    expect(shells.started).toBe(1);
  });

  it("opens another shell with + and shows it", () => {
    render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    expect(names()).toEqual(["Terminal 1", "Terminal 2"]);
    expect(selected()).toBe("Terminal 2");
    expect(shells.started).toBe(2);
    expect(shells.ended).toBe(0);
  });

  it("keeps every shell alive when switching tabs", () => {
    render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    const second = activeShell();
    fireEvent.click(screen.getByRole("tab", { name: "Terminal 1" }));
    expect(selected()).toBe("Terminal 1");
    fireEvent.click(screen.getByRole("tab", { name: "Terminal 2" }));
    // Same shell as before: it was hidden, not restarted.
    expect(activeShell()).toBe(second);
    expect(shells.started).toBe(2);
    expect(shells.ended).toBe(0);
    // The hidden one keeps its layout box so it can be resized while hidden.
    const hidden = screen.getAllByRole("tabpanel", { hidden: true }).find((p) => p.getAttribute("aria-hidden") === "true");
    expect(hidden?.className).toContain("invisible");
    expect(hidden?.className).not.toContain("hidden ");
  });

  it("only one terminal is active at a time", () => {
    render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    fireEvent.click(add());
    expect(screen.getAllByTestId("shell").filter((x) => x.dataset.active === "true")).toHaveLength(1);
  });

  it("closing a tab ends exactly its shell", () => {
    render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    fireEvent.click(add());
    fireEvent.click(screen.getByRole("button", { name: "Close Terminal 2" }));
    expect(names()).toEqual(["Terminal 1", "Terminal 3"]);
    expect(shells.ended).toBe(1);
    // A background tab was closed: the visible one stays.
    expect(selected()).toBe("Terminal 3");
  });

  it("closing the visible tab shows its neighbour", () => {
    render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    fireEvent.click(add());
    fireEvent.click(screen.getByRole("button", { name: "Close Terminal 3" }));
    expect(selected()).toBe("Terminal 2");
    fireEvent.click(screen.getByRole("tab", { name: "Terminal 1" }));
    fireEvent.click(screen.getByRole("button", { name: "Close Terminal 1" }));
    expect(names()).toEqual(["Terminal 2"]);
    expect(selected()).toBe("Terminal 2");
    expect(screen.queryByRole("button", { name: /^Close/ })).toBeNull();
  });

  it("does not reuse the name of a closed terminal", () => {
    render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    fireEvent.click(screen.getByRole("button", { name: "Close Terminal 2" }));
    fireEvent.click(add());
    expect(names()).toEqual(["Terminal 1", "Terminal 3"]);
  });

  it("enforces the limit and frees it when a tab is closed", () => {
    render(<TerminalTabs sessionId="s1" max={2} />);
    fireEvent.click(add());
    expect(add()).toHaveProperty("disabled", true);
    expect(add().getAttribute("title")).toContain("At most 2 terminals");
    fireEvent.click(add());
    expect(shells.started).toBe(2);
    fireEvent.click(screen.getByRole("button", { name: "Close Terminal 1" }));
    expect(add()).toHaveProperty("disabled", false);
  });

  it("takes the limit from the server configuration", async () => {
    config.mockResolvedValue({ maxTerminals: 1 });
    render(<TerminalTabs sessionId="s1" />);
    await act(async () => { await Promise.resolve(); });
    expect(add()).toHaveProperty("disabled", true);
  });

  it("keeps the default limit when the configuration cannot be loaded", async () => {
    config.mockRejectedValue(new Error("offline"));
    render(<TerminalTabs sessionId="s1" />);
    await act(async () => { await Promise.resolve(); });
    for (let i = 0; i < 5; i++) fireEvent.click(add());
    expect(tabs()).toHaveLength(4);
  });

  it("ends every shell when it is unmounted (navigation, exam finished)", () => {
    const view = render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    fireEvent.click(add());
    view.unmount();
    expect(shells.started).toBe(3);
    expect(shells.ended).toBe(3);
  });

  it("starts over with one terminal for another sandbox", () => {
    const view = render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    view.rerender(<TerminalTabs sessionId="s2" max={4} />);
    expect(names()).toEqual(["Terminal 1"]);
    expect(shells.ended).toBe(2);
    expect(screen.getByTestId("shell").dataset.session).toBe("s2");
  });

  it("handles the keyboard shortcuts of the focused terminal", () => {
    render(<TerminalTabs sessionId="s1" max={3} />);
    shortcut("new");
    shortcut("new");
    expect(names()).toEqual(["Terminal 1", "Terminal 2", "Terminal 3"]);
    shortcut("new"); // at the limit
    expect(tabs()).toHaveLength(3);
    shortcut("next"); // wraps around
    expect(selected()).toBe("Terminal 1");
    shortcut("previous");
    expect(selected()).toBe("Terminal 3");
    shortcut({ select: 2 });
    expect(selected()).toBe("Terminal 2");
    shortcut({ select: 9 }); // no such tab
    expect(selected()).toBe("Terminal 2");
    shortcut("close");
    expect(names()).toEqual(["Terminal 1", "Terminal 3"]);
    shortcut("close");
    shortcut("close"); // the last one stays
    expect(tabs()).toHaveLength(1);
  });

  it("moves between tabs with the arrow keys in the tab bar", () => {
    render(<TerminalTabs sessionId="s1" max={4} />);
    fireEvent.click(add());
    fireEvent.click(add());
    const bar = screen.getByRole("tablist", { name: "Terminals" });
    fireEvent.keyDown(within(bar).getByRole("tab", { name: "Terminal 3" }), { key: "ArrowRight" });
    expect(selected()).toBe("Terminal 1");
    expect(document.activeElement?.textContent).toBe("Terminal 1");
    fireEvent.keyDown(within(bar).getByRole("tab", { name: "Terminal 1" }), { key: "End" });
    expect(selected()).toBe("Terminal 3");
    // Only the selected tab is in the tab order.
    expect(tabs().map((x) => x.tabIndex)).toEqual([-1, -1, 0]);
  });
});
