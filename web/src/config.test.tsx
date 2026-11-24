import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";

const config = vi.fn();
vi.mock("./api", () => ({ api: { config: () => config() } }));

import { loadConfig, resetConfigCache, useConfig } from "./config";

const server = (maxTerminals: number) => ({ version: "test", tutor: true, user: "", maxTerminals });

function Show({ name, enabled }: { name: string; enabled?: boolean }) {
  const c = useConfig(enabled);
  return <div data-testid={name}>{c ? `max ${c.maxTerminals}` : "loading"}</div>;
}
const text = (name: string) => screen.getByTestId(name).textContent;
const settle = () => act(async () => { await Promise.resolve(); });

beforeEach(() => { config.mockReset().mockResolvedValue(server(4)); resetConfigCache(); });
afterEach(cleanup);

describe("useConfig", () => {
  it("fetches the configuration once for everything that uses it", async () => {
    render(<><Show name="a" /><Show name="b" /></>);
    expect(text("a")).toBe("loading");
    await settle();
    expect([text("a"), text("b")]).toEqual(["max 4", "max 4"]);
    // Mounted later: there at once, without a request and without a loading state.
    render(<Show name="c" />);
    expect(text("c")).toBe("max 4");
    expect(await loadConfig()).toEqual(server(4));
    expect(config).toHaveBeenCalledTimes(1);
  });

  it("stays null when the configuration cannot be loaded, and tries again for the next user", async () => {
    config.mockRejectedValueOnce(new Error("offline"));
    render(<Show name="a" />);
    await settle();
    expect(text("a")).toBe("loading");
    render(<Show name="b" />);
    await settle();
    expect(text("b")).toBe("max 4");
    expect(config).toHaveBeenCalledTimes(2);
  });

  it("does not fetch while it is not enabled", async () => {
    const view = render(<Show name="a" enabled={false} />);
    await settle();
    expect(text("a")).toBe("loading");
    expect(config).not.toHaveBeenCalled();
    view.rerender(<Show name="a" enabled />);
    await settle();
    expect(text("a")).toBe("max 4");
  });

  it("forgets the configuration on a reset", async () => {
    render(<Show name="a" />);
    await settle();
    resetConfigCache();
    config.mockResolvedValue(server(2));
    render(<Show name="b" />);
    expect(text("b")).toBe("loading");
    await settle();
    expect(text("b")).toBe("max 2");
  });
});
