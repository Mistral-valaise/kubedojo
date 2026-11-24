import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Progress, Session, Track } from "../api";
import { cka, kcna, linux, path, progress } from "../fixtures";

type Answer<T> = T | Error | Promise<T>;
const server = {
  tracks: [linux, kcna, cka] as Answer<Track[]>,
  path: path as Answer<typeof path | null>,
  sessions: [] as Answer<Session[]>,
  progress: ((id: string) => progress(id, id === "cka" ? 70 : 0)) as (id: string) => Progress | Error,
  // Whether the assistant of the server is switched on.
  assistant: false,
  destroyed: [] as string[],
  destroy: null as null | Error,
  trackRequests: 0,
};
const answer = async <T,>(a: Answer<T>): Promise<T> => { if (a instanceof Error) throw a; return a; };
vi.mock("../api", () => ({
  api: {
    tracks: () => { server.trackRequests++; return answer(server.tracks); },
    path: () => answer(server.path),
    sessions: () => answer(server.sessions),
    progress: (id: string) => answer(server.progress(id)),
    config: async () => ({
      version: "0.5.0", tutor: server.assistant, user: "", maxTerminals: 4,
      assistant: { enabled: server.assistant, configured: server.assistant, provider: "ollama", model: "llama3.2", writable: true },
    }),
    destroy: async (id: string) => {
      if (server.destroy) throw server.destroy;
      server.destroyed.push(id);
      server.sessions = (server.sessions as Session[]).filter((s) => s.id !== id);
    },
  },
}));

import { resetConfigCache } from "../config";
import { continueWith, Home, TrackCard } from "./Home";

const card = (title: string) => screen.getByRole("heading", { name: title }).closest("div.rounded-2xl") as HTMLElement;
const show = (ui: React.ReactNode) => render(<MemoryRouter>{ui}</MemoryRouter>);
const session = (id: string, track: string, mode: Session["mode"], envs: Record<string, string> = {}, status: Session["status"] = "running"): Session => ({
  id, mode, track, status, createdAt: "", durationMinutes: 0, envs, questions: null, log: null, now: "", meta: {},
});
const trackGrid = () => screen.getByRole("region", { name: "Tracks" });
/** The primary and the secondary action of the hero, by name. */
const actions = () => {
  const hero = screen.getByRole("heading", { level: 1 }).parentElement as HTMLElement;
  return within(hero).getAllByRole("link").map((a) => `${a.textContent} -> ${a.getAttribute("href")}`);
};

beforeEach(() => {
  Object.assign(server, {
    tracks: [linux, kcna, cka], path, sessions: [], assistant: false, destroyed: [], destroy: null, trackRequests: 0,
    progress: (id: string) => progress(id, id === "cka" ? 70 : 0),
  });
  resetConfigCache();
});
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

  it("opens its track with a link that names it", () => {
    show(<TrackCard track={cka} />);
    const open = within(card(cka.title)).getByRole("link", { name: "Open track CKA" });
    expect(open.getAttribute("href")).toBe("/t/cka");
    expect(open.textContent).toBe("Open track");
  });
});

describe("Home", () => {
  it("puts the path above the track cards and keeps the cards in the order the server sent", async () => {
    show(<Home />);
    const title = await screen.findByRole("heading", { name: "Road to Kubestronaut" });
    const cards = within(trackGrid()).getAllByRole("heading", { level: 3 });
    expect(cards.map((h) => h.textContent)).toEqual([linux.title, kcna.title, cka.title]);
    expect(title.compareDocumentPosition(cards[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(await screen.findByRole("link", { name: /^CKA, .* 70% ready$/ })).toBeTruthy();
    expect(within(trackGrid()).getByText("3 tracks")).toBeTruthy();
  });

  it("shows the cards alone when the content has no path", async () => {
    server.path = null;
    show(<Home />);
    expect(await screen.findByRole("heading", { name: cka.title })).toBeTruthy();
    expect(screen.queryByText("Learning path")).toBeNull();
  });

  it("words an active session by the kind of its track", async () => {
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

  it("fetches the tracks once", async () => {
    show(<Home />);
    // The progress of every track is there: everything the page loads has arrived.
    expect(await screen.findByRole("link", { name: /^CKA, .* 70% ready$/ })).toBeTruthy();
    expect(server.trackRequests).toBe(1);
  });
});

describe("Home while loading and when something is missing", () => {
  it("shows card-shaped placeholders while the tracks load, then the cards", async () => {
    let arrive: (list: Track[]) => void = () => undefined;
    server.tracks = new Promise<Track[]>((resolve) => { arrive = resolve; });
    show(<Home />);
    const loading = screen.getByRole("status", { name: "Loading the tracks" });
    expect(loading.getAttribute("aria-busy")).toBe("true");
    expect(loading.querySelectorAll("div.rounded-2xl")).toHaveLength(3);
    expect(loading.querySelectorAll(".skeleton").length).toBeGreaterThan(3);
    // The path card and the main action wait in shape too: no lone spinner anywhere.
    expect(screen.getByRole("status", { name: "Loading the learning path" })).toBeTruthy();
    expect(screen.getByRole("status", { name: "Loading your progress" })).toBeTruthy();
    expect(document.querySelector(".spin")).toBeNull();
    expect(screen.queryByRole("link", { name: /Browse all tracks/ })).toBeNull();

    await act(async () => { arrive([linux, kcna, cka]); });
    expect(await screen.findByRole("heading", { name: cka.title })).toBeTruthy();
    expect(screen.queryByRole("status", { name: "Loading the tracks" })).toBeNull();
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  });

  it("says so when the server has no tracks", async () => {
    server.tracks = [];
    server.path = null;
    show(<Home />);
    expect(await screen.findByRole("heading", { name: "No tracks yet" })).toBeTruthy();
    expect(within(trackGrid()).queryAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual(["No tracks yet"]);
    expect(screen.getByRole("button", { name: "Check again" })).toBeTruthy();
    // Nothing to continue and nothing to browse to but the empty grid.
    await waitFor(() => expect(actions()).toEqual(["Browse all tracks -> #tracks"]));
  });

  it("shows why the tracks are missing and loads them again on request", async () => {
    server.tracks = new Error("connection refused");
    show(<Home />);
    expect(await screen.findByText("The tracks could not be loaded.")).toBeTruthy();
    expect(screen.getByText("connection refused")).toBeTruthy();
    expect(screen.queryByRole("status", { name: "Loading the tracks" })).toBeNull();
    server.tracks = [cka];
    fireEvent.click(within(trackGrid()).getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("heading", { name: cka.title })).toBeTruthy();
    expect(screen.queryByText("The tracks could not be loaded.")).toBeNull();
  });

  it("names a failed load of the progress, the path and the sessions, and keeps the cards", async () => {
    server.progress = () => new Error("progress down");
    server.path = new Error("path down");
    server.sessions = new Error("sessions down");
    show(<Home />);
    expect(await screen.findByText(/^Your progress could not be loaded/)).toBeTruthy();
    expect(screen.getByText("The learning path could not be loaded.")).toBeTruthy();
    expect(screen.getByText("Your active sessions could not be loaded.")).toBeTruthy();
    for (const detail of ["progress down", "path down", "sessions down"]) expect(screen.getByText(detail)).toBeTruthy();
    // The tracks themselves loaded: their cards are there, without a readiness.
    expect(within(card(cka.title)).getByText("–")).toBeTruthy();
    expect(screen.queryByText("Learning path")).toBeNull();
    // Nothing is known to continue, so browsing is the main action.
    expect(actions()).toEqual(["Browse all tracks -> #tracks"]);

    server.progress = (id) => progress(id, id === "cka" ? 70 : 0);
    fireEvent.click(within(screen.getByText(/^Your progress could not be loaded/).closest("div.rounded-xl") as HTMLElement).getByRole("button", { name: "Try again" }));
    expect(await within(card(cka.title)).findByText("70%")).toBeTruthy();
    expect(screen.queryByText(/^Your progress could not be loaded/)).toBeNull();
  });
});

describe("the main action of Home", () => {
  it("continues an open session, and offers the tracks second", async () => {
    server.sessions = [session("e1", "kcna", "exam")];
    show(<Home />);
    await waitFor(() => expect(actions()).toEqual(["Continue with KCNA -> /exam/e1", "Browse all tracks -> #tracks"]));
    // The link target exists: the track grid.
    expect(trackGrid().id).toBe("tracks");
  });

  it("continues a practice session on its track page", async () => {
    server.sessions = [session("p1", "linux", "practice", { host: "linux7" }, "ready")];
    show(<Home />);
    await waitFor(() => expect(actions()).toEqual(["Continue with Linux -> /t/linux", "Browse all tracks -> #tracks"]));
  });

  it("continues the track the learner has come furthest in when no session is open", async () => {
    server.progress = (id) => progress(id, { linux: 100, kcna: 35, cka: 70 }[id] ?? 0);
    show(<Home />);
    // Linux is complete: there is nothing left to continue there.
    await waitFor(() => expect(actions()).toEqual(["Continue with CKA -> /t/cka", "Browse all tracks -> #tracks"]));
  });

  it("offers the tracks as the one action when there is nothing to continue", async () => {
    server.progress = (id) => progress(id, 0);
    show(<Home />);
    await waitFor(() => expect(actions()).toEqual(["Browse all tracks -> #tracks"]));
    expect(screen.queryByRole("link", { name: /^Continue with/ })).toBeNull();
  });

  it("derives where to continue from sessions before progress", () => {
    const tracks = [linux, kcna, cka];
    const pr = { linux: progress("linux", 100), kcna: progress("kcna", 35), cka: progress("cka", 70) };
    expect(continueWith(tracks, [session("e1", "kcna", "exam")], pr)).toEqual({ track: kcna, to: "/exam/e1" });
    // A session that failed or is gone is nothing to continue; neither is one of an unknown track.
    expect(continueWith(tracks, [session("x", "kcna", "exam", {}, "error"), session("y", "gone", "exam")], pr)).toEqual({ track: cka, to: "/t/cka" });
    expect(continueWith(tracks, [], { linux: progress("linux", 100) })).toBeNull();
    // Attempts count as progress even at 0% readiness.
    expect(continueWith(tracks, null, { kcna: { ...progress("kcna", 0), attempts: 2 } })).toEqual({ track: kcna, to: "/t/kcna" });
    expect(continueWith(tracks, null, null)).toBeNull();
  });
});

describe("ending a session on Home", () => {
  it("reads the list again once the server has ended the session", async () => {
    server.sessions = [session("a", "cka", "exam", { base: "cka1" })];
    show(<Home />);
    fireEvent.click(await screen.findByRole("button", { name: "End" }));
    await waitFor(() => expect(screen.queryByText("Clusters: cka1")).toBeNull());
    expect(server.destroyed).toEqual(["a"]);
    expect(screen.queryByText("Active sandboxes")).toBeNull();
  });

  it("says so when the session could not be ended, and keeps it listed", async () => {
    server.sessions = [session("a", "cka", "exam", { base: "cka1" })];
    server.destroy = new Error("sandbox busy");
    show(<Home />);
    fireEvent.click(await screen.findByRole("button", { name: "End" }));
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText(/The session could not be ended/)).toBeTruthy();
    expect(within(alert).getByText("sandbox busy")).toBeTruthy();
    expect(screen.getByText("Clusters: cka1")).toBeTruthy();
    expect((screen.getByRole("button", { name: "End" }) as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("the tiles of Home", () => {
  const tiles = () => within(screen.getByRole("region", { name: "How it works" })).getAllByRole("heading", { level: 3 }).map((h) => h.textContent);

  it("leaves the assistant out where the server offers none", async () => {
    show(<Home />);
    await screen.findByRole("heading", { name: cka.title });
    expect(tiles()).toEqual(["Learn", "Practise", "Simulate"]);
  });

  it("presents the assistant where the server offers one", async () => {
    server.assistant = true;
    show(<Home />);
    await waitFor(() => expect(tiles()).toEqual(["Learn", "Practise", "Ask", "Simulate"]));
    expect(screen.getByText(/^Stuck on a failed check\?/)).toBeTruthy();
  });
});

describe("the chrome of a page", () => {
  it("ends with the footer: version, source and author", async () => {
    show(<Home />);
    const footer = screen.getByRole("contentinfo");
    expect(await within(footer).findByText("kubedojo v0.5.0")).toBeTruthy();
    const source = within(footer).getByRole("link", { name: "Open source on GitHub" });
    expect(source.getAttribute("href")).toBe("https://github.com/Mistral-valaise/kubedojo");
    const author = within(footer).getByRole("link", { name: "Created by Mistral N" });
    expect(author.getAttribute("href")).toBe("https://mistraln.de");
    // The footer follows the main content.
    expect(screen.getByRole("main").compareDocumentPosition(footer) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("starts with a skip link to the main content", async () => {
    show(<Home />);
    const skip = screen.getByRole("link", { name: "Skip to main content" });
    expect(skip.getAttribute("href")).toBe("#main");
    expect(screen.getByRole("main").id).toBe("main");
    expect(document.querySelector("a")).toBe(skip);
    await screen.findByRole("heading", { name: cka.title });
  });
});
