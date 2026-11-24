import { StrictMode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { Button, Modal } from "../ui";

afterEach(cleanup);

/** A page with one button that opens a confirmation. */
function Example({ refuse = false, footer = true }: { refuse?: boolean; footer?: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Reset question</button>
      {open && (
        <Modal title="Reset this question?" onClose={() => !refuse && setOpen(false)}
          footer={footer ? <><Button onClick={() => setOpen(false)}>Cancel</Button><Button variant="danger" onClick={() => setOpen(false)}>Reset</Button></> : undefined}>
          Everything this question created is removed.
        </Modal>
      )}
    </>
  );
}
const opener = () => screen.getByRole("button", { name: "Reset question" });
const openIt = () => { opener().focus(); fireEvent.click(opener()); return screen.getByRole("dialog") as HTMLDialogElement; };

describe("Modal", () => {
  it("is a native dialog, opened as a modal and named by its title", () => {
    const showModal = vi.spyOn(HTMLDialogElement.prototype, "showModal");
    render(<Example />);
    const dialog = openIt();
    expect(dialog.tagName).toBe("DIALOG");
    // showModal, not show: that is what traps the focus and makes the page behind inert.
    expect(showModal).toHaveBeenCalledTimes(1);
    expect(dialog.hasAttribute("open")).toBe(true);
    expect(screen.getByRole("dialog", { name: "Reset this question?" })).toBe(dialog);
    const title = within(dialog).getByRole("heading", { name: "Reset this question?" });
    expect(dialog.getAttribute("aria-labelledby")).toBe(title.id);
    expect(within(dialog).getByText("Everything this question created is removed.")).toBeTruthy();
  });

  it("opens with the focus inside, on the cautious choice", () => {
    render(<Example />);
    const dialog = openIt();
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Cancel" }));
  });

  it("puts the focus on its close button when it has no other control", () => {
    render(<Example footer={false} />);
    const dialog = openIt();
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Close" }));
  });

  it("closes on Escape and returns the focus to what opened it", () => {
    render(<Example />);
    const dialog = openIt();
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(dialog.isConnected).toBe(false);
    expect(document.activeElement).toBe(opener());
  });

  it("closes on the browser's own request to close it", () => {
    render(<Example />);
    const dialog = openIt();
    // What a browser sends for Escape and for the back gesture of a phone.
    const cancel = new Event("cancel", { cancelable: true });
    fireEvent(dialog, cancel);
    // The owner decides about closing, so the browser must not close it by itself.
    expect(cancel.defaultPrevented).toBe(true);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener());
  });

  it("closes with its close button, with its own actions and on a click on the backdrop", () => {
    render(<Example />);
    fireEvent.click(within(openIt()).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener());

    fireEvent.click(within(openIt()).getByRole("button", { name: "Reset" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    // A press on the dialog element itself is a press beside the box.
    fireEvent.mouseDown(openIt());
    expect(screen.queryByRole("dialog")).toBeNull();
    // A press inside the box is not.
    const dialog = openIt();
    fireEvent.mouseDown(within(dialog).getByText("Everything this question created is removed."));
    expect(screen.getByRole("dialog")).toBe(dialog);
  });

  it("stays open when its owner refuses to close it", () => {
    render(<Example refuse />);
    const dialog = openIt();
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(screen.getByRole("dialog")).toBe(dialog);
    expect(dialog.hasAttribute("open")).toBe(true);
  });

  it("opens again when the browser closed it against the will of its owner", async () => {
    render(<Example refuse />);
    const dialog = openIt();
    dialog.close();
    expect(dialog.hasAttribute("open")).toBe(false);
    await Promise.resolve();
    expect(dialog.hasAttribute("open")).toBe(true);
  });

  it("ignores other keys", () => {
    render(<Example />);
    const dialog = openIt();
    fireEvent.keyDown(document.activeElement!, { key: "Enter" });
    fireEvent.keyDown(document.activeElement!, { key: "2" });
    expect(screen.getByRole("dialog")).toBe(dialog);
  });
});

describe("Modal as a sheet", () => {
  function Sheet() {
    const [open, setOpen] = useState(false);
    return (
      <>
        <button onClick={() => setOpen(true)}>Reset question</button>
        {open && (
          <Modal sheet title="Assistant" onClose={() => setOpen(false)}>
            <a href="/settings">model</a>
            <button onClick={() => setOpen(false)}>Close the assistant</button>
            <textarea data-autofocus aria-label="Ask the assistant" />
          </Modal>
        )}
      </>
    );
  }

  it("is named by its title without showing it, and leaves header and close button to its content", () => {
    render(<Sheet />);
    const dialog = openIt();
    expect(screen.getByRole("dialog", { name: "Assistant" })).toBe(dialog);
    expect(dialog.getAttribute("aria-labelledby")).toBeNull();
    expect(within(dialog).queryByRole("heading")).toBeNull();
    expect(within(dialog).queryByRole("button", { name: "Close" })).toBeNull();
    // Escape and the backdrop still close it.
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener());
  });

  it("starts the focus on the element that asks for it, not on the first control", () => {
    render(<Sheet />);
    const dialog = openIt();
    expect(document.activeElement).toBe(within(dialog).getByRole("textbox", { name: "Ask the assistant" }));
  });
});

describe("Modal in React's strict mode", () => {
  it("stays open: the close of the first run of its effect does not end the second", async () => {
    // A real dialog reports close() one task later, when the effect has run again and opened it anew.
    const close = vi.spyOn(HTMLDialogElement.prototype, "close").mockImplementation(function close(this: HTMLDialogElement) {
      if (!this.hasAttribute("open")) return;
      this.removeAttribute("open");
      setTimeout(() => this.dispatchEvent(new Event("close")), 0);
    });
    render(<StrictMode><Example /></StrictMode>);
    openIt();
    await new Promise((r) => setTimeout(r, 5));
    expect(screen.getByRole("dialog").hasAttribute("open")).toBe(true);
    // A close that is one (the browser closed the dialog) is still asked about.
    const dialog = screen.getByRole("dialog") as HTMLDialogElement;
    dialog.removeAttribute("open");
    fireEvent(dialog, new Event("close"));
    expect(screen.queryByRole("dialog")).toBeNull();
    close.mockRestore();
  });
});
