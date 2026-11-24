import { expect, test } from "@playwright/test";

// xterm.js measures the width of a character cell when it opens. The terminal
// therefore waits for its web font first (whenMonoFontReady in Terminal.tsx);
// opened earlier it would measure the fallback font and keep those cells.

const MONO = '13.5px "JetBrains Mono Variable"';

test("the terminal opens after its web font has loaded, from this origin", async ({ page }) => {
  const fonts: string[] = [];
  page.on("request", (r) => { if (r.resourceType() === "font") fonts.push(r.url()); });
  await page.routeWebSocket(/\/api\/sessions\/e2e\/terminal/, () => undefined);
  // Notes whether the font was usable at the moment xterm.js opened.
  await page.addInitScript((font) => {
    new MutationObserver((_records, observer) => {
      if (!document.querySelector(".xterm")) return;
      (window as unknown as { fontAtOpen: boolean }).fontAtOpen = document.fonts.check(font);
      observer.disconnect();
    }).observe(document, { childList: true, subtree: true });
  }, MONO);

  await page.goto("/e2e/harness.html");
  await expect(page.locator(".xterm-helper-textarea")).toBeAttached();

  expect(await page.evaluate(() => (window as unknown as { fontAtOpen: boolean }).fontAtOpen)).toBe(true);
  // check() is also true for a font nobody declared: make sure the file was really fetched, and from here.
  const mono = fonts.filter((url) => url.includes("jetbrains-mono"));
  expect(mono.length).toBeGreaterThan(0);
  const origin = new URL(page.url()).origin;
  for (const url of fonts) expect(new URL(url).origin, url).toBe(origin);
  // The terminal uses that face.
  expect(await page.locator(".xterm-rows").evaluate((el) => getComputedStyle(el).fontFamily)).toContain("JetBrains Mono Variable");
});

test("the terminal opens with the fallback font when the web font cannot be loaded", async ({ page }) => {
  await page.route(/\.woff2(\?|$)/, (route) => route.abort());
  await page.routeWebSocket(/\/api\/sessions\/e2e\/terminal/, () => undefined);
  await page.goto("/e2e/harness.html");
  // A failed font costs a moment at most, never the terminal.
  await expect(page.locator(".xterm-helper-textarea")).toBeAttached({ timeout: 4000 });
});
