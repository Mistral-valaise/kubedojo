import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const rate = vi.fn();
vi.mock("../api", () => ({ api: { rate: (...args: unknown[]) => rate(...args) } }));

import { Rating } from "./Rating";

const stars = () => screen.getAllByRole("radio");
const star = (n: number) => stars()[n - 1];
const checked = () => stars().findIndex((s) => s.getAttribute("aria-checked") === "true") + 1;
/** Which stars the Tab key stops at. */
const tabStops = () => stars().map((s, i) => (s.tabIndex === 0 ? i + 1 : 0)).filter(Boolean);

beforeEach(() => { rate.mockReset().mockResolvedValue({}); });
afterEach(cleanup);

describe("Rating", () => {
  it("is a radio group of five named stars with one tab stop", () => {
    render(<Rating track="cka" question="q1" />);
    expect(screen.getByRole("radiogroup", { name: "Rating" })).toBeTruthy();
    expect(stars().map((s) => s.getAttribute("aria-label"))).toEqual(["1 of 5 stars", "2 of 5 stars", "3 of 5 stars", "4 of 5 stars", "5 of 5 stars"]);
    expect(checked()).toBe(0);
    // Before a choice the group is entered at its first star.
    expect(tabStops()).toEqual([1]);
  });

  it("moves and selects with the arrow keys, around both ends", () => {
    render(<Rating track="cka" question="q1" />);
    star(1).focus();
    fireEvent.keyDown(star(1), { key: "ArrowRight" });
    expect(checked()).toBe(2);
    expect(document.activeElement).toBe(star(2));
    expect(tabStops()).toEqual([2]);

    fireEvent.keyDown(star(2), { key: "ArrowDown" });
    expect(checked()).toBe(1);
    fireEvent.keyDown(star(1), { key: "ArrowLeft" });
    expect(checked()).toBe(5);
    expect(document.activeElement).toBe(star(5));
    fireEvent.keyDown(star(5), { key: "ArrowUp" });
    expect(checked()).toBe(1);
    expect(document.activeElement).toBe(star(1));
  });

  it("jumps to the ends with Home and End and leaves other keys alone", () => {
    render(<Rating track="cka" question="q1" />);
    star(1).focus();
    fireEvent.keyDown(star(1), { key: "End" });
    expect(checked()).toBe(5);
    fireEvent.keyDown(star(5), { key: "Home" });
    expect(checked()).toBe(1);
    expect(document.activeElement).toBe(star(1));
    fireEvent.keyDown(star(1), { key: "a" });
    fireEvent.keyDown(star(1), { key: "Tab" });
    expect(checked()).toBe(1);
  });

  it("selects on a click and keeps the chosen star as the tab stop", () => {
    render(<Rating track="cka" question="q1" />);
    fireEvent.click(star(4));
    expect(checked()).toBe(4);
    expect(tabStops()).toEqual([4]);
  });

  it("sends the stars with the comment and thanks", async () => {
    render(<Rating track="cka" question="q1" />);
    expect(screen.queryByRole("textbox")).toBeNull();
    fireEvent.click(star(2));
    fireEvent.change(screen.getByRole("textbox", { name: "Comment" }), { target: { value: "The task text is unclear." } });
    fireEvent.click(screen.getByRole("button", { name: "Send feedback" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toMatch(/^Thanks/));
    expect(rate).toHaveBeenCalledWith("cka", "q1", 2, "The task text is unclear.");
    expect(screen.queryByRole("radiogroup")).toBeNull();
  });

  it("says so when the feedback could not be sent, and keeps what was entered", async () => {
    rate.mockRejectedValue(new Error("offline"));
    render(<Rating track="cka" question="q1" />);
    fireEvent.click(star(5));
    fireEvent.click(screen.getByRole("button", { name: "Send feedback" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Could not send. Try again.");
    expect(checked()).toBe(5);
  });
});
