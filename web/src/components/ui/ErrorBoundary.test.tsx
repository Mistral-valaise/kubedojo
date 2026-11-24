import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ErrorBoundary } from "../ui";

const broken = { now: true };
function Sometimes() {
  if (broken.now) throw new Error("track.domains is undefined");
  return <p>all good</p>;
}

beforeEach(() => {
  broken.now = true;
  // React reports the caught error, and so does the boundary: expected here.
  vi.spyOn(console, "error").mockImplementation(() => undefined);
});
afterEach(cleanup);

describe("ErrorBoundary", () => {
  it("shows its children while nothing is wrong", () => {
    broken.now = false;
    render(<ErrorBoundary><Sometimes /></ErrorBoundary>);
    expect(screen.getByText("all good")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows a message with a way on instead of an empty page", () => {
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    render(<ErrorBoundary><Sometimes /></ErrorBoundary>);
    const alert = screen.getByRole("alert");
    expect(screen.getByRole("heading", { level: 1, name: "Something went wrong" })).toBeTruthy();
    expect(alert.textContent).toContain("Your progress and your sandbox are not affected");
    expect(alert.textContent).toContain("track.domains is undefined");
    expect(screen.getByRole("link", { name: "Back to start" }).getAttribute("href")).toBe("/");
    fireEvent.click(screen.getByRole("button", { name: "Reload" }));
    expect(reload).toHaveBeenCalledTimes(1);
    expect(console.error).toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it("lets a part of a page bring its own message and try again", () => {
    render(
      <ErrorBoundary fallback={(error, retry) => <button onClick={retry}>Retry after: {error.message}</button>}>
        <Sometimes />
      </ErrorBoundary>,
    );
    const retry = screen.getByRole("button", { name: "Retry after: track.domains is undefined" });
    broken.now = false;
    fireEvent.click(retry);
    expect(screen.getByText("all good")).toBeTruthy();
  });
});
