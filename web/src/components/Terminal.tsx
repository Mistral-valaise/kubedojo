import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { RotateCw } from "lucide-react";
import { t } from "../i18n";
import { activateTerminal, registerTerminal } from "../terminalBus";

const theme = {
  background: "#0a0e17", foreground: "#d7deea", cursor: "#5b9dff", cursorAccent: "#0a0e17",
  selectionBackground: "#2a3b5e",
  black: "#1b2233", red: "#ff6b7a", green: "#3ddc97", yellow: "#f5b942", blue: "#5b9dff",
  magenta: "#c792ea", cyan: "#56d4dd", white: "#d7deea",
  brightBlack: "#5a6784", brightRed: "#ff8b97", brightGreen: "#6ee7b1", brightYellow: "#ffd479",
  brightBlue: "#86b7ff", brightMagenta: "#d9b3f5", brightCyan: "#86e5ec", brightWhite: "#ffffff",
};

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
      theme, fontFamily: '"JetBrains Mono", ui-monospace, "SF Mono", Menlo, Consolas, monospace',
      fontSize: 13.5, lineHeight: 1.25, cursorBlink: true, scrollback: 10000, allowProposedApi: true,
      // No macOptionIsMeta: it would send ESC+key for Option+key, and layouts that
      // type | @ [ ] { } \ ~ with Option (German, French, …) could not enter them.
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.loadAddon(new WebLinksAddon());
    term.open(el);
    try { fit.fit(); } catch { /* not visible yet */ }
    term.attachCustomKeyEventHandler((ev) => {
      const shortcut = shortcutRef.current ? shortcutOf(ev) : null;
      if (!shortcut) return true;
      ev.preventDefault();
      shortcutRef.current?.(shortcut);
      return false; // not for the shell
    });

    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/api/sessions/${sessionId}/terminal?cols=${term.cols}&rows=${term.rows}`);
    ws.binaryType = "arraybuffer";
    const enc = new TextEncoder();
    let alive = true;

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

    return () => {
      alive = false;
      live.current = null;
      cancelAnimationFrame(raf);
      ro.disconnect();
      unregister();
      onData.dispose();
      onResize.dispose();
      ws.close();
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
    <div className="relative h-full w-full bg-[#0a0e17]">
      <div ref={host} className="h-full w-full" />
      {(state === "closed" || state === "limit") && (
        <div role="status" className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-3 border-t border-white/10 bg-[#141b2b] px-4 py-2.5 text-sm text-[#d7deea]">
          <span>{state === "limit"
            ? t("This session has reached its terminal limit. Close another terminal, then try again.")
            : t("The terminal session ended.")}</span>
          <button onClick={() => { setState("connecting"); setAttempt((n) => n + 1); }}
            className="inline-flex items-center gap-1.5 rounded-md bg-[#5b9dff] px-2.5 py-1 text-[13px] font-medium text-white hover:brightness-110">
            <RotateCw className="size-3.5" /> {t("New terminal")}
          </button>
        </div>
      )}
    </div>
  );
}
