import { expect, test, type Page } from "@playwright/test";

// What the shell receives for a key press, per keyboard layout and platform.
//
// A browser test cannot switch the operating system's keyboard layout. It
// replays the events a layout produces instead: for Option+7 on a German Mac
// the browser reports key "|", code "Digit7", altKey, and the text "|". That
// is exactly the input xterm.js decides on, so the decision is tested for
// real; the step from physical key to that event is the OS's and is covered
// by the manual check in docs/architecture.md.

const ALT = 1, CTRL = 2, SHIFT = 8;
const MAC = "MacIntel", WINDOWS = "Win32", LINUX = "Linux x86_64";

type Key = { name: string; key: string; code: string; keyCode: number; modifiers: number; text?: string };

/** Opens the terminal as `platform` and returns what it sends and how to type. */
async function open(page: Page, platform: string) {
  await page.addInitScript((p) => {
    Object.defineProperty(navigator, "platform", { get: () => p });
  }, platform);
  const sent: string[] = [];
  await page.routeWebSocket(/\/api\/sessions\/e2e\/terminal/, (ws) => {
    ws.onMessage((m) => { if (typeof m !== "string") sent.push(m.toString("utf8")); }); // text frames are resizes
  });
  await page.goto("/e2e/harness.html");
  await page.locator(".xterm-helper-textarea").focus();
  const cdp = await page.context().newCDPSession(page);
  const press = async (k: Key) => {
    const ev = { key: k.key, code: k.code, modifiers: k.modifiers, windowsVirtualKeyCode: k.keyCode, nativeVirtualKeyCode: k.keyCode };
    await cdp.send("Input.dispatchKeyEvent", { type: "keyDown", ...ev, ...(k.text ? { text: k.text, unmodifiedText: k.text } : {}) });
    await cdp.send("Input.dispatchKeyEvent", { type: "keyUp", ...ev });
  };
  /** Everything sent since the last call. */
  const take = async () => {
    await page.waitForTimeout(50); // let xterm flush
    return sent.splice(0).join("");
  };
  const shortcuts = () => page.evaluate(() => (window as unknown as { shortcuts: unknown[] }).shortcuts);
  return { press, take, shortcuts };
}

const digit = (n: number) => ({ code: `Digit${n}`, keyCode: 48 + n });
const letter = (c: string) => ({ code: `Key${c}`, keyCode: c.charCodeAt(0) });
const typed = (name: string, text: string, where: { code: string; keyCode: number }, modifiers: number): Key =>
  ({ name, key: text, text, modifiers, ...where });

const germanMac: Key[] = [
  typed("Option+7", "|", digit(7), ALT),
  typed("Option+5", "[", digit(5), ALT),
  typed("Option+6", "]", digit(6), ALT),
  typed("Option+8", "{", digit(8), ALT),
  typed("Option+9", "}", digit(9), ALT),
  typed("Option+L", "@", letter("L"), ALT),
  typed("Option+Shift+7", "\\", digit(7), ALT | SHIFT),
  typed("Option+N then Space", "~", { code: "Space", keyCode: 32 }, 0),
  typed("Shift+4", "$", digit(4), SHIFT),
  typed("Shift+backtick key then Space", "`", { code: "Space", keyCode: 32 }, 0),
  typed("Shift+7", "/", digit(7), SHIFT),
  typed("#", "#", { code: "Backslash", keyCode: 220 }, 0),
];

const usMac: Key[] = [
  typed("Shift+Backslash", "|", { code: "Backslash", keyCode: 220 }, SHIFT),
  typed("Backslash", "\\", { code: "Backslash", keyCode: 220 }, 0),
  typed("Shift+2", "@", digit(2), SHIFT),
  typed("Shift+[", "{", { code: "BracketLeft", keyCode: 219 }, SHIFT),
  typed("]", "]", { code: "BracketRight", keyCode: 221 }, 0),
  typed("Shift+backtick", "~", { code: "Backquote", keyCode: 192 }, SHIFT),
  typed("Shift+4", "$", digit(4), SHIFT),
  typed("backtick", "`", { code: "Backquote", keyCode: 192 }, 0),
];

// Windows reports AltGr as Ctrl+Alt.
const germanWindows: Key[] = [
  typed("AltGr+<", "|", { code: "IntlBackslash", keyCode: 226 }, CTRL | ALT),
  typed("AltGr+Q", "@", letter("Q"), CTRL | ALT),
  typed("AltGr+7", "{", digit(7), CTRL | ALT),
  typed("AltGr+8", "[", digit(8), CTRL | ALT),
  typed("AltGr+9", "]", digit(9), CTRL | ALT),
  typed("AltGr+0", "}", { code: "Digit0", keyCode: 48 }, CTRL | ALT),
  typed("AltGr+ß", "\\", { code: "Minus", keyCode: 219 }, CTRL | ALT),
  typed("AltGr++", "~", { code: "BracketRight", keyCode: 187 }, CTRL | ALT),
  typed("Shift+4", "$", digit(4), SHIFT),
];

// Linux reports AltGr as its own modifier: neither Ctrl nor Alt is set.
const germanLinux: Key[] = [
  typed("AltGr+<", "|", { code: "IntlBackslash", keyCode: 226 }, 0),
  typed("AltGr+Q", "@", letter("Q"), 0),
  typed("AltGr+7", "{", digit(7), 0),
  typed("AltGr+ß", "\\", { code: "Minus", keyCode: 219 }, 0),
];

for (const [layout, platform, keys] of [
  ["German layout on macOS", MAC, germanMac],
  ["US layout on macOS", MAC, usMac],
  ["German layout on Windows", WINDOWS, germanWindows],
  ["German layout on Linux", LINUX, germanLinux],
] as const) {
  test(`${layout}: shell symbols reach the shell as typed`, async ({ page }) => {
    const term = await open(page, platform);
    for (const k of keys) {
      await term.press(k);
      expect(await term.take(), k.name).toBe(k.text);
    }
    expect(await term.shortcuts()).toEqual([]);
  });
}

test("a pipeline can be typed on a German Mac", async ({ page }) => {
  const term = await open(page, MAC);
  await page.keyboard.type("printf ok ");
  await term.press(typed("Option+7", "|", digit(7), ALT));
  await page.keyboard.type(" cat");
  await page.keyboard.press("Enter");
  expect(await term.take()).toBe("printf ok | cat\r");
});

for (const platform of [MAC, WINDOWS]) {
  test(`shell control keys still work (${platform})`, async ({ page }) => {
    const term = await open(page, platform);
    for (const [combo, bytes] of [
      ["Control+c", "\x03"], ["Control+d", "\x04"], ["Control+r", "\x12"], ["Control+w", "\x17"],
      ["Enter", "\r"], ["Tab", "\t"], ["Escape", "\x1b"], ["Backspace", "\x7f"],
      ["ArrowUp", "\x1b[A"], ["ArrowDown", "\x1b[B"], ["ArrowRight", "\x1b[C"], ["ArrowLeft", "\x1b[D"],
    ]) {
      await page.keyboard.press(combo);
      expect(await term.take(), combo).toBe(bytes);
    }
    expect(await term.shortcuts()).toEqual([]);
  });
}

test("pasted text reaches the shell", async ({ page }) => {
  const term = await open(page, MAC);
  await page.locator(".xterm-helper-textarea").evaluate((el) => {
    const data = new DataTransfer();
    data.setData("text/plain", "kubectl get pods | grep -v Running");
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  });
  expect(await term.take()).toBe("kubectl get pods | grep -v Running");
});

const tabKeys = (modifiers: number): [Key, unknown][] => [
  [{ name: "T", key: "t", modifiers, ...letter("T") }, "new"],
  [{ name: "W", key: "w", modifiers, ...letter("W") }, "close"],
  [{ name: "ArrowRight", key: "ArrowRight", code: "ArrowRight", keyCode: 39, modifiers }, "next"],
  [{ name: "ArrowLeft", key: "ArrowLeft", code: "ArrowLeft", keyCode: 37, modifiers }, "previous"],
  [{ name: "2", key: "2", modifiers, ...digit(2) }, { select: 2 }],
];

for (const [name, platform, modifiers] of [
  ["Ctrl+Option on macOS", MAC, CTRL | ALT],
  ["Alt+Shift on Windows", WINDOWS, ALT | SHIFT],
  ["Alt+Shift on Linux", LINUX, ALT | SHIFT],
] as const) {
  test(`tab shortcuts: ${name}`, async ({ page }) => {
    const term = await open(page, platform);
    const want: unknown[] = [];
    for (const [k, action] of tabKeys(modifiers)) {
      await term.press(k);
      want.push(action);
      expect(await term.take(), `${k.name} must not reach the shell`).toBe("");
    }
    expect(await term.shortcuts()).toEqual(want);
  });
}
