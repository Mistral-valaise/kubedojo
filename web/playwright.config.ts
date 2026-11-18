import { defineConfig } from "@playwright/test";

// Browser tests for what jsdom cannot show: real key events going through the
// real xterm.js. They need no backend; the terminal WebSocket is mocked.
const port = 5199;

export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL: `http://127.0.0.1:${port}`, browserName: "chromium" },
  webServer: {
    command: `npx vite --host 127.0.0.1 --port ${port} --strictPort`,
    url: `http://127.0.0.1:${port}/e2e/harness.html`,
    reuseExistingServer: !process.env.CI,
  },
});
