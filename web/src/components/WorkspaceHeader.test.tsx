import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { HeaderPager, WorkspaceHeader } from "./WorkspaceHeader";

afterEach(cleanup);

const show = (ui: ReactElement) => render(<MemoryRouter>{ui}</MemoryRouter>);
const header = () => screen.getByRole("banner");
const docs = { title: "Kubernetes docs", url: "https://kubernetes.io/docs/" };
/** What the header row consists of, by the text or the name of each item. */
const row = () => [...header().children].map((x) => x.getAttribute("role") ?? x.getAttribute("aria-label") ?? x.textContent);

describe("WorkspaceHeader", () => {
  it("shows the brand and the theme toggle when nothing else is given", () => {
    show(<WorkspaceHeader />);
    expect(header().className).toBe("flex h-12 shrink-0 items-center gap-3 border-b bg-bg px-4");
    expect(screen.getByRole("link", { name: "kubedojo" }).getAttribute("href")).toBe("/");
    // The brand stands alone: no slash, and it stays on a phone.
    expect(header().firstElementChild).toBe(screen.getByRole("link", { name: "kubedojo" }));
    expect(screen.queryByText("/")).toBeNull();
    const toggle = screen.getByRole("button", { name: "Toggle theme" });
    expect(toggle.parentElement?.className).toBe("ml-auto");
    expect(header().children).toHaveLength(2);
  });

  it("switches the theme", () => {
    document.documentElement.dataset.theme = "dark";
    show(<WorkspaceHeader />);
    fireEvent.click(screen.getByRole("button", { name: "Toggle theme" }));
    expect(document.documentElement.dataset.theme).toBe("light");
    fireEvent.click(screen.getByRole("button", { name: "Toggle theme" }));
    expect(document.documentElement.dataset.theme).toBe("dark");
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
    // In the group: navigation, documentation, actions, theme toggle and the closing action, in this order.
    expect([...group.children].map((x) => x.getAttribute("aria-label") ?? x.textContent)).toEqual(["nav", "Docs", "action", "Toggle theme", "end"]);

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
    expect(row().slice(1)).toEqual(["CKA", "timer", "Docs", "action", "Toggle theme", "Finish exam"]);
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
    expect(screen.getByRole("button", { name: "Toggle theme" })).toBeTruthy();
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
