// Lets any part of the UI type into a terminal ("Run" buttons on commands).
// With several terminal tabs open, the text goes to the active one — the tab
// the learner is looking at. Without an active one, the most recently mounted
// terminal receives it.
type Sender = (text: string) => void;

const senders: Sender[] = [];
const listeners = new Set<() => void>();
let active: Sender | null = null;

export function registerTerminal(send: Sender): () => void {
  senders.push(send);
  listeners.forEach((l) => l());
  return () => {
    const i = senders.indexOf(send);
    if (i >= 0) senders.splice(i, 1);
    if (active === send) active = null;
    listeners.forEach((l) => l());
  };
}

/** Marks a registered terminal as the one that receives "Run" commands. */
export function activateTerminal(send: Sender): void {
  if (senders.includes(send)) active = send;
}

export function hasTerminal(): boolean {
  return senders.length > 0;
}

/** Types the command into the terminal and presses Enter. */
export function runInTerminal(command: string): boolean {
  const send = active ?? senders[senders.length - 1];
  if (!send) return false;
  // Comment-only lines are explanation, not input.
  const lines = command.split("\n").map((l) => l.trimEnd()).filter((l) => l.trim() !== "" && !l.trimStart().startsWith("#"));
  if (lines.length === 0) return false;
  send(lines.join("\n") + "\n");
  return true;
}

export function onTerminalChange(fn: () => void): () => void {
  listeners.add(fn);
  return () => { listeners.delete(fn); };
}
