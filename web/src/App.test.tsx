import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { cka } from "./fixtures";

const server = { broken: false };
vi.mock("./api", () => ({
  api: {
    tracks: async () => {
      // A list the pages cannot render: what a server of another version could send.
      if (server.broken) return [{ id: "x" }];
      return [cka];
    },
    path: async () => null,
    sessions: async () => [],
    progress: async () => ({ track: "cka", readiness: 0, passPercent: 66, attempts: 0, domains: [], questions: {}, exams: [], recommendations: null }),
    config: async () => ({ version: "0.5.0", tutor: false, user: "", maxTerminals: 4 }),
  },
}));

import { App } from "./App";

const open = (path: string) => render(<MemoryRouter initialEntries={[path]}><App /></MemoryRouter>);

afterEach(() => { server.broken = false; cleanup(); });

describe("App", () => {
  it("shows the start page at the root", async () => {
    open("/");
    expect(await screen.findByRole("heading", { name: cka.title })).toBeTruthy();
    expect(screen.queryByText("Page not found")).toBeNull();
  });

  it("shows a 404 page for an address that leads nowhere, with the way back", async () => {
    open("/no/such/page");
    expect(screen.getByRole("heading", { level: 1, name: "Page not found" })).toBeTruthy();
    // The address is shown, so a typo can be seen.
    expect(screen.getByText("/no/such/page")).toBeTruthy();
    const main = within(screen.getByRole("main"));
    expect(main.getByRole("link", { name: "Back to the start page" }).getAttribute("href")).toBe("/");
    // It is a page like the others: brand link, footer.
    expect(screen.getByRole("link", { name: "kubedojo" }).getAttribute("href")).toBe("/");
    expect(await within(screen.getByRole("contentinfo")).findByText("kubedojo v0.5.0")).toBeTruthy();
    // And it is not the start page any more.
    expect(screen.queryByRole("heading", { name: cka.title })).toBeNull();
    expect(screen.queryByText("Tracks")).toBeNull();
  });

  it("catches a page that fails to render and offers a way on", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    server.broken = true;
    open("/");
    expect(await screen.findByRole("heading", { level: 1, name: "Something went wrong" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Reload" })).toBeTruthy();
  });
});
