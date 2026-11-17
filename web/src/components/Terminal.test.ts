import { describe, expect, it, vi } from "vitest";

// shortcutOf is pure; keep xterm (which needs a real canvas) out of the test.
vi.mock("@xterm/xterm", () => ({ Terminal: class {} }));
vi.mock("@xterm/addon-fit", () => ({ FitAddon: class {} }));
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class {} }));

import { shortcutOf } from "./Terminal";

const key = (code: string, mods: Partial<Record<"altKey" | "shiftKey" | "ctrlKey" | "metaKey", boolean>> = { altKey: true, shiftKey: true }, type = "keydown") =>
  ({ type, code, altKey: false, shiftKey: false, ctrlKey: false, metaKey: false, ...mods });

describe("shortcutOf", () => {
  it("maps Alt+Shift combinations to tab actions", () => {
    expect(shortcutOf(key("KeyT"))).toBe("new");
    expect(shortcutOf(key("KeyW"))).toBe("close");
    expect(shortcutOf(key("ArrowRight"))).toBe("next");
    expect(shortcutOf(key("ArrowLeft"))).toBe("previous");
    expect(shortcutOf(key("Digit3"))).toEqual({ select: 3 });
  });

  it("leaves everything else to the shell", () => {
    expect(shortcutOf(key("KeyT", { altKey: true }))).toBeNull(); // Alt+T: readline transpose-words
    expect(shortcutOf(key("KeyW", { ctrlKey: true }))).toBeNull(); // Ctrl+W: delete word
    expect(shortcutOf(key("KeyT", { altKey: true, shiftKey: true, ctrlKey: true }))).toBeNull();
    expect(shortcutOf(key("KeyT", { altKey: true, shiftKey: true, metaKey: true }))).toBeNull();
    expect(shortcutOf(key("KeyA"))).toBeNull();
    expect(shortcutOf(key("Digit0"))).toBeNull();
    expect(shortcutOf(key("KeyT", { altKey: true, shiftKey: true }, "keyup"))).toBeNull();
  });
});
