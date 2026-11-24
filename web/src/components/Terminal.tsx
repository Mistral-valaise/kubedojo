import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { RotateCw } from "lucide-react";
import { t } from "../i18n";
import { activateTerminal, registerTerminal } from "../terminalBus";

/** xterm theme keys and the design tokens (styles/tokens.css) they take their colour from. */
const THEME_TOKENS = {
  background: "--term", foreground: "--term-text", cursor: "--term-accent", cursorAccent: "--term",
  selectionBackground: "--term-selection",
  black: "--term-black", red: "--term-red", green: "--term-green", yellow: "--term-yellow", blue: "--term-blue",
  magenta: "--term-magenta", cyan: "--term-cyan", white: "--term-white",
  brightBlack: "--term-bright-black", brightRed: "--term-bright-red", brightGreen: "--term-bright-green",
  brightYellow: "--term-bright-yellow", brightBlue: "--term-bright-blue", brightMagenta: "--term-bright-magenta",
  brightCyan: "--term-bright-cyan", brightWhite: "--term-bright-white",
} as const;

type TerminalTheme = Partial<Record<keyof typeof THEME_TOKENS, string>>;

/**
 * The colours of the terminal, read from the stylesheet. xterm draws with
 * concrete colours, so the tokens are resolved here. The terminal is dark in
 * both themes: its tokens do not change with the theme, and the result needs
 * no refresh when the theme is switched. A token the stylesheet does not
 * define (a test without styles) is left to xterm's own default.
 */
export function terminalTheme(root: Element = document.documentElement): TerminalTheme {
  const style = getComputedStyle(root);
  const theme: TerminalTheme = {};
  for (const [key, token] of Object.entries(THEME_TOKENS) as [keyof typeof THEME_TOKENS, string][]) {
    const value = style.getPropertyValue(token).trim();
    if (value) theme[key] = value;
  }
  return theme;
}

const FONT_SIZE = 13.5;
/** The face fonts.css declares; the rest is what a browser without it falls back to. */
const MONO_FACE = '"JetBrains Mono Variable"';
const FONT_FAMILY = `${MONO_FACE}, "JetBrains Mono", ui-monospace, "SF Mono", Menlo, Consolas, monospace`;
/** How long a terminal waits for its font before it opens with the fallback. */
export const FONT_WAIT_MS = 1500;

/**
 * Resolves once the monospace web font can be measured. xterm.js measures the
 * size of a character cell when it opens: opened while the font is still
 * loading, it would measure the fallback and keep cells of the wrong width.
 * It never rejects and never waits longer than `wait`, so a font that fails
 * to load costs a moment and the terminal still opens.
 */
export function whenMonoFontReady(fonts: Pick<FontFaceSet, "load"> | undefined = document.fonts, wait = FONT_WAIT_MS): Promise<void> {
  if (!fonts?.load) return Promise.resolve();
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, wait);
    const done = () => { clearTimeout(timer); resolve(); };
    try { fonts.load(`${FONT_SIZE}px ${MONO_FACE}`).then(done, done); } catch { done(); }
  });
}

// One wait for all terminals of the page: the font loads once. After it, and
// where a browser cannot load fonts by script at all, a terminal opens at once.
let monoFont: Promise<void> | null = null;
let monoFontSettled = false;

/** For tests only: forget that the font has loaded, so that the next terminal waits for it again. */
export function resetMonoFontWait() {
  monoFont = null;
  monoFontSettled = false;
}

/** WebSocket close code the server sends when the session has no terminal slot left. */
export const CLOSE_TERMINAL_LIMIT = 4429;

/** Tab actions a focused terminal can trigger from the keyboard. */
export type TerminalShortcut = "new" | "close" | "next" | "previous" | { select: number };

/** True on macOS, where Option is the third-level shift that types | [ ] { } @ \ ~ on many layouts. */
export function isMac(nav: { platform?: string; userAgentData?: { platform?: string } } = navigator): boolean {
  // navigator.platform first: it is what xterm.js decides on, and the two must agree.
  return /mac/i.test(nav.platform || nav.userAgentData?.platform || "");
}

/** The modifier keys of the tab shortcuts, for display: "Ctrl+Option" on macOS, "Alt+Shift" elsewhere. */
export function shortcutModifiers(mac = isMac()): string {
  return mac ? "Ctrl+Option" : "Alt+Shift";
}

type ShortcutEvent = Pick<KeyboardEvent, "type" | "altKey" | "shiftKey" | "ctrlKey" | "metaKey" | "code">
  & { getModifierState?: (key: string) => boolean };

/**
 * The tab shortcuts: modifiers + T/W/arrows/1-9, matched on `code` so they sit
 * on the same physical keys on every layout.
 *
 * The modifiers must never be ones a keyboard layout uses to type characters,
 * or the shortcut swallows them:
 *  - macOS: Ctrl+Option. Option and Option+Shift type characters there
 *    (German: Option+7 is |, Option+Shift+7 is \), Ctrl+Option types nothing.
 *  - elsewhere: Alt+Shift. Ctrl+Alt is AltGr on Windows, so it is out, and a
 *    key pressed with AltGr is never a shortcut.
 */
export function shortcutOf(ev: ShortcutEvent, mac = isMac()): TerminalShortcut | null {
  if (ev.type !== "keydown" || ev.metaKey || !ev.altKey) return null;
  if (mac ? !ev.ctrlKey || ev.shiftKey : !ev.shiftKey || ev.ctrlKey) return null;
  if (!mac && ev.getModifierState?.("AltGraph")) return null;
  switch (ev.code) {
    case "KeyT": return "new";
    case "KeyW": return "close";
    case "ArrowRight": return "next";
    case "ArrowLeft": return "previous";
  }
  const digit = /^Digit([1-9])$/.exec(ev.code);
  return digit ? { select: Number(digit[1]) } : null;
}

/**
 * Browser terminal: one shell in the session's `candidate@terminal` container.
 * Every mounted Terminal is its own shell with its own scrollback; all of them
 * share the sandbox. Unmounting closes the WebSocket, which ends the shell.
 *
 * `active` is false for a terminal in a background tab: it stays connected and
 * keeps its scrollback, but does not take focus or "Run" commands.
 */
export function Terminal({ sessionId, active = true, onShortcut }: {
  sessionId: string; active?: boolean; onShortcut?: (s: TerminalShortcut) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<"connecting" | "open" | "closed" | "limit">("connecting");
  const [attempt, setAttempt] = useState(0);
  // The effect below owns the xterm instance; these let the other effects and
  // callbacks reach the current one without re-creating it.
  const live = useRef<{ activate: () => void } | null>(null);
  const activeRef = useRef(active);
  activeRef.current = active;
  const shortcutRef = useRef(onShortcut);
  shortcutRef.current = onShortcut;

  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const term = new XTerm({
      theme: terminalTheme(), fontFamily: FONT_FAMILY,
      fontSize: FONT_SIZE, lineHeight: 1.25, scrollback: 10000, allowProposedApi: true,
      cursorBlink: !window.matchMedia?.("(prefers-reduced-motion: reduce)").matches,
      // No macOptionIsMeta: it would send ESC+key for Option+key, and layouts that
      // type | @ [ ] { } \ ~ with Option (German, French, …) could not enter them.
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.loadAddon(new WebLinksAddon());
    term.attachCustomKeyEventHandler((ev) => {
      const shortcut = shortcutRef.current ? shortcutOf(ev) : null;
      if (!shortcut) return true;
      ev.preventDefault();
      shortcutRef.current?.(shortcut);
      return false; // not for the shell
    });

    let alive = true;
    // Undoes what opening set up; nothing before the terminal has opened.
    let close: () => void = () => undefined;

    const open = () => {
      term.open(el);
      try { fit.fit(); } catch { /* not visible yet */ }

      const proto = location.protocol === "https:" ? "wss" : "ws";
      const ws = new WebSocket(`${proto}://${location.host}/api/sessions/${sessionId}/terminal?cols=${term.cols}&rows=${term.rows}`);
      ws.binaryType = "arraybuffer";
      const enc = new TextEncoder();

      ws.onopen = () => {
        if (!alive) return;
        setState("open");
        ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
        if (activeRef.current) term.focus();
      };
      ws.onmessage = (ev) => term.write(typeof ev.data === "string" ? ev.data : new Uint8Array(ev.data as ArrayBuffer));
      ws.onclose = (ev) => { if (alive) setState(ev.code === CLOSE_TERMINAL_LIMIT ? "limit" : "closed"); };
      const send = (text: string) => {
        if (ws.readyState === WebSocket.OPEN) { ws.send(enc.encode(text)); term.focus(); }
      };
      const unregister = registerTerminal(send);
      live.current = {
        activate: () => {
          activateTerminal(send);
          // The tab was hidden: its size may be stale, and it needs the keyboard.
          try { fit.fit(); } catch { /* hidden */ }
          term.focus();
        },
      };
      if (activeRef.current) activateTerminal(send);
      const onData = term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)));
      const onResize = term.onResize(({ cols, rows }) =>
        ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ type: "resize", cols, rows })));

      let raf = 0;
      const ro = new ResizeObserver(() => {
        cancelAnimationFrame(raf);
        raf = requestAnimationFrame(() => { try { fit.fit(); } catch { /* hidden */ } });
      });
      ro.observe(el);

      close = () => {
        live.current = null;
        cancelAnimationFrame(raf);
        ro.disconnect();
        unregister();
        onData.dispose();
        onResize.dispose();
        ws.close();
      };
    };

    // xterm measures its cells on opening, so the font has to be there first.
    if (monoFontSettled || !document.fonts?.load) {
      open();
    } else {
      monoFont ??= whenMonoFontReady().then(() => { monoFontSettled = true; });
      void monoFont.then(() => { if (alive) open(); });
    }

    return () => {
      alive = false;
      close();
      term.dispose();
    };
  }, [sessionId, attempt]);

  // Becoming the visible tab: take the keyboard and the "Run" commands.
  useEffect(() => {
    if (!active) return;
    const raf = requestAnimationFrame(() => live.current?.activate());
    return () => cancelAnimationFrame(raf);
  }, [active, attempt]);

  return (
    <div className="term-surface relative h-full w-full bg-term">
      <div ref={host} className="h-full w-full" />
      {(state === "closed" || state === "limit") && (
        <div role="status" className="absolute inset-x-0 bottom-0 flex flex-wrap items-center justify-between gap-x-3 gap-y-2 border-t border-term-line bg-term-line px-4 py-2.5 text-sm text-term-text">
          <span className="min-w-0 flex-1 basis-56">{state === "limit"
            ? t("This session has reached its terminal limit. Close another terminal, then try again.")
            : t("The terminal session ended.")}</span>
          <button onClick={() => { setState("connecting"); setAttempt((n) => n + 1); }}
            className="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg bg-term-accent px-3 text-label font-semibold text-term-on-accent hover:brightness-110">
            <RotateCw className="size-3.5" aria-hidden /> {t("New terminal")}
          </button>
        </div>
      )}
    </div>
  );
}
