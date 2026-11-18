import { createElement } from "react";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// xterm needs a real canvas; a stub that records its options is enough here.
// What xterm does with real key events is covered in e2e/keyboard.spec.ts.
const xterm = vi.hoisted(() => ({ options: [] as Record<string, unknown>[] }));
vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    cols = 80; rows = 24;
    constructor(options: Record<string, unknown>) { xterm.options.push(options); }
    loadAddon() {} open() {} focus() {} write() {} dispose() {}
    attachCustomKeyEventHandler() {}
    onData() { return { dispose() {} }; }
    onResize() { return { dispose() {} }; }
  },
}));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class { fit() {} } }));
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class {} }));

import { isMac, shortcutModifiers, shortcutOf, Terminal } from "./Terminal";

type Mods = Partial<Record<"altKey" | "shiftKey" | "ctrlKey" | "metaKey" | "altGraph", boolean>>;
const key = (code: string, mods: Mods, type = "keydown") => ({
  type, code, altKey: false, shiftKey: false, ctrlKey: false, metaKey: false, ...mods,
  getModifierState: (name: string) => name === "AltGraph" && !!mods.altGraph,
});

const MAC = true, OTHER = false;
const ctrlOption: Mods = { ctrlKey: true, altKey: true };
const altShift: Mods = { altKey: true, shiftKey: true };

afterEach(cleanup);

describe("isMac", () => {
  it("recognises macOS from either platform field", () => {
    expect(isMac({ platform: "MacIntel" })).toBe(true);
    expect(isMac({ platform: "", userAgentData: { platform: "macOS" } })).toBe(true);
    expect(isMac({ platform: "Win32" })).toBe(false);
    expect(isMac({ platform: "Linux x86_64" })).toBe(false);
    expect(isMac({})).toBe(false);
  });
});

describe("shortcutOf", () => {
  it("uses Ctrl+Option on macOS", () => {
    expect(shortcutModifiers(MAC)).toBe("Ctrl+Option");
    expect(shortcutOf(key("KeyT", ctrlOption), MAC)).toBe("new");
    expect(shortcutOf(key("KeyW", ctrlOption), MAC)).toBe("close");
    expect(shortcutOf(key("ArrowRight", ctrlOption), MAC)).toBe("next");
    expect(shortcutOf(key("ArrowLeft", ctrlOption), MAC)).toBe("previous");
    expect(shortcutOf(key("Digit3", ctrlOption), MAC)).toEqual({ select: 3 });
  });

  it("uses Alt+Shift everywhere else", () => {
    expect(shortcutModifiers(OTHER)).toBe("Alt+Shift");
    expect(shortcutOf(key("KeyT", altShift), OTHER)).toBe("new");
    expect(shortcutOf(key("KeyW", altShift), OTHER)).toBe("close");
    expect(shortcutOf(key("ArrowRight", altShift), OTHER)).toBe("next");
    expect(shortcutOf(key("ArrowLeft", altShift), OTHER)).toBe("previous");
    expect(shortcutOf(key("Digit3", altShift), OTHER)).toEqual({ select: 3 });
  });

  // Issue #5: every one of these types a character and must reach the shell.
  it.each([
    ["Option+5 [", "Digit5", { altKey: true }],
    ["Option+6 ]", "Digit6", { altKey: true }],
    ["Option+7 |", "Digit7", { altKey: true }],
    ["Option+8 {", "Digit8", { altKey: true }],
    ["Option+9 }", "Digit9", { altKey: true }],
    ["Option+L @", "KeyL", { altKey: true }],
    ["Option+N ~", "KeyN", { altKey: true }],
    ["Option+Shift+7 \\", "Digit7", { altKey: true, shiftKey: true }],
    ["Option+Shift+1…9", "Digit1", { altKey: true, shiftKey: true }],
    ["Option+Shift+T", "KeyT", { altKey: true, shiftKey: true }],
    ["Option+Shift+W", "KeyW", { altKey: true, shiftKey: true }],
    ["Shift+7 /", "Digit7", { shiftKey: true }],
  ] as [string, string, Mods][])("German macOS: %s is not a shortcut", (_name, code, mods) => {
    expect(shortcutOf(key(code, mods), MAC)).toBeNull();
  });

  it.each([
    ["Option+3 £", "Digit3", { altKey: true }],
    ["Option+Shift+2 €", "Digit2", { altKey: true, shiftKey: true }],
    ["Shift+Backslash |", "Backslash", { shiftKey: true }],
  ] as [string, string, Mods][])("US macOS: %s is not a shortcut", (_name, code, mods) => {
    expect(shortcutOf(key(code, mods), MAC)).toBeNull();
  });

  // Windows reports AltGr as Ctrl+Alt, Linux as the AltGraph modifier alone;
  // browsers differ on whether altKey is also set, so every form is covered.
  it.each([
    ["Windows AltGr+7 {", "Digit7", { ctrlKey: true, altKey: true, altGraph: true }],
    ["Windows AltGr+< |", "IntlBackslash", { ctrlKey: true, altKey: true, altGraph: true }],
    ["Windows AltGr+Q @", "KeyQ", { ctrlKey: true, altKey: true, altGraph: true }],
    ["Windows Ctrl+Alt+8 [", "Digit8", { ctrlKey: true, altKey: true }],
    ["Linux AltGr+7 {", "Digit7", { altGraph: true }],
    ["Linux AltGr+Shift+7", "Digit7", { altGraph: true, shiftKey: true }],
    ["AltGr+Shift+7 with altKey set", "Digit7", { altGraph: true, altKey: true, shiftKey: true }],
    ["AltGr+Shift+T with altKey set", "KeyT", { altGraph: true, altKey: true, shiftKey: true }],
  ] as [string, string, Mods][])("German Windows/Linux: %s is not a shortcut", (_name, code, mods) => {
    expect(shortcutOf(key(code, mods), OTHER)).toBeNull();
  });

  it("leaves the shell's own keys alone on both platforms", () => {
    for (const mac of [MAC, OTHER]) {
      expect(shortcutOf(key("KeyT", { altKey: true }), mac)).toBeNull(); // Alt+T: readline transpose-words
      expect(shortcutOf(key("KeyW", { ctrlKey: true }), mac)).toBeNull(); // Ctrl+W: delete word
      expect(shortcutOf(key("KeyC", { ctrlKey: true }), mac)).toBeNull();
      expect(shortcutOf(key("KeyD", { ctrlKey: true }), mac)).toBeNull();
      expect(shortcutOf(key("KeyR", { ctrlKey: true }), mac)).toBeNull();
      expect(shortcutOf(key("ArrowLeft", {}), mac)).toBeNull();
      expect(shortcutOf(key("ArrowRight", { altKey: true }), mac)).toBeNull(); // word movement
      expect(shortcutOf(key("KeyV", { metaKey: true }), mac)).toBeNull(); // paste
    }
  });

  it("ignores other modifier mixes, other keys and keyup", () => {
    expect(shortcutOf(key("KeyT", { ...ctrlOption, shiftKey: true }), MAC)).toBeNull();
    expect(shortcutOf(key("KeyT", { ...ctrlOption, metaKey: true }), MAC)).toBeNull();
    expect(shortcutOf(key("KeyT", { ...altShift, ctrlKey: true }), OTHER)).toBeNull();
    expect(shortcutOf(key("KeyT", { ...altShift, metaKey: true }), OTHER)).toBeNull();
    expect(shortcutOf(key("KeyT", ctrlOption), OTHER)).toBeNull(); // AltGr on Windows
    expect(shortcutOf(key("KeyA", ctrlOption), MAC)).toBeNull();
    expect(shortcutOf(key("Digit0", altShift), OTHER)).toBeNull();
    expect(shortcutOf(key("KeyT", ctrlOption, "keyup"), MAC)).toBeNull();
    expect(shortcutOf(key("KeyT", altShift, "keyup"), OTHER)).toBeNull();
  });
});

describe("Terminal", () => {
  it("does not turn the macOS Option key into Meta", () => {
    vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
    vi.stubGlobal("WebSocket", Object.assign(class { readyState = 0; send() {} close() {} }, { OPEN: 1 }));
    xterm.options.length = 0;
    render(createElement(Terminal, { sessionId: "s1" }));
    expect(xterm.options).toHaveLength(1);
    expect(xterm.options[0].macOptionIsMeta).toBeFalsy();
    vi.unstubAllGlobals();
  });
});
