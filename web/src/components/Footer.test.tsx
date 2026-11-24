import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, within } from "@testing-library/react";

const config = vi.fn();
vi.mock("../api", () => ({ api: { config: () => config() } }));

import { resetConfigCache } from "../config";
import { versionLabel } from "../project";
import { Footer } from "./Footer";

const server = (version: string) => ({ version, tutor: false, user: "", maxTerminals: 4 });
const settle = () => act(async () => { await Promise.resolve(); });
const footer = () => within(screen.getByRole("contentinfo"));

beforeEach(() => { config.mockReset().mockResolvedValue(server("0.5.0")); resetConfigCache(); });
afterEach(cleanup);

describe("Footer", () => {
  it("names the version the server reports", async () => {
    render(<Footer />);
    expect(await footer().findByText("kubedojo v0.5.0")).toBeTruthy();
  });

  it("leaves the version out while it is loading and when it cannot be loaded", async () => {
    config.mockReturnValue(new Promise(() => undefined));
    render(<Footer />);
    expect(footer().getByText("kubedojo")).toBeTruthy();
    cleanup();

    resetConfigCache();
    config.mockReset().mockRejectedValue(new Error("offline"));
    render(<Footer />);
    await settle();
    expect(footer().getByText("kubedojo")).toBeTruthy();
    expect(footer().queryByText(/kubedojo v/)).toBeNull();
  });

  it("links the source and the author in a new tab, without handing over the opener", async () => {
    render(<Footer />);
    await settle();
    const source = footer().getByRole("link", { name: "Open source on GitHub" });
    const author = footer().getByRole("link", { name: "Created by Mistral N" });
    expect(source.getAttribute("href")).toBe("https://github.com/Mistral-valaise/kubedojo");
    expect(author.getAttribute("href")).toBe("https://mistraln.de");
    for (const a of [source, author]) {
      expect(a.getAttribute("target")).toBe("_blank");
      expect(a.getAttribute("rel")).toBe("noopener noreferrer");
    }
    // The author's link leaves the application and says so with an icon; the icon is not read out.
    expect(author.querySelector("svg")?.getAttribute("aria-hidden")).toBe("true");
    expect(footer().getAllByRole("link")).toHaveLength(2);
  });
});

describe("versionLabel", () => {
  it("writes a release number with one v, and anything else as it is", () => {
    expect(versionLabel("0.5.0")).toBe("kubedojo v0.5.0");
    expect(versionLabel("v0.5.0")).toBe("kubedojo v0.5.0");
    expect(versionLabel("v0.4.0-12-gabc1234-dirty")).toBe("kubedojo v0.4.0-12-gabc1234-dirty");
    expect(versionLabel("dev")).toBe("kubedojo dev");
    expect(versionLabel("")).toBe("kubedojo");
    expect(versionLabel(null)).toBe("kubedojo");
    expect(versionLabel(undefined)).toBe("kubedojo");
  });
});
