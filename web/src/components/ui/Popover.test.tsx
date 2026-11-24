import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Popover } from "../ui";

afterEach(cleanup);

function Example() {
  return (
    <>
      <input aria-label="elsewhere" />
      <p>page</p>
      <Popover label="About" trigger={(props, open) => <button {...props}>{open ? "Hide" : "Show"}</button>}>
        {(close) => <><a href="https://example.org">A link</a><button onClick={close}>Done</button></>}
      </Popover>
    </>
  );
}
const trigger = () => screen.getByRole("button", { name: /Show|Hide/ });
const panel = () => screen.queryByRole("dialog", { name: "About" });

describe("Popover", () => {
  it("opens and closes on a click on its trigger, and says so", () => {
    render(<Example />);
    expect(panel()).toBeNull();
    expect(trigger().getAttribute("aria-expanded")).toBe("false");
    expect(trigger().getAttribute("aria-haspopup")).toBe("dialog");

    fireEvent.click(trigger());
    expect(trigger().getAttribute("aria-expanded")).toBe("true");
    expect(trigger().getAttribute("aria-controls")).toBe(panel()?.id);
    expect(trigger().textContent).toBe("Hide");
    expect(screen.getByRole("link", { name: "A link" })).toBeTruthy();

    fireEvent.click(trigger());
    expect(panel()).toBeNull();
    expect(trigger().getAttribute("aria-expanded")).toBe("false");
  });

  it("closes on Escape and gives the focus back to the trigger", () => {
    render(<Example />);
    fireEvent.click(trigger());
    screen.getByRole("link", { name: "A link" }).focus();
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it("ignores other keys", () => {
    render(<Example />);
    fireEvent.click(trigger());
    fireEvent.keyDown(document.body, { key: "Enter" });
    fireEvent.keyDown(document.body, { key: "a" });
    expect(panel()).not.toBeNull();
  });

  it("closes on a click outside and returns the focus that would be lost", () => {
    render(<Example />);
    fireEvent.click(trigger());
    screen.getByRole("button", { name: "Done" }).focus();
    fireEvent.pointerDown(screen.getByText("page"));
    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it("stays open for a click inside, and leaves the focus where a click outside has put it", () => {
    render(<Example />);
    fireEvent.click(trigger());
    fireEvent.pointerDown(screen.getByRole("link", { name: "A link" }));
    expect(panel()).not.toBeNull();

    const elsewhere = screen.getByRole("textbox", { name: "elsewhere" });
    elsewhere.focus();
    fireEvent.pointerDown(elsewhere);
    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(elsewhere);
  });

  it("closes when the focus leaves it with the Tab key", () => {
    render(<Example />);
    fireEvent.click(trigger());
    const done = screen.getByRole("button", { name: "Done" });
    done.focus();
    fireEvent.blur(done, { relatedTarget: screen.getByRole("textbox", { name: "elsewhere" }) });
    expect(panel()).toBeNull();
  });

  it("lets its content close it", () => {
    render(<Example />);
    fireEvent.click(trigger());
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(panel()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it("removes its listeners with itself", () => {
    const view = render(<Example />);
    fireEvent.click(trigger());
    view.unmount();
    // Nothing is left to react: neither of these may throw.
    fireEvent.keyDown(document.body, { key: "Escape" });
    fireEvent.pointerDown(document.body);
  });
});
