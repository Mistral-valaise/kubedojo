import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";

const config = vi.fn();
vi.mock("../api", () => ({ api: { config: () => config() } }));

import { resetConfigCache } from "../config";
import { HeaderPager, WorkspaceHeader } from "./WorkspaceHeader";

beforeEach(() => { config.mockReset().mockResolvedValue({ version: "0.5.0", tutor: false, user: "", maxTerminals: 4 }); resetConfigCache(); });
afterEach(cleanup);

const show = (ui: ReactElement) => render(<MemoryRouter>{ui}</MemoryRouter>);
const header = () => screen.getByRole("banner");
const docs = { title: "Kubernetes docs", url: "https://kubernetes.io/docs/" };
/** An item of the header by its role, its name, the name of the button it wraps (the About popover) or its text. */
const nameOf = (x: Element) => x.getAttribute("role") ?? x.getAttribute("aria-label") ?? x.querySelector("button")?.getAttribute("aria-label") ?? x.textContent;
/** What the header row consists of. */
const row = () => [...header().children].map(nameOf);

describe("WorkspaceHeader", () => {
  it("shows the brand, the About button and the theme toggle when nothing else is given", () => {
    show(<WorkspaceHeader />);
    expect(header().className).toBe("flex h-13 shrink-0 items-center gap-3 border-b bg-panel px-4");
    expect(screen.getByRole("link", { name: "kubedojo" }).getAttribute("href")).toBe("/");
    // The brand stands alone: no slash, and it stays on a phone.
    expect(header().firstElementChild).toBe(screen.getByRole("link", { name: "kubedojo" }));
    expect(screen.queryByText("/")).toBeNull();
    const group = header().lastElementChild as HTMLElement;
    expect(group.className).toBe("ml-auto flex shrink-0 items-center gap-1");
    expect([...group.children].map(nameOf)).toEqual(["About kubedojo", "Dark theme"]);
    expect(header().children).toHaveLength(2);
  });

  it("starts with a skip link to the main content, before the header", () => {
    show(<WorkspaceHeader />);
    const skip = screen.getByRole("link", { name: "Skip to main content" });
    expect(skip.getAttribute("href")).toBe("#main");
    expect(skip.compareDocumentPosition(header()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(header().contains(skip)).toBe(false);
  });

  it("switches the theme and says which one is on", () => {
    document.documentElement.dataset.theme = "dark";
    show(<WorkspaceHeader />);
    const toggle = () => screen.getByRole("button", { name: "Dark theme" });
    expect(toggle().getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(toggle());
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(toggle().getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(toggle());
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(toggle().getAttribute("aria-pressed")).toBe("true");
  });

  it("renders every slot, the controls as one group on the right", () => {
    show(
      <WorkspaceHeader back={<a href="/t/cka">back</a>} title={<span>title</span>} nav={<span>nav</span>} docs={docs}
        actions={<button>action</button>} end={<button>end</button>}>
        <span>extra</span>
      </WorkspaceHeader>,
    );
    // With something to its right the brand hides on a phone and gets a slash.
    const brand = header().firstElementChild as HTMLElement;
    expect(brand.className).toBe("hidden items-center gap-3 sm:flex");
    expect(within(brand).getByRole("link", { name: "kubedojo" })).toBeTruthy();
    expect(within(brand).getByText("/")).toBeTruthy();

    const group = header().lastElementChild as HTMLElement;
    expect(group.className).toBe("ml-auto flex shrink-0 items-center gap-1");
    expect([...header().children].slice(1).map((x) => (x === group ? "group" : x.textContent))).toEqual(["back", "title", "extra", "group"]);
    // In the group: navigation, documentation, actions, About, theme toggle and the closing action, in this order.
    expect([...group.children].map(nameOf)).toEqual(["nav", "Docs", "action", "About kubedojo", "Dark theme", "end"]);

    const link = screen.getByRole("link", { name: "Docs" });
    expect(link.getAttribute("href")).toBe(docs.url);
    expect(link.getAttribute("title")).toBe(docs.title);
    expect(link.getAttribute("target")).toBe("_blank");
    // Text only, and not on a phone.
    expect(link.className).toContain("hidden");
    expect(link.querySelector("svg")).toBeNull();
  });

  it("lays the controls out in the row after a centre element", () => {
    show(
      <WorkspaceHeader title={<span>CKA</span>} center={<div role="timer" className="ml-auto">45:00</div>} docs={docs}
        actions={<button aria-label="action" />} end={<button aria-label="Finish exam" />} />,
    );
    // No group: the centre element pushes itself and what follows to the right.
    expect(row().slice(1)).toEqual(["CKA", "timer", "Docs", "action", "About kubedojo", "Dark theme", "Finish exam"]);
    const link = screen.getByRole("link", { name: "Docs" });
    expect(link.getAttribute("href")).toBe(docs.url);
    // The link keeps an icon on a phone, where its text is hidden.
    expect(link.className).not.toMatch(/(^| )hidden( |$)/);
    expect(link.querySelector("svg")).toBeTruthy();
    expect(within(link).getByText("Docs").className).toBe("hidden sm:inline");
  });

  it("links no documentation when there is none", () => {
    show(<WorkspaceHeader back={<a href="/">back</a>} nav={<span>nav</span>} docs={null} />);
    expect(screen.queryByText("Docs")).toBeNull();
    expect(screen.getByRole("button", { name: "Dark theme" })).toBeTruthy();
  });

  it("opens the credit of the project from the About button", async () => {
    show(<WorkspaceHeader />);
    const about = screen.getByRole("button", { name: "About kubedojo" });
    expect(about.getAttribute("aria-expanded")).toBe("false");
    // Nothing is fetched for a popover nobody opens.
    expect(config).not.toHaveBeenCalled();
    fireEvent.click(about);
    expect(about.getAttribute("aria-expanded")).toBe("true");
    const panel = screen.getByRole("dialog", { name: "About kubedojo" });
    expect(about.getAttribute("aria-controls")).toBe(panel.id);
    expect(await within(panel).findByText("kubedojo v0.5.0")).toBeTruthy();

    const source = within(panel).getByRole("link", { name: "Open source on GitHub" });
    expect(source.getAttribute("href")).toBe("https://github.com/Mistral-valaise/kubedojo");
    const author = within(panel).getByRole("link", { name: "Created by Mistral N" });
    expect(author.getAttribute("href")).toBe("https://mistraln.de");
    for (const a of [source, author]) {
      expect(a.getAttribute("target")).toBe("_blank");
      expect(a.getAttribute("rel")).toBe("noopener noreferrer");
    }

    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(about.getAttribute("aria-expanded")).toBe("false");
  });

  it("names the project without a version until the server has told it", () => {
    config.mockReturnValue(new Promise(() => undefined));
    show(<WorkspaceHeader />);
    fireEvent.click(screen.getByRole("button", { name: "About kubedojo" }));
    const panel = screen.getByRole("dialog", { name: "About kubedojo" });
    expect(within(panel).getByText("kubedojo")).toBeTruthy();
    expect(within(panel).queryByText(/kubedojo v/)).toBeNull();
  });

  it("shows the About button everywhere, or from the sm breakpoint on where the header is full", () => {
    const wrapper = () => screen.getByRole("button", { name: "About kubedojo" }).parentElement as HTMLElement;
    show(<WorkspaceHeader />);
    expect(wrapper().className).not.toContain("max-sm:hidden");
    cleanup();
    show(<WorkspaceHeader about="sm" />);
    expect(wrapper().className.split(" ")).toContain("max-sm:hidden");
  });
});

describe("HeaderPager", () => {
  it("moves to the previous and the next item", () => {
    const previous = vi.fn(), next = vi.fn();
    show(<HeaderPager position="2 / 3" previousLabel="Previous question" nextLabel="Next question" onPrevious={previous} onNext={next} />);
    expect(screen.getByText("2 / 3")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Previous question" }));
    fireEvent.click(screen.getByRole("button", { name: "Next question" }));
    fireEvent.click(screen.getByRole("button", { name: "Next question" }));
    expect(previous).toHaveBeenCalledTimes(1);
    expect(next).toHaveBeenCalledTimes(2);
  });

  it("disables the direction that has no item", () => {
    const next = vi.fn();
    show(<HeaderPager position="1 / 3" previousLabel="Previous question" nextLabel="Next question" onNext={next} />);
    expect(screen.getByRole("button", { name: "Previous question" })).toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: "Next question" })).toHaveProperty("disabled", false);
  });
});
