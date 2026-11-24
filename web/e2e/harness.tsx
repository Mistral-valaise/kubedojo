// A page with nothing but the real Terminal component, for keyboard.spec.ts.
// Served by the Vite dev server only; it is not part of the production build.
import { createRoot } from "react-dom/client";
import "@xterm/xterm/css/xterm.css";
// The font faces of the application: the terminal waits for its font before
// it opens, and the tests must go through that wait as a learner's browser does.
import "../src/styles/fonts.css";
import { Terminal, type TerminalShortcut } from "../src/components/Terminal";

const shortcuts: TerminalShortcut[] = [];
(window as unknown as { shortcuts: TerminalShortcut[] }).shortcuts = shortcuts;

createRoot(document.getElementById("root")!).render(
  <Terminal sessionId="e2e" onShortcut={(s) => shortcuts.push(s)} />,
);
