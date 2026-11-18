import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Session } from "../api";
import { cka, kcna, linux, path, progress } from "../fixtures";

const server = { tracks: [linux, kcna, cka], path: path as typeof path | null, sessions: [] as Session[] };
vi.mock("../api", () => ({
  api: {
    tracks: async () => server.tracks,
    path: async () => server.path,
    sessions: async () => server.sessions,
    progress: async (id: string) => progress(id, id === "cka" ? 70 : 0),
  },
}));

import { Home, TrackCard } from "./Home";

const card = (title: string) => screen.getByRole("heading", { name: title }).closest("div.rounded-2xl") as HTMLElement;
const show = (ui: React.ReactNode) => render(<MemoryRouter>{ui}</MemoryRouter>);

beforeEach(() => { server.path = path; server.sessions = []; });
afterEach(cleanup);

describe("TrackCard", () => {
  it("shows the Kubernetes version of a certification track, as before", () => {
    show(<TrackCard track={cka} progress={progress("cka", 70)} />);
    const c = within(card(cka.title));
    expect(c.getByText("Hands-on")).toBeTruthy();
    expect(c.getByText("Kubernetes v1.37")).toBeTruthy();
    expect(c.getByText("Exam readiness")).toBeTruthy();
    expect(c.getByText("0 questions")).toBeTruthy();
    expect(c.getByText("120 min exam")).toBeTruthy();
    expect(c.getByText("70%")).toBeTruthy();
  });

  it("shows no Kubernetes badge on a track without a version", () => {
    show(<TrackCard track={linux} />);
    const c = within(card(linux.title));
    expect(c.getByText("Foundation")).toBeTruthy();
    expect(c.queryByText(/Kubernetes/)).toBeNull();
    expect(c.getByText("0 exercises")).toBeTruthy();
    expect(c.getByText("60 min assessment")).toBeTruthy();
    expect(c.getByText("Readiness")).toBeTruthy();
  });

  it("badges a theory track and names its mock exam", () => {
    show(<TrackCard track={kcna} />);
    const c = within(card(kcna.title));
    expect(c.getByText("Theory")).toBeTruthy();
    expect(c.queryByText(/Kubernetes v/)).toBeNull();
    expect(c.getByText("45 min mock exam")).toBeTruthy();
  });
});

describe("Home", () => {
  it("puts the path above the track cards and keeps the cards in the order the server sent", async () => {
    show(<Home />);
    const title = await screen.findByRole("heading", { name: "Road to Kubestronaut" });
    const cards = screen.getAllByRole("heading", { level: 2 }).filter((h) => h !== title);
    expect(cards.map((h) => h.textContent)).toEqual([linux.title, kcna.title, cka.title]);
    expect(title.compareDocumentPosition(cards[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(await screen.findByRole("link", { name: /^CKA, .* 70% ready$/ })).toBeTruthy();
  });

  it("shows the cards alone when the content has no path", async () => {
    server.path = null;
    show(<Home />);
    expect(await screen.findByRole("heading", { name: cka.title })).toBeTruthy();
    expect(screen.queryByText("Learning path")).toBeNull();
  });

  it("words an active session by the kind of its track", async () => {
    const session = (id: string, track: string, mode: Session["mode"], envs: Record<string, string>): Session => ({
      id, mode, track, status: "running", createdAt: "", durationMinutes: 0, envs, questions: null, log: null, now: "", meta: {},
    });
    server.sessions = [
      session("a", "cka", "exam", { base: "cka1" }), session("b", "linux", "exam", { host: "linux7" }), session("c", "kcna", "exam", {}),
    ];
    show(<Home />);
    expect(await screen.findByText("Clusters: cka1")).toBeTruthy();
    expect(screen.getByText("Sandbox: linux7")).toBeTruthy();
    expect(screen.getByText("No sandbox needed")).toBeTruthy();
    expect(screen.getByText(/CKA · Exam simulation/)).toBeTruthy();
    expect(screen.getByText(/Linux · Skills assessment/)).toBeTruthy();
    expect(screen.getByText(/KCNA · Mock exam/)).toBeTruthy();
    // One of the sessions has no sandbox, so the heading promises none.
    expect(screen.getByText("Active sessions")).toBeTruthy();
  });
});
