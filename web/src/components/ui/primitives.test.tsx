import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { ButtonLink, EmptyState, IconButton, Loading, SectionLabel, Skeleton } from "../ui";

afterEach(cleanup);

describe("Skeleton", () => {
  it("is decoration in the shape it is given, and its region says that something loads", () => {
    render(<Loading label="Loading the tracks"><Skeleton className="h-4 w-40" /></Loading>);
    const region = screen.getByRole("status", { name: "Loading the tracks" });
    expect(region.getAttribute("aria-busy")).toBe("true");
    const bone = region.firstElementChild as HTMLElement;
    expect(bone.getAttribute("aria-hidden")).toBe("true");
    expect(bone.className.split(" ")).toEqual(expect.arrayContaining(["skeleton", "h-4", "w-40"]));
  });

  it("stops pulsing for who asks for reduced motion", async () => {
    // The pulse is the .skeleton animation of base.css, which the reduced-motion block switches off.
    const css = (await import("../../styles/base.css?raw")).default;
    expect(css).toMatch(/\.skeleton\s*\{[^}]*animation:\s*skeleton-fade/);
    const reduced = css.slice(css.indexOf("@media (prefers-reduced-motion: reduce)"));
    expect(reduced).toMatch(/\.skeleton[^{]*\{\s*animation:\s*none/);
  });
});

describe("EmptyState", () => {
  it("shows what is missing, why, and the one thing to do", () => {
    const again = vi.fn();
    render(<EmptyState icon={<svg data-testid="icon" />} title="No tracks yet" text="Nothing to practise." action={<button onClick={again}>Check again</button>} />);
    expect(screen.getByRole("heading", { level: 2, name: "No tracks yet" })).toBeTruthy();
    expect(screen.getByText("Nothing to practise.")).toBeTruthy();
    // The icon is decoration: the title says it all.
    expect(screen.getByTestId("icon").parentElement?.getAttribute("aria-hidden")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Check again" }));
    expect(again).toHaveBeenCalledTimes(1);
  });

  it("takes the heading level of the place it stands in, and needs neither text nor action", () => {
    const { container } = render(<EmptyState level={1} title="Page not found" />);
    expect(screen.getByRole("heading", { level: 1, name: "Page not found" })).toBeTruthy();
    expect(container.querySelector("p")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("IconButton", () => {
  it("is named by its label, which is also its tooltip, and is at least 36 px square", () => {
    const click = vi.fn();
    render(<IconButton aria-label="Close" onClick={click}><svg /></IconButton>);
    const button = screen.getByRole("button", { name: "Close" });
    expect(button.getAttribute("title")).toBe("Close");
    expect(button.getAttribute("type")).toBe("button");
    expect(button.className.split(" ")).toContain("size-9");
    fireEvent.click(button);
    expect(click).toHaveBeenCalledTimes(1);
  });

  it("has a larger size for page headers and an outline for a button among text", () => {
    render(<IconButton aria-label="Next" size="lg" variant="outline" title="Next question" disabled><svg /></IconButton>);
    const button = screen.getByRole("button", { name: "Next" }) as HTMLButtonElement;
    expect(button.className.split(" ")).toEqual(expect.arrayContaining(["size-10", "border"]));
    expect(button.getAttribute("title")).toBe("Next question");
    expect(button.disabled).toBe(true);
  });

  it("cannot be written without a name", () => {
    // @ts-expect-error an icon alone says nothing to a screen reader: aria-label is required
    render(<IconButton><svg /></IconButton>);
  });
});

describe("SectionLabel", () => {
  it("is a plain label by default and a heading where it titles a section", () => {
    render(<><SectionLabel>References</SectionLabel><SectionLabel as="h2" tone="accent">Domains</SectionLabel></>);
    expect(screen.getByText("References").tagName).toBe("DIV");
    const heading = screen.getByRole("heading", { level: 2, name: "Domains" });
    expect(heading.className.split(" ")).toEqual(expect.arrayContaining(["text-xs", "font-semibold", "uppercase", "tracking-wider", "text-accent"]));
    expect(screen.getByText("References").className.split(" ")).toContain("text-muted");
  });

  it("names a form control as a label", () => {
    render(<><SectionLabel as="label" htmlFor="note">Notes</SectionLabel><textarea id="note" /></>);
    expect(screen.getByLabelText("Notes").tagName).toBe("TEXTAREA");
  });

  it("shows an icon before the text", () => {
    render(<SectionLabel icon={<svg data-testid="icon" />}>Hint 1</SectionLabel>);
    const label = screen.getByText("Hint 1");
    expect(within(label).getByTestId("icon")).toBe(label.firstElementChild);
  });
});

describe("ButtonLink", () => {
  it("is a link of the router that looks like a button", () => {
    render(<MemoryRouter><ButtonLink to="/t/cka" variant="primary" size="lg">Open track</ButtonLink></MemoryRouter>);
    const link = screen.getByRole("link", { name: "Open track" });
    expect(link.getAttribute("href")).toBe("/t/cka");
    // A real link, not a button inside one.
    expect(link.querySelector("button")).toBeNull();
    expect(link.className.split(" ")).toEqual(expect.arrayContaining(["h-11", "bg-accent", "text-on-accent"]));
  });
});
