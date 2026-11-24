import { createElement } from "react";
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The terminal and its font. xterm.js measures the width of a character when
// it opens, so it must not open before the web font is there, and it must
// open all the same when the font never comes. A stub stands for xterm; the
// real one, with real fonts, runs in e2e/keyboard.spec.ts.
const xterm = vi.hoisted(() => ({ options: [] as Record<string, unknown>[], opened: 0 }));
vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    cols = 80; rows = 24;
    constructor(options: Record<string, unknown>) { xterm.options.push(options); }
    open() { xterm.opened++; }
    loadAddon() {} focus() {} write() {} dispose() {}
    attachCustomKeyEventHandler() {}
    onData() { return { dispose() {} }; }
    onResize() { return { dispose() {} }; }
  },
}));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} } }));
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class {} }));

import * as terminal from "./Terminal";
/** The module as a page that has not loaded the font yet: it remembers that, once per page. */
const load = async () => { terminal.resetMonoFontWait(); return terminal; };
/** Gives the document a font loader whose answer the test decides. */
function fontLoader() {
  const requests: string[] = [];
  let settle = { ok: () => undefined as void, fail: () => undefined as void };
  const fonts = {
    load: (font: string) => {
      requests.push(font);
      return new Promise<unknown[]>((resolve, reject) => { settle = { ok: () => resolve([]), fail: () => reject(new Error("network")) }; });
    },
  };
  Object.defineProperty(document, "fonts", { configurable: true, value: fonts });
  return { requests, ok: () => settle.ok(), fail: () => settle.fail() };
}
const tick = () => act(async () => { await Promise.resolve(); await Promise.resolve(); });

beforeEach(() => {
  xterm.options.length = 0;
  xterm.opened = 0;
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  vi.stubGlobal("WebSocket", Object.assign(class { readyState = 0; send() {} close() {} }, { OPEN: 1 }));
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  delete (document as unknown as { fonts?: unknown }).fonts;
  document.documentElement.removeAttribute("style");
});

describe("whenMonoFontReady", () => {
  it("resolves at once where fonts cannot be loaded by script", async () => {
    const { whenMonoFontReady } = await load();
    await expect(whenMonoFontReady(undefined)).resolves.toBeUndefined();
  });

  it("asks for the mono face of the stylesheet at the size of the terminal", async () => {
    const { whenMonoFontReady } = await load();
    const load1 = vi.fn(async () => []);
    await whenMonoFontReady({ load: load1 } as unknown as FontFaceSet);
    expect(load1).toHaveBeenCalledWith('13.5px "JetBrains Mono Variable"');
    // The same family the stylesheet declares, or the wait would be for nothing.
    const css = (await import("../styles/fonts.css?raw")).default;
    expect(css).toContain('font-family: "JetBrains Mono Variable"');
  });

  it("resolves when the font fails to load, and when asking for it throws", async () => {
    const { whenMonoFontReady } = await load();
    await expect(whenMonoFontReady({ load: async () => { throw new Error("network"); } } as unknown as FontFaceSet)).resolves.toBeUndefined();
    await expect(whenMonoFontReady({ load: () => { throw new Error("unsupported"); } } as unknown as FontFaceSet)).resolves.toBeUndefined();
  });

  it("gives up after the wait when the font never answers", async () => {
    const { whenMonoFontReady, FONT_WAIT_MS } = await load();
    vi.useFakeTimers();
    let done = false;
    void whenMonoFontReady({ load: () => new Promise(() => undefined) } as unknown as FontFaceSet).then(() => { done = true; });
    await vi.advanceTimersByTimeAsync(FONT_WAIT_MS - 1);
    expect(done).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(done).toBe(true);
    // Short enough that a missing font is a moment, not a broken terminal.
    expect(FONT_WAIT_MS).toBeLessThanOrEqual(2000);
  });
});

describe("Terminal and its font", () => {
  it("opens only once the font is there", async () => {
    const font = fontLoader();
    const { Terminal } = await load();
    render(createElement(Terminal, { sessionId: "s1" }));
    await tick();
    expect(xterm.options).toHaveLength(1);
    expect(font.requests).toEqual(['13.5px "JetBrains Mono Variable"']);
    expect(xterm.opened).toBe(0);
    font.ok();
    await tick();
    expect(xterm.opened).toBe(1);
  });

  it("opens with the fallback font when the web font cannot be loaded", async () => {
    const font = fontLoader();
    const { Terminal } = await load();
    render(createElement(Terminal, { sessionId: "s1" }));
    await tick();
    font.fail();
    await tick();
    expect(xterm.opened).toBe(1);
    expect(String(xterm.options[0].fontFamily)).toMatch(/^"JetBrains Mono Variable", .*monospace$/);
  });

  it("opens after the wait when the font never answers", async () => {
    fontLoader();
    const { Terminal, FONT_WAIT_MS } = await load();
    vi.useFakeTimers();
    render(createElement(Terminal, { sessionId: "s1" }));
    await act(async () => { await vi.advanceTimersByTimeAsync(FONT_WAIT_MS - 1); });
    expect(xterm.opened).toBe(0);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(xterm.opened).toBe(1);
  });

  it("waits once for all terminals, and opens later ones at once", async () => {
    const font = fontLoader();
    const { Terminal } = await load();
    render(createElement(Terminal, { sessionId: "s1" }));
    render(createElement(Terminal, { sessionId: "s1" }));
    await tick();
    expect(font.requests).toHaveLength(1);
    font.ok();
    await tick();
    expect(xterm.opened).toBe(2);
    // A tab opened later does not wait again.
    render(createElement(Terminal, { sessionId: "s1" }));
    expect(xterm.opened).toBe(3);
    expect(font.requests).toHaveLength(1);
  });

  it("does not open a terminal that was closed while the font loaded", async () => {
    const font = fontLoader();
    const { Terminal } = await load();
    const view = render(createElement(Terminal, { sessionId: "s1" }));
    await tick();
    view.unmount();
    font.ok();
    await tick();
    expect(xterm.opened).toBe(0);
  });
});

describe("terminalTheme", () => {
  it("takes the colours of xterm from the tokens of the stylesheet", async () => {
    const { terminalTheme } = await load();
    const root = document.documentElement;
    root.style.setProperty("--term", " #070a11");
    root.style.setProperty("--term-text", "#d7deec");
    root.style.setProperty("--term-accent", "#5b9dff");
    root.style.setProperty("--term-bright-green", "#6ee7b1");
    expect(terminalTheme()).toEqual({
      background: "#070a11", cursorAccent: "#070a11", foreground: "#d7deec", cursor: "#5b9dff", brightGreen: "#6ee7b1",
    });
  });

  it("leaves a colour to xterm when the stylesheet does not define its token", async () => {
    const { terminalTheme } = await load();
    expect(terminalTheme()).toEqual({});
  });

  it("is what a new terminal is created with", async () => {
    const { Terminal } = await load();
    document.documentElement.style.setProperty("--term", "#070a11");
    render(createElement(Terminal, { sessionId: "s1" }));
    expect(xterm.options[0].theme).toEqual({ background: "#070a11", cursorAccent: "#070a11" });
    // Without a font loader in this browser the terminal opens at once.
    expect(xterm.opened).toBe(1);
  });
});
