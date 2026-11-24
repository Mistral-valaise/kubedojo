import { expect, test, type Page } from "@playwright/test";
import { mockApi, mockTerminal, type MockServer } from "./mock";

// The assistant and its settings in a real browser, against a mocked API:
// what jsdom cannot show is which panes are in view at a width, where real
// key presses go, and that the page talks to nothing but its own origin.

/** Notes every request that goes anywhere but to the origin the application is served from. */
function watchOrigin(page: Page, baseURL: string | undefined): string[] {
  const own = new URL(baseURL ?? "http://127.0.0.1").origin;
  const foreign: string[] = [];
  page.on("request", (r) => {
    const url = new URL(r.url());
    if (url.protocol.startsWith("http") && url.origin !== own) foreign.push(r.url());
  });
  return foreign;
}
const pane = (page: Page, id: string) => page.locator(`#split-pane-${id}`);

async function failTheTask(page: Page) {
  await page.goto("/t/cka/q/fix-rollout");
  await page.getByRole("button", { name: "Check my solution" }).click();
  await expect(page.getByText("2 of 3 checks passed")).toBeVisible();
}
/** The answer of the mocked assistant has arrived, whole. */
async function answered(page: Page, server: MockServer) {
  const log = page.getByRole("log", { name: "Conversation" });
  await expect(log.getByText("What went wrong, and what should I look at next?")).toBeVisible();
  await expect(log.getByText("Looked at: task, check results, cluster snapshot, failing checks")).toBeVisible();
  await expect(log.getByText("They are in ImagePullBackOff", { exact: false })).toBeVisible();
  await expect(log.locator("pre code")).toHaveText("kubectl -n shop describe pod -l app=web | tail -n 12");
  await expect(log.getByText("4.2 s · 164 tokens")).toBeVisible();
  // The default question is asked with an empty message, once.
  expect(server.asked).toEqual([{ message: "" }]);
}

test("a failed check is explained in the assistant column, next to task and terminal", async ({ page, baseURL }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const foreign = watchOrigin(page, baseURL);
  const server = await mockApi(page);
  const typed = await mockTerminal(page);
  await failTheTask(page);

  const toggle = page.getByRole("button", { name: "Assistant", exact: true });
  await expect(toggle).toHaveAttribute("aria-pressed", "false");
  await expect(pane(page, "assistant")).toBeHidden();

  await page.getByRole("button", { name: "Explain what went wrong" }).click();
  await expect(toggle).toHaveAttribute("aria-pressed", "true");
  await answered(page, server);
  // Three columns: nothing gave way to the assistant.
  for (const id of ["left", "right", "assistant"]) await expect(pane(page, id)).toBeVisible();
  await expect(page.getByRole("link", { name: "llama3.2 · Ollama" })).toHaveAttribute("href", "/settings");
  await expect(page.getByRole("status").filter({ hasText: "Answer ready" })).toHaveCount(1);

  // The command of the answer runs in the terminal.
  await page.getByRole("log").getByRole("button", { name: "Run in terminal" }).click();
  await expect.poll(() => typed.join("")).toBe("kubectl -n shop describe pod -l app=web | tail -n 12\n");

  // A follow-up question, typed and sent with Enter.
  const input = page.getByRole("textbox", { name: "Ask the assistant" });
  await input.fill("Which tag is right?");
  await input.press("Enter");
  await expect.poll(() => server.asked).toEqual([{ message: "" }, { message: "Which tag is right?" }]);
  await expect(input).toHaveValue("");

  // Closed again, the terminal has the room back, and the choice is remembered.
  await page.getByRole("button", { name: "Close the assistant" }).click();
  await expect(pane(page, "assistant")).toBeHidden();
  await expect(toggle).toBeFocused();
  expect(await page.evaluate(() => localStorage.getItem("kubedojo.assistant.open"))).toBe("0");
  expect(server.unknown).toEqual([]);
  expect(foreign).toEqual([]);
});

test("on a medium screen the assistant takes the place of the task and the terminal stays", async ({ page }) => {
  await page.setViewportSize({ width: 1000, height: 768 });
  const server = await mockApi(page);
  await mockTerminal(page);
  await failTheTask(page);
  await page.getByRole("button", { name: "Explain what went wrong" }).click();
  await answered(page, server);
  await expect(pane(page, "assistant")).toBeVisible();
  await expect(pane(page, "right")).toBeVisible();
  await expect(pane(page, "left")).toBeHidden();
  // The way back to the task is the switcher above the left column.
  await page.getByRole("tab", { name: "Task" }).click();
  await expect(pane(page, "left")).toBeVisible();
  await expect(pane(page, "assistant")).toBeHidden();
  await expect(pane(page, "right")).toBeVisible();
  // No pane is wider than the window: nothing scrolls sideways.
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test("on a phone the assistant is a tab of its own", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const server = await mockApi(page);
  await mockTerminal(page);
  await failTheTask(page);
  await expect(page.getByRole("tab")).toHaveText(["Task", "Terminal"]);
  await page.getByRole("button", { name: "Explain what went wrong" }).click();
  await expect(page.getByRole("tab")).toHaveText(["Task", "Terminal", "Assistant"]);
  await expect(page.getByRole("tab", { name: "Assistant" })).toHaveAttribute("aria-selected", "true");
  await answered(page, server);
  await expect(pane(page, "left")).toBeHidden();
  await expect(pane(page, "right")).toBeHidden();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test("typing into the assistant does not answer the quiz", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await mockApi(page);
  await page.goto("/t/kcna/q/placing-pods");
  await expect(page.getByRole("radiogroup")).toBeVisible();
  await page.getByRole("button", { name: "Assistant", exact: true }).click();
  const input = page.getByRole("textbox", { name: "Ask the assistant" });
  await expect(input).toBeFocused();
  await page.keyboard.type("Is it option 2 or 3?");
  await expect(input).toHaveValue("Is it option 2 or 3?");
  await expect(page.getByRole("radio", { checked: true })).toHaveCount(0);
  // Escape leaves the draft alone.
  await page.keyboard.press("Escape");
  await expect(input).toHaveValue("Is it option 2 or 3?");
  // Outside the input the number keys are the quiz's again.
  await page.getByRole("heading", { level: 1 }).click();
  await page.keyboard.press("2");
  await expect(page.getByRole("radio", { name: "kube-scheduler" })).toBeChecked();
});

test("the settings are tested and saved, and only what changed is sent", async ({ page, baseURL }) => {
  const foreign = watchOrigin(page, baseURL);
  const server = await mockApi(page);
  await page.goto("/settings");
  await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
  await expect(page.getByLabel("Base URL", { exact: true })).toHaveValue("http://localhost:11434/v1");
  const save = page.getByRole("button", { name: "Save" });
  await expect(save).toBeDisabled();

  await page.getByRole("button", { name: "LM Studio" }).click();
  await expect(page.getByLabel("Base URL", { exact: true })).toHaveValue("http://localhost:1234/v1");
  await page.getByLabel("Model", { exact: true }).fill("qwen2.5-coder");
  await expect(page.getByText("Unsaved changes")).toBeVisible();

  await page.getByRole("button", { name: "Test connection" }).click();
  await expect(page.getByText("Connected to qwen2.5-coder in 412 ms")).toBeVisible();
  expect(server.tested).toEqual([{ provider: "openai", model: "qwen2.5-coder", baseURL: "http://localhost:1234/v1" }]);
  expect(server.saved).toEqual([]);

  const configBefore = server.configRequests;
  await save.click();
  await expect(page.getByText("Saved")).toBeVisible();
  // The switch, the provider and the key were not touched: they are not sent.
  expect(server.saved).toEqual([{ model: "qwen2.5-coder", baseURL: "http://localhost:1234/v1" }]);
  await expect(save).toBeDisabled();
  await expect.poll(() => server.configRequests).toBeGreaterThan(configBefore);

  // A key is typed into an empty password field and is never shown again.
  await page.getByRole("button", { name: "Set a key" }).click();
  const key = page.getByLabel("API key", { exact: true });
  await expect(key).toHaveAttribute("type", "password");
  await expect(key).toHaveAttribute("autocomplete", "off");
  await expect(key).toHaveValue("");
  await key.fill("sk-test-0000000000001234");
  await save.click();
  await expect(page.getByText("Set · ends in ••••1234")).toBeVisible();
  expect(server.saved[1]).toEqual({ apiKey: "sk-test-0000000000001234" });
  expect(await page.content()).not.toContain("sk-test-0000000000001234");
  expect(server.unknown).toEqual([]);
  expect(foreign).toEqual([]);
});
