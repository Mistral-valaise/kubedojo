import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { RotateCw } from "lucide-react";
import { t } from "../i18n";
import { registerTerminal } from "../terminalBus";

const theme = {
  background: "#0a0e17", foreground: "#d7deea", cursor: "#5b9dff", cursorAccent: "#0a0e17",
  selectionBackground: "#2a3b5e",
  black: "#1b2233", red: "#ff6b7a", green: "#3ddc97", yellow: "#f5b942", blue: "#5b9dff",
  magenta: "#c792ea", cyan: "#56d4dd", white: "#d7deea",
  brightBlack: "#5a6784", brightRed: "#ff8b97", brightGreen: "#6ee7b1", brightYellow: "#ffd479",
  brightBlue: "#86b7ff", brightMagenta: "#d9b3f5", brightCyan: "#86e5ec", brightWhite: "#ffffff",
};

/** Browser terminal attached to the session's `candidate@terminal` container. */
export function Terminal({ sessionId }: { sessionId: string }) {
  const host = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<"connecting" | "open" | "closed">("connecting");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const term = new XTerm({
      theme, fontFamily: '"JetBrains Mono", ui-monospace, "SF Mono", Menlo, Consolas, monospace',
      fontSize: 13.5, lineHeight: 1.25, cursorBlink: true, scrollback: 10000, allowProposedApi: true,
      macOptionIsMeta: true,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.loadAddon(new WebLinksAddon());
    term.open(el);
    try { fit.fit(); } catch { /* not visible yet */ }

    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/api/sessions/${sessionId}/terminal?cols=${term.cols}&rows=${term.rows}`);
    ws.binaryType = "arraybuffer";
    const enc = new TextEncoder();
    let alive = true;

    ws.onopen = () => {
      if (!alive) return;
      setState("open");
      ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
      term.focus();
    };
    ws.onmessage = (ev) => term.write(typeof ev.data === "string" ? ev.data : new Uint8Array(ev.data as ArrayBuffer));
    ws.onclose = () => { if (alive) setState("closed"); };
    const unregister = registerTerminal((text) => {
      if (ws.readyState === WebSocket.OPEN) { ws.send(enc.encode(text)); term.focus(); }
    });
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
      cancelAnimationFrame(raf);
      ro.disconnect();
      unregister();
      onData.dispose();
      onResize.dispose();
      ws.close();
      term.dispose();
    };
  }, [sessionId, attempt]);

  return (
    <div className="relative h-full w-full bg-[#0a0e17]">
      <div ref={host} className="h-full w-full" />
      {state === "closed" && (
        <div className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-3 border-t border-white/10 bg-[#141b2b] px-4 py-2.5 text-sm text-[#d7deea]">
          <span>{t("The terminal session ended.")}</span>
          <button onClick={() => { setState("connecting"); setAttempt((n) => n + 1); }}
            className="inline-flex items-center gap-1.5 rounded-md bg-[#5b9dff] px-2.5 py-1 text-[13px] font-medium text-white hover:brightness-110">
            <RotateCw className="size-3.5" /> {t("New terminal")}
          </button>
        </div>
      )}
    </div>
  );
}
