// A page with nothing but the real Terminal component, for keyboard.spec.ts.
// Served by the Vite dev server only; it is not part of the production build.
import { createRoot } from "react-dom/client";
import "@xterm/xterm/css/xterm.css";
import { Terminal, type TerminalShortcut } from "../src/components/Terminal";

const shortcuts: TerminalShortcut[] = [];
(window as unknown as { shortcuts: TerminalShortcut[] }).shortcuts = shortcuts;

createRoot(document.getElementById("root")!).render(
  <Terminal sessionId="e2e" onShortcut={(s) => shortcuts.push(s)} />,
);
