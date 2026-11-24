import { describe, expect, it } from "vitest";
import terminal from "../components/Terminal.tsx?raw";
import terminalTabs from "../components/TerminalTabs.tsx?raw";
import provisioning from "../components/Provisioning.tsx?raw";
import tokens from "./tokens.css?raw";
import base from "./base.css?raw";
import markdown from "./markdown.css?raw";
import terminalCss from "./terminal.css?raw";

// Colours and font sizes live in tokens.css. These scans keep them from
// creeping back into the files that once had their own.

const HEX = /#[0-9a-fA-F]{3,8}\b/g;
/** A Tailwind font size or colour given as a value in square brackets instead of by the name of a token. */
const AD_HOC = /\b(?:text|bg|border|ring|fill|stroke)-\[(?:#|\d)[^\]]*\]/g;

describe("the terminal surfaces", () => {
  it.each([
    ["Terminal.tsx", terminal], ["TerminalTabs.tsx", terminalTabs], ["Provisioning.tsx", provisioning],
  ])("%s has no colour of its own", (_name, source) => {
    expect(source.match(HEX) ?? []).toEqual([]);
    expect(source.match(AD_HOC) ?? []).toEqual([]);
    expect(source).not.toMatch(/\b(?:text|bg|border)-(?:white|black)\b/);
  });

  it("take the xterm colours from tokens that tokens.css defines", () => {
    const used = [...terminal.matchAll(/"(--term[a-z-]*)"/g)].map((m) => m[1]);
    // background, foreground, cursor, selection and the 16 ANSI colours.
    expect(new Set(used).size).toBeGreaterThanOrEqual(20);
    for (const token of used) expect(tokens, token).toMatch(new RegExp(`${token}:\\s*#[0-9a-fA-F]{6};`));
  });

  it("are the same in both themes", () => {
    const dark = tokens.slice(tokens.indexOf(':root[data-theme="dark"]'), tokens.indexOf("@theme"));
    expect(dark).toContain("--bg:");
    expect(dark).not.toContain("--term");
  });
});

describe("the stylesheets", () => {
  it("keep raw colours in tokens.css", () => {
    for (const [name, css] of [["base.css", base], ["markdown.css", markdown], ["terminal.css", terminalCss]] as const) {
      expect(css.match(HEX) ?? [], name).toEqual([]);
    }
  });

  it("define every token for both themes", () => {
    const names = (block: string) => [...block.matchAll(/^\s*(--[a-z0-9-]+):/gm)].map((m) => m[1]);
    const light = tokens.slice(tokens.indexOf(":root {"), tokens.indexOf(':root[data-theme="dark"]'));
    const dark = tokens.slice(tokens.indexOf(':root[data-theme="dark"]'), tokens.indexOf("@theme"));
    const themed = names(light).filter((n) => !n.startsWith("--term"));
    expect(names(dark).sort()).toEqual(themed.sort());
    for (const token of ["--on-accent", "--on-ok", "--on-bad"]) expect(themed).toContain(token);
  });

  it("have a type scale with 12 px as its smallest step", () => {
    const sizes = [...tokens.matchAll(/^\s*--text-[a-z0-9]+:\s*([\d.]+)rem;/gm)].map((m) => Number(m[1]) * 16);
    expect(sizes.length).toBeGreaterThanOrEqual(8);
    expect(Math.min(...sizes)).toBe(12);
  });
});

/** The colours of one theme: the name of each token and its #rrggbb value. */
function palette(theme: "light" | "dark"): Record<string, string> {
  const at = tokens.indexOf(':root[data-theme="dark"]');
  const block = theme === "light" ? tokens.slice(tokens.indexOf(":root {"), at) : tokens.slice(at, tokens.indexOf("@theme"));
  return Object.fromEntries([...block.matchAll(/^\s*--([a-z0-9-]+):\s*(#[0-9a-fA-F]{6});/gm)].map((m) => [m[1], m[2]]));
}

/** The contrast ratio of two colours, as WCAG 2 defines it. */
function contrast(a: string, b: string): number {
  const luminance = (hex: string) => {
    const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
      .map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

describe.each(["light", "dark"] as const)("the %s theme", (theme) => {
  const c = palette(theme);

  it.each([
    ...["bg", "panel", "panel-2", "code", "code-inline", "accent-soft", "ok-soft", "bad-soft", "warn-soft"].flatMap((s) => [["text", s], ["muted", s]]),
    ...["accent", "ok", "bad", "warn"].flatMap((f) => ["bg", "panel", "panel-2", `${f}-soft`].map((s) => [f, s])),
    ["on-accent", "accent"], ["on-ok", "ok"], ["on-bad", "bad"],
    ["code-inline-text", "code-inline"], ["code-inline-text", "accent-soft"], ["code-inline-text", "ok-soft"],
  ])("reads %s on %s at 4.5:1 or more", (fg, bg) => {
    expect(c[fg], fg).toBeDefined();
    expect(c[bg], bg).toBeDefined();
    expect(contrast(c[fg], c[bg])).toBeGreaterThanOrEqual(4.5);
  });

  // Inline code is told apart from the text around it by its colour, not
  // only by its surface, which is close to the surfaces it sits on.
  it("gives inline code a colour of its own", () => {
    expect(c["code-inline-text"]).not.toBe(c.text);
    expect(c["code-inline"]).not.toBe(c.code);
    for (const s of ["panel", "panel-2", "bg"]) expect(c["code-inline"]).not.toBe(c[s]);
  });
});
