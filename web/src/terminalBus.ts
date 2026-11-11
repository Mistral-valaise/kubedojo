// Lets any part of the UI type into the open terminal ("Run" buttons on
// commands). The most recently mounted terminal receives the text.
type Sender = (text: string) => void;

const senders: Sender[] = [];
const listeners = new Set<() => void>();

export function registerTerminal(send: Sender): () => void {
  senders.push(send);
  listeners.forEach((l) => l());
  return () => {
    const i = senders.indexOf(send);
    if (i >= 0) senders.splice(i, 1);
    listeners.forEach((l) => l());
  };
}

export function hasTerminal(): boolean {
  return senders.length > 0;
}

/** Types the command into the terminal and presses Enter. */
export function runInTerminal(command: string): boolean {
  const send = senders[senders.length - 1];
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
