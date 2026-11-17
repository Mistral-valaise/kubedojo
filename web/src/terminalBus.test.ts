import { beforeEach, describe, expect, it, vi } from "vitest";

// The bus is module state: a fresh copy per test.
async function bus() {
  vi.resetModules();
  return import("./terminalBus");
}

describe("terminalBus", () => {
  beforeEach(() => vi.resetModules());

  it("has no terminal until one registers", async () => {
    const b = await bus();
    expect(b.hasTerminal()).toBe(false);
    expect(b.runInTerminal("kubectl get pods")).toBe(false);
  });

  it("types into the only terminal and presses Enter (single-terminal behaviour)", async () => {
    const b = await bus();
    const send = vi.fn();
    b.registerTerminal(send);
    expect(b.runInTerminal("kubectl get pods")).toBe(true);
    expect(send).toHaveBeenCalledWith("kubectl get pods\n");
  });

  it("drops comment lines and blank lines", async () => {
    const b = await bus();
    const send = vi.fn();
    b.registerTerminal(send);
    b.runInTerminal("# explanation\n\nkubectl get nodes  \n  # more\nkubectl get ns");
    expect(send).toHaveBeenCalledWith("kubectl get nodes\nkubectl get ns\n");
    expect(b.runInTerminal("# only a comment")).toBe(false);
  });

  it("types into the active terminal, not the most recently opened one", async () => {
    const b = await bus();
    const one = vi.fn(), two = vi.fn(), three = vi.fn();
    b.registerTerminal(one);
    b.registerTerminal(two);
    b.registerTerminal(three);
    b.activateTerminal(one);
    b.runInTerminal("id");
    expect(one).toHaveBeenCalledWith("id\n");
    expect(two).not.toHaveBeenCalled();
    expect(three).not.toHaveBeenCalled();

    b.activateTerminal(two);
    b.runInTerminal("pwd");
    expect(two).toHaveBeenCalledWith("pwd\n");
    expect(one).toHaveBeenCalledTimes(1);
  });

  it("falls back to the newest terminal when the active one is closed", async () => {
    const b = await bus();
    const one = vi.fn(), two = vi.fn();
    b.registerTerminal(one);
    const closeTwo = b.registerTerminal(two);
    b.activateTerminal(two);
    closeTwo();
    b.runInTerminal("id");
    expect(two).not.toHaveBeenCalled();
    expect(one).toHaveBeenCalledWith("id\n");
  });

  it("ignores activation of a terminal that is not registered", async () => {
    const b = await bus();
    const open = vi.fn(), gone = vi.fn();
    b.registerTerminal(open);
    b.registerTerminal(gone)();
    b.activateTerminal(gone);
    b.runInTerminal("id");
    expect(gone).not.toHaveBeenCalled();
    expect(open).toHaveBeenCalled();
  });

  it("notifies listeners when terminals come and go", async () => {
    const b = await bus();
    const changed = vi.fn();
    const off = b.onTerminalChange(changed);
    const unregister = b.registerTerminal(vi.fn());
    unregister();
    expect(changed).toHaveBeenCalledTimes(2);
    off();
    b.registerTerminal(vi.fn());
    expect(changed).toHaveBeenCalledTimes(2);
  });
});
